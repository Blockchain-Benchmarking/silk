package net


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


func RecvOne(recv Receiver, proto Protocol) <-chan Message {
	return recv.RecvN(proto, 1)
}


// ----------------------------------------------------------------------------
