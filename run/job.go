package run


import (
	"io"
	"os"
	sio "silk/io"
	"silk/net"
	"sync"
	"syscall"
)


// ----------------------------------------------------------------------------


// A remote `Job`.
//
// This is an interface for a `Process` running on one or many remote servers
// and managed by a `Service`.
//
// Each instance of the remote `Process` (one per remote server) is represented
// by an `Agent`.
// This `Job` can broadcast `Process`es inputs to all instances.
//
type Job interface {
	// Accept an `Agent` representing a `Process` instance running on a
	// remote server.
	//
	Accept() <-chan Agent

	// Return a channel to send `os.Signal`s to deliver to all instances
	// of the remote `Process`.
	// This channel is closed unless `JobOption.Signal` was `true`.
	//
	Signal() chan<- os.Signal

	// Return a channel to send `[]byte`s to write on the standard input of
	// all instances of the remote `Process`.
	// Note that the `[]byte`s sent on this channel are not sequentially
	// consistent with `[]byte`s sent on the `Agent.Stdin()` channels.
	// This channel is closed unless `JobOption.Stdin` was `true`.
	//
	Stdin() chan<- []byte

	// Wait for all instances of the remote `Process` to terminate,
	// including the ones for which the `Agent` has not been `Accept`ed.
	//
	Wait() <-chan struct{}
}

// Options for controlling a `Job` behavior.
//
type JobOptions struct {
	// Remote server executes the `Process` in `Cwd` if not empty.
	// Otherwise it executes it in the server cwd.
	Cwd string

	// Add or override the remote `Process` environment with the variables
	// defined in this map.
	Env map[string]string

	// If not `nil` then ignore the `name` argument of `NewJobWith` and
	// transfer what is `Read` from this parameter to the remote servers
	// and execute it.
	LocalExec io.ReadCloser

	// Log what happens on this `sio.Logger` if not `nil`.
	Log sio.Logger

	// Indicate if the process must be launched directly (`ShellNone`),
	// from a shell (`ShellSimple`) or from a shell with the files usually
	// sourced by a login shell (`ShellLogin`).
	// If the shell is not `ShellNone` then it can also indicate a list of
	// file to source before to execute the command.
	Shell ShellType

	// If `true` then the `Job.Signal()` channel is open.
	Signal bool

	// If `true` then the `Job.Stdin()` channel is open.
	Stdin bool

	// If `true` then the `Accept`ed `Agent.Signal()` channel are open.
	AgentSignal bool

	// If `true` then the `Accept`ed `Agent.Stdin()` channel are open.
	AgentStdin bool
}


func NewJob(name string, args []string, route net.Route, p net.Protocol) Job {
	return NewJobWith(name, args, route, p, nil)
}

func NewJobWith(name string, args []string, route net.Route, p net.Protocol, opts *JobOptions)Job{
	if opts == nil {
		opts = &JobOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	if opts.Shell == nil {
		opts.Shell = ShellNone
	}

	return newJob(name, args, route, p, opts)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type ShellType interface {
	shell() bool
	sourceLogin() bool
	sources() []string
}


var ShellNone ShellType = &shellTypeNone{}

type ShellSimple struct {
	Sources []string
}

type ShellLogin struct {
	Sources []string
}


// ----------------------------------------------------------------------------


type job struct {
	log sio.Logger
	route net.Route
	acceptc chan Agent
	signalc chan os.Signal
	stdinc chan []byte
	agentSignal bool
	agentStdin bool
	waitc chan struct{}
}

func newJob(name string, args []string, route net.Route, p net.Protocol, opts *JobOptions) *job {
	var this job
	var m Message
	var err error

	this.log = opts.Log
	this.route = route
	this.acceptc = make(chan Agent)
	this.signalc = make(chan os.Signal, 8)
	this.stdinc = make(chan []byte, 32)
	this.agentSignal = opts.AgentSignal
	this.agentStdin = opts.AgentStdin
	this.waitc = make(chan struct{})

	if opts.Signal == false {
		close(this.signalc)
	}

	if opts.Stdin == false {
		close(this.stdinc)
	}

	if opts.LocalExec != nil {
		m.name = ""
	} else if name == "" {
		this.prematureExit()
		return &this
	} else {
		m.name = name
	}

	m.args = args
	m.cwd = opts.Cwd
	m.env = opts.Env

	if opts.Shell.shell() == false {
		m.shell = shellNone
	} else {
		m.sources = opts.Shell.sources()
		if opts.Shell.sourceLogin() {
			m.shell = shellLogin
		} else {
			m.shell = shellSimple
		}
	}

	err = m.check()
	if err != nil {
		this.prematureExit()
		return &this
	}

	go this.initiate(&m, p, opts)

	return &this
}

func (this *job) prematureExit() {
	close(this.acceptc)
	close(this.waitc)
}

func (this *job) initiate(m *Message, p net.Protocol, opts *JobOptions) {
	var transmitting sync.WaitGroup

	this.route.Send() <- net.MessageProtocol{ m, p }

	if opts.LocalExec != nil {
		this.sendExecutable(opts.LocalExec)
	}

	go this.run()

	transmitting.Add(2)

	go this.transmitSignal(&transmitting)
	go this.transmitStdin(&transmitting)

	transmitting.Wait()

	this.log.Trace("close")
	close(this.route.Send())
}

func (this *job) sendExecutable(localExec io.ReadCloser) {
	var b []byte

	for b = range sio.NewReaderChannel(localExec) {
		this.log.Trace("send %d bytes of executable", len(b))
		this.route.Send() <- net.MessageProtocol{
			M: &serviceExecutableData{ b },
			P: protocol,
		}
	}

	this.log.Trace("send end of executable")
	this.route.Send() <- net.MessageProtocol{
		M: &serviceExecutableDone{},
		P: protocol,
	}

	localExec.Close()
}

func (this *job) run() {
	var accepting sync.WaitGroup
	var running sync.WaitGroup
	var conn net.Connection
	var log sio.Logger
	var i int

	i = 0

	for conn = range this.route.Accept() {
		log = this.log.WithLocalContext("agent[%d]", i)
		log.Trace("open")

		accepting.Add(1)
		go this.handleAgent(conn, &accepting, &running, log)

		i += 1
	}

	accepting.Wait()

	close(this.acceptc)

	running.Wait()

	close(this.waitc)
}

func (this *job) transmitSignal(transmitting *sync.WaitGroup) {
	var s os.Signal
	var scode uint8
	var err error

	for s = range this.signalc {
		scode, err = signalCode(s.(syscall.Signal))
		if err != nil {
			this.log.Warn("%s", err.Error())
			continue
		}

		this.log.Trace("send signal %s", this.log.Emph(1, s.String()))
		this.route.Send() <- net.MessageProtocol{
			M: &jobSignal{ scode },
			P: protocol,
		}
	}

	transmitting.Done()
}

func (this *job) transmitStdin(transmitting *sync.WaitGroup) {
	var b []byte

	for b = range this.stdinc {
		this.log.Trace("send %d bytes of stdin", len(b))
		this.route.Send() <- net.MessageProtocol{
			M: &jobStdinData{ b },
			P: protocol,
		}
	}

	this.log.Trace("send stdin close notice")
	this.route.Send() <- net.MessageProtocol{
		M: &jobStdinCloseBcast{},
		P: protocol,
	}

	transmitting.Done()
}

func (this *job) handleAgent(conn net.Connection, accepting, running *sync.WaitGroup, log sio.Logger) {
	var msg net.Message
	var agent Agent
	var more bool

	defer accepting.Done()

	msg, more = <-conn.RecvN(protocol, 1)
	if more == false {
		log.Warn("connection closed unexpectedly")
		close(conn.Send())
		return
	}

	switch m := msg.(type) {
	case *serviceMeta:
		log.Trace("receive service name: %s", log.Emph(0, m.name))
		running.Add(1)

		agent = newAgent(m.name, conn, running, log)

		if this.agentSignal == false {
			close(agent.Signal())
		}

		if this.agentStdin == false {
			close(agent.Stdin())
		}

		this.acceptc <- agent
		return
	default:
		log.Warn("unexpected message type: %T", log.Emph(2, msg))
		close(conn.Send())
		return
	}
}

func (this *job) Accept() <-chan Agent {
	return this.acceptc
}

func (this *job) Signal() chan<- os.Signal {
	return this.signalc
}

func (this *job) Stdin() chan<- []byte {
	return this.stdinc
}

func (this *job) Wait() <-chan struct{} {
	return this.waitc
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type shellTypeNone struct {
}

func (this *shellTypeNone) shell() bool {
	return false
}

func (this *shellTypeNone) sourceLogin() bool {
	return false
}

func (this *shellTypeNone) sources() []string {
	return nil
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *ShellSimple) shell() bool {
	return true
}

func (this *ShellSimple) sourceLogin() bool {
	return false
}

func (this *ShellSimple) sources() []string {
	return this.Sources
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *ShellLogin) shell() bool {
	return true
}

func (this *ShellLogin) sourceLogin() bool {
	return true
}

func (this *ShellLogin) sources() []string {
	return this.Sources
}
