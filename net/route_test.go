package net


import (
	"io"
	sio "silk/io"
	"sync/atomic"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


var mockRouteProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &RoutingMessage{},
	1: &mockRouteMessage{},
})


type mockRouteMessage struct {
}

func (this *mockRouteMessage) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *mockRouteMessage) Decode(source sio.Source) error {
	return source.Error()
}


type mockRouteAccepter struct {
	acceptc chan Connection
}

func newMockRouteAccepter() *mockRouteAccepter {
	var this mockRouteAccepter

	this.acceptc = make(chan Connection)

	return &this
}

func (this *mockRouteAccepter) Send(Message, Protocol) error {
	panic("not implemented")
}

func (this *mockRouteAccepter) accept(conn Connection) {
	this.acceptc <- conn
}

func (this *mockRouteAccepter) Accept() (Connection, error) {
	var conn Connection
	var ok bool

	conn, ok = <-this.acceptc
	if !ok {
		return nil, io.EOF
	}

	return conn, nil
}

func (this *mockRouteAccepter) Close() error {
	defer func () { recover() }()
	close(this.acceptc)
	return nil
}


type mockRouteError struct {
}

func (this *mockRouteError) Error() string {
	return "mock route error"
}

var mockRouteErr *mockRouteError = &mockRouteError{}


// ----------------------------------------------------------------------------


func TestTerminalRouteZeroClose(t *testing.T) {
	var route Route
	var err error

	route = NewTerminalRoute([]Connection{})
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTerminalRouteZeroSend(t *testing.T) {
	var route Route
	var err error

	route = NewTerminalRoute([]Connection{})
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTerminalRouteZeroAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	route = NewTerminalRoute([]Connection{})
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("fist accept: %p %v", conn, err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("second accept: %p %v", conn, err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTerminalRouteZeroCloseAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	route = NewTerminalRoute([]Connection{})
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestTerminalRouteOneSyncClose(t *testing.T) {
	var inner, conn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncAcceptClose(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("routed connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncCloseAccept(t *testing.T) {
	var inner, conn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != io.EOF) {
		t.Errorf("accept should fail: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept should fail: %v", err)
	}
}

func TestTerminalRouteOneSyncAcceptCloseAccept(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncBroadcast(t *testing.T) {
	var inner, conn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	errc := make(chan error)
	go func () {
		errc <- route.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("bcast: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncCloseBroadcast(t *testing.T) {
	var inner, conn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != io.EOF {
		t.Errorf("bcast should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncCloseSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneSyncCloseRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestTerminalRouteOneAsyncBroadcast(t *testing.T) {
	var inner, conn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(1)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("bcast: %v", err)
	}

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneAsyncSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(1)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneAsyncCloseSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(1)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneAsyncRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(1)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteOneAsyncCloseRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(1)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	route = NewTerminalRoute([]Connection{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestTerminalRouteTwoSyncClose(t *testing.T) {
	var inners, conns []Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncAcceptClose(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncCloseAccept(t *testing.T) {
	var inners, conns []Connection
	var rconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != io.EOF) {
		t.Errorf("accept should fail: %v", err)
	}
}

func TestTerminalRouteTwoSyncOneAcceptCloseAccept(t *testing.T) {
	var inners, conns []Connection
	var rconn, lconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Errorf("accept[0]: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != io.EOF) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestTerminalRouteTwoSyncTwoAcceptCloseAccept(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Errorf("accept[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncBroadcast(t *testing.T) {
	var inners, conns []Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	errc := make(chan error)
	go func () {
		errc <- route.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	err = <-errc
	if err != nil {
		t.Errorf("bcast: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncCloseBroadcast(t *testing.T) {
	var inners, conns []Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != io.EOF {
		t.Errorf("bcast should fail: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncCloseSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- conns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestTerminalRouteTwoSyncCloseRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	route = NewTerminalRoute(inners)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- conns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestCompositeRouteZeroClose(t *testing.T) {
	var route Route
	var err error

	route = NewCompositeRoute([]Route{})
	if route == nil {
		t.Fatalf("new: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestCompositeRouteZeroSend(t *testing.T) {
	var route Route
	var err error

	route = NewCompositeRoute([]Route{})
	if route == nil {
		t.Fatalf("new: nil")
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestCompositeRouteZeroAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	route = NewCompositeRoute([]Route{})
	if route == nil {
		t.Fatalf("new: nil")
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestCompositeRouteOneSyncClose(t *testing.T) {
	var inner, conn Connection
	var term, route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncAcceptClose(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var term, route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Fatalf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncCloseAccept(t *testing.T) {
	var inner, conn, rconn Connection
	var term, route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != io.EOF) {
		t.Errorf("accept should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestCompositeRouteOneChannelTwoAcceptCloseAccept(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var chanr, route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection, 1) ; connc <- inner ; close(connc)
	errc := make(chan error) ; close(errc)
	chanr = NewChannelRoute(connc, errc)
	if chanr == nil {
		t.Fatalf("new channel: nil")
	}

	route = NewCompositeRoute([]Route{ chanr })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept[0]: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept[1]: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncBroadcast(t *testing.T) {
	var inner, conn Connection
	var term, route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	go func () {
		err := route.Send(&mockRouteMessage{}, mockRouteProtocol)
		if err != nil {
			t.Errorf("bcast: %v", err)
		}
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncCloseBroadcast(t *testing.T) {
	var inner, conn Connection
	var term, route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err= route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != io.EOF {
		t.Errorf("bcast should fail: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncSend(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var term, route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Fatalf("last accept: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncCloseSend(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var term, route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Fatalf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncRecv(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var term, route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Fatalf("last accept: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}

func TestCompositeRouteOneSyncCloseRecv(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var term, route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	term = NewTerminalRoute([]Connection{ inner })
	if term == nil {
		t.Fatalf("new terminal: nil")
	}

	route = NewCompositeRoute([]Route{ term })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Fatalf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	errc := make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestCompositeRouteTwoTwoSyncClose(t *testing.T) {
	var inners, conns []Connection
	var terms []Route
	var route Route
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}
}

func TestCompositeRouteTwoTwoSyncAcceptClose(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var terms []Route
	var route Route
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	rconns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	for i := 0; i < 2*2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestCompositeRouteTwoTwoSyncBroadcast(t *testing.T) {
	var inners, conns []Connection
	var terms []Route
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	go func () {
		err := route.Send(&mockRouteMessage{}, mockRouteProtocol)
		if err != nil {
			t.Errorf("bcast: %v", err)
		}
	}()

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			msg, err = conns[i * 2 + j].Recv(mockRouteProtocol)
			if (msg == nil) || (err != nil) {
				t.Errorf("recv[%d][%d]: %v", i, j, err)
			}
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}
}

func TestCompositeRouteTwoTwoSyncSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var terms []Route
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	rconns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	for i := 0; i < 2*2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2 * 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			msg, err = conns[i * 2 + j].Recv(mockRouteProtocol)
			if (msg == nil) || (err != nil) {
				t.Errorf("recv[%d][%d]: %v", i, j, err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestCompositeRouteTwoTwoSyncCloseSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var terms []Route
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	rconns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	for i := 0; i < 2*2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2 * 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			msg, err = conns[i * 2 + j].Recv(mockRouteProtocol)
			if (msg == nil) || (err != nil) {
				t.Errorf("recv[%d][%d]: %v", i, j, err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestCompositeRouteTwoTwoSyncRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var terms []Route
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	rconns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	for i := 0; i < 2*2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			go func (index int) {
				errc <- conns[index].Send(&mockRouteMessage{},
					mockRouteProtocol)
			}(i * 2 + j)
		}
	}

	for i := 0; i < 2 * 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestCompositeRouteTwoTwoSyncCloseRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var terms []Route
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2 * 2)
	conns = make([]Connection, 2 * 2)
	rconns = make([]Connection, 2 * 2)
	terms = make([]Route, 2)

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			inners[i*2+j], conns[i*2+j] = NewLocalConnection(0)
			if (inners[i*2+j] == nil) || (conns[i*2+j] == nil) {
				t.Fatalf("new connection[%d][%d]: %p %p",
					i, j, inners[i*2+j], conns[i*2+j])
			}
		}

		terms[i] = NewTerminalRoute(inners[i*2:i*2+2])
		if terms[i] == nil {
			t.Fatalf("new terminal[%d]: nil", i)
		}
	}

	route = NewCompositeRoute(terms)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	for i := 0; i < 2*2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc := make(chan error)
	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			go func (index int) {
				errc <- conns[index].Send(&mockRouteMessage{},
					mockRouteProtocol)
			}(i * 2 + j)
		}
	}

	for i := 0; i < 2 * 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		for j := 0; j < 2; j++ {
			err = conns[i * 2 + j].Close()
			if err != nil {
				t.Errorf("connection[%d][%d] close: %v", i, j,
					err)
			}
		}
	}

	for i := 0; i < 2 * 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestCompositeRouteHangingAccept(t *testing.T) {
	var inner *mockRouteAccepter = newMockRouteAccepter()
	const timeout = 30 * time.Millisecond
	var inconn, conn, rconn Connection
	var flag atomic.Bool
	var route Route
	var err error

	inconn, conn = NewLocalConnection(0)
	if (inconn == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inconn, conn)
	}

	route = NewCompositeRoute([]Route{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		inner.accept(inconn)
	})

	rconn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	t1.Stop()

	err = rconn.Close()
	if err != nil {
		t.Errorf("route connection close: %v", err)
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

func TestCompositeRouteHangingAcceptClose(t *testing.T) {
	var inner *mockRouteAccepter = newMockRouteAccepter()
	const timeout = 30 * time.Millisecond
	var rconn Connection
	var flag atomic.Bool
	var route Route
	var err error

	route = NewCompositeRoute([]Route{ inner })
	if route == nil {
		t.Fatalf("new route: nil")
	}

	errc := make(chan error)
	t1 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		errc <- route.Close()
	})

	rconn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (rconn != nil) || (err != io.EOF) {
		t.Fatalf("accept should fail: %v", err)
	}

	t1.Stop()

	err = <-errc
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestChannelRouteZeroClose(t *testing.T) {
	var route Route
	var err error

	connc := make(chan Connection) ; close(connc)
	errc := make(chan error) ; close(errc)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestChannelRouteEmptyClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var conn Connection
	var route Route
	var err error

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	errc2 := make(chan error)
	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		errc2 <- route.Close()
	})

	conn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (conn != nil) || (err != io.EOF) {
		t.Errorf("accept should fail: %v", err)
	}

	t0.Stop()

	err = <-errc2
	if err != nil {
		t.Errorf("close: %v", err)
	}

	close(connc)
	close(errc)
}

func TestChannelRouteZeroBroadcast(t *testing.T) {
	var route Route
	var err error

	connc := make(chan Connection) ; close(connc)
	errc := make(chan error) ; close(errc)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestChannelRouteZeroAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	connc := make(chan Connection) ; close(connc)
	errc := make(chan error) ; close(errc)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("fist accept: %p %v", conn, err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("second accept: %p %v", conn, err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestChannelRouteZeroAcceptCloseAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	connc := make(chan Connection) ; close(connc)
	errc := make(chan error) ; close(errc)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new: %p", route)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept: %v", err)
	}
}

func TestChannelRouteEmptyAcceptError(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	close(connc)
	errc <- mockRouteErr
	errc <- mockRouteErr

	conn, err = route.Accept()
	if (conn != nil) || (err != mockRouteErr) {
		t.Errorf("accept should fail: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	close(errc)
}

func TestChannelRouteEmptyCloseAccept(t *testing.T) {
	var conn Connection
	var route Route
	var err error

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != io.EOF) {
		t.Errorf("accept[0] should fail: %v", err)
	}

	conn, err = route.Accept()
	if (conn != nil) || (err != nil) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	close(connc)
	close(errc)
}

func TestChannelRouteEmptyCloseBroadcast(t *testing.T) {
	var route Route
	var err error

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
	if err != io.EOF {
		t.Errorf("bcast should fail: %v", err)
	}

	close(connc)
	close(errc)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestChannelRouteOneSyncClose(t *testing.T) {
	var inner, conn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	connc <- inner

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	close(connc)
	close(errc)
}

func TestChannelRouteOneSyncAcceptClose(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	const timeout = 30 * time.Millisecond
	var flag0, flag1 atomic.Bool
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	t0 := time.AfterFunc(timeout, func () {
		flag0.Store(true)
		connc <- inner
	})

	rconn, err = route.Accept()
	if flag0.Load() == false {
		t.Errorf("accept should hang")
	}
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	t0.Stop()

	t1 := time.AfterFunc(timeout, func () {
		flag1.Store(true)
		close(connc)
		close(errc)
	})

	lconn, err = route.Accept()
	if flag1.Load() == false {
		t.Errorf("last accept should hang")
	}
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	t1.Stop()

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("routed connection close: %v", err)
	}
}

func TestChannelRouteOneSyncAcceptCloseAccept(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error) ; close(errc)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	connc <- inner

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != io.EOF) {
		t.Errorf("last accept[0]: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept[1]: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("routed connection close: %v", err)
	}

	close(connc)
}

func TestChannelRouteOneSyncErrorAccept(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var rconn Connection
	var route Route
	var err error

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	close(connc)

	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		errc <- mockRouteErr
	})

	rconn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (rconn != nil) || (err != mockRouteErr) {
		t.Fatalf("accept should fail: %v", err)
	}

	t0.Stop()

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}
}

func TestChannelRouteOneSyncAcceptError(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	const timeout = 30 * time.Millisecond
	var flag0, flag1 atomic.Bool
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	errc <- mockRouteErr

	t0 := time.AfterFunc(timeout, func () {
		flag0.Store(true)
		connc <- inner
	})

	rconn, err = route.Accept()
	if flag0.Load() == false {
		t.Errorf("accept[0] should hang")
	}
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept[0]: %v", err)
	}

	t0.Stop()

	t1 := time.AfterFunc(timeout, func () {
		flag1.Store(true)
		close(connc)
	})

	lconn, err = route.Accept()
	if flag1.Load() == false {
		t.Errorf("accept[1] should hang")
	}
	if (lconn != nil) || (err != mockRouteErr) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept should fail: %v", err)
	}

	t1.Stop()

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("routed connection close: %v", err)
	}

	close(errc)
}

func TestChannelRouteOneSyncTwoAcceptCloseAccept(t *testing.T) {
	var inner, conn, rconn, lconn Connection
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var route Route
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		connc <- inner
	})

	rconn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("accept should hang")
	}
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	t0.Stop()

	close(connc)
	close(errc)

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestChannelRouteOneSyncBroadcast(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var inner, conn Connection
	var flag atomic.Bool
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		connc <- inner
		close(connc)
	})

	go func () {
		err := route.Send(&mockRouteMessage{}, mockRouteProtocol)
		if err != nil {
			t.Errorf("bcast: %v", err)
		}
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if flag.Load() == false {
		t.Errorf("recv should hang")
	}
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	t0.Stop()

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	close(errc)
}

func TestChannelRouteOneSyncSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection, 1)
	connc <- inner
	close(connc)

	errc := make(chan error)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	errc = make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestChannelRouteOneSyncCloseSend(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection, 1)
	connc <- inner
	close(connc)

	errc := make(chan error)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc = make(chan error)
	go func () {
		errc <- rconn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = conn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestChannelRouteOneSyncRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection, 1)
	connc <- inner
	close(connc)

	errc := make(chan error)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	errc = make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestChannelRouteOneSyncCloseRecv(t *testing.T) {
	var inner, conn, rconn Connection
	var route Route
	var msg Message
	var err error

	inner, conn = NewLocalConnection(0)
	if (inner == nil) || (conn == nil) {
		t.Fatalf("new connection: %p %p", inner, conn)
	}

	connc := make(chan Connection, 1)
	connc <- inner
	close(connc)

	errc := make(chan error)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: nil")
	}

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Fatalf("accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc = make(chan error)
	go func () {
		errc <- conn.Send(&mockRouteMessage{}, mockRouteProtocol)
	}()

	msg, err = rconn.Recv(mockRouteProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = <-errc
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestChannelRouteTwoSyncAcceptClose(t *testing.T) {
	var inners, conns, rconns []Connection
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var rconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		flag.Store(false)
		timer := time.AfterFunc(timeout, func () {
			flag.Store(true)
			connc <- inners[i]
		})

		rconns[i], err = route.Accept()
		if flag.Load() == false {
			t.Errorf("accept[%d] should hang", i)
		}
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}

		timer.Stop()
	}

	flag.Store(false)
	timer := time.AfterFunc(timeout, func () {
		flag.Store(true)
		close(connc)
		close(errc)
	})

	rconn, err = route.Accept()
	if flag.Load() == false {
		t.Errorf("last accept should hang")
	}
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	timer.Stop()

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncOneAcceptCloseAccept(t *testing.T) {
	var inners, conns []Connection
	var rconn, lconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	connc <- inners[0]
	connc <- inners[1]
	close(connc)
	close(errc)

	rconn, err = route.Accept()
	if (rconn == nil) || (err != nil) {
		t.Errorf("accept[0]: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != io.EOF) {
		t.Errorf("accept[1] should fail: %v", err)
	}

	err = rconn.Close()
	if err != nil {
		t.Errorf("connection close: %v", err)
	}
}

func TestChannelRouteTwoSyncThreeAcceptCloseAccept(t *testing.T) {
	var inners, conns, rconns []Connection
	var lconn Connection
	var route Route
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		connc <- inners[i]
	}
	close(connc)
	close(errc)

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	lconn, err = route.Accept()
	if (lconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = rconns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncBroadcast(t *testing.T) {
	var inners, conns []Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
	}

	connc := make(chan Connection)
	errc := make(chan error)
	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		connc <- inners[i]
	}

	close(connc)
	close(errc)

	go func () {
		err = route.Send(&mockRouteMessage{}, mockRouteProtocol)
		if err != nil {
			t.Errorf("bcast: %v", err)
		}
	}()

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)
	connc := make(chan Connection, 2)
	errc := make(chan error)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
		connc <- inners[i]
	}

	close(connc)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc = make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncCloseSend(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)
	connc := make(chan Connection, 2)
	errc := make(chan error)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
		connc <- inners[i]
	}

	close(connc)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc = make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- rconns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = conns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)
	connc := make(chan Connection, 2)
	errc := make(chan error)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
		connc <- inners[i]
	}

	close(connc)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	errc = make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- conns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}

func TestChannelRouteTwoSyncCloseRecv(t *testing.T) {
	var inners, conns, rconns []Connection
	var rconn Connection
	var route Route
	var msg Message
	var err error

	inners = make([]Connection, 2)
	conns = make([]Connection, 2)
	rconns = make([]Connection, 2)
	connc := make(chan Connection, 2)
	errc := make(chan error)

	for i := 0; i < 2; i++ {
		inners[i], conns[i] = NewLocalConnection(0)
		if (inners[i] == nil) || (conns[i] == nil) {
			t.Fatalf("new connection[%d]: %p %p", i, inners[i],
				conns[i])
		}
		connc <- inners[i]
	}

	close(connc)
	close(errc)

	route = NewChannelRoute(connc, errc)
	if route == nil {
		t.Fatalf("new route: %p", route)
	}

	for i := 0; i < 2; i++ {
		rconns[i], err = route.Accept()
		if (rconns[i] == nil) || (err != nil) {
			t.Fatalf("accept[%d]: %v", i, err)
		}
	}

	rconn, err = route.Accept()
	if (rconn != nil) || (err != nil) {
		t.Errorf("last accept: %v", err)
	}

	err = route.Close()
	if err != nil {
		t.Errorf("route close: %v", err)
	}

	errc = make(chan error)
	for i := 0; i < 2; i++ {
		go func (index int) {
			errc <- conns[index].Send(&mockRouteMessage{},
				mockRouteProtocol)
		}(i)
	}

	for i := 0; i < 2; i++ {
		msg, err = rconns[i].Recv(mockRouteProtocol)
		if (msg == nil) || (err != nil) {
			t.Errorf("recv[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = <-errc
		if err != nil {
			t.Errorf("send[%d]: %v", i, err)
		}
	}

	for i := 0; i < 2; i++ {
		err = conns[i].Close()
		if err != nil {
			t.Errorf("connection[%d] close: %v", i, err)
		}

		err = rconns[i].Close()
		if err != nil {
			t.Errorf("route connection[%d] close: %v", i, err)
		}
	}
}
