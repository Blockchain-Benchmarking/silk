package net


import (
	"context"
	"fmt"
	"net"
	sio "silk/io"
	"strconv"
	"strings"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


const BASE_TIMEOUT = 1000 * time.Millisecond

func timeout(n int) <-chan struct{} {
	var ret chan struct{} = make(chan struct{})

	go func () {
		<-time.After(time.Duration(n) * BASE_TIMEOUT)
		close(ret)
	}()

	return ret
}


var mockProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &RoutingMessage{},
	1: &AggregationMessage{},
	2: &mockMessage{},
})


type mockMessage struct {
	value uint64
	payload []byte
	shallowPayload uint32
	encodingError bool
	decodingError bool
}

func (this *mockMessage) Encode(sink sio.Sink) error {
	var flags uint8

	if this.encodingError {
		return fmt.Errorf("mock encoding error")
	}

	if this.decodingError {
		flags |= 0x1
	}

	return sink.WriteUint64(this.value).
		WriteBytes32(this.payload).
		WriteBytes32(make([]byte, this.shallowPayload)).
		WriteUint8(flags).
		Error()
}

func (this *mockMessage) Decode(src sio.Source) error {
	var flags uint8
	var b []byte

	return src.ReadUint64(&this.value).
		ReadBytes32(&this.payload).
		ReadBytes32(&b).
		And(func () { this.shallowPayload = uint32(len(b)) }).
		ReadUint8(&flags).
		And(func () { this.decodingError = ((flags & 0x1) != 0) }).
		AndThen(func () error {
			if this.decodingError {
				return fmt.Errorf("mock decoding error")
			} else {
				return nil
			}
		}).
		Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func listenTcpPort(t *testing.T) (uint16, net.Listener) {
        var fields []string
        var l net.Listener
        var port64 uint64
        var port string
        var err error

        l, err = net.Listen("tcp", ":0")
        if err != nil {
                t.Fatalf("listen: %v", err)
                return 0, nil
        }
        
        fields = strings.Split(l.Addr().String(), ":")

        port = fields[len(fields) - 1]
        port64, err = strconv.ParseUint(port, 10, 16)
        if err != nil {
		l.Close()
                t.Fatalf("listen: %v", err)
                return 0, nil
        }

        return uint16(port64), l
}

func findUsedTcpPort(t *testing.T) (uint16, context.CancelFunc) {
	var l net.Listener
	var port uint16

	port, l = listenTcpPort(t)

	return port, func () { l.Close() }
}

func findUsedTcpAddr(t *testing.T) (string, context.CancelFunc) {
	var cancel context.CancelFunc
	var port uint16

	port, cancel = findUsedTcpPort(t)

	return fmt.Sprintf("localhost:%d", port), cancel
}

func findTcpPort(t *testing.T) uint16 {
	var l net.Listener
	var port uint16

	port, l = listenTcpPort(t)
	l.Close()

	return port
}

func findTcpAddr(t *testing.T) string {
        return fmt.Sprintf("localhost:%d", findTcpPort(t))
}

func findUnresolvableAddr(t *testing.T) string {
	// Completely empiric!
	// Probably does not work on many setups
	return "1.1.1.1:1"
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func acceptConnections(a Accepter, connc chan<- Connection) {
	var c Connection

	for c = range a.Accept() {
		connc <- c
	}

	close(connc)
}

func _gatherConnections(cc <-chan Connection,t <-chan struct{}) []Connection {
	var cs []Connection = make([]Connection, 0)
	var c Connection
	var more bool

	for {
		select {
		case c, more = <-cc:
			if more == false {
				return cs
			} else {
				cs = append(cs, c)
			}
		case <-t:
			go func () { for _ = range cc {} }()
			return nil
		}
	}
}

func gatherConnections(c<-chan Connection,t<-chan struct{}) <-chan[]Connection{
	var csc chan []Connection = make(chan []Connection)
	var cs []Connection

	go func () {
		cs = _gatherConnections(c, t)
		if cs != nil {
			csc <- cs
		}
		close(csc)
	}()

	return csc
}

func transmitConnections(src <-chan Connection, n int) <-chan Connection {
	var dest chan Connection = make(chan Connection, n)

	go func () {
		var c Connection
		var more bool

		for {
			if n <= 0 {
				break
			}

			c, more = <-src
			if more == false {
				break
			}

			dest <- c

			n -= 1
		}

		close(dest)
	}()

	return dest
}

func sendUniqueRecvMessages(cs []Connection) []*mockMessage {
	var ms []*mockMessage
	var msg Message
	var more bool
	var i int

	for i = range cs {
		go func (index int) {
			cs[index].Send() <- MessageProtocol{
				M: &mockMessage{ value: uint64(index) },
				P: mockProtocol,
			}
		}(i)
	}

	ms = make([]*mockMessage, len(cs))

	for i = range cs {
		select {
		case msg, more = <-cs[i].RecvN(mockProtocol, 1):
			if more {
				ms[i] = msg.(*mockMessage)
			}
		case <-time.After(10 * BASE_TIMEOUT):
		}
	}

	return ms
}

func testConnectionsConnectivity(as []Connection,bs []Connection,t *testing.T){
	const MAX_ERROR_BEFORE_STOP = 10
	var recvd map[uint64]int
	var ms []*mockMessage
	var name string
	var i, e int

	e = 0

	if len(bs) < len(as) {
		t.Errorf("lost %d connections", len(as) - len(bs))
		return
	} else if len(bs) > len(as) {
		t.Errorf("got %d unexpected connections", len(bs) - len(as))
		return
	}

	ms = sendUniqueRecvMessages(append(append([]Connection{},as...),bs...))
	recvd = make(map[uint64]int)

	for i = range ms {
		if e >= MAX_ERROR_BEFORE_STOP {
			t.Logf("stop reporting after %d errors", e)
			return
		}

		if i < len(as) {
			name = fmt.Sprintf("a[%d]", i)
		} else {
			name = fmt.Sprintf("b[%d]", i - len(as))
		}

		if ms[i] == nil {
			t.Errorf("connection %s has not received", name)
			e += 1
		} else {
			recvd[ms[i].value] += 1
		}
	}

	for i = range ms {
		if e >= MAX_ERROR_BEFORE_STOP {
			t.Logf("stop reporting after %d errors", e)
			return
		}

		if i < len(as) {
			name = fmt.Sprintf("a[%d]", i)
		} else {
			name = fmt.Sprintf("b[%d]", i - len(as))
		}

		if recvd[uint64(i)] < 1 {
			t.Errorf("lost message from %s", name)
			e += 1
		} else if recvd[uint64(i)] > 1 {
			t.Errorf("got duplicated message from %s", name)
			e += 1
		}
	}
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type trivialReceiver struct {
	c <-chan Message
}

func newTrivialReceiver(c <-chan Message) *trivialReceiver {
	return &trivialReceiver{ c }
}

func (this *trivialReceiver) decode(d chan<- Message, n int) {
	var m Message
	var more bool

	for n != 0 {
		m, more = <-this.c
		if more == false {
			break
		}

		d <- m

		if n > 0 {
			n -= 1
		}
	}

	close(d)
}

func (this *trivialReceiver) Recv(proto Protocol) <-chan Message {
	var d chan Message = make(chan Message)
	go this.decode(d, -1)
	return d
}

func (this *trivialReceiver) RecvN(proto Protocol, n int) <-chan Message {
	var d chan Message = make(chan Message)
	go this.decode(d, n)
	return d
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func generateLinearShallowMessages(n int, max int) []*mockMessage {
	var ret []*mockMessage = make([]*mockMessage, 0, n)
	var i, l int

	for i = 0; i < n; i++ {
		l = int(float64(i) * (float64(max) / float64(n - 1)))
		ret = append(ret, &mockMessage{
			value: uint64(i),
			shallowPayload: uint32(l),
		})
	}

	return ret
}

func recvMock(msgc <-chan Message) <-chan *mockMessage {
	var mc chan *mockMessage = make(chan *mockMessage)

	go func () {
		var m *mockMessage
		var msg Message
		var ok bool

		for msg = range msgc {
			m, ok = msg.(*mockMessage)
			if ok == false {
				break
			}

			mc <- m
		}

		close(mc)

		for _ = range msgc {}
	}()

	return mc
}

func recvMessage(mc <-chan *mockMessage) <-chan Message {
	var msgc chan Message = make(chan Message)

	go func () {
		var m *mockMessage

		for m = range mc {
			msgc <- m
		}

		close(msgc)
	}()

	return msgc
}

func _gatherMessages(msgc <-chan Message, t <-chan struct{}) []Message {
	var msgs []Message = make([]Message, 0)
	var msg Message
	var more bool

	for {
		select {
		case msg, more = <-msgc:
			if more == false {
				return msgs
			} else {
				msgs = append(msgs, msg)
			}
		case <-t:
			go func () { for _ = range msgc {} }()
			return nil
		}
	}
}

func gatherMessages(msgc <-chan Message, t <-chan struct{}) <-chan []Message {
	var msgsc chan []Message = make(chan []Message)
	var msgs []Message

	go func () {
		msgs = _gatherMessages(msgc, t)
		if msgs != nil {
			msgsc <- msgs
		}
		close(msgsc)
	}()

	return msgsc
}

func filterMockMessages(msgs []Message) []*mockMessage {
	var ms []*mockMessage = make([]*mockMessage, len(msgs))
	var i int

	for i = range msgs {
		ms[i], _ = msgs[i].(*mockMessage)
	}

	return ms
}

func cmpMessages(a *mockMessage, b *mockMessage) (bool, string) {
	if a.value != b.value {
		return false, fmt.Sprintf("values differ: %d != %d",
			a.value, b.value)
	}

	if a.shallowPayload != b.shallowPayload {
		return false, fmt.Sprintf("shallow payloads differ: %d != %d",
			a.shallowPayload, b.shallowPayload)
	}

	return true, ""
}

func cmpAllMessages(ms []*mockMessage) (bool, string) {
	var ref *mockMessage
	var emsg string
	var ok bool
	var i int

	if len(ms) == 0 {
		return true, ""
	}

	ref = ms[0]

	for i = 1; i < len(ms); i++ {
		ok, emsg = cmpMessages(ref, ms[i])
		if ok == false {
			return false, fmt.Sprintf("messages [%d] & [%d]: %s",
				0, i, emsg)
		}
	}

	return true, ""
}

func testMessagesEquality(in []*mockMessage, out []Message, t *testing.T) {
	const MAX_ERROR_BEFORE_STOP = 10
	var im, om *mockMessage
	var i, n, e int
	var emsg string
	var ok bool

	e = 0

	if len(out) < len(in) {
		t.Errorf("lost %d messages", len(in) - len(out))
		n = len(out)
		e += 1
	} else if len(out) > len(in) {
		t.Errorf("got %d unexpected messages", len(out) - len(in))
		n = len(in)
		e += 1
	} else {
		n = len(in)
	}

	for i = 0; i < n; i++ {
		if e >= MAX_ERROR_BEFORE_STOP {
			t.Logf("stop reporting after %d errors", e)
			break
		}

		im = in[i]
		om, ok = out[i].(*mockMessage)
		if ok == false {
			t.Errorf("message[%d]: unexpected type: %T:%v", i,
				out[i], out[i])
			e += 1
			continue
		}

		ok, emsg = cmpMessages(im, om)
		if ok == false {
			t.Errorf("message[%d]: %s", i, emsg)
			e += 1
		}
	}
}

func testMessageSetEquality(in []*mockMessage, out []Message, t *testing.T) {
	if len(out) > len(in) {
		t.Errorf("got %d unexpected messages", len(out) - len(in))
	} else {
		testMessageSetInclusive(in, out, t)
	}
}

func testMessageSetInclusive(in []*mockMessage, out []Message, t *testing.T) {
	const MAX_ERROR_BEFORE_STOP = 10
	var im, om *mockMessage
	var i, j, n, e int
	var discard []bool
	var ok, found bool

	e = 0

	if len(out) < len(in) {
		t.Errorf("lost %d messages", len(in) - len(out))
		n = len(out)
		e += 1
	} else {
		n = len(in)
	}

	discard = make([]bool, len(out))

	for i = 0; i < n; i++ {
		if e >= MAX_ERROR_BEFORE_STOP {
			t.Logf("stop reporting after %d errors", e)
			break
		}

		im = in[i]
		found = false

		for j = 0; j < len(out); j++ {
			if discard[j] {
				continue
			}

			om, ok = out[j].(*mockMessage)
			if ok == false {
				t.Errorf("message[%d]: unexpected type: %T:%v",
					j, out[j], out[j])
				discard[j] = true
				e += 1
				continue
			}

			ok, _ = cmpMessages(im, om)
			if ok == true {
				found = true
				discard[j] = true
				break
			}
		}

		if found == false {
			t.Errorf("message[%d]: no matching output", i)
			e += 1
		}
	}
}
