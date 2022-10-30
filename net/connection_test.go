package net


import (
	"context"
	"testing"
)


// ----------------------------------------------------------------------------


func TestTcpConnectionEmptyAddr(t *testing.T) {
	var c Connection
	var more bool

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
	var c Connection

	c = NewTcpConnection(findUnresolvableAddr(t))

	close(c.Send())
}

func TestTcpConnectionSender(t *testing.T) {
	testSender(t, func () *senderTestSetup {
		var addr string = findTcpAddr(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var recvc <-chan Message
		var c, a Connection
		var s Accepter

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(addr)
		a = <-s.Accept()

		if a == nil {
			ac := make(chan Message) ; close(ac)
			recvc = ac
		} else {
			recvc = a.Recv(mockProtocol)
		}

		return &senderTestSetup{
			sender: c,
			recvc: recvc,
			teardown: func () {
				cancel()
				close(a.Send())
			},
		}
	})
}

func TestTcpConnectionReceiver(t *testing.T) {
	testReceiver(t, func () *receiverTestSetup {
		var addr string = findTcpAddr(t)
		var sendc chan<- MessageProtocol
		var cancel context.CancelFunc
		var ctx context.Context
		var c, a Connection
		var s Accepter

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(addr, &TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(addr)
		a = <-s.Accept()

		if a == nil {
			ac := make(chan MessageProtocol)
			go func () { for _ = range ac {} }()
			sendc = ac
		} else {
			sendc = a.Send()
		}

		return &receiverTestSetup{
			receiver: c,
			sendc: sendc,
			teardown: func () {
				cancel()
				close(c.Send())
			},
		}
	})
}
