package run


import (
	"os"
	sio "silk/io"
	"silk/net"
	"sync"
)


// ----------------------------------------------------------------------------


type Job interface {
	Accept() <-chan Agent

	Signal() chan<- os.Signal

	Stdin() chan<- []byte

	Wait() <-chan struct{}
}

type JobOptions struct {
	Log sio.Logger

	Cwd string

	Signal bool

	Stdin bool

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

	return newJob(name, args, route, p, opts)
}


// ----------------------------------------------------------------------------


type job struct {
	log sio.Logger
	route net.Route
	acceptc chan Agent
	signalc chan os.Signal
	stdinc chan []byte
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
	this.agentStdin = opts.AgentStdin
	this.waitc = make(chan struct{})

	if opts.Signal == false {
		close(this.signalc)
	}

	if opts.Stdin == false {
		close(this.stdinc)
	}

	m.name = name
	m.args = args
	m.cwd = opts.Cwd

	err = m.check()
	if err != nil {
		return &this
	}

	go this.initiate(&m, p)
	go this.run()

	return &this
}

func (this *job) initiate(m *Message, p net.Protocol) {
	var transmitting sync.WaitGroup

	this.route.Send() <- net.MessageProtocol{ m, p }

	transmitting.Add(2)

	go this.transmitSignal(&transmitting)
	go this.transmitStdin(&transmitting)

	transmitting.Wait()

	this.log.Trace("close")
	close(this.route.Send())
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
		scode, err = signalCode(s)
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
	case *serviceName:
		log.Trace("receive service name: %s", log.Emph(0, m.name))
		running.Add(1)

		agent = newAgent(m.name, conn, running, log)

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
