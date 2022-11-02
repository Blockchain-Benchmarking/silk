package net


import (
	"bytes"
	"context"
	"net"
	sio "silk/io"
	"sync"
)


// ----------------------------------------------------------------------------


type Connection interface {
	Sender
	Receiver
}


func NewLocalConnection(n int) (Connection, Connection) {
	return newLocalConnection(n)
}


func NewPiecewiseConnection(sender Sender, receiver Receiver) Connection {
	return newPiecewiseConnection(sender, receiver)
}

func NewTcpConnection(addr string) Connection {
	return dialTcpConnection(addr)
}


// ----------------------------------------------------------------------------


type localConnection struct {
	sendc chan MessageProtocol
	recvc chan []byte
}

func newLocalConnection(n int) (*localConnection, *localConnection) {
	var a, b localConnection

	a.sendc = make(chan MessageProtocol)
	a.recvc = make(chan []byte, n)

	b.sendc = make(chan MessageProtocol)
	b.recvc = make(chan []byte, n)

	go a.encode(b.recvc)
	go b.encode(a.recvc)

	return &a, &b
}

func (this *localConnection) encode(dest chan<- []byte) {
	var mp MessageProtocol
	var buf *bytes.Buffer
	var closed bool
	var err error

	closed = false

	for mp = range this.sendc {
		if closed {
			continue
		}

		buf = bytes.NewBuffer(nil)
		err = mp.P.Encode(sio.NewWriterSink(buf), mp.M)
		if err != nil {
			close(dest)
			closed = true
			continue
		}

		dest <- buf.Bytes()
	}
}

func (this *localConnection) decode(dest chan<- Message, p Protocol, n int) {
	var msg Message
	var more bool
	var err error
	var b []byte

	for {
		if n == 0 {
			break
		}

		b, more = <-this.recvc
		if more == false {
			break
		}

		msg, err = p.Decode(sio.NewReaderSource(bytes.NewBuffer(b)))
		if err != nil {
			break
		}

		dest <- msg

		if n > 0 {
			n -= 1
		}
	}

	close(dest)
}

func (this *localConnection) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *localConnection) Recv(proto Protocol) <-chan Message {
	var c chan Message = make(chan Message)

	go this.decode(c, proto, -1)

	return c
}

func (this *localConnection) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go this.decode(c, proto, n)

	return c
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type piecewiseConnection struct {
	sender Sender
	receiver Receiver
}

func newPiecewiseConnection(s Sender, r Receiver) *piecewiseConnection {
	var this piecewiseConnection

	this.sender = s
	this.receiver = r

	return &this
}

func (this *piecewiseConnection) Send() chan<- MessageProtocol {
	return this.sender.Send()
}

func (this *piecewiseConnection) Recv(proto Protocol) <-chan Message {
	return this.receiver.Recv(proto)
}

func (this *piecewiseConnection) RecvN(proto Protocol, n int) <-chan Message {
	return this.receiver.RecvN(proto, n)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type tcpConnection struct {
	lock sync.Mutex
	cond *sync.Cond
	ready bool
	conn net.Conn
	sendc chan MessageProtocol
}

func newTcpConnection(conn net.Conn) *tcpConnection {
	var this tcpConnection

	this.cond = sync.NewCond(&this.lock)
	this.ready = true
	this.conn = conn
	this.sendc = make(chan MessageProtocol, 32)

	go this.encode()

	return &this
}

func dialTcpConnection(addr string) *tcpConnection {
	var this tcpConnection

	this.cond = sync.NewCond(&this.lock)
	this.ready = false
	this.sendc = make(chan MessageProtocol, 32)

	go this.dial(addr)

	return &this
}

func (this *tcpConnection) dial(addr string) {
	var dialer net.Dialer
	var conn net.Conn

	conn, _ = dialer.DialContext(context.Background(), "tcp", addr)

	this.lock.Lock()

	this.conn = conn
	this.ready = true

	this.cond.Broadcast()
	this.lock.Unlock()

	this.encode()
}

func (this *tcpConnection) encode() {
	var mp MessageProtocol
	var closed bool
	var err error

	closed = (this.conn == nil)

	for mp = range this.sendc {
		if closed {
			continue
		}

		err = mp.P.Encode(sio.NewWriterSink(this.conn), mp.M)
		if err != nil {
			this.conn.Close()
			closed = true
		}
	}

	if this.conn != nil {
		this.conn.Close()
	}
}

func (this *tcpConnection) hasConnected() bool {
	this.lock.Lock()
	defer this.lock.Unlock()

	for this.ready == false {
		this.cond.Wait()
	}

	return (this.conn != nil)
}

func (this *tcpConnection) decode(dest chan<- Message, p Protocol, n int) {
	var msg Message
	var err error

	if this.hasConnected() == false {
		close(dest)
		return
	}

	for {
		if n == 0 {
			break
		}

		msg, err = p.Decode(sio.NewReaderSource(this.conn))
		if err != nil {
			this.conn.Close()
			break
		}

		dest <- msg

		if n > 0 {
			n -= 1
		}
	}

	close(dest)
}

func (this *tcpConnection) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *tcpConnection) Recv(proto Protocol) <-chan Message {
	var c chan Message = make(chan Message, 32)

	go this.decode(c, proto, -1)

	return c
}

func (this *tcpConnection) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go this.decode(c, proto, n)

	return c
}
