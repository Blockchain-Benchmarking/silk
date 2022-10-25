package net


import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


var mockResolveProtocol Protocol = NewRawProtocol(nil)


// ----------------------------------------------------------------------------


func TestTcpResolveEmpty(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	routes, protos, err = resolver.Resolve("")
	if (routes != nil) || (protos != nil) || (err == nil) {
		t.Errorf("resolve should fail")
	}

	if _, ok := err.(*ResolverInvalidNameError); !ok {
		t.Errorf("resolve wrong error type: %T", err)
	}
}

func TestTcpResolveRandomString(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	routes, protos, err = resolver.Resolve("Hello World!")
	if (routes != nil) || (protos != nil) || (err == nil) {
		t.Errorf("resolve should fail")
	}

	if _, ok := err.(*ResolverInvalidNameError); !ok {
		t.Errorf("resolve wrong error type: %T", err)
	}
}

func TestTcpResolveIpv4(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	routes, protos, err = resolver.Resolve("127.0.0.1")
	if (routes != nil) || (protos != nil) || (err == nil) {
		t.Errorf("resolve should fail")
	}

	if _, ok := err.(*ResolverInvalidNameError); !ok {
		t.Errorf("resolve wrong error type: %T", err)
	}
}

func TestTcpResolveIpv4PortUnreachable(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var conn Connection
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", findTcpPort(t))

	routes, protos, err = resolver.Resolve(addr)
	if (routes == nil) || (protos == nil) || (err != nil) {
		t.Errorf("resolve: %v", err)
	}

	if len(routes) != 1 {
		t.Errorf("resolve in too many routes: %v", routes)
	}

	if len(protos) != 1 {
		t.Fatalf("resolve in too many protocols: %v", protos)
	}

	if protos[0] != mockResolveProtocol {
		t.Errorf("resolve wrong protocol: %v", protos[0])
	}

	conn, err = routes[0].Accept()
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail")
	}

	err = routes[0].Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTcpResolveHostname(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	routes, protos, err = resolver.Resolve("localhost")
	if (routes != nil) || (protos != nil) || (err == nil) {
		t.Errorf("resolve should fail")
	}

	if _, ok := err.(*ResolverInvalidNameError); !ok {
		t.Errorf("resolve wrong error type: %T", err)
	}
}

func TestTcpResolveHostnamePort(t *testing.T) {
	var resolver Resolver
	var protos []Protocol
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	addr := fmt.Sprintf("localhost:%d", findTcpPort(t))

	routes, protos, err = resolver.Resolve(addr)
	if (routes == nil) || (protos == nil) || (err != nil) {
		t.Errorf("resolve: %v", err)
	}

	if len(routes) != 1 {
		t.Errorf("resolve in too many routes: %v", routes)
	}

	if len(protos) != 1 {
		t.Fatalf("resolve in too many protocols: %v", protos)
	}

	if protos[0] != mockResolveProtocol {
		t.Errorf("resolve wrong protocol: %v", protos[0])
	}
}

func TestTcpResolveHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var resolver Resolver
	var protos []Protocol
	var flag atomic.Bool
	var routes []Route
	var err error

	resolver = NewTcpResolver(mockResolveProtocol)
	if resolver == nil {
		t.Fatalf("new: %v", err)
	}

	addr := "1.1.1.1:1"

	routes, protos, err = resolver.Resolve(addr)
	if (routes == nil) || (protos == nil) || (err != nil) {
		t.Errorf("resolve: %v", err)
	}

	if len(routes) != 1 {
		t.Errorf("resolve in too many routes: %v", routes)
	}

	if len(protos) != 1 {
		t.Fatalf("resolve in too many protocols: %v", protos)
	}

	if protos[0] != mockResolveProtocol {
		t.Errorf("resolve wrong protocol: %v", protos[0])
	}

	errc := make(chan error)
	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		errc <- routes[0].Close()
	})

	conn, err := routes[0].Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail: %v %v", conn, err)
	}

	t0.Stop()

	err = <-errc
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}
