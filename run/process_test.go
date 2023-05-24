package run


import (
	"testing"
)


// ----------------------------------------------------------------------------


func TestProcessTrue(t *testing.T) {
	var proc Process
	var err error
	var n uint8

	proc, err = NewProcess("true", []string{})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()

	n = proc.Exit() 
	if n != 0 {
		t.Errorf("exit: %d", n)
	}
}

func TestProcessFalse(t *testing.T) {
	var proc Process
	var err error

	proc, err = NewProcess("false", []string{})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()

	if proc.Exit() == 0 {
		t.Errorf("exit: 0")
	}
}

func TestProcessFalseWaitTwice(t *testing.T) {
	var proc Process
	var err error

	proc, err = NewProcess("false", []string{})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()
	proc.Wait()

	if proc.Exit() == 0 {
		t.Errorf("exit: 0")
	}
}

func TestProcessBadName(t *testing.T) {
	var err error

	_, err = NewProcess("__does_not_exist__", []string{})
	if err == nil {
		t.Errorf("new should fail")
	}
}

func TestProcessPrintf(t *testing.T) {
	var data []byte = make([]byte, 0)
	const msg = "Hello World!"
	var proc Process
	var err error
	var n uint8

	proc, err = NewProcessWith("printf", []string{ msg },
		&ProcessOptions{
			Stdout: func (b []byte) error {
				data = append(data, b...)
				return nil
			},
		})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()

	n = proc.Exit() 
	if n != 0 {
		t.Errorf("exit: %d", n)
	}

	if string(data) != msg {
		t.Errorf("stdout: '%s'", string(data))
	}
}

func TestProcessLsBadOption(t *testing.T) {
	var data []byte = make([]byte, 0)
	var proc Process
	var err error

	proc, err = NewProcessWith("ls", []string{ "--bad-option" },
		&ProcessOptions{
			Stderr: func (b []byte) error {
				data = append(data, b...)
				return nil
			},
		})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()

	if proc.Exit() == 0 {
		t.Errorf("exit: 0")
	}

	if string(data) == "" {
		t.Errorf("stderr: ''")
	}
}

func TestProcessCat(t *testing.T) {
	var data []byte = make([]byte, 0)
	const msg = "Hello World!"
	var proc Process
	var err error
	var n uint8

	proc, err = NewProcessWith("cat", []string{},
		&ProcessOptions{
			Stdout: func (b []byte) error {
				data = append(data, b...)
				return nil
			},
		})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	err = proc.Stdin([]byte(msg))
	if err != nil {
		t.Errorf("stdin: %v", err)
	}

	proc.CloseStdin()

	proc.Wait()

	n = proc.Exit()
	if n != 0 {
		t.Errorf("exit: %d", n)
	}

	if string(data) != msg {
		t.Errorf("stdout: '%s'", string(data))
	}
}

func TestProcessPrintEnv(t *testing.T) {
	var data []byte = make([]byte, 0)
	const val = "test_val"
	var proc Process
	var err error
	var n uint8

	proc, err = NewProcessWith("bash", []string{ "-c", "printf $VAR" },
		&ProcessOptions{
			Env: map[string]string{
				"VAR": val,
			},
			Stdout: func (b []byte) error {
				data = append(data, b...)
				return nil
			},
		})
	if err != nil {
		t.Errorf("new: %v", err)
	}

	proc.Wait()

	n = proc.Exit()
	if n != 0 {
		t.Errorf("exit: %d", n)
	}

	if string(data) != val {
		t.Errorf("stdout: '%s'", string(data))
	}
}
