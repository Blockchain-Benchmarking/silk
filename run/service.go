package run


import (
	"fmt"
	"io/ioutil"
	"math"
	"os"
	"os/exec"
	"regexp"
	sio "silk/io"
	"silk/kv"
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
	Kv kv.Formatter

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

	if opts.Kv == nil {
		opts.Kv = kv.Verbatim
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
	// Name of the executable to run.
	// If empty then wait to receive client executable.
	name string

	// Arguments to give to the process.
	args []string

	// If not empty then indicate a directory to set as cwd for the process
	// to run.
	cwd string

	// The variables/values to add or override in the process environment.
	env map[string]string

	// Indicate if the remote process must be launched outside of a shell
	// (`shellNone`), in a simple shell with (`shellLogin`) or without
	// (`shellSimple`) sourcing the files usually sourced by a login shell.
	shell shellType

	// If the process is to be launched in a shell then indicate a list of
	// path to source before to execute the job.
	sources []string

	// The `kv.Key`s from which to get the corresponding `kv.Value`s before
	// to run the process.
	keys []kv.Key
}


const MaxServiceNameLength = math.MaxUint8

const MaxJobNameLength     = math.MaxUint16

const MaxJobArguments      = math.MaxUint16

const MaxJobArgumentLength = math.MaxUint16

const MaxJobPathLength     = math.MaxUint16

const MaxJobEnvVariables   = math.MaxUint16

const MaxJobEnvKeyLength   = math.MaxUint8

const MaxJobEnvValueLength = math.MaxUint16

const MaxJobSources        = math.MaxUint16

const MaxJobSourceLength   = math.MaxUint16

const MaxJobKeys           = math.MaxUint16


type JobInvalidEnvKeyError struct {
	Key string
	Value string
}

type JobInvalidEnvValueError struct {
	Key string
	Value string
}

type JobNameTooLongError struct {
	Name string
}

type JobSourceTooLongError struct {
	Source string
}

type JobTooManyArgumentsError struct {
	Args []string
}

type JobTooManySourcesError struct {
	Sources []string
}

type JobArgumentTooLongError struct {
	Arg string
}

type JobPathTooLongError struct {
	Path string
}

type JobTooManyEnvVariablesError struct {
	Env map[string]string
}

type JobTooManyKeysError struct {
	Keys []kv.Key
}

type JobTooManyValuesError struct {
	Values []kv.Value
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
	kv kv.Formatter
	nextId atomic.Uint64
}

func newService(opts *ServiceOptions) *service {
	var this service

	this.log = opts.Log
	this.name = opts.Name
	this.kv = opts.Kv
	this.nextId.Store(0)

	return &this
}

func (this *service) Handle(msg *Message, conn net.Connection) {
	var id uint64 = this.nextId.Add(1) - 1
	var log sio.Logger = this.log.WithLocalContext("job[%d]", id)

	log.Trace("request: %s %v", log.Emph(0, msg.name),
		log.Emph(0, msg.args))

	newServiceProcess(this, msg, this.kv, conn, log).run()

	close(conn.Send())
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type serviceProcess struct {
	parent *service
	log sio.Logger
	req *Message
	kv kv.Formatter
	conn net.Connection
	tempExec *os.File
}

func newServiceProcess(parent *service, req *Message, kv kv.Formatter, conn net.Connection, log sio.Logger) *serviceProcess {
	var this serviceProcess

	this.parent = parent
	this.log = log
	this.req = req
	this.kv = kv
	this.conn = conn
	this.tempExec = nil

	return &this
}

func (this *serviceProcess) run() {
	var sending sync.WaitGroup
	var args []string
	var proc Process
	var name string
	var err error
	var i int

	this.log.Trace("send service name: %s",
		this.log.Emph(0, this.parent.name))
	this.conn.Send() <- net.MessageProtocol{
		M: &serviceMeta{ this.parent.name, []kv.Value{} },
		P: protocol,
	}

	if this.req.name == "" {
		this.log.Trace("receive client executable")
		err = this.receiveExecutable()
		if err != nil {
			this.log.Warn("%s", err.Error())
			return
		}

		name = this.tempExec.Name()

		defer os.Remove(name)
	} else {
		name = this.req.name
	}

	name, err = this.kv.Format(name)
	if err != nil {
		this.log.Warn("%s", err.Error())
		return
	}

	for i = range this.req.args {
		this.req.args[i], err = this.kv.Format(this.req.args[i])
		if err != nil {
			this.log.Warn("%s", err.Error())
			return
		}
	}

	name, args = buildCommand(name, this.req)

	sending.Add(2)  // stdout + stderr

	proc, err = NewProcessWith(name, args, &ProcessOptions{
		Log: this.log,

		Cwd: this.req.cwd,

		Env: this.req.env,

		Setpgid: true,

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

	go func () {
		proc.Wait()
		this.log.Trace("send job exit: %d",
			this.log.Emph(1, proc.Exit()))
		this.conn.Send() <- net.MessageProtocol{
			M: &jobExit{ proc.Exit() },
			P: protocol,
		}
	}()

	this.transmit(proc)
	sending.Wait()
}

func (this *serviceProcess) receiveExecutable() error {
	var msg net.Message
	var err error

	this.tempExec, err = ioutil.TempFile("", "silk-run.*")
	if err != nil {
		return err
	}

	this.log.Trace("receive executable in %s",
		this.log.Emph(0, this.tempExec.Name()))

	loop: for {
		msg = <-this.conn.RecvN(protocol, 1)

		switch m := msg.(type) {
		case *serviceExecutableData:
			this.log.Trace("receive %d bytes of executable",
				len(m.content))
			_, err = this.tempExec.Write(m.content)
			if err != nil {
				os.Remove(this.tempExec.Name())
				return err
			}
		case *serviceExecutableDone:
			this.log.Trace("receive end of executable")
			break loop
		default:
			os.Remove(this.tempExec.Name())
			return &UnknownMessageError{ msg }
		}
	}

	err = this.tempExec.Chmod(0500)
	if err != nil {
		os.Remove(this.tempExec.Name())
		return err
	}

	err = this.tempExec.Close()
	if err != nil {
		os.Remove(this.tempExec.Name())
		return err
	}

	return nil
}

func (this *serviceProcess) transmit(proc Process) {
	var stdinBcast, stdinUcast bool
	var sig syscall.Signal
	var msg net.Message
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
				this.log.Trace("receive close notice for " +
					"broadcast stdin")
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
				this.log.Trace("receive close notice for " +
					"unicast stdin")
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


func buildCommand(name string, req *Message) (string, []string) {
	var shell, sourcePath, shellCmd string
	var args []string

	if req.shell == shellNone {
		args = req.args
	} else {
		shell = os.Getenv("SHELL")
		args = []string{}

		if req.shell == shellLogin {
			if strings.HasSuffix(shell, "/bash") {
				args = append(args, "--login")
			} else if strings.HasSuffix(shell, "/zsh") {
				args = append(args, "--login")
			}
		}

		shellCmd = ""

		for _, sourcePath = range req.sources {
			shellCmd += fmt.Sprintf("source %s ; ",
				protectShellWord(sourcePath))
		}

		shellCmd += protectShellCommand(name, req.args)

		name = shell
		args = append(args, "-c", shellCmd)
	}

	return name, args
}

func protectShellWord(word string) string {
	return strings.Replace(word, "'", "'\\''", -1)
}

func protectShellCommand(name string, args []string) string {
	var b strings.Builder
	var arg string

	b.WriteString(protectShellWord(name))

	for _, arg = range args {
		b.WriteRune(' ')
		b.WriteString(protectShellWord(arg))
	}

	return b.String()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type shellType = uint8

const shellNone   = 0
const shellSimple = 1
const shellLogin  = 2


func (this *Message) check() error {
	var key, value string
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

	if len(this.env) > MaxJobEnvVariables {
		return &JobTooManyEnvVariablesError{ this.env }
	}

	for key, value = range this.env {
		if len(key) > MaxJobEnvKeyLength {
			return &JobInvalidEnvKeyError{ key, value }
		}

		if envKeyRegexp.Match([]byte(key)) == false {
			return &JobInvalidEnvKeyError{ key, value }
		}

		if len(value) > MaxJobEnvValueLength {
			return &JobInvalidEnvValueError{ key, value }
		}
	}

	if len(this.sources) > MaxJobSources {
		return &JobTooManySourcesError{ this.sources }
	}

	for i = range this.sources {
		if len(this.sources[i]) > MaxJobSourceLength {
			return &JobSourceTooLongError{ this.sources[i] }
		}
	}

	if len(this.keys) > MaxJobKeys {
		return &JobTooManyKeysError{ this.keys }
	}

	return nil
}

var envKeyRegexp *regexp.Regexp = regexp.MustCompile("^[-_a-zA-Z0-9/]{1,255}$")

func (this *Message) Encode(sink sio.Sink) error {
	var key, value string
	var i int

	sink = sink.WriteString16(this.name).
		WriteUint16(uint16(len(this.args)))

	for i = range this.args {
		sink = sink.WriteString16(this.args[i])
	}

	sink = sink.WriteString16(this.cwd).
		WriteUint16(uint16(len(this.env)))

	for key, value = range this.env {
		sink = sink.WriteString8(key).WriteString16(value)
	}

	sink = sink.WriteUint8(this.shell).
		WriteUint16(uint16(len(this.sources)))

	for i = range this.sources {
		sink = sink.WriteString16(this.sources[i])
	}

	sink = sink.WriteUint16(uint16(len(this.keys)))

	for i = range this.keys {
		sink = sink.WriteEncodable(this.keys[i])
	}

	return sink.Error()
}

func (this *Message) Decode(source sio.Source) error {
	var key, value string
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
		ReadUint16(&n).AndThen(func () error {
			this.env = make(map[string]string)

			for i = 0; i < int(n); i++ {
				source = source.ReadString8(&key).
					ReadString16(&value).
					And(func () { this.env[key] = value })
			}

			return source.Error()
		}).
		ReadUint8(&this.shell).
		ReadUint16(&n).AndThen(func () error {
			this.sources = make([]string, n)

			for i = range this.sources {
				source = source.ReadString16(&this.sources[i])
			}

			return source.Error()
		}).
		ReadUint16(&n).AndThen(func () error {
			this.keys = make([]kv.Key, n)

			for i = range this.keys {
				this.keys[i], source = kv.ReadKey(source)
			}

			return source.Error()
		}).
		Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


var protocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	  0: &serviceMeta{},
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


type serviceMeta struct {
	name string
	values []kv.Value
}

func (this *serviceMeta) Encode(sink sio.Sink) error {
	var i int

	if len(this.values) > MaxJobKeys {
		return &JobTooManyValuesError{ this.values }
	}

	sink = sink.WriteString8(this.name).
		WriteUint16(uint16(len(this.values)))

	for i = range this.values {
		sink = sink.WriteEncodable(this.values[i])
	}

	return sink.Error()
}

func (this *serviceMeta) Decode(source sio.Source) error {
	var n uint16

	return source.ReadString8(&this.name).
		ReadUint16(&n).AndThen(func () error {
			var i int

			this.values = make([]kv.Value, n)

			for i = range this.values {
				this.values[i], source = kv.ReadValue(source)
			}

			return source.Error()
		}).
		Error()
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


func signalCode(sig syscall.Signal) (uint8, error) {
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

func codeSignal(scode uint8) (syscall.Signal, error) {
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
	default: return 0, &JobUnknownSignalCodeError{ scode }
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *JobNameTooLongError) Error() string {
	return fmt.Sprintf("job name too long: %s", this.Name)
}

func (this *JobTooManyArgumentsError) Error() string {
	return fmt.Sprintf("job has too many arguments: %d", len(this.Args))
}

func (this *JobTooManySourcesError) Error() string {
	return fmt.Sprintf("job has too many sources: %d", len(this.Sources))
}

func (this *JobArgumentTooLongError) Error() string {
	return fmt.Sprintf("job argument too long: %s", this.Arg)
}

func (this *JobSourceTooLongError) Error() string {
	return fmt.Sprintf("job source path too long: %s", this.Source)
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

func (this *JobTooManyEnvVariablesError) Error() string {
	return fmt.Sprintf("job has too many environment variables: %d",
		len(this.Env))
}

func (this *JobTooManyKeysError) Error() string {
	return fmt.Sprintf("job has too many keys: %d", len(this.Keys))
}

func (this *JobTooManyValuesError) Error() string {
	return fmt.Sprintf("job has too many values: %d", len(this.Values))
}

func (this *JobInvalidEnvKeyError) Error() string {
	return fmt.Sprintf("job environment variable name invalid: %s",
		this.Key)
}

func (this *JobInvalidEnvValueError) Error() string {
	return fmt.Sprintf("job environment variable value invalid: %s (%s)",
		this.Value, this.Key)
}
