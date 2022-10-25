package io


import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)


// ----------------------------------------------------------------------------


type Sink interface {
	WriteUint8(data uint8) Sink
	WriteUint16(data uint16) Sink
	WriteUint32(data uint32) Sink
	WriteUint64(data uint64) Sink

	WriteString(data string) Sink
	WriteString8(data string) Sink
	WriteString16(data string) Sink

	WriteBytes(data []byte) Sink
	WriteBytes8(data []byte) Sink
	WriteBytes16(data []byte) Sink
	WriteBytes32(data []byte) Sink

	WriteEncodable(data Encodable) Sink

	Error() error
}

type Encodable interface {
	Encode(sink Sink) error
}

func NewErrorSink(err error) Sink {
	return WrapSink(NewErrorBaseSink(err))
}

func NewWriterSink(writer io.Writer) Sink {
	return WrapSink(NewWriterBaseSink(writer))
}


type BaseSink interface {
	WriteBytes(data []byte) BaseSink
	WriteString(data string) BaseSink
	Error() error
}

func WrapSink(base BaseSink) Sink {
	return wrapSink(base)
}

func NewErrorBaseSink(err error) BaseSink {
	return newErrorSink(err)
}

func NewWriterBaseSink(writer io.Writer) BaseSink {
	return newWriterSink(writer)
}


type SinkOverflowError struct {
	maxSize int
	actualSize int
}



// ----------------------------------------------------------------------------


type errorSink struct {
	err error
}

func newErrorSink(err error) *errorSink {
	var this errorSink

	this.err = err

	return &this
}

func (this *errorSink) WriteBytes(data []byte) BaseSink {
	return this
}

func (this *errorSink) WriteString(data string) BaseSink {
	return this
}

func (this *errorSink) Error() error {
	return this.err
}


type writerSink struct {
	writer io.Writer
}

func newWriterSink(writer io.Writer) *writerSink {
	var this writerSink

	this.writer = writer

	return &this
}

func (this *writerSink) WriteBytes(data []byte) BaseSink {
	var err error

	_, err = this.writer.Write(data)
	if err != nil {
		return newErrorSink(err)
	}

	return this
}

func (this *writerSink) WriteString(data string) BaseSink {
	var err error

	_, err = io.WriteString(this.writer, data)
	if err != nil {
		return newErrorSink(err)
	}

	return this
}

func (this *writerSink) Error() error {
	return nil
}


type wrappedSink struct {
	inner BaseSink
}

func wrapSink(base BaseSink) *wrappedSink {
	return &wrappedSink{ base }
}

func (this *wrappedSink) WriteUint8(data uint8) Sink {
	var buf []byte = []byte{ data }
	return this.WriteBytes(buf)
}

func (this *wrappedSink) WriteUint16(data uint16) Sink {
	var buf []byte = []byte{ 0, 0 }
	binary.BigEndian.PutUint16(buf, data)
	return this.WriteBytes(buf)
}

func (this *wrappedSink) WriteUint32(data uint32) Sink {
	var buf []byte = []byte{ 0, 0, 0, 0 }
	binary.BigEndian.PutUint32(buf, data)
	return this.WriteBytes(buf)
}

func (this *wrappedSink) WriteUint64(data uint64) Sink {
	var buf []byte = []byte{ 0, 0, 0, 0, 0, 0, 0, 0 }
	binary.BigEndian.PutUint64(buf, data)
	return this.WriteBytes(buf)
}

func (this *wrappedSink) WriteString(data string) Sink {
	this.inner = this.inner.WriteString(data)
	return this
}

func (this *wrappedSink) WriteString8(data string) Sink {
	if len(data) > math.MaxUint8 {
		this.inner = newErrorSink(&SinkOverflowError{
			len(data),
			int(math.MaxUint8),
		})
	}

	return this.WriteUint8(uint8(len(data))).WriteString(data)
}

func (this *wrappedSink) WriteString16(data string) Sink {
	if len(data) > math.MaxUint16 {
		this.inner = newErrorSink(&SinkOverflowError{
			len(data),
			int(math.MaxUint16),
		})
	}

	return this.WriteUint16(uint16(len(data))).WriteString(data)
}

func (this *wrappedSink) WriteBytes(data []byte) Sink {
	this.inner = this.inner.WriteBytes(data)
	return this
}

func (this *wrappedSink) WriteBytes8(data []byte) Sink {
	if len(data) > math.MaxUint8 {
		this.inner = newErrorSink(&SinkOverflowError{
			len(data),
			int(math.MaxUint8),
		})
	}

	return this.WriteUint8(uint8(len(data))).WriteBytes(data)
}

func (this *wrappedSink) WriteBytes16(data []byte) Sink {
	if len(data) > math.MaxUint16 {
		this.inner = newErrorSink(&SinkOverflowError{
			len(data),
			int(math.MaxUint16),
		})
	}

	return this.WriteUint16(uint16(len(data))).WriteBytes(data)
}

func (this *wrappedSink) WriteBytes32(data []byte) Sink {
	if len(data) > math.MaxUint32 {
		this.inner = newErrorSink(&SinkOverflowError{
			len(data),
			int(math.MaxUint32),
		})
	}

	return this.WriteUint32(uint32(len(data))).WriteBytes(data)
}

func (this *wrappedSink) WriteEncodable(data Encodable) Sink {
	var err error = data.Encode(this)
	if err != nil {
		this.inner = newErrorSink(err)
	}
	return this
}

func (this *wrappedSink) Error() error {
	return this.inner.Error()
}


func (this *SinkOverflowError) Error() string {
	return fmt.Sprintf("data to large (%d > %d)", this.actualSize,
		this.maxSize)
}
