package net


import (
	"context"
	"fmt"
	"go.uber.org/goleak"
	"testing"
	"time"
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
	go acceptConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), connc)
}

func serveRelay(ctx context.Context, addr string, res Resolver) {
	go _routeConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), NewRoutingService(res))
}

func serveTerminal(ctx context.Context, addr string, connc chan<- Connection) {
	var rs RoutingService

	rs = NewRoutingServiceWith(nil, &RoutingServiceOptions{
		Context: ctx,
	})

	go acceptConnections(rs, connc)

	go _routeConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), rs)
}


// ----------------------------------------------------------------------------


func TestRouteEmpty(t *testing.T) {
	var res Resolver
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	res = NewTcpResolver(mockProtocol)
	r = NewRoute([]string{}, res)

	_, more = <-r.Accept()
	if more {
		t.Fatalf("Accept")
	}

	close(r.Send())
}

func TestRouteShortInvalid(t *testing.T) {
	var res Resolver
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	res = NewTcpResolver(mockProtocol)
	r = NewRoute([]string{ "Hello World!" }, res)

	_, more = <-r.Accept()
	if more {
		t.Fatalf("Accept")
	}

	close(r.Send())
}

func TestRouteShortUnreachable(t *testing.T) {
	var cancel context.CancelFunc
	var ctx context.Context
	var res Resolver
	var c Connection
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res = NewTcpResolverWith(mockProtocol, &TcpResolverOptions{
		ConnectionContext: ctx,
	})
	r = NewRoute([]string{ findTcpAddr(t) }, res)

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
	var cancel context.CancelFunc
	var ctx context.Context
	var res Resolver
	var c Connection
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	res = NewTcpResolverWith(mockProtocol, &TcpResolverOptions{
		ConnectionContext: ctx,
	})
	r = NewRoute([]string{ findUnresolvableAddr(t) }, res)

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
	var cancel context.CancelFunc
	var ctx context.Context
	var res Resolver
	var port uint16
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	res = NewTcpResolver(mockProtocol)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	port = findTcpPort(t)
	serveRelay(ctx, fmt.Sprintf(":%d", port), res)

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
	var cancel context.CancelFunc
	var ctx context.Context
	var p0, p1 uint16
	var c Connection
	var res Resolver
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	res = NewTcpResolverWith(mockProtocol, &TcpResolverOptions{
		ConnectionContext: ctx,
	})

	p0 = findTcpPort(t)
	serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

	p1 = findTcpPort(t)
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
	var cancel context.CancelFunc
	var ctx context.Context
	var c Connection
	var res Resolver
	var port uint16
	var more bool
	var r Route

	defer goleak.VerifyNone(t)

	res = NewTcpResolver(mockProtocol)
	port = findTcpPort(t)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	serveRelay(ctx, fmt.Sprintf(":%d", port), res)

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

		ctx, cancel = context.WithCancel(context.Background())

		p0 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

		p1 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p1), connc)

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

		ctx, cancel = context.WithCancel(context.Background())

		p0 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

		p1 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p1), connc)

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
		var res Resolver=NewGroupResolver(NewTcpResolver(mockProtocol))
		var c0, c1 chan Connection
		var cancel context.CancelFunc
		var ctx context.Context
		var p0, p1, p2 uint16
		var r Route

		ctx, cancel = context.WithCancel(context.Background())

		c0 = make(chan Connection)
		c1 = make(chan Connection)

		p0 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

		p1 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p1), c0)

		p2 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p2), c1)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d+localhost:%d", p1, p2),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-c0, <-c1 },
			teardown: cancel,
		}
	})
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestRouteTree(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var res Resolver=NewGroupResolver(NewTcpResolver(mockProtocol))
		var p0, p1, p2, p3, p4 uint16
		var cancel context.CancelFunc
		var c0, c1 chan Connection
		var ctx context.Context
		var r Route

		ctx, cancel = context.WithCancel(context.Background())

		c0 = make(chan Connection)
		c1 = make(chan Connection)

		p0 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p0), res)

		p1 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p1), res)

		p2 = findTcpPort(t)
		serveRelay(ctx, fmt.Sprintf(":%d", p2), res)

		p3 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p3), c0)

		p4 = findTcpPort(t)
		serveTerminal(ctx, fmt.Sprintf(":%d", p4), c1)

		r = NewRoute([]string{
			fmt.Sprintf("localhost:%d", p0),
			fmt.Sprintf("localhost:%d+localhost:%d", p1, p2),
			fmt.Sprintf("localhost:%d+localhost:%d", p3, p4),
		}, res)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-c0, <-c0, <-c1, <-c1 },
			teardown: cancel,
		}
	})
}
