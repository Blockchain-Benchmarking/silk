package core


import (
	"context"
	sio "silk/io"
	"silk/net"
	"silk/run"
)


// ----------------------------------------------------------------------------


type Job interface {
	NewAgent() (Agent, error)
	NewAgentWith(opts *AgentOptions) (Agent, error)

	Stdin(data []byte) error

	CloseStdin()

	Wait()
}

func NewJob(name string, args, target []string) (Job, error) {
	return NewJobWith(name, args, target, nil)
}

func NewJobWith(name string,args,target []string,opts *JobOptions)(Job,error) {
	if opts == nil {
		opts = &JobOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	if opts.Context == nil {
		opts.Context = context.Background()
	}

	if opts.Resolver == nil {
		opts.Resolver = net.NewTcpResolver(opts.Protocol)
	}

	return newJob(name, args, target, opts), nil
}

type JobOptions struct {
	Log sio.Logger

	Context context.Context

	Resolver net.Resolver

	Protocol net.Protocol
}


type Agent interface {
	run.Process

	Name() string
}

type AgentOptions struct {
	run.ProcessOptions
}


// ----------------------------------------------------------------------------


type job struct {
}

func newJob(name string, args, target []string, opts *JobOptions) *job {
	var this job

	return &this
}

func (this *job) NewAgent() (Agent, error) {
	return this.NewAgentWith(nil)
}

func (this *job) NewAgentWith(opts *AgentOptions) (Agent, error) {
	if opts == nil {
		opts = &AgentOptions{}
	}

	return newAgent(this, opts), nil
}

func (this *job) Stdin(data []byte) error {
	return nil
}

func (this *job) CloseStdin() {
}

func (this *job) Wait() {
}


type agent struct {
}

func newAgent(parent *job, opts *AgentOptions) *agent {
	var this agent

	return &this
}

func (this *agent) Exit() uint8 {
	return 0
}

func (this *agent) Stdin(data []byte) error {
	return nil
}

func (this *agent) CloseStdin() {
}

func (this *agent) Wait() {
}

func (this *agent) Name() string {
	return ""
}
