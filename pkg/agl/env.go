package agl

import (
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"
	"agl/pkg/types"
	"embed"
	_ "embed"
	"fmt"
	"path/filepath"
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
	NoIdxUnwrap   bool
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

func (e *Env) loadCoreTypes() {
	// Number types
	e.Define(nil, "int", types.TypeType{W: types.IntType{}})
	e.Define(nil, "uint", types.TypeType{W: types.UintType{}})
	e.Define(nil, "i8", types.TypeType{W: types.I8Type{}})
	e.Define(nil, "i16", types.TypeType{W: types.I16Type{}})
	e.Define(nil, "i32", types.TypeType{W: types.I32Type{}})
	e.Define(nil, "i64", types.TypeType{W: types.I64Type{}})
	e.Define(nil, "u8", types.TypeType{W: types.U8Type{}})
	e.Define(nil, "u16", types.TypeType{W: types.U16Type{}})
	e.Define(nil, "u32", types.TypeType{W: types.U32Type{}})
	e.Define(nil, "u64", types.TypeType{W: types.U64Type{}})
	e.Define(nil, "f32", types.TypeType{W: types.F32Type{}})
	e.Define(nil, "f64", types.TypeType{W: types.F64Type{}})

	// Aliases should be avoided
	e.Define(nil, "int8", types.TypeType{W: types.I8Type{}})
	e.Define(nil, "int16", types.TypeType{W: types.I16Type{}})
	e.Define(nil, "int32", types.TypeType{W: types.I32Type{}})
	e.Define(nil, "int64", types.TypeType{W: types.I64Type{}})
	e.Define(nil, "uint8", types.TypeType{W: types.U8Type{}})
	e.Define(nil, "uint16", types.TypeType{W: types.U16Type{}})
	e.Define(nil, "uint32", types.TypeType{W: types.U32Type{}})
	e.Define(nil, "uint64", types.TypeType{W: types.U64Type{}})
	e.Define(nil, "float32", types.TypeType{W: types.F32Type{}})
	e.Define(nil, "float64", types.TypeType{W: types.F64Type{}})

	e.Define(nil, "void", types.TypeType{W: types.VoidType{}})
	e.Define(nil, "any", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "bool", types.TypeType{W: types.BoolType{}})
	e.Define(nil, "string", types.TypeType{W: types.StringType{}})
	e.Define(nil, "byte", types.TypeType{W: types.ByteType{}})

	e.Define(nil, "@LINE", types.StringType{})
}

func (e *Env) loadCoreFunctions() {
	e.DefineFn("assert", "func (pred bool, msg ...string)")
	e.DefineFn("make", "func[T, U any](t T, size ...U) T")
	e.DefineFn("len", "func [T any](v T) int")
	e.DefineFn("cap", "func [T any](v T) int")
	e.DefineFn("min", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("max", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("clear", "func [T ~[]Type | ~map[Type]Type1](t T)")
	e.DefineFn("append", "func [T any](slice []T, elems ...T) []T")
	e.DefineFn("close", "func (c chan<- Type)")
	e.DefineFn("panic", "func (v any)")
	e.DefineFn("new", "func [T any](T) *T")
}

func (e *Env) loadPkgFmt() {
	e.DefinePkg("fmt", "fmt")
	e.DefineFnNative("fmt.Println", "func (a ...any) int!")
	e.DefineFnNative("fmt.Printf", "func (format string, a ...any) int!")
	e.DefineFnNative("fmt.Errorf", "func (format string, a ...any) error")
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
}

func (e *Env) loadPkgIo() {
	e.DefinePkg("io", "io")
	e.Define(nil, "io.EOF", types.StructType{Name: "error", Pkg: "errors"})
	e.Define(nil, "io.Reader", types.InterfaceType{Name: "Reader", Pkg: "io"})
	e.Define(nil, "io.ReadCloser", types.InterfaceType{Pkg: "io", Name: "ReadCloser"})
	e.DefineFnNative("io.ReadAll", "func (r io.Reader) ([]byte)!")
	e.DefineFnNative("io.ReadFull", "func (r io.Reader, buf []byte) int!")
	e.DefineFnNative("io.WriteString", "func (w io.Writer, s string) int!")
	e.DefineFnNative("io.CopyBuffer", "func (dst io.Writer, src io.Reader, buf []byte) int64!")
	e.DefineFnNative("io.CopyN", "func (dst io.Writer, src io.Reader, n int64) int64!")
	e.DefineFnNative("io.Copy", "func (dst io.Writer, src io.Reader) int64!")
	e.DefineFnNative("io.Pipe", "func () (*io.PipeReader, *io.PipeWriter)")
	e.DefineFnNative("io.ReadCloser.Read", "func (p []byte) int!")
	e.DefineFnNative("io.ReadCloser.Close", "func () !")
}

func defineFromSrc(env *Env, path string, src []byte) {
	fset := token.NewFileSet()
	node := Must(parser.ParseFile(fset, "", src, parser.AllErrors|parser.ParseComments))
	pkgName := node.Name.Name
	env.DefinePkg(pkgName, path)
	for _, d := range node.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			fullName := decl.Name.Name
			if decl.Recv != nil {
				t := decl.Recv.List[0].Type
				var recvName string
				switch v := t.(type) {
				case *ast.Ident:
					recvName = v.Name
				case *ast.StarExpr:
					recvName = v.X.(*ast.Ident).Name
				}
				fullName = recvName + "." + fullName
			}
			fullName = pkgName + "." + fullName
			ft := funcTypeToFuncType("", decl.Type, env, true)
			if decl.Doc != nil && decl.Doc.List[0].Text == "// agl:wrapped" {
				env.DefineFn(fullName, ft.String())
			} else {
				env.DefineFnNative(fullName, ft.String())
			}
		case *ast.GenDecl:
			for _, s := range decl.Specs {
				switch spec := s.(type) {
				case *ast.TypeSpec:
					switch v := spec.Type.(type) {
					case *ast.Ident:
						t := env.GetType2(v)
						env.Define(nil, pkgName+"."+spec.Name.Name, t)
					case *ast.StructType:
						env.Define(nil, pkgName+"."+spec.Name.Name, types.StructType{Pkg: pkgName, Name: spec.Name.Name})
						if v.Fields != nil {
							for _, field := range v.Fields.List {
								t := env.GetType2(field.Type)
								for _, name := range field.Names {
									switch vv := t.(type) {
									case types.InterfaceType:
										fieldName := pkgName + "." + spec.Name.Name + "." + name.Name
										env.Define(nil, fieldName, types.InterfaceType{Pkg: vv.Pkg, Name: vv.Name})
									}
								}
							}
						}
					}
				}
			}
		}
	}
}

func (e *Env) loadPkgUrl() {
	e.DefinePkg("url", "net/url")
	e.Define(nil, "url.Values", types.MapType{K: types.StringType{}, V: types.StringType{}})
}

//go:embed std/*
var content embed.FS

func (e *Env) loadPkg(path string) {
	f := filepath.Base(path)
	stdFilePath := filepath.Join("std", path, f+".agl")
	by := Must(content.ReadFile(stdFilePath))
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "std/"))
	defineFromSrc(e, final, by)
}

func (e *Env) loadPkgStrings() {
	e.DefinePkg("strings", "strings")
	e.Define(nil, "strings.Reader", types.StructType{Pkg: "strings", Name: "Reader"})
	e.Define(nil, "strings.Builder", types.StructType{Pkg: "strings", Name: "Builder"})
	e.DefineFnNative("strings.Join", "func (elems []string, sep string) string")
	e.DefineFnNative("strings.NewReader", "func (s string) *strings.Reader")
	e.DefineFnNative("strings.Reader.Read", "func (b []byte) int!")
	e.DefineFnNative("strings.Builder.Write", "func (p []byte) int!")
	e.DefineFnNative("strings.Builder.WriteByte", "func (c byte) !")
	e.DefineFnNative("strings.Builder.WriteRune", "func (r rune) int!")
	e.DefineFnNative("strings.Builder.WriteString", "func (s string) int!")
	e.DefineFnNative("strings.Builder.String", "func () string")
	e.DefineFnNative("strings.Builder.Reset", "func ()")
	e.DefineFnNative("strings.Builder.Len", "func () int")
	e.DefineFnNative("strings.Builder.Grow", "func (n int)")
	e.DefineFnNative("strings.Builder.Cap", "func () int")
}

func (e *Env) loadPkgStrconv() {
	e.DefinePkg("strconv", "strconv")
	e.DefineFnNative("strconv.Itoa", "func(int) string")
	e.DefineFnNative("strconv.Atoi", "func(string) int!")
}

func (e *Env) loadPkgMath() {
	e.DefinePkg("math", "math")
	e.Define(nil, "math.Sqrt2", types.F64Type{})
	e.Define(nil, "math.Pi", types.F64Type{})
	e.DefineFnNative("math.Sqrt", "func (x float64) float64")
}

func (e *Env) loadPkgSync() {
	e.DefinePkg("sync", "sync")
	e.Define(nil, "sync.Mutex", types.StructType{Name: "Mutex", Pkg: "sync"})
	e.DefineFnNative("sync.Mutex.Lock", "func ()")
	e.DefineFnNative("sync.Mutex.Unlock", "func ()")
}

func (e *Env) loadPkgErrors() {
	e.DefinePkg("errors", "errors")
	e.DefineFn("errors.New", "func (text string) error")
}

func (e *Env) loadPkgBufio() {
	e.DefinePkg("bufio", "bufio")
	e.Define(nil, "bufio.Reader", types.StructType{Name: "Reader", Pkg: "bufio"})
	e.DefineFnNative("bufio.ScanBytes", "func (data []byte, atEOF bool) (int, []byte)!")
}

func (e *Env) loadPkgIter() {
	e.DefinePkg("iter", "iter")
	e.DefineFnNative("iter.Seq", "func [V any](yield func(V) bool)")
}

func (e *Env) loadPkgPath() {
	e.DefinePkg("path", "path")
}

func (e *Env) loadPkgReflect() {
	e.DefinePkg("reflect", "reflect")
	e.Define(nil, "reflect.Type", types.InterfaceType{Name: "Type", Pkg: "reflect"})
	e.DefineFnNative("reflect.TypeOf", "func (any) reflect.Type")
}

func (e *Env) loadPkgRuntime() {
	e.DefinePkg("runtime", "runtime")
	e.DefineFnNative("runtime.GOROOT", "func () string")
}

func (e *Env) loadPkgCmp() {
	e.DefinePkg("cmp", "cmp")
	e.Define(nil, "cmp.Ordered", types.AnyType{})
}

var astIdent = types.StructType{Pkg: "ast", Name: "Ident"}
var astStarIden = types.StarType{X: astIdent}

func (e *Env) loadPkgGoAst() {
	astDecl := types.InterfaceType{Pkg: "ast", Name: "Decl"}
	astDecls := types.ArrayType{Elt: astDecl}
	astExpr := types.InterfaceType{Pkg: "ast", Name: "Expr"}
	astFieldNames := types.ArrayType{Elt: astStarIden}
	astField := types.StructType{Pkg: "ast", Name: "Field"}
	astFieldListList := types.ArrayType{Elt: types.StarType{X: astField}}
	astFieldList := types.StructType{Pkg: "ast", Name: "FieldList"}
	astStarFieldList := types.StarType{X: astFieldList}
	astFuncType := types.StructType{Pkg: "ast", Name: "FuncType"}
	astSelectorExpr := types.StructType{Pkg: "ast", Name: "SelectorExpr"}
	astStarExpr := types.StructType{Pkg: "ast", Name: "StarExpr"}
	astFuncDecl := types.StructType{Pkg: "ast", Name: "FuncDecl"}
	astSpec := types.InterfaceType{Pkg: "ast", Name: "Spec"}
	astGenDecl := types.StructType{Pkg: "ast", Name: "GenDecl"}
	astFile := types.StructType{Pkg: "ast", Name: "File"}
	astTypeSpec := types.StructType{Pkg: "ast", Name: "TypeSpec"}
	astStructType := types.StructType{Pkg: "ast", Name: "StructType"}
	astInterfaceType := types.StructType{Pkg: "ast", Name: "InterfaceType"}
	e.DefinePkg("ast", "go/ast")
	e.Define(nil, "ast.Decl", astDecl)
	e.Define(nil, "ast.Expr", astExpr)
	e.Define(nil, "ast.Field", astField)
	e.Define(nil, "ast.Field.Names", astFieldNames)
	e.Define(nil, "ast.Field.Type", astExpr)
	e.Define(nil, "ast.FieldList", astFieldList)
	e.Define(nil, "ast.FieldList.List", astFieldListList)
	e.Define(nil, "ast.File", astFile)
	e.Define(nil, "ast.File.Decls", astDecls)
	e.Define(nil, "ast.FuncDecl", astFuncDecl)
	e.Define(nil, "ast.FuncDecl.Name", astStarIden)
	e.Define(nil, "ast.FuncDecl.Recv", astStarFieldList)
	e.Define(nil, "ast.FuncDecl.Type", types.StarType{X: astFuncType})
	e.Define(nil, "ast.GenDecl", astGenDecl)
	e.Define(nil, "ast.GenDecl.Specs", types.ArrayType{Elt: astSpec})
	e.Define(nil, "ast.FuncType", astFuncType)
	e.Define(nil, "ast.FuncType.Params", astStarFieldList)
	e.Define(nil, "ast.FuncType.Results", astStarFieldList)
	e.Define(nil, "ast.Ident", astIdent)
	e.Define(nil, "ast.Ident.Name", types.StringType{})
	e.Define(nil, "ast.SelectorExpr", astSelectorExpr)
	e.Define(nil, "ast.SelectorExpr.X", astExpr)
	e.Define(nil, "ast.SelectorExpr.Sel", astIdent)
	e.Define(nil, "ast.Spec", astSpec)
	e.Define(nil, "ast.StarExpr", astStarExpr)
	e.Define(nil, "ast.StarExpr.X", astExpr)
	e.Define(nil, "ast.StructType", astStructType)
	e.Define(nil, "ast.StructType.Fields", astStarFieldList)
	e.Define(nil, "ast.TypeSpec", astTypeSpec)
	e.Define(nil, "ast.TypeSpec.Name", astStarIden)
	e.Define(nil, "ast.TypeSpec.Type", astExpr)
	e.Define(nil, "ast.InterfaceType", astInterfaceType)
	e.Define(nil, "ast.InterfaceType.Methods", astStarFieldList)
}

func (e *Env) loadPkgGoParser() {
	e.DefinePkg("parser", "go/parser")
	e.Define(nil, "parser.AllErrors", types.UintType{})
	e.DefineFnNative("parser.ParseFile", "func (fset *token.FileSet, filename string, src any, mode Mode) (*ast.File)!")
}

func (e *Env) loadPkgGoToken() {
	e.DefinePkg("token", "go/token")
	e.Define(nil, "token.FileSet", types.StructType{Pkg: "token", Name: "FileSet"})
	e.DefineFnNative("token.NewFileSet", "func () *token.FileSet")
}

func (e *Env) loadPkgGoTypes() {
	typesImporter := types.InterfaceType{Pkg: "types", Name: "Importer"}
	e.DefinePkg("types", "go/types")
	e.Define(nil, "types.Config", types.StructType{Pkg: "types", Name: "Config"})
	e.Define(nil, "types.Importer", typesImporter)
	e.Define(nil, "types.Config.Importer", typesImporter)
	e.Define(nil, "types.Info", types.StructType{Pkg: "types", Name: "Info"})
	e.Define(nil, "types.Info.Defs", types.MapType{K: astStarIden, V: types.InterfaceType{Pkg: "types", Name: "Object"}})
	e.Define(nil, "types.Object", types.InterfaceType{Pkg: "types", Name: "Object"})
	e.Define(nil, "types.Package", types.InterfaceType{Pkg: "types", Name: "Package"})
	e.DefineFnNative("types.Config.Check", "func (path string, fset *token.FileSet, files []*ast.File, info *types.Info) (*types.Package)!")
}

func (e *Env) loadPkgFilepath() {
	e.DefinePkg("filepath", "path/filepath")
	e.DefineFnNative("filepath.Join", "func (elem ...string) string")
}

func (e *Env) loadPkgAgl() {
	e.DefinePkg("agl", "agl")
	e.Define(nil, "agl.Set", types.SetType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.Define(nil, "agl.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.DefineFn("agl.NewSet", "func [T comparable](els ...T) *agl.Set")
	e.DefineFn("agl.Set.Len", "func [T comparable](s *agl.Set[T]) int")
	e.DefineFn("agl.Set.Insert", "func [T comparable](s *agl.Set[T], el T) bool")
	e.DefineFn("agl.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T")
	e.DefineFn("agl.Vec.Map", "func [T, R any](a []T, f func(T) R) []R")
	e.DefineFn("agl.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R")
	e.DefineFn("agl.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
	e.DefineFn("agl.Vec.Sum", "func [T cmp.Ordered](a []T) T")
	e.DefineFn("agl.Vec.Joined", "func (a []string) string")
	e.DefineFn("agl.Vec.First", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.Last", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.Push", "func [T any](a []T, els ...T)")
	e.DefineFn("agl.Vec.PushFront", "func [T any](a []T, el T)")
	e.DefineFn("agl.Vec.Pop", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.PopFront", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.PopIf", "func [T any](a []T, pred func() bool) T?")
	e.DefineFn("agl.Vec.Insert", "func [T any](a []T, idx int, el T)")
	e.DefineFn("agl.Vec.Len", "func [T any](a []T) int")
	e.DefineFn("agl.Vec.IsEmpty", "func [T any](a []T) bool")
	e.DefineFn("agl.Map.Get", "func [K comparable, V any](m map[K]V) V?")
	e.DefineFn("agl.Map.Keys", "func [K comparable, V any](m map[K]V) iter.Seq[K]")
	e.DefineFn("agl.Map.Values", "func [K comparable, V any](m map[K]V) iter.Seq[V]")
	e.DefineFn("agl.Option.UnwrapOr", "func [T any](T) T")
	e.DefineFn("agl.Option.IsSome", "func () bool")
	e.DefineFn("agl.Option.IsNone", "func () bool")
	e.DefineFn("agl.Option.Unwrap", "func [T any]() T")
	e.DefineFn("agl.Result.UnwrapOr", "func [T any](T) T")
	e.DefineFn("agl.Result.IsOk", "func () bool")
	e.DefineFn("agl.Result.IsErr", "func () bool")
	e.DefineFn("agl.Result.Unwrap", "func [T any]() T")
}

func (e *Env) loadBaseValues() {
	e.loadCoreTypes()
	e.loadPkgCmp()
	e.loadCoreFunctions()
	e.loadPkgIo()
	e.loadPkgFmt()
	e.loadPkgBufio()
	e.loadPkgUrl()
	e.loadPkg("net/http")
	e.loadPkg("os")
	e.loadPkg("time")
	e.loadPkgStrings()
	e.loadPkgStrconv()
	e.loadPkgMath()
	e.loadPkgSync()
	e.loadPkgErrors()
	e.loadPkgIter()
	e.loadPkgPath()
	e.loadPkgReflect()
	e.loadPkgRuntime()
	e.loadPkgGoAst()
	e.loadPkgGoToken()
	e.loadPkgGoParser()
	e.loadPkgGoTypes()
	e.loadPkgFilepath()
	e.loadPkgAgl()
	e.Define(nil, "Option", types.OptionType{})
	e.Define(nil, "error", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "nil", types.TypeType{W: types.NilType{}})
	e.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
	e.Define(nil, "true", types.BoolValue{V: true})
	e.Define(nil, "false", types.BoolValue{V: false})
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
		NoIdxUnwrap: e.NoIdxUnwrap,
	}
	return env
}

func (e *Env) GetDirect(name string) types.Type {
	info := e.lookupTable[name]
	if info == nil {
		return nil
	}
	return info.Type
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

func (e *Env) GetNameInfoDirect(name string) *Info {
	return e.lookupTable[name]
}

func (e *Env) GetNameInfo(name string) *Info {
	info := e.lookupTable[name]
	if info == nil && e.parent != nil {
		return e.parent.GetNameInfo(name)
	}
	return info
}

func (e *Env) GetOrCreateNameInfo(name string) *Info {
	info := Or(e.GetNameInfoDirect(name), &Info{})
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

func (e *Env) DefineForce(n ast.Node, name string, typ types.Type) {
	e.defineHelper(n, name, typ, true)
}

func (e *Env) Define(n ast.Node, name string, typ types.Type) {
	e.defineHelper(n, name, typ, false)
}

func (e *Env) defineHelper(n ast.Node, name string, typ types.Type, force bool) {
	if name == "_" {
		return
	}
	if !force {
		assertf(e.GetDirect(name) == nil, "duplicate declaration of %s", name)
	}
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

func (e *Env) Assign(parentInfo *Info, n ast.Node, name string) {
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

func (e *Env) GetInfo(x ast.Node) *Info {
	return e.lspNode(x)
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
		base := e.GetType2(xx.X)
		if vv, ok := base.(types.StarType); ok {
			base = vv.X
		}
		switch v := base.(type) {
		case types.PackageType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			return e.GetType2(&ast.Ident{Name: name})
		case types.InterfaceType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			if v.Pkg != "" {
				name = v.Pkg + "." + name
			}
			return e.GetType2(&ast.Ident{Name: name})
		case types.StructType:
			name := v.GetFieldName(xx.Sel.Name)
			return e.GetType2(&ast.Ident{Name: name})
		case types.TypeAssertType:
			return v.X
		default:
			panic(fmt.Sprintf("%v", reflect.TypeOf(base)))
		}
		return nil
	case *ast.IndexExpr:
		t := e.GetType2(xx.X)
		if !e.NoIdxUnwrap {
			switch v := t.(type) {
			case types.ArrayType:
				return v.Elt
			}
		}
		return t
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
	case *ast.CompositeLit:
		ct := e.GetType2(xx.Type).(types.CustomType)
		return types.StructType{Name: ct.Name}
	case *ast.TypeAssertExpr:
		xT := e.GetType2(xx.X)
		var typeT types.Type
		if xx.Type != nil {
			typeT = e.GetType2(xx.Type)
		}
		n := types.TypeAssertType{X: xT, Type: typeT}
		info := e.lspNodeOrCreate(xx)
		info.Type = types.OptionType{W: xT} // TODO ensure xT is the right thing
		return n
	case *ast.BubbleOptionExpr:
		return e.GetType2(xx.X)
	case *ast.SliceExpr:
		return e.GetType2(xx.X) // TODO
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
