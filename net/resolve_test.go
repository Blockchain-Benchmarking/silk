package net


import (
	"context"
	"fmt"
	"math"
	"silk/util/test/goleak"
	"testing"
)


// ----------------------------------------------------------------------------


func testResolverInvalidName(t *testing.T, r Resolver, name string) {
	var e *ResolverInvalidNameError
	var err error
	var ok bool

	defer goleak.VerifyNone(t)

	_, _, err = r.Resolve(name)

	if err == nil {
		t.Errorf("Resolve")
		return
	}

	e, ok = err.(*ResolverInvalidNameError)
	if ok == false {
		t.Errorf("cast")
		return
	}

	if e.Name != name {
		t.Errorf("Name")
	}
}


// ----------------------------------------------------------------------------


func TestTcpResolverEmpty(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), "")
}

func TestTcpResolverRandom(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), "Hello World")
}

func TestTcpResolverColumn(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), ":")
}

func TestTcpResolverPortOnly(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol),
		fmt.Sprintf(":%d", findTcpPort(t)))
}

func TestTcpResolverInvalidPort(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), ":a12")
}

func TestTcpResolverPortTooBig(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol),
		fmt.Sprintf(":%d", math.MaxUint16 + 1))
}

func TestTcpResolverHostOnly(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), "localhost")
}

func TestTcpResolverHostColumnOnly(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol), "localhost:")
}

func TestTcpResolverHostInvalidPort(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol),
		"localhost:a12")
}

func TestTcpResolverHostPortTooBig(t *testing.T) {
	testResolverInvalidName(t, NewTcpResolver(mockProtocol),
		fmt.Sprintf("localhost:%d", math.MaxUint16 + 1))
}

func TestTcpResolverLocalhost(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var r Resolver = NewTcpResolver(mockProtocol)
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var ps []Protocol
		var c Connection
		var s Accepter
		var rs []Route
		var more bool
		var err error

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf(":%d", port),
			&TcpServerOptions{ Context: ctx })

		rs, ps, err = r.Resolve(fmt.Sprintf("localhost:%d", port))

		if err != nil {
			t.Fatalf("Resolve")
		}

		if (ps == nil) || (len(ps) != 1) || (ps[0] != mockProtocol) {
			t.Fatalf("[]Protocol")
		}

		if (rs == nil) || (len(rs) != 1) {
			t.Fatalf("[]Route")
		}

		c, more = <-s.Accept()
		if more == false {
			t.Fatalf("Accept")
		}

		return &routeTestSetup{
			route: rs[0],
			conns: []Connection{ c },
			teardown: cancel,
		}
	})
}

func TestTcpResolverUnreachable(t *testing.T) {
	var ps []Protocol
	var c Connection
	var rs []Route
	var r Resolver
	var more bool
	var err error

	defer goleak.VerifyNone(t)

	r = NewTcpResolver(mockProtocol)

	rs, ps, err = r.Resolve(findTcpAddr(t))

	if err != nil {
		t.Fatalf("Resolve")
	}

	if (ps == nil) || (len(ps) != 1) || (ps[0] != mockProtocol) {
		t.Fatalf("[]Protocol")
	}

	if (rs == nil) || (len(rs) != 1) {
		t.Fatalf("[]Route")
	}

	defer close(rs[0].Send())

	c, more = <-rs[0].Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	defer close(c.Send())

	_, more = <-c.Recv(mockProtocol)
	if more {
		t.Fatalf("Recv")
	}
}

func TestTcpResolverUnresolvableAddr(t *testing.T) {
	var cancel context.CancelFunc
	var ctx context.Context
	var ps []Protocol
	var c Connection
	var rs []Route
	var r Resolver
	var more bool
	var err error

	defer goleak.VerifyNone(t)

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()

	r = NewTcpResolverWith(mockProtocol, &TcpResolverOptions{
		ConnectionContext: ctx,
	})

	rs, ps, err = r.Resolve(findUnresolvableAddr(t))

	if err != nil {
		t.Fatalf("Resolve")
	}

	if (ps == nil) || (len(ps) != 1) || (ps[0] != mockProtocol) {
		t.Fatalf("[]Protocol")
	}

	if (rs == nil) || (len(rs) != 1) {
		t.Fatalf("[]Route")
	}

	defer close(rs[0].Send())

	c, more = <-rs[0].Accept()
	if more == false {
		t.Fatalf("Accept")
	}

	close(c.Send())
}
