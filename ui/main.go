package main


import (
	sio "silk/io"
	"silk/net"
	"sync"
	"time"
)


var mainProtocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	0: &net.RoutingMessage{},
	1: &mainMessage{},
})


type mainMessage struct {
}

func (this *mainMessage) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *mainMessage) Decode(source sio.Source) error {
	return source.Error()
}


func relay(addr string, log sio.Logger) {
	var nchan net.ServerMessageChannel
	var cmsg net.ConnectionMessage
	var routing net.RoutingService
	var resolver net.Resolver
	var network net.Server
	var wg sync.WaitGroup
	var err error

	network, err = net.NewTcpServer(addr)
	if err != nil {
		panic(err)
	}

	log.Info("running")

	resolver = net.NewTcpResolverWith(mainProtocol,
		&net.TcpResolverOptions{
			Log: log.WithLocalContext("resolve"),
		})

	routing = net.NewRoutingServiceWith(resolver,
		&net.RoutingServiceOptions{
			Log: log.WithLocalContext("route"),
		})

	nchan = net.NewServerMessageChannel(network, mainProtocol)

	for cmsg = range nchan.Accept() {
		log.Trace("accept")

		wg.Add(1)
		go func (msg *net.RoutingMessage, conn net.Connection) {
			routing.Handle(msg, conn)
			wg.Done()
		}(cmsg.Message().(*net.RoutingMessage), cmsg.Connection())
	}

	if nchan.Err() != nil {
		panic(nchan.Err())
	}

	wg.Wait()

	log.Info("terminate")
}


func main() {
	var log sio.Logger = sio.NewStderrLogger(sio.LOG_TRACE)
	var nchan, rchan net.ServerMessageChannel
	var routing net.RoutingService
	var resolver net.Resolver
	var conn net.Connection
	var network net.Server
	var route net.Route
	var err error

	resolver = net.NewTcpResolver(mainProtocol)
	routing = net.NewRoutingServiceWith(resolver,
		&net.RoutingServiceOptions{
			Log: log.WithGlobalContext("3200"),
		})
	network, err = net.NewTcpServer(":3200")
	if err != nil {
		panic(err)
	}

	nchan = net.NewServerMessageChannel(network, mainProtocol)
	rchan = net.NewServerMessageChannel(routing, mainProtocol)

	go func () {
		var cmsg net.ConnectionMessage

		for {
			select {
			case cmsg = <-nchan.Accept():
			case cmsg = <-rchan.Accept():
			}

			log.Info("got %T:%v", cmsg.Message(), cmsg.Message())

			switch m := cmsg.Message().(type) {
			case *net.RoutingMessage:
				go routing.Handle(m, cmsg.Connection())
			case *mainMessage:
				cmsg.Connection().Send(m, mainProtocol)
				cmsg.Connection().Close()
			}
		}
	}()

	go relay(":3201", log.WithGlobalContext("3201"))

	time.Sleep(10 * time.Millisecond)

	route, err = net.NewRouteWith([]string{ "localhost:3201", "localhost:3200" }, resolver, &net.RouteOptions{ log.WithGlobalContext("0000").WithLocalContext("route") })
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Millisecond)

	err = route.Send(&mainMessage{}, mainProtocol)
	if err != nil {
		panic(err)
	}

	time.Sleep(10 * time.Millisecond)

	conn, err = route.Accept()
	if err != nil {
		panic(err)
	}

	_, err = conn.Recv(mainProtocol)
	if err != nil {
		panic(err)
	}

	route.Close()
}
