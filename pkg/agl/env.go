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
				t = types.GenericType{W: t, Name: typeParamName.Name, IsType: true}
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
		} else if t1, ok := result.(types.OptionType); ok {
			t1.Native = native
			result = t1
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
	return parseFuncTypeFromStringHelper(name, s, env, false)
}

func parseFuncTypeFromStringNative(name, s string, env *Env) types.FuncType {
	return parseFuncTypeFromStringHelper(name, s, env, true)
}

func parseFuncTypeFromStringHelper(name, s string, env *Env, native bool) types.FuncType {
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
	env.DefinePkg("os")
	env.DefinePkg("io")
	env.DefinePkg("bufio")
	env.DefinePkg("fmt")
	env.DefinePkg("http")
	env.DefinePkg("errors")
	env.DefinePkg("time")
	env.Define("error", types.TypeType{W: types.AnyType{}})
	env.Define("void", types.TypeType{W: types.VoidType{}})
	env.Define("any", types.TypeType{W: types.AnyType{}})
	env.Define("i8", types.TypeType{W: types.I8Type{}})
	env.Define("i16", types.TypeType{W: types.I16Type{}})
	env.Define("i32", types.TypeType{W: types.I32Type{}})
	env.Define("i64", types.TypeType{W: types.I64Type{}})
	env.Define("u8", types.TypeType{W: types.U8Type{}})
	env.Define("u16", types.TypeType{W: types.U16Type{}})
	env.Define("u32", types.TypeType{W: types.U32Type{}})
	env.Define("u64", types.TypeType{W: types.U64Type{}})
	env.Define("int", types.TypeType{W: types.IntType{}})
	env.Define("uint", types.TypeType{W: types.UintType{}})
	env.Define("f32", types.TypeType{W: types.F32Type{}})
	env.Define("f64", types.TypeType{W: types.F64Type{}})
	env.Define("string", types.TypeType{W: types.StringType{}})
	env.Define("bool", types.TypeType{W: types.BoolType{}})
	env.Define("true", types.BoolValue{V: true})
	env.Define("false", types.BoolValue{V: false})
	env.Define("byte", types.TypeType{W: types.ByteType{}})
	env.Define("cmp.Ordered", types.AnyType{})
	env.DefineFn("assert", "func (pred bool, msg ...string)")
	env.DefineFn("make", "func[T, U any](t T, size ...U) T")
	env.DefineFn("len", "func [T any](v T) int")
	env.DefineFn("cap", "func [T any](v T) int")
	env.DefineFn("min", "func [T cmp.Ordered](x T, y ...T) T")
	env.DefineFn("max", "func [T cmp.Ordered](x T, y ...T) T")
	env.DefineFn("clear", "func [T ~[]Type | ~map[Type]Type1](t T)")
	env.DefineFn("append", "func [T any](slice []T, elems ...T) []T")
	env.DefineFn("close", "func (c chan<- Type)")
	env.DefineFnNative("time.Sleep", "func (time.Duration)")
	env.Define("time.Duration", types.I64Type{})
	env.DefineFnNative("fmt.Println", "func (a ...any) int!")
	env.DefineFnNative("fmt.Sprintf", "func (format string, a ...any) string")
	env.DefineFnNative("fmt.Scan", "func (a ...any) int!")
	env.DefineFnNative("fmt.Scanf", "func (format string, a ...any) int!")
	env.DefineFnNative("fmt.Scanln", "func (a ...any) int!")
	env.DefineFnNative("fmt.Sprint", "func (a ...any) string")
	env.DefineFnNative("fmt.Sprintln", "func (a ...any) string")
	env.DefineFnNative("fmt.Sscan", "func (str string, a ...any) int!")
	env.DefineFnNative("fmt.Sscanf", "func (str string, format string, a ...any) int!")
	env.DefineFnNative("fmt.Sscanln", "func (str string, a ...any) int!")
	env.DefineFnNative("fmt.Fprint", "func (w io.Writer, a ...any) int!")
	env.DefineFnNative("fmt.Fprintf", "func (w io.Writer, format string, a ...any) int!")
	env.DefineFnNative("fmt.Fprintln", "func (w io.Writer, a ...any) int!")
	env.DefineFnNative("fmt.Fscan", "func (r io.Reader, a ...any) int!")
	env.DefineFnNative("fmt.Fscanf", "func (r io.Reader, format string, a ...any) int!")
	env.DefineFnNative("fmt.Fscanln", "func (r io.Reader, a ...any) int!")
	env.DefineFnNative("io.ReadAll", "func (r Reader) ([]byte)!")
	env.DefineFnNative("io.ReadFull", "func (r Reader, buf []byte) int!")
	env.DefineFnNative("io.WriteString", "func (w Writer, s string) int!")
	env.DefineFnNative("io.CopyBuffer", "func (dst Writer, src Reader, buf []byte) int64!")
	env.DefineFnNative("io.CopyN", "func (dst Writer, src Reader, n int64) int64!")
	env.DefineFnNative("io.Copy", "func (dst Writer, src Reader) int64!")
	env.DefineFnNative("io.Pipe", "func () (*PipeReader, *PipeWriter)")
	env.DefineFnNative("bufio.ScanBytes", "func (data []byte, atEOF bool) (int, []byte)!")
	env.DefineFnNative("http.Get", "func (url string) (*http.Response)!")
	env.DefineFnNative("os.ReadFile", "func (name string) ([]byte)!")
	env.DefineFnNative("os.WriteFile", "func (name string, data []byte, perm os.FileMode) !")
	env.DefineFnNative("os.Chdir", "func (string) !")
	env.DefineFnNative("os.Chown", "func (name string, uid, gid int) !")
	env.DefineFnNative("os.Mkdir", "func (name string, perm FileMode) !")
	env.DefineFnNative("os.MkdirAll", "func (path string, perm FileMode) !")
	env.DefineFnNative("os.MkdirTemp", "func (dir, pattern string) string!")
	env.DefineFnNative("os.Remove", "func (name string) !")
	env.DefineFnNative("os.RemoveAll", "func (path string) !")
	env.DefineFnNative("os.Rename", "func (oldpath, newpath string) !")
	env.DefineFnNative("os.SameFile", "func (fi1, fi2 FileInfo) bool")
	env.DefineFnNative("os.TempDir", "func () string")
	env.DefineFnNative("os.Truncate", "func (name string, size int64) !")
	env.DefineFnNative("os.Setenv", "func (key, value string) !")
	env.DefineFnNative("os.Unsetenv", "func (key string) !")
	env.DefineFnNative("os.UserCacheDir", "func () string!")
	env.DefineFnNative("os.UserConfigDir", "func () string!")
	env.DefineFnNative("os.UserHomeDir", "func () string!")
	env.DefineFnNative("os.Getwd", "func () string!")
	env.DefineFnNative("os.Hostname", "func () string!")
	env.DefineFnNative("os.IsExist", "func (err error) bool")
	env.DefineFnNative("os.IsNotExist", "func (err error) bool")
	env.DefineFnNative("os.IsPathSeparator", "func (c uint8) bool")
	env.DefineFnNative("os.IsPermission", "func (err error) bool")
	env.DefineFnNative("os.IsTimeout", "func (err error) bool")
	env.DefineFnNative("os.LookupEnv", "func (key string) string?")
	env.DefineFnNative("strconv.Itoa", "func(int) string")
	env.DefineFnNative("strconv.Atoi", "func(string) int!")
	env.DefineFn("errors.New", "func (text string) error")
	env.Define("agl.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	env.DefineFn("agl.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T")
	env.DefineFn("agl.Vec.Map", "func [T, R any](a []T, f func(T) R) []R")
	env.DefineFn("agl.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R")
	env.DefineFn("agl.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
	env.DefineFn("agl.Vec.Sum", "func [T cmp.Ordered](a []T) T")
	env.DefineFn("agl.Vec.Joined", "func (a []string) string")
	env.DefineFn("agl.Option.UnwrapOr", "func [T any](T) T")
	env.DefineFn("agl.Option.IsSome", "func () bool")
	env.DefineFn("agl.Option.IsNone", "func () bool")
	env.DefineFn("agl.Option.Unwrap", "func [T any]() T")
	env.DefineFn("agl.Result.UnwrapOr", "func [T any](T) T")
	env.DefineFn("agl.Result.IsOk", "func () bool")
	env.DefineFn("agl.Result.IsErr", "func () bool")
	env.DefineFn("agl.Result.Unwrap", "func [T any]() T")
	return env
}

func (e *Env) Clone() *Env { // TODO avoid cloning, use recursive child/parent pattern instead
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

func (e *Env) DefineFn(name string, fnStr string) {
	fnT := parseFuncTypeFromString(name, fnStr, e)
	e.Define(name, fnT)
}

func (e *Env) DefineFnNative(name string, fnStr string) {
	fnT := parseFuncTypeFromStringNative(name, fnStr, e)
	e.Define(name, fnT)
}

func (e *Env) DefinePkg(name string) {
	e.Define(name, types.PackageType{Name: name})
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
		if tt, ok := v.(types.TypeType); ok {
			v = tt.W
		}
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
		return e.GetType2(xx.X)
	case *ast.ParenExpr:
		return e.GetType2(xx.X)
	case *ast.VoidExpr:
		return types.VoidType{}
	case *ast.StarExpr:
		return types.StarType{X: e.GetType2(xx.X)}
	case *ast.MapType:
		return types.MapType{K: e.GetType2(xx.Key), V: e.GetType2(xx.Value)}
	case *ast.ChanType:
		return types.ChanType{}
	case *ast.TupleExpr:
		var elts []types.Type
		for _, v := range xx.Values { // TODO NO GOOD
			elt := e.GetType2(v)
			elts = append(elts, elt)
		}
		return types.TupleType{Name: "", Elts: elts}
	case *ast.BinaryExpr:
		return types.BinaryType{X: e.GetType2(xx.X), Y: e.GetType2(xx.Y)}
	case *ast.UnaryExpr:
		return types.UnaryType{X: e.GetType2(xx.X)}
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
