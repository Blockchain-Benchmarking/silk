package net


import (
	"context"
	"io"
	sio "silk/io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


var mockRoutingProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &RoutingMessage{},
	1: &mockRoutingMessage{},
})

var mockRoutingResolver Resolver = newMockRoutingExprResolver()


type mockRoutingExprResolver struct {
	inner Resolver
}

func newMockRoutingExprResolver() *mockRoutingExprResolver {
	return &mockRoutingExprResolver{
		inner: NewTcpResolver(mockRoutingProtocol),
	}
}

func (this *mockRoutingExprResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var protos, protoRet []Protocol
	var routes, routeRet []Route
	var comp string
	var err error

	for _, comp = range strings.Split(name, "+") {
		routes, protos, err = this.inner.Resolve(comp)
		if err != nil {
			return nil, nil, err
		}

		routeRet = append(routeRet, routes...)
		protoRet = append(protoRet, protos...)
	}

	return routeRet, protoRet, nil
}


type mockRoutingMessage struct {
	value uint64
}

func (this *mockRoutingMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.value).Error()
}

func (this *mockRoutingMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.value).Error()
}


func runRoutingServer(ctx context.Context, server Server) {
	var service RoutingService = NewRoutingService(mockRoutingResolver)
	var schan ServerMessageChannel

	schan = NewServerMessageChannel(server, mockRoutingProtocol)

	loop: for {
		select {
		case cmsg := <-schan.Accept():
			go service.Handle(cmsg.Message().(*RoutingMessage),
				cmsg.Connection())
		case <-ctx.Done():
			break loop
		}
	}

	schan.Close()
	service.Close()
}

func runRoutingAddr(ctx context.Context, addr string) error {
	var server Server
	var err error

	server, err = NewTcpServer(addr)
	if err != nil {
		return err
	}

	go runRoutingServer(ctx, server)

	return nil
}


func runServer(ctx context.Context, serv Server, f func (Message,Connection)) {
	var schan ServerMessageChannel

	schan = NewServerMessageChannel(serv, mockRoutingProtocol)

	loop: for {
		select {
		case cmsg := <-schan.Accept():
			go f (cmsg.Message(), cmsg.Connection())
		case <-ctx.Done():
			break loop
		}
	}

	schan.Close()
}

func runAddr(c context.Context, a string, f func (Message, Connection)) error {
	var server Server
	var err error

	server, err = NewTcpServer(a)
	if err != nil {
		return err
	}

	go runServer(c, server, f)

	return nil
}

func runCloserAddr(ctx context.Context, addr string) error {
	return runAddr(ctx, addr, func (msg Message, conn Connection) {
		conn.Close()
	})
}

func runEchoAddr(ctx context.Context, addr string) error {
	return runAddr(ctx, addr, func (msg Message, conn Connection) {
		conn.Send(msg, mockRoutingProtocol)
		conn.Close()
	})
}


// ----------------------------------------------------------------------------


func TestRoutingServiceClose(t *testing.T) {
	var service RoutingService = NewRoutingService(mockRoutingResolver)
	var err error

	err = service.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRoutingServiceHandle(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var handleSignal sync.WaitGroup
	var service RoutingService
	var flag atomic.Bool
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	service = NewRoutingService(mockRoutingResolver)

	handleSignal.Add(1)
	err = runAddr(ctx, addr0, func (msg Message, conn Connection) {
		handleSignal.Wait()
		service.Handle(msg.(*RoutingMessage), conn)
	})
	if err != nil { 
		t.Fatalf("run relay: %v", err)
	}

	err = runEchoAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("run endpoint: %v", err)
	}

	route, err := NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}
	defer route.Close()

	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		handleSignal.Done()
	})

	conn, err := route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	t1.Stop()

	err = service.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRoutingServiceHandleClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var handledSignal sync.WaitGroup
	var service RoutingService
	var flag atomic.Bool
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	service = NewRoutingService(mockRoutingResolver)

	handledSignal.Add(1)
	err = runAddr(ctx, addr0, func (msg Message, conn Connection) {
		service.Handle(msg.(*RoutingMessage), conn)
		handledSignal.Done()
	})
	if err != nil { 
		t.Fatalf("run relay: %v", err)
	}

	server, err := NewTcpServer(addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	route, err := NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}
	defer route.Close()

	errc := make(chan error)
	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		errc <- service.Close()
	})

	t.Fatalf("hanging")
	conn, err := route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail")
	}

	t1.Stop()

	err = <-errc
	if err != nil {
		t.Errorf("close: %v", err)
	}

	handledSignal.Wait()
}


func TestRoutingServiceHandleReturnAfterRouteClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var service RoutingService
	var flag atomic.Bool
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service = NewRoutingService(mockRoutingResolver)
	server, err := NewTcpServer(addr0)
	if (server == nil) || (err != nil) {
		t.Fatalf("new relay server: %v", err)
	}
	defer server.Close()

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	go func () {
		route, _ := NewRoute([]string{ addr0, addr1 },
			mockRoutingResolver)

		time.Sleep(timeout)
		flag.Store(true)

		route.Close()
	}()

	t.Fatalf("hanging")
	conn, err := server.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	msg, err := conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Fatalf("recv: %v", err)
	}

	service.Handle(msg.(*RoutingMessage), conn)
	if flag.Load() == false {
		t.Errorf("handle should hang")
	}

	err = service.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRoutingServiceHandleReturnAfterClientClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var service RoutingService
	var flag atomic.Bool
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	service = NewRoutingService(mockRoutingResolver)
	server, err := NewTcpServer(addr0)
	if (server == nil) || (err != nil) {
		t.Fatalf("new relay server: %v", err)
	}
	defer server.Close()

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	t.Fatalf("panic")
	go func () {
		route, _ := NewRoute([]string{ addr0, addr1 },
			mockRoutingResolver)

		client, _ := route.Accept()
		route.Close()

		time.Sleep(timeout)
		flag.Store(true)

		client.Close()
	}()

	conn, err := server.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	msg, err := conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Fatalf("recv: %v", err)
	}

	service.Handle(msg.(*RoutingMessage), conn)
	if flag.Load() == false {
		t.Errorf("handle should hang")
	}

	err = service.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRoutingServiceHandleReturnAfterServerClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var doneSignal sync.WaitGroup
	var service RoutingService
	var flag atomic.Bool
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)

	service = NewRoutingService(mockRoutingResolver)
	server, err := NewTcpServer(addr0)
	if (server == nil) || (err != nil) {
		t.Fatalf("new relay server: %v", err)
	}
	defer server.Close()

	closingServer, err := NewTcpServer(addr1)
	if (closingServer == nil) || (err != nil) {
		t.Fatalf("new relay server: %v", err)
	}
	defer closingServer.Close()

	go func () {
		conn, _ := closingServer.Accept()

		time.Sleep(timeout)
		flag.Store(true)

		conn.Close()
	}()

	doneSignal.Add(1)
	go func () {
		route, _ := NewRoute([]string{ addr0, addr1 },
			mockRoutingResolver)
		doneSignal.Wait()
		route.Close()
	}()

	t.Fatalf("hanging")
	conn, err := server.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	msg, err := conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Fatalf("recv: %v", err)
	}

	service.Handle(msg.(*RoutingMessage), conn)
	if flag.Load() == false {
		t.Errorf("handle should hang")
	}

	err = service.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	doneSignal.Done()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestRouteEmpty(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	route, err = NewRoute([]string{}, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Errorf("new: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteUnresolved(t *testing.T) {
	var route Route
	var err error

	route, err = NewRoute([]string{ "." }, mockRoutingResolver)
	if (route != nil) || (err == nil) {
		t.Errorf("new should fail")
	}
}

func TestRouteUnreachable(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	addr := findTcpAddress(t)

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteOneHopAccept(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runCloserAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneHopCloseAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runCloserAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != io.EOF) {
		t.Errorf("accept should fail: %v", err)
	}
}

func TestRouteOneHopAcceptCloseAccept(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runCloserAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestRouteOneHopAcceptHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var route Route
	var err error

	addr := findTcpAddress(t)
	server, err := NewTcpServer(addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	t1 := time.AfterFunc(timeout, func () {
		conn, _ := server.Accept()
		if conn != nil {
			conn.Close()
		}
	})

	conn, err := route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	t1.Stop()

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneHopBroadcast(t *testing.T) {
	var msg Message
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	msgc := make(chan Message)
	err = runAddr(ctx, addr, func (msg Message, conn Connection) {
		msgc <- msg
		conn.Close()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg = <-msgc
	if _, ok := msg.(*mockRoutingMessage) ; !ok {
		t.Errorf("msg: %T", msg)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteOneHopCloseBroadcast(t *testing.T) {
	var doneSignal sync.WaitGroup
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	doneSignal.Add(1)
	err = runAddr(ctx, addr, func (msg Message, conn Connection) {
		doneSignal.Wait()
		conn.Close()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != io.EOF {
		t.Errorf("send should fail: %v", err)
	}

	doneSignal.Done()
}

func TestRouteOneHopAcceptEcho(t *testing.T) {
	var conn Connection
	var msg Message
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runEchoAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = conn.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneHopAcceptCloseEcho(t *testing.T) {
	var conn Connection
	var msg Message
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runEchoAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestRouteOneTwoHopAccept(t *testing.T) {
	var conns []Connection
	var rconn Connection
	var route Route
	var err error

	addrs := []string{ findTcpAddress(t), findTcpAddress(t) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := range addrs {
		err = runCloserAddr(ctx, addrs[i])
		if err != nil {
			t.Fatalf("new server[%d]: %v", i, err)
		}
	}

	addr := strings.Join(addrs, "+")
	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conns = make([]Connection, len(addrs))
	for i := range conns {
		conns[i], err = route.Accept()
		if (conns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept should fail: %v", err)
	}

	for i := range conns {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoHopAcceptOneUnreachable(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runCloserAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	addr := addr0 + "+" + addr1
	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept[0]: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err == nil) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoHopAcceptOneUnreachableOneHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var conn, rconn Connection
	var flag atomic.Bool
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)

	server, err := NewTcpServer(addr0)
	if (server == nil) || (err != nil) {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	addr := addr0 + "+" + addr1
	route, err = NewRoute([]string{ addr }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)

		conn, _ := server.Accept()
		if conn != nil {
			conn.Close()
		}
	})

	conn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept[0] should hang")
	}
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept[0]: %v", err)
	}

	t1.Stop()

	rconn, err = route.Accept()
	if (rconn != nil) || (err == nil) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteTwoHopsLastUnresolved(t *testing.T) {
	var route Route
	var err error

	addr := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	route, err = NewRoute([]string{ addr, "." }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err := route.Accept()
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteTwoHopsLastUnreachable(t *testing.T) {
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err := route.Accept()
	if (conn != nil) || (err == nil) {
		t.Errorf("accept should fail")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteTwoHopsAccept(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}
	
	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Fatalf("last accept should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}	
}

func TestRouteTwoHopsCloseAccept(t *testing.T) {
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}	

	conn, err := route.Accept()
	if (conn != nil) || (err != io.EOF) {
		t.Fatalf("accept should fail: %v", err)
	}
}

func TestRouteTwoHopsAcceptCloseAccept(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}
	
	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}	

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Fatalf("last accept should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestRouteTwoHopsAcceptHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var conn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	server, err := NewTcpServer(addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)

		conn, _ := server.Accept()
		if conn != nil {
			conn.Close()
		}
	})

	conn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	t1.Stop()

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteTwoHopsBroadcast(t *testing.T) {
	var msg Message
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	msgc := make(chan Message)
	err = runAddr(ctx, addr1, func (msg Message, conn Connection) {
		msgc <- msg
		conn.Close()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	t.Fatalf("hanging")
	msg = <-msgc
	if _, ok := msg.(*mockRoutingMessage) ; !ok {
		t.Errorf("msg: %T", msg)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestRouteTwoHopsAcceptEcho(t *testing.T) {
	var conn Connection
	var msg Message
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runEchoAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = conn.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteTwoHopsAcceptCloseEcho(t *testing.T) {
	var conn Connection
	var msg Message
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runEchoAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRoutingProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestRouteOneTwoForkAccept(t *testing.T) {
	var conns []Connection
	var rconn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addrs := []string{ findTcpAddress(t), findTcpAddress(t) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	for i := range addrs {
		err = runCloserAddr(ctx, addrs[i])
		if err != nil {
			t.Fatalf("new server[%d]: %v", i, err)
		}
	}

	addr1 := strings.Join(addrs, "+")
	route, err = NewRoute([]string{ addr0, addr1 }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conns = make([]Connection, len(addrs))
	for i := range conns {
		conns[i], err = route.Accept()
		if (conns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept should fail: %v", err)
	}

	for i := range conns {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoForkAcceptOneUnreachable(t *testing.T) {
	var conn, rconn Connection
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	addr2 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	err = runCloserAddr(ctx, addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	addrs := addr1 + "+" + addr2
	route, err = NewRoute([]string{ addr0, addrs }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	conn, err = route.Accept()
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept[0]: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err == nil) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection[0] close: %v", err)
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoForkAcceptOneUnreachableOneHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var conn, rconn Connection
	var flag atomic.Bool
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	addr2 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	server, err := NewTcpServer(addr1)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	addrs := addr1 + "+" + addr2
	route, err = NewRoute([]string{ addr0, addrs }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)

		conn, _ := server.Accept()
		if conn != nil {
			conn.Close()
		}
	})

	conn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept[0] should hang")
	}
	if (conn == nil) || (err != nil) {
		t.Fatalf("accept[0]: %v", err)
	}

	t1.Stop()

	rconn, err = route.Accept()
	if (rconn != nil) || (err == nil) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection[0] close: %v", err)
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoForkBroadcastOneUnreachable(t *testing.T) {
	var msg Message
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	addr2 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	msgc := make(chan Message)
	err = runAddr(ctx, addr1, func (msg Message, conn Connection) {
		msgc <- msg
		conn.Close()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	addrs := addr1 + "+" + addr2
	route, err = NewRoute([]string{ addr0, addrs }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("bcast: %v", err)
	}

	t.Fatalf("hanging")
	msg = <-msgc
	if msg == nil {
		t.Errorf("recv: nil")
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestRouteOneTwoForkBroadcastOneClosed(t *testing.T) {
	var msg Message
	var route Route
	var err error

	addr0 := findTcpAddress(t)
	addr1 := findTcpAddress(t)
	addr2 := findTcpAddress(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = runRoutingAddr(ctx, addr0)
	if err != nil {
		t.Fatalf("new relay: %v", err)
	}

	msgc := make(chan Message)
	err = runAddr(ctx, addr1, func (msg Message, conn Connection) {
		msgc <- msg
		conn.Close()
	})
	if err != nil {
		t.Fatalf("new server[0]: %v", err)
	}

	err = runCloserAddr(ctx, addr2)
	if err != nil {
		t.Fatalf("new server[1]: %v", err)
	}

	addrs := addr1 + "+" + addr2
	route, err = NewRoute([]string{ addr0, addrs }, mockRoutingResolver)
	if (route == nil) || (err != nil) {
		t.Fatalf("new route: %v", err)
	}

	err = route.Send(&mockRoutingMessage{}, mockRoutingProtocol)
	if err != nil {
		t.Errorf("bcast: %v", err)
	}

	t.Fatalf("hanging")
	msg = <-msgc
	if msg == nil {
		t.Errorf("recv: nil")
	}
	
	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}
