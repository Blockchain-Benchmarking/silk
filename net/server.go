package net


import (
	"context"
	"net"
	sio "silk/io"
	"strconv"
	"strings"
	"unicode"
)


// ----------------------------------------------------------------------------


func NewTcpServer(addr string) Accepter {
	return NewTcpServerWith(addr, nil)
}


type TcpServerOptions struct {
	Context context.Context

	Log sio.Logger
}

func NewTcpServerWith(addr string, opts *TcpServerOptions) Accepter {
	if opts == nil {
		opts = &TcpServerOptions{}
	}

	if opts.Log == nil {
		opts.Log = sio.NewNopLogger()
	}

	return newTcpServer(addr, opts)
}


// ----------------------------------------------------------------------------


type tcpServer struct {
	log sio.Logger
	listener net.Listener
	acceptc chan Connection
}

func newTcpServer(addr string, opts *TcpServerOptions) *tcpServer {
	var this tcpServer
	var err error

	this.log = opts.Log

	this.acceptc = make(chan Connection, 16)

	if checkTcpServerName(addr) == false {
		this.log.Warn("invalid server address: %s",
			this.log.Emph(0, addr))
		close(this.acceptc)
		return &this
	}

	this.listener, err = net.Listen("tcp", addr)

	if err != nil {
		this.log.Warn("%s", err.Error())
		close(this.acceptc)
		return &this
	}

	this.log.Debug("listen")

	if opts.Context != nil {
		go func () {
			<-opts.Context.Done()
			this.listener.Close()
		}()
	}

	go this.run()

	return &this
}

func (this *tcpServer) run() {
	var conn net.Conn
	var err error

	for {
		conn, err = this.listener.Accept()
		if err != nil {
			break
		}

		this.log.Trace("accept")

		this.acceptc <- newTcpConnection(conn)
	}

	this.log.Debug("close: %s", err.Error())

	close(this.acceptc)
}

func (this *tcpServer) Accept() <-chan Connection {
	return this.acceptc
}


func checkTcpServerName(name string) bool {
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

	if len(hostname) == 0 {
		return true          // listen any
	}

	if len(hostname) > 253 {
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
