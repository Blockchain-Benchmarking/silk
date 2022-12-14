package main


import (
	"fmt"
	"io"
	"os"
	"os/signal"
	sio "silk/io"
	"silk/kv"
	"silk/net"
	"silk/run"
	"silk/ui"
	"strings"
	"sync"
	"syscall"
)


const SILK_CWD_NAME = "SILK_CWD"

const SILK_PREFIX_NAME = "SILK_PREFIX"

const SILK_SHELL_NAME = "SILK_SHELL"


// ----------------------------------------------------------------------------


func runUsage() {
	fmt.Printf(`
Usage: %s run [-C<path>] [-e<str>] [-E<str>] [-L] [-o<str>] [-s]
         <route> <cmd> [<args>]

Run a command on remote server.
The <route> indicate on what remote server to run the command.
The provided <args> are subject to text substitution (see 'kv --help').
The <cmd> is also subject to txt substitution unless -L is specified.

Options:

  -C<path>, --cwd=<path>      Change current directory before to run the
                              command. If <path> starts with '/' then the
                              current directory is set to <path>. Otherwise the
                              current directory is the concatenation of the
                              %s environment variable and <path>.

  -e<str>, --stderr=<str>     Print the standard error of the remote processes
                              accordingly to the given <str> specification.
                              See (Printing) section.

  -E<str>, --env=<str>        Add the following <str> environment variable to
                              the remote process. The provided <str> can be
                              either in the form <var> to define an empty
                              variable or <var>=<val> to assign it a value.
                              If the '--source' option is supplied, the
                              environment of the shell is updated.

  -L, --local-command         Interpret <cmd> as a local file to be sent to
                              the remote server and to be executed.
                              If <cmd> is '-' then read the file to be executed
                              on the standard input.

  -o<str>, --stdout=<str>     Print the standard output of the remote processes
                              accordingly to the given <str> specification.
                              See (Printing) section.

  -s[<str>] --source[=<str>]  Launch the remote process from a shell after
                              sourcing the remote file whose path is <str> if
                              supplied or the files that would be sourced by a
                              login shell otherwise.
                              The shell used is the one stored in the $SHELL
                              variable in the remote server environment.


Environment:

  %-20s        Set the current directory to its value on the
                              remote server before to run the command.

  %-20s        Use the its value as a formatting string for stderr/stdout
                              prefix.

  %-20s        Launch the remote command in a shell with login files sourced.


Printing:

  raw                         Print the streams as they are on their
                              corresponding local stream (stdout or stderr).
                              This is the default when there is a single remote
                              process

  prefix[=<fmt>]              Prefix each line by the given <fmt> format string
                              where '%%n' is interpreted as the remote server
                              name. If not specified, <fmt> is the value of
                              %s or '%%n :: '.
                              This is the default when there are more than one
                              remote process.

  file=<fmt>                  Print the stream of each remote process in a
                              dedicated file whose path is given by the <fmt>
                              format string where '%%n' is interpreted as the
                              remote server name.

`, os.Args[0], SILK_CWD_NAME, SILK_CWD_NAME, SILK_PREFIX_NAME, SILK_SHELL_NAME)

	os.Exit(0)
}


// ----------------------------------------------------------------------------


type runConfig struct {
	log sio.Logger
	route string
	name string
	args []string
	cwd ui.OptionString
	env ui.OptionStringList
	localCommand ui.OptionBool
	stderr *printerOption
	stdout *printerOption
	source ui.OptionStringList
}


func setupRunSigmask() {
	signal.Ignore(syscall.SIGTSTP)
	signal.Ignore(syscall.SIGTTIN)
	signal.Ignore(syscall.SIGTTOU)
}

func doRun(config *runConfig) {
	var stderrPrinters, stdoutPrinters []printer
	var printing sync.WaitGroup
	var localExec io.ReadCloser
	var resolver net.Resolver
	var env map[string]string
	var shell run.ShellType
	var transmitStdin bool
	var agents []run.Agent
	var cwd, keyval string
	var route net.Route
	var agent run.Agent
	var keys []kv.Key
	var success bool
	var job run.Job
	var err error
	var i, n int

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

	if strings.HasPrefix(config.cwd.Value(), "/") {
		cwd = config.cwd.Value()
	} else {
		cwd = os.Getenv(SILK_CWD_NAME)
		if (cwd != "") && (config.cwd.Value() != "") {
			cwd = cwd + "/" + config.cwd.Value()
		} else {
			cwd = cwd + config.cwd.Value()
		}
	}

	if len(config.env.Values()) > 0 {
		env = make(map[string]string)

		for _, keyval = range config.env.Values() {
			i = strings.Index(keyval, "=")
			if i < 0 {
				env[keyval] = ""
			} else {
				env[keyval[:i]] = keyval[i+1:]
			}
		}
	}

	n = len(config.source.Assignments())
	if (os.Getenv(SILK_SHELL_NAME) == "") && (n == 0) {
		shell = run.ShellNone
	} else if os.Getenv(SILK_SHELL_NAME) != "" {
		shell = &run.ShellLogin{
			Sources: config.source.Values(),
		}
	} else if n > len(config.source.Values()) {
		shell = &run.ShellLogin{
			Sources: config.source.Values(),
		}
	} else {
		shell = &run.ShellSimple{
			Sources: config.source.Values(),
		}
	}

	keys = make([]kv.Key, 0)
	keys = append(keys, config.stdout.value().keys()...)
	keys = append(keys, config.stderr.value().keys()...)

	setupRunSigmask()

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: config.log.WithLocalContext("resolve"),
		}))
	route = net.NewRoute([]string{ config.route }, resolver)

	job = run.NewJobWith(config.name, config.args, route, protocol,
		&run.JobOptions{
			Cwd: cwd,
			Env: env,
			Keys: keys,
			Shell: shell,
			LocalExec: localExec,
			Log: config.log.WithLocalContext("job[%s]",
				config.name),
			Signal: true,
			Stdin: transmitStdin,
		})

	signal.Notify(job.Signal(), os.Interrupt)

	for agent = range job.Accept() {
		config.log.Debug("accept agent %s",
			config.log.Emph(0, agent.Name()))

		agents = append(agents, agent)
	}

	stdoutPrinters = config.stdout.value().instances(agents, config.log)
	stderrPrinters = config.stderr.value().instances(agents, config.log)

	for i, agent = range agents {
		printing.Add(2)

		go func (index int, agent run.Agent) {
			stdoutPrinters[index].printChannel(agent.Stdout())
			printing.Done()
		}(i, agent)

		go func (index int, agent run.Agent) {
			stderrPrinters[index].printChannel(agent.Stderr())
			printing.Done()
		}(i, agent)
	}

	if transmitStdin {
		go sio.ReadInChannel(os.Stdin, job.Stdin())
	}

	<-job.Wait()

	printing.Wait()

	success = true

	for _, agent = range agents {
		<-agent.Wait()

		if agent.Exit() != 0 {
			config.log.Info("agent %s exits with %d",
				config.log.Emph(0, agent.Name()),
				config.log.Emph(1, agent.Exit()))

			success = false
		}
	}

	if success == false {
		os.Exit(1)
	}
}


// ----------------------------------------------------------------------------


type printerType interface {
	keys() []kv.Key
	instances(agents []run.Agent, log sio.Logger) []printer
}

func parsePrinterType(spec string, base io.Writer) (printerType, error) {
	var envPrefix string
	var ret printerType
	var err error

	if spec == "raw" {
		return newRawPrinterType(base), nil
	}

	if spec == "prefix" {
		envPrefix = os.Getenv(SILK_PREFIX_NAME)
		if envPrefix == "" {
			return newDefaultPrefixPrinterType(base), nil
		}

		ret, err = newPrefixPrinterType(base, envPrefix)
		if err != nil {
			return newDefaultPrefixPrinterType(base), nil
		}

		return ret, nil
	}

	if strings.HasPrefix(spec, "prefix=") {
		return newPrefixPrinterType(base, spec[len("prefix="):])
	}

	if strings.HasPrefix(spec, "file=") {
		return newFilePrinterType(spec[len("file="):])
	}

	return nil, &invalidPrinterType{ spec }
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type defaultPrinterType struct {
	base io.Writer
	prefixInner printerType
}

func newDefaultPrinterType(base io.Writer) *defaultPrinterType {
	var this defaultPrinterType
	var envPrefix string
	var err error

	this.base = base

	envPrefix = os.Getenv(SILK_PREFIX_NAME)
	if envPrefix == "" {
		this.prefixInner = newDefaultPrefixPrinterType(base)
	} else {
		this.prefixInner, err = newPrefixPrinterType(base, envPrefix)
		if err != nil {
			this.prefixInner = newDefaultPrefixPrinterType(base)
		}
	}

	return &this
}

func (this *defaultPrinterType) keys() []kv.Key {
	return this.prefixInner.keys()
}

func (this *defaultPrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	if len(agents) <= 1 {
		return newRawPrinterType(this.base).
			instances(agents, log)
	} else {
		return this.prefixInner.instances(agents, log)
	}
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

func (this *rawPrinterType) keys() []kv.Key {
	return make([]kv.Key, 0)
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
	expr kv.Expression
	ekeys []kv.Key
}

func newPrefixPrinterType(base io.Writer, format string) (*prefixPrinterType, error) {
	var tokens []kv.ExpressionToken
	var this prefixPrinterType
	var err error
	var i int

	this.base = base

	this.expr, err = kv.ParseExpression(format)
	if err != nil {
		return nil, err
	}

	this.ekeys = make([]kv.Key, 0)
	tokens = this.expr.Tokens()
	for i = range tokens {
		switch t := tokens[i].(type) {
		case kv.ExpressionKeyToken:
			this.ekeys = append(this.ekeys, t.Key())
		}
	}

	return &this, nil
}

func (this *prefixPrinterType) keys() []kv.Key {
	return this.ekeys
}

func (this *prefixPrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var ret []printer = make([]printer, len(agents))
	var prefix string
	var i int

	for i = range ret {
		prefix = formatAgent(this.expr, agents[i], log)
		ret[i] = newPrefixPrinter(this.base, prefix)
	}

	return ret
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type defaultPrefixPrinterType struct {
	base io.Writer
}

func newDefaultPrefixPrinterType(base io.Writer) *defaultPrefixPrinterType {
	var this defaultPrefixPrinterType

	this.base = base

	return &this
}


func (this *defaultPrefixPrinterType) keys() []kv.Key {
	return make([]kv.Key, 0)
}

func (this *defaultPrefixPrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var ret []printer = make([]printer, len(agents))
	var prefix string
	var i int

	for i = range ret {
		prefix = formatDefaultAgent(agents[i])
		ret[i] = newPrefixPrinter(this.base, prefix)
	}

	return ret
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type filePrinterType struct {
	expr kv.Expression
	ekeys []kv.Key
}

func newFilePrinterType(format string) (*filePrinterType, error) {
	var tokens []kv.ExpressionToken
	var this filePrinterType
	var err error
	var i int

	this.expr, err = kv.ParseExpression(format)
	if err != nil {
		return nil, err
	}

	this.ekeys = make([]kv.Key, 0)
	tokens = this.expr.Tokens()
	for i = range tokens {
		switch t := tokens[i].(type) {
		case kv.ExpressionKeyToken:
			this.ekeys = append(this.ekeys, t.Key())
		}
	}

	return &this, nil
}

func (this *filePrinterType) keys() []kv.Key {
	return this.ekeys
}

func (this *filePrinterType) instances(agents []run.Agent, log sio.Logger) []printer {
	var ret []printer = make([]printer, len(agents))
	var file *os.File
	var path string
	var err error
	var i int

	for i = range ret {
		path = formatAgent(this.expr, agents[i], log)
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


func formatAgent(expr kv.Expression, agent run.Agent, log sio.Logger) string {
	var tokens []kv.ExpressionToken
	var ret, txt string
	var err error
	var i int

	tokens = expr.Tokens()
	for i = range tokens {
		switch t := tokens[i].(type) {

		case kv.ExpressionPlainToken:
			txt = t.Plain()
			txt = strings.ReplaceAll(txt, "%n", agent.Name())
			ret += txt

		default:
			txt, err = tokens[i].Format(agent.Meta())
			if err != nil {
				log.Warn("formatting failure for agent %s: %s",
					log.Emph(0, agent.Name()), err.Error())
				return formatDefaultAgent(agent)
			}
			ret += txt
		}
	}

	return ret
}

func formatDefaultAgent(agent run.Agent) string {
	return fmt.Sprintf("%s :: ", agent.Name())
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type printerOption struct {
	base io.WriteCloser
	inner ui.OptionString
	ptype printerType
}

func newPrinterOption(base io.WriteCloser) *printerOption {
	var this printerOption

	this.base = base
	this.inner = ui.OptString{}.New()
	this.ptype = newDefaultPrinterType(this.base)

	return &this
}

func (this *printerOption) Assignments() []ui.OptionAssignment {
	return this.inner.Assignments()
}

func (this *printerOption) AssignValue(v string, st ui.ParsingState) error {
	var ptype printerType
	var err error

	ptype, err = parsePrinterType(v, this.base)
	if err != nil {
		return err
	}

	err = this.inner.AssignValue(v, st)
	if err != nil {
		return err
	}

	this.ptype = ptype

	return nil
}

func (this *printerOption) value() printerType {
	return this.ptype
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type invalidPrinterType struct {
	spec string
}

func (this *invalidPrinterType) Error() string {
	return fmt.Sprintf("invalid printing: '%s'", this.spec)
}


// ----------------------------------------------------------------------------


func runMain(cli ui.Cli, verbose *verbosity) {
	var config runConfig = runConfig{
		cwd: ui.OptString{}.New(),
		env: ui.OptStringList{}.New(),
		localCommand: ui.OptBool{}.New(),
		source: ui.OptStringList{ Variadic: true }.New(),
		stderr: newPrinterOption(os.Stderr),
		stdout: newPrinterOption(os.Stdout),
	}
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { runUsage() ; return nil },
	}.New()
	var err error
	var ok bool

	cli.AddOption('C', "cwd", config.cwd)
	cli.AddOption('e', "stderr", config.stderr)
	cli.AddOption('E', "env", config.env)
	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)
	cli.AddOption('L', "local-command", config.localCommand)
	cli.AddOption('o', "stdout", config.stdout)
	cli.AddOption('s', "source", config.source)

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
		if config.localCommand.Value() {
			config.name = "-"
		} else {
			fatal("missing cmd operand")
		}
	}

	config.args = cli.Arguments()[cli.Parsed():]

	config.log = verbose.log()

	doRun(&config)
}
