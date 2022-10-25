package net


import (
	"bytes"
	"io"
	sio "silk/io"
	"testing"
	"time"
)


type bindingTestEmptyMessage struct {
}

func (this *bindingTestEmptyMessage) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *bindingTestEmptyMessage) Decode(source sio.Source) error {
	return source.Error()
}

var bindingTestEmptyProtocol = NewRawProtocol(&bindingTestEmptyMessage{})


type bindingTestPadProtocol struct {
	pad []byte
}

func newBindingTestPadProtocol(mlen int) *bindingTestPadProtocol {
	return &bindingTestPadProtocol{ pad: make([]byte, mlen) }
}

func (this *bindingTestPadProtocol) Encode(sink sio.Sink, msg Message) error {
	return sink.WriteBytes(this.pad).Error()
}

func (this *bindingTestPadProtocol) Decode(source sio.Source) (Message, error){
	return nil, source.ReadBytes(this.pad).Error()
}


type connectionRttMock struct {
	rtt time.Duration
	sending chan<- []byte
	receiving <-chan []byte
}

func newConnectionRttMocks(rtt time.Duration) (*connectionRttMock, *connectionRttMock) {
	var left, right chan []byte

	left = make(chan []byte, 32)
	right = make(chan []byte, 32)

	return &connectionRttMock{
		rtt: rtt,
		sending: left,
		receiving: right,
	}, &connectionRttMock{
		rtt: rtt,
		sending: right,
		receiving: left,
	}
}

func (this *connectionRttMock) Send(msg Message, proto Protocol) error {
	var buf bytes.Buffer
	var err error

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	go func () {
		time.Sleep(this.rtt)
		this.sending <- buf.Bytes()
	}()

	return nil
}

func (this *connectionRttMock) Recv(proto Protocol) (Message, error) {
	var buf *bytes.Buffer
	var b []byte
	var ok bool

	b, ok = <-this.receiving
	if !ok {
		return nil, io.EOF
	}

	buf = bytes.NewBuffer(b)

	return proto.Decode(sio.NewReaderSource(buf))
}

func (this *connectionRttMock) Close() error {
	close(this.sending)
	return nil
}


func benchmarkBindingOneWay(b *testing.B, mlen int, rtt time.Duration) {
	var connA, connB *connectionRttMock
	var bindA, bindB PassiveBinding
	var done chan struct{}
	var proto Protocol
	var msg Message
	var i int

	if mlen == 0 {
		msg = &bindingTestEmptyMessage{}
		proto = bindingTestEmptyProtocol
	} else {
		msg = nil
		proto = newBindingTestPadProtocol(mlen)
	}

	for i = 0; i < b.N; i++ {
		connA, connB = newConnectionRttMocks(rtt)

		bindA = NewPassiveBinding()
		bindB = NewPassiveBinding()

		bindA.Reconnect(connA)
		bindB.Reconnect(connB)

		done = make(chan struct{})

		go func () {
			var j int

			for j = 0; j < 10; j++ {
				bindA.Send(msg, proto)
			}

			done <- struct{}{}
		}()

		go func () {
			var j int

			for j = 0; j < 10; j++ {
				bindB.Recv(proto)
			}

			done <- struct{}{}
		}()

		<-done
		<-done

		close(done)

		bindA.Disconnect().Close()
		bindB.Disconnect().Close()
	}
}

func BenchmarkBindingOneWayEmptyNoRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 0, 0)
}

func BenchmarkBindingOneWayEmptySmallRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 0, 10 * time.Millisecond)
}

func BenchmarkBindingOneWayEmptyBigRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 0, 500 * time.Millisecond)
}

func BenchmarkBindingOneWayFatNoRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 2097152, 0)
}

func BenchmarkBindingOneWayFatSmallRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 2097152, 10 * time.Millisecond)
}

func BenchmarkBindingOneWayFatBigRtt(b *testing.B) {
	benchmarkBindingOneWay(b, 2097152, 500 * time.Millisecond)
}
