package net


import (
	"context"
	"fmt"
	sio "silk/io"
	"strconv"
	"strings"
	"sync"
	"unicode"
)


// ----------------------------------------------------------------------------


type Resolver interface {
	// Resolve the given `name` and give the set of `Route` this `name` is
	// associated with.
	// Each returned `Route` has an associated `Protocol` which must be
	// used to initiate message exchange.
	// If the given `name` cannot be resolved, return an `error`.
	//
	Resolve(name string) ([]Route, []Protocol, error)
}


func NewFirstFitResolver(inners []Resolver) Resolver {
	return newFirstFitResolver(inners)
}


type TcpResolverOptions struct {
	Log sio.Logger
}

func NewTcpResolver(proto Protocol) Resolver {
	return NewTcpResolverWith(proto, nil)
}

func NewTcpResolverWith(proto Protocol, opts *TcpResolverOptions) Resolver {
	if opts == nil {
		opts = &TcpResolverOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	return newTcpResolver(proto, opts)
}


func NewBindingResolver(service BindingService, proto Protocol) Resolver {
	return NewBindingResolverWithLogger(service, proto, sio.NewNopLogger())
}

func NewBindingResolverWithLogger(service BindingService, proto Protocol, log sio.Logger) Resolver {
	return newBindingResolver(service, proto, log)
}


type MapResolver interface {
	Resolver

	Insert(string, []Route, []Protocol) ([]Route, []Protocol)

	Delete(string) ([]Route, []Protocol)
}

func NewMapResolver() Resolver {
	return NewMapResolverWithLogger(sio.NewNopLogger())
}

func NewMapResolverWithLogger(log sio.Logger) Resolver {
	return newMapResolver(log)
}


// The `Resolver` attempted to create a new `Route` for the `Name` but could
// not reach the target because of `Err`.
//
type ResolverCannotReachError struct {
	Name string
	Err error
}

// The `Resolver` does not known how to resolve the given `Name`.
//
type ResolverInvalidNameError struct {
	Name string
}


// ----------------------------------------------------------------------------


type firstFitResolver struct {
	inners []Resolver
}

func newFirstFitResolver(inners []Resolver) *firstFitResolver {
	var this firstFitResolver
	var i int

	this.inners = make([]Resolver, len(inners))
	for i = range inners {
		this.inners[i] = inners[i]
	}

	return &this
}

func (this *firstFitResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var protos []Protocol
	var routes []Route
	var inner Resolver
	var err error

	for _, inner = range this.inners {
		routes, protos, err = inner.Resolve(name)
		if err == nil {
			return routes, protos, nil
		}
	}

	return nil, nil, &ResolverInvalidNameError{ name }
}



type tcpResolver struct {
	log sio.Logger
	proto Protocol
}

func newTcpResolver(proto Protocol, opts *TcpResolverOptions) *tcpResolver {
	var this tcpResolver

	this.log = opts.Log
	this.proto = proto

	return &this
}

func (this *tcpResolver) checkName(name string) bool {
	var parts []string = strings.Split(name, ":")
	var hostname, strport, label string
	var err error
	var r rune

	if len(parts) != 2 {
		return false
	}

	hostname, strport = parts[0], parts[1]

	_, err = strconv.ParseUint(strport, 10, 16)
	if err != nil {
		return false
	}

	// https://en.wikipedia.org/wiki/Hostname

	if (len(hostname) < 1) || (len(hostname) > 253) {
		return false
	}

	for _, label = range strings.Split(hostname, ".") {
		if (len(label) < 1) || (len(label) > 63) {
			return false
		}

		for _, r = range label {
			if r > unicode.MaxASCII {
				return false
			}

			if unicode.IsLetter(r) {
				continue
			}

			if unicode.IsDigit(r) {
				continue
			}

			if (r == '-') || (r == '_') {
				continue
			}

			return false
		}
	}

	return true
}

func (this *tcpResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var cancel context.CancelFunc
	var connc chan Connection
	var ctx context.Context
	var errc chan error
	var ret Route

	this.log.Trace("resolve '%s' as new TCP connection",
		this.log.Emph(0, name))

	if !this.checkName(name) {
		return nil, nil, &ResolverInvalidNameError{ name }
	}

	ctx, cancel = context.WithCancel(context.Background())
	connc = make(chan Connection)
	errc = make(chan error)
	go func () {
		var conn Connection
		var err error

		conn, err = NewTcpConnectionWith(name, &TcpConnectionOptions{
			Context: ctx,
		})

		if err == nil {
			connc <- conn
		} else {
			errc <- err
		}

		close(connc)
		close(errc)
	}()

	ret = newTcpResolveRoute(NewChannelRoute(connc, errc), cancel)

	return []Route{ ret }, []Protocol{ this.proto }, nil
}


type tcpResolverRoute struct {
	inner Route
	cancel context.CancelFunc
}

func newTcpResolveRoute(r Route, c context.CancelFunc) *tcpResolverRoute {
	var this tcpResolverRoute

	this.inner = r
	this.cancel = c

	return &this
}

func (this *tcpResolverRoute) Send(msg Message, proto Protocol) error {
	return this.inner.Send(msg, proto)
}

func (this *tcpResolverRoute) Accept() (Connection, error) {
	return this.inner.Accept()
}

func (this *tcpResolverRoute) Close() error {
	var err error

	err = this.inner.Close()
	this.cancel()

	return err
}



// type tcpResolver struct {
// 	log sio.Logger
// 	proto Protocol
// }

// func newTcpResolver(proto Protocol, log sio.Logger) *tcpResolver {
// 	var this tcpResolver

// 	this.log = log
// 	this.proto = proto

// 	return &this
// }

// func (this *tcpResolver) Resolve(name string) ([]Route, []Protocol, error) {
// 	var conn Connection
// 	var err error

// 	this.log.Trace("resolve '%s' as new TCP connection",
// 		this.log.Emph(0, name))

// 	conn, err = NewTcpConnection(name)
// 	if err != nil {
// 		return nil, nil, &ResolverCannotReachError{ name, err }
// 	}

// 	return []Route{NewConnectionRoute(conn)}, []Protocol{this.proto}, nil
// }


type bindingResolver struct {
	log sio.Logger
	binding BindingService
	proto Protocol
}

func newBindingResolver(service BindingService, proto Protocol, log sio.Logger) *bindingResolver {
	var this bindingResolver

	this.log = log
	this.binding = service
	this.proto = proto

	return &this
}

func (this *bindingResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var conn Connection
	var route Route
	var err error

	conn, err = this.binding.Connect(name)
	if err != nil {
		return nil, nil, &ResolverCannotReachError{ name, err }
	}

	route = NewConnectionRoute(conn)

	return []Route{ route }, []Protocol{ this.proto }, nil
}


type mapResolver struct {
	log sio.Logger
	lock sync.Mutex
	content map[string]*mapResolverEntry
}

type mapResolverEntry struct {
	routes []Route
	protos []Protocol
}

func newMapResolver(log sio.Logger) *mapResolver {
	var this mapResolver

	this.log = log
	this.content = make(map[string]*mapResolverEntry)

	return &this
}

func (this *mapResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var entry *mapResolverEntry

	this.log.Trace("resolve '%s' as explicit route",
		this.log.Emph(0, name))

	this.lock.Lock()
	entry = this.content[name]
	this.lock.Unlock()

	if entry != nil {
		return entry.routes, entry.protos, nil
	} else {
		return nil, nil, &ResolverInvalidNameError{ name }
	}
}

func (this *mapResolver) Insert(name string, routes []Route, protos []Protocol) ([]Route, []Protocol) {
	var nentry, oentry *mapResolverEntry

	if len(routes) != len(protos) {
		panic("invalid insert")
	}

	nentry = &mapResolverEntry{ routes, protos }

	this.log.Trace("insert '%s' as explicit route",
		this.log.Emph(0, name))

	this.lock.Lock()
	oentry = this.content[name]
	this.content[name] = nentry
	this.lock.Unlock()

	if oentry != nil {
		return oentry.routes, oentry.protos
	} else {
		return nil, nil
	}
}

func (this *mapResolver) Delete(name string) ([]Route, []Protocol) {
	var oentry *mapResolverEntry

	this.log.Trace("delete '%s' as explicit route",
		this.log.Emph(0, name))

	this.lock.Lock()
	oentry = this.content[name]
	delete(this.content, name)
	this.lock.Unlock()

	if oentry != nil {
		return oentry.routes, oentry.protos
	} else {
		return nil, nil
	}
}


func (this *ResolverCannotReachError) Error() string {
	return fmt.Sprintf("cannot reach name '%s': %s", this.Name,
		this.Err.Error())
}

func (this *ResolverInvalidNameError) Error() string {
	return fmt.Sprintf("invalid name '%s'", this.Name)
}
