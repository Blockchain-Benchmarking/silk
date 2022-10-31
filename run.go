package main


import (
	"fmt"
	"os"
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
}


// ----------------------------------------------------------------------------


func runCommand(route, name string, args []string, cwd string, log sio.Logger){
	var resolver net.Resolver
	var nroute net.Route
	var agent run.Agent
	var job run.Job

	resolver = net.NewGroupResolver(net.NewTcpResolverWith(protocol,
		&net.TcpResolverOptions{
			Log: log.WithLocalContext("resolve"),
		}))

	nroute = net.NewRoute([]string{ route }, resolver)
	job = run.NewJobWith(name, args, nroute, protocol, &run.JobOptions{
		Log: log.WithLocalContext("job[%s]", name),

		Cwd: cwd,
	})

	for agent = range job.Accept() {
		close(agent.Stdin())

		go func (agent run.Agent) {
			var b []byte

			for b = range agent.Stdout() {
				os.Stdout.Write(b)
			}
		}(agent)

		go func (agent run.Agent) {
			var b []byte

			for b = range agent.Stdout() {
				os.Stderr.Write(b)
			}
		}(agent)

		go func (agent run.Agent) {
			<-agent.Wait()
			log.Info("agent %s exits with %d",
				log.Emph(0, agent.Name()),
				log.Emph(1, agent.Exit()))
		}(agent)
	}

	var b []byte
	var e error
	var n int

	for {
		b = make([]byte, 1 << 21)

		n, e = os.Stdin.Read(b)
		if e != nil {
			break
		}

		job.Stdin() <- b[:n]
	}

	close(job.Stdin())

	<-job.Wait()
}


// ----------------------------------------------------------------------------


func runMain(cli ui.Cli) {
	var cwdOption ui.OptionString = ui.OptString{}.New()
	var route, name string
	var args []string
	var err error
	var ok bool

	cli.AddOption('C', "cwd", cwdOption)

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	route, ok = cli.SkipWord()
	if ok == false {
		fatal("missing route operand")
	}

	name, ok = cli.SkipWord()
	if ok == false {
		fatal("missing cmd operand")
	}

	args = cli.Arguments()[cli.Parsed():]

	runCommand(route, name, args, cwdOption.Value(),
		sio.NewStderrLogger(sio.LOG_TRACE))
}
