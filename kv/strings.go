package kv


import (
	"fmt"
	"math"
	"regexp"
	sio "silk/io"
)


// ----------------------------------------------------------------------------


type Key interface {
	sio.Decodable
	sio.Encodable
	String() string
}

func NewKey(str string) (Key, error) {
	return newKey(str)
}


type Value interface {
	sio.Encodable
	String() string
}

func NewValue(str string) (Value, error) {
	return newValue(str)
}


var NoValue Value = &value{}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type InvalidKeyError struct {
	Name string
}

type InvalidValueError struct {
	Content string
}


// ----------------------------------------------------------------------------


var keyRegexp *regexp.Regexp = regexp.MustCompile("^[-_a-zA-Z0-9/]{1,255}$")

type key struct {
	name string
}

func newKey(str string) (*key, error) {
	var this key

	if keyRegexp.Match([]byte(str)) == false {
		return nil, &InvalidKeyError{ str }
	}

	this.name = str

	return &this, nil
}

func (this *key) String() string {
	return this.name
}

func (this *key) Encode(sink sio.Sink) error {
	return sink.WriteString8(this.name).Error()
}

func (this *key) Decode(source sio.Source) error {
	return source.ReadString8(&this.name).Error()
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


const maxValueLength = math.MaxUint16

type value struct {
	content string
}

func newValue(str string) (*value, error) {
	var this value

	if len(str) > maxValueLength {
		return nil, &InvalidValueError{ str }
	}

	this.content = str

	return &this, nil
}

func (this *value) String() string {
	return this.content
}

func (this *value) Encode(sink sio.Sink) error {
	if this == NoValue {
		return sink.WriteUint8(0).Error()
	} else {
		return sink.WriteUint8(1).WriteString16(this.content).Error()
	}
}

func decodeValue(source sio.Source) (Value, sio.Source) {
	var has uint8
	var ret Value

	ret = nil

	source = source.ReadUint8(&has).AndThen(func () error {
		var v value

		if has == 0 {
			ret = NoValue
		} else {
			source = source.ReadString16(&v.content)
			ret = &v
		}

		return source.Error()
	})

	return ret, source
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *InvalidKeyError) Error() string {
	return fmt.Sprintf("invalid key name: %s", this.Name)
}

func (this *InvalidValueError) Error() string {
	return fmt.Sprintf("invalid value content: %s", this.Content)
}
