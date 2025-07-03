package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
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
}

func NewTest(src string) *Test {
	fset, f := ParseSrc(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)
	file := fset.File(1)
	return &Test{
		f:    f,
		fset: fset,
		env:  env,
		file: file,
	}
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
	tassert.Equal(t, "func (set[int]) IsDisjoint(set[int]) bool", test.TypeAt(5, 12).String())
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
	fset, f := ParseSrc(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)

	// Find the main function
	var mainFunc *ast.FuncDecl
	for _, decl := range f.Decls {
		if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == "main" {
			mainFunc = fd
			break
		}
	}

	if mainFunc == nil {
		t.Fatal("Could not find main function")
	}

	// Check the types of the variables
	stmts := mainFunc.Body.List

	// args := os.Args should be []string
	if assign, ok := stmts[0].(*ast.AssignStmt); ok {
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			argType := env.GetType(ident)
			if arrType, ok := argType.(types.ArrayType); !ok {
				t.Fatalf("Expected os.Args to be array type, got %T", argType)
			} else if arrType.GoStr() != "[]string" {
				t.Fatalf("Expected os.Args to be []string, got %s", arrType.GoStr())
			}
		}
	}

	// firstArg := os.Args[0] should be string
	if assign, ok := stmts[1].(*ast.AssignStmt); ok {
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			argType := env.GetType(ident)
			if argType.GoStr() != "string" {
				t.Fatalf("Expected os.Args[0] to be string, got %s", argType.GoStr())
			}
		}
	}

	// argCount := len(os.Args) should be int
	if assign, ok := stmts[2].(*ast.AssignStmt); ok {
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			argType := env.GetType(ident)
			if argType.GoStr() != "int" {
				t.Fatalf("Expected len(os.Args) to be int, got %s", argType.GoStr())
			}
		}
	}
}
