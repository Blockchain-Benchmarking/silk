package main


import (
	"fmt"
	"os"
	"os/signal"
	sio "silk/io"
	"silk/net"
	"silk/run"
	"silk/ui"
)


// ----------------------------------------------------------------------------


func runUsage() {
	fmt.Printf(`Usage: %s run [-C<path> | --cwd=<path>] <route> <cmd> [<args>]

Run a command on remote server.
The <route> indicate on what remote server to run the command.

Options:

  -C<path>, --cwd=<path>      Change current directory to the given <path>
                              before to run the command.

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
}


func doRun(config *runConfig) {
	var resolver net.Resolver
	var route net.Route
	var agent run.Agent
	var job run.Job

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: config.log.WithLocalContext("resolve"),
		}))
	route = net.NewRoute([]string{ config.route }, resolver)

	job = run.NewJobWith(config.name, config.args, route, protocol,
		&run.JobOptions{
			Cwd: config.cwd.Value(),
			Log: config.log.WithLocalContext("job[%s]",
				config.name),
			Signal: true,
			Stdin: true,
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

	go sio.ReadInChannel(os.Stdin, job.Stdin())

	<-job.Wait()
	close(job.Signal())
}


// ----------------------------------------------------------------------------


func runMain(cli ui.Cli, verbose *verbosity) {
	var config runConfig = runConfig{
		cwd: ui.OptString{}.New(),
	}
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { runUsage() ; return nil },
	}.New()
	var err error
	var ok bool

	cli.AddOption('C', "cwd", config.cwd)
	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)

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
