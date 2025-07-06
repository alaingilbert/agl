package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func findNodeAtPosition(file *ast.File, fset *token.FileSet, pos token.Position) ast.Node {
	var result ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		nodePos := fset.Position(n.Pos())
		nodeEnd := fset.Position(n.End())

		// Check if the position is within this node's range
		if nodePos.Offset <= pos.Offset && pos.Offset <= nodeEnd.Offset {
			// If this is a more specific node (smaller range), use it
			if result == nil ||
				(fset.Position(result.Pos()).Offset <= nodePos.Offset &&
					nodeEnd.Offset <= fset.Position(result.End()).Offset) {
				result = n
			}
			return true // Continue searching for more specific nodes
		}
		return false
	})
	return result
}

type Test struct {
	f    *ast.File
	fset *token.FileSet
	env  *Env
	file *token.File
	errs []error
}

func (t *Test) PrintErrors() {
	for _, err := range t.errs {
		fmt.Println(err)
	}
}

func NewTest(src string) *Test {
	fset, f := ParseSrc(src)
	env := NewEnv()
	i := NewInferrer(env)
	errs := i.InferFile("", f, fset)
	file := fset.File(1)
	return &Test{
		f:    f,
		fset: fset,
		env:  env,
		file: file,
		errs: errs,
	}
}

func (t *Test) GenCode() string {
	return NewGenerator(t.env, t.f, t.fset).Generate()
}

func (t *Test) TypeAt(row, col int) types.Type {
	offset := t.file.LineStart(row) + token.Pos(col-1)
	n := findNodeAtPosition(t.f, t.fset, t.fset.Position(offset))
	return t.env.GetType(n)
}

func TestInfer1(t *testing.T) {
	src := `package main
import "net/http"
func main() {
	r := http.Get("")!
	bod := r.Body
	bod.Close()
}
`
	tassert.Equal(t, "ReadCloser", NewTest(src).TypeAt(5, 2).(types.InterfaceType).Name)
}

func TestInfer2(t *testing.T) {
	src := `package main
import "net/http"
func main() {
	req := http.NewRequest(http.MethodGet, "https://jsonip.com", nil)!
}
`
	NewTest(src)
	//tassert.Equal(t, 1, env.Get("bod"))
}

func TestInfer3(t *testing.T) {
	src := `package main
import "net/http"
func main() {
	req := http.NewRequest(http.MethodGet, "https://jsonip.com", nil)!
}
`
	tt := NewTest(src)
	ast.Inspect(tt.f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		//fmt.Println(reflect.TypeOf(n), n, env.GetType(n))
		return true
	})
	//tassert.Equal(t, 1, env.Get("bod"))
}

func TestInfer4(t *testing.T) {
	src := `package main
func test() int! {
	return Err("error")
}
func main() {
	match test() {
	case Ok(res):
	case Err(err):
	}
}
`
	test := NewTest(src)
	tassert.Equal(t, "int", test.TypeAt(7, 11).String())
	tassert.Equal(t, "error", test.TypeAt(8, 12).String())
}

func TestInfer5(t *testing.T) {
	src := `package main
func main() {
	s1 := set[int]{1, 2}
	s2 := set[int]{2, 3}
	s3 := s1.IsDisjoint(s2)
}
`
	test := NewTest(src)
	tassert.Equal(t, "func (set[int]) IsDisjoint(agl.Iterator[int]) bool", test.TypeAt(5, 12).String())
}

func TestInfer6(t *testing.T) {
	src := `package main
func test(update []int) int { 42 }
func main() {
	updates := [][]int{}
    updates.Map(test).Sum()
}
`
	test := NewTest(src)
	tassert.Equal(t, "func ([][]int) Map(func([]int) int) []int", test.TypeAt(5, 14).String())
}

func TestInfer7(t *testing.T) {
	src := `package main
func main() {
	a := make([]int, 0)
	b := make(map[int]int)
	c := make(set[int])
}
`
	test := NewTest(src)
	tassert.Equal(t, "[]int", test.TypeAt(3, 2).String())
	tassert.Equal(t, "map[int]int", test.TypeAt(4, 2).String())
	tassert.Equal(t, "set[int]", test.TypeAt(5, 2).String())
	//tassert.Equal(t, "func make([]int, ...IntegerType) []int", test.TypeAt(3, 7).String())
	//tassert.Equal(t, "func make(map[int]int, ...IntegerType) map[int]int", test.TypeAt(4, 7).String())
	//tassert.Equal(t, "func make(set[int], ...IntegerType) set[int]", test.TypeAt(5, 7).String())
}

func TestInfer8(t *testing.T) {
	src := `package main
func main() {
	var a int = 42
	b := min(a, 1)
	c := min(a, 1, 2)
}
`
	test := NewTest(src)
	tassert.Equal(t, "func min(int, int) int", test.TypeAt(4, 7).String())
	tassert.Equal(t, "func min(int, int, int) int", test.TypeAt(5, 7).String())
}

func TestInfer9(t *testing.T) {
	src := `package main
func main() {
	a := [][]int{{1, 2}, {2, 3}}
}
`
	test := NewTest(src)
	tassert.Equal(t, "[][]int", test.TypeAt(3, 2).String())
}

func TestInfer10(t *testing.T) {
	src := `package main
func main() {
	mut m := map[int]u8{}
	m[1] = 2
}
`
	test := NewTest(src)
	tassert.Equal(t, "mut map[int]u8", test.TypeAt(3, 6).String())
	tassert.Equal(t, "mut map[int]u8", test.TypeAt(4, 2).String())
	tassert.Equal(t, "int", test.TypeAt(4, 4).String())
	tassert.Equal(t, "u8", test.TypeAt(4, 6).String())
}

func TestInfer11(t *testing.T) {
	src := `package main
func main() {
	k1 := u8(1)
	mut arr := []int{1, 2}
	arr[k1] = 2
	arr[1] = 2
}
`
	test := NewTest(src)
	tassert.Equal(t, "mut []int", test.TypeAt(5, 2).String())
	tassert.Equal(t, "u8", test.TypeAt(5, 6).String())
	tassert.Nil(t, test.TypeAt(6, 6))
}

func TestInfer12(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := Vertex{3, 4}
	v.Scale(10)
}
`
	test := NewTest(src)
	tassert.Equal(t, "func (mut *Vertex) Scale(f64)", test.TypeAt(11, 4).String())
}

func TestInfer13(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := &Vertex{3, 4}
	v.Scale(10)
}
`
	test := NewTest(src)
	tassert.Equal(t, "mut *Vertex", test.TypeAt(11, 2).String())
}

func TestInfer14(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) int {
	v.X = v.X * f
	v.Y = v.Y * f
    return 1
}
func main() {
	mut v := &Vertex{3, 4}
	mut vv := v
	vv.Scale(10)
}
`
	test := NewTest(src)
	tassert.Equal(t, "mut *Vertex", test.TypeAt(12, 2).String())
	tassert.Equal(t, "mut *Vertex", test.TypeAt(12, 6).String())
}

func TestInfer15(t *testing.T) {
	src := `package main
import "fmt"
import "strings"
func main() {
	var mut sb strings.Builder
	sb.WriteString("hello world")
	fmt.Println(sb.String())
}`
	test := NewTest(src)
	tassert.Equal(t, "func (mut *strings.Builder) WriteString(string) int!", test.TypeAt(6, 5).String())
}

func TestInfer16(t *testing.T) {
	src := `package main
var mut m map[int]set[int]
func main() {
	m = make(map[int]set[int])
	s := m[1]
}`
	test := NewTest(src)
	tassert.Equal(t, "map[int]set[int]", test.TypeAt(4, 2).String())
	tassert.Equal(t, "set[int]", test.TypeAt(5, 2).String())
}

func TestInfer17(t *testing.T) {
	src := `package main
func main() {
	mut s := set[int]{1, 2}
	s.Insert(3)
	s.Contains(3)
}`
	test := NewTest(src)
	tassert.Equal(t, "func (mut set[int]) Insert(int) bool", test.TypeAt(4, 4).String())
	tassert.Equal(t, "func (set[int]) Contains(int) bool", test.TypeAt(5, 4).String())
}

func TestInfer18(t *testing.T) {
	src := `package main
func main() {
	m := map[int]int{1: 1}
	r1 := m.Get(1).Unwrap()
	r2 := m.Get(1).UnwrapOr(42)
	r3 := m.Get(1).UnwrapOrDefault()
}`
	test := NewTest(src)
	tassert.Equal(t, "int", test.TypeAt(4, 2).String())
	tassert.Equal(t, "func (int?) Unwrap() int", test.TypeAt(4, 17).String())
	tassert.Equal(t, "func (int?) UnwrapOr(int) int", test.TypeAt(5, 17).String())
	tassert.Equal(t, "func (int?) UnwrapOrDefault() int", test.TypeAt(6, 17).String())
}

func TestInfer19(t *testing.T) {
	src := `package main
func main() {
	m := map[int]set[int]{1: set[int]{2}}
	if Some(s) := m.Get(1) {
	}
}`
	test := NewTest(src)
	tassert.Equal(t, "set[int]", test.TypeAt(4, 10).String())
}

func TestInfer20(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	arr.
	arr[1] = 2
}`
	test := NewTest(src)
	tassert.Equal(t, "[]int", test.TypeAt(4, 2).String())
}

func TestInfer21(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	arr.
	b := make(map[int]int)
}`
	test := NewTest(src)
	tassert.Equal(t, "[]int", test.TypeAt(4, 2).String())
}

func TestInfer22(t *testing.T) {
	src := `package main
func main() {
	var b map[int]int
	arr := []int{1, 2, 3}
	arr.
	b = make(map[int]int)
}`
	test := NewTest(src)
	tassert.Equal(t, "[]int", test.TypeAt(5, 5).String())
}

//func TestInfer1(t *testing.T) {
//	src := `
//fn fn1(a, b int) int { return a + b }
//fn fn2(a, b i64) i64 { return a + b }
//fn fn3(a, b i32) i32 { return a + b }
//fn fn4(a, b i16) i16 { return a + b }
//fn fn5(a, b i8) i8 { return a + b }
//fn fn6(a, b f64) f64 { return a + b }
//fn fn7(a, b f32) f32 { return a + b }
//fn fn8(a, b i64) i64? { return Some(a + b) }
//fn fn9(a, b string) string { return a + b }
//fn fn10(a, b i64) i64! { return Ok(a + b) }
//`
//	i, _ := infer(parser(NewTokenStream(src)))
//	if _, ok := i.funcs[0].out.expr.GetType().(IntType); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[0].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[1].out.expr.GetType().(I64Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[1].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[2].out.expr.GetType().(I32Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[2].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[3].out.expr.GetType().(I16Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[3].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[4].out.expr.GetType().(I8Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[4].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[5].out.expr.GetType().(F64Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[5].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[6].out.expr.GetType().(F32Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[6].out.expr.GetType()))
//	}
//	if tt, ok := i.funcs[7].out.expr.GetType().(OptionType); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[7].out.expr.GetType()))
//	} else if _, ok := tt.wrappedType.(I64Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[7].out.expr.GetType()))
//	}
//	if _, ok := i.funcs[8].out.expr.GetType().(StringType); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[8].out.expr.GetType()))
//	}
//	if tt, ok := i.funcs[9].out.expr.GetType().(ResultType); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[9].out.expr.GetType()))
//	} else if _, ok := tt.wrappedType.(I64Type); !ok {
//		t.Fatalf("Infer1(): unexpected type %v", reflect.TypeOf(i.funcs[9].out.expr.GetType()))
//	}
//}
//
//func TestParseFuncTypeFromString(t *testing.T) {
//	f := parseFuncTypeFromString("fn (s string) int! {}", NewEnv())
//	tassert.Equal(t, "func(string) Result[int]", f.GoStr())
//}
//
//func TestParseFuncTypeFromString1(t *testing.T) {
//	f := parseFuncTypeFromString("fn filter[T any](a []T, f fn(e T) bool) []T", NewEnv())
//	tassert.Equal(t, "func [T any] filter([]T, func(T) bool) []T", f.GoStr())
//}

func TestInferOsArgs(t *testing.T) {
	src := `package main
import "os"
func main() {
	args := os.Args
	firstArg := os.Args[0]
	argCount := len(os.Args)
	_ = args
	_ = firstArg
	_ = argCount
}
`
	test := NewTest(src)
	tassert.Equal(t, "[]string", test.TypeAt(4, 14).String())
	tassert.Equal(t, "string", test.TypeAt(5, 2).String())
	tassert.Equal(t, "int", test.TypeAt(6, 2).String())
}
