package agl

import (
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
)

type Env struct {
	fset          *token.FileSet
	structCounter atomic.Int64
	lookupTable   map[string]*Info  // Store constants/variables/functions
	lspTable      map[NodeKey]*Info // Store type for Expr/Stmt
	parent        *Env
}

type Info struct {
	Message    string
	Definition token.Pos
	Type       types.Type
}

func (i *Info) GetType() types.Type {
	if i != nil {
		return i.Type
	}
	return nil
}

func funcTypeToFuncType(name string, expr *ast.FuncType, env *Env, native bool) types.FuncType {
	var paramsT []types.Type
	if expr.TypeParams != nil {
		for _, typeParam := range expr.TypeParams.List {
			for _, typeParamName := range typeParam.Names {
				typeParamType := typeParam.Type
				t := env.GetType2(typeParamType)
				t = types.GenericType{W: t, Name: typeParamName.Name, IsType: true}
				env.Define(typeParamName, typeParamName.Name, t)
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
	parts := strings.Split(name, ".")
	name = parts[len(parts)-1]
	if result == nil {
		switch expr.Result.(type) {
		case *ast.ResultExpr:
			result = types.ResultType{W: types.VoidType{}}
		case *ast.OptionExpr:
			result = types.OptionType{W: types.VoidType{}}
		default:
			result = types.VoidType{}
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
	env = env.SubEnv()
	e, err := parser.ParseExpr(s)
	if err != nil {
		panic(err)
	}
	expr := e.(*ast.FuncType)
	return funcTypeToFuncType(name, expr, env, native)
}

func NewEnv(fset *token.FileSet) *Env {
	env := &Env{fset: fset, lookupTable: make(map[string]*Info), lspTable: make(map[NodeKey]*Info)}
	env.DefinePkg("os", "os")
	env.DefinePkg("io", "io")
	env.DefinePkg("bufio", "bufio")
	env.DefinePkg("fmt", "fmt")
	env.DefinePkg("http", "net/http")
	env.DefinePkg("errors", "errors")
	env.DefinePkg("strings", "strings")
	env.DefinePkg("time", "time")
	env.DefinePkg("math", "math")
	env.DefinePkg("strconv", "strconv")
	env.Define(nil, "Option", types.OptionType{})
	env.Define(nil, "error", types.TypeType{W: types.AnyType{}})
	env.Define(nil, "nil", types.TypeType{W: types.NilType{}})
	env.Define(nil, "void", types.TypeType{W: types.VoidType{}})
	env.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
	env.Define(nil, "any", types.TypeType{W: types.AnyType{}})
	env.Define(nil, "i8", types.TypeType{W: types.I8Type{}})
	env.Define(nil, "i16", types.TypeType{W: types.I16Type{}})
	env.Define(nil, "i32", types.TypeType{W: types.I32Type{}})
	env.Define(nil, "i64", types.TypeType{W: types.I64Type{}})
	env.Define(nil, "u8", types.TypeType{W: types.U8Type{}})
	env.Define(nil, "u16", types.TypeType{W: types.U16Type{}})
	env.Define(nil, "u32", types.TypeType{W: types.U32Type{}})
	env.Define(nil, "u64", types.TypeType{W: types.U64Type{}})
	env.Define(nil, "int", types.TypeType{W: types.IntType{}})
	env.Define(nil, "uint", types.TypeType{W: types.UintType{}})
	env.Define(nil, "f32", types.TypeType{W: types.F32Type{}})
	env.Define(nil, "f64", types.TypeType{W: types.F64Type{}})
	env.Define(nil, "string", types.TypeType{W: types.StringType{}})
	env.Define(nil, "bool", types.TypeType{W: types.BoolType{}})
	env.Define(nil, "true", types.BoolValue{V: true})
	env.Define(nil, "false", types.BoolValue{V: false})
	env.Define(nil, "byte", types.TypeType{W: types.ByteType{}})
	env.Define(nil, "cmp.Ordered", types.AnyType{})
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
	env.Define(nil, "math.Sqrt2", types.F64Type{})
	env.Define(nil, "time.Duration", types.I64Type{})
	env.Define(nil, "io.Reader", types.InterfaceType{Name: "Reader", Pkg: "io"})
	env.Define(nil, "time.Time", types.StructType{Name: "Time", Pkg: "time"})
	env.Define(nil, "strings.Reader", types.StructType{Name: "Reader", Pkg: "strings"})
	env.Define(nil, "io.EOF", types.StructType{Name: "error", Pkg: "errors"})
	env.DefineFnNative("math.Sqrt", "func (x float64) float64")
	env.DefineFnNative("strings.Join", "func (elems []string, sep string) string")
	env.DefineFnNative("strings.NewReader", "func (s string) *strings.Reader")
	env.DefineFnNative("strings.Reader.Read", "func (b []byte) int!")
	env.DefineFnNative("fmt.Println", "func (a ...any) int!")
	env.DefineFnNative("fmt.Printf", "func (format string, a ...any) int!")
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
	env.DefineFnNative("io.ReadAll", "func (r io.Reader) ([]byte)!")
	env.DefineFnNative("io.ReadFull", "func (r io.Reader, buf []byte) int!")
	env.DefineFnNative("io.WriteString", "func (w io.Writer, s string) int!")
	env.DefineFnNative("io.CopyBuffer", "func (dst io.Writer, src io.Reader, buf []byte) int64!")
	env.DefineFnNative("io.CopyN", "func (dst io.Writer, src io.Reader, n int64) int64!")
	env.DefineFnNative("io.Copy", "func (dst io.Writer, src io.Reader) int64!")
	env.DefineFnNative("io.Pipe", "func () (*io.PipeReader, *io.PipeWriter)")
	env.DefineFnNative("bufio.ScanBytes", "func (data []byte, atEOF bool) (int, []byte)!")
	readFn := parseFuncTypeFromStringNative("io.Read", "func (p []byte) int!", env)
	closeFn := parseFuncTypeFromStringNative("io.Close", "func () !", env)
	ioReader := types.InterfaceType{
		Pkg:  "io",
		Name: "Reader",
		Methods: []types.FieldType{
			{Name: "Read", Typ: readFn},
		},
	}
	ioCloser := types.InterfaceType{
		Pkg:  "io",
		Name: "Closer",
		Methods: []types.FieldType{
			{Name: "Close", Typ: closeFn},
		},
	}
	ioReadCloser := types.InterfaceType{
		Pkg:  "io",
		Name: "ReadCloser",
		Methods: []types.FieldType{
			{Name: "Reader", Typ: ioReader},
			{Name: "Closer", Typ: ioCloser},
		},
	}
	env.Define(nil, "io.ReadCloser", ioReadCloser)
	env.DefineFnNative("io.ReadCloser.Read", "func (p []byte) int!")
	env.DefineFnNative("io.ReadCloser.Close", "func () !")
	env.Define(nil, "http.Response", types.StructType{Name: "Response", Pkg: "http", Fields: []types.FieldType{{Name: "Body", Typ: types.InterfaceType{Pkg: "io", Name: "ReadCloser"}}}})
	env.Define(nil, "http.Response.Body", ioReadCloser)
	env.DefineFnNative("http.Get", "func (url string) (*http.Response)!")
	env.Define(nil, "http.Request", types.StructType{Name: "Request", Pkg: "http"})
	env.DefineFnNative("http.NewRequest", "func (method, url string, body io.Reader) (*http.Request)!")
	env.Define(nil, "http.MethodGet", types.StringType{})
	env.Define(nil, "http.Client", types.StructType{Pkg: "http", Name: "Client"})
	env.DefineFnNative("http.Client.Do", "func (req *http.Request) (*http.Response)!")
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
	env.Define(nil, "agl.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
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

func (e *Env) SubEnv() *Env {
	env := &Env{
		fset:        e.fset,
		lookupTable: make(map[string]*Info),
		lspTable:    e.lspTable,
		parent:      e,
	}
	return env
}

func (e *Env) Get(name string) types.Type {
	res := e.getHelper(name)
	if res == nil && e.parent != nil {
		return e.parent.Get(name)
	}
	return res
}

func (e *Env) getHelper(name string) types.Type {
	if el := e.GetNameInfo(name); el != nil {
		return el.Type
	}
	return nil
}

func (e *Env) GetNameInfo(name string) *Info {
	info := e.lookupTable[name]
	if info == nil && e.parent != nil {
		return e.parent.GetNameInfo(name)
	}
	return info
}

func (e *Env) GetOrCreateNameInfo(name string) *Info {
	info := Or(e.GetNameInfo(name), &Info{})
	e.lookupTable[name] = info
	return info
}

func (e *Env) GetFn(name string) types.FuncType {
	return e.Get(name).(types.FuncType)
}

func (e *Env) DefineFn(name string, fnStr string) {
	fnT := parseFuncTypeFromString(name, fnStr, e)
	e.Define(nil, name, fnT)
}

func (e *Env) DefineFnNative(name string, fnStr string) {
	fnT := parseFuncTypeFromStringNative(name, fnStr, e)
	e.Define(nil, name, fnT)
}

func (e *Env) DefinePkg(name, path string) {
	e.Define(nil, name, types.PackageType{Name: name, Path: path})
}

func (e *Env) Define(n ast.Node, name string, typ types.Type) {
	assertf(e.Get(name) == nil, "duplicate declaration of %s", name)
	info := e.GetOrCreateNameInfo(name)
	info.Type = typ
	if n != nil {
		info.Definition = n.Pos()
		lookupInfo := e.lspNodeOrCreate(n)
		lookupInfo.Definition = n.Pos()
	}
}

func (e *Env) lspNode(n ast.Node) *Info {
	return e.lspTable[makeKey(n)]
}

func (e *Env) lspNodeOrCreate(n ast.Node) *Info {
	info := Or(e.lspNode(n), &Info{})
	e.lspSetNode(n, info)
	return info
}

func (e *Env) lspSetNode(n ast.Node, info *Info) {
	e.lspTable[makeKey(n)] = info
}

func (e *Env) Assign(parentInfo *Info, n ast.Node, name string, typ types.Type) {
	if name == "_" {
		return
	}
	assertf(e.Get(name) != nil, "undeclared %s", name)
	e.lspNodeOrCreate(n).Definition = parentInfo.Definition
}

func (e *Env) SetType(p *Info, x ast.Node, t types.Type) {
	assertf(t != nil, "%s: try to set type nil, %v %v", e.fset.Position(x.Pos()), x, to(x))
	info := e.lspNodeOrCreate(x)
	info.Type = t
	if p != nil {
		info.Definition = p.Definition
	}
}

type NodeKey string

func makeKey(n ast.Node) NodeKey {
	return NodeKey(fmt.Sprintf("%d_%d", n.Pos(), n.End()))
}

func (e *Env) GetType(x ast.Node) types.Type {
	if v := e.lspNode(x); v != nil {
		return v.Type
	}
	return nil
}

func (e *Env) GetType2(x ast.Node) types.Type {
	res := e.getType2Helper(x)
	if res == nil && e.parent != nil {
		return e.parent.getType2Helper(x)
	}
	return res
}

func (e *Env) getType2Helper(x ast.Node) types.Type {
	if v := e.lspNode(x); v != nil {
		return v.Type
	}
	switch xx := x.(type) {
	case *ast.Ident:
		if v2 := e.GetNameInfo(xx.Name); v2 != nil {
			return v2.Type
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
		return types.TupleType{Elts: elts}
	case *ast.BinaryExpr:
		return types.BinaryType{X: e.GetType2(xx.X), Y: e.GetType2(xx.Y)}
	case *ast.UnaryExpr:
		return types.UnaryType{X: e.GetType2(xx.X)}
	case *ast.InterfaceType:
		return types.AnyType{}
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
