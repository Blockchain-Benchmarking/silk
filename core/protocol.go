package core


import (
	"math"
	sio "silk/io"
	"silk/net"
)


// ----------------------------------------------------------------------------


var protocol net.Protocol = net.NewUint8Protocol(map[uint8]net.Message{
	0:   &closeMessage{},
	1:   &errorMessage{},
	2:   &nameRequestMessage{},
	3:   &nameReplyMessage{},
	4:   &runMessage{},
	5:   &exitMessage{},
	6:   &stdinMessage{},
	7:   &stdoutMessage{},
	8:   &stderrMessage{},
	9:   &closeStdinMessage{},
	10:  &closeStdoutMessage{},
	11:  &closeStderrMessage{},
	100: &net.RoutingMessage{},
})


type closeMessage struct {
	emptyMessage
}


type errorMessage struct {
	text string
}

func (this *errorMessage) Encode(sink sio.Sink) error {
	var text string

	if len(this.text) > math.MaxUint16 {
		text = this.text[:math.MaxUint16-3] + "..."
	} else {
		text = this.text
	}

	return sink.WriteString16(text).Error()
}

func (this *errorMessage) Decode(source sio.Source) error {
	return source.ReadString16(&this.text).Error()
}


type nameRequestMessage struct {
	emptyMessage
}


type nameReplyMessage struct {
	name string
}

func (this *nameReplyMessage) Encode(sink sio.Sink) error {
	if len(this.name) > MaxNameLength {
		return &NameTooLongError{ this.name }
	}

	return sink.WriteString16(this.name).Error()
}

func (this *nameReplyMessage) Decode(source sio.Source) error {
	return source.ReadString16(&this.name).Error()
}


type runMessage struct {
	name string
	args []string
}

func (this *runMessage) Encode(sink sio.Sink) error {
	var arg string

	if len(this.args) > MaxArgNumber {
		return &TooManyArgumentsError{ this.args }
	}

	for _, arg = range this.args {
		if len(arg) > MaxArgLength {
			return &ArgumentTooLongError{ arg }
		}
	}

	sink = sink.WriteString16(this.name).
		WriteUint16(uint16(len(this.args)))

	for _, arg = range this.args {
		sink = sink.WriteString16(arg)
	}

	return sink.Error()
}

func (this *runMessage) Decode(source sio.Source) error {
	var n uint16
	var i int

	return source.ReadString16(&this.name).ReadUint16(&n).And(func () {
		this.args = make([]string, n)
		for i = range this.args {
			source = source.ReadString16(&this.args[i])
		}
	}).Error()
}


type exitMessage struct {
	code uint8
}

func (this *exitMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint8(this.code).Error()
}

func (this *exitMessage) Decode(source sio.Source) error {
	return source.ReadUint8(&this.code).Error()
}


type stdinMessage struct {
	content []byte
}

func (this *stdinMessage) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *stdinMessage) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type stdoutMessage struct {
	content []byte
}

func (this *stdoutMessage) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *stdoutMessage) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type stderrMessage struct {
	content []byte
}

func (this *stderrMessage) Encode(sink sio.Sink) error {
	return sink.WriteBytes32(this.content).Error()
}

func (this *stderrMessage) Decode(source sio.Source) error {
	return source.ReadBytes32(&this.content).Error()
}


type closeStdinMessage struct {
	emptyMessage
}


type closeStdoutMessage struct {
	emptyMessage
}


type closeStderrMessage struct {
	emptyMessage
}


type emptyMessage struct {
}

func (this *emptyMessage) Encode(sink sio.Sink) error {
	return sink.Error()
}

func (this *emptyMessage) Decode(source sio.Source) error {
	return source.Error()
}
