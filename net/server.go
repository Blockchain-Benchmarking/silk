package net


import (
	"io"
	"net"
	"sync"
)


// ----------------------------------------------------------------------------


type Server interface {
	io.Closer

	Accept() (Connection, error)
}


func NewTcpServer(addr string) (Server, error) {
	return newTcpServer(addr)
}


type ServerChannel interface {
	io.Closer

	Accept() <-chan Connection

	Err() error
}

func NewServerChannel(inner Server) ServerChannel {
	return newServerChannel(inner)
}


type ServerMessageChannel interface {
	io.Closer

	Accept() <-chan ConnectionMessage

	Err() error
}

func NewServerMessageChannel(inner Server, p Protocol) ServerMessageChannel {
	return newServerMessageChannel(inner, p)
}


type ConnectionMessage interface {
	Connection() Connection

	Message() Message
}


// ----------------------------------------------------------------------------


type tcpServer struct {
	listener net.Listener
}

func newTcpServer(addr string) (*tcpServer, error) {
	var listener net.Listener
	var err error

	listener, err = net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &tcpServer{ listener }, nil
}

func (this *tcpServer) Accept() (Connection, error) {
	var conn net.Conn
	var err error

	conn, err = this.listener.Accept()
	if err != nil {
		return nil, err
	}

	return newTcpConnection(conn), nil
}

func (this *tcpServer) Close() error {
	return this.listener.Close()
}


type serverChannel struct {
	inner Server
	lock sync.Mutex
	connc chan Connection
	err error
	closed bool
}

func newServerChannel(inner Server) *serverChannel {
	var this serverChannel

	this.inner = inner
	this.connc = make(chan Connection)
	this.err = nil
	this.closed = false

	go this.run()

	return &this
}

func (this *serverChannel) run() {
	var conn Connection
	var err error

	for {
		conn, err = this.inner.Accept()

		if (conn == nil) || (err != nil) {
			break
		}

		this.connc <- conn
	}

	this.lock.Lock()
	if !this.closed {
		this.err = err
	}
	this.lock.Unlock()

	close(this.connc)

	this.inner.Close()
}

func (this *serverChannel) purge() {
	var conn Connection

	for conn = range this.connc {
		conn.Close()
	}
}

func (this *serverChannel) Close() error {
	var err error

	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return nil
	}
	this.closed = true
	this.lock.Unlock()

	err =  this.inner.Close()

	this.purge()

	return err
}

func (this *serverChannel) Accept() <-chan Connection {
	return this.connc
}

func (this *serverChannel) Err() error {
	return this.err
}


type serverMessageChannel struct {
	inner ServerChannel
	proto Protocol
	cmsgc chan ConnectionMessage
	lock sync.Mutex
	aconn Connection
	err error
	closed bool
}

func newServerMessageChannel(inner Server, p Protocol) *serverMessageChannel {
	var this serverMessageChannel

	this.inner = NewServerChannel(inner)
	this.proto = p
	this.cmsgc = make(chan ConnectionMessage)
	this.aconn = nil
	this.err = nil
	this.closed = false

	go this.run()

	return &this
}

func (this *serverMessageChannel) run() {
	var conn Connection
	var msg Message
	var abort bool
	var err error

	abort = false

	for conn = range this.inner.Accept() {
		this.lock.Lock()
		if this.closed {
			abort = true
		} else {
			this.aconn = conn
		}
		this.lock.Unlock()

		if abort {
			conn.Close()
			break
		}

		msg, err = conn.Recv(this.proto)
		if err != nil {
			conn.Close()
			continue
		}

		this.lock.Lock()
		abort = this.closed
		this.lock.Unlock()

		if abort {
			break
		}

		this.cmsgc <- newConnectionMessage(conn, msg)
	}

	if err == nil {
		err = this.inner.Err()
	}

	this.lock.Lock()
	if !this.closed {
		this.err = err
	}
	this.lock.Unlock()

	close(this.cmsgc)

	this.inner.Close()
}

func (this *serverMessageChannel) purge() {
	var cmsg ConnectionMessage

	for cmsg = range this.cmsgc {
		cmsg.Connection().Close()
	}
}

func (this *serverMessageChannel) Close() error {
	var aconn Connection
	var err error

	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return nil
	}
	this.closed = true
	aconn = this.aconn
	this.lock.Unlock()

	err = this.inner.Close()

	if aconn != nil {
		aconn.Close()
	}

	this.purge()

	return err
}

func (this *serverMessageChannel) Accept() <-chan ConnectionMessage {
	return this.cmsgc
}

func (this *serverMessageChannel) Err() error {
	return this.err
}


type connectionMessage struct {
	conn Connection
	msg Message
}

func newConnectionMessage(conn Connection, msg Message) *connectionMessage {
	var this connectionMessage

	this.conn = conn
	this.msg = msg

	return &this
}

func (this *connectionMessage) Connection() Connection {
	return this.conn
}

func (this *connectionMessage) Message() Message {
	return this.msg
}
