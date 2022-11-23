package kv


import (
	"fmt"
	"math"
	sio "silk/io"
	"silk/net"
	"sync"
)


// ----------------------------------------------------------------------------


type Service interface {
	Formatter

	Handle(*Request, net.Connection)
}


type ServiceOptions struct {
	Log sio.Logger
}

func NewService() Service {
	return NewServiceWith(nil)
}

func NewServiceWith(opts *ServiceOptions) Service {
	if opts == nil {
		opts = &ServiceOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	return newService(opts)
}


type Formatter interface {
	Format(string) (string, error)
}


var Verbatim Formatter = &verbatim{}


type Request struct {
	// A list of keys to get the associated values.
	// The `Value`s are to get after the modifications of this `Request`
	// has been applied.
	Get []Key

	// List all the `Key`s on the `Service`.
	// The `Key`s are to get after the modifications of this `Request` has
	// been applied.
	List bool

	// A list of key value pairs to set or delete (if `NoValue`).
	Set map[Key]Value
}


var Protocol net.Protocol = net.NewRawProtocol(&Reply{})


type Reply struct {
	// A list of `Value`s corresponding to the `Key`s in the corresponding
	// `Request` in the same order.
	Get []Value

	// A list of `Key`s in the `Service` if the corresponding `Request` has
	// its `List` field set to `true`.
	List []Key
}


// The maximum number of `Key` `Value` pairs in a `Request` or that can be
// stored in a single `Service`.
//
const MaxPair = math.MaxUint16


type TooManyPairsError struct {
}

type InvalidFormatError struct {
	Fmt string
	Pos int
}

type UnknownKeyError struct {
	Key Key
}


// ----------------------------------------------------------------------------


type service struct {
	log sio.Logger
	lock sync.Mutex
	kvs map[string]Value
}

func newService(opts *ServiceOptions) *service {
	var this service

	this.log = opts.Log
	this.kvs = make(map[string]Value)

	return &this
}

func (this *service) Handle(request *Request, conn net.Connection) {
	var reply Reply

	this.log.Trace("receive request (get: %d, list: %t, set: %d)",
		len(request.Get), request.List, len(request.Set))

	this.lock.Lock()
	defer this.lock.Unlock()

	this.handleSet(request.Set, &reply)

	this.handleGet(request.Get, &reply)

	if request.List {
		this.handleList(&reply)
	}

	this.log.Trace("send reply (get: %d, list: %d)", len(reply.Get),
		len(reply.List))

	conn.Send() <- net.MessageProtocol{
		M: &reply,
		P: Protocol,
	}

	close(conn.Send())
}

func (this *service) handleSet(set map[Key]Value, reply *Reply) {
	var value Value
	var key Key

	for key, value = range set {
		if value == NoValue {
			this.log.Trace("delete %s",
				this.log.Emph(0, key.String()))
			delete(this.kvs, key.String())
		} else {
			this.log.Trace("set %s = %s",
				this.log.Emph(0, key.String()),
				this.log.Emph(0, value.String()))
			this.kvs[key.String()] = value
		}
	}
}

func (this *service) handleGet(get []Key, reply *Reply) {
	var value Value
	var found bool
	var i int

	reply.Get = make([]Value, len(get))

	for i = range get {
		value, found = this.kvs[get[i].String()]

		if found {
			this.log.Trace("get %s = %s",
				this.log.Emph(0, get[i].String()),
				this.log.Emph(0, value.String()))
			reply.Get[i] = value
		} else {
			this.log.Trace("get %s has no value",
				this.log.Emph(0, get[i].String()))
			reply.Get[i] = NoValue
		}
	}
}

func (this *service) handleList(reply *Reply) {
	var keyName string
	var i int

	reply.List = make([]Key, len(this.kvs))

	i = 0
	for keyName = range this.kvs {
		this.log.Trace("list %s", this.log.Emph(0, keyName))
		reply.List[i] = &key{ keyName }
		i += 1
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *service) Format(str string) (string, error) {
        var start, pos int
        var percent bool
	var value Value
        var ret string
	var found bool
	var err error
	var key Key
        var r rune

	percent = false
	start = -1
	ret = ""

        for pos, r = range str {
                if start >= 0 {
                        if r == '}' {
				key, err = NewKey(str[start:pos])
				if err != nil {
					return "", err
				}

				this.lock.Lock()
				value, found = this.kvs[key.String()]
				this.lock.Unlock()

				if found == false {
					return "", &UnknownKeyError{ key }
				}

                                ret += value.String()
				start = -1
                        }
                        continue
                }

                if percent {
			if r == '%' {
				ret += "%"
			} else if r == '{' {
				start = pos + 1
			} else {
				return "", &InvalidFormatError{ str, pos }
                        }

                        percent = false
                        continue
                }

                if r == '%' {
                        percent = true
                } else {
                        ret += string(r)
                }
        }

        return ret, nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type verbatim struct {
}

func (this *verbatim) Format(str string) (string, error) {
	return str, nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *Request) Encode(sink sio.Sink) error {
	var value Value
	var list uint8
	var key Key

	if len(this.Get) > MaxPair {
		return &TooManyPairsError{}
	}

	if len(this.Set) > MaxPair {
		return &TooManyPairsError{}
	}

	if this.List {
		list = 1
	} else {
		list = 0
	}

	sink = sink.WriteUint16(uint16(len(this.Get)))
	for _, key = range this.Get {
		sink = sink.WriteEncodable(key)
	}

	sink = sink.WriteUint8(list)

	sink = sink.WriteUint16(uint16(len(this.Set)))
	for key, value = range this.Set {
		sink = sink.WriteEncodable(key).
			WriteEncodable(value)
	}

	return sink.Error()
}

func (this *Request) Decode(source sio.Source) error {
	var list uint8
	var n uint16
	var i int

	return source.ReadUint16(&n).AndThen(func () error {
		this.Get = make([]Key, n)

		for i = range this.Get {
			this.Get[i], source = ReadKey(source)
		}

		return source.Error()
	}).
	ReadUint8(&list).And(func () { this.List = (list != 0) }).
	ReadUint16(&n).AndThen(func () error {
		var v Value
		var k Key

		this.Set = make(map[Key]Value, n)

		for i = 0; i < int(n); i++ {
			k, source = ReadKey(source)
			v, source = ReadValue(source)
			this.Set[k] = v
		}

		return source.Error()
	}).
	Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *Reply) Encode(sink sio.Sink) error {
	var value Value
	var key Key

	if len(this.Get) > MaxPair {
		return &TooManyPairsError{}
	}

	if len(this.List) > MaxPair {
		return &TooManyPairsError{}
	}

	sink = sink.WriteUint16(uint16(len(this.Get)))
	for _, value = range this.Get {
		sink = sink.WriteEncodable(value)
	}

	sink = sink.WriteUint16(uint16(len(this.List)))
	for _, key = range this.List {
		sink = sink.WriteEncodable(key)
	}

	return sink.Error()
}

func (this *Reply) Decode(source sio.Source) error {
	var n uint16
	var i int

	return source.ReadUint16(&n).AndThen(func () error {
		this.Get = make([]Value, n)

		for i = range this.Get {
			this.Get[i], source = ReadValue(source)
		}

		return source.Error()
	}).
	ReadUint16(&n).AndThen(func () error {
		this.List = make([]Key, n)

		for i = range this.List {
			this.List[i], source = ReadKey(source)
		}

		return source.Error()
	}).
	Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *TooManyPairsError) Error() string {
	return "too many key/value pairs"
}

func (this *InvalidFormatError) Error() string {
	return fmt.Sprintf("invalid format: %s", this.Fmt)
}

func (this *UnknownKeyError) Error() string{
	return fmt.Sprintf("unknown key: %s", this.Key)
}
