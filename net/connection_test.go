package net


import (
	"context"
	"silk/util/test/goleak"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


type connectionTestSetup struct {
	left Connection
	right Connection
	teardown func ()
}


// ----------------------------------------------------------------------------


func testConnection(t *testing.T, setupf func () *connectionTestSetup) {
	t.Logf("testConnectionSender")
	testConnectionSender(t, setupf)

	t.Logf("testConnectionReceiver")
	testConnectionReceiver(t, setupf)
}

func testConnectionSender(t *testing.T, setupf func () *connectionTestSetup) {
	testFifoSender(t, func () *senderTestSetup {
		var setup *connectionTestSetup = setupf()

		return &senderTestSetup{
			sender: setup.left,
			recvc: setup.right.Recv(mockProtocol),
			teardown: func () {
				close(setup.right.Send())
				setup.teardown()
			},
		}
	})
}

func testConnectionReceiver(t *testing.T, setupf func () *connectionTestSetup){
	testFifoReceiver(t, func () *receiverTestSetup {
		var setup *connectionTestSetup = setupf()

		return &receiverTestSetup{
			receiver: newTrivialReceiver(setup.left.Recv(
				mockProtocol)),
			sendc: setup.right.Send(),
			teardown: func () {
				close(setup.left.Send())
				setup.teardown()
			},
		}
	})
}


// ----------------------------------------------------------------------------


func TestTcpConnectionEmptyAddr(t *testing.T) {
	var c Connection
	var more bool

	defer goleak.VerifyNone(t)

	c = NewTcpConnection("")

	c.Send() <- MessageProtocol{ &mockMessage{}, mockProtocol }

	_, more = <-c.Recv(mockProtocol)
	if more {
		t.Fatal("Recv")
	}

	_, more = <-c.RecvN(mockProtocol, 10)
	if more {
		t.Fatal("RecvN")
	}

	close(c.Send())
}

func TestTcpConnectionUnreachable(t *testing.T) {
	var c Connection
	var more bool

	defer goleak.VerifyNone(t)

	c = NewTcpConnection(findTcpAddr(t))

	c.Send() <- MessageProtocol{ &mockMessage{}, mockProtocol }

	_, more = <-c.Recv(mockProtocol)
	if more {
		t.Fatal("Recv")
	}

	_, more = <-c.RecvN(mockProtocol, 10)
	if more {
		t.Fatal("RecvN")
	}

	close(c.Send())
}

func TestTcpConnectionUnresolvable(t *testing.T) {
	var cancel context.CancelFunc
	var ctx context.Context
	var c Connection

	defer goleak.VerifyNone(t)

	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	c = NewTcpConnectionWith(findUnresolvableAddr(t),&TcpConnectionOptions{
		ConnectionContext: ctx,
	})

	close(c.Send())
}

func TestTcpConnection(t *testing.T) {
	testConnection(t, func () *connectionTestSetup {
		var addr string = findTcpAddr(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c, a Connection
		var s Accepter

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(addr)
		a = <-s.Accept()

		return &connectionTestSetup{
			left: c,
			right: a,
			teardown: cancel,
		}
	})
}
