package net


import (
	"bytes"
	"io"
	sio "silk/io"
	// "testing"
	"time"
)


// Mock part ------------------------------------------------------------------


type multiplexMockMessage struct {
	msg Message
	encoded []byte
}

type multiplexMockChannel struct {
	buffer []*multiplexMockMessage
	sending chan<- *multiplexMockMessage
	receiving <-chan *multiplexMockMessage
	autoFlush bool
}

func newMultiplexMockChannels(autoFlush bool) (*multiplexMockChannel, *multiplexMockChannel) {
	var left, right chan *multiplexMockMessage

	left = make(chan *multiplexMockMessage, 1024)
	right = make(chan *multiplexMockMessage, 1024)

	return &multiplexMockChannel{
		buffer: make([]*multiplexMockMessage, 0),
		sending: left,
		receiving: right,
		autoFlush: autoFlush,
	}, &multiplexMockChannel{
		buffer: make([]*multiplexMockMessage, 0),
		sending: right,
		receiving: left,
		autoFlush: autoFlush,
	}
}

func (this *multiplexMockChannel) Send(msg Message, proto Protocol) error {
	var mmsg *multiplexMockMessage
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	mmsg = newMultiplexMockMessage(msg, buf.Bytes())

	if this.autoFlush {
		this.sending <- mmsg
	} else {
		this.buffer = append(this.buffer, mmsg)
	}

	return nil
}

func (this *multiplexMockChannel) Recv(proto Protocol) (Message, error) {
	var mmsg *multiplexMockMessage
	var buf *bytes.Buffer
	var ok bool

	mmsg, ok = <-this.receiving
	if !ok {
		return nil, io.EOF
	}

	buf = bytes.NewBuffer(mmsg.encoded)

	return proto.Decode(sio.NewReaderSource(buf))
}

func (this *multiplexMockChannel) autoFlushMock(autoFlush bool) {
	if autoFlush {
		this.flushMock()
	}

	this.autoFlush = autoFlush
}

func (this *multiplexMockChannel) flushMock() {
	var mmsg *multiplexMockMessage

	for _, mmsg = range this.buffer {
		this.sending <- mmsg
	}

	this.buffer = this.buffer[:0]
}

func (this *multiplexMockChannel) closeMock() {
	close(this.sending)
}

func newMultiplexMockMessage(msg Message, bs []byte) *multiplexMockMessage {
	return &multiplexMockMessage{
		msg: msg,
		encoded: bs,
	}
}


type multiplexTestMessage struct {
	value uint64
}

func (this *multiplexTestMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.value).Error()
}

func (this *multiplexTestMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.value).Error()
}

var multiplexTestProtocol Protocol = NewRawProtocol(&multiplexTestMessage{})


func eventuallyWith(ntentative int, sleep time.Duration, cb func () bool) bool{
	for {
		ntentative -= 1

		if cb() {
			return true
		}

		if ntentative == 0 {
			return false
		}

		time.Sleep(sleep)
	}
}

func eventually(cb func () bool) bool {
	return eventuallyWith(100, time.Millisecond, cb)
}


// Test part ------------------------------------------------------------------


// func TestMultiplexFifoLocalClose(t *testing.T) {
// 	var channel *multiplexMockChannel
// 	var mplex Multiplexer
// 	var err error

// 	channel, _ = newMultiplexMockChannels(false)

// 	mplex = NewFifoMultiplexer(channel)
// 	mplex.Close()

// 	_, err = mplex.Connect()
// 	if err == nil {
// 		t.Errorf("locally closed multiplex Connect")
// 	} else if err != io.EOF {
// 		t.Errorf("wrong Connect error type: %T:%v", err, err)
// 	}

// 	_, err = mplex.Accept()
// 	if err == nil {
// 		t.Errorf("locally closed multiplex Accept")
// 	} else if err != io.EOF {
// 		t.Errorf("wrong Accept error type: %T:%v", err, err)
// 	}

// 	time.Sleep(10 * time.Millisecond)
// }

// func TestMultiplexFifoRemoteClose(t *testing.T) {
// 	var chanA, chanB *multiplexMockChannel
// 	var mplexA, mplexB Multiplexer
// 	var conn Connection
// 	var err error

// 	chanA, chanB = newMultiplexMockChannels(true)

// 	mplexA = NewFifoMultiplexer(chanA)
// 	mplexA.Close()

// 	mplexB = NewFifoMultiplexer(chanB)

// 	if !eventually(func () bool {
// 		conn, err = mplexB.Connect()

// 		if err == io.EOF {
// 			return true
// 		}

// 		if err != nil {
// 			t.Errorf("wrong Connect error type: %T:%v", err, err)
// 			return true
// 		}

// 		conn.Close()
// 		return false
// 	}) {
// 		t.Errorf("remote closed multiplex eventually cannot Connect")
// 	}

// 	_, err = mplexB.Accept()

// 	if err == nil {
// 		t.Errorf("remote closed multiplex eventually cannot Accept")
// 	} else if err != io.EOF {
// 		t.Errorf("wrong Accept error type: %T:%v", err, err)
// 	}

// 	time.Sleep(10 * time.Millisecond)
// }

// func TestMultiplexFifoSingleConnection(t *testing.T) {
// 	var chanA, chanB *multiplexMockChannel
// 	var mplexA, mplexB Multiplexer
// 	var connA, connB Connection
// 	var msg Message
// 	var err error

// 	chanA, chanB = newMultiplexMockChannels(true)

// 	mplexA = NewFifoMultiplexer(chanA)
// 	mplexB = NewFifoMultiplexer(chanB)

// 	connB, err = mplexB.Connect()
// 	if err != nil {
// 		t.Fatalf("failed to Connect")
// 	}

// 	err = connB.Send(&multiplexTestMessage{ 70 }, multiplexTestProtocol)
// 	if err != nil {
// 		t.Fatalf("failed to Send from Connecter")
// 	}

// 	if !eventually(func () bool {
// 		connA, err = mplexA.Accept()
// 		return (err == nil)
// 	}) {
// 		t.Errorf("Accepter cannot eventually Accept")
// 	}

// 	err = connA.Send(&multiplexTestMessage{ 40 }, multiplexTestProtocol)
// 	if err != nil {
// 		t.Fatalf("failed to Send from Accepter")
// 	}

// 	if !eventually(func () bool {
// 		msg, err = connA.Recv(multiplexTestProtocol)
// 		return (err == nil)
// 	}) {
// 		t.Fatalf("Accepter cannot eventually Recv")
// 	}

// 	if msg.(*multiplexTestMessage).value != 70 {
// 		t.Fatalf("failed to Recv correctly from Accepter")
// 	}

// 	err = connA.Close()
// 	if err != nil {
// 		t.Fatalf("failed to Close from Accepter")
// 	}

// 	if !eventually(func () bool {
// 		msg, err = connB.Recv(multiplexTestProtocol)
// 		return (err == nil)
// 	}) {
// 		t.Fatalf("Connecter cannot eventually Recv")
// 	}

// 	if msg.(*multiplexTestMessage).value != 40 {
// 		t.Fatalf("failed to Recv correctly from Connecter")
// 	}
	
// 	err = connB.Close()
// 	if err != nil {
// 		t.Fatalf("failed to Close from Connecter")
// 	}

// 	time.Sleep(10 * time.Millisecond)
// }

// func TestMultiplexFifoManyConnections(t *testing.T) {
// 	var chanA, chanB *multiplexMockChannel
// 	var mplexA, mplexB Multiplexer
// 	var connAs, connBs []Connection
// 	var conn Connection
// 	var expected uint64
// 	var msg Message
// 	var err error
// 	var i, j int

// 	chanA, chanB = newMultiplexMockChannels(true)

// 	mplexA = NewFifoMultiplexer(chanA)
// 	mplexB = NewFifoMultiplexer(chanB)

// 	connAs = make([]Connection, 0)
// 	connBs = make([]Connection, 0)

// 	for i = 0; i < 10; i++ {
// 		conn, err = mplexA.Connect()
// 		if err != nil {
// 			t.Fatalf("failed to Connect A[%d]", i)
// 		}
// 		connAs = append(connAs, conn)

// 		conn, err = mplexB.Connect()
// 		if err != nil {
// 			t.Fatalf("failed to Connect B[%d]", i)
// 		}
// 		connBs = append(connBs, conn)
// 	}

// 	for i = 0; i < 10; i++ {
// 		if !eventually(func () bool {
// 			conn, err = mplexA.Accept()
// 			return (err == nil)
// 		}) {
// 			t.Fatalf("failed to eventually Accept A[%d]", 10 + i)
// 		}
// 		connAs = append(connAs, conn)

// 		if !eventually(func () bool {
// 			conn, err = mplexB.Accept()
// 			return (err == nil)
// 		}) {
// 			t.Fatalf("failed to eventually Accept B[%d]", 10 + i)
// 		}
// 		connBs = append(connBs, conn)
// 	}

// 	for i = 0; i < (10 * 2); i++ {
// 		for j = 0; j < 1000; j++ {
// 			err = connAs[i].Send(
// 				&multiplexTestMessage{ uint64(i*1000 + j) },
// 				multiplexTestProtocol)
// 			if err != nil {
// 				t.Fatalf("failed to Send[%d] on A[%d]", j, i)
// 			}

// 			err = connBs[i].Send(
// 				&multiplexTestMessage{
// 					uint64(1000000 + i*1000 + j),
// 				}, multiplexTestProtocol)
// 			if err != nil {
// 				t.Fatalf("failed to Send[%d] on B[%d]", j, i)
// 			}
// 		}
// 	}

// 	for j = 0; j < 1000; j++ {
// 		for i = 0; i < (10 * 2); i++ {
// 			expected = uint64(1000000 + ((i + 10) % 20) * 1000 + j)

// 			if !eventually(func () bool {
// 				msg,err = connAs[i].Recv(multiplexTestProtocol)
// 				return (err == nil)
// 			}) {
// 				t.Fatalf("failed to eventually Recv[%d] A[%d]",
// 					j, i)
// 			}

// 			if msg.(*multiplexTestMessage).value != expected {
// 				t.Fatalf("incorrect value for Recv[%d] on A[%d]: %d != %d",
// 					j, i,
// 					msg.(*multiplexTestMessage).value,
// 					expected)
// 			}
// 		}

// 		for i = 0; i < (10 * 2); i++ {
// 			expected = uint64(((i + 10) % 20) * 1000 + j)

// 			if !eventually(func () bool {
// 				msg,err = connBs[i].Recv(multiplexTestProtocol)
// 				return (err == nil)
// 			}) {
// 				t.Fatalf("failed to eventually Recv[%d] B[%d]",
// 					j, i)
// 			}

// 			if msg.(*multiplexTestMessage).value != expected {
// 				t.Fatalf("incorrect value for Recv[%d] on B[%d]: %d != %d",
// 					j, i,
// 					msg.(*multiplexTestMessage).value,
// 					expected)
// 			}
// 		}
// 	}

// 	for i = 0; i < (10 * 2); i++ {
// 		err = connAs[i].Close()
// 		if err != nil {
// 			t.Fatalf("failed to Close A[%d]", i)
// 		}
// 	}

// 	mplexA.Close()
// 	mplexB.Close()

// 	time.Sleep(10 * time.Millisecond)
// }



// type testConnectionWrapper struct {
// 	inner Connection
// 	deliverc chan struct{}
// }

// func newTestConnectionWrapper(inner Connection, n int) *testConnectionWrapper {
// 	var this testConnectionWrapper

// 	this.inner = inner
// 	this.deliverc = make(chan struct{}, n)

// 	return &this
// }

// func (this *testConnectionWrapper) deliver(n int) {
// 	for n > 0 {
// 		this.deliverc <- struct{}{}
// 		n -= 1
// 	}
// }

// func (this *testConnectionWrapper) deliverForever() {
// 	close(this.deliverc)
// }

// func (this *testConnectionWrapper) Send(msg Message, proto Protocol) error {
// 	return this.inner.Send(msg, proto)
// }

// func (this *testConnectionWrapper) Recv(proto Protocol) (Message, error) {
// 	<-this.deliverc
// 	return this.inner.Recv(proto)
// }

// func (this *testConnectionWrapper) Close() error {
// 	return this.inner.Close()
// }



// func TestMultiplexFifoConcurrentSendClose(t *testing.T) {
// 	var connX, connY *testConnectionWrapper
// 	var readerA, readerB *io.PipeReader
// 	var writerA, writerB *io.PipeWriter
// 	var mplexX, mplexY Multiplexer
// 	var mconnX, mconnY Connection
// 	var doneX, doneY chan struct{}
// 	var connc chan Connection
// 	var err error

// 	readerA, writerA = io.Pipe()
// 	readerB, writerB = io.Pipe()
// 	connX = newTestConnectionWrapper(NewIoConnection(readerA, writerB), 64)
// 	connY = newTestConnectionWrapper(NewIoConnection(readerB, writerA), 64)

// 	mplexY = NewFifoMultiplexer(connY)
// 	connc = make(chan Connection)
// 	go func () {
// 		var conn Connection
// 		var err error

// 		conn, err = mplexY.Accept()
// 		if err != nil {
// 			t.Fatalf("accept: %v", err)
// 		}

// 		connc <- conn
// 		close(connc)
// 	}()

// 	connY.deliver(1)

// 	mplexX = NewFifoMultiplexer(connX)
// 	mconnX, err = mplexX.Connect()
// 	if err != nil {
// 		t.Fatalf("connect: %v", err)
// 	}

// 	mconnY = <-connc

// 	// ---

// 	doneX = make(chan struct{})
// 	doneY = make(chan struct{})

// 	go func () {
// 		var err error

// 		err = mconnY.Close()
// 		if err != nil {
// 			t.Fatalf("close conn: %v", err)
// 		}

// 		close(doneY)
// 	}()

// 	go func () {
// 		var err error

// 		err = mconnX.Send(&multiplexTestMessage{ 0 },
// 			multiplexTestProtocol)
// 		if err != nil {
// 			t.Fatalf("send: %v", err)
// 		}
		
// 		err = mplexX.Close()
// 		if err != nil {
// 			t.Fatalf("close: %v", err)
// 		}

// 		close(doneX)
// 	}()

// 	// X.conn send
// 	// X.mplex close
// 	// Y.conn close
// 	time.Sleep(10 * time.Millisecond)
// 	connY.deliver(1)   // Y.mplex receive {X.conn send}

// 	time.Sleep(10 * time.Millisecond)
// 	connX.deliverForever()   // X.mplex receive {Y.conn close}

// 	time.Sleep(10 * time.Millisecond)
// 	connY.deliverForever()

// 	select {
// 	case <-doneY:
// 	case <-time.After(100 * time.Millisecond):
// 		t.Fatalf("stalled on Y")
// 	}

// 	select {
// 	case <-doneX:
// 	case <-time.After(100 * time.Millisecond):
// 		t.Fatalf("stalled on X")
// 	}
// }
