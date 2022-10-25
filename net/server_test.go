package net


import (
	"bytes"
	"io"
	sio "silk/io"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


type mockServerMessage struct {
}

func (this *mockServerMessage) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *mockServerMessage) Decode(source sio.Source) error {
	return source.Error()
}


var mockServerProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &mockServerMessage{},
})


type mockServer struct {
	connc chan Connection
	closed bool
}

func newMockServer() *mockServer {
	return &mockServer{
		connc: make(chan Connection, 128),
		closed: false,
	}
}

func (this *mockServer) Close() error {
	this.closed = true
	defer func () { recover() }()
	close(this.connc)
	return nil
}

func (this *mockServer) Accept() (Connection, error) {
	var conn Connection
	var ok bool

	conn, ok = <-this.connc
	if !ok {
		return nil, nil
	} else if conn == nil {
		return nil, io.EOF
	}

	return conn, nil
}

func (this *mockServer) fail() {
	this.connc <- nil
}

func (this *mockServer) connect(conn Connection) {
	this.connc <- conn
}


type mockServerConnection struct {
	datac chan []byte
	closed bool
}

func newMockServerConnection() *mockServerConnection {
	return &mockServerConnection{
		datac: make(chan []byte, 128),
		closed: false,
	}
}

func (this *mockServerConnection) Close() error {
	this.closed = true
	defer func () { recover() }()
	close(this.datac)
	return nil
}

func (this *mockServerConnection) Send(msg Message, proto Protocol) error {
	return nil
}

func (this *mockServerConnection) Recv(proto Protocol) (Message, error) {
	var buf *bytes.Buffer
	var d []byte

	d = <-this.datac
	if d == nil {
		return nil, io.EOF
	}

	buf = bytes.NewBuffer(d)

	return proto.Decode(sio.NewReaderSource(buf))
}

func (this *mockServerConnection) fail() {
	this.datac <- nil
}

func (this *mockServerConnection) send() {
	var buf bytes.Buffer
	mockServerProtocol.Encode(sio.NewWriterSink(&buf),&mockServerMessage{})
	this.datac <- buf.Bytes()
}


// ----------------------------------------------------------------------------


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestServerChannelClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerChannel = NewServerChannel(server)
	var err error

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerChannelAcceptError(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerChannel = NewServerChannel(server)
	var conn Connection
	var err error
	var ok bool

	server.fail()

	conn, ok = <-channel.Accept()
	if (conn != nil) || ok {
		t.Errorf("accept should fail")
	}

	if channel.Err() == nil {
		t.Errorf("err: nil")
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestServerChannelAcceptClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerChannel = NewServerChannel(server)
	var conn Connection
	var err error
	var ok bool

	server.connect(newMockServerConnection())

	conn, ok = <-channel.Accept()
	if (conn == nil) || !ok {
		t.Fatalf("accept failed")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}

	if server.closed {
		t.Errorf("closed: true")
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerChannelPendingAcceptClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerChannel = NewServerChannel(server)
	var conn Connection
	var err error
	var ok bool

	server.connect(newMockServerConnection())
	server.connect(newMockServerConnection())

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	conn, ok = <-channel.Accept()
	if (conn != nil) || ok {
		t.Errorf("accept should fail")
	}
	
	if server.closed == false {
		t.Errorf("closed: false")
	}

	if channel.Err() != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerChannelAcceptAllClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerChannel = NewServerChannel(server)
	var conn Connection
	var err error
	var ok bool

	server.connect(newMockServerConnection())
	server.connect(newMockServerConnection())
	server.Close()

	conn, ok = <-channel.Accept()
	if (conn == nil) || !ok {
		t.Fatalf("first accept failed")
	}

	conn, ok = <-channel.Accept()
	if (conn == nil) || !ok {
		t.Fatalf("second accept failed")
	}

	conn, ok = <-channel.Accept()
	if (conn != nil) || ok {
		t.Fatalf("third accept should fail")
	}

	if channel.Err() != nil {
		t.Errorf("err: %v", err)
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestServerMessageChannelClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var err error

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerMessageChannelAcceptError(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var cmsg ConnectionMessage
	var err error
	var ok bool

	server.fail()

	cmsg, ok = <-channel.Accept()
	if (cmsg != nil) || ok {
		t.Errorf("accept should fail")
	}

	if channel.Err() == nil {
		t.Errorf("err: nil")
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestServerMessageChannelAcceptRecvError(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var conn *mockServerConnection
	var err error

	conn = newMockServerConnection()
	server.connect(conn)
	conn.fail()

	select {
	case <-channel.Accept():
		t.Errorf("accept should hang")
	case <-time.After(50 * time.Millisecond):
	}

	if channel.Err() != nil {
		t.Errorf("err: %v", err)
	}

	if server.closed {
		t.Errorf("server closed: true")
	}

	if conn.closed == false {
		t.Errorf("conn closed: false")
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}

func TestServerChannelAcceptRecvClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var conn *mockServerConnection
	var cmsg ConnectionMessage
	var err error
	var ok bool

	conn = newMockServerConnection()
	server.connect(conn)
	conn.send()

	cmsg, ok = <-channel.Accept()
	if (cmsg == nil) || !ok {
		t.Fatalf("accept failed")
	} else if cmsg.Connection() == nil {
		t.Errorf("accept nil connection")
	} else if cmsg.Message() == nil {
		t.Errorf("accept nil message")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}

	if server.closed {
		t.Errorf("closed: true")
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	if server.closed == false {
		t.Errorf("closed: false")
	}

	err = channel.Err()
	if err != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerChannelAcceptPendingRecvClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var conn0, conn1 *mockServerConnection
	var cmsg ConnectionMessage
	var err error
	var ok bool

	conn0 = newMockServerConnection()
	conn1 = newMockServerConnection()
	server.connect(conn0)
	server.connect(conn1)

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}

	cmsg, ok = <-channel.Accept()
	if (cmsg != nil) || ok {
		t.Errorf("accept should fail")
	}
	
	if server.closed == false {
		t.Errorf("server closed: false")
	}

	if conn0.closed == false {
		t.Errorf("connection 0 closed: false")
	}

	if conn1.closed == false {
		t.Errorf("connection 1 closed: false")
	}

	if channel.Err() != nil {
		t.Errorf("err: %v", err)
	}
}

func TestServerMessageChannelAcceptAllClose(t *testing.T) {
	var server *mockServer = newMockServer()
	var channel ServerMessageChannel =
		NewServerMessageChannel(server, mockServerProtocol)
	var conn0, conn1 *mockServerConnection
	var cmsg ConnectionMessage
	var err error
	var ok bool

	conn0 = newMockServerConnection()
	conn1 = newMockServerConnection()
	server.connect(conn0)
	server.connect(conn1)
	server.Close()
	conn0.send()
	conn1.send()

	cmsg, ok = <-channel.Accept()
	if (cmsg == nil) || !ok {
		t.Fatalf("first accept failed")
	}

	cmsg, ok = <-channel.Accept()
	if (cmsg == nil) || !ok {
		t.Fatalf("second accept failed")
	}

	cmsg, ok = <-channel.Accept()
	if (cmsg != nil) || ok {
		t.Fatalf("third accept should fail")
	}

	if channel.Err() != nil {
		t.Errorf("err: %v", err)
	}

	err = channel.Close()
	if err != nil {
		t.Errorf("close: %v", err)
	}
}
