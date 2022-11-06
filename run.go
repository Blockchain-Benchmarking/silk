package main


import (
	"fmt"
	"io"
	"os"
	"os/signal"
	sio "silk/io"
	"silk/net"
	"silk/run"
	"silk/ui"
)


// ----------------------------------------------------------------------------


func runUsage() {
	fmt.Printf(`Usage: %s run [-C<path> | --cwd=<path>] [-L | --local-command] <route> <cmd>
         [<args>]

Run a command on remote server.
The <route> indicate on what remote server to run the command.

Options:

  -C<path>, --cwd=<path>      Change current directory to the given <path>
                              before to run the command.

  -L, --local-command         Interpret <cmd> as a local file to be sent to
                              the remote server and to be executed.
                              If <cmd> is '-' then read the file to be executed
                              on the standard input.

`, os.Args[0])

	os.Exit(0)
}


// ----------------------------------------------------------------------------


type runConfig struct {
	log sio.Logger
	route string
	name string
	args []string
	cwd ui.OptionString
	localCommand ui.OptionBool
}


func doRun(config *runConfig) {
	var localExec io.ReadCloser
	var resolver net.Resolver
	var transmitStdin bool
	var route net.Route
	var agent run.Agent
	var job run.Job
	var err error

	transmitStdin = true

	if config.localCommand.Value() {
		if config.name == "-" {
			localExec = os.Stdin
			transmitStdin = false
		} else {
			localExec, err = os.Open(config.name)
			if err != nil {
				fatale(err)
			}
		}
	}

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: config.log.WithLocalContext("resolve"),
		}))
	route = net.NewRoute([]string{ config.route }, resolver)

	job = run.NewJobWith(config.name, config.args, route, protocol,
		&run.JobOptions{
			Cwd: config.cwd.Value(),
			LocalExec: localExec,
			Log: config.log.WithLocalContext("job[%s]",
				config.name),
			Signal: true,
			Stdin: transmitStdin,
		})

	signal.Notify(job.Signal(), os.Interrupt)

	for agent = range job.Accept() {
		go sio.WriteFromChannel(os.Stdout, agent.Stdout())
		go sio.WriteFromChannel(os.Stderr, agent.Stderr())

		go func (agent run.Agent) {
			<-agent.Wait()
			config.log.Info("agent %s exits with %d",
				config.log.Emph(0, agent.Name()),
				config.log.Emph(1, agent.Exit()))
		}(agent)
	}

	if transmitStdin {
		go sio.ReadInChannel(os.Stdin, job.Stdin())
	}

	<-job.Wait()
	close(job.Signal())
}


// ----------------------------------------------------------------------------


type printerType interface {
	instances(agents []run.Agent, log sio.Logger) []printer
}


// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type rawPrinterType struct {
	base io.Writer
}

func newRawPrinterType(base io.Writer) *rawPrinterType {
	var this rawPrinterType

	this.base = base

	return &this
}

func (this *rawPrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var p printer = newRawPrinter(this.base)
	var ret []printer = make([]printer, len(agents))
	var i int

	for i = range ret {
		ret[i] = p
	}

	return ret
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type printer interface {
	printChannel(c <-chan []byte)
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type rawPrinter struct {
	dest io.Writer
}

func newRawPrinter(dest io.Writer) *rawPrinter {
	var this rawPrinter

	this.dest = dest

	return &this
}

func (this *rawPrinter) printChannel(c <-chan []byte) {
	sio.WriteFromChannel(this.dest, c)
}


// ----------------------------------------------------------------------------


func runMain(cli ui.Cli, verbose *verbosity) {
	var config runConfig = runConfig{
		cwd: ui.OptString{}.New(),
		localCommand: ui.OptBool{}.New(),
	}
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { runUsage() ; return nil },
	}.New()
	var err error
	var ok bool

	cli.AddOption('C', "cwd", config.cwd)
	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)
	cli.AddOption('L', "local-command", config.localCommand)

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	config.route, ok = cli.SkipWord()
	if ok == false {
		fatal("missing route operand")
	}

	config.name, ok = cli.SkipWord()
	if ok == false {
		fatal("missing cmd operand")
	}

	config.args = cli.Arguments()[cli.Parsed():]

	config.log = verbose.log()

	doRun(&config)
}
