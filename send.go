package main


import (
	"fmt"
	"io"
	"os"
	"os/exec"
	sio "silk/io"
	"silk/net"
	"silk/run"
	"silk/ui"
	"strings"
)


// ----------------------------------------------------------------------------


func sendUsage() {
	fmt.Printf(`
Usage: %s send [-t<path> | --target-directory=<path>] [-z | --compress] <route>
         <paths>

Send one or more files at the given <paths> to remote servers
The <route> indicate on what remote server to run the command.

Options:

  -t<path>, --target-directory=<path>
                              Send the source files into the remote directory
                              specified by <path>.

  -z, --compress              Compress the files during transfer.

`, os.Args[0])

	os.Exit(0)
}


// ----------------------------------------------------------------------------


func doSendOne(route, targetDirectory, cwd, source string, compress bool, log sio.Logger) {
	var resolver net.Resolver
	var stdout io.ReadCloser
	var nroute net.Route
	var agent run.Agent
	var job run.Job
	var cmd *exec.Cmd
	var opt string
	var err error

	log = log.WithLocalContext("%s/%s", cwd, source)

	if compress {
		opt = "z"
	} else {
		opt = ""
	}

	cmd = exec.Command("tar", fmt.Sprintf("-c%sf", opt), "-", source)
	cmd.Dir = cwd

	stdout, err = cmd.StdoutPipe()
	if err != nil {
		fatale(err)
	}

	err = cmd.Start()
	if err != nil {
		fatale(err)
	}

	resolver = net.NewGroupResolver(net.NewAggregatedTcpResolverWith(
		protocol, &net.TcpResolverOptions{
			Log: log.WithLocalContext("resolve"),
		}))
	nroute = net.NewRoute([]string{ route }, resolver)

	job = run.NewJobWith("tar", []string{
		fmt.Sprintf("-ix%spf", opt), "-",
	}, nroute, protocol, &run.JobOptions{
		Log: log.WithLocalContext("job[tar]"),
		Cwd: targetDirectory,
		Stdin: true,
	})

	for agent = range job.Accept() {
		log.Info("agent %s receive", log.Emph(0, agent.Name()))

		go sio.WriteFromChannel(os.Stdout, agent.Stdout())
		go sio.WriteFromChannel(os.Stderr, agent.Stderr())

		go func (agent run.Agent) {
			<-agent.Wait()
			log.Info("agent %s exits with %d",
				log.Emph(0, agent.Name()),
				log.Emph(1, agent.Exit()))
		}(agent)
	}

	go sio.ReadInChannel(stdout, job.Stdin())

	<-job.Wait()

	err = cmd.Wait()
	if err != nil {
		fatale(err)
	}
}

func doSend(route, targetDirectory string, sources []string, compress bool, log sio.Logger) {
	var source, cwd string
	var elems []string

	for _, source = range sources {
		source = strings.TrimSuffix(source, "/")
		elems = strings.Split(source, "/")

		if len(elems) == 1 {
			cwd = ""
			source = elems[0]
		} else {
			cwd = strings.Join(elems[:len(elems)-1], "/")
			source = elems[len(elems)-1]
		}

		doSendOne(route, targetDirectory, cwd, source, compress, log)
	}
}


// ----------------------------------------------------------------------------


func sendMain(cli ui.Cli, verbose *verbosity) {
	var targetDirectoryOption ui.OptionString = ui.OptString{}.New()
	var helpOption ui.Option = ui.OptCall{
		WithoutArg: func () error { sendUsage() ; return nil },
	}.New()
	var compressOption ui.OptionBool = ui.OptBool{}.New()
	var args []string
	var route string
	var err error
	var ok bool

	cli.DelOption('h', "help")
	cli.AddOption('h', "help", helpOption)
	cli.AddOption('t', "target-directory", targetDirectoryOption)
	cli.AddOption('z', "compress", compressOption)

	err = cli.Parse()
	if err != nil {
		fatale(err)
	}

	route, ok = cli.SkipWord()
	if ok == false {
		fatal("missing route operand")
	}

	args = cli.Arguments()[cli.Parsed():]

	if len(args) < 1 {
		fatal("missing paths operand")
	}

	if len(targetDirectoryOption.Assignments()) == 0 {
		fatal("option '-t' mandatory (base case not yet implemented)")
	}

	doSend(route, targetDirectoryOption.Value(), args,
		compressOption.Value(), verbose.log())
}
