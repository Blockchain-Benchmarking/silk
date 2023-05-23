package net


import (
	"context"
	"silk/util/test/goleak"
	"testing"
)


// ----------------------------------------------------------------------------


type receiverTestSetup struct {
	receiver Receiver
	sendc chan<- MessageProtocol
	teardown func ()
}


// ----------------------------------------------------------------------------


func testReceiver(t *testing.T, setupf func () *receiverTestSetup) {
	t.Logf("testReceiverCloseImmediately")
	testReceiverCloseImmediately(t, setupf())

	t.Logf("testReceiverAsync")
	testReceiverAsync(t, setupf())

	t.Logf("testReceiverSync")
	testReceiverSync(t, setupf())

	t.Logf("testReceiverEncodingError")
	testReceiverEncodingError(t, setupf())

	t.Logf("testReceiverDecodingError")
	testReceiverDecodingError(t, setupf())
}

func testReceiverCloseImmediately(t *testing.T, setup *receiverTestSetup) {
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}
	if len(out) != 0 {
		t.Errorf("received %d unexpected messages: %v", len(out), out)
	}
}

func testReceiverAsync(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetEquality(in, out, t)
}

func testReceiverSync(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message = make([]Message, 0)
	var cancel context.CancelFunc
	var ctx context.Context
	var msg Message
	var more bool
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	loop: for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}

		select {
		case msg, more = <-setup.receiver.RecvN(mockProtocol, 1):
			if more == false {
				t.Errorf("closed unexpectedly")
				break loop
			} else {
				out = append(out, msg)
			}
		case <-ctx.Done():
			t.Errorf("timeout")
			break loop
		}
	}

	close(setup.sendc)

	_, more = <-setup.receiver.RecvN(mockProtocol, 1)
	if more {
		t.Errorf("unexpected message")
	}

	testMessagesEquality(in, out, t)
}

func testReceiverEncodingError(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	in[70].encodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetInclusive(in[:70], out, t)
}

func testReceiverDecodingError(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	in[70].decodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetInclusive(in[:70], out, t)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func testFifoReceiver(t *testing.T, setupf func () *receiverTestSetup) {
	t.Logf("testReceiverCloseImmediately")
	testReceiverCloseImmediately(t, setupf())

	t.Logf("testFifoReceiverAsync")
	testFifoReceiverAsync(t, setupf())

	t.Logf("testReceiverSync")
	testReceiverSync(t, setupf())

	t.Logf("testFifoReceiverEncodingError")
	testFifoReceiverEncodingError(t, setupf())

	t.Logf("testFifoReceiverDecodingError")
	testFifoReceiverDecodingError(t, setupf())
}

func testFifoReceiverAsync(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in, out, t)
}

func testFifoReceiverEncodingError(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	in[70].encodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in[:70], out, t)
}

func testFifoReceiverDecodingError(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), TIMEOUT)
	defer cancel()

	in[70].decodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), ctx.Done())

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in[:70], out, t)
}


// ----------------------------------------------------------------------------


func TestReceiverAggregatorEmpty(t *testing.T) {
	var r Receiver
	var more bool

	defer goleak.VerifyNone(t)

	r = NewSliceReceiverAggregator([]ReceiverProtocol{})

	_, more = <-r.Recv(mockProtocol)
	if more {
		t.Errorf("Recv")
	}

	_, more = <-r.RecvN(mockProtocol, 1)
	if more {
		t.Errorf("RecvN")
	}
}

func TestReceiverAggregatorOne(t *testing.T) {
	testFifoReceiver(t, func () *receiverTestSetup {
		var addr string = findTcpAddr(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c, a Connection
		var s Accepter

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(addr)
		a = <-s.Accept()

		return &receiverTestSetup{
			receiver: NewSliceReceiverAggregator(
				[]ReceiverProtocol{ ReceiverProtocol{
					R: c,
					P: mockProtocol,
				}}),
			sendc: a.Send(),
			teardown: func () {
				cancel()
				close(c.Send())
			},
		}
	})
}

func TestReceiverAggregatorMany(t *testing.T) {
	testReceiver(t, func () *receiverTestSetup {
		var addr string = findTcpAddr(t)
		var sendc chan MessageProtocol
		var cancel context.CancelFunc
		var rps []ReceiverProtocol
		var ctx context.Context
		var cs, as []Connection
		var c Connection
		var s Accepter
		var seq []int
		var i int

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })

		for i = 0; i < 10; i++ {
			c = NewTcpConnection(addr)
			rps = append(rps, ReceiverProtocol{ c, mockProtocol })
			cs = append(cs, c)
			as = append(as, <-s.Accept())
		}

		seq = []int{ 0, 0, 1, 2, 7, 3, 4, 9, 9, 0, 1, 8 }
		sendc = make(chan MessageProtocol, 128)
		go func () {
			var mp MessageProtocol
			var n int = 0

			for mp = range sendc {
				as[seq[n]].Send() <- mp
				n = (n + 1) % len(seq)
			}

			for n = range as {
				close(as[n].Send())
			}
		}()

		return &receiverTestSetup{
			receiver: NewSliceReceiverAggregator(rps),
			sendc: sendc,
			teardown: func () {
				cancel()
				for _, c = range cs {
					close(c.Send())
				}
			},
		}
	})
}
