package net


import (
	"context"
	"fmt"
	"testing"
)


// ----------------------------------------------------------------------------


func _aggregateConnections(a Accepter, as AggregationService) {
	var m *AggregationMessage
	var c Connection
	var msg Message
	var ok bool

	for c = range a.Accept() {
		msg = <-c.RecvN(mockProtocol, 1)
		if msg == nil {
			close(c.Send())
			continue
		}

		m, ok = msg.(*AggregationMessage)
		if ok == false {
			close(c.Send())
			continue
		}

		go as.Handle(m, c)
	}
}

func serveAggregation(ctx context.Context, addr string, c chan<- Connection) {
	var as AggregationService = NewAggregationService()

	go acceptConnections(as, c)

	go _aggregateConnections(NewTcpServerWith(addr, &TcpServerOptions{
		Context: ctx,
	}), as)
}


// ----------------------------------------------------------------------------


func TestAggregatedTcpConnectionOne(t *testing.T) {
	testConnection(t, func () *connectionTestSetup {
		var connc chan Connection = make(chan Connection)
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c Connection

		ctx, cancel = context.WithCancel(context.Background())
		go serveAggregation(ctx, fmt.Sprintf(":%d", port), connc)

		c = NewAggregatedTcpConnection(
			fmt.Sprintf("localhost:%d", port), mockProtocol, 1)

		return &connectionTestSetup{
			left: c,
			right: <-connc,
			teardown: cancel,
		}
	})
}

func TestAggregatedTcpConnectionMany(t *testing.T) {
	testConnection(t, func () *connectionTestSetup {
		var connc chan Connection = make(chan Connection)
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c Connection

		ctx, cancel = context.WithCancel(context.Background())
		serveAggregation(ctx, fmt.Sprintf(":%d", port), connc)

		c = NewAggregatedTcpConnection(
			fmt.Sprintf("localhost:%d", port), mockProtocol, 32)

		return &connectionTestSetup{
			left: c,
			right: <-connc,
			teardown: cancel,
		}
	})
}
