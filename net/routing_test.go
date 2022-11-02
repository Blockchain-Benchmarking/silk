package net


import (
	"context"
	"fmt"
	"testing"
)


// ----------------------------------------------------------------------------


func _routeConnections(a Accepter, rs RoutingService) {
	var m *RoutingMessage
	var c Connection
	var msg Message
	var ok bool

	for c = range a.Accept() {
		msg = <-c.RecvN(mockProtocol, 1)
		if msg == nil {
			close(c.Send())
			continue
		}

		m, ok = msg.(*RoutingMessage)
		if ok == false {
			close(c.Send())
			continue
		}

		go rs.Handle(m, c)
	}
}

func serveEndpoint(ctx context.Context, addr string, connc chan<- Connection) {
	acceptConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), connc)
}

func serveRelay(ctx context.Context, addr string, res Resolver) {
	_routeConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), NewRoutingService(res))
}

func serveTerminal(ctx context.Context, addr string, connc chan<- Connection) {
	var rs RoutingService = NewRoutingService(nil)

	go acceptConnections(rs, connc)

	_routeConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), rs)
}


// ----------------------------------------------------------------------------


func TestRouteEmpty(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var r Route = NewRoute([]string{}, res)
	var more bool

	_, more = <-r.Accept()
	if more {
		t.Fatalf("Accept")
	}

	close(r.Send())
}

func TestRouteShortInvalid(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var r Route = NewRoute([]string{ "Hello World!" }, res)
	var more bool

	_, more = <-r.Accept()
	if more {
		t.Fatalf("Accept")
	}

	close(r.Send())
}

func TestRouteShortUnreachable(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var r Route = NewRoute([]string{ findTcpAddr(t) }, res)
	var c Connection
	var more bool

	defer close(r.Send())

	c, more = <-r.Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	defer close(c.Send())

	_, more = <-c.Recv(mockProtocol)
	if more {
		t.Fatalf("Recv")
	}
}

func TestRouteShortUnresolvable(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var r Route = NewRoute([]string{ findUnresolvableAddr(t) }, res)
	var c Connection
	var more bool

	defer close(r.Send())

	c, more = <-r.Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	close(c.Send())
}

func TestRouteShort(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var connc chan Connection = make(chan Connection)
		var res Resolver = NewTcpResolver(mockProtocol)
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var r Route

		ctx, cancel = context.WithCancel(context.Background())
		go serveEndpoint(ctx, fmt.Sprintf(":%d", port), connc)

		r = NewRoute([]string{fmt.Sprintf("localhost:%d", port)}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <- connc },
			teardown: cancel,
		}
	})
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestRouteLongInvalid(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var port uint16 = findTcpPort(t)
	var cancel context.CancelFunc
	var ctx context.Context
	var more bool
	var r Route

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go serveRelay(ctx, fmt.Sprintf(":%d", port), res)

	r = NewRoute([]string{
		fmt.Sprintf("localhost:%d", port),
		"Hello World!",
	}, res)

	defer close(r.Send())

	_, more = <-r.Accept()
	if more {
		t.Fatalf("Accept")
	}
}

func TestRouteLongUnreachable(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var cancel context.CancelFunc
	var ctx context.Context
	var p0, p1 uint16
	var c Connection
	var more bool
	var r Route

	p0 = findTcpPort(t)
	p1 = findTcpPort(t)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

	r = NewRoute([]string{
		fmt.Sprintf("localhost:%d", p0),
		fmt.Sprintf("localhost:%d", p1),
	}, res)

	defer close(r.Send())

	c, more = <-r.Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	defer close(c.Send())

	_, more = <-c.Recv(mockProtocol)
	if more {
		t.Fatalf("Recv")
	}
}

func TestRouteLongUnresolvable(t *testing.T) {
	var res Resolver = NewTcpResolver(mockProtocol)
	var port uint16 = findTcpPort(t)
	var cancel context.CancelFunc
	var ctx context.Context
	var c Connection
	var more bool
	var r Route

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	go serveRelay(ctx, fmt.Sprintf(":%d", port), res)

	r = NewRoute([]string{
		fmt.Sprintf("localhost:%d", port),
		findUnresolvableAddr(t),
	}, res)

	defer close(r.Send())

	c, more = <-r.Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	close(c.Send())
}

func TestRouteLong(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var connc chan Connection = make(chan Connection)
		var res Resolver = NewTcpResolver(mockProtocol)
		var cancel context.CancelFunc
		var ctx context.Context
		var p0, p1 uint16
		var r Route

		p0 = findTcpPort(t)
		p1 = findTcpPort(t)

		ctx, cancel = context.WithCancel(context.Background())
		go serveRelay(ctx, fmt.Sprintf(":%d", p0), res)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p1), connc)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d", p1),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-connc },
			teardown: cancel,
		}
	})
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestRouteForkInvalid(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var res Resolver=NewGroupResolver(NewTcpResolver(mockProtocol))
		var connc chan Connection = make(chan Connection)
		var cancel context.CancelFunc
		var ctx context.Context
		var p0, p1 uint16
		var r Route

		p0 = findTcpPort(t)
		p1 = findTcpPort(t)

		ctx, cancel = context.WithCancel(context.Background())
		go serveRelay(ctx, fmt.Sprintf(":%d", p0), res)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p1), connc)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d+Hello World!", p1),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-connc },
			teardown: cancel,
		}
	})
}

func TestRouteFork(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var connc chan Connection = make(chan Connection)
		var res Resolver=NewGroupResolver(NewTcpResolver(mockProtocol))
		var cancel context.CancelFunc
		var ctx context.Context
		var p0, p1, p2 uint16
		var r Route

		p0 = findTcpPort(t)
		p1 = findTcpPort(t)
		p2 = findTcpPort(t)

		ctx, cancel = context.WithCancel(context.Background())
		go serveRelay(ctx, fmt.Sprintf(":%d", p0), res)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p1), connc)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p2), connc)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d+localhost:%d", p1, p2),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-connc, <-connc },
			teardown: cancel,
		}
	})
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestRouteTree(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var connc chan Connection = make(chan Connection)
		var res Resolver=NewGroupResolver(NewTcpResolver(mockProtocol))
		var p0, p1, p2, p3, p4 uint16
		var cancel context.CancelFunc
		var ctx context.Context
		var r Route

		p0 = findTcpPort(t)
		p1 = findTcpPort(t)
		p2 = findTcpPort(t)
		p3 = findTcpPort(t)
		p4 = findTcpPort(t)

		ctx, cancel = context.WithCancel(context.Background())
		go serveRelay(ctx, fmt.Sprintf(":%d", p0), res)
		go serveRelay(ctx, fmt.Sprintf(":%d", p1), res)
		go serveRelay(ctx, fmt.Sprintf(":%d", p2), res)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p3), connc)
		go serveTerminal(ctx, fmt.Sprintf(":%d", p4), connc)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d+localhost:%d", p1, p2),
			fmt.Sprintf("localhost:%d+localhost:%d", p3, p4),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{<-connc,<-connc,<-connc,<-connc},
			teardown: cancel,
		}
	})
}
