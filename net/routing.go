package net


import (
	"bytes"
	"fmt"
	"math"
	sio "silk/io"
	"silk/util/atomic"
	"strings"
	"sync"
)


// ----------------------------------------------------------------------------


// A service to relay `Route` messages.
//
type RoutingService interface {
	Accepter

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
		o.Log = sio.NewNopLogger()
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
func NewRoute(names []string, resolver Resolver) Route {
	return NewRouteWith(names, resolver, nil)
}

func NewRouteWith(names []string, r Resolver, o *RouteOptions) Route {
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
	acceptc chan Connection
	nextId atomic.Uint64
}

func newRoutingService(r Resolver, o *RoutingServiceOptions) *routingService {
	var this routingService

	this.log = o.Log
	this.resolver = r
	this.acceptc = make(chan Connection)
	this.nextId.Store(0)

	return &this
}

func (this *routingService) Handle(msg *RoutingMessage, conn Connection) {
	var ep *endpointConnection
	var protos []Protocol
	var routes []Route
	var log sio.Logger
	var route Route
	var err error

	if len(msg.names) == 0 {
		log = this.log.WithLocalContext("endpoint[%d]",
			this.nextId.Add(1) - 1)
		ep = newEndpointConnection(conn, log)

		this.acceptc <- ep

		log.Debug("open")
		ep.encode()
		return
	}

	log = this.log.WithLocalContext("relay[%d]", this.nextId.Add(1) - 1)

	var nexts [][]string
	var parts []string
	var rs []Route
	var i int

	parts, nexts = resolveRoutingExpr(msg.names[0])
	for i = range parts {
		log.Debug("connect: %s", log.Emph(0, parts[i]))
		routes, protos, err = this.resolver.Resolve(parts[i])
		if err != nil {
			log.Warn("%s", err.Error())
			continue
		}

		log.Debug("request: %v", log.Emph(0,
			append(nexts[i], msg.names[1:]...)))
		route = newRequestRoute(append(nexts[i], msg.names[1:]...),
			routes, protos)

		if len(append(nexts[i], msg.names[1:]...)) > 0 {
			route = newDispatchRoute(route, log)
		}

		rs = append(rs, route)
	}

	if len(rs) == 0 {
		close(conn.Send())
		return
	}

	route = NewSliceCompositeRoute(rs)

	newRelay(conn, route, log).run()

	// log.Debug("connect: %s", log.Emph(0, msg.names[0]))
	// routes, protos, err = this.resolver.Resolve(msg.names[0])
	// if err != nil {
	// 	log.Warn("%s", err.Error())
	// 	close(conn.Send())
	// 	return
	// }

	// log.Debug("request: %v", log.Emph(0, msg.names[1:]))
	// route = newRequestRoute(msg.names[1:], routes, protos)

	// if len(msg.names) > 1 {
	// 	route = newDispatchRoute(route, log)
	// }

	// newRelay(conn, route, log).run()
}

func (this *routingService) Accept() <-chan Connection {
	return this.acceptc
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func newRoute(names []string, r Resolver, opts *RouteOptions) Route {
	var protos []Protocol
	var routes []Route
	var route Route
	var err error

	if len(names) == 0 {
		return NewSliceLeafRoute([]Connection{})
	}

	var nexts [][]string
	var parts []string
	var rs []Route
	var i int

	parts, nexts = resolveRoutingExpr(names[0])
	for i = range parts {
		opts.Log.Debug("connect: %s", opts.Log.Emph(0, parts[i]))
		routes, protos, err = r.Resolve(parts[i])
		if err != nil {
			continue
		}

		if (len(nexts[i]) + len(names[1:])) == 0 {
			if len(routes) == 1 {
				rs = append(rs, routes[0])
			} else {
				rs = append(rs, NewSliceCompositeRoute(routes))
			}

			continue
		}

		opts.Log.Debug("request: %v", opts.Log.Emph(0,
			append(nexts[i], names[1:]...)))
		route = newRequestRoute(append(nexts[i], names[1:]...),
			routes, protos)
		route = newDispatchRoute(route, opts.Log)
		rs = append(rs, newEndpointRoute(route,
			opts.Log.WithLocalContext("()")))
	}

	return NewSliceCompositeRoute(rs)
		
	// opts.Log.Debug("connect: %s", opts.Log.Emph(0, names[0]))
	// routes, protos, err = r.Resolve(names[0])
	// if err != nil {
	// 	return NewSliceLeafRoute([]Connection{})
	// }

	// if len(names) == 1 {
	// 	if len(routes) == 1 {
	// 		return routes[0]
	// 	} else {
	// 		return NewSliceCompositeRoute(routes)
	// 	}
	// }

	// opts.Log.Debug("request: %v", opts.Log.Emph(0, names[1:]))
	// route = newRequestRoute(names[1:], routes, protos)
	// route = newDispatchRoute(route, opts.Log)
	// return newEndpointRoute(route, opts.Log.WithLocalContext("()"))
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


// This is crappy as fuck but only temporary.
//
func resolveRoutingExpr(expr string) ([]string, [][]string) {
	var start, i, index, lvl int
	var nexts [][]string
	var parts []string
	var part string
	var c rune

	if strings.HasPrefix(expr, "(") {
		expr = expr[1:]

		lvl = 0
		start = 0

		for index, c = range expr {
			if (lvl == 0) && ((c == '|') || (c == ')')) {
				parts = append(parts, expr[start:index])
				start = index + 1
			}

			if c == '(' {
				lvl += 1
			} else if c == ')' {
				lvl -= 1
			}
		}
	} else {
		parts = []string{ expr }
	}

	for i, part = range parts {
		index = strings.Index(part, ",")
		if index == -1 {
			nexts = append(nexts, []string{})
		} else {
			nexts = append(nexts, []string{ part[index+1:] })
			parts[i] = part[:index]
		}
	}

	return parts, nexts
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func newRequestRoute(names []string, routes []Route, protos []Protocol) Route {
	var requesting sync.WaitGroup
	var routec chan Route
	var i int

	routec = make(chan Route, len(routes))

	requesting.Add(len(routes))
	go func () {
		requesting.Wait()
		close(routec)
	}()

	for i = range routes {
		go func (index int) {
			routes[index].Send() <- MessageProtocol{
				M: &RoutingMessage{ names },
				P: protos[index],
			}

			routec <- routes[index]

			requesting.Done()
		}(i)
	}

	return NewCompositeRoute(routec)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type relay struct {
	log sio.Logger
	origin Connection
	remote Route
	lock sync.Mutex
	conns []Connection
}

func newRelay(origin Connection, remote Route, log sio.Logger) *relay {
	var this relay

	this.log = log
	this.origin = origin
	this.remote = remote
	this.conns = make([]Connection, 0)

	return &this
}

func (this *relay) run() {
	go this.accept()
	this.forward()
}

func (this *relay) accept() {
	var transferring sync.WaitGroup
	var conn Connection
	var id uint16

	for conn = range this.remote.Accept() {
		this.lock.Lock()
		id = uint16(len(this.conns))
		this.conns = append(this.conns, conn)
		this.lock.Unlock()

		this.log.Trace("relay back accept as %d", this.log.Emph(1, id))
		this.origin.Send() <- MessageProtocol{
			M: &routingAccept{ id },
			P: routingProtocol,
		}

		transferring.Add(1)
		go this.backward(conn, id, &transferring)
	}

	this.log.Trace("relay back end of accept")
	this.origin.Send() <- MessageProtocol{
		M: &routingAccepted{},
		P: routingProtocol,
	}

	transferring.Wait()

	close(this.origin.Send())
}

func (this *relay) backward(conn Connection, id uint16, txwg *sync.WaitGroup) {
	var msg Message

	for msg = range conn.Recv(routingProtocol) {
		switch m := msg.(type) {
		case *routingUnicast:
			this.log.Trace("relay back %d bytes of unicast as %d",
				len(m.content), this.log.Emph(1, id))
			this.origin.Send() <- MessageProtocol{
				M: &routingIdUnicast{ id, m.content },
				P: routingProtocol,
			}
		default:
			this.log.Warn("unexpected back message: %T:%v",
				this.log.Emph(2, msg), msg)
		}
	}

	this.log.Trace("relay back close notice as %d", this.log.Emph(1, id))
	this.origin.Send() <- MessageProtocol{
		M: &routingIdClose{ id },
		P: routingProtocol,
	}

	txwg.Done()
}

func (this *relay) forward() {
	var conn Connection
	var closed bool
	var msg Message

	closed = false

	for msg = range this.origin.Recv(routingProtocol) {
		switch m := msg.(type) {
		case *routingIdUnicast:
			this.lock.Lock()
			conn = nil
			if int(m.id) < len(this.conns) {
				conn = this.conns[m.id]
			}
			this.lock.Unlock()

			if conn == nil {
				this.log.Warn("unknown id %d for unicast forw",
					this.log.Emph(1, m.id))
				continue
			}

			this.log.Trace("relay forw %d bytes of unicast for %d",
				len(m.content), this.log.Emph(1, m.id))
			conn.Send() <- MessageProtocol{
				M: &routingUnicast{ m.content },
				P: routingProtocol,
			}

		case *routingBroadcast:
			if closed {
				this.log.Warn("broadcast on closed route")
				continue
			}

			this.log.Trace("relay forw %d bytes of broadcast",
				len(m.content))
			this.remote.Send() <- MessageProtocol{
				M: m,
				P: routingProtocol,
			}

		case *routingIdClose:
			this.lock.Lock()
			conn = nil
			if int(m.id) < len(this.conns) {
				conn = this.conns[m.id]
				this.conns[m.id] = nil
			}
			this.lock.Unlock()

			if conn == nil {
				this.log.Warn("unknown id %d for close notice",
					this.log.Emph(1, m.id))
				continue
			}

			if conn != nil {
				this.log.Trace("close for %d",
					this.log.Emph(1, m.id))
				close(conn.Send())
			}

		default:
			this.log.Warn("unexpected forw message: %T:%v",
				this.log.Emph(2, msg), msg)
		}
	}

	close(this.remote.Send())
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type dispatchConnection struct {
	log sio.Logger
	inner Connection
	id uint16
	sendc chan MessageProtocol
	handlec chan []byte
}

func newDispatchConnection(inner Connection, id uint16, using *sync.WaitGroup, log sio.Logger) *dispatchConnection {
	var this dispatchConnection

	this.log = log
	this.inner = inner
	this.id = id
	this.sendc = make(chan MessageProtocol)
	this.handlec = make(chan []byte, 16)

	go this.run(using)

	return &this
}

func (this *dispatchConnection) run(using *sync.WaitGroup) {
	var mp MessageProtocol

	for mp = range this.sendc {
		if mp.P != routingProtocol {
			panic("wrong protocol")
		}

		switch m := mp.M.(type) {
		case *routingUnicast:
			this.log.Trace("unicast %d bytes", len(m.content))
			this.inner.Send() <- MessageProtocol{
				M: &routingIdUnicast{ this.id, m.content },
				P: routingProtocol,
			}
		default:
			panic("wrong message type")
		}
	}

	this.log.Trace("send close notice")
	this.inner.Send() <- MessageProtocol{
		M: &routingIdClose{ this.id },
		P: routingProtocol,
	}

	using.Done()
}

func (this *dispatchConnection) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *dispatchConnection) decode(dest chan<- Message, n int) {
	var more bool
	var b []byte

	for {
		if n == 0 {
			break
		}

		b, more = <-this.handlec
		if more == false {
			break
		}

		this.log.Trace("receive %d bytes of unicast", len(b))
		dest <- &routingUnicast{ b }

		n -= 1
	}

	close(dest)
}

func (this *dispatchConnection) handle() chan<- []byte {
	return this.handlec
}

func (this *dispatchConnection) Recv(proto Protocol) <-chan Message {
	var c chan Message = make(chan Message)

	if proto != routingProtocol {
		panic("wrong protocol")
	}

	go this.decode(c, -1)

	return c
}

func (this *dispatchConnection) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if proto != routingProtocol {
		panic("wrong protocol")
	} else if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go this.decode(c, n)

	return c
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type dispatchRoute struct {
	log sio.Logger
	inner Route
	acceptc chan Connection
}

func newDispatchRoute(inner Route, log sio.Logger) *dispatchRoute {
	var this dispatchRoute

	this.log = log
	this.inner = inner
	this.acceptc = make(chan Connection)

	go this.accept()

	return &this
}

func (this *dispatchRoute) accept() {
	var accepting sync.WaitGroup
	var stream Connection
	var id int

	for stream = range this.inner.Accept() {
		accepting.Add(1)
		go this.dispatch(stream, id, &accepting)
	}

	accepting.Wait()
	close(this.acceptc)
}

func (this *dispatchRoute) dispatch(stream Connection, id int, accepting *sync.WaitGroup) {
	var conns map[uint16]*dispatchConnection
	var conn *dispatchConnection
	var slog, log sio.Logger
	var using sync.WaitGroup
	var accepted bool
	var msg Message

	slog = this.log.WithLocalContext("conn[%d:?]", id)
	conns = make(map[uint16]*dispatchConnection)
	accepted = false

	using.Add(1)
	go func () {
		using.Wait()
		close(stream.Send())
	}()

	for msg = range stream.Recv(routingProtocol) {
		switch m := msg.(type) {

		case *routingAccept:
			if conns[m.id] != nil {
				this.log.Warn("duplicate accept of %d",
					this.log.Emph(1, m.id))
				continue
			}

			log = this.log.WithLocalContext("conn[%d:%d]", id,m.id)
			log.Debug("open")
			using.Add(1)
			conns[m.id] = newDispatchConnection(stream, m.id,
				&using, log)
			this.acceptc <- conns[m.id]

		case *routingAccepted:
			if accepted {
				slog.Warn("duplicate end of accept")
				continue
			}

			accepting.Done()
			using.Done()
			accepted = true

		case *routingIdUnicast:
			conn = conns[m.id]
			if conn == nil {
				this.log.Warn("unknown id %d for unicast",
					this.log.Emph(1, m.id))
				continue
			}

			conn.handle() <- m.content

		case *routingIdClose:
			conn = conns[m.id]
			if conn == nil {
				this.log.Warn("unknown id %d for close notice",
					this.log.Emph(1, m.id))
				continue
			}

			close(conn.handle())
			delete(conns, m.id)

		default:
			slog.Warn("unexpected message: %T:%v",
				slog.Emph(2, msg), msg)
		}
	}

	if accepted == false {
		slog.Warn("closed unexpectedly")
		accepting.Done()
		using.Done()
	}
}

func (this *dispatchRoute) Accept() <-chan Connection {
	return this.acceptc
}

func (this *dispatchRoute) Send() chan<- MessageProtocol {
	return this.inner.Send()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type endpointConnection struct {
	log sio.Logger
	sendc chan MessageProtocol
	inner Connection
}

func newEndpointConnection(inner Connection,log sio.Logger)*endpointConnection{
	var this endpointConnection

	this.log = log
	this.sendc = make(chan MessageProtocol)
	this.inner = inner

	return &this
}

func (this *endpointConnection) encode() {
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
			this.log.Warn("encoding %T: %s",
				this.log.Emph(2, mp.P), err.Error())

			this.log.Debug("close")
			close(this.inner.Send())

			closed = true
			continue
		}

		this.log.Trace("unicast %d bytes", buf.Len())
		this.inner.Send() <- MessageProtocol{
			M: &routingUnicast{ buf.Bytes() },
			P: routingProtocol,
		}
	}

	if closed == false {
		this.log.Debug("close")
		close(this.inner.Send())
	}
}

func (this *endpointConnection) decode(c chan<- Message, p Protocol, n int) {
	var innerc <-chan Message
	var umsg, msg Message
	var err error
	var b []byte

	if n < 0 {
		innerc = this.inner.Recv(routingProtocol)
	} else {
		innerc = this.inner.RecvN(routingProtocol, n)
	}

	for msg = range innerc {
		switch m := msg.(type) {
		case *routingUnicast:
			b = m.content
			this.log.Trace("receive %d bytes of unicast", len(b))
		case *routingBroadcast:
			b = m.content
			this.log.Trace("receive %d bytes of broadcast", len(b))
		default:
			this.log.Warn("unexpected message: %T:%v", msg, msg)
		}

		umsg, err = p.Decode(sio.NewReaderSource(bytes.NewBuffer(b)))
		if err != nil {
			this.log.Warn("%s", err.Error())
			break
		}

		c <- umsg
	}

	close(c)

	for msg = range innerc {}
}

func (this *endpointConnection) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *endpointConnection) Recv(proto Protocol) <-chan Message {
	var c chan Message = make(chan Message)

	go this.decode(c, proto, -1)

	return c
}

func (this *endpointConnection) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go this.decode(c, proto, n)

	return c
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type endpointRoute struct {
	log sio.Logger
	sendc chan MessageProtocol
	acceptc chan Connection
}

func newEndpointRoute(inner Route, log sio.Logger) *endpointRoute {
	var this endpointRoute

	this.log = log
	this.sendc = make(chan MessageProtocol)
	this.acceptc = make(chan Connection)

	go this.wrapAccepted(inner)
	go this.encodeSent(inner)

	return &this
}

func (this *endpointRoute) wrapAccepted(inner Route) {
	var endpoint *endpointConnection
	var conn Connection
	var log sio.Logger
	var id int

	id = 0

	for conn = range inner.Accept() {
		log = this.log.WithLocalContext("conn[%d]", id)

		log.Debug("open")
		endpoint = newEndpointConnection(conn, log)

		go endpoint.encode()
		this.acceptc <- endpoint

		id += 1
	}

	close(this.acceptc)
}

func (this *endpointRoute) Accept() <-chan Connection {
	return this.acceptc
}

func (this *endpointRoute) encodeSent(inner Route) {
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
			this.log.Warn("encoding %T: %s",
				this.log.Emph(2, mp.P), err.Error())

			this.log.Debug("close")
			close(inner.Send())

			closed = true
			continue
		}

		this.log.Trace("broadcast %d bytes", buf.Len())
		inner.Send() <- MessageProtocol{
			M: &routingBroadcast{ buf.Bytes() },
			P: routingProtocol,
		}
	}

	if closed == false {
		this.log.Debug("close")
		close(inner.Send())
	}
}

func (this *endpointRoute) Send() chan<- MessageProtocol {
	return this.sendc
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
	0: &routingAccept{},      // accept multiplexed connection
	1: &routingAccepted{},    // end of accept for a stream of multiplexed
	2: &routingUnicast{},     // unicast from/to a connection
	3: &routingIdUnicast{},   // unicast from/to a multiplexed connection
	4: &routingBroadcast{},   // broadcast to all routes
	5: &routingIdClose{},     // close notice of a multiplexed connection
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


type routingIdClose struct {
	id uint16
}

func (this *routingIdClose) Encode(sink sio.Sink) error {
	return sink.WriteUint16(this.id).Error()
}

func (this *routingIdClose) Decode(source sio.Source) error {
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
