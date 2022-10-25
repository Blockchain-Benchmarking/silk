package core


import (
	"context"
	"fmt"
	"silk/net"
	"strings"
	"testing"
	"time"
)


// ----------------------------------------------------------------------------


func TestTcpServerNameLargest(t *testing.T) {
	var server Server
	var err error

	server, err = NewTcpServerWith(0, &ServerOptions{
		Name: strings.Repeat("a", MaxNameLength),
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if server.Name() == "" {
		t.Errorf("name: ''")
	}
}

func TestTcpServerNameTooLong(t *testing.T) {
	var err error
	var ok bool

	_, err = NewTcpServerWith(0, &ServerOptions{
		Name: strings.Repeat("a", MaxNameLength + 1),
	})
	if err == nil {
		t.Fatalf("new should fail")
	}

	_, ok = err.(*NameTooLongError)
	if !ok {
		t.Fatalf("error type: %T", err)
	}
}

func TestTcpServerName(t *testing.T) {
	var server Server
	var err error

	server, err = NewTcpServer(0)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if server.Name() == "" {
		t.Errorf("name: ''")
	}
}

func TestTcpServerRunCancel(t *testing.T) {
	var cancel context.CancelFunc
	var ctx context.Context
	var server Server
	var err error

	server, err = NewTcpServer(0)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, cancel = context.WithCancel(context.Background())

	done := make(chan struct{})
	go func () {
		select {
		case <-done:
			t.Errorf("server stopped")
		case <-time.After(50 * time.Millisecond):
			cancel()
		}
	}()

	err = server.Run(ctx)
	if err != context.Canceled {
		t.Errorf("run: %v", err)
	}

	close(done)
}

func TestTcpServerRunClose(t *testing.T) {
	var server Server
	var err error

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunNameClose(t *testing.T) {
	var m *nameReplyMessage
	var msg net.Message
	var server Server
	var err error
	var ok bool

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&nameRequestMessage{}, protocol)
	if err != nil {
		t.Errorf("send name: %v", err)
	}

	msg, err = conn.Recv(protocol)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}

	m, ok = msg.(*nameReplyMessage)
	if !ok {
		t.Fatalf("recv type: %T:%v", msg, msg)
	}

	if m.name != server.Name() {
		t.Errorf("name: '%s' != '%s'", m.name, server.Name())
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunTrueClose(t *testing.T) {
	var msg net.Message
	var m *exitMessage
	var server Server
	var err error
	var ok bool

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "true",
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	for m == nil {
		msg, err = conn.Recv(protocol)
		if err != nil {
			t.Errorf("recv: %v", err)
			break
		}

		m, ok = msg.(*exitMessage)
		if !ok {
			continue
		}

		if m.code != 0 {
			t.Errorf("exit: %d", m.code)
		}
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunFalseClose(t *testing.T) {
	var msg net.Message
	var m *exitMessage
	var server Server
	var err error
	var ok bool

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "false",
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	for m == nil {
		msg, err = conn.Recv(protocol)
		if err != nil {
			t.Errorf("recv: %v", err)
			break
		}

		m, ok = msg.(*exitMessage)
		if !ok {
			continue
		}

		if m.code == 0 {
			t.Errorf("exit: 0")
		}
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunManyTrueClose(t *testing.T) {
	var msg net.Message
	var m *exitMessage
	var server Server
	var err error
	var ok bool
	const n = 3

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	for i := 0; i < n; i++ {
		err = conn.Send(&runMessage{
			name: "true",
		}, protocol)
		if err != nil {
			t.Errorf("send run: %v", err)
		}

		m = nil
		for m == nil {
			msg, err = conn.Recv(protocol)
			if err != nil {
				t.Errorf("recv: %v", err)
				break
			}

			m, ok = msg.(*exitMessage)
			if !ok {
				continue
			}

			if m.code != 0 {
				t.Errorf("exit: %d", m.code)
			}
		}
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunBadNameClose(t *testing.T) {
	var m *errorMessage
	var msg net.Message
	var server Server
	var err error
	var ok bool

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
 	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "__does_not_exist__",
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	msg, err = conn.Recv(protocol)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}

	m, ok = msg.(*errorMessage)
	if !ok {
		t.Fatalf("recv type: %T:%v", msg, msg)
	}

	if m.text == "" {
		t.Errorf("recv empty error")
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunBadNameStdinClose(t *testing.T) {
	var m *errorMessage
	var msg net.Message
	var server Server
	var err error
	var ok bool

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "__does_not_exist__",
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	err = conn.Send(&stdinMessage{
		content: []byte("Hello World!"),
	}, protocol)
	if err != nil {
		t.Errorf("send stdin: %v", err)
	}

	msg, err = conn.Recv(protocol)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}

	m, ok = msg.(*errorMessage)
	if !ok {
		t.Fatalf("recv type: %T:%v", msg, msg)
	}

	if m.text == "" {
		t.Errorf("recv empty error")
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunPrintfClose(t *testing.T) {
	var b []byte = make([]byte, 0)
	const strmsg = "Hello World!"
	var msg net.Message
	var server Server
	var err error

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "printf",
		args: []string{ strmsg },
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	recv_loop: for {
		msg, err = conn.Recv(protocol)
		if err != nil {
			t.Errorf("recv: %v", err)
			break
		}

		switch m := msg.(type) {
		case *stdoutMessage:
			b = append(b, m.content...)
		case *exitMessage:
			if m.code != 0 {
				t.Errorf("exit: %d", m.code)
			}
			break recv_loop
		}
	}

	if string(b) != strmsg {
		t.Errorf("stdout: '%s'", string(b))
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunManyPrintfClose(t *testing.T) {
	const strfmt = "Hello World! %d"
	var msg net.Message
	var server Server
	var err error
	var b []byte
	const n = 3

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	for i := 0; i < n; i++ {
		strmsg := fmt.Sprintf(strfmt, i)
		b = make([]byte, 0)

		err = conn.Send(&runMessage{
			name: "printf",
			args: []string{ strmsg },
		}, protocol)
		if err != nil {
			t.Errorf("send run: %v", err)
		}

		recv_loop: for {
			msg, err = conn.Recv(protocol)
			if err != nil {
				t.Errorf("recv: %v", err)
				break
			}

			switch m := msg.(type) {
			case *stdoutMessage:
				b = append(b, m.content...)
			case *exitMessage:
				if m.code != 0 {
					t.Errorf("exit: %d", m.code)
				}
				break recv_loop
			}
		}

		if string(b) != strmsg {
			t.Errorf("stdout: '%s'", string(b))
		}
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunLsBadOptionClose(t *testing.T) {
	var b []byte = make([]byte, 0)
	var msg net.Message
	var server Server
	var err error

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "ls",
		args: []string{ "--bad-option" },
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	recv_loop: for {
		msg, err = conn.Recv(protocol)
		if err != nil {
			t.Errorf("recv: %v", err)
			break
		}

		switch m := msg.(type) {
		case *stderrMessage:
			b = append(b, m.content...)
		case *exitMessage:
			if m.code == 0 {
				t.Errorf("exit: 0")
			}
			break recv_loop
		}
	}

	if len(b) == 0 {
		t.Errorf("stderr: ''")
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunCatClose(t *testing.T) {
	var b []byte = make([]byte, 0)
	const strmsg = "Hello World!"
	var msg net.Message
	var server Server
	var err error

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	err = conn.Send(&runMessage{
		name: "cat",
		args: []string{},
	}, protocol)
	if err != nil {
		t.Errorf("send run: %v", err)
	}

	err = conn.Send(&stdinMessage{
		content: []byte(strmsg),
	}, protocol)
	if err != nil {
		t.Errorf("send stdin: %v", err)
	}

	err = conn.Send(&closeStdinMessage{}, protocol)
	if err != nil {
		t.Errorf("send close stdin: %v", err)
	}

	recv_loop: for {
		msg, err = conn.Recv(protocol)
		if err != nil {
			t.Errorf("recv: %v", err)
			break
		}

		switch m := msg.(type) {
		case *stdoutMessage:
			b = append(b, m.content...)
		case *exitMessage:
			if m.code != 0 {
				t.Errorf("exit: %d", m.code)
			}
			break recv_loop
		}
	}

	if string(b) != strmsg {
		t.Errorf("stdout: '%s'", string(b))
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}

func TestTcpServerRunManyCatClose(t *testing.T) {
	const strfmt = "Hello World! %d"
	var msg net.Message
	var server Server
	var err error
	var b []byte
	const n = 3

	port := findTcpPort(t)

	server, err = NewTcpServer(port)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, _ := context.WithTimeout(context.Background(),
		100 * time.Millisecond)

	done := make(chan error)
	go func () {
		done <- server.Run(ctx)
		close(done)
	}()

	conn, err := net.NewTcpConnectionWith(fmt.Sprintf("localhost:%d",port),
		&net.TcpConnectionOptions{ Context: ctx })
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	for i := 0; i < n; i++ {
		strmsg := fmt.Sprintf(strfmt, i)
		b = make([]byte, 0)

		err = conn.Send(&runMessage{
			name: "cat",
			args: []string{},
		}, protocol)
		if err != nil {
			t.Errorf("send run: %v", err)
		}

		err = conn.Send(&stdinMessage{
			content: []byte(strmsg),
		}, protocol)
		if err != nil {
			t.Errorf("send stdin: %v", err)
		}

		err = conn.Send(&closeStdinMessage{}, protocol)
		if err != nil {
			t.Errorf("send close stdin: %v", err)
		}

		recv_loop: for {
			msg, err = conn.Recv(protocol)
			if err != nil {
				t.Errorf("recv: %v", err)
				break
			}

			switch m := msg.(type) {
			case *stdoutMessage:
				b = append(b, m.content...)
			case *exitMessage:
				if m.code != 0 {
					t.Errorf("exit: %d", m.code)
				}
				break recv_loop
			}
		}

		if string(b) != strmsg {
			t.Errorf("stdout: '%s'", string(b))
		}
	}

	err = conn.Send(&closeMessage{}, protocol)
	if err != nil {
		t.Errorf("send close: %v", err)
	}

	err = <-done
	if err != nil {
		t.Errorf("run: %v", err)
	}
}
