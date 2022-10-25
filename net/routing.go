package net


import (
	"bytes"
	"fmt"
	"io"
	"math"
	sio "silk/io"
	"sync"
	"sync/atomic"
)


// ----------------------------------------------------------------------------


// A service to relay `Route` messages.
//
type RoutingService interface {
	// `io.Closer` Close all `Route`s currently relayed by this service.
	// Return once all handled `Connection`s have been closed.
	//
	// `Accept()` Accept an incoming routed `Connection` or an `error` if
	// something bad happened.
	//
	Server

	// Handle a `RoutingMessage` coming from the given `Connection`.
	// This service manages the given `Connection` and eventually `Close()`
	// it.
	// Return once the relayed `Route` is fully closed.
	//
	Handle(*RoutingMessage, Connection)
}


type RoutingServiceOptions struct {
	Log sio.Logger
}

// Create a new `RoutingService` routing messages accordingly to the given
// `Resolver`.
//
func NewRoutingService(resolver Resolver) RoutingService {
	return NewRoutingServiceWith(resolver, nil)
}

func NewRoutingServiceWith(r Resolver,o *RoutingServiceOptions) RoutingService{
	if o == nil {
		o = &RoutingServiceOptions{}
	}

	if o.Log == nil {
		o.Log = sio.NewStderrLogger(sio.LOG_TRACE)
	}

	return newRoutingService(r, o)
}


type RouteOptions struct {
	Log sio.Logger
}

// Create a new `Route` going through the given list of resolved `names`.
// Use the provided `resolver` to resolve the first `name`.
// Return immediately with either a `Route` or an `error` depending on if the
// given `resolver` managed to resolve the first of the `names`.
//
func NewRoute(names []string, resolver Resolver) (Route, error) {
	return NewRouteWith(names, resolver, nil)
}

func NewRouteWith(names []string, r Resolver, o *RouteOptions) (Route, error) {
	if o == nil {
		o = &RouteOptions{}
	}

	if o.Log == nil {
		o.Log = sio.NewNopLogger()
	}

	return newRoute(names, r, o)
}


const MaxRouteNameLength = math.MaxUint16

const MaxRouteLength = math.MaxUint8


type RoutingMessage struct {
	names []string
}


type RouteNameTooLongError struct {
	Name string
}

type RouteTooLongError struct {
	Names []string
}

type UnexpectedMessageError struct {
	Msg Message
}


// ----------------------------------------------------------------------------


type routingService struct {
	log sio.Logger
	resolver Resolver
	nextHandledId atomic.Uint64
}

func newRoutingService(r Resolver, o *RoutingServiceOptions) *routingService {
	var this routingService

	this.log = o.Log
	this.resolver = r
	this.nextHandledId.Store(0)

	return &this
}

func (this *routingService) Handle(msg *RoutingMessage, conn Connection) {
	var targetc chan Route
	var relay *finalRelay
	var protos []Protocol
	var routes []Route
	var log sio.Logger
	var id uint64
	var err error
	var i int

	id = this.nextHandledId.Add(1) - 1
	log = this.log.WithLocalContext("relay[%d]", id)

	if len(msg.names) == 0 {
		log.Warn("invalid routing request: %v", log.Emph(0, msg.names))
		conn.Close()
		return
	}

	log.Trace("connect %s", this.log.Emph(0, msg.names[0]))

	routes, protos, err = this.resolver.Resolve(msg.names[0])
	if err != nil {
		log.Warn("failed to resolve: %v: %s",
			log.Emph(0, msg.names[0]), err.Error())
		conn.Close()
		return
	}

	targetc = make(chan Route, len(routes))

	if len(msg.names) == 1 {
		relay = newFinalRelay(conn, targetc, log)

		for i = range routes {
			targetc <- routes[i]
		}

		close(targetc)
	} else {
		_ = protos
	}

	relay.run()
}

func (this *routingService) Accept() (Connection, error) {
	var c chan struct{}
	<-c
	return nil, nil
}

func (this *routingService) Close() error {
	return nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func newRoute(names []string, r Resolver, opts *RouteOptions) (Route, error) {
	var protos []Protocol
	var routes []Route
	var err error

	if len(names) == 0 {
		return NewTerminalRoute([]Connection{}), nil
	}

	opts.Log.Trace("connect: %s", opts.Log.Emph(0, names[0]))

	routes, protos, err = r.Resolve(names[0])
	if err != nil {
		return nil, err
	}

	if len(names) == 1 {
		if len(routes) == 1 {
			return routes[0], nil
		} else {
			return NewCompositeRoute(routes), nil
		}
	}

	return newRelayedRoute(names[1:], routes, protos, opts.Log), nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type relayedRoute struct {
	log sio.Logger
	lock sync.Mutex
	sendCond *sync.Cond
	sentRequest bool
	targets []Route
	acceptc chan *relayedConnection
}

func newRelayedRoute(names []string, targets []Route, protos []Protocol, log sio.Logger) *relayedRoute {
	var this relayedRoute

	this.log = log
	this.sendCond = sync.NewCond(&this.lock)
	this.sentRequest = false
	this.targets = targets
	this.acceptc = make(chan *relayedConnection)

	go this.run(names, protos)

	return &this
}

func (this *relayedRoute) run(names []string, protos []Protocol) {
	var relayed []Route = make([]Route, 0)
	var rmsg RoutingMessage
	var errs []error
	var i int

	this.log.Trace("request: %v", this.log.Emph(0, names))
	rmsg.names = names

	errs = goRangeErr(len(this.targets), func (index int) error {
		return this.targets[index].Send(&rmsg, protos[index])
	})

	for i = range errs {
		if errs[i] != nil {
			this.log.Warn("request %d/%d: %s", i, len(errs),
				errs[i].Error())
			this.targets[i].Close()
		} else {
			relayed = append(relayed, this.targets[i])
		}
	}

	this.lock.Lock()

	this.sentRequest = true
	this.targets = relayed
	this.sendCond.Broadcast()

	this.lock.Unlock()

	goRangeErr(len(this.targets), func (index int) error {
		return this.relayTarget(this.targets[index])
	})
}

func (this *relayedRoute) relayTarget(target Route) error {
	var tchan ServerChannel
	var stream Connection

	tchan = NewServerChannel(target)

	for stream = range tchan.Accept() {
		// go this.relayStream(stream)
		_ = stream
	}

	return tchan.Err()
}

// func (this *relayedRoute) relayStream(stream Connection) {
// 	var msg Message
// 	var err error

// 	for {
// 		msg, err = stream.Recv(routingProtocol)
// 		if err != nil {
// 			break
// 		}

// 		switch m := msg.(type) {
// 		case *routingAccept:
// 			this.log.Trace("accept %d", m.id)
// 			this.acceptc <- newRelayedConnection(stream, m.id,
// 				this.log.WithLocalContext("end[%d]", m.id))
// 		case *routingAccepted:
// 			this.log.Trace("accepted")
// 		case *routingUnicast:
// 			this.log.Trace("unicast")
// 		case *routingIdUnicast:
// 			this.log.Trace("unicast %d", m.id)
// 		case *routingClose:
// 			this.log.Trace("close %d", m.id)
// 		default:
// 			err = &UnexpectedMessageError{ msg }
// 		}
// 	}

// 	stream.Close()
// }

func (this *relayedRoute) Send(msg Message, proto Protocol) error {
	var rmsg routingBroadcast
	var buf bytes.Buffer
	var targets []Route
	var errs []error
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	rmsg.content = buf.Bytes()

	this.lock.Lock()

	for this.sentRequest == false {
		this.sendCond.Wait()
	}
	targets = this.targets

	this.lock.Unlock()

	this.log.Trace("broadcast %d bytes", len(rmsg.content))

	errs = goRangeErr(len(targets), func (index int) error {
		return targets[index].Send(&rmsg, routingProtocol)
	})

	return firstError(errs)
}

func (this *relayedRoute) Accept() (Connection, error) {
	var conn *relayedConnection
	var ok bool

	conn, ok = <-this.acceptc
	if !ok {
		return nil, nil
	}

	return conn, nil
}

func (this *relayedRoute) Close() error {
	return nil
}


type relayedConnection struct {
	log sio.Logger
	stream Connection
	id uint16
	lock sync.Mutex
	handleCond *sync.Cond
	recvCond *sync.Cond
	recv *routingIdUnicast
	closed bool
}

func newRelayedConnection(stream Connection, id uint16, log sio.Logger) *relayedConnection {
	var this relayedConnection

	this.log = log
	this.stream = stream
	this.id = id
	this.handleCond = sync.NewCond(&this.lock)
	this.recvCond = sync.NewCond(&this.lock)
	this.recv = nil

	return &this
}

func (this *relayedConnection) Send(msg Message, proto Protocol) error {
	var rmsg routingIdUnicast
	var buf bytes.Buffer
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

	rmsg.id = this.id
	rmsg.content = buf.Bytes()

	return this.stream.Send(&rmsg, routingProtocol)
}

func (this *relayedConnection) handleUnicast(msg *routingIdUnicast) {
	if msg.id != this.id {
		panic("invalid id")
	}

	this.lock.Lock()
	defer this.lock.Unlock()

retry:
	if this.closed {
		return
	}

	if this.recv != nil {
		this.handleCond.Wait()
		goto retry
	}

	this.recv = msg
	this.recvCond.Broadcast()
}

func (this *relayedConnection) Recv(proto Protocol) (Message, error) {
	var rmsg *routingIdUnicast

	this.lock.Lock()

retry:
	if this.closed {
		this.lock.Unlock()
		return nil, io.EOF
	}

	if this.recv == nil {
		this.recvCond.Wait()
		goto retry
	}

	rmsg = this.recv
	this.recv = nil
	this.handleCond.Broadcast()

	this.lock.Unlock()

	return proto.Decode(sio.NewReaderSource(bytes.NewBuffer(rmsg.content)))
}

func (this *relayedConnection) Close() error {
	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil
	}

	this.closed = true
	this.recv = nil
	this.handleCond.Broadcast()
	this.recvCond.Broadcast()

	this.lock.Unlock()

	return this.stream.Send(&routingClose{ this.id }, routingProtocol)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


// type relayedRoute struct {
// 	log sio.Logger
// }

// func newRelayedRoute(log sio.Logger) *relayedRoute {
// 	var this relayedRoute

// 	this.log = log

// 	return &this
// }

// func (this *relayedRoute) relay(ns []string, rs []Route, ps []Protocol) {
// 	var relaying sync.WaitGroup
// 	var log sio.Logger
// 	var i int

// 	relaying.Add(len(rs))

// 	for i = range rs {
// 		go func (index int) {
// 			log = this.log.WithLocalContext("relayed[%d]", index)
// 			this.relayTarget(ns, rs[index], ps[index], log)
// 			relaying.Done()
// 		}(i)
// 	}

// 	relaying.Wait()
// }

// func (this *relayedRoute) relayTarget(ns []string, r Route, p Protocol, log sio.Logger) {
// 	var relaying sync.WaitGroup
// 	var rchan ServerChannel
// 	var stream Connection
// 	var err error
// 	var id int

// 	log.Trace("send target: %v", log.Emph(0, ns))
// 	err = r.Send(&RoutingMessage{ ns }, p)
// 	if err != nil {
// 		log.Warn("%s", err.Error())
// 		r.Close()
// 		return
// 	}

// 	rchan = NewServerChannel(r)
// 	id = 0

// 	for stream = range rchan.Accept() {
// 		relaying.Add(1)

// 		go func (stream Connection, slog sio.Logger) {
// 			this.relayStream(stream, slog)
// 			relaying.Done()
// 		}(stream, log.WithLocalContext("stream[%d]", id))

// 		id += 1
// 	}

// 	err = rchan.Err()
// 	if err != nil {
// 		log.Warn("accepting: %s", err.Error())
// 	}

// 	relaying.Wait()
// }

// func (this *relayedRoute) relayStream(stream Connection, log sio.Logger) {
// 	var msg Message
// 	var err error

// 	for {
// 		msg, err = stream.Recv(routingProtocol)

// 		log.Debug("%T:%v : %v", msg, msg, err)
// 	}
// }

// func (this *relayedRoute) Send(msg Message, proto Protocol) error {
// 	return nil
// }

// func (this *relayedRoute) Accept() (Connection, error) {
// 	return nil, nil
// }

// func (this *relayedRoute) Close() error {
// 	return nil
// }


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


// Relay between an origin `Connection`s and a set of progressively added
// target `Route`s.
// This relays a `routingAccept` as soon as a new `Connection` is accepted from
// a target `Route`.
//
type finalRelay struct {
	log sio.Logger
	origin Connection
	targetc <-chan Route
}

func newFinalRelay(origin Connection, targetc <-chan Route, log sio.Logger) *finalRelay {
	var this finalRelay

	this.log = log
	this.origin = origin
	this.targetc = targetc

	return &this
}

func (this *finalRelay) run() {
	var target Route

	for target = range this.targetc {
		go this.runTarget(target)
	}
}

func (this *finalRelay) runTarget(target Route) {
	var tchan ServerChannel = NewServerChannel(target)
	var conn Connection
	var log sio.Logger
	var id uint16

	id = 0

	for conn = range tchan.Accept() {
		log = this.log.WithLocalContext("stream[%d]", id)
		go newStreamRelay(this.origin, conn, id, log).run()
		id += 1
	}
}


type streamRelay struct {
	log sio.Logger
	origin Connection
	stream Connection
	id uint16
}

func newStreamRelay(origin, stream Connection, id uint16, log sio.Logger) *streamRelay {
	var this streamRelay

	this.log = log
	this.origin = origin
	this.stream = stream
	this.id = id

	return &this
}

func (this *streamRelay) run() {
	var err error

	this.log.Trace("send accept back")

	err = this.origin.Send(&routingAccept{ this.id }, routingProtocol)
	if err != nil {
		this.log.Warn("relay back: %s", err.Error())
		this.origin.Close()
		this.stream.Close()
		return
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


// Encapsulate sent `Message`s in `routingUnicast` before to send it.
// Also decapsulate from `routingUnicast` or `routingBroadcast` at reception.
//
type endpointConnection struct {
	inner Connection
}

func newEndpointConnection(inner Connection) *endpointConnection {
	var this endpointConnection

	this.inner = inner

	return &this
}

func (this *endpointConnection) Send(msg Message, proto Protocol) error {
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	return this.inner.Send(&routingUnicast{buf.Bytes()}, routingProtocol)
}

func (this *endpointConnection) Recv(proto Protocol) (Message, error) {
	var buf *bytes.Buffer
	var msg Message
	var err error

	msg, err = this.inner.Recv(routingProtocol)
	if err != nil {
		return nil, err
	}

	switch m := msg.(type) {
	case *routingUnicast:
		buf = bytes.NewBuffer(m.content)
	case *routingBroadcast:
		buf = bytes.NewBuffer(m.content)
	case *routingClose:
		this.Close()
	default:
		this.Close()
		return nil, &UnexpectedMessageError{ msg }
	}

	return proto.Decode(sio.NewReaderSource(buf))
}

func (this *endpointConnection) Close() error {
	return this.inner.Close()
}


// Encapsulate sent `Message`s in `routingBroadcast` before to send it.
// Also return `*endpointConnection`s on `Accept()`.
//
type endpointRoute struct {
	inner Route
}

func newEndpointRoute(inner Route) *endpointRoute {
	var this endpointRoute

	this.inner = inner

	return &this
}

func (this *endpointRoute) Send(msg Message, proto Protocol) error {
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	return this.inner.Send(&routingBroadcast{buf.Bytes()}, routingProtocol)
}

func (this *endpointRoute) Accept() (Connection, error) {
	var conn Connection
	var err error

	conn, err = this.inner.Accept()
	if err != nil {
		return nil, err
	} else if conn == nil {
		return nil, nil
	}

	return newEndpointConnection(conn), nil
}

func (this *endpointRoute) Close() error {
	return this.inner.Close()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *RoutingMessage) Encode(sink sio.Sink) error {
	var i int

	if len(this.names) > MaxRouteLength {
		return &RouteTooLongError{ this.names }
	}

	for i = range this.names {
		if len(this.names[i]) > MaxRouteNameLength {
			return &RouteNameTooLongError{ this.names[i] }
		}
	}

	sink = sink.WriteUint8(uint8(len(this.names)))
	
	for i = range this.names {
		sink = sink.WriteString16(this.names[i])
	}

	return sink.Error()
}

func (this *RoutingMessage) Decode(source sio.Source) error {
	var n uint8
	var i int

	return source.ReadUint8(&n).AndThen(func () error {
		this.names = make([]string, n)

		for i = range this.names {
			source = source.ReadString16(&this.names[i])
		}

		return source.Error()
	}).Error()
}


var routingProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &routingAccept{},
	1: &routingAccepted{},
	2: &routingUnicast{},
	3: &routingIdUnicast{},
	4: &routingBroadcast{},
	5: &routingClose{},
})


type routingAccept struct {
	id uint16
}

func (this *routingAccept) Encode(sink sio.Sink) error {
	return sink.WriteUint16(this.id).Error()
}

func (this *routingAccept) Decode(source sio.Source) error {
	return source.ReadUint16(&this.id).Error()
}


type routingAccepted struct {
}

func (this *routingAccepted) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *routingAccepted) Decode(source sio.Source) error {
	return source.Error()
}


type routingUnicast struct {
	content []byte
}

func (this *routingUnicast) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *routingUnicast) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type routingIdUnicast struct {
	id uint16
	content []byte
}

func (this *routingIdUnicast) Encode(sink sio.Sink) error {
	return sink.WriteUint16(this.id).WriteBytes32(this.content).Error()
}

func (this *routingIdUnicast) Decode(source sio.Source) error {
	return source.ReadUint16(&this.id).ReadBytes32(&this.content).Error()
}


type routingBroadcast struct {
	content []byte
}

func (this *routingBroadcast) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *routingBroadcast) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type routingClose struct {
	id uint16
}

func (this *routingClose) Encode(sink sio.Sink) error {
	return sink.WriteUint16(this.id).Error()
}

func (this *routingClose) Decode(source sio.Source) error {
	return source.ReadUint16(&this.id).Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *RouteTooLongError) Error() string {
	return fmt.Sprintf("route too long: %v", this.Names)
}

func (this *RouteNameTooLongError) Error() string {
	return fmt.Sprintf("route name too long: %s", this.Name)
}

func (this *UnexpectedMessageError) Error() string {
	return fmt.Sprintf("unexpected message: %T:%v", this.Msg, this.Msg)
}
















































// type RoutingService interface {
// 	// Accept `Connection`s coming from remotely initiated `Route`s.
// 	//
// 	Server

// 	// Handle the given `RoutingMessage` coming from the given
// 	// `Connection`.
// 	// The caller must not use the given `Connection` anymore.
// 	// The `RoutingService` takes care of closing `Connection` when
// 	// necessary.
// 	//
// 	Handle(*RoutingMessage, Connection)
// }


// func NewRoutingService(resolver Resolver) RoutingService {
// 	return NewRoutingServiceWithLogger(resolver, sio.NewNopLogger())
// }

// func NewRoutingServiceWithLogger(resolver Resolver, log sio.Logger) RoutingService {
// 	return newRoutingService(resolver, log)
// }


// func NewRoute(names []string, resolver Resolver) (Route, error) {
// 	return NewRouteWithLogger(names, resolver, sio.NewNopLogger())
// }

// func NewRouteWithLogger(names []string, resolver Resolver, log sio.Logger) (Route, error) {
// 	return newRoute(names, resolver, log)
// }


// type RoutingMessage struct {
// 	payload Message
// }


// const RouteMaxLength int = math.MaxUint8

// const RouteAddressMaxLength int = math.MaxUint8

// func EncodeRouteNames(sink sio.Sink, names []string) sio.Sink {
// 	return encodeRouteNames(sink, names)
// }

// func DecodeRouteNames(source sio.Source, names *[]string) sio.Source {
// 	return decodeRouteNames(source, names)
// }


// type RouteTooLongError struct {
// 	Path []string
// }

// type AddressTooLongError struct {
// 	Path []string
// 	Index int
// }


// // ----------------------------------------------------------------------------


// type routingService struct {
// 	log sio.Logger
// 	resolver Resolver
// 	lock sync.Mutex
// 	acceptc chan Connection
// 	closed bool
// }

// func newRoutingService(resolver Resolver, log sio.Logger) *routingService {
// 	var this routingService

// 	this.log = log
// 	this.resolver = resolver
// 	this.acceptc = make(chan Connection)
// 	this.closed = false

// 	return &this
// }

// func (this *routingService) Accept() (Connection, error) {
// 	var conn Connection
// 	var ok bool

// 	conn, ok = <-this.acceptc
// 	if !ok {
// 		return nil, io.EOF
// 	}

// 	return conn, nil
// }

// func (this *routingService) handleTerminal(conn Connection) error {
// 	var reply routingReply
// 	var err error

// 	reply.id = 0

// 	err = conn.Send(&reply, routingProtocol)
// 	if err != nil {
// 		this.log.Warn("cannot send routing reply")
// 	}

// 	err = conn.Send(&routingCompletion{}, routingProtocol)
// 	if err != nil {
// 		this.log.Warn("cannot send routing completion")
// 	}

// 	this.lock.Lock()
// 	if this.closed {
// 		this.log.Debug("cancel routing connection")
// 		conn.Close()
// 	} else {
// 		this.log.Debug("accept routing connection")
// 		this.acceptc <- newRoutingConnection(conn)
// 	}
// 	this.lock.Unlock()

// 	return nil
// }

// func (this *routingService) handleRelay(names []string, conn Connection) error{
// 	var protos []Protocol
// 	var relay *relayRoute
// 	var routes []Route
// 	var log sio.Logger
// 	var err error

// 	routes, protos, err = this.resolver.Resolve(names[0])
// 	if err != nil {
// 		this.log.Warn("cannot resolve '%s': %s",
// 			this.log.Emph(0, names[0]), err.Error())
// 		return err
// 	}

// 	log = this.log.WithLocalContext(fmt.Sprintf("%v", names))

// 	relay, err = newRelayRoute(routes, protos, names[1:], log)
// 	if err != nil {
// 		return err
// 	}

// 	go newRoutingRelay(conn, relay, log).run()

// 	return nil
// }

// func (this *routingService) handleConnect(msg *routingRequest,conn Connection){
// 	var err error

// 	if len(msg.names) == 0 {
// 		this.log.Trace("receive terminal routing request")
// 		err = this.handleTerminal(conn)
// 	} else {
// 		this.log.Trace("receive routing request for %v",
// 			this.log.Emph(0, msg.names))
// 		err = this.handleRelay(msg.names, conn)
// 	}

// 	if err != nil {
// 		// send error msg
// 		conn.Close()
// 	}
// }

// func (this *routingService) Handle(msg *RoutingMessage, conn Connection) {
// 	switch m := msg.payload.(type) {
// 	case *routingRequest:
// 		this.handleConnect(m, conn)
// 	default:
// 		this.log.Warn("unexpected routing payload type %T",
// 			this.log.Emph(2, msg.payload))
// 		conn.Close()
// 	}
// }

// func (this *routingService) Close() error {
// 	this.lock.Lock()

// 	if !this.closed {
// 		this.closed = true
// 		close(this.acceptc)
// 	}

// 	this.lock.Unlock()

// 	return nil
// }


// type routingConnection struct {
// 	conn Connection
// }

// func newRoutingConnection(conn Connection) *routingConnection {
// 	var this routingConnection

// 	this.conn = conn

// 	return &this
// }

// func (this *routingConnection) Send(msg Message, proto Protocol) error {
// 	var routed routingContent
// 	var buf bytes.Buffer
// 	var err error

// 	err = proto.Encode(sio.NewWriterSink(&buf), msg)
// 	if err != nil {
// 		return err
// 	}

// 	routed.id = 0
// 	routed.content = buf.Bytes()

// 	return this.conn.Send(&routed, routingProtocol)
// }

// func (this *routingConnection) Recv(proto Protocol) (Message, error) {
// 	var routed *routingContent
// 	var buf *bytes.Buffer
// 	var msg Message
// 	var err error
// 	var ok bool

// 	msg, err = this.conn.Recv(routingProtocol)
// 	if err != nil {
// 		return nil, err
// 	}

// 	routed, ok = msg.(*routingContent)
// 	if !ok {
// 		this.Close()
// 		return nil, fmt.Errorf("do proper error")
// 	}

// 	buf = bytes.NewBuffer(routed.content)

// 	return proto.Decode(sio.NewReaderSource(buf))
// }

// func (this *routingConnection) Close() error {
// 	return this.conn.Close()
// }


// func newRoute(names []string, resolver Resolver, log sio.Logger) (Route, error) {
// 	var protos []Protocol
// 	var routes []Route
// 	var err error

// 	routes, protos, err = resolver.Resolve(names[0])
// 	if err != nil {
// 		return nil, err
// 	}

// 	if len(names) == 1 {
// 		if len(routes) == 1 {
// 			return routes[0], nil
// 		} else {
// 			return newCompositeRoute(routes), nil
// 		}
// 	}

// 	return newRelayRoute(routes, protos, names[1:], log)
// }


// type relayRoute struct {
// 	log sio.Logger
// 	routes []Route
// 	lock sync.Mutex
// 	cond *sync.Cond
// 	conns []*relayConnection
// 	aexhausted bool              // all routes have accepted (conn is full)
// 	cremaining int               // number of non exhausted connections
// 	aindex int                   // number of returned `Accept()`
// }

// func newRelayRoute(routes []Route, protos []Protocol, names []string, log sio.Logger) (*relayRoute, error) {
// 	var this relayRoute
// 	var err error

// 	this.log = log
// 	this.routes = routes
// 	this.cond = sync.NewCond(&this.lock)
// 	this.conns = make([]*relayConnection, 0)
// 	this.aexhausted = false
// 	this.cremaining = 0
// 	this.aindex = 0

// 	err = this.sendRequests(protos, names)
// 	if err != nil {
// 		this.Close()
// 		return nil, err
// 	}

// 	go this.run()

// 	return &this, nil
// }

// func (this *relayRoute) sendRequests(protos []Protocol, names []string) error {
// 	var errc chan error = make(chan error)
// 	var msg RoutingMessage
// 	var err, e error
// 	var route Route
// 	var index int

// 	msg.payload = &routingRequest{ names }

// 	if len(names) == 0 {
// 		this.log.Trace("broadcast terminal routing request")
// 	} else {
// 		this.log.Trace("broadcast routing request for %s",
// 			this.log.Emph(0, names))
// 	}

// 	for index, route = range this.routes {
// 		go func (route Route, proto Protocol) {
// 			errc <- route.Send(&msg, proto)
// 		}(route, protos[index])
// 	}

// 	err = nil

// 	for index = range this.routes {
// 		e = <-errc
// 		if (e != nil) && (err == nil) {
// 			err = e
// 		}
// 	}

// 	close(errc)

// 	return err
// }

// func (this *relayRoute) broadcast(msg *routingContent) error {
// 	var errs []error

// 	errs = goRangeErr(len(this.routes), func (index int) error{
// 		return this.routes[index].Send(msg, routingProtocol)
// 	})

// 	return firstError(errs)

// }

// func (this *relayRoute) Send(msg Message, proto Protocol) error {
// 	var buf bytes.Buffer
// 	var err error

// 	err = proto.Encode(sio.NewWriterSink(&buf), msg)
// 	if err != nil {
// 		return err
// 	}

// 	this.log.Trace("broadcast %d bytes of routed content", buf.Len())

// 	return this.broadcast(&routingContent{ math.MaxUint16, buf.Bytes() })
// }

// func (this *relayRoute) relayForward(msg *routingContent) error {
// 	var conn *relayConnection

// 	if msg.id == math.MaxUint16 {
// 		this.log.Trace("routing %d bytes of broadcast content",
// 			len(msg.content))
// 		return this.broadcast(msg)
// 	}

// 	this.log.Trace("routing %d bytes of unicast content to %d",
// 		len(msg.content), this.log.Emph(0, msg.id))

// 	this.lock.Lock()

// 	if int(msg.id) < len(this.conns) {
// 		conn = this.conns[msg.id]
// 	} else {
// 		conn = nil
// 	}

// 	this.lock.Unlock()

// 	if conn == nil {
// 		this.log.Warn("unknown back connection with id %d",
// 			this.log.Emph(0, msg.id))
// 		return fmt.Errorf("do proper errors")
// 	}

// 	return conn.relayForward(msg.content)
// }

// func (this *relayRoute) relayBackward(rindex, bcindex int, conn Connection) {
// 	var backmap map[uint16]*relayConnection
// 	var rconn *relayConnection
// 	var exhausted bool = false
// 	var msg Message
// 	var index int
// 	var err error

// 	backmap = make(map[uint16]*relayConnection)

// 	loop: for {
// 		msg, err = conn.Recv(routingProtocol)
// 		if err != nil {
// 			break
// 		}

// 		switch m := msg.(type) {

// 		case *routingReply:
// 			this.lock.Lock()

// 			index = len(this.conns)

// 			this.log.Trace("recv route reply of %d on %d.%d as %d",
// 				this.log.Emph(0, m.id),
// 				this.log.Emph(1, rindex),
// 				this.log.Emph(1, bcindex),
// 				this.log.Emph(0, index))

// 			rconn = newRelayConnection(conn, m.id, this.log.
// 				WithLocalContext(fmt.Sprintf("[%d=%d.%d.%d]",
// 				index, rindex, bcindex, m.id)))

// 			this.conns = append(this.conns, rconn)
// 			backmap[m.id] = rconn

// 			this.cond.Broadcast()
// 			this.lock.Unlock()

// 		case *routingCompletion:
// 			this.log.Trace("receive routing completion from %d.%d",
// 				this.log.Emph(1, rindex),
// 				this.log.Emph(1, bcindex))

// 			if exhausted {
// 				this.log.Warn("redundant routing completion")
// 				continue loop
// 			}

// 			exhausted = true

// 			this.lock.Lock()
// 			this.cremaining -= 1
// 			this.cond.Broadcast()
// 			this.lock.Unlock()

// 		case *routingContent:
// 			this.log.Trace("receive routed content of %d on %d.%d",
// 				this.log.Emph(0, m.id),
// 				this.log.Emph(1, rindex),
// 				this.log.Emph(1, bcindex))

// 			rconn = backmap[m.id]
// 			if rconn == nil {
// 				this.log.Warn("no peer %d for %d.%d",
// 					this.log.Emph(0, m.id),
// 					this.log.Emph(1, rindex),
// 					this.log.Emph(1, bcindex))
// 				// send error msg
// 				continue loop
// 			}

// 			rconn.relayBackward(m.content)

// 		default:
// 			this.log.Warn("unexpected message type %T from %d.%d",
// 				this.log.Emph(2, msg),
// 				this.log.Emph(1, rindex),
// 				this.log.Emph(1, bcindex))

// 			break loop

// 		}
// 	}

// 	this.log.Trace("closing route on %d.%d", this.log.Emph(1, rindex),
// 		this.log.Emph(1, bcindex))

// 	conn.Close()

// 	for _, rconn = range backmap {
// 		rconn.Close()
// 	}
// }

// func (this *relayRoute) accept(rindex int, route Route) {
// 	var conn Connection
// 	var bcindex int
// 	var err error

// 	bcindex = 0

// 	for {
// 		conn, err = route.Accept()
// 		if err != nil {
// 			this.log.Warn("failed to accept from route %d",
// 				this.log.Emph(1, rindex))
// 			break
// 		}

// 		if conn == nil {
// 			break
// 		}

// 		this.lock.Lock()
// 		this.cremaining += 1
// 		this.lock.Unlock()

// 		go this.relayBackward(rindex, bcindex, conn)

// 		bcindex += 1
// 	}

// 	this.lock.Lock()
// 	this.aexhausted = true
// 	this.cond.Broadcast()
// 	this.lock.Unlock()
// }

// func (this *relayRoute) run() {
// 	var route Route
// 	var rindex int

// 	for rindex, route = range this.routes {
// 		go this.accept(rindex, route)
// 	}
// }

// func (this *relayRoute) acceptRelay() (*relayConnection, error) {
// 	var conn *relayConnection
// 	var err error

// 	this.lock.Lock()

// 	for {
// 		if this.aindex < len(this.conns) {
// 			conn = this.conns[this.aindex]
// 			this.aindex += 1
// 			break
// 		} else if this.aexhausted {
// 			if this.cremaining == 0 {
// 				err = nil
// 			} else {
// 				err = io.EOF
// 			}
// 			conn = nil
// 			break
// 		} else {
// 			this.cond.Wait()
// 		}
// 	}

// 	this.lock.Unlock()

// 	return conn, err
// }

// func (this *relayRoute) Accept() (Connection, error) {
// 	var rconn *relayConnection
// 	var conn Connection
// 	var err error

// 	rconn, err = this.acceptRelay()

// 	if rconn == (*relayConnection)(nil) {
// 		conn = nil
// 	} else {
// 		conn = rconn
// 	}

// 	return conn, err
// }

// func (this *relayRoute) Close() error {
// 	var errs []error

// 	this.log.Trace("closing")

// 	errs = goRangeErr(len(this.routes), func (index int) error{
// 		return this.routes[index].Close()
// 	})

// 	return firstError(errs)
// }


// type relayConnection struct {
// 	log sio.Logger
// 	conn Connection
// 	id uint16
// 	recvc chan []byte
// 	lock sync.Mutex
// 	closed bool
// }

// func newRelayConnection(conn Connection, id uint16, log sio.Logger) *relayConnection {
// 	var this relayConnection

// 	this.log = log
// 	this.conn = conn
// 	this.id = id
// 	this.recvc = make(chan []byte, 128)
// 	this.closed = false

// 	return &this
// }

// func (this *relayConnection) relayBackward(content []byte) {
// 	this.recvc <- content
// }

// func (this *relayConnection) relayForward(content []byte) error {
// 	return this.conn.Send(&routingContent{ this.id, content },
// 		routingProtocol)
// }

// func (this *relayConnection) Send(msg Message, proto Protocol) error {
// 	var routed routingContent
// 	var buf bytes.Buffer
// 	var err error

// 	err = proto.Encode(sio.NewWriterSink(&buf), msg)
// 	if err != nil {
// 		return err
// 	}

// 	routed.id = this.id
// 	routed.content = buf.Bytes()
	
// 	return this.conn.Send(&routed, routingProtocol)
// }

// func (this *relayConnection) recvRelayed() ([]byte, error) {
// 	var b []byte
// 	var ok bool

// 	b, ok = <-this.recvc
// 	if !ok {
// 		return nil, io.EOF
// 	}

// 	return b, nil
// }

// func (this *relayConnection) Recv(proto Protocol) (Message, error) {
// 	var buf *bytes.Buffer
// 	var err error
// 	var b []byte

// 	b, err = this.recvRelayed()
// 	if err != nil {
// 		return nil, err
// 	}

// 	buf = bytes.NewBuffer(b)

// 	return proto.Decode(sio.NewReaderSource(buf))
// }

// func (this *relayConnection) Close() error {
// 	this.lock.Lock()

// 	if this.closed {
// 		this.lock.Unlock()
// 		return nil
// 	}

// 	this.closed = true

// 	this.lock.Unlock()

// 	close(this.recvc)

// 	return this.conn.Close()
// }


// type routingRelay struct {
// 	log sio.Logger
// 	origin Connection
// 	target *relayRoute
// 	lock sync.Mutex
// 	closed bool
// 	accepted bool
// 	conns []Connection
// 	closedConns int
// }

// func newRoutingRelay(origin Connection, target *relayRoute, log sio.Logger) *routingRelay {
// 	var this routingRelay

// 	this.log = log
// 	this.origin = origin
// 	this.target = target
// 	this.closed = false
// 	this.accepted = false
// 	this.conns = make([]Connection, 0)
// 	this.closedConns = 0

// 	return &this
// }

// func (this *routingRelay) relayBackward(bcindex int, conn *relayConnection) {
// 	var accepted, allClosed bool
// 	var content []byte
// 	var err error

// 	for {
// 		content, err = conn.recvRelayed()
// 		if err != nil {
// 			break
// 		}

// 		this.log.Trace("routing back %d bytes as %d", len(content),
// 			this.log.Emph(0, bcindex))

// 		err = this.origin.Send(&routingContent{ uint16(bcindex),
// 			content }, routingProtocol)

// 		if err != nil {
// 			this.log.Warn("failed to route back")
// 			this.closeRelay()
// 			break
// 		}
// 	}

// 	this.log.Trace("closing relay as %d", this.log.Emph(0, bcindex))

// 	conn.Close()

// 	this.lock.Lock()
// 	this.closedConns += 1
// 	accepted = this.accepted
// 	allClosed = this.closedConns == len(this.conns)
// 	this.lock.Unlock()

// 	if accepted && allClosed {
// 		this.closeRelay()
// 	}
// }

// func (this *routingRelay) relayForward() {
// 	var msg Message
// 	var err error

// 	for {
// 		msg, err = this.origin.Recv(routingProtocol)
// 		if err != nil {
// 			break
// 		}

// 		switch m := msg.(type) {

// 		case *routingContent:
// 			err = this.target.relayForward(m)
// 			if err != nil {
// 				this.log.Warn("cannot relay routed content")
// 				// send err msg
// 			}

// 		default:
// 			this.log.Warn("unexpected message type %T from origin",
// 				this.log.Emph(0, msg))
// 		}
// 	}

// 	this.closeRelay()
// }

// func (this *routingRelay) accept() {
// 	var conn *relayConnection
// 	var reply routingReply
// 	var allClosed bool
// 	var bcindex int
// 	var msg Message
// 	var err error

// 	msg = &reply

// 	for {
// 		conn, err = this.target.acceptRelay()
// 		if err != nil {
// 			this.log.Warn("failed to accept")
// 			break
// 		}

// 		if conn != nil {
// 			this.lock.Lock()
// 			bcindex = len(this.conns)
// 			this.conns = append(this.conns, conn)
// 			this.lock.Unlock()

// 			reply.id = uint16(bcindex)

// 			this.log.Trace("send back routing reply as %d",
// 				this.log.Emph(0, bcindex))
// 		} else {
// 			msg = &routingCompletion{}

// 			this.log.Trace("send routing completion")
// 		}

// 		err = this.origin.Send(msg, routingProtocol)

// 		if err != nil {
// 			this.log.Warn("cannot send routing reply")
// 			return
// 		}

// 		if conn == nil {
// 			break
// 		}

// 		go this.relayBackward(bcindex, conn)
// 	}

// 	this.lock.Lock()
// 	this.accepted = true
// 	allClosed = this.closedConns == len(this.conns)
// 	this.lock.Unlock()

// 	if allClosed {
// 		this.closeRelay()
// 	}
// }

// func (this *routingRelay) run() {
// 	go this.accept()
// 	this.relayForward()
// }

// func (this *routingRelay) closeRelay() {
// 	this.lock.Lock()

// 	if this.closed {
// 		this.lock.Unlock()
// 		return
// 	}

// 	this.closed = true

// 	this.lock.Unlock()

// 	this.log.Trace("closing relay")

// 	this.origin.Close()
// 	this.target.Close()
// }



// var routingProtocol Protocol = NewUint8Protocol(map[uint8]Message{
// 	0: &routingRequest{},
// 	1: &routingReply{},
// 	2: &routingCompletion{},
// 	3: &routingContent{},
// })

// func (this *RoutingMessage) Encode(sink sio.Sink) error {
// 	return routingProtocol.Encode(sink, this.payload)
// }

// func (this *RoutingMessage) Decode(source sio.Source) error {
// 	var err error
// 	this.payload, err = routingProtocol.Decode(source)
// 	return err
// }


// func encodeRouteNames(sink sio.Sink, names []string) sio.Sink {
// 	var name string
// 	var index int

// 	if len(names) > RouteMaxLength {
// 		return sio.NewErrorSink(&RouteTooLongError{ names })
// 	}

// 	for index = range names {
// 		if len(names[index]) > RouteAddressMaxLength {
// 			return sio.NewErrorSink(&AddressTooLongError{
// 				Path: names,
// 				Index: index,
// 			})
// 		}
// 	}

// 	sink = sink.WriteUint8(uint8(len(names)))
// 	for _, name = range names {
// 		sink = sink.WriteString8(name)
// 	}

// 	return sink
// }

// func decodeRouteNames(source sio.Source, names *[]string) sio.Source {
// 	var nlen uint8
// 	var i int

// 	return source.ReadUint8(&nlen).AndThen(func () error {
// 		var tmp []string = make([]string, int(nlen))
// 		for i = range tmp {
// 			source = source.ReadString8(&tmp[i])
// 		}
// 		*names = tmp
// 		return source.Error()
// 	})
// }


// type routingRequest struct {
// 	names []string
// }

// func (this *routingRequest) Encode(sink sio.Sink) error {
// 	return EncodeRouteNames(sink, this.names).Error()
// }

// func (this *routingRequest) Decode(source sio.Source) error {
// 	return DecodeRouteNames(source, &this.names).Error()
// }


// type routingReply struct {
// 	id uint16
// }

// func (this *routingReply) Encode(sink sio.Sink) error {
// 	return sink.WriteUint16(this.id).Error()
// }

// func (this *routingReply) Decode(source sio.Source) error {
// 	return source.ReadUint16(&this.id).Error()
// }


// type routingCompletion struct {
// }

// func (this *routingCompletion) Encode(sink sio.Sink) error {
// 	return sink.Error()
// }

// func (this *routingCompletion) Decode(source sio.Source) error {
// 	return source.Error()
// }


// type routingContent struct {
// 	id uint16
// 	content []byte
// }

// func (this *routingContent) Encode(sink sio.Sink) error {
// 	return sink.WriteUint16(this.id).WriteBytes32(this.content).Error()
// }

// func (this *routingContent) Decode(source sio.Source) error {
// 	return source.ReadUint16(&this.id).ReadBytes32(&this.content).Error()
// }


// func (this *RouteTooLongError) Error() string {
// 	return fmt.Sprintf("route too long (%d hops / %d max)", len(this.Path),
// 		RouteMaxLength)
// }

// func (this *AddressTooLongError) Error() string {
// 	return fmt.Sprintf("address '%s' too long (%d bytes / %d max)",
// 		this.Path[this.Index], len(this.Path[this.Index]),
// 		RouteAddressMaxLength)
// }
