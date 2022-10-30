package net


import (
	"testing"
)


// ----------------------------------------------------------------------------


type accepterTestSetup struct {
	accepter Accepter
	connectf func () Connection
	teardown func ()
}

type closeAccepterTestSetup struct {
	accepterTestSetup
	closef func ()
}


// ----------------------------------------------------------------------------


func testAccepter(t *testing.T, setupf func () *accepterTestSetup) {
	t.Logf("testAccepterAsyncN")
	testAccepterAsyncN(t, setupf())

	t.Logf("testAccepterSync")
	testAccepterSync(t, setupf())
}

func testCloseAccepter(t *testing.T, setupf func () *closeAccepterTestSetup) {
	t.Logf("testAccepterCloseImmediately")
	testAccepterCloseImmediately(t, setupf())

	t.Logf("testAccepterAsync")
	testAccepterAsync(t, setupf())

	t.Logf("testAccepterAsyncN")
	testAccepterAsyncN(t, &setupf().accepterTestSetup)

	t.Logf("testAccepterSync")
	testAccepterSync(t, &setupf().accepterTestSetup)
}


func testAccepterCloseImmediately(t *testing.T, setup *closeAccepterTestSetup){
	var more bool

	defer setup.teardown()

	setup.closef()

	_, more = <-setup.accepter.Accept()
	if more {
		t.Errorf("unexpected accept")
	}
}

func testAccepterAsync(t *testing.T, setup *closeAccepterTestSetup) {
	var cs []Connection = make([]Connection, 1000)
	var asc <-chan []Connection
	var ac <-chan Connection
	var as []Connection
	var more bool
	var i int

	defer setup.teardown()

	ac = transmitConnections(setup.accepter.Accept(), len(cs))
	asc = gatherConnections(ac, timeout(100))

	for i = range cs {
		cs[i] = setup.connectf()
		if cs[i] == nil {
			cs = cs[:i]
			break
		}
	}

	as = <-asc

	setup.closef()
	
	_, more = <-setup.accepter.Accept()
	if more {
		t.Errorf("unexpected accept")
	}

	if as == nil {
		t.Errorf("timeout")
	}

	testConnectionsConnectivity(cs, as, t)

	for i = range cs { close(cs[i].Send()) }
	for i = range as { close(as[i].Send()) }
}

func testAccepterAsyncN(t *testing.T, setup *accepterTestSetup) {
	var cs []Connection = make([]Connection, 1000)
	var asc <-chan []Connection
	var ac <-chan Connection
	var as []Connection
	var i int

	defer setup.teardown()

	ac = transmitConnections(setup.accepter.Accept(), len(cs))
	asc = gatherConnections(ac, timeout(100))

	for i = range cs {
		cs[i] = setup.connectf()
		if cs[i] == nil {
			cs = cs[:i]
			break
		}
	}

	as = <-asc

	if as == nil {
		t.Errorf("timeout")
	}

	testConnectionsConnectivity(cs, as, t)

	for i = range cs { close(cs[i].Send()) }
	for i = range as { close(as[i].Send()) }
}

func testAccepterSync(t *testing.T, setup *accepterTestSetup) {
	var cs []Connection = make([]Connection, 1000)
	var as []Connection = make([]Connection, 0, len(cs))
	var over <-chan struct{} = timeout(100)
	var a Connection
	var more bool
	var i int

	defer setup.teardown()

	loop: for i = range cs {
		cs[i] = setup.connectf()
		if cs[i] == nil {
			cs = cs[:i]
			break
		}

		select {
		case a, more = <-setup.accepter.Accept():
			if more == false {
				t.Errorf("closed unexpectedly")
				break loop
			} else {
				as = append(as, a)
			}
		case <-over:
			t.Errorf("timeout")
			break loop
		}
	}

	testConnectionsConnectivity(cs, as, t)

	for i = range cs { close(cs[i].Send()) }
	for i = range as { close(as[i].Send()) }
}
