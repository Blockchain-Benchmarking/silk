package net


import (
	"context"
	"fmt"
	sio "silk/io"
	"sync/atomic"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


var mockConnectionProtocol Protocol = NewRawProtocol(&mockConnectionMessage{})


type mockConnectionMessage struct {
	value uint64
}

func (this *mockConnectionMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.value).Error()
}

func (this *mockConnectionMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.value).Error()
}


func runEchoServer(server Server) {
	var conn Connection
	var msg Message
	var err error

	conn, err = server.Accept()
	if (conn == nil) || (err != nil) {
		return
	}

	defer conn.Close()

	msg, err = conn.Recv(mockConnectionProtocol)
	if (msg == nil) || (err != nil) {
		return
	}

	conn.Send(msg, mockConnectionProtocol)
}


// ----------------------------------------------------------------------------


func TestTcpConnectionEmpty(t *testing.T) {
	var conn Connection
	var err error

	conn, err = NewTcpConnection("")
	if (conn != nil) || (err == nil) {
		t.Errorf("new should fail")
	}
}

func TestTcpConnectionRandomString(t *testing.T) {
	var conn Connection
	var err error

	conn, err = NewTcpConnection("Hello World!")
	if (conn != nil) || (err == nil) {
		t.Errorf("new should fail")
	}
}

func TestTcpConnectionIpv4(t *testing.T) {
	var conn Connection
	var err error

	conn, err = NewTcpConnection("127.0.0.1")
	if (conn != nil) || (err == nil) {
		t.Errorf("new should fail")
	}
}

func TestTcpConnectionIpv4PortUnreachable(t *testing.T) {
	var conn Connection
	var err error

	addr := fmt.Sprintf("127.0.0.1:%d", findTcpPort(t))

	conn, err = NewTcpConnection(addr)
	if (conn != nil) || (err == nil) {
		t.Errorf("new should fail")
	}
}

func TestTcpConnectionIpv4Port(t *testing.T) {
	var conn Connection
	var msg Message
	var err error

	addr := fmt.Sprintf("127.0.0.1:%d", findTcpPort(t))
	server, err := NewTcpServer(addr)
	if (server == nil) || (err != nil) {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()
	go runEchoServer(server)

	conn, err = NewTcpConnection(addr)
	if (conn == nil) || (err != nil) {
		t.Fatalf("new: %v", err)
	}

	err = conn.Send(&mockConnectionMessage{}, mockConnectionProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockConnectionProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	err = conn.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTcpConnectionIpv4PortHanging(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var conn Connection
	var msg Message
	var err error

	addr := fmt.Sprintf("127.0.0.1:%d", findTcpPort(t))
	server, err := NewTcpServer(addr)
	if (server == nil) || (err != nil) {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		runEchoServer(server)
	})

	conn, err = NewTcpConnection(addr)
	if (conn == nil) || (err != nil) {
		t.Fatalf("new: %v", err)
	}

	err = conn.Send(&mockConnectionMessage{}, mockConnectionProtocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	msg, err = conn.Recv(mockConnectionProtocol)
	if (msg == nil) || (err != nil) {
		t.Errorf("recv: %v", err)
	}

	if flag.Load() == false {
		t.Errorf("connection should hang")
	}

	t0.Stop()

	err = conn.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestTcpConnectionIpv4PortHangingClose(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var flag atomic.Bool
	var conn Connection
	var msg Message
	var err error

	addr := fmt.Sprintf("127.0.0.1:%d", findTcpPort(t))
	server, err := NewTcpServer(addr)
	if (server == nil) || (err != nil) {
		t.Fatalf("new server: %v", err)
	}
	defer server.Close()

	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		server.Close()
	})

	conn, err = NewTcpConnection(addr)
	if conn != nil {
		if err != nil {
			t.Errorf("new is contradictory: %v", err)
		}

		conn.Send(&mockConnectionMessage{}, mockConnectionProtocol)

		msg, err = conn.Recv(mockConnectionProtocol)
		if (msg != nil) || (err == nil) {
			t.Errorf("recv should fail: %#v %v", msg, err)
		}
	} else if err == nil {
		t.Errorf("new is contradictory")
	}

	if flag.Load() == false {
		t.Errorf("connection should hang")
	}

	t0.Stop()
}

func TestTcpConnectionHangingCancel(t *testing.T) {
	const timeout = 30 * time.Millisecond
	var cancel context.CancelFunc
	const addr = "1.1.1.1:1"
	var ctx context.Context
	var flag atomic.Bool
	var conn Connection
	var err error

	ctx, cancel = context.WithCancel(context.Background())
	t0 := time.AfterFunc(timeout, func () {
		flag.Store(true)
		cancel()
	})

	conn, err = NewTcpConnectionWith(addr, &TcpConnectionOptions{
		Context: ctx,
	})
	if flag.Load() == false {
		t.Errorf("new should hang")
	}
	if (conn != nil) || (err == nil) {
		t.Errorf("new should fail")
	}

	t0.Stop()
}
