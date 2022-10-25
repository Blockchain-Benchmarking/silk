package core


import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	sio "silk/io"
	"silk/net"
	"silk/run"
	"strings"
)


// ----------------------------------------------------------------------------


type Server interface {
	Name() string

	Run(ctx context.Context) error
}


func NewTcpServer(port uint16) (Server, error) {
	return NewTcpServerWith(port, nil)
}

func NewTcpServerWith(port uint16, opts *ServerOptions) (Server, error) {
	var out []byte
	var err error

	if opts == nil {
		opts = &ServerOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	if opts.Name == "" {
		opts.Name = os.Getenv("HOSTNAME")
	}
	if opts.Name == "" {
		out, err = exec.Command("hostname").Output()
		if err == nil {
			opts.Name = strings.TrimSpace(string(out))
		}
	}
	if opts.Name == "" {
		opts.Name = fmt.Sprintf("server-%d", rand.Uint64())
	}

	return newTcpServer(port, opts)
}


type ServerOptions struct {
	Log sio.Logger

	Name string
}


// ----------------------------------------------------------------------------


type tcpServer struct {
	log sio.Logger
	port uint16
	name string
	net net.Server
	stopped bool
	cancel context.CancelFunc
}

func newTcpServer(port uint16, opts *ServerOptions) (*tcpServer, error) {
	var this tcpServer
	var err error

	if len(opts.Name) > MaxNameLength {
		return nil, &NameTooLongError{ opts.Name }
	}

	this.log = opts.Log
	this.port = port
	this.name = opts.Name

	this.log.Trace("new server '%s'", opts.Log.Emph(0, this.name))

	this.net, err = net.NewTcpServer(fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	return &this, nil
}

func (this *tcpServer) handleRun(ctx context.Context, rmsg *runMessage, msgc <-chan net.Message, conn net.Connection, log sio.Logger) error {
	var exited chan struct{}
	var proc run.Process
	var msg net.Message
	var err error

	proc, err = run.NewProcessWith(rmsg.name,rmsg.args,&run.ProcessOptions{
		Log: log.WithLocalContext(rmsg.name),

		Stdout: func (b []byte) error {
			log.Trace("send stdout message (%d bytes)", len(b))
			return conn.Send(&stdoutMessage{ b }, protocol)
		},

		Stderr: func (b []byte) error {
			log.Trace("send stderr message (%d bytes)", len(b))
			return conn.Send(&stderrMessage{ b }, protocol)
		},

		CloseStdout: func () {
			log.Trace("send stdout close message")
			conn.Send(&closeStdoutMessage{}, protocol)
		},

		CloseStderr: func () {
			log.Trace("send stderr close message")
			conn.Send(&closeStderrMessage{}, protocol)
		},
	})

	if err != nil {
		log.Debug("send error message: %s", err.Error())
		return conn.Send(&errorMessage{ err.Error() }, protocol)
	}

	exited = make(chan struct{})
	go func () {
		proc.Wait()
		close(exited)
	}()

	loop: for {
		select {
		case msg = <-msgc:
			switch m := msg.(type) {
			case *stdinMessage:
				proc.Stdin(m.content)
			case *closeStdinMessage:
				proc.CloseStdin()
			default:
				log.Warn("%v", &UnknownMessageError{ msg })
			}
		case <-exited:
			break loop
		}
	}

	<-exited

	log.Trace("send exit message: %d", log.Emph(1, proc.Exit()))

	return conn.Send(&exitMessage{ proc.Exit() }, protocol)
}

func (this *tcpServer) handleNameRequest(msg *nameRequestMessage, conn net.Connection, log sio.Logger) error {
	log.Trace("send name reply: '%s'", log.Emph(0, this.name))
	return conn.Send(&nameReplyMessage{ name: this.name }, protocol)
}

func (this *tcpServer) receiveMessages(ctx context.Context, msgc chan<- net.Message, conn net.Connection, log sio.Logger) error {
	var msg net.Message
	var err error

	for err == nil {
		msg, err = conn.Recv(protocol)

		if ctx.Err() != nil {
			err = ctx.Err()
		}

		if err != nil {
			break
		}

		msgc <- msg
	}

	close(msgc)

	conn.Close()

	return err
}

func (this *tcpServer) handle(ctx context.Context, conn net.Connection, log sio.Logger) {
	var msgc chan net.Message = make(chan net.Message)
	var done chan struct{} = make(chan struct{})
	var errc chan error = make(chan error)
	var msg net.Message
	var err error

	go func () {
		select {
		case <-done:
		case <-ctx.Done():
			conn.Close()
		}
	}()

	go func () {
		errc <- this.receiveMessages(ctx, msgc, conn, log)
		close(errc)
	}()

	for msg = range msgc {
		switch m := msg.(type) {
		case *closeMessage:
			log.Trace("receive close request")
			this.stop()
		case *nameRequestMessage:
			log.Trace("receive name request")
			err = this.handleNameRequest(m, conn, log)
		case *runMessage:
			log.Trace("receive run request")
			err = this.handleRun(ctx, m, msgc, conn, log)
		case *stdinMessage:
			log.Trace("receive stdin message -> ignore")
		case *closeStdinMessage:
			log.Trace("receive stdin close message -> ignore")
		default:
			err = &UnknownMessageError{ msg }
		}

		if err != nil {
			break
		}
	}

	close(done)
	conn.Close()

	if err == nil {
		err = <-errc
	} else {
		<-errc
	}

	if this.stopped && (err == context.Canceled) {
		err = nil
	}

	if err != nil {
		log.Warn("%s", err.Error())
	}
}

func (this *tcpServer) run(ctx context.Context, server net.Server, log sio.Logger) error {
	var done chan struct{} = make(chan struct{})
	var conn net.Connection
	var err error

	go func () {
		select {
		case <-done:
		case <-ctx.Done():
			server.Close()
		}
	}()

	log.Debug("start accepting")

	for {
		conn, err = server.Accept()

		if ctx.Err() != nil {
			err = ctx.Err()
		}

		if err != nil {
			break
		}

		go this.handle(ctx, conn, log)
	}

	if this.stopped && (err == context.Canceled) {
		err = nil
	}

	if err != nil {
		log.Error("%s", err.Error())
	}

	log.Debug("stop accepting")

	server.Close()

	close(done)

	return err
}

func (this *tcpServer) Run(ctx context.Context) error {
	ctx, this.cancel = context.WithCancel(ctx)
	return this.run(ctx, this.net, this.log.WithLocalContext("net"))
}

func (this *tcpServer) Name() string {
	return this.name
}

func (this *tcpServer) stop() {
	this.log.Debug("stopping")

	this.stopped = true
	this.cancel()
}
