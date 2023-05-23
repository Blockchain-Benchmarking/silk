package net


// ----------------------------------------------------------------------------


// A one-to-many communication link.
//
type Route interface {
	// Accept remote `Connection`s when they are available.
	// A given `Route` can `Accept` only a limited number of `Connection`
	// before to return `nil` forever.
	// Returning `nil` does not necessary mean that an error occured.
	//
	Accepter

	// Broadcast a `Message` encoded with the given `Protocol` to all
	// remote peers. This is not limited to the accepted peers.
	//
	// There is sequential consistency between calls to `Send()` but not
	// between `Route.Send()` and `Connection.Send()`.
	// Specifically, the following situation can occur:
	//
	//     var r Route = ...
	//     var a = <-r.Accept()
	//     var b = <-r.Accept()
	//
	//     go func () { a.Send() <- mp0 ; b.Send() <- mp1 }()
	//     go func () { r.Send() <- mp }()
	//
	// The remote end of `a` can receive `mp` then `mp0` while the remote
	// end of `b` receives `mp1` then `mp`.
	//
	Sender
}


func NewLeafRoute(connc <-chan Connection) Route {
	return newLeafRoute(connc)
}

func NewSliceLeafRoute(conns []Connection) Route {
	var connc chan Connection = make(chan Connection, len(conns))
	var conn Connection

	for _, conn = range conns {
		connc <- conn
	}

	close(connc)

	return NewLeafRoute(connc)
}

func NewUnitLeafRoute(conn Connection) Route {
	return NewSliceLeafRoute([]Connection{ conn })
}


func NewCompositeRoute(routec <-chan Route) Route {
	return newCompositeRoute(routec)
}

func NewSliceCompositeRoute(routes []Route) Route {
	var routec chan Route = make(chan Route, len(routes))
	var route Route

	for _, route = range routes {
		routec <- route
	}

	close(routec)

	return NewCompositeRoute(routec)
}


// ----------------------------------------------------------------------------


type leafRoute struct {
	acceptc chan Connection
	bcast Sender
}

func newLeafRoute(connc <-chan Connection) *leafRoute {
	var this leafRoute
	var senderc chan Sender

	senderc = make(chan Sender)
	this.acceptc = make(chan Connection)
	this.bcast = NewBroadcaster(senderc)

	go this.run(senderc, connc)

	return &this
}

func (this *leafRoute) run(senderc chan<- Sender, connc <-chan Connection) {
	var conns []Connection = make([]Connection, 0)
	var bsd, csd SenderDup
	var conn Connection

	for conn = range connc {
		bsd = NewSendScatter(conn)
		csd = bsd.Dup()
		senderc <- bsd
		conns = append(conns, NewPiecewiseConnection(csd, conn))
	}

	close(senderc)

	for _, conn = range conns {
		this.acceptc <- conn
	}

	close(this.acceptc)
}

func (this *leafRoute) Accept() <-chan Connection {
	return this.acceptc
}

func (this *leafRoute) Send() chan<- MessageProtocol {
	return this.bcast.Send()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type compositeRoute struct {
	bcast Sender
	cctor Accepter
}

func newCompositeRoute(routec <-chan Route) *compositeRoute {
	var this compositeRoute
	var senderc chan Sender
	var accepterc chan Accepter

	senderc = make(chan Sender)
	this.bcast = NewBroadcaster(senderc)

	accepterc = make(chan Accepter)
	this.cctor = NewAcceptGather(accepterc)

	go this.run(senderc, accepterc, routec)

	return &this
}

func (this *compositeRoute) run(senderc chan<- Sender, accepterc chan<- Accepter, routec <-chan Route) {
	var route Route

	for route = range routec {
		senderc <- route
		accepterc <- route
	}

	close(senderc)
	close(accepterc)
}

func (this *compositeRoute) Accept() <-chan Connection {
	return this.cctor.Accept()
}

func (this *compositeRoute) Send() chan<- MessageProtocol {
	return this.bcast.Send()
}
