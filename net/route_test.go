package net


import (
	"context"
	"fmt"
	"sync"
	"testing"
)


// ----------------------------------------------------------------------------


type routeTestSetup struct {
	route Route
	conns []Connection
	teardown func ()
}


type trivialReceiver struct {
	c <-chan Message
}

func newTrivialReceiver(c <-chan Message) *trivialReceiver {
	return &trivialReceiver{ c }
}

func (this *trivialReceiver) decode(d chan<- Message, n int) {
	var m Message
	var more bool

	for n != 0 {
		m, more = <-this.c
		if more == false {
			break
		}

		d <- m

		if n > 0 {
			n -= 1
		}
	}

	close(d)
}

func (this *trivialReceiver) Recv(proto Protocol) <-chan Message {
	var d chan Message = make(chan Message)
	go this.decode(d, -1)
	return d
}

func (this *trivialReceiver) RecvN(proto Protocol, n int) <-chan Message {
	var d chan Message = make(chan Message)
	go this.decode(d, n)
	return d
}


func recvEqMocks(mcs []<-chan *mockMessage) <-chan *mockMessage {
	var mc chan *mockMessage = make(chan *mockMessage)

	go func () {
		var ms []*mockMessage
		var more, ok bool
		var i int

		loop: for {
			ms = make([]*mockMessage, len(mcs))

			for i = range mcs {
				ms[i], more = <-mcs[i]
				if more == false {
					break loop
				}
			}

			ok, _ = cmpAllMessages(ms)
			if ok == false {
				break
			}

			mc <- ms[0]
		}

		close(mc)

		for i = range mcs { for _ = range mcs[i] {} }

	}()

	return mc
}

func recvAllMocks(mcs []<-chan *mockMessage) <-chan *mockMessage {
	var mc chan *mockMessage = make(chan *mockMessage)
	var running sync.WaitGroup
	var i int

	running.Add(len(mcs))
	go func () {
		running.Wait()
		close(mc)
	}()

	for i = range mcs {
		go func (index int) {
			var m *mockMessage

			for m = range mcs[index] {
				mc <- m
			}

			running.Done()
		}(i)
	}

	return mc
}


// ----------------------------------------------------------------------------


func testRoute(t *testing.T, setupf func () *routeTestSetup) {
	t.Logf("testRouteAccepter")
	testRouteAccepter(t, setupf)

	t.Logf("testRouteSender")
	testRouteSender(t, setupf)

	t.Logf("testRouteConnectionSender")
	testRouteConnectionSender(t, setupf)

	t.Logf("testRouteReceiver")
	testRouteReceiver(t, setupf)

	t.Logf("testRouteConnectionReceiver")
	testRouteConnectionReceiver(t, setupf)
}

// func testPartialRoute(t *testing.T, setupf func () *routeTestSetup) {
// 	t.Logf("testRouteAccepter")
// 	testRouteAccepter(t, setupf)

// 	// t.Logf("testRouteSender")
// 	// testRouteSender(t, setupf)

// 	// t.Logf("testRouteConnectionSender")
// 	// testRouteConnectionSender(t, setupf)

// 	// t.Logf("testRouteReceiver")
// 	// testRouteReceiver(t, setupf)

// 	// t.Logf("testRouteConnectionReceiver")
// 	// testRouteConnectionReceiver(t, setupf)
// }

func testRouteAccepter(t *testing.T, setupf func() *routeTestSetup) {
	testAccepter(t, func () *accepterTestSetup {
		var setup *routeTestSetup = setupf()
		var i int = 0

		return &accepterTestSetup{
			accepter: setup.route,
			connectf: func () Connection {
				if i < len(setup.conns) {
					i += 1
					return setup.conns[i - 1]
				} else {
					return nil
				}
			},
			teardown: func () {
				setup.teardown()

				close(setup.route.Send())

				for i < len(setup.conns) {
					close(setup.conns[i].Send())
					i += 1
				}
			},
		}
	})
}

func testRouteSender(t *testing.T, setupf func () *routeTestSetup) {
	testSender(t, func () *senderTestSetup {
		var setup *routeTestSetup = setupf()
		var mcs []<-chan *mockMessage
		var conn Connection
		var i int

		for conn = range setup.route.Accept() {
			close(conn.Send())
		}

		mcs = make([]<-chan *mockMessage, len(setup.conns))
		for i = range setup.conns {
			mcs[i] = recvMock(setup.conns[i].Recv(mockProtocol))
		}

		return &senderTestSetup{
			sender: setup.route,
			recvc: recvMessage(recvEqMocks(mcs)),
			teardown: func () {
				setup.teardown()
				for i = range setup.conns {
					close(setup.conns[i].Send())
				}
			},
		}
	})
}

func testRouteConnectionSender(t *testing.T, setupf func () *routeTestSetup) {
	testSender(t, func () *senderTestSetup {
		var setup *routeTestSetup = setupf()
		var mcs []<-chan *mockMessage
		var sconn, conn Connection
		var i int

		sconn = <-setup.route.Accept()
		for conn = range setup.route.Accept() {
			close(conn.Send())
		}

		close(setup.route.Send())

		mcs = make([]<-chan *mockMessage, len(setup.conns))
		for i = range setup.conns {
			mcs[i] = recvMock(setup.conns[i].Recv(mockProtocol))
		}

		return &senderTestSetup{
			sender: sconn,
			recvc: recvMessage(recvAllMocks(mcs)),
			teardown: func () {
				setup.teardown()
				for i = range setup.conns {
					close(setup.conns[i].Send())
				}
			},
		}
	})
}

func testRouteReceiver(t *testing.T, setupf func () *routeTestSetup) {
	testReceiver(t, func () *receiverTestSetup {
		var setup *routeTestSetup = setupf()
		var mcs []<-chan *mockMessage
		var conn Connection
		var recv Receiver
		var i int

		for conn = range setup.route.Accept() {
			close(conn.Send())
		}

		mcs = make([]<-chan *mockMessage, len(setup.conns))
		for i = range setup.conns {
			mcs[i] = recvMock(setup.conns[i].Recv(mockProtocol))
		}

		recv = newTrivialReceiver(recvMessage(recvEqMocks(mcs)))

		return &receiverTestSetup{
			receiver: recv,
			sendc: setup.route.Send(),
			teardown: func () {
				setup.teardown()
				for i = range setup.conns {
					close(setup.conns[i].Send())
				}
			},
		}
	})
}

func testRouteConnectionReceiver(t *testing.T, setupf func () *routeTestSetup){
	testReceiver(t, func () *receiverTestSetup {
		var setup *routeTestSetup = setupf()
		var mcs []<-chan *mockMessage
		var sconn, conn Connection
		var recv Receiver
		var i int

		sconn = <-setup.route.Accept()
		for conn = range setup.route.Accept() {
			close(conn.Send())
		}

		close(setup.route.Send())

		mcs = make([]<-chan *mockMessage, len(setup.conns))
		for i = range setup.conns {
			mcs[i] = recvMock(setup.conns[i].Recv(mockProtocol))
		}

		recv = newTrivialReceiver(recvMessage(recvAllMocks(mcs)))

		return &receiverTestSetup{
			receiver: recv,
			sendc: sconn.Send(),
			teardown: func () {
				setup.teardown()
				for i = range setup.conns {
					close(setup.conns[i].Send())
				}
			},
		}
	})
}


// ----------------------------------------------------------------------------


func TestLeafRouteEmpty(t *testing.T) {
	t.Logf("testRouteAccepter")
	testRouteAccepter(t, func () *routeTestSetup {
		var c chan Connection = make(chan Connection) ; close(c)
		var r Route = NewLeafRoute(c)

		return &routeTestSetup{
			route: r,
			conns: []Connection{},
			teardown: func () {},
		}
	})
}

func TestUnitLeafRoute(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var c Connection
		var s Accepter
		var r Route

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf(":%d", port),
			&TcpServerOptions{ Context: ctx })
		c = NewTcpConnection(fmt.Sprintf("localhost:%d", port))
		r = NewUnitLeafRoute(c)

		return &routeTestSetup{
			route: r,
			conns: []Connection{ <-s.Accept() },
			teardown: cancel,
		}
	})
}

func TestSliceLeafRoute(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		const NUM_CONNECTIONS = 32   // too many would trigger timeouts
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var cs, as []Connection
		var s Accepter
		var r Route
		var i int

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf(":%d", port),
			&TcpServerOptions{ Context: ctx })

		for i = 0; i < NUM_CONNECTIONS; i++ {
			cs = append(cs, NewTcpConnection(
				fmt.Sprintf("localhost:%d", port)))
			as = append(as, <-s.Accept())
		}

		r = NewSliceLeafRoute(cs)

		return &routeTestSetup{
			route: r,
			conns: as,
			teardown: cancel,
		}
	})
}

// func TestSliceLeafRouteUnreachable(t *testing.T) {
// 	testPartialRoute(t, func () *routeTestSetup {
// 		const NUM_CONNECTIONS = 32   // too many would trigger timeouts
// 		const NUM_UNREACHABLE = 8
// 		var cancel context.CancelFunc
// 		var ctx context.Context
// 		var cs, as []Connection
// 		var p0, p1 uint16
// 		var s Accepter
// 		var r Route
// 		var i int

// 		p0  = findTcpPort(t)
// 		p1  = findTcpPort(t)

// 		ctx, cancel = context.WithCancel(context.Background())
// 		s = NewTcpServerWith(fmt.Sprintf(":%d", p0),
// 			&TcpServerOptions{ Context: ctx })

// 		for i = 0; i < NUM_CONNECTIONS; i++ {
// 			cs = append(cs, NewTcpConnection(
// 				fmt.Sprintf("localhost:%d", p0)))
// 			as = append(as, <-s.Accept())
// 		}

// 		for i = 0; i < NUM_UNREACHABLE; i++ {
// 			cs = append(cs, NewTcpConnection(
// 				fmt.Sprintf("localhost:%d", p1)))
// 		}

// 		r = NewSliceLeafRoute(cs)

// 		return &routeTestSetup{
// 			route: r,
// 			conns: as,
// 			teardown: cancel,
// 		}
// 	})
// }


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func TestCompositeRouteEmpty(t *testing.T) {
	t.Logf("testRouteAccepter")
	testRouteAccepter(t, func () *routeTestSetup {
		var c chan Route = make(chan Route) ; close(c)
		var r Route = NewCompositeRoute(c)

		return &routeTestSetup{
			route: r,
			conns: []Connection{},
			teardown: func () {},
		}
	})
}

func TestSliceCompositeRoute(t *testing.T) {
	testRoute(t, func () *routeTestSetup {
		const NUM_EMPTY_ROUTES = 2
		const NUM_2_ROUTES = 10
		const NUM_5_ROUTES = 2
		var port uint16 = findTcpPort(t)
		var cancel context.CancelFunc
		var ctx context.Context
		var cs, as []Connection
		var rs []Route
		var s Accepter
		var i, j int
		var r Route

		ctx, cancel = context.WithCancel(context.Background())
		s = NewTcpServerWith(fmt.Sprintf(":%d", port),
			&TcpServerOptions{ Context: ctx })

		for i = 0; i < NUM_EMPTY_ROUTES; i++ {
			rs = append(rs, NewSliceLeafRoute([]Connection{}))
		}

		for i = 0; i < NUM_2_ROUTES; i++ {
			cs = []Connection{}
			for j = 0; j < 2; j++ {
				cs = append(cs, NewTcpConnection(
					fmt.Sprintf("localhost:%d", port)))
				as = append(as, <-s.Accept())
			}
			rs = append(rs, NewSliceLeafRoute(cs))
		}

		for i = 0; i < NUM_5_ROUTES; i++ {
			cs = []Connection{}
			for j = 0; j < 5; j++ {
				cs = append(cs, NewTcpConnection(
					fmt.Sprintf("localhost:%d", port)))
				as = append(as, <-s.Accept())
			}
			rs = append(rs, NewSliceLeafRoute(cs))
		}

		r = NewSliceCompositeRoute(rs)

		return &routeTestSetup{
			route: r,
			conns: as,
			teardown: cancel,
		}
	})
}
