package net


import (
	"context"
	"fmt"
	"io"
	sio "silk/io"
	"time"
	"sync"
)


// ----------------------------------------------------------------------------


type BindingService interface {
	Server

	Connect(string) (Connection, error)

	// Handle the given `BindingMessage` coming from the given
	// `Connection`.
	// The caller must not use the given `Connection` anymore.
	// The `BindingService` takes care of closing `Connection` when
	// necessary.
	//
	Handle(*BindingMessage, Connection)

	Insert(string, Binding)

	Delete(string)

	Has(string) bool

	Status() map[string]BindingStatus
}

func NewBindingService() BindingService {
	return newBindingService(sio.NewNopLogger())
}

func NewBindingServiceWithLogger(log sio.Logger) BindingService {
	return newBindingService(log)
}

// Request the remote TPC peer at `addr` to create a new binding.
// The binding receives the given `name` on the remote peer side.
// Messages are encoded over new connections according to the given `proto`.
// If the remote peer accepts to establish the binding then return the new
// local end of the binding and `nil`.
// If the remote peers denies the binding then return `nil` and an error
// indicating the deny reason.
//
func RequestTcpBinding(addr, name string, proto Protocol) (Binding, error) {
	return requestTcpBinding(addr, name, proto, sio.NewNopLogger())
}

func RequestTcpBindingWithLogger(addr, name string, proto Protocol, log sio.Logger) (Binding, error) {
	return requestTcpBinding(addr, name, proto, log)
}

func RequestBindingDeletion(remoteName string) *BindingMessage {
	return nil
}

type BindingMessage struct {
	payload Message
}

type BindingStatus = uint8

const (
	BINDING_STATUS_CONNECTED    BindingStatus = 0
	BINDING_STATUS_DISCONNECTED BindingStatus = 1
)


// ----------------------------------------------------------------------------


type bindingService struct {
	log sio.Logger
	lock sync.Mutex
	bindings map[string]Binding
	mplexers map[string]Multiplexer
	newMultiplexers chan Multiplexer
	newConnections chan Connection
}

func newBindingService(log sio.Logger) *bindingService {
	var this bindingService

	this.log = log
	this.bindings = make(map[string]Binding)
	this.mplexers = make(map[string]Multiplexer)
	this.newMultiplexers = make(chan Multiplexer)
	this.newConnections = make(chan Connection)

	go this.handleNewMultiplexers()

	return &this
}

func (this *bindingService) handleNewMultiplexers() {
	var mplex Multiplexer

	for mplex = range this.newMultiplexers {
		go func (m Multiplexer) {
			var c Connection
			var e error

			for {
				c, e = m.Accept()
				if e != nil {
					return
				}

				this.newConnections <- c
			}
		}(mplex)
	}
}

func (this *bindingService) Accept() (Connection, error) {
	var conn Connection
	var ok bool

	conn, ok = <-this.newConnections
	if !ok {
		return conn, io.EOF
	}

	return conn, nil
}

func (this *bindingService) Close() error {
	panic("unimplemented")
	return nil
}

func (this *bindingService) Connect(name string) (Connection, error) {
	var mplex Multiplexer

	this.lock.Lock()
	mplex = this.mplexers[name]
	this.lock.Unlock()

	if mplex == nil {
		return nil, fmt.Errorf("TODO: proper error")
	}

	return mplex.Connect()
}

func (this *bindingService) handleReset(msg *bindingReset, conn Connection) {
	var mplex, nmplex Multiplexer
	var reply bindingReplyOk
	var new PassiveBinding
	var old Binding
	var err error

	err = conn.Send(&BindingMessage{ &reply }, NewRawProtocol(nil))
	if err != nil {
		conn.Close()
		return
	}

	new = NewPassiveBindingWithLogger(this.log.
		WithLocalContext("[" + msg.name + "]"))
	nmplex = NewFifoMultiplexerWithLogger(new, this.log.
		WithLocalContext("[" + msg.name + "]").
		WithLocalContext("mplex"))

	this.lock.Lock()

	old = this.bindings[msg.name]
	mplex = this.mplexers[msg.name]

	this.log.Debug("reset binding '%s'", this.log.Emph(0, msg.name))

	this.bindings[msg.name] = new
	this.mplexers[msg.name] = nmplex

	this.lock.Unlock()

	if old != nil {
		mplex.Close()
		old.Close()
	}

	new.Reconnect(conn)

	this.newMultiplexers <- nmplex
}

func (this *bindingService) handleReconnect(msg *bindingReconnect, conn Connection) {
	var passive PassiveBinding
	var reply BindingMessage
	var oldconn Connection
	var binding Binding
	var err error
	var ok bool

	this.lock.Lock()
	binding = this.bindings[msg.name]
	this.lock.Unlock()

	if binding == nil {
		reply.payload = &bindingReplyUnknown{}
		ok = false
	} else {
		passive, ok = binding.(PassiveBinding)
		if !ok {
			reply.payload = &bindingReplyActive{} 
		} else {
			reply.payload = &bindingReplyOk{} 
		}
		this.log.Debug("reconnect binding '%s'",
			this.log.Emph(0, msg.name))
	}

	err = conn.Send(&reply, NewRawProtocol(nil))
	if err != nil {
		conn.Close()
		return
	} else if !ok {
		return
	}

	oldconn = passive.Reconnect(conn)
	if oldconn != nil {
		oldconn.Close()
	}
}

func (this *bindingService) handleDelete(msg *bindingDelete, conn Connection) {
	panic("unimplemeted")
}

func (this *bindingService) Handle(msg *BindingMessage, conn Connection) {
	switch m := msg.payload.(type) {
	case *bindingReset:
		this.handleReset(m, conn)
	case *bindingReconnect:
		this.handleReconnect(m, conn)
	case *bindingDelete:
		this.handleDelete(m, conn)
	default:
		this.log.Warn("handle unknown message %T:%v",
			this.log.Emph(1, msg.payload), msg.payload)
		conn.Close()
	}
}

func (this *bindingService) Insert(name string, binding Binding) {
	var mplex, nmplex Multiplexer
	var old Binding

	if binding == nil {
		panic("cannot insert nil binding")
	}

	nmplex = NewFifoMultiplexerWithLogger(binding, this.log.
		WithLocalContext("[" + name + "]").WithLocalContext("mplex"))

	this.lock.Lock()

	old = this.bindings[name]
	mplex = this.mplexers[name]

	this.log.Debug("insert binding '%s'", this.log.Emph(0, name))

	this.bindings[name] = binding
	this.mplexers[name] = nmplex

	this.lock.Unlock()

	if old != nil {
		mplex.Close()
		old.Close()
	}

	this.newMultiplexers <- nmplex
}

func (this *bindingService) Delete(name string) {
	var mplex Multiplexer
	var old Binding

	this.lock.Lock()

	old = this.bindings[name]

	if old != nil {
		this.log.Debug("delete binding '%s'", this.log.Emph(0, name))
		mplex = this.mplexers[name]
		delete(this.bindings, name)
		delete(this.mplexers, name)
	}

	this.lock.Unlock()

	if old != nil {
		mplex.Close()
		old.Close()
	}
}

func (this *bindingService) Has(name string) bool {
	var found bool

	this.lock.Lock()
	_, found = this.bindings[name]
	this.lock.Unlock()

	return found
}

func (this *bindingService) Status() map[string]BindingStatus {
	return nil
}


func requestTcpBinding(addr, name string, proto Protocol, log sio.Logger) (Binding, error) {
	var errc chan error = make(chan error)
	var reconnect bindingReconnect
	var reset bindingReset
	var msg BindingMessage
	var ret Binding
	var err error

	reset.name = name
	reconnect.name = name
	msg.payload = &reset

	ret = NewActiveBindingWithLogger(func (ctx context.Context) Connection{
		var reply *BindingMessage
		var conn Connection

		for {
			if ctx.Err() != nil {
				log.Trace("abort connect '%s' as '%s'",
					log.Emph(2, addr), log.Emph(0, name))
				return nil
			}

			log.Trace("try connect '%s' as '%s'",
				log.Emph(2, addr), log.Emph(0, name))
			conn, reply = connectTcpBinding(ctx, addr, &msg, proto)

			if conn == nil {
				select {
				case <-ctx.Done():
				case <-time.After(1 * time.Second):
				}

				continue
			}

			switch reply.payload.(type) {
			case *bindingReplyOk:
				if msg.payload == &reset {
					log.Debug("connected to '%s' as '%s'",
						log.Emph(2, addr),
						log.Emph(0, name))
					close(errc)
					msg.payload = &reconnect
				}
				return conn
			default:
				if msg.payload == &reset {
					errc <- fmt.Errorf("TODO real error")
					close(errc)
				}
				<-ctx.Done()
			}
		}
	}, log)

	err = <-errc

	if err == nil {
		return ret, nil
	} else {
		ret.Close()
		return nil, err
	}
}

func connectTcpBinding(ctx context.Context, addr string, msg *BindingMessage, proto Protocol) (Connection, *BindingMessage) {
	var done chan struct{}
	var conn Connection
	var rmsg Message
	var err error

	conn, err = NewTcpConnectionWith(addr, &TcpConnectionOptions{
		Context: ctx,
	})
	if err != nil {
		return nil, nil
	}

	done = make(chan struct{})

	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()

	err = conn.Send(msg, proto)
	if err != nil {
		conn.Close()
		close(done)
		return nil, nil
	}

	rmsg, err = conn.Recv(NewRawProtocol(&BindingMessage{}))
	if err != nil {
		conn.Close()
		close(done)
		return nil, nil
	}

	close(done)
	return conn, rmsg.(*BindingMessage)
}


var bindingProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &bindingReset{},
	1: &bindingReconnect{},
	2: &bindingDelete{},
	3: &bindingReplyOk{},            // request successful
	4: &bindingReplyUnknown{},       // unknown binding name
	5: &bindingReplyActive{},        // cannot connect to active binding
})


func (this *BindingMessage) Encode(sink sio.Sink) error {
	return bindingProtocol.Encode(sink, this.payload)
}

func (this *BindingMessage) Decode(source sio.Source) error {
	var err error
	this.payload, err = bindingProtocol.Decode(source)
	return err
}


type bindingRequest struct {
	name string
}

func (this *bindingRequest) Encode(sink sio.Sink) error {
	return sink.WriteString8(this.name).Error()
}

func (this *bindingRequest) Decode(source sio.Source) error {
	return source.ReadString8(&this.name).Error()
}


type bindingReset struct {
	bindingRequest
}

type bindingReconnect struct {
	bindingRequest
}

type bindingDelete struct {
	bindingRequest
}


type bindingReply struct {
}

func (this *bindingReply) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *bindingReply) Decode(source sio.Source) error {
	return source.Error()
}


type bindingReplyOk struct {
	bindingReply
}

type bindingReplyUnknown struct {
	bindingReply
}

type bindingReplyActive struct {
	bindingReply
}
