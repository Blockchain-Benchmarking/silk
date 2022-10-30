package net


import (
	"context"
	"net"
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
}

func NewTcpServerWith(addr string, opts *TcpServerOptions) Accepter {
	if opts == nil {
		opts = &TcpServerOptions{}
	}

	return newTcpServer(addr, opts)
}


// ----------------------------------------------------------------------------


type tcpServer struct {
	listener net.Listener
	acceptc chan Connection
}

func newTcpServer(addr string, opts *TcpServerOptions) *tcpServer {
	var this tcpServer
	var err error

	this.acceptc = make(chan Connection, 16)

	if checkTcpServerName(addr) == false {
		close(this.acceptc)
		return &this
	}

	this.listener, err = net.Listen("tcp", addr)

	if err != nil {
		close(this.acceptc)
		return &this
	}

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

	for {
		conn, _ = this.listener.Accept()
		if conn == nil {
			break
		}

		this.acceptc <- newTcpConnection(conn)
	}

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
