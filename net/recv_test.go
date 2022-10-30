package net


import (
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
	var outc <-chan []Message
	var out []Message

	defer setup.teardown()

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), timeout(1))

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
	var outc <-chan []Message
	var out []Message
	var i int

	defer setup.teardown()

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), timeout(100))

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

func testReceiverSync(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var over <-chan struct{} = timeout(100)
	var out []Message = make([]Message, 0)
	var msg Message
	var more bool
	var i int

	defer setup.teardown()

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
		case <-over:
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
	var outc <-chan []Message
	var out []Message
	var i int

	in[70].encodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), timeout(100))

	for i = range in {
		setup.sendc <- MessageProtocol{in[i], mockProtocol}
	}

	close(setup.sendc)

	out = <-outc

	if out == nil {
		t.Errorf("timeout")
	}

	testMessagesEquality(in[:70], out, t)

	setup.teardown()
}

func testReceiverDecodingError(t *testing.T, setup *receiverTestSetup) {
	var in []*mockMessage = generateLinearShallowMessages(100, 1 << 21)
	var outc <-chan []Message
	var out []Message
	var i int

	defer setup.teardown()

	in[70].decodingError = true

	outc = gatherMessages(setup.receiver.Recv(mockProtocol), timeout(100))

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
