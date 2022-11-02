package net


import (
	"sync"
)


// ----------------------------------------------------------------------------


// Receive `Message`s encoded with a given `Protocol`.
//
type Receiver interface {
	// Return a channel to receive the next `Message`s and decode them with
	// the provided `Protocol`.
	//
	Recv(Protocol) <-chan Message

	// Return a channel to receive the provided number of next `Message`s
	// and decode them with the provided `Protocol`.
	//
	RecvN(Protocol, int) <-chan Message
}


// Return a new aggregator over the pairs receivers/protocol sent on the given
// `recvc` channel.
//
// Receivers sent on the `recvc` channel immediately start to `Recv` with the
// protocol they are associated.
//
// Receiving from the aggregator for a given `Protocol` return a `Message` from
// the first `Receiver` sent on `recvc` with the corresponding `Protocol`.
//
// If all the received `Receiver`s for a given `Protocol` have been closed and
// `recvc` has been closed then the returned channel for this `Protocol` is
// closed.
//
func NewReceiverAggregator(recvc <-chan ReceiverProtocol) Receiver {
	return newReceiverAggregator(recvc)
}

func NewSliceReceiverAggregator(recvs []ReceiverProtocol) Receiver {
	var c chan ReceiverProtocol = make(chan ReceiverProtocol, len(recvs))
	var rp ReceiverProtocol

	for _, rp = range recvs {
		c <- rp
	}

	close(c)

	return NewReceiverAggregator(c)
}

// A simple pair of `Receiver` and `Protocol`.
//
type ReceiverProtocol struct {
	R Receiver
	P Protocol
}


// Just an alias for `recv.RecvN(proto, 1)`.
//
func RecvOne(recv Receiver, proto Protocol) <-chan Message {
	return recv.RecvN(proto, 1)
}


// ----------------------------------------------------------------------------


type receiverAggregator struct {
	lock sync.Mutex
	rcs map[Protocol]*receiverAggregatorChannel
	adding sync.WaitGroup
}

func newReceiverAggregator(recvc <-chan ReceiverProtocol) *receiverAggregator {
	var this receiverAggregator

	this.rcs = make(map[Protocol]*receiverAggregatorChannel)
	this.adding.Add(1)

	go this.run(recvc)

	return &this
}

func (this *receiverAggregator) run(recvc <-chan ReceiverProtocol) {
	var rp ReceiverProtocol
	var rac *receiverAggregatorChannel

	for rp = range recvc {
		rac = this.protocolChan(rp.P)
		rac.using.Add(1)
		go this.recv(rp, rac)
	}

	this.adding.Done()
}

func (this *receiverAggregator) recv(rp ReceiverProtocol, rac *receiverAggregatorChannel) {
	var msg Message

	for msg = range rp.R.Recv(rp.P) {
		rac.c <- msg
	}

	rac.using.Done()
}

func (this *receiverAggregator) protocolChan(proto Protocol) *receiverAggregatorChannel {
	var rac *receiverAggregatorChannel
	var found bool

	this.lock.Lock()
	defer this.lock.Unlock()

	rac, found = this.rcs[proto]
	if found == false {
		rac = newReceiverAggregatorChannel(&this.adding)
		this.rcs[proto] = rac
	}

	return rac
}

func (this *receiverAggregator) Recv(proto Protocol) <-chan Message {
	return this.protocolChan(proto).c
}

func (this *receiverAggregator) RecvN(proto Protocol, n int) <-chan Message {
	var c chan Message

	if n <= 0 {
		panic("receive negative or zero messages")
	}

	c = make(chan Message, n)

	go func () {
		var msgc <-chan Message = this.protocolChan(proto).c
		var msg Message
		var more bool

		for {
			if n == 0 {
				break
			}

			msg, more = <-msgc
			if more == false {
				break
			}

			c <- msg

			n -= 1
		}

		close(c)
	}()

	return c
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type receiverAggregatorChannel struct {
	c chan Message
	adding *sync.WaitGroup
	using sync.WaitGroup
}

func newReceiverAggregatorChannel(adding *sync.WaitGroup) *receiverAggregatorChannel {
	var this receiverAggregatorChannel

	this.c = make(chan Message)
	this.adding = adding

	go this.run()

	return &this
}

func (this *receiverAggregatorChannel) run() {
	this.adding.Wait()
	this.using.Wait()
	close(this.c)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -
