package main


import (
	"fmt"
	"math"
	"os"
	sio "silk/io"
	"silk/net"
	"silk/run"
	"silk/ui"
)


const DEFAULT_TCP_PORT = 3200


// ----------------------------------------------------------------------------


func serverUsage() {
	fmt.Printf(`Usage: %s server [-n<str> | --name=<str>] [--tcp[=<int>]]

Launch a server.
By default, the server listen for connections on the tcp port %d and run in
foreground.

Options:

  -n<str>, --name=<str>       Launch the server with the given <str> name.

  --tcp[=<int>]               Tell the server to listen for tcp connections.
                              If <int> is specified then listen on this tcp
                              port.

`, os.Args[0], DEFAULT_TCP_PORT)

	os.Exit(0)
}


// ----------------------------------------------------------------------------


var protocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	0: &net.RoutingMessage{},
	1: &run.Message{},
})


func handle(c net.Connection, routing net.RoutingService, running run.Service){
	var msg net.Message
	var more bool

	msg, more = <-c.RecvN(protocol, 1)
	if more == false {
		close(c.Send())
		return
	}

	switch m := msg.(type) {
	case *net.RoutingMessage:
		routing.Handle(m, c)
	case *run.Message:
		running.Handle(m, c)
	}
}

func serve(a net.Accepter, routing net.RoutingService, running run.Service) {
	var conn net.Connection

	for conn = range a.Accept() {
		go handle(conn, routing, running)
	}
}

func serverStart(port int, name string, log sio.Logger) {
	var routingService net.RoutingService
	var runService run.Service
	var resolver net.Resolver
	var tcp net.Accepter
	var err error

	tcp = net.NewTcpServer(fmt.Sprintf(":%d", port))

	resolver = net.NewGroupResolver(net.NewTcpResolverWith(protocol,
		&net.TcpResolverOptions{
			Log: log.WithLocalContext("resolve"),
		}))

	routingService = net.NewRoutingServiceWith(resolver,
		&net.RoutingServiceOptions{
			Log: log.WithLocalContext("route"),
		})

	runService, err = run.NewServiceWith(&run.ServiceOptions{
		Log: log.WithLocalContext("run"),
		Name: name,
	})

	if err != nil {
		fatale(err)
	}

	log.Info("start")

	go serve(tcp, routingService, runService)
	go serve(routingService, routingService, runService)

	var c chan struct{} = nil
	<-c  // fucking yolo
}


// ----------------------------------------------------------------------------


func serverMain(cli ui.Cli, verbose *verbosity) {
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { serverUsage() ; return nil },
	}.New()
	var nameOption ui.OptionString = ui.OptString{
		ValidityPredicate: func (val string) error {
			if val == "" {
				return fmt.Errorf("name must not be empty")
			} else {
				return nil
			}
		},
	}.New()
	var tcpOption ui.OptionInt = ui.OptInt{
		DefaultValue: DEFAULT_TCP_PORT,
		ValidityPredicate: func (val int) error {
			if (val < 1) || (val > math.MaxUint16) {
				return fmt.Errorf("invalid tcp port: %d", val)
			} else {
				return nil
			}
		},
		Variadic: true,
	}.New()
	var err error
	var op string
	var ok bool

	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)
	cli.AddOption('n', "name", nameOption)
	cli.AddLongOption("tcp", tcpOption)

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	op, ok = cli.SkipWord()
	if ok {
		fatal("unexpected operand: %s", op)
	}
	
	serverStart(tcpOption.Value(), nameOption.Value(), verbose.log())
}
