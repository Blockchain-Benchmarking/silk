// +build go1.18


package goleak


import (
	"go.uber.org/goleak"
	"testing"
)


func VerifyNone(t *testing.T) {
	goleak.VerifyNone(t)
}
