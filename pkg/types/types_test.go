package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReplGen(t *testing.T) {
	t1 := ArrayType{Elt: GenericType{Name: "T", W: AnyType{}}}
	expected := ArrayType{Elt: StringType{}}
	nt := ReplGen(t1, "T", StringType{})
	assert.Equal(t, expected, nt)
}
