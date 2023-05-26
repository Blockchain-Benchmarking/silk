package net


import (
	"context"
	"fmt"
	"silk/util/test/goleak"
	"testing"
)


// ----------------------------------------------------------------------------


func TestTcpServerEmptyAddr(t *testing.T) {
	var s Accepter = NewTcpServer("")
	var more bool

	defer goleak.VerifyNone(t)

	_, more = <-s.Accept()

	if more {
		t.Fatal("Accept")
	}
}

func TestTcpServerAlreadyUsedPort(t *testing.T) {
	var cancel func ()
	var port uint16
	var s Accepter
	var more bool

	defer goleak.VerifyNone(t)

	port, cancel = findUsedTcpPort(t)
	defer cancel()

	s = NewTcpServer(fmt.Sprintf(":%d", port))

	_, more = <-s.Accept()

	if more {
		t.Fatal("Accept")
	}
}

func TestTcpServerPort(t *testing.T) {
	testCloseAccepter(t, func () *closeAccepterTestSetup {
		var cancel context.CancelFunc
		var ctx context.Context
		var port uint16
		var addr string
		var s Accepter

		port = findTcpPort(t)
		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf(":%d", port),
			&TcpServerOptions{ Context: ctx })
		addr = fmt.Sprintf("localhost:%d", port)

		return &closeAccepterTestSetup{
			accepterTestSetup: accepterTestSetup{
				accepter: s,
				connectf: func () Connection {
					return NewTcpConnection(addr)
				},
				teardown: cancel,
			},
			closef: cancel,
		}
	})
}

func TestTcpServerAddr(t *testing.T) {
	testAccepter(t, func () *accepterTestSetup {
		var cancel context.CancelFunc
		var ctx context.Context
		var port uint16
		var addr string
		var s Accepter

		port = findTcpPort(t)
		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf("localhost:%d", port),
			&TcpServerOptions{ Context: ctx })
		addr = fmt.Sprintf("localhost:%d", port)

		return &accepterTestSetup{
			accepter: s,
			connectf: func () Connection {
				return NewTcpConnection(addr)
			},
			teardown: cancel,
		}
	})
}
