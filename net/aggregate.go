package net


import (
	"bytes"
	"context"
	sio "silk/io"
	"silk/util/rand"
	"sync"
)


// ----------------------------------------------------------------------------


// A service to `Accept()` aggregated connections.
//
// An aggregated connection is a set of `Connection`s appearing as a single
// `Connection` but which balances the load across the different underlying
// `Connection`s.
//
// An `AggregationService` accepts new `Connection`s by `Handle()`ing incoming
// `AggregationMessage`s coming from a given `Connection`. Once enough of these
// `Connection`s of the same set have been `Handle()`d, a new aggregatd
// `Connection` can be received from the `Accept()` channel.
//
type AggregationService interface {
	// Interface for `Accept()`ing incoming aggregated `Connection`s.
	// The returned accept channel is closed when the `Context` of this
	// service is done.
	//
	Accepter

	// Handle an `AggregationMessage` carried by the associated
	// `Connection` so this connection gets associated to the appropriate
	// aggregated connection.
	//
	// Calling `Handle()` when this service has an associated `Context`
	// that is done is undefined behavior.
	//
	Handle(*AggregationMessage, Connection)
}

type AggregationServiceOptions struct {
	Context context.Context

	Log sio.Logger
}

func NewAggregationService() AggregationService {
	return NewAggregationServiceWith(nil)
}

func NewAggregationServiceWith(opts *AggregationServiceOptions) AggregationService {
	if opts == nil {
		opts = &AggregationServiceOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	return newAggregationService(opts)
}


type AggregatedTcpConnectionOptions struct {
	Log sio.Logger

	N int
}

func NewAggregatedTcpConnection(addr string, p Protocol, n int) Connection {
	return NewAggregatedTcpConnectionWith(addr, p,
		&AggregatedTcpConnectionOptions{
			N: n,
		})
}

func NewAggregatedTcpConnectionWith(addr string, p Protocol, opts *AggregatedTcpConnectionOptions) Connection {
	if opts == nil {
		opts = &AggregatedTcpConnectionOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	if opts.N == 0 {
		opts.N = 1
	}

	return newAggregatedTcpConnection(addr, p, opts)
}


type AggregationMessage struct {
	// The unique id of a set of aggregated connections.
	uid uint64

	// The total number of aggregated connections in the set.
	total uint16
}


// ----------------------------------------------------------------------------


type aggregationService struct {
	log sio.Logger
	reqc chan *aggregationRequest
	acceptc chan Connection
}

func newAggregationService(opts *AggregationServiceOptions)*aggregationService{
	var this aggregationService

	this.log = opts.Log
	this.reqc = make(chan *aggregationRequest)
	this.acceptc = make(chan Connection, 16)

	if opts.Context != nil {
		go func () {
			<-opts.Context.Done()
			close(this.reqc)
		}()
	}

	go this.run()

	return &this
}

func (this *aggregationService) run() {
	var building map[uint64]*connectionAggregator
	var aggregator *connectionAggregator
	var req *aggregationRequest
	var conn Connection
	var found bool
	var uid uint64

	building = make(map[uint64]*connectionAggregator)

	for req = range this.reqc {
		if req.total == 0 {
			this.log.Warn("receive request for 0 connections")
			close(req.conn.Send())
			continue
		}

		aggregator, found = building[req.uid]
		if found == false {
			conn, aggregator = newConnectionAggregator(req.total,
				this.log.WithLocalContext("set[%x]", req.uid))

			aggregator.log.Trace("waiting %d connections",
				req.total)

			building[req.uid] = aggregator
			this.acceptc <- conn
		}

		aggregator.log.Trace("new connection")
		aggregator.senderc <- req.conn
		aggregator.receiverc <- ReceiverProtocol{
			R: req.conn,
			P: aggregationProtocol,
		}
		aggregator.missing -= 1

		if aggregator.missing == 0 {
			aggregator.log.Trace("complete")
			delete(building, req.uid)
			close(aggregator.senderc)
			close(aggregator.receiverc)
		}
	}

	close(this.acceptc)

	for uid, aggregator = range building {
		this.log.Warn("incomplete aggregated connection %x",
			this.log.Emph(1, uid))
		close(aggregator.senderc)
		close(aggregator.receiverc)
	}

	building = nil
}

func (this *aggregationService) Accept() <-chan Connection {
	return this.acceptc
}

func (this *aggregationService) Handle(m *AggregationMessage, conn Connection){
	this.reqc <- &aggregationRequest{
		uid: m.uid,
		total: m.total,
		conn: conn,
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type aggregationRequest struct {
	uid uint64
	total uint16
	conn Connection
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type connectionAggregator struct {
	log sio.Logger
	missing uint16
	senderc chan Sender
	receiverc chan ReceiverProtocol
}

func newConnectionAggregator(n uint16, log sio.Logger) (Connection, *connectionAggregator) {
	var this connectionAggregator

	this.log = log
	this.missing = n
	this.senderc = make(chan Sender)
	this.receiverc = make(chan ReceiverProtocol)

	return newAggregatedConnection(this.senderc, this.receiverc), &this
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type aggregatedConnection struct {
	sendc chan MessageProtocol
	recvc chan []byte
}

func newAggregatedConnection(sc <-chan Sender, rc <-chan ReceiverProtocol) *aggregatedConnection {
	var this aggregatedConnection
	var receiver Receiver
	var sender Sender

	sender = NewSenderAggregator(sc)
	receiver = NewReceiverAggregator(rc)

	this.sendc = make(chan MessageProtocol, 32)
	this.recvc = make(chan []byte, 32)

	go this.encode(sender)
	go this.reorder(receiver)

	return &this
}

func (this *aggregatedConnection) encode(sender Sender) {
	var mp MessageProtocol
	var buf *bytes.Buffer
	var seq uint64
	var err error

	for mp = range this.sendc {
		buf = bytes.NewBuffer(nil)

		err = mp.P.Encode(sio.NewWriterSink(buf), mp.M)
		if err != nil {
			break
		}

		sender.Send() <- MessageProtocol{
			M: &aggregationMessage{ seq, buf.Bytes() },
			P: aggregationProtocol,
		}

		seq += 1
	}

	close(sender.Send())

	for mp = range this.sendc {}
}

func (this *aggregatedConnection) reorder(receiver Receiver) {
	var unordered map[uint64][]byte = make(map[uint64][]byte)
	var m *aggregationMessage
	var msg Message
	var seq uint64
	var b []byte
	var ok bool

	for msg = range receiver.Recv(aggregationProtocol) {
		m, ok = msg.(*aggregationMessage)
		if ok == false {
			break
		}

		if m.seq < seq {
			continue
		}

		if m.seq > seq {
			unordered[m.seq] = m.content
			continue
		}

		if m.seq == seq {
			this.recvc <- m.content
			seq += 1
		}

		for {
			b, ok = unordered[seq]
			if ok == false {
				break
			}

			this.recvc <- b

			delete(unordered, seq)
			seq += 1
		}
	}

	close(this.recvc)

	unordered = nil

	for msg = range receiver.Recv(aggregationProtocol) {}	
}

func (this *aggregatedConnection) decode(c chan<- Message, p Protocol, n int) {
	var msg Message
	var err error
	var more bool
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

		c <- msg

		if n > 0 {
			n -= 1
		}
	}

	close(c)
}

func (this *aggregatedConnection) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *aggregatedConnection) Recv(proto Protocol) <-chan Message {
	var c chan Message = make(chan Message, 32)

	go this.decode(c, proto, -1)

	return c
}

func (this *aggregatedConnection) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go this.decode(c, proto, n)

	return c
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func newAggregatedTcpConnection(addr string, p Protocol, opts *AggregatedTcpConnectionOptions) *aggregatedConnection {
	var rc chan ReceiverProtocol = make(chan ReceiverProtocol)
	var sc chan Sender = make(chan Sender)
	var initiating sync.WaitGroup
	var req AggregationMessage
	var i int

	req.uid = rand.Uint64()
	req.total = uint16(opts.N)

	initiating.Add(opts.N)
	go func () {
		initiating.Wait()
		close(sc)
		close(rc)
	}()

	opts.Log.Trace("send %d requests for %x", opts.N,
		opts.Log.Emph(1, req.uid))

	for i = 0; i < opts.N; i++ {
		go func () {
			var conn Connection = NewTcpConnection(addr)

			conn.Send() <- MessageProtocol{ &req, p }

			sc <- conn
			rc <- ReceiverProtocol{ conn, aggregationProtocol }

			initiating.Done()
		}()
	}

	return newAggregatedConnection(sc, rc)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *AggregationMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.uid).WriteUint16(this.total).Error()
}

func (this *AggregationMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.uid).ReadUint16(&this.total).Error()
}


var aggregationProtocol Protocol = NewRawProtocol(&aggregationMessage{})


type aggregationMessage struct {
	seq uint64
	content []byte
}

func (this *aggregationMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.seq).WriteBytes32(this.content).Error()
}

func (this *aggregationMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.seq).ReadBytes32(&this.content).Error()
}
