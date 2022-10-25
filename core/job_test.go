package core


import (
	// "fmt"
	"testing"
)


// ----------------------------------------------------------------------------


func TestJobNoAddress(t *testing.T) {
	var err error

	_, err = NewJob("true", []string{}, []string{})
	if err == nil {
		t.Errorf("new shoud fail")
	}
}

func TestJobUnresolved(t *testing.T) {
	var err error

	_, err = NewJob("true", []string{}, []string{ "unresolved:1" })
	if err == nil {
		t.Errorf("new shoud fail")
	}
}

func TestJobDenied(t *testing.T) {
}

func TestJobBroken(t *testing.T) {
}

func TestJobEarlyCancel(t *testing.T) {
}

func TestJobLateCancel(t *testing.T) {
}

func TestJobTrue(t *testing.T) {
}

func TestJobFalse(t *testing.T) {
}

func TestJobBadName(t *testing.T) {
}

func TestJobPrintf(t *testing.T) {
}

func TestJobCat(t *testing.T) {
}

func TestJobManyTrue(t *testing.T) {
}

func TestJobManyFalse(t *testing.T) {
}

func TestJobManyBadName(t *testing.T) {
}

func TestJobManyPrintf(t *testing.T) {
}

func TestJobManyCat(t *testing.T) {
}

func TestJobManyTrueOneFailure(t *testing.T) {
}

func TestJobManyPrintfOneFailure(t *testing.T) {
}

func TestJobManyCatOneFailure(t *testing.T) {
}
