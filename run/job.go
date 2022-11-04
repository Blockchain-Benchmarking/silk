package run


import (
	sio "silk/io"
	"silk/net"
	"sync"
)


// ----------------------------------------------------------------------------


type Job interface {
	Accept() <-chan Agent

	Stdin() chan<- []byte

	Wait() <-chan struct{}
}

type JobOptions struct {
	Log sio.Logger

	Cwd string

	Stdin bool
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
	stdinc chan []byte
	waitc chan struct{}
}

func newJob(name string, args []string, route net.Route, p net.Protocol, opts *JobOptions) *job {
	var this job
	var m Message
	var err error

	this.log = opts.Log
	this.route = route
	this.acceptc = make(chan Agent)
	this.stdinc = make(chan []byte)
	this.waitc = make(chan struct{})

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
	this.route.Send() <- net.MessageProtocol{ m, p }
	this.transmit()
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

func (this *job) transmit() {
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

	this.log.Trace("close")
	close(this.route.Send())
}

func (this *job) handleAgent(conn net.Connection, accepting, running *sync.WaitGroup, log sio.Logger) {
	var msg net.Message
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
		this.acceptc <- newAgent(m.name, conn, running, log)
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

func (this *job) Stdin() chan<- []byte {
	return this.stdinc
}

func (this *job) Wait() <-chan struct{} {
	return this.waitc
}
