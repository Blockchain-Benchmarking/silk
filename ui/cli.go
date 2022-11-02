package ui


import (
	"fmt"
	"strings"
)


// ----------------------------------------------------------------------------


type Cli interface {
	AddOption(rune, string, Option)
	AddShortOption(rune, Option)
	AddLongOption(string, Option)

	DelOption(rune, string)
	DelShortOption(rune)
	DelLongOption(string)

	Append([]string)
	Arguments() []string
	Parsed() int

	Parse() error

	SkipWord() (string, bool)
}


type ParsingState interface {
	// Return the slice of all arguments that have been appended until now.
	//
	Arguments() []string

	// Return the index of the currently parsed word in `Arguments()`.
	//
	CurrentWord() int

	// Return the index of the currently parsed rune in the word indicated
	// by `CurruentWord()`.
	//
	CurrentRune() int

	// Return the index of the word containing the option currently parsed.
	//
	OptionWord() int

	// Return the indices of the first and last runes in the option word
	// containing the option currently parsed.
	//
	OptionRunes() (int, int)

	// Return the index of the word containing the value of the option
	// currently parsed.
	//
	ValueWord() int

	// Return the indices of the first and last runes in the value word
	// containing the value for the option currently parsed.
	//
	ValueRunes() (int, int)
}


func NewCli() Cli {
	return newCli()
}


type CliUnknownOptionError struct {
	State ParsingState
}

type CliUnexpectedOptionValueError struct {
	State ParsingState
}

type CliMissingOptionValueError struct {
	State ParsingState
}


// ----------------------------------------------------------------------------


type cli struct {
	shorts map[rune]Option
	longs map[string]Option
	state *cliParsingState
	next int
}

func newCli() *cli {
	var this cli

	this.shorts = make(map[rune]Option)
	this.longs = make(map[string]Option)
	this.state = newCliParsingState()
	this.next = 0

	return &this
}

func (this *cli) AddOption(r rune, s string, o Option) {
	this.AddShortOption(r, o)
	this.AddLongOption(s, o)
}

func (this *cli) AddShortOption(r rune, o Option) {
	if o == nil {
		panic("nil option")
	} else if this.shorts[r] != nil {
		panic("conflicting options")
	}

	this.shorts[r] = o
}

func (this *cli) AddLongOption(s string, o Option) {
	if o == nil {
		panic("nil option")
	} else if this.longs[s] != nil {
		panic("conflicting options")
	}

	this.longs[s] = o
}

func (this *cli) DelOption(r rune, s string) {
	this.DelShortOption(r)
	this.DelLongOption(s)
}

func (this *cli) DelShortOption(r rune) {
	if this.shorts[r] == nil {
		panic("no option to delete")
	}

	delete(this.shorts, r)
}

func (this *cli) DelLongOption(s string) {
	if this.longs[s] == nil {
		panic("no option to delete")
	}

	delete(this.longs, s)
}

func (this *cli) Append(args []string) {
	this.state.args = append(this.state.args, args...)
}

func (this *cli) Arguments() []string {
	return this.state.args
}

func (this *cli) Parsed() int {
	return this.state.CurrentWord()
}

func (this *cli) SkipWord() (string, bool) {
	var word string

	if this.state.hasMoreWord() == false {
		return "", false
	}

	word = this.state.getCurrentWord()
	this.state.nextOptionWord()

	return word, true
}

func (this *cli) Parse() error {
	var pending OptionUnary
	var word string
	var err error

	for this.state.hasMoreWord() {
		word = this.state.getCurrentWord()

		if pending != nil {
			this.state.setValueRunes(0, len([]rune(word)))
			err = pending.AssignValue(word, this.state.clone())
			if err != nil {
				return err
			}

			pending = nil
			goto next
		}

		if strings.HasPrefix(word, "--") {
			if word == "--" {
				// end of options
				this.state.nextOptionWord()
				break
			}

			pending, err = this.parseLong()
			if err != nil {
				return err
			}
		} else if strings.HasPrefix(word, "-") {
			pending, err = this.parseShort()
			if err != nil {
				return err
			}
		} else {
			// end of options
			break
		}

	next:
		if pending != nil {
			this.state.nextValueWord()
		} else {
			this.state.nextOptionWord()
		}
	}

	if pending != nil {
		return &CliMissingOptionValueError{ this.state.clone() }
	}

	return nil
}

func (this *cli) parseLong() (OptionUnary, error) {
	var word, opname, opval string
	var ovar OptionVariadic
	var onull OptionNullary
	var oun OptionUnary
	var err error
	var op Option
	var ok bool
	var eq int

	word = this.state.getOptionWord()

	eq = strings.IndexRune(word, '=')
	if eq == -1 {
		this.state.setOptionRunes(2, len([]rune(word)))
	} else {
		this.state.setOptionRunes(2, eq)
		this.state.setValueWord()
		this.state.setValueRunes(eq + 1, len([]rune(word)))
		opval = this.state.getValue()
	}

	opname = this.state.getOptionLong()

	op = this.longs[opname]
	if op == nil {
		return nil, &CliUnknownOptionError{ this.state.clone() }
	}

	if ovar, ok = op.(OptionVariadic); ok {
		if eq == -1 {
			err = ovar.Assign(this.state.clone())
		} else {
			err = ovar.AssignValue(opval, this.state.clone())
		}
	} else if oun, ok = op.(OptionUnary); ok {
		if eq >= 0 {
			err = oun.AssignValue(opval, this.state.clone())
		} else {
			return oun, nil
		}
	} else if onull, ok = op.(OptionNullary); ok {
		if eq == -1 {
			err = onull.Assign(this.state.clone())
		} else {
			err = &CliUnexpectedOptionValueError{
				this.state.clone(),
			}
		}
	}

	return nil, err
}

func (this *cli) parseShort() (OptionUnary, error) {
	var ovar OptionVariadic
	var onull OptionNullary
	var oun OptionUnary
	var word []rune
	var opname rune
	var op Option
	var err error
	var ok bool
	var i int

	word = []rune(this.state.getOptionWord())

	for i = 1; i < len(word); i++ {
		this.state.setOptionRunes(i, i+1)

		opname = this.state.getOptionShort()

		op = this.shorts[opname]
		if op == nil {
			return nil, &CliUnknownOptionError{this.state.clone()}
		}

		if ovar, ok = op.(OptionVariadic); ok {
			err = ovar.Assign(this.state.clone())
		} else if oun, ok = op.(OptionUnary); ok {
			if i < (len(word) - 1) {
				err = oun.AssignValue(string(word[i+1:]),
					this.state.clone())
				return nil, err
			} else {
				return oun, nil
			}
		} else if onull, ok = op.(OptionNullary); ok {
			err = onull.Assign(this.state.clone())
		}

		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}


type cliParsingState struct {
	args []string
	currentWord int
	optionWord int
	optionFirst int
	optionEnd int
	valueWord int
	valueFirst int
	valueEnd int
}

func newCliParsingState() *cliParsingState {
	var this cliParsingState

	this.args = make([]string, 0)
	this.currentWord = 0
	this.optionWord = 0
	this.optionFirst = 0
	this.optionEnd = 0
	this.valueWord = 0
	this.valueFirst = 0
	this.valueEnd = 0

	return &this
}

func (this *cliParsingState) clone() *cliParsingState {
	var c cliParsingState

	c = *this

	return &c
}

func (this *cliParsingState) Arguments() []string {
	return this.args
}

func (this *cliParsingState) hasMoreWord() bool {
	return this.CurrentWord() < len(this.Arguments())
}

func (this *cliParsingState) nextOptionWord() {
	this.nextCurrentWord()
	this.setOptionWord()
	this.setValueWord()
}

func (this *cliParsingState) nextValueWord() {
	this.nextCurrentWord()
	this.setValueWord()
}

func (this *cliParsingState) getCurrentWord() string {
	return this.Arguments()[this.CurrentWord()]
}	

func (this *cliParsingState) setCurrentWord(n int) {
	this.currentWord = n
}	

func (this *cliParsingState) nextCurrentWord() {
	this.setCurrentWord(this.CurrentWord() + 1)
}	

func (this *cliParsingState) CurrentWord() int {
	return this.currentWord
}

func (this *cliParsingState) CurrentRune() int {
	return 0
}

func (this *cliParsingState) getOptionWord() string {
	return this.Arguments()[this.optionWord]
}

func (this *cliParsingState) setOptionWord() {
	this.optionWord = this.currentWord
	this.optionFirst = 0
	this.optionEnd = 0
}

func (this *cliParsingState) OptionWord() int {
	return this.optionWord
}

func (this *cliParsingState) getOptionLong() string {
	var f, e int

	f, e = this.OptionRunes()

	return string([]rune(this.getOptionWord())[f:e])
}

func (this *cliParsingState) getOptionShort() rune {
	var f int

	f, _ = this.OptionRunes()

	return []rune(this.getOptionWord())[f]
}

func (this *cliParsingState) setOptionRunes(first int, end int) {
	this.optionFirst = first
	this.optionEnd = end
}

func (this *cliParsingState) OptionRunes() (int, int) {
	return this.optionFirst, this.optionEnd
}

func (this *cliParsingState) setValueWord() {
	this.valueWord = this.currentWord
	this.valueFirst = 0
	this.valueEnd = 0
}

func (this *cliParsingState) getValueWord() string {
	return this.Arguments()[this.ValueWord()]
}

func (this *cliParsingState) ValueWord() int {
	return this.valueWord
}

func (this *cliParsingState) setValueRunes(first int, end int) {
	this.valueFirst = first
	this.valueEnd = end
}

func (this *cliParsingState) getValue() string {
	var f, e int

	f, e = this.ValueRunes()

	return string([]rune(this.getValueWord())[f:e])
}

func (this *cliParsingState) ValueRunes() (int, int) {
	return this.valueFirst, this.valueEnd
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


func (this *CliUnknownOptionError) Error() string {
	var ow, f, e int
	var args []string

	args = this.State.Arguments()
	ow = this.State.OptionWord()
	f, e = this.State.OptionRunes()

	return fmt.Sprintf("unknown option '%s' in '%s'",
		string([]rune(args[ow])[f:e]), args[ow])
}

func (this *CliUnexpectedOptionValueError) Error() string {
	var ow, vw, of, oe, vf, ve int
	var args []string

	args = this.State.Arguments()
	ow = this.State.OptionWord()
	of, oe = this.State.OptionRunes()
	vw = this.State.ValueWord()
	vf, ve = this.State.ValueRunes()

	if (vf != 0) || (ve != len([]rune(args[vw]))) {
		return fmt.Sprintf("unexpected value '%s' in '%s' for " +
			"option '%s'", string([]rune(args[vw])[vf:ve]),
			args[vw], string([]rune(args[ow])[of:oe]))
	} else {
		return fmt.Sprintf("unexpected value '%s' for option '%s'",
			args[vw], string([]rune(args[ow])[of:oe]))
	}
}

func (this *CliMissingOptionValueError) Error() string {
	var ow, f, e int
	var args []string

	args = this.State.Arguments()
	ow = this.State.OptionWord()
	f, e = this.State.OptionRunes()

	return fmt.Sprintf("missing value for option '%s' in '%s'",
		string([]rune(args[ow])[f:e]), args[ow])
}
