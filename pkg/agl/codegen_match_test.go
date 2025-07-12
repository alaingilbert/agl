package agl

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestCodeGenMatch_1(t *testing.T) {
	src := `package main
type Color enum {
	Red
	Green
	Blue
}
func main() {
	c := Color.Red
	match c {
	case .Red:
	case .Green:
	case .Blue:
	}
}`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
}

func TestCodeGenMatch_2(t *testing.T) {
	src := `package main
type Color enum {
	Red
	Green
	Blue
}
func main() {
	c := Color.Red
	match c {
	case .Red:
	case .Blue:
	}
}`
	test := NewTest(src, WithMutEnforced(false))
	test.PrintErrors()
	tassert.Contains(t, NewTest(src).errs[0].Error(), "9:2: match expression is not exhaustive")
}

func TestCodeGenMatch_3(t *testing.T) {
	src := `package main
type Color enum {
	Red
	Green
	Blue
}
func main() {
	c := Color.Red
	match c {
	case .Red:
	case .Blue:
	default:
	}
}`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
}
