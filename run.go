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
	"strings"
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

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type prefixPrinterType struct {
	base io.Writer
	format string
}

func newPrefixPrinterType(base io.Writer, format string) *prefixPrinterType {
	var this prefixPrinterType

	this.base = base
	this.format = format

	return &this
}

func (this *prefixPrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var ret []printer = make([]printer, len(agents))
	var prefix string
	var i int

	for i = range ret {
		prefix = formatAgent(this.format, agents[i])
		ret[i] = newPrefixPrinter(this.base, prefix)
	}

	return ret
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type filePrinterType struct {
	format string
}

func newFilePrinterType(format string) *filePrinterType {
	var this filePrinterType

	this.format = format

	return &this
}

func (this *filePrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var ret []printer = make([]printer, len(agents))
	var file *os.File
	var path string
	var err error
	var i int

	for i = range ret {
		path = formatAgent(this.format, agents[i])
		file, err = os.Create(path)
		if err != nil {
			log.Warn("cannot create file %s: %s",
				log.Emph(0, path), err.Error())
			ret[i] = newNopPrinter()
		} else {
			ret[i] = newFilePrinter(file)
		}
	}

	return ret
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type printer interface {
	printChannel(c <-chan []byte)
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type nopPrinter struct {
}

func newNopPrinter() *nopPrinter {
	return &nopPrinter{}
}

func (this *nopPrinter) printChannel(<-chan []byte) {
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

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type prefixPrinter struct {
	dest io.Writer
	prefix string
}

func newPrefixPrinter(dest io.Writer, prefix string) *prefixPrinter {
	var this prefixPrinter

	this.dest = dest
	this.prefix = prefix

	return &this
}

func (this *prefixPrinter) printChannel(c <-chan []byte) {
	var s string

	for s = range sio.ParseLinesFromChannel(c) {
		io.WriteString(this.dest, this.prefix + s)
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type filePrinter struct {
	dest *os.File
}

func newFilePrinter(dest *os.File) *filePrinter {
	var this filePrinter

	this.dest = dest

	return &this
}

func (this *filePrinter) printChannel(c <-chan []byte) {
	sio.WriteFromChannel(this.dest, c)
	this.dest.Close()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func formatAgent(format string, agent run.Agent) string {
	return strings.ReplaceAll(format, "%n", agent.Name())
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
