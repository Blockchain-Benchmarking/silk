package net


import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
)


// ----------------------------------------------------------------------------


func findTcpPort(t *testing.T) uint16 {
	var fields []string
	var l net.Listener
	var port64 uint64
	var port string
	var err error

	l, err = net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
		return 0
	}
	
	fields = strings.Split(l.Addr().String(), ":")
	l.Close()

	port = fields[len(fields) - 1]
	port64, err = strconv.ParseUint(port, 10, 16)
	if err != nil {
		t.Fatalf("listen: %v", err)
		return 0
	}

	return uint16(port64)
}

func findTcpAddress(t *testing.T) string {
	return fmt.Sprintf("localhost:%d", findTcpPort(t))
}
