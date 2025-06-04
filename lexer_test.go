package main

import (
	"fmt"
	tassert "github.com/stretchr/testify/assert"
	"testing"
)

func TestLexer(t *testing.T) {
	src := `
package main
import "fmt"
fn main() {
	fmt.Println("Hello world")
}
`
	tokens := lexer(src)
	fmt.Println(tokens)
}

func TestLexer1(t *testing.T) {
	src := `
fn main() {
	a := 0
	a += 1
	a -= 1
	a *= 1
	a /= 1
	a %= 1
	a &= 1
	a |= 1
	a ^= 1
	a <<= 1
	a >>= 1
	a &^= 1
	a++
	a--
}
`
	tokens := lexer(src)
	fmt.Println(tokens)
}

func TestLexer2(t *testing.T) {
	src := `fn find[T comparable](arr []T, val T) T?`
	tokens := lexer(src)
	fmt.Println(tokens)
}

func TestLexer_TokenStream(t *testing.T) {
	ts := NewTokenStream(`fn main() {}`)
	tassert.Equal(t, FN, ts.Next().typ)
	tassert.Equal(t, IDENT, ts.Peek().typ)
	tassert.Equal(t, IDENT, ts.Next().typ)
	tassert.Equal(t, LPAREN, ts.Next().typ)
	tassert.Equal(t, RPAREN, ts.Next().typ)
	tassert.Equal(t, LBRACE, ts.Next().typ)
	tassert.Equal(t, RBRACE, ts.Peek().typ)
	tassert.Equal(t, RBRACE, ts.Next().typ)
	tassert.Equal(t, EOF, ts.Peek().typ)
	tassert.Equal(t, EOF, ts.Next().typ)
	tassert.Equal(t, EOF, ts.Next().typ)
}
