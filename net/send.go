package net


import (
	"silk/util/atomic"
)


// ----------------------------------------------------------------------------


// Send `Message`s encoded with a given `Protocol`.
//
type Sender interface {
	// Return a channel to send pairs of `Message` and `Protocol`.
	// The `Message`s sent to this channel are encoded with their
	// associated `Protocol` before to be sent.
	// The user is responsibe for closing the channel when there is nothing
	// more to send.
	//
	Send() chan<- MessageProtocol
}


// Return a new broadcaster over the senders sent on the given `senderc`
// channel.
//
// Sending on the broadcaster calls `Send` on all the senders eventually
// received from `senderc`.
// Especially, a call to `Send` even calls `Send` on the elements which are
// sent concurrently to the `senderc` channel.
//
// A call to `Send` may block until the given `senderc` is closed.
//
// Closing the brodcaster causes it to close all the `Sender`s ever received or
// to be received later on `senderc`.
//
func NewBroadcaster(senderc <-chan Sender) Sender {
	return newBroadcaster(senderc)
}


// Return a new aggregator over the senders sent on the given `senderc`
// channel.
//
// Sending on the aggregator calls `Send` on one of the sender eventually
// received from `senderc`.
// Especially, a call to `Send` may call `Send` on the elements which are sent
// concurrently to the `senderc` channel.
//
// A call to `Send` may block until the given `senderc` is closed.
//
// If `senderc` is closed before any `Sender` is sent then a call to this
// aggregator `Send` blocks forever.
//
// Closing the aggregator causes it to close all the `Sender`s ever received or
// to be received later on `senderc`.
//
func NewSenderAggregator(senderc <-chan Sender) Sender {
	return newSenderQueueAggregator(senderc)
}

func NewSliceSenderAggregator(senders []Sender) Sender {
	var senderc chan Sender = make(chan Sender, len(senders))
	var sender Sender

	for _, sender = range senders {
		senderc <- sender
	}

	close(senderc)

	return NewSenderAggregator(senderc)
}


// A `Sender` with the ability to `Dup`licate i.e. clone itself so both copies
// send to the same target.
//
type SenderDup interface {
	Sender

	// Create a duplicate of this sender.
	// Calling `Dup` on a closed `SenderDup` is undefined behavior.
	//
	// There is no sequential consistency between calls to `Send` of two
	// duplicates of the same `Sender`.
	// Specifically, the following situation can occur:
	//
	//     var s0 SenderDup = ...
	//     var s1 = s0.Dup()
	//
	//     s0.Send() <- mp0
	//     s0.Send() <- mp1
	//     s1.Send() <- mp2
	//
	// The remote end of this `Sender` can receive `mp2` before to receive
	// `mp0`. However `mp0` will always be received before `mp1`.
	//
	Dup() SenderDup
}


// Return a new `SenderDup` of the given `Sender`.
// This essentially adds a reference counter on the given `Sender` and closes
// the given `Sender` automatically when the reference falls to 0.
//
func NewSendScatter(inner Sender) SenderDup {
	return newSenderReference(inner)
}


// ----------------------------------------------------------------------------


type broadcaster struct {
	sendc chan MessageProtocol
}

func newBroadcaster(senderc <-chan Sender) *broadcaster {
	var this broadcaster

	this.sendc = make(chan MessageProtocol)

	go this.run(senderc)

	return &this
}

func (this *broadcaster) run(senderc <-chan Sender) {
	var mp MessageProtocol
	var senders []Sender
	var sender Sender

	for sender = range senderc {
		senders = append(senders, sender)
	}

	for mp = range this.sendc {
		for _, sender = range senders {
			sender.Send() <- mp
		}
	}

	for _, sender = range senders {
		close(sender.Send())
	}
}

func (this *broadcaster) Send() chan<- MessageProtocol {
	return this.sendc
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type senderQueueAggregator struct {
	sendc chan MessageProtocol
}

func newSenderQueueAggregator(senderc <-chan Sender) *senderQueueAggregator {
	var this senderQueueAggregator

	this.sendc = make(chan MessageProtocol)

	go this.run(senderc)

	return &this
}

func (this *senderQueueAggregator) run(senderc <-chan Sender) {
	var sender Sender

	for sender = range senderc {
		go this.transfer(sender)
	}
}

func (this *senderQueueAggregator) transfer(sender Sender) {
	var mp MessageProtocol

	for mp = range this.sendc {
		sender.Send() <- mp
	}

	close(sender.Send())
}

func (this *senderQueueAggregator) Send() chan<- MessageProtocol {
	return this.sendc
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type senderReference struct {
	inner Sender
	arc *atomic.Int64
	sendc chan MessageProtocol
}

func newSenderReference(inner Sender) *senderReference {
	var this senderReference

	this.inner = inner
	this.arc = &atomic.Int64{}
	this.sendc = make(chan MessageProtocol)

	this.arc.Store(1)

	go this.run()

	return &this
}

func (this *senderReference) run() {
	var mp MessageProtocol

	for mp = range this.sendc {
		this.inner.Send() <- mp
	}

	if this.arc.Add(-1) <= 0 {
		close(this.inner.Send())
	}
}

func (this *senderReference) Send() chan<- MessageProtocol {
	return this.sendc
}

func (this *senderReference) Dup() SenderDup {
	var ret senderReference

	if this.arc.Add(1) <= 0 {
		panic("duplicate dropped sender")
	}

	ret.inner = this.inner
	ret.arc = this.arc
	ret.sendc = make(chan MessageProtocol)

	go ret.run()

	return &ret
}
