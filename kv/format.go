package kv


import (
	"fmt"
	"strings"
)


// ----------------------------------------------------------------------------


func Format(view View, str string) (string, error) {
	var expr Expression
	var err error

	expr, err = ParseExpression(str)
	if err != nil {
		return "", err
	}

	return expr.Format(view)
}


type Expression interface {
	Format(View) (string, error)

	Tokens() []ExpressionToken
}

func ParseExpression(fmt string) (Expression, error) {
	return parseExpression(fmt)
}


type ExpressionToken interface {
	Format(View) (string, error)
	Text() string
}

type ExpressionPlainToken interface {
	ExpressionToken
	Plain() string
}

type ExpressionKeyToken interface {
	ExpressionToken
	Key() Key
}


type InvalidFormatError struct {
	Fmt string
	Pos int
}

type UnknownKeyError struct {
	Key Key
}


// ----------------------------------------------------------------------------


type expression struct {
	tokens []ExpressionToken
}

func parseExpression(str string) (*expression, error) {
	var percent, keyfmt bool
	var text strings.Builder
	var this expression
	var err error
	var pos int
	var key Key
	var r rune

	this.tokens = make([]ExpressionToken, 0)

	percent = false
	keyfmt = false

	for pos, r = range str {
		if percent {
			if r == '%' {
				text.WriteRune(r)
				percent = false
				continue
			} else if r == '{' {
				if text.Len() > 0 {
					this.tokens = append(this.tokens,
						newExpressionPlainToken(
							text.String()))
					text.Reset()
				}

				percent = false
				keyfmt = true
				continue
			} else {
				text.WriteRune('%')
				text.WriteRune(r)
				percent = false
				continue
			}
		}

		if keyfmt && (r == '}') {
			key, err = NewKey(text.String())
			if err != nil {
				return nil, err
			}

			this.tokens = append(this.tokens,
				newExpressionKeyToken(key, text.String()))

			text.Reset()
			keyfmt = false
			continue
		}

		if r == '%' {
			percent = true
			continue
		}

		text.WriteRune(r)
	}

	if percent || keyfmt {
		return nil, &InvalidFormatError{ str, pos }
	}

	if text.Len() > 0 {
		this.tokens = append(this.tokens,
			newExpressionPlainToken(text.String()))
	}

	return &this, nil
}

func (this *expression) Format(view View) (string, error) {
	var token ExpressionToken
	var ret strings.Builder
	var err error
	var str string

	for _, token = range this.tokens {
		str, err = token.Format(view)
		if err != nil {
			return "", err
		}

		ret.WriteString(str)
	}

	return ret.String(), nil
}

func (this *expression) Tokens() []ExpressionToken {
	return this.tokens
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type expressionPlainToken struct {
	text string
}

func newExpressionPlainToken(text string) *expressionPlainToken {
	var this expressionPlainToken

	this.text = text

	return &this
}

func (this *expressionPlainToken) Format(View) (string, error) {
	return this.text, nil
}

func (this *expressionPlainToken) Text() string {
	return this.text
}

func (this *expressionPlainToken) Plain() string {
	return this.text
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -


type expressionKeyToken struct {
	key Key
	text string
}

func newExpressionKeyToken(key Key, text string) *expressionKeyToken {
	var this expressionKeyToken

	this.key = key
	this.text = text

	return &this
}

func (this *expressionKeyToken) Format(view View) (string, error) {
	var val Value = view.Get(this.key)

	if val == NoValue {
		return "", &UnknownKeyError{ this.key }
	}

	return val.String(), nil
}

func (this *expressionKeyToken) Text() string {
	return this.text
}

func (this *expressionKeyToken) Key() Key {
	return this.key
}


//  - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - -

func (this *InvalidFormatError) Error() string {
	return fmt.Sprintf("invalid format: %s", this.Fmt)
}

func (this *UnknownKeyError) Error() string{
	return fmt.Sprintf("unknown key: %s", this.Key)
}
