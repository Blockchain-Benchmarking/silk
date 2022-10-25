package net


// ----------------------------------------------------------------------------


type Channel interface {
	// Send the given `Message` encoded with the given `Protocol`.
	// Return `nil` if the message is sent successfully or an error
	// otherwise.
	//
	// Concurrent calls to `Send` are safe: concurrently sent `Message`s
	// cannot be interleaved.
	//
	Send(Message, Protocol) error

	// Receive the next `Message` decded with the given `Protocol`.
	// Return the `Message` and `nil` if the message is received correctly
	// otherwise return an unspecified value and and error.
	//
	// Concurrent calls to `Recv` are safe: concurrently received
	// `Message`s cannot be interleaved.
	//
	Recv(Protocol) (Message, error)
}


// ----------------------------------------------------------------------------
