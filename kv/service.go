package kv


import (
	"math"
	sio "silk/io"
	"silk/net"
	"sync"
)


// ----------------------------------------------------------------------------


type Service interface {
	Handle(*Request, net.Connection)

	Table
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


// ----------------------------------------------------------------------------


type service struct {
	log sio.Logger
	lock sync.Mutex
	table Table
}

func newService(opts *ServiceOptions) *service {
	var this service

	this.log = opts.Log
	this.table = NewTable()

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

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *service) set(key Key, value Value) Value {
	if value == NoValue {
		this.log.Trace("delete %v", this.log.Emph(0, key))
		return this.table.Set(key, value)
	} else {
		this.log.Trace("set %v = %v", this.log.Emph(0, key),
			this.log.Emph(0, value))
		return this.table.Set(key, value)
	}
}

func (this *service) Set(key Key, value Value) Value {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.set(key, value)
}

func (this *service) handleSet(set map[Key]Value, reply *Reply) {
	var value Value
	var key Key

	for key, value = range set {
		this.set(key, value)
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *service) get(key Key) Value {
	var ret Value = this.table.Get(key)

	if ret == NoValue {
		this.log.Trace("get %v has no value", this.log.Emph(0, key))
	} else {
		this.log.Trace("get %v = %v", this.log.Emph(0, key),
			this.log.Emph(0, ret))
	}

	return ret
}

func (this *service) Get(key Key) Value {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.get(key)
}

func (this *service) handleGet(keys []Key, reply *Reply) {
	var i int

	reply.Get = make([]Value, len(keys))

	for i = range keys {
		reply.Get[i] = this.get(keys[i])
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *service) list() []Key {
	var ret []Key = this.table.List()
	var key Key

	for _, key = range ret {
		this.log.Trace("list %v", this.log.Emph(0, key))
	}

	return ret
}

func (this *service) List() []Key {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.list()
}

func (this *service) handleList(reply *Reply) {
	reply.List = this.list()
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func (this *service) Snapshot() View {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.table.Snapshot()
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
