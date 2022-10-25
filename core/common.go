package core


import (
	"fmt"
	"math"
	"silk/net"
)


// ----------------------------------------------------------------------------


const MaxNameLength = math.MaxUint16

const MaxArgNumber = math.MaxUint16

const MaxArgLength = math.MaxUint16


type NameTooLongError struct {
	Name string
}

type TooManyArgumentsError struct {
	Args []string
}

type ArgumentTooLongError struct {
	Arg string
}

type UnknownMessageError struct {
	Msg net.Message
}


// ----------------------------------------------------------------------------


func (this *NameTooLongError) Error() string {
	return fmt.Sprintf("name too long: (%d bytes)", len(this.Name))
}

func (this *TooManyArgumentsError) Error() string {
	return fmt.Sprintf("too many arguments: %d", len(this.Args))
}

func (this *ArgumentTooLongError) Error() string {
	return fmt.Sprintf("argument too long: (%d bytes)", len(this.Arg))
}

func (this *UnknownMessageError) Error() string {
	return fmt.Sprintf("unknown message type: %T", this.Msg)
}
