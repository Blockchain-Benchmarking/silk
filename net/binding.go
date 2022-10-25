package net


import (
	"bytes"
	"context"
	"fmt"
	"io"
	sio "silk/io"
	"sync"
)


// ----------------------------------------------------------------------------


type Binding interface {
	// Acts as a reliable `Channel`.
	//
	// If the underlying connection to the remote peer ends unexpectedly
	// then keep the sent messages locally until there is an opportunity
	// to retransmit or until this `Binding` is closed.
	//
	// Until this `Binding` is closed, there is always a possibility of
	// sending or receiving messages and no error can be reported.
	//
	Channel

	// Close this `Binding`.
	//
	// This also closes the underlying connection.
	//
	// Return the list of messages which were never sent (the remote peer
	// has not received them) and the list of message which have been sent
	// but not acknowledged (the remote peer may or may not have received
	// them).
	//
	Close() ([]Message, []Message)
}

func NewActiveBinding(connect func (context.Context) Connection) Binding {
	return newActiveBinding(connect, sio.NewNopLogger())
}

func NewActiveBindingWithLogger(connect func (context.Context) Connection, log sio.Logger) Binding {
	return newActiveBinding(connect, log)
}


type PassiveBinding interface {
	Binding

	// Change the underlying connection.
	// Return the previous connection or `nil` if it was disconnected.
	//
	Reconnect(Connection) Connection

	// Stop using the underlying connection and return it.
	// Return `nil` if there was no connection.
	//
	Disconnect() Connection
}

func NewPassiveBinding() PassiveBinding {
	return newBlockingBinding(sio.NewNopLogger())
}

func NewPassiveBindingWithLogger(log sio.Logger) PassiveBinding {
	return newBlockingBinding(log)
}


func NewBindingConnection(binding Binding) Connection {
	return newBindingConnection(binding)
}


// ----------------------------------------------------------------------------


type blockingBinding struct {
	log sio.Logger
	clock sync.Mutex
	ccond *sync.Cond
	conn *blockingBindingConnection
	closed bool
	unsent Message
	unacked Message
	slock sync.Mutex
	snum uint64
	rnum uint64
}

func newBlockingBinding(log sio.Logger) *blockingBinding {
	var this blockingBinding

	this.log = log
	this.ccond = sync.NewCond(&this.clock)
	this.conn = nil
	this.closed = false
	this.unsent = nil
	this.unacked = nil
	this.snum = 0
	this.rnum = 0

	return &this
}

func (this *blockingBinding) Send(msg Message, proto Protocol) error {
	var seq blockingBindingSeq
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		panic(fmt.Sprintf("cannot encode message %v: %v", msg, err))
	}

	seq.content = buf.Bytes()

	this.slock.Lock()

	seq.number = this.snum
	this.snum += 1

	err = this.send(msg, &seq)

	this.slock.Unlock()

	return err
}

func (this *blockingBinding) send(msg Message, seq *blockingBindingSeq) error {
	var conn *blockingBindingConnection
	var err error

	this.clock.Lock()

	this.unsent = msg

	for {
		if this.closed {
			err = io.EOF
			break
		} else if this.conn == nil {
			this.ccond.Wait()
			continue
		}

		conn = this.conn

		if this.unsent != nil {
			this.unacked = this.unsent
			this.unsent = nil
		}

		this.clock.Unlock()

		err = this.sendOn(seq, conn)

		this.clock.Lock()

		if err == nil {
			this.log.Trace("received ack for sequence number %d",
				this.log.Emph(1, seq.number))
			break
		} else if conn == this.conn {
			this.conn = nil
		}
	}

	this.unsent = nil
	this.unacked = nil

	this.clock.Unlock()

	return err
}

func (this *blockingBinding) sendOn(seq *blockingBindingSeq, conn *blockingBindingConnection) error {
	var err error

	this.log.Trace("try send %d bytes with sequence number %d",
		len(seq.content), this.log.Emph(1, seq.number))
	err = conn.send(seq)
	if err != nil {
		return err
	}

	_, err = conn.recvAck()

	return err
}

func (this *blockingBinding) Recv(proto Protocol) (Message, error) {
	var conn *blockingBindingConnection
	var msg Message
	var err error

	this.clock.Lock()

	for {
		if this.closed {
			err = io.EOF
			break
		} else if this.conn == nil {
			this.ccond.Wait()
			continue
		}

		conn = this.conn

		this.clock.Unlock()

		msg, err = this.recvOn(proto, conn)

		this.clock.Lock()

		if err == nil {
			break
		} else if conn == this.conn {
			this.conn = nil
		}
	}

	this.clock.Unlock()

	return msg, err
}

func (this *blockingBinding) recvOn(proto Protocol, conn *blockingBindingConnection) (Message, error) {
	var seq *blockingBindingSeq
	var buf *bytes.Buffer
	var msg Message
	var err error

	for {
		seq, err = conn.recvSeq()
		if err != nil {
			return nil, err
		}

		buf = bytes.NewBuffer(seq.content)
		msg, err = proto.Decode(sio.NewReaderSource(buf))
		if err != nil {
			return nil, err
		}

		err = conn.send(&blockingBindingAck{})
		if err != nil {
			return nil, err
		}

		if seq.number == this.rnum {
			this.rnum += 1
			break
		}
	}

	return msg, nil
}

func (this *blockingBinding) Reconnect(conn Connection) Connection {
	var oconn Connection

	if conn == nil {
		panic("connection cannot be nil")
	}

	this.clock.Lock()

	if this.conn == nil {
		oconn = nil
	} else {
		oconn = this.conn.conn
	}

	this.conn = newBlockingBindingConnection(conn)

	this.ccond.Broadcast()
	this.clock.Unlock()

	return oconn
}

func (this *blockingBinding) Disconnect() Connection {
	var conn Connection

	this.clock.Lock()

	conn = this.conn.conn
	this.conn = nil

	this.clock.Unlock()

	return conn
}

func (this *blockingBinding) Close() ([]Message, []Message) {
	var unsent, unacked []Message

	this.clock.Lock()

	this.closed = true

	if this.conn != nil {
		this.conn.conn.Close()
		this.conn = nil
	}

	if this.unsent != nil {
		unsent = []Message{ this.unsent }
		this.unsent = nil
	} else {
		unsent = []Message{}
	}

	if this.unacked != nil {
		unacked = []Message{ this.unacked }
		this.unacked = nil
	} else {
		unacked = []Message{}
	}

	this.clock.Unlock()

	return unsent, unacked
}


type blockingBindingConnection struct {
	lock sync.Mutex
	cond *sync.Cond
	conn Connection
	sending bool
	receiving bool
	cache Message
}

func newBlockingBindingConnection(conn Connection) *blockingBindingConnection {
	var this blockingBindingConnection

	this.cond = sync.NewCond(&this.lock)
	this.conn = conn
	this.sending = false
	this.receiving = false
	this.cache = nil

	return &this
}

func (this *blockingBindingConnection) send(msg Message) error {
	var err error

	this.lock.Lock()

	for {
		if this.sending {
			this.cond.Wait()
			continue
		} else {
			this.sending = true
			break
		}
	}

	this.lock.Unlock()

	err = this.conn.Send(msg, blockingBindingProtocol)
	if err != nil {
		this.conn.Close()
	}

	this.lock.Lock()

	this.sending = false
	this.cond.Broadcast()

	this.lock.Unlock()

	return err
}

func (this *blockingBindingConnection) recv(accept func (Message) bool) error {
	var msg Message
	var err error

	this.lock.Lock()

	for {
		if this.cache != nil && accept(this.cache) {
			this.cache = nil
			this.lock.Unlock()
			return nil
		} else if this.receiving {
			this.cond.Wait()
			continue
		} else {
			this.receiving = true
			break
		}
	}

	this.lock.Unlock()

	for {
		msg, err = this.conn.Recv(blockingBindingProtocol)
		if err != nil {
			this.conn.Close()
			break
		}

		if accept(msg) {
			break
		}

		this.lock.Lock()
		this.cache = msg
		this.cond.Broadcast()
		this.lock.Unlock()
	}

	this.lock.Lock()

	this.receiving = false
	this.cond.Broadcast()

	this.lock.Unlock()

	return err
}

func (this *blockingBindingConnection) recvAck() (*blockingBindingAck, error) {
	var ack *blockingBindingAck
	var err error

	err = this.recv(func (msg Message) bool {
		var ok bool

		ack, ok = msg.(*blockingBindingAck)

		return ok
	})

	return ack, err
}

func (this *blockingBindingConnection) recvSeq() (*blockingBindingSeq, error) {
	var seq *blockingBindingSeq
	var err error

	err = this.recv(func (msg Message) bool {
		var ok bool

		seq, ok = msg.(*blockingBindingSeq)

		return ok
	})

	return seq, err
}


var blockingBindingProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &blockingBindingAck{},
	1: &blockingBindingSeq{},
})


type blockingBindingSeq struct {
	content []byte
	number uint64
}

func (this *blockingBindingSeq) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).
		WriteUint64(this.number).
		Error()
}

func (this *blockingBindingSeq) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).
		ReadUint64(&this.number).
		Error()
}


type blockingBindingAck struct {
}

func (this *blockingBindingAck) Encode(sio.Sink) error {
	return nil
}

func (this *blockingBindingAck) Decode(source sio.Source) error {
	return nil
}


type activeBinding struct {
	passive PassiveBinding
	cancel context.CancelFunc
}

func newActiveBinding(cb func (context.Context) Connection, log sio.Logger) *activeBinding {
	var ctx context.Context
	var this activeBinding

	this.passive = NewPassiveBindingWithLogger(log)
	ctx, this.cancel = context.WithCancel(context.Background())

	go this.reconnect(ctx, cb)

	return &this
}

func (this *activeBinding) reconnect(ctx context.Context, cb func (context.Context) Connection) {
	var closeSignal chan struct{} = make(chan struct{})
	var aconn *activeBindingConnection
	var conn Connection

	for {
		if ctx.Err() != nil {
			break
		}

		conn = cb(ctx)
		if conn == nil {
			if ctx.Err() != nil {
				break
			}
			panic("active binding cannot use nil Connection")
		}

		aconn = newActiveBindingConnection(conn, closeSignal)
		this.passive.Reconnect(aconn)

		<-closeSignal
	}

	close(closeSignal)
}

func (this *activeBinding) Close() ([]Message, []Message) {
	this.cancel()
	return this.passive.Close()
}

func (this *activeBinding) Send(msg Message, proto Protocol) error {
	return this.passive.Send(msg, proto)
}

func (this *activeBinding) Recv(proto Protocol) (Message, error) {
	return this.passive.Recv(proto)
}


type activeBindingConnection struct {
	inner Connection
	closed bool
	closeSignal chan<- struct{}
}

func newActiveBindingConnection(inner Connection, closeSignal chan<- struct{}) *activeBindingConnection {
	return &activeBindingConnection{ inner, false, closeSignal }
}

func (this *activeBindingConnection) Close() error {
	if this.closed {
		return nil
	}
	this.closed = true
	this.closeSignal <- struct{}{}
	return this.inner.Close()
}

func (this *activeBindingConnection) Send(msg Message, proto Protocol) error {
	return this.inner.Send(msg, proto)
}

func (this *activeBindingConnection) Recv(proto Protocol) (Message, error) {
	return this.inner.Recv(proto)
}


type bindingConnection struct {
	binding Binding
}

func newBindingConnection(binding Binding) Connection {
	return &bindingConnection{ binding }
}

func (this *bindingConnection) Send(msg Message, proto Protocol) error {
	return this.binding.Send(msg, proto)
}

func (this *bindingConnection) Recv(proto Protocol) (Message, error) {
	return this.binding.Recv(proto)
}

func (this *bindingConnection) Close() error {
	var unsent, unacked []Message

	unsent, unacked = this.binding.Close()
	if (len(unsent) == 0) && (len(unacked) == 0) {
		return nil
	}

	return fmt.Errorf("TODO: proper error")
}
