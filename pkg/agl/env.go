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

func (e *Env) loadBaseValues() {
	e.DefinePkg("os", "os")
	e.DefinePkg("io", "io")
	e.DefinePkg("bufio", "bufio")
	e.DefinePkg("fmt", "fmt")
	e.DefinePkg("http", "net/http")
	e.DefinePkg("errors", "errors")
	e.DefinePkg("strings", "strings")
	e.DefinePkg("time", "time")
	e.DefinePkg("math", "math")
	e.DefinePkg("strconv", "strconv")
	e.Define(nil, "Option", types.OptionType{})
	e.Define(nil, "error", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "nil", types.TypeType{W: types.NilType{}})
	e.Define(nil, "void", types.TypeType{W: types.VoidType{}})
	e.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
	e.Define(nil, "any", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "i8", types.TypeType{W: types.I8Type{}})
	e.Define(nil, "i16", types.TypeType{W: types.I16Type{}})
	e.Define(nil, "i32", types.TypeType{W: types.I32Type{}})
	e.Define(nil, "i64", types.TypeType{W: types.I64Type{}})
	e.Define(nil, "u8", types.TypeType{W: types.U8Type{}})
	e.Define(nil, "u16", types.TypeType{W: types.U16Type{}})
	e.Define(nil, "u32", types.TypeType{W: types.U32Type{}})
	e.Define(nil, "u64", types.TypeType{W: types.U64Type{}})
	e.Define(nil, "int", types.TypeType{W: types.IntType{}})
	e.Define(nil, "uint", types.TypeType{W: types.UintType{}})
	e.Define(nil, "f32", types.TypeType{W: types.F32Type{}})
	e.Define(nil, "f64", types.TypeType{W: types.F64Type{}})
	e.Define(nil, "string", types.TypeType{W: types.StringType{}})
	e.Define(nil, "bool", types.TypeType{W: types.BoolType{}})
	e.Define(nil, "true", types.BoolValue{V: true})
	e.Define(nil, "false", types.BoolValue{V: false})
	e.Define(nil, "byte", types.TypeType{W: types.ByteType{}})
	e.Define(nil, "cmp.Ordered", types.AnyType{})
	e.DefineFn("assert", "func (pred bool, msg ...string)")
	e.DefineFn("make", "func[T, U any](t T, size ...U) T")
	e.DefineFn("len", "func [T any](v T) int")
	e.DefineFn("cap", "func [T any](v T) int")
	e.DefineFn("min", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("max", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("clear", "func [T ~[]Type | ~map[Type]Type1](t T)")
	e.DefineFn("append", "func [T any](slice []T, elems ...T) []T")
	e.DefineFn("close", "func (c chan<- Type)")
	e.DefineFnNative("time.Sleep", "func (time.Duration)")
	e.Define(nil, "math.Sqrt2", types.F64Type{})
	e.Define(nil, "time.Duration", types.I64Type{})
	e.Define(nil, "io.Reader", types.InterfaceType{Name: "Reader", Pkg: "io"})
	e.Define(nil, "time.Time", types.StructType{Name: "Time", Pkg: "time"})
	e.Define(nil, "strings.Reader", types.StructType{Name: "Reader", Pkg: "strings"})
	e.Define(nil, "io.EOF", types.StructType{Name: "error", Pkg: "errors"})
	e.DefineFnNative("math.Sqrt", "func (x float64) float64")
	e.DefineFnNative("strings.Join", "func (elems []string, sep string) string")
	e.DefineFnNative("strings.NewReader", "func (s string) *strings.Reader")
	e.DefineFnNative("strings.Reader.Read", "func (b []byte) int!")
	e.DefineFnNative("fmt.Println", "func (a ...any) int!")
	e.DefineFnNative("fmt.Printf", "func (format string, a ...any) int!")
	e.DefineFnNative("fmt.Sprintf", "func (format string, a ...any) string")
	e.DefineFnNative("fmt.Scan", "func (a ...any) int!")
	e.DefineFnNative("fmt.Scanf", "func (format string, a ...any) int!")
	e.DefineFnNative("fmt.Scanln", "func (a ...any) int!")
	e.DefineFnNative("fmt.Sprint", "func (a ...any) string")
	e.DefineFnNative("fmt.Sprintln", "func (a ...any) string")
	e.DefineFnNative("fmt.Sscan", "func (str string, a ...any) int!")
	e.DefineFnNative("fmt.Sscanf", "func (str string, format string, a ...any) int!")
	e.DefineFnNative("fmt.Sscanln", "func (str string, a ...any) int!")
	e.DefineFnNative("fmt.Fprint", "func (w io.Writer, a ...any) int!")
	e.DefineFnNative("fmt.Fprintf", "func (w io.Writer, format string, a ...any) int!")
	e.DefineFnNative("fmt.Fprintln", "func (w io.Writer, a ...any) int!")
	e.DefineFnNative("fmt.Fscan", "func (r io.Reader, a ...any) int!")
	e.DefineFnNative("fmt.Fscanf", "func (r io.Reader, format string, a ...any) int!")
	e.DefineFnNative("fmt.Fscanln", "func (r io.Reader, a ...any) int!")
	e.DefineFnNative("io.ReadAll", "func (r io.Reader) ([]byte)!")
	e.DefineFnNative("io.ReadFull", "func (r io.Reader, buf []byte) int!")
	e.DefineFnNative("io.WriteString", "func (w io.Writer, s string) int!")
	e.DefineFnNative("io.CopyBuffer", "func (dst io.Writer, src io.Reader, buf []byte) int64!")
	e.DefineFnNative("io.CopyN", "func (dst io.Writer, src io.Reader, n int64) int64!")
	e.DefineFnNative("io.Copy", "func (dst io.Writer, src io.Reader) int64!")
	e.DefineFnNative("io.Pipe", "func () (*io.PipeReader, *io.PipeWriter)")
	e.DefineFnNative("bufio.ScanBytes", "func (data []byte, atEOF bool) (int, []byte)!")
	readFn := parseFuncTypeFromStringNative("io.Read", "func (p []byte) int!", e)
	closeFn := parseFuncTypeFromStringNative("io.Close", "func () !", e)
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
	e.Define(nil, "io.ReadCloser", ioReadCloser)
	e.DefineFnNative("io.ReadCloser.Read", "func (p []byte) int!")
	e.DefineFnNative("io.ReadCloser.Close", "func () !")
	e.Define(nil, "http.Response", types.StructType{Name: "Response", Pkg: "http", Fields: []types.FieldType{{Name: "Body", Typ: types.InterfaceType{Pkg: "io", Name: "ReadCloser"}}}})
	e.Define(nil, "http.Response.Body", ioReadCloser)
	e.DefineFnNative("http.Get", "func (url string) (*http.Response)!")
	e.Define(nil, "http.Request", types.StructType{Name: "Request", Pkg: "http"})
	e.DefineFnNative("http.NewRequest", "func (method, url string, body io.Reader) (*http.Request)!")
	e.Define(nil, "http.MethodGet", types.StringType{})
	e.Define(nil, "http.Client", types.StructType{Pkg: "http", Name: "Client"})
	e.DefineFnNative("http.Client.Do", "func (req *http.Request) (*http.Response)!")
	e.DefineFnNative("os.ReadFile", "func (name string) ([]byte)!")
	e.DefineFnNative("os.WriteFile", "func (name string, data []byte, perm os.FileMode) !")
	e.DefineFnNative("os.Chdir", "func (string) !")
	e.DefineFnNative("os.Chown", "func (name string, uid, gid int) !")
	e.DefineFnNative("os.Mkdir", "func (name string, perm FileMode) !")
	e.DefineFnNative("os.MkdirAll", "func (path string, perm FileMode) !")
	e.DefineFnNative("os.MkdirTemp", "func (dir, pattern string) string!")
	e.DefineFnNative("os.Remove", "func (name string) !")
	e.DefineFnNative("os.RemoveAll", "func (path string) !")
	e.DefineFnNative("os.Rename", "func (oldpath, newpath string) !")
	e.DefineFnNative("os.SameFile", "func (fi1, fi2 FileInfo) bool")
	e.DefineFnNative("os.TempDir", "func () string")
	e.DefineFnNative("os.Truncate", "func (name string, size int64) !")
	e.DefineFnNative("os.Setenv", "func (key, value string) !")
	e.DefineFnNative("os.Unsetenv", "func (key string) !")
	e.DefineFnNative("os.UserCacheDir", "func () string!")
	e.DefineFnNative("os.UserConfigDir", "func () string!")
	e.DefineFnNative("os.UserHomeDir", "func () string!")
	e.DefineFnNative("os.Getwd", "func () string!")
	e.DefineFnNative("os.Hostname", "func () string!")
	e.DefineFnNative("os.IsExist", "func (err error) bool")
	e.DefineFnNative("os.IsNotExist", "func (err error) bool")
	e.DefineFnNative("os.IsPathSeparator", "func (c uint8) bool")
	e.DefineFnNative("os.IsPermission", "func (err error) bool")
	e.DefineFnNative("os.IsTimeout", "func (err error) bool")
	e.DefineFnNative("os.LookupEnv", "func (key string) string?")
	e.DefineFnNative("strconv.Itoa", "func(int) string")
	e.DefineFnNative("strconv.Atoi", "func(string) int!")
	e.DefineFn("errors.New", "func (text string) error")
	e.Define(nil, "agl.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.DefineFn("agl.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T")
	e.DefineFn("agl.Vec.Map", "func [T, R any](a []T, f func(T) R) []R")
	e.DefineFn("agl.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R")
	e.DefineFn("agl.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
	e.DefineFn("agl.Vec.Sum", "func [T cmp.Ordered](a []T) T")
	e.DefineFn("agl.Vec.Joined", "func (a []string) string")
	e.DefineFn("agl.Option.UnwrapOr", "func [T any](T) T")
	e.DefineFn("agl.Option.IsSome", "func () bool")
	e.DefineFn("agl.Option.IsNone", "func () bool")
	e.DefineFn("agl.Option.Unwrap", "func [T any]() T")
	e.DefineFn("agl.Result.UnwrapOr", "func [T any](T) T")
	e.DefineFn("agl.Result.IsOk", "func () bool")
	e.DefineFn("agl.Result.IsErr", "func () bool")
	e.DefineFn("agl.Result.Unwrap", "func [T any]() T")
}

func NewEnv(fset *token.FileSet) *Env {
	env := &Env{
		fset:        fset,
		lookupTable: make(map[string]*Info),
		lspTable:    make(map[NodeKey]*Info),
	}
	env.loadBaseValues()
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
		return e.parent.GetType2(x)
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
		return types.ChanType{W: e.GetType2(xx.Value)}
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
