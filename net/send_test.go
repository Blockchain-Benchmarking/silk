package net


import (
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
