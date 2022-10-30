package net


import (
	"sync"
)


// ----------------------------------------------------------------------------


// Accept incoming `Connection`s.
//
type Accepter interface {
	// Return a channel to receive accepted `Connection`s.
	// The channel gets closed when there is no more connection to accept.
	//
	Accept() <-chan Connection
}


// Return a unique `Accepter` to receive the connections accepted from all
// elements sent to the given `accepterc` channel.
//
func NewAcceptGather(accepterc <-chan Accepter) Accepter {
	return newConcentrator(accepterc)
}

func NewSliceAcceptGather(accepters []Accepter) Accepter {
	var accepterc chan Accepter = make(chan Accepter, len(accepters))
	var accepter Accepter

	for _, accepter = range accepters {
		accepterc <- accepter
	}

	close(accepterc)

	return NewAcceptGather(accepterc)
}


func AcceptAll(accepters ...Accepter) <-chan Connection {
	return NewSliceAcceptGather(accepters).Accept()
}


// ----------------------------------------------------------------------------


type concentrator struct {
	acceptc chan Connection
}

func newConcentrator(accepterc <-chan Accepter) *concentrator {
	var this concentrator

	this.acceptc = make(chan Connection, 16)

	go this.run(accepterc)

	return &this
}

func (this *concentrator) run(accepterc <-chan Accepter) {
	var accepting sync.WaitGroup
	var accepter Accepter

	for accepter = range accepterc {
		accepting.Add(1)
		go this.accept(accepter, &accepting)
	}

	accepting.Wait()

	close(this.acceptc)
}

func (this *concentrator) accept(accepter Accepter,accepting *sync.WaitGroup) {
	var conn Connection

	for conn = range accepter.Accept() {
		this.acceptc <- conn
	}

	accepting.Done()
}

func (this *concentrator) Accept() <-chan Connection {
	return this.acceptc
}
