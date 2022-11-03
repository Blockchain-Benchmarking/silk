package run


import (
	sio "silk/io"
	"silk/net"
	"sync"
	"sync/atomic"
)


// ----------------------------------------------------------------------------


type Agent interface {
	Name() string

	Stdin() chan<- []byte

	Stdout() <-chan []byte

	Stderr() <-chan []byte

	Wait() <-chan struct{}

	Exit() uint8
}


// ----------------------------------------------------------------------------


type agent struct {
	log sio.Logger
	name string
	conn net.Connection
	stdinc chan []byte
	stdoutc chan []byte
	stderrc chan []byte
	waitc chan struct{}
	exit atomic.Value
}

func newAgent(name string, conn net.Connection, running *sync.WaitGroup, log sio.Logger) *agent {
	var this agent
	var using sync.WaitGroup

	this.log = log
	this.name = name
	this.conn = conn
	this.stdoutc = make(chan []byte)
	this.stderrc = make(chan []byte)
	this.stdinc = make(chan []byte)
	this.waitc = make(chan struct{})

	using.Add(2)
	go func () {
		using.Wait()
		this.log.Trace("close")
		close(this.conn.Send())
	}()

	go this.run(running, &using)
	go this.transmit(&using)

	return &this
}

func (this *agent) run(running, using *sync.WaitGroup) {
	var stdout, stderr bool
	var msg net.Message
	var exited bool

	this.log.Debug("start %s", this.log.Emph(0, this.name))

	stdout = true
	stderr = true
	exited = false

	loop: for msg = range this.conn.Recv(protocol) {
		switch m := msg.(type) {

		case *jobExit:
			this.log.Trace("receive job exit: %d",
				this.log.Emph(1, m.code))
			this.exit.Store(m.code)
			exited = true
			break loop

		case *jobStdoutData:
			if stdout == false {
				this.log.Warn("receive %d bytes for closed " +
					"stdout", len(m.content))
			} else {
				this.log.Trace("receive %d bytes of stdout",
					len(m.content))
				this.stdoutc <- m.content
			}

		case *jobStdoutClose:
			if stdout == false {
				this.log.Warn("receive close notice for " +
					"closed stdout")
			} else {
				this.log.Trace("receive stdout close notice")
				stdout = false
				close(this.stdoutc)
			}

		case *jobStderrData:
			if stderr == false {
				this.log.Warn("receive %d bytes for closed " +
					"stderr", len(m.content))
			} else {
				this.log.Trace("receive %d bytes of stderr",
					len(m.content))
				this.stderrc <- m.content
			}

		case *jobStderrClose:
			if stderr == false {
				this.log.Warn("receive close notice for " +
					"closed stderr")
			} else {
				this.log.Trace("receive stderr close notice")
				stderr = false
				close(this.stderrc)
			}

		default:
			this.log.Warn("unexpected message: %T",
				this.log.Emph(2, msg))
			break loop

		}
	}

	if stdout {
		this.log.Warn("close with open stdout")
		close(this.stdoutc)
	}

	if stderr {
		this.log.Warn("close with open stderr")
		close(this.stderrc)
	}

	if exited == false {
		this.log.Warn("close before process exit")
		this.exit.Store(uint8(255))
	}

	close(this.waitc)

	running.Done()
	using.Done()

	for msg = range this.conn.Recv(protocol) {
		this.log.Warn("unexpected message: %T", this.log.Emph(2, msg))
	}
}

func (this *agent) transmit(using *sync.WaitGroup) {
	var b []byte

	for b = range this.stdinc {
		this.log.Trace("send %d bytes to stdin", len(b))
		this.conn.Send() <- net.MessageProtocol{
			M: &jobStdinData{ b },
			P: protocol,
		}
	}

	this.log.Trace("send stdin close notice")
	this.conn.Send() <- net.MessageProtocol{
		M: &jobStdinCloseUcast{},
		P: protocol,
	}

	using.Done()
}

func (this *agent) Name() string {
	return this.name
}

func (this *agent) Stdin() chan<- []byte {
	return this.stdinc
}

func (this *agent) Stdout() <-chan []byte {
	return this.stdoutc
}

func (this *agent) Stderr() <-chan []byte {
	return this.stderrc
}

func (this *agent) Wait() <-chan struct{} {
	return this.waitc
}

func (this *agent) Exit() uint8 {
	return this.exit.Load().(uint8)
}
