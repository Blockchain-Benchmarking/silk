package run


import (
	"io"
	"os"
	"os/exec"
	sio "silk/io"
	"strings"
)


// ----------------------------------------------------------------------------


type Process interface {
	Exit() uint8

	Kill(os.Signal)

	Stdin(data []byte) error

	CloseStdin()

	Wait()
}


func NewProcess(name string, args []string) (Process, error) {
	return NewProcessWith(name, args, nil)
}

func NewProcessWith(n string, a []string, o *ProcessOptions) (Process, error) {
	var ret *process
	var err error

	if o == nil {
		o = &ProcessOptions{}
	}

	if o.Log == nil {
		o.Log = sio.NewNopLogger()
	}

	if o.Stdout == nil {
		o.Stdout = func (b []byte) error {
			var err error
			_, err = os.Stdout.Write(b)
			return err
		}
	}

	if o.CloseStdout == nil {
		o.CloseStdout = func () {}
	}

	if o.Stderr == nil {
		o.Stderr = func (b []byte) error {
			var err error
			_, err = os.Stderr.Write(b)
			return err
		}
	}

	if o.CloseStderr == nil {
		o.CloseStderr = func () {}
	}

	ret, err = newProcess(n, a, o)
	if err != nil {
		return nil, err
	}

	err = ret.run()
	if err != nil {
		return nil, err
	}

	return ret, nil
}


type ProcessOptions struct {
	Log sio.Logger

	Cwd string

	Stdout func ([]byte) error

	Stderr func ([]byte) error

	CloseStdout func ()

	CloseStderr func ()
}


// ----------------------------------------------------------------------------


const ioBufferSize = 1 << 16


type process struct {
	log sio.Logger
	logStdin sio.Logger
	inner *exec.Cmd
	stdin io.WriteCloser
	stdout *processReader
	stderr *processReader
	waitc chan struct{}
	exit uint8
}

func newProcess(name string, args []string, opts *ProcessOptions) (*process, error) {
	var stdout, stderr io.ReadCloser
	var this process
	var err error

	this.inner = exec.Command(name, args...)

	this.stdin, err = this.inner.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err = this.inner.StdoutPipe()
	if err != nil {
		this.stdin.Close()
		return nil, err
	}

	stderr, err = this.inner.StderrPipe()
	if err != nil {
		this.stdin.Close()
		stdout.Close()
		return nil, err
	}

	this.log = opts.Log
	this.logStdin = this.log.WithLocalContext("stdin")
	this.inner.Dir = opts.Cwd
	this.stdout = newProcessReader(stdout, opts.Stdout, opts.CloseStdout,
		this.log.WithLocalContext("stdout"))
	this.stderr = newProcessReader(stderr, opts.Stderr, opts.CloseStderr,
		this.log.WithLocalContext("stderr"))
	this.waitc = make(chan struct{})
	this.exit = 0

	return &this, nil
}

func (this *process) run() error {
	var err error

	this.log.Debug("start '%s': '%s'", this.log.Emph(0, this.inner.Path),
		strings.Join(this.inner.Args, "' '"))
	this.log.Trace("  cwd: '%s'", this.inner.Dir)
	this.log.Trace("  env: '%s'", strings.Join(this.inner.Env, "' '"))

	err = this.inner.Start()
	if err != nil {
		return err
	}

	go this.stdout.transfer()
	go this.stderr.transfer()
	go this.waitTermination()

	return nil
}

func (this *process) Kill(sig os.Signal) {
	this.log.Trace("send signal %s", this.log.Emph(1, sig.String()))
	this.inner.Process.Signal(sig)
}

func (this *process) Stdin(data []byte) error {
	var err error

	this.logStdin.Trace("transfer %d bytes", len(data))

	_, err = this.stdin.Write(data)

	return err
}

func (this *process) CloseStdin() {
	this.logStdin.Trace("close")
	this.stdin.Close()
}

func (this *process) waitTermination() {
	var ecode int

	this.inner.Wait()

	ecode = this.inner.ProcessState.ExitCode()
	if ecode == -1 {
		this.exit = 255
	} else {
		this.exit = uint8(ecode)
	}

	close(this.waitc)
}

func (this *process) Wait() {
	<-this.waitc
	this.stdout.wait()
	this.stderr.wait()
	this.log.Trace("exit")
}

func (this *process) Exit() uint8 {
	return this.exit
}


type processReader struct {
	log sio.Logger
	pipe io.ReadCloser
	writef func ([]byte) error
	closef func ()
	closec chan struct{}
}

func newProcessReader(pipe io.ReadCloser, writef func ([]byte) error, closef func (), log sio.Logger) *processReader {
	var this processReader

	this.log = log
	this.pipe = pipe
	this.writef = writef
	this.closef = closef
	this.closec = make(chan struct{})

	return &this
}

func (this *processReader) transfer() {
	var b []byte = make([]byte, ioBufferSize)
	var readErr, callErr error
	var n int

	for {
		n, readErr = this.pipe.Read(b)

		if n > 0 {
			this.log.Trace("transfer %d bytes", n)

			callErr = this.writef(b[:n])
			if callErr != nil {
				break
			}
		}

		if readErr != nil {
			break
		}
	}

	this.log.Trace("close")
	this.pipe.Close()
	this.closef()

	close(this.closec)
}

func (this *processReader) wait() {
	<-this.closec
}
