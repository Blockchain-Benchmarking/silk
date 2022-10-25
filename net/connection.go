package net


import (
	"bytes"
	"context"
	"io"
	"net"
	sio "silk/io"
	"sync"


	"fmt"
)


// ----------------------------------------------------------------------------


type Connection interface {
	// Send and receive messages.
	// If the underlying connection ends unexpectedly, then return an
	// error.
	//
	Channel

	// Close the connection.
	// Any call to `Close()` after the first call does nothing and returns
	// `nil`.
	//
	// The `Close()` function can safely be called concurrently to other
	// calls to `Close()`, `Send()` or `Recv()`.
	//
	io.Closer
}


func NewChanConnections(bufsize int) (Connection, Connection) {
	return newChanConnection(bufsize)
}

func NewIoConnection(reader io.ReadCloser, writer io.WriteCloser) Connection {
	return newIoConnection(reader, writer)
}


func NewLocalConnection(bufsize int) (Connection, Connection) {
	return newChanConnection(bufsize)
}


// Return a new `Connection` wrapping `inner`.
// The only effect is that when the returned `Connection` is closed then any
// subsequent call to `Close()` returns `nil` and calls to `Send()` or `Recv()`
// return `io.EOF` but `inner` is not closed.
//
func NewConnectionWrapper(inner Connection) Connection {
	return newConnectionWrapper(inner)
}


type TcpConnectionOptions struct {
	Context context.Context
}

func NewTcpConnection(addr string) (Connection, error) {
	return NewTcpConnectionWith(addr, nil)
}

func NewTcpConnectionWith(a string, o *TcpConnectionOptions)(Connection,error){
	if o == nil {
		o = &TcpConnectionOptions{}
	}

	if o.Context == nil {
		o.Context = context.Background()
	}

	return dialTcpConnection(a, o)
}


// Plug two `Connection`s `a` and `b` so `Message`s received from `a` with the
// `Protocol` `p` are sent on `b` with `p` and vice versa.
// If `a` is closed for any reason then close `b` and return `a` closing
// `error` ; ditto if `b` is closed.
//
// func PlugConnections(a, b Connection, p Protocol) error {
// 	return PlugConnectionsWithLogger(a, b, p, sio.NewNopLogger())
// }

// func PlugConnectionsWithLogger(a,b Connection, p Protocol, l sio.Logger) error{
// 	return plugConnections(a, b, p, l)
// }


// ----------------------------------------------------------------------------


type chanConnection struct {
	send chan []byte
	recv chan []byte
}

func newChanConnection(bufsize int) (*chanConnection, *chanConnection) {
	var left, right chanConnection
	var a, b chan []byte

	a = make(chan []byte, bufsize)
	b = make(chan []byte, bufsize)

	left.send, right.recv = a, a
	left.recv, right.send = b, b

	return &left, &right
}

func (this *chanConnection) Send(msg Message, proto Protocol) error {
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	func () {
		defer func () {
			if recover() != nil {
				fmt.Printf("send recovery on %p\n", this.send)
				err = io.EOF
			}
		}()

		this.send <- buf.Bytes()
	}()

	return err
}

func (this *chanConnection) Recv(proto Protocol) (Message, error) {
	var b []byte
	var ok bool

	b, ok = <-this.recv
	if !ok {
		return nil, io.EOF
	}

	return proto.Decode(sio.NewReaderSource(bytes.NewBuffer(b)))
}

func (this *chanConnection) Close() error {
	var safeClose func (c chan []byte)

	safeClose = func (c chan []byte) {
		defer func () { recover() }()
		close(c)
	}

	safeClose(this.send)
	safeClose(this.recv)

	return nil
}


type ioConnection struct {
	rlock sync.Mutex
	reader io.ReadCloser
	wlock sync.Mutex
	writer io.WriteCloser
	clock sync.Mutex
	closed bool
}

func newIoConnection(reader io.ReadCloser,writer io.WriteCloser)*ioConnection {
	var this ioConnection

	this.reader = reader
	this.writer = writer
	this.closed = false

	return &this
}

func (this *ioConnection) Send(msg Message, proto Protocol) error {
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	this.wlock.Lock()
	defer this.wlock.Unlock()

	_, err = this.writer.Write(buf.Bytes())

	return err
}

func (this *ioConnection) Recv(proto Protocol) (Message, error) {
	this.rlock.Lock()
	defer this.rlock.Unlock()
	return proto.Decode(sio.NewReaderSource(this.reader))
}

func (this *ioConnection) Close() error {
	var rerr, werr error

	this.clock.Lock()
	if this.closed {
		this.clock.Unlock()
		return nil
	} else {
		this.closed = true
	}
	this.clock.Unlock()

	rerr = this.reader.Close()
	werr = this.writer.Close()

	if rerr != nil {
		return rerr
	} else if werr != nil {
		return werr
	} else {
		return nil
	}
}


type connectionWrapper struct {
	inner Connection
	lock sync.Mutex
	closed bool
}

func newConnectionWrapper(inner Connection) *connectionWrapper {
	var this connectionWrapper

	this.inner = inner
	this.closed = false

	return &this
}

func (this *connectionWrapper) Send(msg Message, proto Protocol) error {
	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return io.EOF
	}
	this.lock.Unlock()

	return this.inner.Send(msg, proto)
}

func (this *connectionWrapper) Recv(proto Protocol) (Message, error) {
	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return nil, io.EOF
	}
	this.lock.Unlock()

	return this.inner.Recv(proto)
}

func (this *connectionWrapper) Close() error {
	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return nil
	} else {
		this.closed = true
	}
	this.lock.Unlock()

	return nil
}


type tcpConnection struct {
	sendLock sync.Mutex
	recvLock sync.Mutex
	closeLock sync.Mutex
	conn net.Conn
	closed bool
}

func newTcpConnection(conn net.Conn) *tcpConnection {
	var this tcpConnection

	this.conn = conn
	this.closed = false

	return &this
}

func dialTcpConnection(addr string, opts *TcpConnectionOptions) (Connection, error) {
	var this tcpConnection
	var dialer net.Dialer
	var err error

	this.conn, err = dialer.DialContext(opts.Context, "tcp", addr)
	if err != nil {
		return nil, err
	}

	this.closed = false

	return &this, nil
}

func (this *tcpConnection) Send(msg Message, proto Protocol) error {
	this.sendLock.Lock()
	defer this.sendLock.Unlock()
	return proto.Encode(sio.NewWriterSink(this.conn), msg)
}

func (this *tcpConnection) Recv(proto Protocol) (Message, error) {
	this.recvLock.Lock()
	defer this.recvLock.Unlock()
	return proto.Decode(sio.NewReaderSource(this.conn))
}

func (this *tcpConnection) Close() error {
	this.closeLock.Lock()
	defer this.closeLock.Unlock()

	if this.closed {
		return nil
	}

	this.closed = true

	return this.conn.Close()
}


// type tcpConnection struct {
// 	slock sync.Mutex
// 	rlock sync.Mutex
// 	conn net.Conn
// 	clock sync.Mutex
// 	closed bool
// }

// func newTcpConnection(ctx context.Context, addr string)(*tcpConnection,error) {
// 	var this tcpConnection
// 	var dialer net.Dialer
// 	var conn net.Conn
// 	var err error

// 	if ctx == nil {
// 		conn, err = dialer.Dial("tcp", addr)
// 	} else {
// 		conn, err = dialer.DialContext(ctx, "tcp", addr)
// 	}

// 	if err != nil {
// 		return nil, err
// 	}

// 	this.conn = conn
// 	this.closed = false

// 	return &this, nil
// }

// func wrapTcpConnection(conn net.Conn) *tcpConnection {
// 	var this tcpConnection

// 	this.conn = conn
// 	this.closed = false

// 	return &this
// }

// func (this *tcpConnection) Send(msg Message, proto Protocol) error {
// 	this.slock.Lock()
// 	defer this.slock.Unlock()
// 	return proto.Encode(sio.NewWriterSink(this.conn), msg)
// }

// func (this *tcpConnection) Recv(proto Protocol) (Message, error) {
// 	this.rlock.Lock()
// 	defer this.rlock.Unlock()
// 	return proto.Decode(sio.NewReaderSource(this.conn))
// }

// func (this *tcpConnection) Close() error {
// 	this.clock.Lock()
// 	defer this.clock.Unlock()

// 	if this.closed {
// 		return nil
// 	}

// 	this.closed = true

// 	return this.conn.Close()
// }


func plugConnections(a, b Connection, p Protocol, log sio.Logger) error {
	var aerrc chan error = make(chan error)
	var berrc chan error = make(chan error)
	var err, e error

	log.Trace("start connection bridge")

	go func () {
		aerrc <- plugConnectionsForward(a, b, p, log)
		close(aerrc)
	}()

	go func () {
		berrc <- plugConnectionsBackward(a, b, p, log)
		close(berrc)
	}()

	err = <-aerrc
	e = <-berrc
	if err == nil {
		err = e
	}

	a.Close()
	b.Close()

	return err
}

func plugConnectionsForward(a,b Connection, p Protocol, log sio.Logger) error {
	var msg Message
	var err error

	for {
		msg, err = a.Recv(p)
		if err != nil {
			a.Close()
			log.Trace("stop connection bridge forward")
			break
		}

		log.Trace("transfer forward %T:%v", log.Emph(2, msg), msg)

		err = b.Send(msg, p)
		if err != nil {
			b.Close()
			break
		}
	}

	return err
}

func plugConnectionsBackward(a,b Connection,p Protocol, log sio.Logger) error {
	var msg Message
	var err error

	for {
		msg, err = b.Recv(p)
		if err != nil {
			b.Close()
			log.Trace("stop connection bridge backward")
			break
		}

		log.Trace("transfer backward %T:%v", log.Emph(2, msg), msg)

		err = a.Send(msg, p)
		if err != nil {
			a.Close()
			break
		}
	}

	return err
}
