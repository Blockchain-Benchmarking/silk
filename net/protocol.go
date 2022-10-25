package net


import (
	"fmt"
	"silk/io"
	"reflect"
)


// ----------------------------------------------------------------------------


type Message interface {
	io.Decodable
	io.Encodable
}

type Protocol interface {
	Encode(io.Sink, Message) error
	Decode(io.Source) (Message, error)
}


func NewRawProtocol(recv Message) Protocol {
	return newRawProtocol(recv)
}

func NewUint8Protocol(hmap map[uint8]Message) Protocol {
	return newUint8Protocol(hmap)
}


// ----------------------------------------------------------------------------


type rawProtocol struct {
	recv Message
	used bool
	rtype reflect.Type
}

func newRawProtocol(recv Message) *rawProtocol {
	return &rawProtocol{ recv, false, nil }
}

func (this *rawProtocol) Encode(sink io.Sink, msg Message) error {
	return sink.WriteEncodable(msg).Error()
}

func (this *rawProtocol) Decode(source io.Source) (Message, error) {
	var msg Message
	var err error

	if this.used {
		if this.rtype == nil {
			this.rtype = reflect.ValueOf(this.recv).Type().Elem()
			this.recv = nil
		}

		msg = reflect.New(this.rtype).Interface().(Message)
	} else {
		this.used = true
		msg = this.recv
	}

	err = source.ReadDecodable(msg).Error()
	if err != nil {
		return nil, err
	}

	return msg, nil
}


type bytesTypesProtocol struct {
	h2m map[string]reflect.Type
	m2h map[reflect.Type][]byte
	hlen int
}

func newBytesTypesProtocol(bs [][]byte, ts []reflect.Type) *bytesTypesProtocol{
	var this bytesTypesProtocol
	var t reflect.Type
	var found bool
	var s string
	var b []byte
	var i int

	if (len(bs) != len(ts)) || (len(bs) == 0) {
		panic(fmt.Sprintf("invalid parameter sizes: %d %d", len(bs),
			len(ts)))
	}

	this.h2m = make(map[string]reflect.Type)
	this.m2h = make(map[reflect.Type][]byte)

	for i = range bs {
		if i == 0 {
			this.hlen = len(bs[i])
		} else if this.hlen != len(bs[i]) {
			panic(fmt.Sprintf("incompatible headers: %d:%v %d:%v",
				this.hlen, bs[0], len(bs[i]), bs[i]))
		}

		s = string(bs[i])

		t, found = this.h2m[s]
		if found {
			panic(fmt.Sprintf("multiple types for %v: %v %v",
				bs[i], t, ts[i]))
				
		}

		b, found = this.m2h[ts[i]]
		if found {
			panic(fmt.Sprintf("multiple headers for %v: %v %v",
				ts[i], b, bs[i]))
		}

		this.h2m[s] = ts[i]
		this.m2h[ts[i]] = bs[i]
	}

	return &this
}

func (this *bytesTypesProtocol) Encode(sink io.Sink, msg Message) error {
	var header []byte
	var ok bool

	header, ok = this.m2h[reflect.TypeOf(msg).Elem()]
	if !ok {
		panic(fmt.Sprintf("unknown message type %T", msg))
	}

	return sink.WriteBytes(header).WriteEncodable(msg).Error()
}

func (this *bytesTypesProtocol) Decode(source io.Source) (Message, error) {
	var header []byte = make([]byte, this.hlen)
	var mtype reflect.Type
	var msg Message = nil
	var err error
	var ok bool

	err = source.ReadBytes(header).AndThen(func () error {
		mtype, ok = this.h2m[string(header)]

		if !ok {
			return fmt.Errorf("unknown message type %v", header)
		}

		msg = reflect.New(mtype).Interface().(Message)

		return nil
	}).ReadDecodable(msg).Error()

	return msg, err
}

func newUint8Protocol(hmap map[uint8]Message) Protocol {
	var mtypes []reflect.Type = make([]reflect.Type, 0, len(hmap))
	var bheaders [][]byte = make([][]byte, 0, len(hmap))
	var header uint8
	var msg Message

	for header, msg = range hmap {
		bheaders = append(bheaders, []byte{ header })
		mtypes = append(mtypes, reflect.ValueOf(msg).Type().Elem())
	}

	return newBytesTypesProtocol(bheaders, mtypes)
}
