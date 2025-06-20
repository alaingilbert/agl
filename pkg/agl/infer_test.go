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

func TestInfer1(t *testing.T) {
	src := `package main
func main() {
	r := http.Get("")!
	bod := r.Body
	bod.Close()
}
`
	fset, f := parser2(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)
	file := fset.File(1)
	offset := file.LineStart(4) + token.Pos(2-1)
	n := findNodeAtPosition(f, fset, fset.Position(offset))
	tassert.Equal(t, "ReadCloser", env.GetType(n).(types.InterfaceType).Name)
}

func TestInfer2(t *testing.T) {
	src := `package main
func main() {
	req := http.NewRequest(http.MethodGet, "https://jsonip.com", nil)!
}
`
	fset, f := parser2(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)
	//tassert.Equal(t, 1, env.Get("bod"))
}

func TestInfer3(t *testing.T) {
	src := `package main
func main() {
	req := http.NewRequest(http.MethodGet, "https://jsonip.com", nil)!
}
`
	fset, f := parser2(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)

	ast.Inspect(f, func(n ast.Node) bool {
		if n == nil {
			return true
		}
		//fmt.Println(reflect.TypeOf(n), n, env.GetType(n))
		return true
	})
	//tassert.Equal(t, 1, env.Get("bod"))
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
