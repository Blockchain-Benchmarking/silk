package net


import (
	"fmt"
	sio "silk/io"
	"strconv"
	"strings"
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


func NewAggregatedTcpResolver(proto Protocol) Resolver {
	return NewAggregatedTcpResolverWith(proto, nil)
}

func NewAggregatedTcpResolverWith(p Protocol, o *TcpResolverOptions) Resolver{
	if o == nil {
		o = &TcpResolverOptions{}
	}

	if o.Log == nil {
		o.Log = sio.NewNopLogger()
	}

	return newAggregatedTcpResolver(p, o)
}


func NewGroupResolver(inner Resolver) Resolver {
	return newGroupResolver(inner)
}


// The `Resolver` does not known how to resolve the given `Name`.
//
type ResolverInvalidNameError struct {
	Name string
}


// ----------------------------------------------------------------------------


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

func (this *tcpResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var ret Route

	this.log.Trace("resolve '%s' as new TCP connection",
		this.log.Emph(0, name))

	if !checkTcpName(name) {
		return nil, nil, &ResolverInvalidNameError{ name }
	}

	ret = NewUnitLeafRoute(NewTcpConnection(name))

	return []Route{ ret }, []Protocol{ this.proto }, nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type aggregatedTcpResolver struct {
	log sio.Logger
	proto Protocol
}

func newAggregatedTcpResolver(p Protocol, o *TcpResolverOptions) *aggregatedTcpResolver {
	var this aggregatedTcpResolver

	this.log = o.Log
	this.proto = p

	return &this
}

func (this *aggregatedTcpResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var index int
	var ret Route
	var err error
	var n uint64

	index = strings.Index(name, "*")
	if index == -1 {
		this.log.Trace("resolve '%s' as new TCP connection",
			this.log.Emph(0, name))

		if !checkTcpName(name) {
			return nil, nil, &ResolverInvalidNameError{ name }
		}

		ret = NewUnitLeafRoute(NewTcpConnection(name))
	} else {
		n, err = strconv.ParseUint(name[:index], 10, 16)
		if err != nil {
			return nil, nil, &ResolverInvalidNameError{ name }
		}

		this.log.Trace("resolve '%s' as %d aggregated new TCP " +
			"connections", this.log.Emph(0, name), n)

		name = name[index+1:]

		if !checkTcpName(name) {
			return nil, nil, &ResolverInvalidNameError{ name }
		}

		ret = NewUnitLeafRoute(NewAggregatedTcpConnection(name,
			this.proto, int(n)))
	}

	return []Route{ ret }, []Protocol{ this.proto }, nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type groupResolver struct {
	inner Resolver
}

func newGroupResolver(inner Resolver) *groupResolver {
	var this groupResolver

	this.inner = inner

	return &this
}

func (this *groupResolver) Resolve(name string) ([]Route, []Protocol, error) {
	var retp, ps []Protocol
	var retr, rs []Route
	var part string
	var err error

	for _, part = range strings.Split(name, "+") {
		rs, ps, err = this.inner.Resolve(part)
		if err != nil {
			continue
		}

		retr = append(retr, rs...)
		retp = append(retp, ps...)
	}

	return retr, retp, nil
}


func (this *ResolverInvalidNameError) Error() string {
	return fmt.Sprintf("invalid name '%s'", this.Name)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func checkTcpName(name string) bool {
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
