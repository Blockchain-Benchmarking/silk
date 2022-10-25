package io


import (
	"io"
	"testing"
)


type nopWriter struct {
}

func newNopWriter() *nopWriter {
	return &nopWriter{}
}

func (this *nopWriter) Write(b []byte) (int, error) {
	return len(b), nil
}


func BenchmarkNopLogPlain(b *testing.B) {
	var log Logger = NewNopLogger()
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkNopLogPlainContext(b *testing.B) {
	var log Logger = NewNopLogger().
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkNopLogPlainEmph(b *testing.B) {
	var log Logger = NewNopLogger()
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}

func BenchmarkNopLogPlainContextEmph(b *testing.B) {
	var log Logger = NewNopLogger().
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}


func BenchmarkWriterLogPlainSuppressed(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_INFO, false)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogPlain(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, false)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogPlainContext(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, false).
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogPlainEmph(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, false)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}

func BenchmarkWriterLogPlainContextEmph(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, false).
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}

func BenchmarkWriterLogColorSuppressed(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_INFO, true)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogColor(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, true)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogColorContext(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, true).
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", "foo", 42, struct{}{})
	}
}

func BenchmarkWriterLogColorEmph(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, true)
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}

func BenchmarkWriterLogColorContextEmph(b *testing.B) {
	var writer io.Writer = newNopWriter()
	var log Logger = NewWriterLogger(writer, LOG_TRACE, true).
		WithGlobalContext("global0").
		WithGlobalContext("global1").
		WithLocalContext("local0").
		WithLocalContext("local1")
	var i int

	for i = 0; i < b.N; i++ {
		log.Trace("Formatting string %s %d %v", log.Emph(0, "foo"),
			log.Emph(1, 42), log.Emph(2, struct{}{}))
	}
}
