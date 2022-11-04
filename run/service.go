package run


import (
	"fmt"
	"math"
	"os"
	"os/exec"
	sio "silk/io"
	"silk/net"
	"silk/util/atomic"
	"silk/util/rand"
	"strings"
	"sync"
	"syscall"
)


// ----------------------------------------------------------------------------


type Service interface {
	Handle(*Message, net.Connection)
}

type ServiceOptions struct {
	Log sio.Logger

	Name string
}

func NewService() (Service, error) {
	return NewServiceWith(nil)
}

func NewServiceWith(opts *ServiceOptions) (Service, error) {
	var out []byte
	var err error

	if opts == nil {
		opts = &ServiceOptions{}
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

	if len(opts.Name) > MaxServiceNameLength {
		return nil, &ServiceNameTooLongError{ opts.Name }
	}

	return newService(opts), nil
}


type Message struct {
	name string

	args []string

	cwd string
}


const MaxServiceNameLength = math.MaxUint8

const MaxJobNameLength = math.MaxUint16

const MaxJobArguments = math.MaxUint16

const MaxJobArgumentLength = math.MaxUint16

const MaxJobPathLength = math.MaxUint16


type JobNameTooLongError struct {
	Name string
}

type JobTooManyArgumentsError struct {
	Args []string
}

type JobArgumentTooLongError struct {
	Arg string
}

type JobPathTooLongError struct {
	Path string
}

type JobUnknownSignalError struct {
	Signal os.Signal
}

type JobUnknownSignalCodeError struct {
	Code uint8
}

type ServiceNameTooLongError struct {
	Name string
}

type UnknownMessageError struct {
	Msg net.Message
}


// ----------------------------------------------------------------------------


type service struct {
	log sio.Logger
	name string
	nextId atomic.Uint64
}

func newService(opts *ServiceOptions) *service {
	var this service

	this.log = opts.Log
	this.name = opts.Name
	this.nextId.Store(0)

	return &this
}

func (this *service) Handle(msg *Message, conn net.Connection) {
	var id uint64 = this.nextId.Add(1) - 1
	var log sio.Logger = this.log.WithLocalContext("job[%d]", id)

	log.Trace("request: %s %v", log.Emph(0, msg.name),
		log.Emph(0, msg.args))

	newServiceProcess(this, msg, conn, log).run()

	close(conn.Send())
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type serviceProcess struct {
	parent *service
	log sio.Logger
	req *Message
	conn net.Connection
}

func newServiceProcess(parent *service, req *Message, conn net.Connection, log sio.Logger) *serviceProcess {
	var this serviceProcess

	this.parent = parent
	this.log = log
	this.req = req
	this.conn = conn

	return &this
}

func (this *serviceProcess) run() {
	var sending sync.WaitGroup
	var proc Process
	var err error

	this.log.Trace("send service name: %s",
		this.log.Emph(0, this.parent.name))
	this.conn.Send() <- net.MessageProtocol{
		M: &serviceName{ this.parent.name },
		P: protocol,
	}

	sending.Add(2)  // stdout + stderr

	proc, err = NewProcessWith(this.req.name,this.req.args,&ProcessOptions{
		Log: this.log,

		Cwd: this.req.cwd,

		Stdout: func (b []byte) error {
			this.conn.Send() <- net.MessageProtocol{
				M: &jobStdoutData{ b },
				P: protocol,
			}

			return nil
		},

		CloseStdout: func () {
			this.conn.Send() <- net.MessageProtocol{
				M: &jobStdoutClose{},
				P: protocol,
			}
			sending.Done()
		},

		Stderr: func (b []byte) error {
			this.conn.Send() <- net.MessageProtocol{
				M: &jobStderrData{ b },
				P: protocol,
			}

			return nil
		},

		CloseStderr: func () {
			this.conn.Send() <- net.MessageProtocol{
				M: &jobStderrClose{},
				P: protocol,
			}
			sending.Done()
		},
	})
	if err != nil {
		this.log.Warn("%s", err.Error())
		return
	}

	go this.transmit(proc)

	proc.Wait()
	sending.Wait()

	this.log.Trace("send job exit: %d", this.log.Emph(1, proc.Exit()))
	this.conn.Send() <- net.MessageProtocol{
		M: &jobExit{ proc.Exit() },
		P: protocol,
	}
}

func (this *serviceProcess) transmit(proc Process) {
	var stdinBcast, stdinUcast bool
	var msg net.Message
	var sig os.Signal
	var err error

	stdinBcast = true
	stdinUcast = true

	for msg = range this.conn.Recv(protocol) {
		switch m := msg.(type) {

		case *jobSignal:
			sig, err = codeSignal(m.signum)
			if err != nil {
				this.log.Warn("%s", err.Error())
			} else {
				proc.Kill(sig)
			}

		case *jobStdinData:
			if (stdinBcast == false) && (stdinUcast == false) {
				this.log.Warn("receive %d bytes of closed " +
					"stdin", len(m.content))
			} else {
				proc.Stdin(m.content)
			}

		case *jobStdinCloseBcast:
			if stdinBcast == false {
				this.log.Warn("receive close notice for " +
					"closed broadcast stdin")
			} else {
				stdinBcast = false
				if stdinUcast == false {
					proc.CloseStdin()
				}
			}

		case *jobStdinCloseUcast:
			if stdinUcast == false {
				this.log.Warn("receive close notice for " +
					"closed unicast stdin")
			} else {
				stdinUcast = false
				if stdinBcast == false {
					proc.CloseStdin()
				}
			}

		default:
			err = &UnknownMessageError{ msg }
			this.log.Warn("%s", err.Error())
			continue

		}
	}

	if stdinBcast || stdinUcast {
		this.log.Warn("connection closed before stdin closed")
		proc.CloseStdin()
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *Message) check() error {
	var i int

	if len(this.name) > MaxJobNameLength {
		return &JobNameTooLongError{ this.name }
	}

	if len(this.args) > MaxJobArguments {
		return &JobTooManyArgumentsError{ this.args }
	}

	for i = range this.args {
		if len(this.args[i]) > MaxJobArgumentLength {
			return &JobArgumentTooLongError{ this.args[i] }
		}
	}

	if len(this.cwd) > MaxJobPathLength {
		return &JobPathTooLongError{ this.cwd }
	}

	return nil
}

func (this *Message) Encode(sink sio.Sink) error {
	var i int

	sink = sink.WriteString16(this.name).
		WriteUint16(uint16(len(this.args)))

	for i = range this.args {
		sink = sink.WriteString16(this.args[i])
	}

	sink = sink.WriteString16(this.cwd)

	return sink.Error()
}

func (this *Message) Decode(source sio.Source) error {
	var n uint16
	var i int

	return source.ReadString16(&this.name).
		ReadUint16(&n).AndThen(func () error {
			this.args = make([]string, n)

			for i = range this.args {
				source = source.ReadString16(&this.args[i])
			}

			return source.Error()
		}).
		ReadString16(&this.cwd).
		Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


var protocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	  0: &serviceName{},
	  1: &serviceExecutableData{},
	  2: &serviceExecutableDone{},

	100: &jobExit{},
	101: &jobStdinData{},
	102: &jobStdinCloseBcast{},
	103: &jobStdinCloseUcast{},
	104: &jobStdoutData{},
	105: &jobStdoutClose{},
	106: &jobStderrData{},
	107: &jobStderrClose{},
	108: &jobSignal{},
})


type serviceName struct {
	name string
}

func (this *serviceName) Encode(sink sio.Sink) error {
	return sink.WriteString8(this.name).Error()
}

func (this *serviceName) Decode(source sio.Source) error {
	return source.ReadString8(&this.name).Error()
}


type serviceExecutableData struct {
	content []byte
}

func (this *serviceExecutableData) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *serviceExecutableData) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type serviceExecutableDone struct {
}

func (this *serviceExecutableDone) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *serviceExecutableDone) Decode(source sio.Source) error {
	return source.Error()
}


type jobExit struct {
	code uint8
}

func (this *jobExit) Encode(sink sio.Sink) error {
	return sink.WriteUint8(this.code).Error()
}

func (this *jobExit) Decode(source sio.Source) error {
	return source.ReadUint8(&this.code).Error()
}


type jobStdinData struct {
	content []byte
}

func (this *jobStdinData) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *jobStdinData) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type jobStdinCloseBcast struct {
}

func (this *jobStdinCloseBcast) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *jobStdinCloseBcast) Decode(source sio.Source) error {
	return source.Error()
}


type jobStdinCloseUcast struct {
}

func (this *jobStdinCloseUcast) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *jobStdinCloseUcast) Decode(source sio.Source) error {
	return source.Error()
}


type jobStdoutData struct {
	content []byte
}

func (this *jobStdoutData) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *jobStdoutData) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type jobStdoutClose struct {
}

func (this *jobStdoutClose) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *jobStdoutClose) Decode(source sio.Source) error {
	return source.Error()
}


type jobStderrData struct {
	content []byte
}

func (this *jobStderrData) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *jobStderrData) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type jobStderrClose struct {
}

func (this *jobStderrClose) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *jobStderrClose) Decode(source sio.Source) error {
	return source.Error()
}


type jobSignal struct {
	signum uint8
}

func (this *jobSignal) Encode(sink sio.Sink) error {
	return sink.WriteUint8(this.signum).Error()
}

func (this *jobSignal) Decode(source sio.Source) error {
	return source.ReadUint8(&this.signum).Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func signalCode(sig os.Signal) (uint8, error) {
	switch sig {
	case syscall.SIGABRT:   return  0, nil
	case syscall.SIGALRM:   return  1, nil
	case syscall.SIGBUS:    return  2, nil
	case syscall.SIGCHLD:   return  3, nil
	case syscall.SIGCONT:   return  4, nil
	case syscall.SIGFPE:    return  5, nil
	case syscall.SIGHUP:    return  6, nil
	case syscall.SIGILL:    return  7, nil
	case syscall.SIGINT:    return  8, nil
	case syscall.SIGIO:     return  9, nil
	case syscall.SIGPIPE:   return 10, nil
	case syscall.SIGPROF:   return 11, nil
	case syscall.SIGPWR:    return 12, nil
	case syscall.SIGQUIT:   return 13, nil
	case syscall.SIGSEGV:   return 14, nil
	case syscall.SIGSTKFLT: return 15, nil
	case syscall.SIGSYS:    return 16, nil
	case syscall.SIGTERM:   return 17, nil
	case syscall.SIGTRAP:   return 18, nil
	case syscall.SIGTSTP:   return 19, nil
	case syscall.SIGTTIN:   return 20, nil
	case syscall.SIGTTOU:   return 21, nil
	case syscall.SIGURG:    return 22, nil
	case syscall.SIGUSR1:   return 23, nil
	case syscall.SIGUSR2:   return 24, nil
	case syscall.SIGVTALRM: return 25, nil
	case syscall.SIGWINCH:  return 26, nil
	case syscall.SIGXCPU:   return 27, nil
	case syscall.SIGXFSZ:   return 28, nil
	default: return 0, &JobUnknownSignalError{ sig }
	}
}

func codeSignal(scode uint8) (os.Signal, error) {
	switch scode {
	case  0: return syscall.SIGABRT,   nil
	case  1: return syscall.SIGALRM,   nil
	case  2: return syscall.SIGBUS,    nil
	case  3: return syscall.SIGCHLD,   nil
	case  4: return syscall.SIGCONT,   nil
	case  5: return syscall.SIGFPE,    nil
	case  6: return syscall.SIGHUP,    nil
	case  7: return syscall.SIGILL,    nil
	case  8: return syscall.SIGINT,    nil
	case  9: return syscall.SIGIO,     nil
	case 10: return syscall.SIGPIPE,   nil
	case 11: return syscall.SIGPROF,   nil
	case 12: return syscall.SIGPWR,    nil
	case 13: return syscall.SIGQUIT,   nil
	case 14: return syscall.SIGSEGV,   nil
	case 15: return syscall.SIGSTKFLT, nil
	case 16: return syscall.SIGSYS,    nil
	case 17: return syscall.SIGTERM,   nil
	case 18: return syscall.SIGTRAP,   nil
	case 19: return syscall.SIGTSTP,   nil
	case 20: return syscall.SIGTTIN,   nil
	case 21: return syscall.SIGTTOU,   nil
	case 22: return syscall.SIGURG,    nil
	case 23: return syscall.SIGUSR1,   nil
	case 24: return syscall.SIGUSR2,   nil
	case 25: return syscall.SIGVTALRM, nil
	case 26: return syscall.SIGWINCH,  nil
	case 27: return syscall.SIGXCPU,   nil
	case 28: return syscall.SIGXFSZ,   nil
	default: return nil, &JobUnknownSignalCodeError{ scode }
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *JobNameTooLongError) Error() string {
	return fmt.Sprintf("job name too long: %s", this.Name)
}

func (this *JobTooManyArgumentsError) Error() string {
	return fmt.Sprintf("job has too many arguments: %d", len(this.Args))
}

func (this *JobArgumentTooLongError) Error() string {
	return fmt.Sprintf("job argument too long: %s", this.Arg)
}

func (this *JobPathTooLongError) Error() string {
	return fmt.Sprintf("job path too long: %s", this.Path)
}

func (this *JobUnknownSignalError) Error() string {
	return fmt.Sprintf("unknown signal: %s", this.Signal.String())
}

func (this *JobUnknownSignalCodeError) Error() string {
	return fmt.Sprintf("unknown signal code: %d", this.Code)
}

func (this *ServiceNameTooLongError) Error() string {
	return fmt.Sprintf("service name too long: %s", this.Name)
}

func (this *UnknownMessageError) Error() string {
	return fmt.Sprintf("unknown message: %T", this.Msg)
}
