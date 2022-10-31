package main


import (
	"fmt"
	"os"
	"silk/ui"
)


const ProgramName = "silk"
const ProgramVersion = "0.1.0"
const AuthorName = "Gauthier Voron"
const AuthorEmail = "gauthier.voron@epfl.ch"


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


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


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func usage() error {
	fmt.Printf(`
Usage: %s [-h | --help] [-l<path> | --log=<path>] [-v | --verbose[=<str>]]
         [--version] <command> [<args>]

Available commands:

  run         Run a command on a remote server

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

	return nil
}

func version() error {
	fmt.Printf(`%s %s
%s
%s
`, ProgramName, ProgramVersion, AuthorName, AuthorEmail)

	os.Exit(0)

	return nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func increaseVerbosity() {
	fmt.Printf("verbose++\n")
}

func setVerbosity(val string) error {
	fmt.Printf("verbose = %s\n", val)
	return nil
}


func main() {
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { usage() ; return nil },
	}.New()
	var logOption ui.OptionString = ui.OptString{
		ValidityPredicate: ui.OptPathParentExists,
	}.New()
	var verboseOption ui.Option = ui.OptCall{
		WithoutArg: func () error { increaseVerbosity() ; return nil },
		WithArg: func (v string) error { return setVerbosity(v) }, 
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
	case "run":
		runMain(cli)
	case "server":
		serverMain(cli)
	case "cp":
		fatal("unimplemented")
	default:
		fatal("unknown command operand '%s'", command)
	}
}
