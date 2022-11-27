package main


import (
	"fmt"
	"math"
	"os"
	"os/signal"
	sio "silk/io"
	"silk/kv"
	"silk/net"
	"silk/run"
	"silk/ui"
	"syscall"
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
	1: &net.AggregationMessage{},
	2: &run.Message{},
	3: &kv.Request{},
})


func handle(c net.Connection, aggregation net.AggregationService, keyval kv.Service, routing net.RoutingService, running run.Service, log sio.Logger){
	var msg net.Message
	var more bool

	msg, more = <-c.RecvN(protocol, 1)
	if more == false {
		log.Warn("closed unexpectedly")
		close(c.Send())
		return
	}

	switch m := msg.(type) {
	case *net.RoutingMessage:
		log.Debug("receive routing request")
		routing.Handle(m, c)
	case *net.AggregationMessage:
		log.Debug("receive aggregation request")
		aggregation.Handle(m, c)
	case *run.Message:
		log.Debug("receive run request")
		running.Handle(m, c)
	case *kv.Request:
		log.Debug("receive kv request")
		keyval.Handle(m, c)
	default:
		log.Debug("receive unexpected message: %T",
			log.Emph(2, msg))
		close(c.Send())
	}
}

func serve(a net.Accepter, aggregation net.AggregationService, keyval kv.Service, routing net.RoutingService, running run.Service, log sio.Logger) {
	var conn net.Connection
	var clog sio.Logger
	var i int

	i = 0

	for conn = range a.Accept() {
		clog = log.WithLocalContext("conn[%d]", i)
		clog.Trace("accept")
		go handle(conn, aggregation, keyval, routing, running, clog)
		i += 1
	}
}

func setupServerSigmask(log sio.Logger) {
	var ignorec chan os.Signal = make(chan os.Signal, 1)

	signal.Notify(ignorec, syscall.SIGINT)

	go func () {
		var sig os.Signal

		for sig = range ignorec {
			log.Info("ignore signal %d", log.Emph(1, sig))
		}
	}()
}

func serverStart(port int, name string, log sio.Logger) {
	var aggregationService net.AggregationService
	var routingService net.RoutingService
	var runService run.Service
	var resolver net.Resolver
	var kvService kv.Service
	var tcp net.Accepter
	var err error

	log.Info("listen on tcp %d", log.Emph(1, port))
	tcp = net.NewTcpServer(fmt.Sprintf(":%d", port))

	aggregationService = net.NewAggregationServiceWith(
		&net.AggregationServiceOptions{
			Log: log.WithLocalContext("aggregate"),
		})

	kvService = kv.NewServiceWith(&kv.ServiceOptions{
		Log: log.WithLocalContext("kv"),
	})

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: log.WithLocalContext("resolve"),
		}))

	routingService = net.NewRoutingServiceWith(resolver,
		&net.RoutingServiceOptions{
			Log: log.WithLocalContext("route"),
		})

	runService, err = run.NewServiceWith(&run.ServiceOptions{
		View: kvService,
		Log: log.WithLocalContext("run"),
		Name: name,
	})

	if err != nil {
		fatale(err)
	}

	setupServerSigmask(log)

	log.Info("start")

	go serve(tcp, aggregationService, kvService, routingService,
		runService, log)
	go serve(aggregationService, aggregationService, kvService,
		routingService, runService, log)
	go serve(routingService, aggregationService, kvService, routingService,
		runService, log)

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
