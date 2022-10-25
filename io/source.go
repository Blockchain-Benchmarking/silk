package io


import (
	"encoding/binary"
	"io"
)


// ----------------------------------------------------------------------------


type Source interface {
	ReadUint8(data *uint8) Source
	ReadUint16(data *uint16) Source
	ReadUint32(data *uint32) Source
	ReadUint64(data *uint64) Source

	ReadString(data *string, n int) Source
	ReadString8(data *string) Source
	ReadString16(data *string) Source

	ReadBytes(data []byte) Source
	ReadBytes8(data *[]byte) Source
	ReadBytes16(data *[]byte) Source
	ReadBytes32(data *[]byte) Source

	ReadDecodable(data Decodable) Source

	And(call func ()) Source
	AndThen(call func () error) Source

	Error() error
}

type Decodable interface {
	Decode(source Source) error
}

func NewErrorSource(err error) Source {
	return WrapSource(NewErrorBaseSource(err))
}

func NewReaderSource(reader io.Reader) Source {
	return WrapSource(NewReaderBaseSource(reader))
}


type BaseSource interface {
	ReadBytes(data []byte) BaseSource
	Error() error
}

func WrapSource(base BaseSource) Source {
	return wrapSource(base)
}

func NewErrorBaseSource(err error) BaseSource {
	return newErrorSource(err)
}

func NewReaderBaseSource(reader io.Reader) BaseSource {
	return newReaderSource(reader)
}


// ----------------------------------------------------------------------------


type errorSource struct {
	err error
}

func newErrorSource(err error) *errorSource {
	return &errorSource{ err }
}

func (this *errorSource) ReadBytes(data []byte) BaseSource {
	return this
}

func (this *errorSource) Error() error {
	return this.err
}


type readerSource struct {
	reader io.Reader
}

func newReaderSource(reader io.Reader) *readerSource {
	return &readerSource{ reader }
}

func (this *readerSource) ReadBytes(data []byte) BaseSource {
	var err error

	_, err = io.ReadFull(this.reader, data)
	if err != nil {
		return newErrorSource(err)
	}

	return this
}

func (this *readerSource) Error() error {
	return nil
}


type wrappedSource struct {
	inner BaseSource
}

func wrapSource(base BaseSource) *wrappedSource {
	return &wrappedSource{ base }
}

func (this *wrappedSource) ReadUint8(data *uint8) Source {
	var buf []byte = []byte { 0 }
	this.inner = this.inner.ReadBytes(buf)
	*data = buf[0]
	return this
}

func (this *wrappedSource) ReadUint16(data *uint16) Source {
	var buf []byte = []byte{ 0, 0 }
	this.inner = this.inner.ReadBytes(buf)
	*data = binary.BigEndian.Uint16(buf)
	return this
}

func (this *wrappedSource) ReadUint32(data *uint32) Source {
	var buf []byte = []byte{ 0, 0, 0, 0 }
	this.inner = this.inner.ReadBytes(buf)
	*data = binary.BigEndian.Uint32(buf)
	return this
}

func (this *wrappedSource) ReadUint64(data *uint64) Source {
	var buf []byte = []byte{ 0, 0, 0, 0, 0, 0, 0, 0 }
	this.inner = this.inner.ReadBytes(buf)
	*data = binary.BigEndian.Uint64(buf)
	return this
}

func (this *wrappedSource) ReadString(data *string, n int) Source {
	var buf []byte
	if this.inner.Error() != nil {
		return this
	}
	buf = make([]byte, n)
	this.inner = this.inner.ReadBytes(buf)
	*data = string(buf)
	return this
}

func (this *wrappedSource) ReadString8(data *string) Source {
	var n uint8
	return this.ReadUint8(&n).ReadString(data, int(n))
}

func (this *wrappedSource) ReadString16(data *string) Source {
	var n uint16
	return this.ReadUint16(&n).ReadString(data, int(n))
}

func (this *wrappedSource) ReadBytes(data []byte) Source {
	this.inner = this.inner.ReadBytes(data)
	return this
}

func (this *wrappedSource) ReadBytes8(data *[]byte) Source {
	var buf []byte
	var n uint8
	this.ReadUint8(&n)
	if this.Error() == nil {
		buf = make([]byte, int(n))
		this.ReadBytes(buf)
		*data = buf
	}
	return this
}

func (this *wrappedSource) ReadBytes16(data *[]byte) Source {
	var buf []byte
	var n uint16
	this.ReadUint16(&n)
	if this.Error() == nil {
		buf = make([]byte, int(n))
		this.ReadBytes(buf)
		*data = buf
	}
	return this
}

func (this *wrappedSource) ReadBytes32(data *[]byte) Source {
	var buf []byte
	var n uint32
	this.ReadUint32(&n)
	if this.Error() == nil {
		buf = make([]byte, int(n))
		this.ReadBytes(buf)
		*data = buf
	}
	return this
}

func (this *wrappedSource) ReadDecodable(data Decodable) Source {
	var err error
	if this.Error() == nil {
		err = data.Decode(this)
		if err != nil {
			this.inner = newErrorSource(err)
		}
	}
	return this
}

func (this *wrappedSource) And(call func ()) Source {
	if this.inner.Error() == nil {
		call()
	}
	return this
}

func (this *wrappedSource) AndThen(call func () error) Source {
	var err error
	if this.inner.Error() == nil {
		err = call()
		if err != nil {
			this.inner = newErrorSource(err)
		}
	}
	return this
}

func (this *wrappedSource) Error() error {
	return this.inner.Error()
}
