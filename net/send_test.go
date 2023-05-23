package net


import (
	"context"
	"go.uber.org/goleak"
	"sync"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


type senderTestSetup struct {
	sender Sender
	recvc <-chan Message
	teardown func ()
}


// ----------------------------------------------------------------------------


func testSender(t *testing.T, setupf func () *senderTestSetup) {
	t.Logf("testSenderCloseImmediately")
	testSenderCloseImmediately(t, setupf())

	t.Logf("testSenderAsync")
	testSenderAsync(t, setupf())

	t.Logf("testSenderSync")
	testSenderSync(t, setupf())

	t.Logf("testSenderEncodingError")
	testSenderEncodingError(t, setupf())

	t.Logf("testSenderDecodingError")
	testSenderDecodingError(t, setupf())
}

func testSenderCloseImmediately(t *testing.T, setup *senderTestSetup) {
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}
	if len(out) != 0 {
		t.Errorf("received %d unexpected messages: %v", len(out), out)
	}

}

func testSenderAsync(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetEquality(in, out, t)
}

func testSenderSync(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message = make([]Message, 0)
	var cancel context.CancelFunc
	var ctx context.Context
	var msg Message
	var more bool
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	loop: for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}

		select {
		case msg, more = <-setup.recvc:
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

	close(setup.sender.Send())

	testMessagesEquality(in, out, t)
}

func testSenderEncodingError(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	in[70].encodingError = true

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetInclusive(in[:70], out, t)
}

func testSenderDecodingError(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	in[70].decodingError = true

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessageSetInclusive(in[:70], out, t)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func testFifoSender(t *testing.T, setupf func () *senderTestSetup) {
	t.Logf("testSenderCloseImmediately")
	testSenderCloseImmediately(t, setupf())

	t.Logf("testFifoSenderAsync")
	testFifoSenderAsync(t, setupf())

	t.Logf("testSenderSync")
	testSenderSync(t, setupf())

	t.Logf("testFifoSenderEncodingError")
	testFifoSenderEncodingError(t, setupf())

	t.Logf("testFifoSenderDecodingError")
	testFifoSenderDecodingError(t, setupf())
}

func testFifoSenderAsync(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in, out, t)
}

func testFifoSenderEncodingError(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	in[70].encodingError = true

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in[:70], out, t)
}

func testFifoSenderDecodingError(t *testing.T, setup *senderTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var cancel context.CancelFunc
	var outc <-chan []Message
	var ctx context.Context
	var out []Message
	var i int

	defer goleak.VerifyNone(t)
	defer setup.teardown()

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	outc = gatherMessages(setup.recvc, ctx.Done())

	in[70].decodingError = true

	for i = range in {
		setup.sender.Send() <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sender.Send())

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in[:70], out, t)
}


// ----------------------------------------------------------------------------


func TestSenderAggregatorEmpty(t *testing.T) {
	var s Sender

	defer goleak.VerifyNone(t)

	s = NewSliceSenderAggregator([]Sender{})

	close(s.Send())
}

func TestSenderAggregatorOne(t *testing.T) {
	testFifoSender(t, func () *senderTestSetup {
		var addr string = findTcpAddr(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c, a Connection
		var s Accepter

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(addr)
		a = <-s.Accept()

		return &senderTestSetup{
			sender: NewSliceSenderAggregator([]Sender{ c }),
			recvc: a.Recv(mockProtocol),
			teardown: func () {
				cancel()
				close(a.Send())
			},
		}
	})
}

func TestSenderAggregatorMany(t *testing.T) {
	testSender(t, func () *senderTestSetup {
		var addr string = findTcpAddr(t)
		var cancel context.CancelFunc
		var receiving sync.WaitGroup
		var ctx context.Context
		var recvc chan Message
		var as []Connection
		var a Connection
		var ss []Sender
		var s Accepter
		var i int

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		recvc = make(chan Message, 128)

		for i = 0; i < 10; i++ {
			ss = append(ss, NewTcpConnection(addr))

			a = <-s.Accept()
			as = append(as, a)

			receiving.Add(1)

			go func (a Connection) {
				var msg Message

				for msg = range a.Recv(mockProtocol) {
					recvc <- msg
				}

				receiving.Done()
			}(a)
		}

		go func () {
			receiving.Wait()
			close(recvc)
		}()

		return &senderTestSetup{
			sender: NewSliceSenderAggregator(ss),
			recvc: recvc,
			teardown: func () {
				cancel()
				for _, a = range as {
					close(a.Send())
				}
			},
		}
	})
}
