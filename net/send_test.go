package net


import (
	"context"
	"sync"
	"testing"
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
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(1))
	var out []Message

	defer setup.teardown()

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
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

	defer setup.teardown()

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
	var over <-chan struct{} = timeout(100)
	var out []Message = make([]Message, 0)
	var msg Message
	var more bool
	var i int

	defer setup.teardown()

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
		case <-over:
			t.Errorf("timeout")
			break loop
		}
	}

	close(setup.sender.Send())

	testMessagesEquality(in, out, t)
}

func testSenderEncodingError(t *testing.T, setup *senderTestSetup) {
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

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

	setup.teardown()
}

func testSenderDecodingError(t *testing.T, setup *senderTestSetup) {
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

	defer setup.teardown()

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
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

	defer setup.teardown()

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
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

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

	setup.teardown()
}

func testFifoSenderDecodingError(t *testing.T, setup *senderTestSetup) {
	var outc <-chan []Message = gatherMessages(setup.recvc, timeout(100))
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var out []Message
	var i int

	defer setup.teardown()

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
	var s Sender = NewSliceSenderAggregator([]Sender{})

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
