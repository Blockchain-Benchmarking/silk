package io


import (
	"io"
)


// ----------------------------------------------------------------------------


const ChannelMaxBuffer = (1 << 21)
const ChannelMinBuffer = (1 << 21)


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func ReadInChannel(reader io.Reader, dest chan<- []byte) {
	var b []byte = make([]byte, ChannelMaxBuffer)
	var err error
	var n int

	for {
		if len(b) < ChannelMinBuffer {
			b = make([]byte, ChannelMaxBuffer)
		}

		n, err = reader.Read(b)

		if n > 0 {
			dest <- b[:n]
			b = b[n:]
		}

		if err != nil {
			break
		}
	}

	close(dest)
}

func NewReaderChannel(reader io.Reader) <-chan []byte {
	var c chan []byte = make(chan []byte, 128)

	go ReadInChannel(reader, c)

	return c
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func WriteFromChannel(writer io.Writer, c <-chan []byte) {
	var err error
	var b []byte

	for b = range c {
		_, err = writer.Write(b)
		if err != nil {
			break
		}
	}

	for _ = range c {}
}

func NewWriterChannel(writer io.Writer) chan<- []byte {
	var c chan []byte = make(chan []byte, 128)

	go WriteFromChannel(writer, c)

	return c
}


// ----------------------------------------------------------------------------
