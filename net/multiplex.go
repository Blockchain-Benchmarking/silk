package net


import (
	"bytes"
	"fmt"
	"io"
	sio "silk/io"
	"sync"
	"time"
)


// ----------------------------------------------------------------------------


type Multiplexer interface {
	// Accept incoming multiplexed connections with the `Accept()` method.
	// Return an error if this `Multiplexer` has been closed.
	//
	// The `Close()` method makes this `Multiplexer` unable to `Accept()`
	// or `Connect()` to the remote peer but does not close already
	// establised `Connection`s.
	//
	Server

	// Create a new multiplexed `Connection`.
	// Return the new `Connection` and `nil` in case of success or an
	// `error` if the connection fails.
	// Such failure typically happens because the `Multiplexer` has been
	// `Close`d locally or if the underyling channel encountered an
	// `error` itself.
	//
	Connect() (Connection, error)
}

// Create a new `Multiplexer` where `Message`s are sent in the order in which
// they arrive from the different `Connection`s.
//
func NewFifoMultiplexer(base Channel) Multiplexer {
	return newFifoMultiplexer(base, sio.NewNopLogger())
}

func NewFifoMultiplexerWithLogger(base Channel, log sio.Logger) Multiplexer {
	return newFifoMultiplexer(base, log)
}


// ----------------------------------------------------------------------------


type fifoMultiplexer struct {
	log sio.Logger
	lock sync.Mutex
	refcnt uint
	closed bool
	nextLocal uint32
	freeLocals []uint32
	conns map[uint32]*fifoMultiplexerConnection
	acceptc chan *fifoMultiplexerConnection
	slock sync.Mutex
	base Channel
}

func newFifoMultiplexer(base Channel, log sio.Logger) *fifoMultiplexer {
	var this fifoMultiplexer

	this.log = log
	this.refcnt = 1
	this.closed = false
	this.nextLocal = 0
	this.freeLocals = make([]uint32, 0)
	this.conns = make(map[uint32]*fifoMultiplexerConnection)
	this.acceptc = make(chan *fifoMultiplexerConnection, 32)
	this.base = base

	go this.run()

	return &this
}

func (this *fifoMultiplexer) run() {
	var msg Message
	var err error

	this.log.Debug("start running")

	for this.isReferenced() {
		msg, err = this.base.Recv(prefixMultiplexerProtocol)

		if this.isReferenced() == false {
			break
		} else if err != nil {
			break
		}

		switch m := msg.(type) {
		case *prefixMultiplexerMessageConnect:
			this.handleConnect(m)
		case *prefixMultiplexerMessageDisconnect:
			this.handleDisconnect(m)
		case *prefixMultiplexerMessageContent:
			this.handleContent(m)
		case *prefixMultiplexerMessageClosed:
			this.handleClosed(m)
		default:
			panic(fmt.Sprintf("unexpected message type %T:%v",
				msg, msg))
		}
	}

	this.log.Debug("stop running")

	this.Close()
}

func (this *fifoMultiplexer) _handleConnect(connid uint32) error {
	var conn *fifoMultiplexerConnection

	if this.closed {
		return nil   // remote will be informed by `closed` message
	}

	conn = this._newRemoteConnection(connid)

	select {
	case this.acceptc <- conn:
		return nil
	default:
		return fmt.Errorf("backlog full")
	}

}

func (this *fifoMultiplexer) handleConnect(msg *prefixMultiplexerMessageConnect) {
	var rep prefixMultiplexerMessageDisconnect
	var err error

	this.log.Trace("receive connect:%d (= %d)",
		this.log.Emph(1, msg.connid),
		msg.connid & prefix_multiplexer_index_mask)

	this.lock.Lock()
	err = this._handleConnect(msg.connid)
	this.lock.Unlock()
	
	if err == nil {
		return
	}

	rep.connid = flipPrefixMultiplexerType(msg.connid)
	go this.send(&rep)
}

func (this *fifoMultiplexer) handleDisconnect(msg *prefixMultiplexerMessageDisconnect) {
	var ctype uint32 = msg.connid & prefix_multiplexer_type_mask
	var conn *fifoMultiplexerConnection

	this.log.Trace("receive disconnect:%d (= %d)",
		this.log.Emph(1, msg.connid),
		msg.connid & prefix_multiplexer_index_mask)

	this.lock.Lock()

	conn = this.conns[msg.connid]
	if conn == nil {
		this.lock.Unlock()
		this.log.Trace("disconnect unknown connection %d",
			this.log.Emph(1, msg.connid))
		return
	}

	this._recycleConnection(conn)

	this.lock.Unlock()

	if ctype == prefix_multiplexer_type_remote {
		msg.connid = flipPrefixMultiplexerType(msg.connid)
		go this.send(msg)
	}
}

func (this *fifoMultiplexer) closeConnection(conn *fifoMultiplexerConnection) error {
	var ctype uint32 = conn.id & prefix_multiplexer_type_mask
	var msg prefixMultiplexerMessageDisconnect

	this.lock.Lock()

	if this.conns[conn.id] == nil {
		this.lock.Unlock()
		return nil
	}

	if ctype == prefix_multiplexer_type_remote {
		this._recycleConnection(conn)
	}

	this.lock.Unlock()

	msg.connid = flipPrefixMultiplexerType(conn.id)

	return this.send(&msg)
}

func (this *fifoMultiplexer) handleContent(msg *prefixMultiplexerMessageContent) {
	var rep prefixMultiplexerMessageDisconnect
	var conn *fifoMultiplexerConnection
	var delivered bool

	this.lock.Lock()
	conn = this.conns[msg.connid]
	this.lock.Unlock()

	if conn != nil {
		delivered = conn.recv(msg.content)
	} else {
		this.log.Warn("deliver to unknown connection %d",
			this.log.Emph(1, msg.connid))
		delivered = false
	}

	if !delivered {
		rep.connid = flipPrefixMultiplexerType(msg.connid)
		go this.send(&rep)
	}
}

func (this *fifoMultiplexer) handleClosed(msg *prefixMultiplexerMessageClosed) {
	var remindex map[uint32]struct{} = make(map[uint32]struct{})
	var conn *fifoMultiplexerConnection
	var connid uint32
	var found bool

	for _, connid = range msg.remaining {
		remindex[connid] = struct{}{}
	}

	this.closeLocally()

	this.lock.Lock()

	for connid, conn = range this.conns {
		_, found = remindex[connid]
		if !found {
			this.log.Info("closing late connection %d",
				this.log.Emph(1, connid))
			this._recycleConnection(conn)
		}
	}

	this.lock.Unlock()
}

func (this *fifoMultiplexer) Connect() (Connection, error) {
	var msg prefixMultiplexerMessageConnect
	var conn *fifoMultiplexerConnection
	var err error

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil, io.EOF
	}

	conn = this._newLocalConnection()

	this.lock.Unlock()

	msg.connid = conn.id | prefix_multiplexer_type_remote

	err = this.send(&msg)
	if err != nil {
		this.recycleConnection(conn)
		return nil, err
	}

	return conn, nil
}

func (this *fifoMultiplexer) Accept() (Connection, error) {
	var conn *fifoMultiplexerConnection
	var ok bool

	conn, ok = <-this.acceptc
	if !ok {
		return nil, io.EOF
	}

	return conn, nil
}

func (this *fifoMultiplexer) isReferenced() bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	return (this.refcnt > 0)
}

func (this *fifoMultiplexer) closeLocally() []uint32 {
	var remaining []uint32
	var connid uint32

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return nil
	}

	this.refcnt -= 1
	this.closed = true

	this.log.Trace("close : %d %v", this.refcnt, this.conns)

	remaining = make([]uint32, 0, len(this.conns))
	for connid = range this.conns {
		remaining = append(remaining, connid)
	}

	close(this.acceptc)

	this.lock.Unlock()

	return remaining
}

func (this *fifoMultiplexer) Close() error {
	var remaining []uint32
	var index int

	remaining = this.closeLocally()
	if remaining == nil {
		return nil
	}

	for index = range remaining {
		remaining[index] = flipPrefixMultiplexerType(remaining[index])
	}

	return this.send(&prefixMultiplexerMessageClosed{ remaining })
}

func (this *fifoMultiplexer) _newLocalConnection() *fifoMultiplexerConnection {
	var conn *fifoMultiplexerConnection
	var connid uint32

	connid = this._newLocalId()
	conn = newFifoMultiplexerConnection(this, connid,
		this.log.WithLocalContext(fmt.Sprintf("[local:%d]", connid)))
	this.conns[connid] = conn
	this.refcnt += 1

	this.log.Trace("new local connection %d : %d %v",
		this.log.Emph(1, connid), this.refcnt, this.conns)

	return conn
}

func (this *fifoMultiplexer) _newRemoteConnection(connid uint32) *fifoMultiplexerConnection {
	var conn *fifoMultiplexerConnection

	conn = newFifoMultiplexerConnection(this, connid,
		this.log.WithLocalContext(fmt.Sprintf("[remote:%d]",
			connid & prefix_multiplexer_index_mask)))

	this.conns[connid] = conn
	this.refcnt += 1

	this.log.Trace("new remote connection %d (= %d) : %d %v",
		this.log.Emph(1, connid),
		connid & prefix_multiplexer_index_mask, this.refcnt,
		this.conns)

	return conn
}

func (this *fifoMultiplexer) _recycleConnection(conn *fifoMultiplexerConnection) {
	var ctype uint32

	ctype = conn.id & prefix_multiplexer_type_mask

	delete(this.conns, conn.id)
	this.refcnt -= 1

	if ctype == prefix_multiplexer_type_local {
		this._recycleLocalId(conn.id & prefix_multiplexer_index_mask)
		this.log.Trace("recycle local connection %d : %v %v",
			this.log.Emph(1, conn.id), this.refcnt, this.conns)
	} else {
		this.log.Trace("recycle remote connection %d (= %d) : %v %v",
			this.log.Emph(1, conn.id),
			conn.id & prefix_multiplexer_index_mask,
			this.refcnt, this.conns)
	}
}

func (this *fifoMultiplexer) recycleConnection(conn *fifoMultiplexerConnection) {
	this.lock.Lock()
	defer this.lock.Unlock()
	this._recycleConnection(conn)
}

func (this *fifoMultiplexer) _newLocalId() uint32 {
	var l int = len(this.freeLocals)
	var id uint32

	if l > 0 {
		id = this.freeLocals[l-1]
		this.freeLocals = this.freeLocals[:l-1]
	} else {
		id = this.nextLocal
		this.nextLocal += 1
	}

	this.log.Trace("new local id %d : %d %v", id, this.nextLocal,
		this.freeLocals)

	return id
}

func (this *fifoMultiplexer) _recycleLocalId(connid uint32) {
	this.freeLocals = append(this.freeLocals, connid)

	this.log.Trace("recycle local id %d : %d %v", connid, this.nextLocal,
		this.freeLocals)
}

func (this *fifoMultiplexer) send(msg Message) error {
	this.slock.Lock()
	defer this.slock.Unlock()
	return this.base.Send(msg, prefixMultiplexerProtocol)
}


type fifoMultiplexerConnection struct {
	parent *fifoMultiplexer
	id uint32
	log sio.Logger
	lock sync.Mutex
	closed bool
	recvc chan []byte
}

func newFifoMultiplexerConnection(parent *fifoMultiplexer, connid uint32, log sio.Logger) *fifoMultiplexerConnection {
	return &fifoMultiplexerConnection{
		parent: parent,
		id: connid,
		log: log,
		closed: false,
		recvc: make(chan []byte, 1024),
	}
}

func (this *fifoMultiplexerConnection) Send(msg Message, proto Protocol) error{
	var cmsg prefixMultiplexerMessageContent
	var buf bytes.Buffer
	var err error

	this.lock.Lock()
	if this.closed {
		this.lock.Unlock()
		return io.EOF
	}
	this.lock.Unlock()

	err = proto.Encode(sio.NewWriterSink(&buf), msg)
	if err != nil {
		return err
	}

	cmsg.connid = flipPrefixMultiplexerType(this.id)
	cmsg.content = buf.Bytes()

	this.log.Trace("send %v", cmsg.content)

	return this.parent.send(&cmsg)
}

func (this *fifoMultiplexerConnection) recv(content []byte) bool {
	this.log.Trace("receive %v", content)

	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		this.log.Trace("deliver to closed connection")
		return false
	}

	select {
	case this.recvc <- content:
	case <-time.After(1 * time.Second):
		this.lock.Unlock()
		this.log.Trace("deliver queue full")
		return false
	}

	this.lock.Unlock()

	return true
}

func (this *fifoMultiplexerConnection) Recv(proto Protocol) (Message, error) {
	var content []byte
	var ok bool

	content, ok = <-this.recvc
	if !ok {
		return nil, io.EOF
	}
	
	return proto.Decode(sio.NewReaderSource(bytes.NewBuffer(content)))
}

func (this *fifoMultiplexerConnection) closeLocally() bool {
	this.lock.Lock()

	if this.closed {
		this.lock.Unlock()
		return false
	}

	this.closed = true

	this.log.Trace("close")

	close(this.recvc)

	this.lock.Unlock()

	return true
}

func (this *fifoMultiplexerConnection) Close() error {
	var first bool

	first = this.closeLocally()
	if !first {
		return nil
	}

	return this.parent.closeConnection(this)
}


var prefixMultiplexerProtocol Protocol = NewUint8Protocol(map[uint8]Message{
	0: &prefixMultiplexerMessageConnect{},
	1: &prefixMultiplexerMessageDisconnect{},
	2: &prefixMultiplexerMessageContent{},
	3: &prefixMultiplexerMessageClosed{},
})

// A `connid` is made of two parts:
//
//   - bit 31: The connection type (local or remote) indicates which end has
//             created the connection (called `Connect()`).
//
//   - bit 30..0: The connection index is a 31 bits integer to identify the
//                connection.
//
const (
	prefix_multiplexer_type_local uint32 = 0 << 31
	prefix_multiplexer_type_remote uint32 = 1 << 31
	prefix_multiplexer_type_mask uint32 = 1 << 31
	prefix_multiplexer_index_mask uint32 = ^prefix_multiplexer_type_mask
)

func flipPrefixMultiplexerType(connid uint32) uint32 {
	var ctype, cindex uint32

	ctype = connid & prefix_multiplexer_type_mask
	cindex = connid & prefix_multiplexer_index_mask

	if ctype == prefix_multiplexer_type_local {
		return cindex | prefix_multiplexer_type_remote
	} else {
		return cindex | prefix_multiplexer_type_local
	}
}

type prefixMultiplexerMessage struct {
	connid uint32
}

func (this *prefixMultiplexerMessage) Encode(sink sio.Sink) error {
	return sink.WriteUint32(this.connid).Error()
}

func (this *prefixMultiplexerMessage) Decode(source sio.Source) error {
	return source.ReadUint32(&this.connid).Error()
}

// Ask to create a new connection with the given `connid`.
// The `connid` always has the `type` set to remote.
//
type prefixMultiplexerMessageConnect struct {
	prefixMultiplexerMessage
}

// Inform that the connection with the given `connid` has been closed.
//
type prefixMultiplexerMessageDisconnect struct {
	prefixMultiplexerMessage
}

// Convey a `content` attached to the connection with the `connid`.
//
type prefixMultiplexerMessageContent struct {
	prefixMultiplexerMessage
	content []byte
}

func (this *prefixMultiplexerMessageContent) Encode(sink sio.Sink) error {
	return sink.WriteEncodable(&this.prefixMultiplexerMessage).
		WriteBytes32(this.content).
		Error()
}

func (this *prefixMultiplexerMessageContent) Decode(source sio.Source) error {
	return source.ReadDecodable(&this.prefixMultiplexerMessage).
		ReadBytes32(&this.content).
		Error()
}

// Inform that the remote end has been closed.
//
type prefixMultiplexerMessageClosed struct {
	remaining []uint32
}

func (this *prefixMultiplexerMessageClosed) Encode(sink sio.Sink) error {
	var connid uint32

	sink = sink.WriteUint32(uint32(len(this.remaining)))
	for _, connid = range this.remaining {
		sink = sink.WriteUint32(connid)
	}

	return sink.Error()
}

func (this *prefixMultiplexerMessageClosed) Decode(source sio.Source) error {
	var i, l uint32

	return source.ReadUint32(&l).AndThen(func () error {
		this.remaining = make([]uint32, int(l))
		for i = 0; i < l; i++ {
			source = source.ReadUint32(&this.remaining[i])
		}
		return source.Error()
	}).Error()
}
