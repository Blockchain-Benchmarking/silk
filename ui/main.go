package main


import (
	sio "silk/io"
	"silk/net"
	// "sync"
	"time"
)


var mainProtocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	0: &net.RoutingMessage{},
	1: &mainMessage{},
})


type mainMessage struct {
	value uint64
}

func (this *mainMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint64(this.value).Error()
}

func (this *mainMessage) Decode(source sio.Source) error {
	return source.ReadUint64(&this.value).Error()
}


// func relay(addr string, log sio.Logger) {
// 	var routing net.RoutingService
// 	var resolver net.Resolver
// 	var network net.Accepter
// 	var conn net.Connection
// 	var wg sync.WaitGroup

// 	network = net.NewTcpServer(addr)

// 	log.Info("running")

// 	resolver = net.NewTcpResolver(mainProtocol)
// 	routing = net.NewRoutingServiceWith(resolver,
// 		&net.RoutingServiceOptions{
// 			Log: log.WithLocalContext("route"),
// 		})

// 	go func () {
// 		for _ = range routing.Accept() {
// 			log.Warn("unexpected routing connection")
// 		}
// 	}()

// 	for conn = range network.Accept() {
// 		wg.Add(1)

// 		go func (conn net.Connection) {
// 			var msg net.Message

// 			log.Info("new connection")

// 			msg = <-conn.RecvN(mainProtocol, 1)

// 			log.Info("new request")

// 			if msg != nil {
// 				routing.Handle(msg.(*net.RoutingMessage), conn)
// 			}

// 			wg.Done()
// 		}(conn)
// 	}

// 	wg.Wait()

// 	log.Info("terminate")
// }


// func serve(addr string, log sio.Logger) {
// 	var routing net.RoutingService
// 	var resolver net.Resolver
// 	var network net.Accepter
// 	var conn net.Connection
// 	var wg sync.WaitGroup

// 	network = net.NewTcpServer(addr)

// 	log.Info("running")

// 	resolver = net.NewTcpResolver(mainProtocol)
// 	routing = net.NewRoutingServiceWith(resolver,
// 		&net.RoutingServiceOptions{
// 			Log: log.WithLocalContext("route"),
// 		})

// 	for conn = range net.AcceptAll(network, routing) {
// 		wg.Add(1)

// 		go func (conn net.Connection) {
// 			var msg net.Message

// 			log.Info("new connection")

// 			msg = <-conn.RecvN(mainProtocol, 1)

// 			switch m := msg.(type) {
// 			case *net.RoutingMessage:
// 				log.Info("new routing request")
// 				routing.Handle(m, conn)
// 			case *mainMessage:
// 				log.Info("new application message")
// 				conn.Send() <- net.MessageProtocol{
// 					M: m,
// 					P: mainProtocol,
// 				}
// 				close(conn.Send())
// 			}

// 			wg.Done()
// 		}(conn)
// 	}

// 	wg.Wait()

// 	log.Info("terminate")
// }

func relay(addr string, log sio.Logger) {
	var rs net.RoutingService
	var m *net.RoutingMessage
	var res net.Resolver
	var c net.Connection
	var msg net.Message
	var s net.Accepter
	var more, ok bool

	s = net.NewTcpServer(addr)

	log.Info("running")

	res = net.NewGroupResolver(net.NewTcpResolver(mainProtocol))
	rs = net.NewRoutingServiceWith(res, &net.RoutingServiceOptions{
		Log: log.WithLocalContext("route"),
	})

	for c = range s.Accept() {
		log.Info("new connection")
		
		msg, more = <-c.RecvN(mainProtocol, 1)
		if more == false {
			close(c.Send())
			continue
		}

		log.Info("new request")

		m, ok = msg.(*net.RoutingMessage)
		if ok == false {
			close(c.Send())
			continue
		}

		log.Info("routing request")

		go rs.Handle(m, c)
	}
}


func serve(addr string, log sio.Logger) {
	var rs net.RoutingService
	var m *net.RoutingMessage
	var res net.Resolver
	var c net.Connection
	var msg net.Message
	var s net.Accepter
	var more, ok bool

	s = net.NewTcpServer(addr)

	log.Info("running")

	res = net.NewTcpResolver(mainProtocol)
	rs = net.NewRoutingServiceWith(res, &net.RoutingServiceOptions{
		Log: log.WithLocalContext("route"),
	})

	go func () {
		var c net.Connection
		var msg net.Message
		var m *mainMessage
		var more, ok bool

		for c = range rs.Accept() {
			log.Info("new routed connection")

			msg, more = <-c.RecvN(mainProtocol, 1)
			if more == false {
				log.Info("routed connection closes")
				close(c.Send())
				continue
			}

			log.Info("new request %v", msg)

			m, ok = msg.(*mainMessage)
			if ok == false {
				close(c.Send())
				continue
			}

			log.Info("application request")

			c.Send() <- net.MessageProtocol{
				M: &mainMessage{ m.value + 1 },
				P: mainProtocol,
			}

			log.Info("send application reply")

			close(c.Send())
		}
	}()

	for c = range s.Accept() {
		log.Info("new connection")
		
		msg, more = <-c.RecvN(mainProtocol, 1)
		if more == false {
			close(c.Send())
			continue
		}

		log.Info("new request")

		m, ok = msg.(*net.RoutingMessage)
		if ok == false {
			close(c.Send())
			continue
		}

		log.Info("routing request")

		go rs.Handle(m, c)
	}
}


func main() {
	var log sio.Logger = sio.NewStderrLogger(sio.LOG_TRACE)
	var resolver net.Resolver
	var c0, c1 net.Connection
	var route net.Route

	go relay(":3200", log.WithGlobalContext("r:3200"))
	// go relay(":3201", log.WithGlobalContext("r:3201"))
	// go relay(":3202", log.WithGlobalContext("r:3201"))
	// go relay(":3203", log.WithGlobalContext("r:3201"))
	// go relay(":3204", log.WithGlobalContext("r:3201"))
	go serve(":3201", log.WithGlobalContext("s:3201"))
	go serve(":3202", log.WithGlobalContext("s:3202"))
	log = log.WithGlobalContext("client")

	time.Sleep(10 * time.Millisecond)

	resolver = net.NewTcpResolver(mainProtocol)
	route = net.NewRouteWith([]string{
		"localhost:3200", "localhost:3201+localhost:3202",
	}, resolver, &net.RouteOptions{
		Log: log.WithLocalContext("route"),
	})

	// route.Send() <- net.MessageProtocol{ &mainMessage{}, mainProtocol }
	// log.Info("sent")

	c0 = <-route.Accept()
	if c0 == nil {
		panic("accept0")
	}
	log.Info("accepted0")

	c1 = <-route.Accept()
	if c1 == nil {
		panic("accept1")
	}
	log.Info("accepted1")

	time.Sleep(10 * time.Millisecond)

	c0.Send() <- net.MessageProtocol{ &mainMessage{10}, mainProtocol }
	log.Info("sent0")
	c1.Send() <- net.MessageProtocol{ &mainMessage{20}, mainProtocol }
	log.Info("sent1")

	time.Sleep(10 * time.Millisecond)

	m := <-c0.RecvN(mainProtocol, 1)
	if m == nil {
		panic("receive0")
	}
	log.Info("received0 %v", m)

	m = <-c1.RecvN(mainProtocol, 1)
	if m == nil {
		panic("receive1")
	}
	log.Info("received1 %v", m)

	time.Sleep(100 * time.Millisecond)

	close(c0.Send())
	close(c1.Send())
	close(route.Send())

	time.Sleep(100 * time.Millisecond)
}
