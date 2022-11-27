package main


import (
	"fmt"
	"os"
	sio "silk/io"
	"silk/ui"
	"strconv"
	"strings"
)


// ----------------------------------------------------------------------------


const ProgramName = "silk"
const ProgramVersion = "0.4.0"
const AuthorName = "Gauthier Voron"
const AuthorEmail = "gauthier.voron@epfl.ch"


// ----------------------------------------------------------------------------


func fatal(fstr string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "%s: " + fstr + "\n",
		append([]interface{}{ ProgramName }, args...)...)

	fmt.Fprintf(os.Stderr, "Please type '%s --help' for more " +
		"information\n", os.Args[0])

	os.Exit(1)
}

func fatale(err error) {
	fatal("%s", err.Error())
}


// ----------------------------------------------------------------------------


func usage() {
	fmt.Printf(`
Usage: %s [-h | --help] [-l<path> | --log=<path>] [-v | --verbose[=<str>]]
         [--version] <command> [<args>]

Available commands:

  kv          List, get, set or delete key/value pairs on remote servers

  run         Run a command on remote servers

  send        Send files to remote servers

  server      Start a server

Please type '%s <command> --help' for information about a command.


Common options:

  -h, --help                  Print a help message and exit.

  -l<path>, --log=<path>      Write the logs in the given <path> instead of on
                              the stderr.

  -v, --verbose[=<str>]       Increase the verbosity level by one. This option
                              can be given multiple times. If <str> is given
                              then it indicates a verbosity level either by its
                              numeric or string value. The possible values are:
                              0=none, 1=error, 2=warn, 3=info, 4=debug, 5=trace

  --version                   Print the version message and exit.

`, os.Args[0], os.Args[0])

	os.Exit(0)
}

func version() {
	fmt.Printf(`%s %s
%s
%s
`, ProgramName, ProgramVersion, AuthorName, AuthorEmail)

	os.Exit(0)
}


// ----------------------------------------------------------------------------


type verbosity struct {
	level int
	path string
}

func newVerbosity(level int) *verbosity {
	var this verbosity

	switch level {
	case sio.LOG_ERROR: this.level = 1
	case sio.LOG_WARN:  this.level = 2
	case sio.LOG_INFO:  this.level = 3
	case sio.LOG_DEBUG: this.level = 4
	case sio.LOG_TRACE: this.level = 5
	default: this.level = 0
	}

	return &this
}

func (this *verbosity) setPath(value string) {
	this.path = value
}

func (this *verbosity) increaseLevel() {
	this.level += 1
}

func (this *verbosity) setLevel(value string) error {
	var err error
	var i uint64

	i, err = strconv.ParseUint(value, 10, 8)
	if err == nil {
		this.level = int(i)
		return nil
	}

	switch strings.ToLower(value) {
	case "n", "none", "quiet", "silent": this.level = 0
	case "e", "err", "error": this.level = 1
	case "w", "warn", "warning": this.level = 2
	case "i", "info": this.level = 3
	case "d", "debug": this.level = 4
	case "t", "trace": this.level = 5
	default: return fmt.Errorf("unknown verbosity level: '%s'", value)
	}

	return nil
}

func (this *verbosity) log() sio.Logger {
	var file *os.File
	var level int
	var err error

	switch this.level {
	case 0: level = 0
	case 1: level = sio.LOG_ERROR
	case 2: level = sio.LOG_WARN
	case 3: level = sio.LOG_INFO
	case 4: level = sio.LOG_DEBUG
	default: level = sio.LOG_TRACE
	}

	if this.path == "" {
		return sio.NewStderrLogger(level)
	} else {
		file, err = os.Create(this.path)
		if err != nil {
			fatale(err)
		}

		return sio.NewFileLogger(file, level)
	}
}


// ----------------------------------------------------------------------------


func main() {
	var verbose *verbosity = newVerbosity(sio.LOG_WARN)
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { usage() ; return nil },
	}.New()
	var logOption ui.Option = ui.OptCall{
		WithArg: func (v string) error {
			verbose.setPath(v)
			return nil
		}, 
	}.New()
	var verboseOption ui.Option = ui.OptCall{
		WithoutArg: func () error {
			verbose.increaseLevel()
			return nil
		},
		WithArg: func (v string) error {
			return verbose.setLevel(v)
		}, 
	}.New()
	var versionOption ui.Option = ui.OptCall{
		WithoutArg: func () error { version() ; return nil },
	}.New()
	var cli ui.Cli = ui.NewCli()
	var command string
	var err error
	var ok bool

	cli.AddOption('h', "help", helpOption)
	cli.AddOption('l', "log", logOption)
	cli.AddOption('v', "verbose", verboseOption)
	cli.AddLongOption("version", versionOption)

	cli.Append(os.Args[1:])

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	command, ok = cli.SkipWord()
	if ok == false {
		fatal("missing command operand")
	}

	switch command {
	case "kv":
		kvMain(cli, verbose)
	case "run":
		runMain(cli, verbose)
	case "send":
		sendMain(cli, verbose)
	case "server":
		serverMain(cli, verbose)
	default:
		fatal("unknown command operand '%s'", command)
	}
}
