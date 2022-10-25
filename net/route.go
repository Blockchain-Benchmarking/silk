package net


import (
	"bytes"
	"io"
	sio "silk/io"
	"sync"
)


// ----------------------------------------------------------------------------


// A one-to-many communication link.
//
type Route interface {
	Server

	// Broadcast a `Message` encoded with the given `Protocol` to all
	// remote peers. This is not limited to the accepted peers.
	//
	Send(Message, Protocol) error
}


// Create a trivial `Route` over a set of `Connection`s.
// Broadcasts are done in parallel and the `Accept()` method of the returned
// `Route` immediately returns all the given `Connection`s.
// The establishement of a trivial `Route` does not involve a `RoutingService`.
//
func NewTerminalRoute(conns []Connection) Route {
	return newTerminalRoute(conns)
}

// Just an alias for `NewTerminalRoute` with a set of one `Connection`.
//
func NewConnectionRoute(conn Connection) Route {
	return NewTerminalRoute([]Connection{ conn })
}


// Create a trivial `Route` over a set of `Route`s.
// Broadcasts are done in parallel and the `Accept()` method of the returned
// `Route` simply transmits the results of the given `Route`s as soon as they
// become available.
// The establishement of a trivial `Route` does not involve a `RoutingService`.
//
func NewCompositeRoute(routes []Route) Route {
	return newCompositeRoute(routes)
}


// Create a trivial `Route` over a set of `Connection`s received from `connc`.
// Broadcast `Message`s are kept internally and sent to received `Connection`s.
// After all `Connection`s from `connc` have been `Accept()`ed, the returned
// `Route` waits for either an `error` to be sent on `errc` or both `chan`s to
// be closed.
// If an `error` is sent on `errc` then the last `Accept()` invocation returns
// `nil` and the given `error`.
//
func NewChannelRoute(connc <-chan Connection, errc <-chan error) Route {
	return newChannelRoute(connc, errc)
}


// ----------------------------------------------------------------------------


type terminalRoute struct {
	slock sync.Mutex
	conns []Connection
	lock sync.Mutex
	accept int
	closed bool
}

func newTerminalRoute(conns []Connection) *terminalRoute {
	var this terminalRoute

	this.conns = make([]Connection, len(conns))
	this.accept = 0
	this.closed = false

	copy(this.conns, conns)

	return &this
}

func (this *terminalRoute) Send(msg Message, proto Protocol) error {
	var eproto Protocol = NewRawProtocol(nil)
	var emsg encodedMessage
	var buf bytes.Buffer
	var errs []error
	var err error

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return io.EOF
	}

	this.lock.Unlock()

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	emsg.content = buf.Bytes()

	this.slock.Lock()
	errs = goRangeErr(len(this.conns), func (index int) error {
		return this.conns[index].Send(&emsg, eproto)
	})
	this.slock.Unlock()

	return firstError(errs)
}

func (this *terminalRoute) Accept() (Connection, error) {
	var conn Connection

	this.lock.Lock()

	if this.accept >= len(this.conns) {
		this.lock.Unlock()
		return nil, nil
	}

	if this.closed {
		this.accept = len(this.conns)
		this.lock.Unlock()
		return nil, io.EOF
	}

	conn = this.conns[this.accept]
	this.accept += 1

	this.lock.Unlock()

	return conn, nil
}

func (this *terminalRoute) Close() error {
	var i int

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil
	}

	this.closed = true
	i = this.accept

	this.lock.Unlock()

	for i < len(this.conns) {
		this.conns[i].Close()
		i += 1
	}

	return nil
}


type compositeRoute struct {
	slock sync.Mutex
	routes []Route
	connc chan Connection
	errc chan error
	chanr Route
	accepting sync.WaitGroup
	lock sync.Mutex
	closed bool
}

func newCompositeRoute(routes []Route) *compositeRoute {
	var this compositeRoute
	var route Route

	this.routes = make([]Route, len(routes))
	this.connc = make(chan Connection, len(routes))
	this.errc = make(chan error, len(routes))
	this.chanr = NewChannelRoute(this.connc, this.errc)
	this.closed = false

	copy(this.routes, routes)

	this.accepting.Add(len(this.routes))

	for _, route = range this.routes {
		go this.accept(route)
	}

	go func () {
		this.accepting.Wait()
		close(this.connc)
		close(this.errc)
	}()

	return &this
}

func (this *compositeRoute) Send(msg Message, proto Protocol) error {
	var eproto Protocol = NewRawProtocol(nil)
	var emsg encodedMessage
	var buf bytes.Buffer
	var errs []error
	var err error

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return io.EOF
	}

	this.lock.Unlock()

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	emsg.content = buf.Bytes()

	this.slock.Lock()

	errs = goRangeErr(len(this.routes), func (index int) error {
		return this.routes[index].Send(&emsg, eproto)
	})

	this.slock.Unlock()

	return firstError(errs)
}

func (this *compositeRoute) accept(route Route) {
	var conn Connection
	var err error

	for {
		conn, err = route.Accept()

		if err != nil {
			this.errc <- err
		}

		if conn == nil {
			break
		}

		this.connc <- conn
	}

	this.accepting.Done()
}

func (this *compositeRoute) Accept() (Connection, error) {
	return this.chanr.Accept()
}

func (this *compositeRoute) Close() error {
	var errs []error
	var err error

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil
	}

	this.closed = true
	this.lock.Unlock()

	err = this.chanr.Close()
	errs = goRangeErr(len(this.routes), func (index int) error {
		return this.routes[index].Close()
	})

	errs = append([]error{ err }, errs...)

	return firstError(errs)
}


type channelRoute struct {
	connc <-chan Connection
	errc <-chan error
	lock sync.Mutex
	cond *sync.Cond
	conns []Connection
	accepted int                 // # of already accepted in conns
	exhausted bool               // no more coming from connc
	recverr bool                 // has received from errc
	err error                    // received from errc if recverr
	closed bool                  // close has been called
	eof bool                     // accept was not done whan closed
}

func newChannelRoute(connc <-chan Connection, errc <-chan error) *channelRoute{
	var this channelRoute

	this.connc = connc
	this.errc = errc
	this.cond = sync.NewCond(&this.lock)
	this.conns = make([]Connection, 0)
	this.accepted = 0
	this.exhausted = false
	this.recverr = false
	this.err = nil
	this.closed = false

	go this.runConnc()
	go this.runErrc()

	return &this
}

func (this *channelRoute) Send(msg Message, proto Protocol) error {
	var enc encodedMessage
	var buf bytes.Buffer
	var errs []error
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	enc.content = buf.Bytes()

	this.lock.Lock()

retry:
	if this.closed {
		this.lock.Unlock()
		return io.EOF
	}

	if this.exhausted == false {
		this.cond.Wait()
		goto retry
	}

	this.lock.Unlock()

	errs = goRangeErr(len(this.conns), func (index int) error {
		return this.conns[index].Send(&enc, NewRawProtocol(nil))
	})

	return firstError(errs)
}

func (this *channelRoute) runConnc() {
	var conn Connection

	for conn = range this.connc {
		this.lock.Lock()

		if this.closed {
			this.lock.Unlock()
			conn.Close()
			continue
		}

		this.conns = append(this.conns, conn)
		this.cond.Broadcast()

		this.lock.Unlock()
	}

	this.lock.Lock()

	this.exhausted = true
	this.cond.Broadcast()

	this.lock.Unlock()
}

func (this *channelRoute) runErrc() {
	var err error
	var ok bool

	err, ok = <-this.errc

	this.lock.Lock()

	if ok && !this.closed {
		this.err = err
	}

	this.recverr = true

	this.cond.Broadcast()
	this.lock.Unlock()

	for err = range this.errc {
	}
}

func (this *channelRoute) Accept() (Connection, error) {
	var conn Connection
	var err error

	this.lock.Lock()
	defer this.lock.Unlock()

retry:
	if this.closed {
		if this.eof {
			this.eof = false
			return nil, io.EOF
		} else {
			return nil, nil
		}
	}

	if this.exhausted && (this.accepted >= len(this.conns)) {
		if this.recverr {
			err = this.err
			this.err = nil
			return nil, err
		}

		this.cond.Wait()
		goto retry
	}

	if !this.exhausted && (this.accepted >= len(this.conns)) {
		this.cond.Wait()
		goto retry
	}

	conn = this.conns[this.accepted]
	this.accepted += 1

	return conn, nil
}

func (this *channelRoute) Close() error {
	var i int

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil
	}

	this.closed = true

	if !this.exhausted || (this.accepted < len(this.conns)) {
		this.eof = true
	} else if !this.recverr {
		this.eof = true
	}

	this.cond.Broadcast()
	this.lock.Unlock()

	for i = this.accepted; i < len(this.conns); i++ {
		this.conns[i].Close()
	}

	return nil
}



// type channelRoute struct {
// 	connc <-chan Connection
// 	errc <-chan error
// 	lock sync.Mutex
// 	cond *sync.Cond
// 	slock sync.Mutex
// 	conns []Connection
// 	accepted int                 // # of already accepted in conns
// 	exhausted bool               // no more coming from connc
// 	bcasts []*encodedMessage     // waiting to be broadcast on connc
// 	recverr bool                 // has received from errc
// 	err error                    // received from errc if recverr
// 	closed bool                  // close has been called
// 	eof bool                     // accept was not done whan closed
// }

// func newChannelRoute(connc <-chan Connection, errc <-chan error) *channelRoute{
// 	var this channelRoute

// 	this.connc = connc
// 	this.errc = errc
// 	this.cond = sync.NewCond(&this.lock)
// 	this.conns = make([]Connection, 0)
// 	this.accepted = 0
// 	this.exhausted = false
// 	this.bcasts = make([]*encodedMessage, 0)
// 	this.recverr = false
// 	this.err = nil
// 	this.closed = false

// 	go this.runConnc()
// 	go this.runErrc()

// 	return &this
// }

// func (this *channelRoute) Send(msg Message, proto Protocol) error {
// 	var conns []Connection
// 	var enc encodedMessage
// 	var buf bytes.Buffer
// 	var errs []error
// 	var err error

// 	err = proto.Encode(sio.NewWriterSink(&buf), msg)
// 	if err != nil {
// 		return err
// 	}

// 	enc.content = buf.Bytes()

// 	this.lock.Lock()

// 	if this.closed {
// 		this.lock.Unlock()
// 		return io.EOF
// 	}

// 	conns = this.conns

// 	this.slock.Lock()

// 	if !this.exhausted {
// 		this.bcasts = append(this.bcasts, &enc)
// 	}

// 	this.lock.Unlock()

// 	errs = goRangeErr(len(conns), func (index int) error {
// 		return this.conns[index].Send(&enc, NewRawProtocol(nil))
// 	})

// 	this.slock.Unlock()

// 	return firstError(errs)
// }

// func (this *channelRoute) sendLate(conn Connection, bcasts []*encodedMessage) {
// 	var proto Protocol = NewRawProtocol(nil)
// 	var msg *encodedMessage
// 	var err error

// 	for _, msg = range bcasts {
// 		err = conn.Send(msg, proto)
// 		if err != nil {
// 			conn.Close()
// 			break
// 		}
// 	}
// }

// func (this *channelRoute) runConnc() {
// 	var conn Connection

// 	for conn = range this.connc {
// 		this.lock.Lock()

// 		if this.closed {
// 			this.lock.Unlock()
// 			conn.Close()
// 			continue
// 		}

// 		// FIXME: ordering issue
// 		//   - the below goroutine is waiting
// 		//   - user calls Send(m)
// 		//   - the below gorouting wakes up
// 		//   - the below conn now has m ordered first
// 		go this.sendLate(conn, this.bcasts)

// 		this.conns = append(this.conns, conn)
// 		this.cond.Broadcast()

// 		this.lock.Unlock()
// 	}

// 	this.lock.Lock()

// 	this.exhausted = true
// 	this.bcasts = nil
// 	this.cond.Broadcast()

// 	this.lock.Unlock()
// }

// func (this *channelRoute) runErrc() {
// 	var err error
// 	var ok bool

// 	err, ok = <-this.errc

// 	this.lock.Lock()

// 	if ok && !this.closed {
// 		this.err = err
// 	}

// 	this.recverr = true

// 	this.cond.Broadcast()
// 	this.lock.Unlock()

// 	for err = range this.errc {
// 	}
// }

// func (this *channelRoute) Accept() (Connection, error) {
// 	var conn Connection
// 	var err error

// 	this.lock.Lock()
// 	defer this.lock.Unlock()

// retry:
// 	if this.closed {
// 		if this.eof {
// 			this.eof = false
// 			return nil, io.EOF
// 		} else {
// 			return nil, nil
// 		}
// 	}

// 	if this.exhausted && (this.accepted >= len(this.conns)) {
// 		if this.recverr {
// 			err = this.err
// 			this.err = nil
// 			return nil, err
// 		}

// 		this.cond.Wait()
// 		goto retry
// 	}

// 	if !this.exhausted && (this.accepted >= len(this.conns)) {
// 		this.cond.Wait()
// 		goto retry
// 	}

// 	conn = this.conns[this.accepted]
// 	this.accepted += 1

// 	return conn, nil
// }

// func (this *channelRoute) Close() error {
// 	var i int

// 	this.lock.Lock()

// 	if this.closed {
// 		this.lock.Unlock()
// 		return nil
// 	}

// 	this.closed = true

// 	if !this.exhausted || (this.accepted < len(this.conns)) {
// 		this.eof = true
// 	} else if !this.recverr {
// 		this.eof = true
// 	}

// 	this.cond.Broadcast()
// 	this.lock.Unlock()

// 	for i = this.accepted; i < len(this.conns); i++ {
// 		this.conns[i].Close()
// 	}

// 	return nil
// }


func goRangeErr(n int, f func (int) error) []error {
	var errs []error = make([]error, n)
	var wg sync.WaitGroup
	var i int

	wg.Add(n)

	for i = 0; i < n; i++ {
		go func (index int) {
			errs[index] = f(index)
			wg.Done()
		}(i)
	}

	wg.Wait()

	return errs
}

func firstError(errs []error) error {
	var err error

	for _, err = range errs {
		if err != nil {
			return err
		}
	}

	return nil
}


type encodedMessage struct {
	content []byte
}

func (this *encodedMessage) Encode(sink sio.Sink) error {
	return sink.WriteBytes(this.content).Error()
}

func (this *encodedMessage) Decode(source sio.Source) error {
	panic("not implementable")
}
