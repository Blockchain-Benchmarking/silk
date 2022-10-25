package io


import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)


// ----------------------------------------------------------------------------


const (
	LOG_ERROR int = 1
	LOG_WARN  int = 2
	LOG_INFO  int = 3
	LOG_DEBUG int = 4
	LOG_TRACE int = 5
)

// A Logger object to selectively log information.
//
type Logger interface {
	// Log information likely to cause a fatal error.
	//
	Error(fstr string, args ...interface{})

	// Log information which is concerning but not (yet) causing a fatal
	// error.
	//
	Warn(fstr string, args ...interface{})

	// Log information which is not threatening the process stability but
	// is nevertheless noticeable.
	//
	Info(fstr string, args ...interface{})

	// Log information which is normally not important but can be useful
	// for debugging purpose.
	//
	Debug(fstr string, args ...interface{})

	// Log information which is only useful during development phase.
	//
	Trace(fstr string, args ...interface{})


	// Return a new `Logger` with the given `name` appended to its global
	// context.
	// If additional `args` are supplied then `name` is a printf format for
	// these `args`.
	//
	WithGlobalContext(name string, args ...interface{}) Logger

	// Return a new `Logger` with the given `name` appended to its local
	// context.
	// If additional `args` are supplied then `name` is a printf format for
	// these `args`.
	//
	WithLocalContext(name string, args ...interface{}) Logger


	// Return an emphasized version of the `arg` value.
	// Emphasis is useful to spot values related to each others in the log.
	// Because there can be several groups of related values, each group
	// can be identified with a `group` index.
	//
	Emph(group int, arg interface{}) interface{}
}


func NewNopLogger() Logger {
	return newNopLogger()
}


func NewStderrLogger(level int) Logger {
	return newFileLogger(os.Stderr, level)
}

func NewFileLogger(file *os.File, level int) Logger {
	return newFileLogger(file, level)
}

func NewWriterLogger(writer io.Writer, level int, color bool) Logger {
	return newWriterLogger(writer, level, color)
}


// ----------------------------------------------------------------------------


type nopLogger struct {
}

func newNopLogger() *nopLogger {
	return &nopLogger{}
}

func (this *nopLogger) Error(fstr string, args ...interface{}) {
}

func (this *nopLogger) Warn(fstr string, args ...interface{}) {
}

func (this *nopLogger) Info(fstr string, args ...interface{}) {
}

func (this *nopLogger) Debug(fstr string, args ...interface{}) {
}

func (this *nopLogger) Trace(fstr string, args ...interface{}) {
}

func (this *nopLogger) WithGlobalContext(string, ...interface{}) Logger {
	return this
}

func (this *nopLogger) WithLocalContext(string, ...interface{}) Logger {
	return this
}

func (this *nopLogger) Emph(group int, arg interface{}) interface{} {
	return nil
}


const (
	log_color_none string         = "\033[0m"

	log_color_red string          = "\033[31m"
	log_color_green string        = "\033[32m"
	log_color_yellow string       = "\033[33m"
	log_color_blue string         = "\033[34m"
	log_color_magenta string      = "\033[35m"
	log_color_teal string         = "\033[36m"

	log_color_bold_red string     = "\033[31;1m"
	log_color_bold_green string   = "\033[32;1m"
	log_color_bold_yellow string  = "\033[33;1m"
	log_color_bold_blue string    = "\033[34;1m"
	log_color_bold_magenta string = "\033[35;1m"
	log_color_bold_teal string    = "\033[36;1m"
)


type writerLogger struct {
	lock sync.Mutex
	writer io.Writer
	globalContext string
	localContext string
	context string
	lfuncs []func (*writerLogger, string, ...interface{})
	efunc func (int, interface{}) interface{}
	gcfunc func (string) string
	lcfunc func (string) string
}

func newWriterLogger(writer io.Writer, level int, color bool) *writerLogger {
	var lfuncs []func (*writerLogger, string, ...interface{})
	var this writerLogger
	var l int

	this.writer = writer
	this.globalContext = ""
	this.localContext = ""
	this.context = ""

	if color {
		lfuncs = writerLoggerColorFuncs
		this.efunc = writerLoggerColorEmph
		this.gcfunc = writerLoggerColorGcFunc
		this.lcfunc = writerLoggerColorLcFunc
	} else {
		lfuncs = writerLoggerPlainFuncs
		this.efunc = writerLoggerPlainEmph
		this.gcfunc = writerLoggerPlainGcFunc
		this.lcfunc = writerLoggerPlainLcFunc
	}

	this.lfuncs = make([]func (*writerLogger, string, ...interface{}), 0)
	for l = 0; l <= LOG_TRACE; l++ {
		if l <= level {
			this.lfuncs = append(this.lfuncs, lfuncs[l])
		} else {
			this.lfuncs = append(this.lfuncs, writerLoggerNopFunc)
		}
	}

	return &this
}

func newFileLogger(file *os.File, level int) *writerLogger {
	var fi os.FileInfo
	var color bool
	var err error

	fi, err = file.Stat()

	if (err == nil) && ((fi.Mode() & os.ModeCharDevice) != 0) {
		color = true
	} else {
		color = false
	}

	return newWriterLogger(file, level, color)
}

func (this *writerLogger) child(globalContext, localContext string) Logger {
	var clogger writerLogger

	clogger.writer = this.writer
	clogger.globalContext = globalContext
	clogger.localContext = localContext

	if len(localContext) == 0 {
		if len(globalContext) == 0 {
			clogger.context = ""
		} else {
			clogger.context = this.gcfunc(globalContext) + " "
		}
	} else if len(globalContext) == 0 {
		clogger.context = this.lcfunc(localContext) + " "
	} else {
		clogger.context = this.gcfunc(globalContext) + "::" +
			this.lcfunc(localContext) + " "
	}

	clogger.lfuncs = this.lfuncs
	clogger.efunc = this.efunc
	clogger.gcfunc = this.gcfunc
	clogger.lcfunc = this.lcfunc

	return &clogger
}

func (this *writerLogger) WithGlobalContext(name string, args ...interface{}) Logger {
	var globalContext string

	if len(args) > 0 {
		name = fmt.Sprintf(name, args...)
	}

	if len(this.globalContext) == 0 {
		globalContext = name
	} else if len(name) == 0 {
		globalContext = this.globalContext
	} else {
		globalContext = this.globalContext + ":" + name
	}

	return this.child(globalContext, this.localContext)
}

func (this *writerLogger) WithLocalContext(name string, args ...interface{}) Logger {
	var localContext string

	if len(args) > 0 {
		name = fmt.Sprintf(name, args...)
	}

	if len(this.localContext) == 0 {
		localContext = name
	} else if len(name) == 0 {
		localContext = this.localContext
	} else {
		localContext = this.localContext + ":" + name
	}

	return this.child(this.globalContext, localContext)
}

func (this *writerLogger) Emph(group int, arg interface{}) interface{} {
	return this.efunc(group, arg)
}

var writerLoggerPlainEmph = func (group int, arg interface{}) interface{} {
	return arg
}

type writerLoggerEmph struct {
	group int
	arg interface{}
}

var writerLoggerColorEmph = func (group int, arg interface{}) interface{} {
	return &writerLoggerEmph{ group, arg }
}

var writerLoggerEmphColors = []string{
	log_color_teal,
	log_color_green,
	log_color_yellow,
}

func (this *writerLogger) logEmph(fstr, lstr string, args ...interface{}) {
	var builder, buffer strings.Builder
	var emph *writerLoggerEmph
	var aindex, colIndex int
	var format bool = false
	var ok bool = false
	var col string
	var c rune

	builder.Grow(len(fstr))

	for _, c = range fstr {
		if !format {
			if c == '%' {
				format = true
			} else {
				builder.WriteRune(c)
			}

			continue
		}

		switch c {
		case 'd', 'o', 'O', 'b', 'x', 'X', 'f', 'F', 'e', 'E',
			'g', 'G', 'c', 'q', 'U', 't', 's', 'v', 'T',
			'p':

			if aindex < len(args) {
				emph, ok = args[aindex].(*writerLoggerEmph)
			} else {
				ok = false
			}

			if ok {
				colIndex = len(writerLoggerEmphColors)
				colIndex = emph.group % colIndex
				col = writerLoggerEmphColors[colIndex]

				args[aindex] = emph.arg

				builder.WriteString(col)
			}

			builder.WriteRune('%')
			builder.WriteString(buffer.String())
			builder.WriteRune(c)
			buffer.Reset()

			if ok {
				builder.WriteString(log_color_none)
			}

			aindex += 1
			format = false
			continue
		case '%':
			builder.WriteRune('%')
			builder.WriteRune(c)
			format = false
			continue
		default:
			buffer.WriteRune(c)
			continue
		}
	}

	this.log(builder.String(), lstr, args...)
}

func (this *writerLogger) write(data []byte) {
	this.lock.Lock()
	this.writer.Write(data)
	this.lock.Unlock()
}

func (this *writerLogger) log(fstr string, lstr string, args ...interface{}) {
	var buf bytes.Buffer
	var now time.Time = time.Now().UTC()

	fmt.Fprintf(&buf, "%d-%02d-%02d %02d:%02d:%02d.%09d %s %s",
		now.Year(), now.Month(), now.Day(),
		now.Hour(), now.Minute(), now.Second(), now.Nanosecond(),
		lstr, this.context)

	fmt.Fprintf(&buf, fstr, args...)

	fmt.Fprintf(&buf, "\n")

	this.write(buf.Bytes())
}

var writerLoggerNopFunc = func (*writerLogger, string, ...interface{}) {}


var writerLoggerPlainFuncs = []func (*writerLogger, string, ...interface{}){
	func (this *writerLogger, fstr string, args ...interface{}) {
		panic("dead code")
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.log(fstr, "ERROR", args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.log(fstr, "WARN ", args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.log(fstr, "INFO ", args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.log(fstr, "DEBUG", args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.log(fstr, "TRACE", args...)
	},
}

var writerLoggerPlainGcFunc = func (str string) string { return str }

var writerLoggerPlainLcFunc = func (str string) string { return str }


var writerLoggerColorFuncs = []func (*writerLogger, string, ...interface{}){
	func (this *writerLogger, fstr string, args ...interface{}) {
		panic("dead code")
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.logEmph(fstr, log_color_red + "ERROR" + log_color_none,
			args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.logEmph(fstr, log_color_yellow + "WARN " + log_color_none,
			args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.logEmph(fstr, log_color_green + "INFO " + log_color_none,
			args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.logEmph(fstr, log_color_blue + "DEBUG" + log_color_none,
			args...)
	},
	func (this *writerLogger, fstr string, args ...interface{}) {
		this.logEmph(fstr,log_color_magenta + "TRACE" + log_color_none,
			args...)
	},
}

var writerLoggerColorGcFunc = func (str string) string {
	return log_color_magenta + str + log_color_none
}

var writerLoggerColorLcFunc = func (str string) string {
	return log_color_green + str + log_color_none
}


func (this *writerLogger) Error(fstr string, args ...interface{}) {
	this.lfuncs[LOG_ERROR](this, fstr, args...)
}

func (this *writerLogger) Warn(fstr string, args ...interface{}) {
	this.lfuncs[LOG_WARN](this, fstr, args...)
}

func (this *writerLogger) Info(fstr string, args ...interface{}) {
	this.lfuncs[LOG_INFO](this, fstr, args...)
}

func (this *writerLogger) Debug(fstr string, args ...interface{}) {
	this.lfuncs[LOG_DEBUG](this, fstr, args...)
}

func (this *writerLogger) Trace(fstr string, args ...interface{}) {
	this.lfuncs[LOG_TRACE](this, fstr, args...)
}
