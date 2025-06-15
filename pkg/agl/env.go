package agl

import (
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	"maps"
	"reflect"
	"sync/atomic"
)

type Env struct {
	fset          *token.FileSet
	structCounter atomic.Int64
	lookupTable   map[string]types.Type // Store constants/variables/functions
	lookupTable2  map[string]types.Type // Store type for Expr/Stmt
}

func funcTypeToFuncType(name string, expr *ast.FuncType, env *Env, native bool) types.FuncType {
	var paramsT []types.Type
	if expr.TypeParams != nil {
		for _, typeParam := range expr.TypeParams.List {
			for _, typeParamName := range typeParam.Names {
				typeParamType := typeParam.Type
				t := env.GetType2(typeParamType)
				t = types.GenericType{W: t, Name: typeParamName.Name}
				env.Define(typeParamName.Name, t)
				paramsT = append(paramsT, t)
			}
		}
	}
	var params []types.Type
	if expr.Params != nil {
		for _, param := range expr.Params.List {
			t := env.GetType2(param.Type)
			n := max(len(param.Names), 1)
			for i := 0; i < n; i++ {
				params = append(params, t)
			}
		}
	}
	var result types.Type
	if expr.Result != nil {
		result = env.GetType2(expr.Result)
		if t, ok := result.(types.ResultType); ok {
			t.Native = native
			result = t
		}
	}
	ft := types.FuncType{
		Name:       name,
		TypeParams: paramsT,
		Params:     params,
		Return:     result,
		IsNative:   native,
	}
	return ft
}

func parseFuncTypeFromString(name, s string, env *Env) types.FuncType {
	return parseFuncTypeFromString1(name, s, env, false)
}

func parseFuncTypeFromStringNative(name, s string, env *Env) types.FuncType {
	return parseFuncTypeFromString1(name, s, env, true)
}

func parseFuncTypeFromString1(name, s string, env *Env, native bool) types.FuncType {
	env = env.Clone()
	e, err := parser.ParseExpr(s)
	if err != nil {
		panic(err)
	}
	expr := e.(*ast.FuncType)
	return funcTypeToFuncType(name, expr, env, native)
}

func NewEnv(fset *token.FileSet) *Env {
	env := &Env{fset: fset, lookupTable2: make(map[string]types.Type), lookupTable: make(map[string]types.Type)}
	env.Define("os", types.PackageType{Name: "os"})
	env.Define("fmt", types.PackageType{Name: "fmt"})
	env.Define("any", types.AnyType{})
	env.Define("i8", types.I8Type{})
	env.Define("i16", types.I16Type{})
	env.Define("i32", types.I32Type{})
	env.Define("i64", types.I64Type{})
	env.Define("u8", types.U8Type{})
	env.Define("u16", types.U16Type{})
	env.Define("u32", types.U32Type{})
	env.Define("u64", types.U64Type{})
	env.Define("int", types.IntType{})
	env.Define("uint", types.UintType{})
	env.Define("f32", types.F32Type{})
	env.Define("f64", types.F64Type{})
	env.Define("string", types.StringType{})
	env.Define("bool", types.BoolType{})
	env.Define("true", types.BoolValue{V: true})
	env.Define("false", types.BoolValue{V: false})
	env.Define("byte", types.ByteType{})
	env.Define("cmp.Ordered", types.AnyType{})
	env.Define("assert", parseFuncTypeFromString("assert", "func (pred bool, msg ...string)", env))
	env.Define("make", parseFuncTypeFromString("make", "func[T, U any](t T, size ...U) T", env))
	env.Define("len", parseFuncTypeFromString("len", "func [T any](v T) int", env))
	env.Define("cap", parseFuncTypeFromString("cap", "func [T any](v T) int", env))
	env.Define("min", parseFuncTypeFromString("min", "func [T cmp.Ordered](x T, y ...T) T", env))
	env.Define("max", parseFuncTypeFromString("max", "func [T cmp.Ordered](x T, y ...T) T", env))
	//env.Define("clear", parseFuncTypeFromString("clear", "func [T ~[]Type | ~map[Type]Type1](t T)", env))
	env.Define("close", parseFuncTypeFromString("close", "func (c chan<- Type)", env))
	env.Define("fmt.Println", parseFuncTypeFromStringNative("Println", "func(a ...any) int!", env))
	env.Define("fmt.Sprintf", parseFuncTypeFromStringNative("Sprintf", "func(format string, a ...any) string", env))
	env.Define("os.ReadFile", parseFuncTypeFromStringNative("ReadFile", "func(name string) ([]byte)!", env))
	env.Define("os.WriteFile", parseFuncTypeFromStringNative("WriteFile", "func(name string, data []byte, perm os.FileMode) !", env))
	env.Define("strconv.Itoa", parseFuncTypeFromString("Itoa", "func(int) string", env))
	env.Define("agl.Vec.Filter", parseFuncTypeFromString("Filter", "func [T any](a []T, f func(e T) bool) []T", env))
	env.Define("agl.Vec.Map", parseFuncTypeFromString("Map", "func [T, R any](a []T, f func(T) R) []R", env))
	env.Define("agl.Vec.Reduce", parseFuncTypeFromString("Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R", env))
	env.Define("agl.Vec.Find", parseFuncTypeFromString("Find", "func [T any](a []T, f func(e T) bool) T?", env))
	env.Define("agl.Vec.Sum", parseFuncTypeFromString("Sum", "func [T cmp.Ordered](a []T) T", env))
	env.Define("agl.Vec.Joined", parseFuncTypeFromString("Joined", "func (a []string) string", env))
	return env
}

func (e *Env) Clone() *Env {
	env := &Env{
		fset:         e.fset,
		lookupTable:  maps.Clone(e.lookupTable),
		lookupTable2: e.lookupTable2,
	}
	return env
}

func (e *Env) CloneFull() *Env {
	env := &Env{
		fset:         e.fset,
		lookupTable:  maps.Clone(e.lookupTable),
		lookupTable2: maps.Clone(e.lookupTable2),
	}
	return env
}

func (e *Env) Get(name string) types.Type {
	return e.lookupTable[name]
}

func (e *Env) GetFn(name string) types.FuncType {
	return e.Get(name).(types.FuncType)
}

func (e *Env) Define(name string, typ types.Type) {
	//p("Define", name, typ)
	//printCallers(3)
	assertf(e.Get(name) == nil, "duplicate declaration of %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) Assign(name string, typ types.Type) {
	if name == "_" {
		return
	}
	assertf(e.Get(name) != nil, "undeclared %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) SetType(x ast.Node, t types.Type) {
	assertf(t != nil, "%s: try to set type nil, %v %v", e.fset.Position(x.Pos()), x, to(x))
	e.lookupTable2[makeKey(x)] = t
}

func makeKey(n ast.Node) string {
	return fmt.Sprintf("%d_%d", n.Pos(), n.End())
}

func (e *Env) GetType(x ast.Node) types.Type {
	if v, ok := e.lookupTable2[makeKey(x)]; ok {
		return v
	}
	return nil
}

func (e *Env) GetType2(x ast.Node) types.Type {
	if v, ok := e.lookupTable2[makeKey(x)]; ok {
		return v
	}
	switch xx := x.(type) {
	case *ast.Ident:
		if v2, ok := e.lookupTable[xx.Name]; ok {
			return v2
		}
		return nil
	case *ast.FuncType:
		return funcTypeToFuncType("", xx, e, false)
	case *ast.Ellipsis:
		return types.EllipsisType{Elt: e.GetType2(xx.Elt)}
	case *ast.ArrayType:
		return types.ArrayType{Elt: e.GetType2(xx.Elt)}
	case *ast.ResultExpr:
		return types.ResultType{W: e.GetType2(xx.X)}
	case *ast.OptionExpr:
		return types.OptionType{W: e.GetType2(xx.X)}
	case *ast.CallExpr:
		return nil
	case *ast.BasicLit:
		switch xx.Kind {
		case token.INT:
			return types.UntypedNumType{}
		default:
			panic("")
		}
	case *ast.SelectorExpr:
		return e.GetType2(&ast.Ident{Name: fmt.Sprintf("%s.%s", xx.X.(*ast.Ident).Name, xx.Sel.Name)})
	case *ast.IndexExpr:
		return nil
	case *ast.ParenExpr:
		return e.GetType2(xx.X)
	case *ast.VoidExpr:
		return types.VoidType{}
	case *ast.StarExpr:
		return types.StarType{}
	case *ast.MapType:
		return types.MapType{}
	case *ast.ChanType:
		return types.ChanType{}
	case *ast.TupleExpr:
		var elts []types.Type
		for _, v := range xx.Values { // TODO NO GOOD
			elt := e.GetType2(v)
			elts = append(elts, elt)
		}
		return types.TupleType{Name: "", Elts: elts}
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
