package agl

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func Test1(t *testing.T) {
	env := NewEnv(nil)
	tt := env.Get("http.NewRequest")
	tassert.Equal(t, "func NewRequest(string, string, io.Reader) Result[*Request]", tt.String())
}

func Test2(t *testing.T) {
	env := NewEnv(nil)
	tt := env.Get("fmt.Println")
	tassert.Equal(t, "func Println(...any) Result[int]", tt.String())
}
