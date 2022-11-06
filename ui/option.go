package ui


import (
	"strconv"
)


// ----------------------------------------------------------------------------


type Option interface {
	Assignments() []OptionAssignment
}


type OptionAssignment interface {
	State() ParsingState
}


// This option must be supplied without a value.
//
// For short options, it means the following syntax is valid:
//
//   -o
//
// And bundling is possible with opther short options:
//
//   -lao   # equivalent to -l -a -o
//
// For long options, it means the following syntax is valid:
//
//   --opt
//
type OptionNullary interface {
	Option

	Assign(ParsingState) error
}


// This option must be supplied with a value.
//
// For short options, it means the following syntax are valid:
//
//   -ovalue
//   -o value
//
// And bundling is possible with other short options which do not need
// a value as long as this option is the last one of the bundle:
//
//   -laovalue   # equivalent to -l -a -ovalue
//   -lao value  # equivalent to -l -a -o value
//
// For long opiotns, it means the following syntax are valid:
//
//   --opt=value
//   --opt value
//
type OptionUnary interface {
	Option

	AssignValue(string, ParsingState) error
}


// This option can be supplied with a value.
//
// For short options, it is not possible to supply a value.
// For long options, it means the following syntax are valid:
//
//   --opt=value
//   --opt
//
type OptionVariadic interface {
	OptionNullary

	// OptionUnary   -   disabled for compatilibity with go 1.13
	AssignValue(string, ParsingState) error
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type OptBool struct {
}

func (this OptBool) New() OptionBool {
	return newOptionBool(&this)
}

type OptionBool interface {
	Option

	Value() bool
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type OptCall struct {
	// The function to call if specified without argument.
	//
	WithoutArg func () error

	// The function to call if specified with an argument.
	//
	WithArg func (string) error
}

func (this OptCall) New() Option {
	return newOptionCall(&this)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type OptInt struct {
	// Value if never assigned
	//
	DefaultValue int

	// Test if a provided value is valid.
	// If not, return an `error` indicating why the value is invalid.
	//
	ValidityPredicate func (int) error

	// Indicate if the option can be supplied with no value.
	//
	Variadic bool
}

func (this OptInt) New() OptionInt {
	return newOptionInt(&this)
}

type OptionInt interface {
	Option

	Value() int
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


// A validity predicate for `OptionPath` to check that the supplied path must
// exist.
//
var OptPathExists func (string) error = nil

// A validity predicate for `OptionPath` to check that the supplied path parent
// must exist.
//
var OptPathParentExists func (string) error = nil

// A validity predicate for `OptionPath` to check that the supplied path parent
// must exist and the path itself does not exist.
//
var OptPathOnlyParentExists func (string) error = nil


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type OptString struct {
	// Value if never assigned
	//
	DefaultValue string

	// Test if a provided value is valid.
	// If not, return an `error` indicating why the value is invalid.
	//
	ValidityPredicate func (string) error

	// Indicate if the option can be supplied with no value.
	//
	Variadic bool
}

func (this OptString) New() OptionString {
	return newOptionString(&this)
}

type OptionString interface {
	OptionUnary

	Value() string
}


// ----------------------------------------------------------------------------


type optionAssignment struct {
	state ParsingState
}

func newOptionAssignment(state ParsingState) *optionAssignment {
	var this optionAssignment

	this.state = state

	return &this
}

func (this *optionAssignment) State() ParsingState {
	return this.state
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type optionBase struct {
	assignments []OptionAssignment
}

func (this *optionBase) init() {
	this.assignments = make([]OptionAssignment, 0)
}

func (this *optionBase) assign(state ParsingState) {
	this.assignments = append(this.assignments, newOptionAssignment(state))
}

func (this *optionBase) Assignments() []OptionAssignment {
	return this.assignments
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type optionBool struct {
	optionBase
	value bool
}

func newOptionBool(params *OptBool) *optionBool {
	var this optionBool

	this.optionBase.init()
	this.value = false

	return &this
}

func (this *optionBool) Assign(state ParsingState) error {
	this.optionBase.assign(state)
	this.value = true
	return nil
}

func (this *optionBool) Value() bool {
	return this.value
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func newOptionCall(params *OptCall) Option {
	if (params.WithoutArg != nil) && (params.WithArg == nil) {
		return newOptionCallNullary(params.WithoutArg)
	} else if (params.WithoutArg == nil) && (params.WithArg != nil) {
		return newOptionCallUnary(params.WithArg)
	} else if (params.WithoutArg != nil) && (params.WithArg != nil) {
		return newOptionCallVariadic(params.WithoutArg, params.WithArg)
	} else {
		panic("no callback given")
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionCallNullary struct {
	optionBase
	call func () error
}

func newOptionCallNullary(call func () error) *optionCallNullary {
	var this optionCallNullary

	this.optionBase.init()
	this.call = call

	return &this
}

func (this *optionCallNullary) Assign(state ParsingState) error {
	var err error

	err = this.call()
	if err != nil {
		return err
	}

	this.assign(state)

	return nil
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionCallUnary struct {
	optionBase
	call func (string) error
}

func newOptionCallUnary(call func (string) error) *optionCallUnary {
	var this optionCallUnary

	this.optionBase.init()
	this.call = call

	return &this
}

func (this *optionCallUnary) AssignValue(v string, s ParsingState) error {
	var err error

	err = this.call(v)
	if err != nil {
		return err
	}

	this.assign(s)

	return nil
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionCallVariadic struct {
	optionBase
	calln func () error
	callu func (string) error
}

func newOptionCallVariadic(fn func () error, fu func (string) error) *optionCallVariadic {
	var this optionCallVariadic

	this.optionBase.init()
	this.calln = fn
	this.callu = fu

	return &this
}

func (this *optionCallVariadic) Assign(state ParsingState) error {
	var err error

	err = this.calln()
	if err != nil {
		return err
	}

	this.assign(state)

	return nil
}

func (this *optionCallVariadic) AssignValue(v string, s ParsingState) error {
	var err error

	err = this.callu(v)
	if err != nil {
		return err
	}

	this.assign(s)

	return nil
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type optionIntBase struct {
	optionBase
	value int
	parse func (string) (int, error)
	validate func (int) error
}

func (this *optionIntBase) init(params *OptInt) {
	this.optionBase.init()

	this.value = params.DefaultValue

	this.parse = func (s string) (int, error) {
		var i int64
		var e error

		i, e = strconv.ParseInt(s, 10, 64)
		if e != nil {
			return 0, e
		}

		return int(i), nil
	}

	if params.ValidityPredicate == nil {
		this.validate = func (int) error { return nil }
	} else {
		this.validate = params.ValidityPredicate
	}
}

func (this *optionIntBase) assign(v string, s ParsingState) error {
	var err error
	var i int

	i, err = this.parse(v)
	if err != nil {
		return err
	}

	err = this.validate(i)
	if err != nil {
		return err
	}

	this.optionBase.assign(s)

	this.value = i

	return nil
}

func (this *optionIntBase) Value() int {
	return this.value
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func newOptionInt(params *OptInt) OptionInt {
	if params.Variadic {
		return newOptionIntVariadic(params)
	} else {
		return nil
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionIntVariadic struct {
	optionIntBase
}

func newOptionIntVariadic(params *OptInt) *optionIntVariadic {
	var this optionIntVariadic

	this.init(params)

	return &this
}

func (this *optionIntVariadic) AssignValue(v string, s ParsingState) error {
	return this.optionIntBase.assign(v, s)
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type optionStringBase struct {
	optionBase
	value string
	validityPredicate func (string) error
}

func (this *optionStringBase) init(params *OptString) {
	this.optionBase.init()

	this.value = params.DefaultValue

	if params.ValidityPredicate == nil {
		this.validityPredicate = func (string) error { return nil }
	} else {
		this.validityPredicate = params.ValidityPredicate
	}
}

func (this *optionStringBase) assign(v string, state ParsingState) error {
	var err error

	err = this.validityPredicate(v)
	if err != nil {
		return err
	}

	this.optionBase.assign(state)

	this.value = v

	return nil
}

func (this *optionStringBase) Value() string {
	return this.value
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

func newOptionString(params *OptString) OptionString {
	if params.Variadic {
		return newOptionStringVariadic(params)
	} else {
		return newOptionStringUnary(params)
	}
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionStringUnary struct {
	optionStringBase
}

func newOptionStringUnary(params *OptString) *optionStringUnary {
	var this optionStringUnary

	this.optionStringBase.init(params)

	return &this
}

func (this *optionStringUnary) AssignValue(v string, s ParsingState) error {
	return this.optionStringBase.assign(v, s)
}

// -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -  -

type optionStringVariadic struct {
	optionStringBase
}

func newOptionStringVariadic(params *OptString) *optionStringVariadic {
	var this optionStringVariadic

	this.optionStringBase.init(params)

	return &this
}

func (this *optionStringVariadic) Assign(s ParsingState) error {
	this.optionBase.assign(s)
	return nil
}

func (this *optionStringVariadic) AssignValue(v string, s ParsingState) error {
	return this.optionStringBase.assign(v, s)
}
