package agl

import (
	"agl/pkg/token"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func Test1(t *testing.T) {
	env := NewEnv()
	_ = env.loadPkgAglStd("agl1/net/http", "")
	tt := env.Get("http.NewRequest")
	tassert.Equal(t, "func NewRequest(string, string, (io.Reader)?) (*http.Request)!", tt.String())
}

func Test2(t *testing.T) {
	env := NewEnv()
	_ = env.loadPkgAglStd("agl1/fmt", "")
	tt := env.Get("fmt.Println")
	tassert.Equal(t, "func Println(...any) int!", tt.String())
}

func Test3(t *testing.T) {
	env := NewEnv()
	_ = env.loadPkgAglStd("agl1/net/http", "")
	tt := env.Get("http.Get")
	tassert.Equal(t, "func Get(string) (*http.Response)!", tt.String())
}

func Test4(t *testing.T) {
	env := NewEnv()
	_ = env.loadPkgAglStd("agl1/io", "")
	tt := env.Get("io.ReadCloser.Close")
	tassert.Equal(t, "func Close() !", tt.String())
}

func Test5(t *testing.T) {
	env := NewEnv()
	_ = env.loadPkgAglStd("agl1/strings", "")
	fT := parseFuncDeclFromStringHelper("WriteString", "func (mut r *strings.Builder) WriteString(io.Writer, string) int!", env, token.NewFileSet())
	tassert.Equal(t, "func (mut *strings.Builder) WriteString(io.Writer, string) int!", fT.String())
}

//func Test5(t *testing.T) {
//	env := NewEnv(nil)
//	tt := parseFuncTypeFromString("Test", "func (a (int, bool)) (int, bool)", env)
//	tassert.Equal(t, "func Test(a AglTupleStruct_int_bool) AglTupleStruct_int_bool", tt.GoStr())
//}
