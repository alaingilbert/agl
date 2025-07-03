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

//go:embed std/* pkgs/* core/*
var contentFs embed.FS

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
		if result == nil {
			panic(fmt.Sprintf("%s: type not found %v %v", env.fset.Position(expr.Pos()), expr.Result, to(expr.Result)))
		}
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

	e.Define(nil, "uintptr", types.TypeType{W: types.UintptrType{}})
	e.Define(nil, "complex128", types.TypeType{W: types.Complex128Type{}})
	e.Define(nil, "complex64", types.TypeType{W: types.Complex64Type{}})
	e.Define(nil, "void", types.TypeType{W: types.VoidType{}})
	e.Define(nil, "any", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "bool", types.TypeType{W: types.BoolType{}})
	e.Define(nil, "string", types.TypeType{W: types.StringType{}})
	e.Define(nil, "byte", types.TypeType{W: types.ByteType{}})
	e.Define(nil, "rune", types.TypeType{W: types.RuneType{}})
	e.Define(nil, "error", types.TypeType{W: types.AnyType{}})
	e.Define(nil, "nil", types.TypeType{W: types.NilType{}})
	e.Define(nil, "true", types.BoolValue{V: true})
	e.Define(nil, "false", types.BoolValue{V: false})

	e.Define(nil, "@LINE", types.StringType{})
}

func (e *Env) loadCoreFunctions() {
	e.DefineFn("assert", "func (pred bool, msg ...string)")
	e.DefineFn("make", "func[T, U any](t T, size ...U) T")
	e.DefineFn("len", "func [T any](v T) int")
	e.DefineFn("cap", "func [T any](v T) int")
	e.DefineFn("min", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("max", "func [T cmp.Ordered](x T, y ...T) T")
	e.DefineFn("abs", "func [T AglNumber](x T) T")
	//e.DefineFn("zip", "func [T, U any](x []T, y []U) [](T, U)")
	//e.DefineFn("clear", "func [T ~[]Type | ~map[Type]Type1](t T)")
	e.DefineFn("append", "func [T any](slice []T, elems ...T) []T")
	e.DefineFn("close", "func (c chan<- Type)")
	e.DefineFn("panic", "func (v any)")
	e.DefineFn("new", "func [T any](T) *T")
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
				default:
					panic(fmt.Sprintf("%v", to(t)))
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
					specName := pkgName + "." + spec.Name.Name
					switch v := spec.Type.(type) {
					case *ast.Ident:
						t := env.GetType2(v)
						env.Define(nil, specName, types.CustomType{Pkg: pkgName, Name: spec.Name.Name, W: t})
					case *ast.MapType:
						t := env.GetType2(v)
						env.Define(nil, specName, t)
					case *ast.StarExpr:
						t := env.GetType2(v)
						env.Define(nil, specName, t)
					case *ast.ArrayType:
						t := env.GetType2(v)
						env.Define(nil, specName, t)
					case *ast.InterfaceType:
						env.Define(nil, specName, types.InterfaceType{Pkg: pkgName, Name: spec.Name.Name})
					case *ast.StructType:
						env.Define(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name})
						if v.Fields != nil {
							for _, field := range v.Fields.List {
								t := env.GetType2(field.Type)
								for _, name := range field.Names {
									fieldName := pkgName + "." + spec.Name.Name + "." + name.Name
									switch vv := t.(type) {
									case types.InterfaceType:
										env.Define(nil, fieldName, types.InterfaceType{Pkg: vv.Pkg, Name: vv.Name})
									case types.StructType:
										env.Define(nil, fieldName, types.StructType{Pkg: vv.Pkg, Name: vv.Name})
									case types.TypeType:
										env.Define(nil, fieldName, vv.W)
									case types.ArrayType:
										env.Define(nil, fieldName, vv)
									case types.StarType:
										env.Define(nil, fieldName, vv)
									case types.MapType:
										env.Define(nil, fieldName, vv)
									case types.CustomType:
										env.Define(nil, fieldName, vv)
									default:
										panic(fmt.Sprintf("%v", to(t)))
									}
								}
							}
						}
					default:
						panic(fmt.Sprintf("%v", to(spec.Type)))
					}
				}
			}
		}
	}
}

func (e *Env) loadPkg(path string) {
	f := filepath.Base(path)
	stdFilePath := filepath.Join("std", path, f+".agl")
	by := Must(contentFs.ReadFile(stdFilePath))
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "std/"))
	defineFromSrc(e, final, by)
}

func (e *Env) loadVendor(path string) {
	f := filepath.Base(path)
	stdFilePath := filepath.Join("pkgs", path, f+".agl")
	by := Must(contentFs.ReadFile(stdFilePath))
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "pkgs/"))
	defineFromSrc(e, final, by)
}

func (e *Env) loadPkgAgl() {
	e.DefinePkg("agl", "agl")
	e.Define(nil, "agl.Set", types.SetType{K: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.Define(nil, "agl.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.DefineFn("agl.NewSet", "func [T comparable](els ...T) *agl.Set")
	e.DefineFn("agl.Set.Len", "func [T comparable](s agl.Set[T]) int")
	e.DefineFn("agl.Set.Insert", "func [T comparable](s agl.Set[T], el T) bool")
	e.DefineFn("agl.Set.Union", "func [T comparable](s, other agl.Set[T]) agl.Set[T]")
	e.DefineFn("agl.Set.Subtracting", "func [T comparable](s, other agl.Set[T]) agl.Set[T]")
	e.DefineFn("agl.String.Split", "func (s, sep string) []string")
	e.DefineFn("agl.String.HasPrefix", "func (s, prefix string) bool")
	e.DefineFn("agl.String.HasSuffix", "func (s, prefix string) bool")
	e.DefineFn("agl.String.Lowercased", "func (s string) string")
	e.DefineFn("agl.String.Uppercased", "func (s string) string")
	e.DefineFn("agl.String.Int", "func (s) int?")
	e.DefineFn("agl.String.I8", "func (s) i8?")
	e.DefineFn("agl.String.I16", "func (s) i16?")
	e.DefineFn("agl.String.I32", "func (s) i32?")
	e.DefineFn("agl.String.I64", "func (s) i64?")
	e.DefineFn("agl.String.Uint", "func (s) uint?")
	e.DefineFn("agl.String.U8", "func (s) u8?")
	e.DefineFn("agl.String.U16", "func (s) u16?")
	e.DefineFn("agl.String.U32", "func (s) u32?")
	e.DefineFn("agl.String.U64", "func (s) u64?")
	e.DefineFn("agl.String.F32", "func (s) f32?")
	e.DefineFn("agl.String.F64", "func (s) f64?")
	e.DefineFn("agl.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T")
	e.DefineFn("agl.Vec.AllSatisfy", "func [T any](a []T, f func(T) bool) bool")
	e.DefineFn("agl.Vec.Map", "func [T, R any](a []T, f func(T) R) []R")
	e.DefineFn("agl.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R")
	e.DefineFn("agl.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
	e.DefineFn("agl.Vec.Sum", "func [T cmp.Ordered](a []T) T")
	e.DefineFn("agl.Vec.Joined", "func (a []string) string")
	e.DefineFn("agl.Vec.Sorted", "func [E cmp.Ordered](a []E) []E")
	e.DefineFn("agl.Vec.First", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.Last", "func [T any](a []T) T?")
	e.DefineFn("agl.Vec.Remove", "func [T any](a []T, i int)")
	e.DefineFn("agl.Vec.Clone", "func [T any](a []T) []T")
	e.DefineFn("agl.Vec.Indices", "func [T any](a []T) []int")
	e.DefineFn("agl.Vec.Contains", "func [T comparable](a []T, e T) bool")
	e.DefineFn("agl.Vec.Any", "func [T any](a []T, f func(T) bool) bool")
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
	//e.DefineFn("agl.Vec.Enumerated", "func [T any](a []T) [](int, T)")
}

func CoreFns() string {
	return string(Must(contentFs.ReadFile(filepath.Join("core", "core.agl"))))
}

func (e *Env) loadBaseValues() {
	e.loadCoreTypes()
	e.loadPkg("cmp")
	e.loadCoreFunctions()
	e.loadPkg("io")
	e.loadPkg("fmt")
	e.loadPkg("encoding/binary")
	e.loadPkg("encoding/hex")
	e.loadPkg("bufio")
	e.loadPkg("net/url")
	e.loadPkg("net/http")
	e.loadPkg("os")
	e.loadPkg("sort")
	e.loadPkg("time")
	e.loadPkg("strings")
	e.loadPkg("strconv")
	e.loadPkg("context")
	e.loadPkg("math")
	//e.loadPkg("math/rand")
	e.loadPkg("math/big")
	e.loadPkg("crypto/rand")
	e.loadPkg("errors")
	e.loadPkg("unsafe")
	e.loadPkg("io/fs")
	e.loadPkg("embed")
	e.loadPkg("sync")
	e.loadPkg("sync/atomic")
	e.loadPkg("log")
	e.loadPkg("log/slog")
	e.loadPkg("reflect")
	e.loadPkg("runtime")
	e.loadPkg("iter")
	e.loadPkg("bytes")
	e.loadPkg("encoding/json")
	e.loadPkg("path")
	e.loadPkg("go/ast")
	e.loadPkg("go/token")
	e.loadPkg("go/parser")
	e.loadPkg("go/types")
	e.loadPkg("path/filepath")
	e.loadPkg("regexp")
	e.loadPkg("slices")
	//e.loadVendor("golang.org/x/net/html")
	e.loadPkgAgl()
	e.Define(nil, "Option", types.OptionType{})
	e.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
}

func NewEnv(fset *token.FileSet) *Env {
	return NewEnv1(fset, false)
}

func NewEnv1(fset *token.FileSet, isCore bool) *Env {
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
	return e.lspTable[e.makeKey(n)]
}

func (e *Env) lspNodeOrCreate(n ast.Node) *Info {
	info := Or(e.lspNode(n), &Info{})
	e.lspSetNode(n, info)
	return info
}

func (e *Env) lspSetNode(n ast.Node, info *Info) {
	e.lspTable[e.makeKey(n)] = info
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
	return NodeKey(fmt.Sprintf("%d_%d_%v", n.Pos(), n.End(), reflect.TypeOf(n)))
}

func (e *Env) makeKey(x ast.Node) NodeKey {
	return makeKey(x)
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

func panicIfNil(t types.Type, typ any) {
	if t == nil {
		panic(fmt.Sprintf("type not found %v %v", typ, to(typ)))
	}
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
		t := e.GetType2(xx.Elt)
		panicIfNil(t, xx.Elt)
		return types.EllipsisType{Elt: t}
	case *ast.ArrayType:
		t := e.GetType2(xx.Elt)
		panicIfNil(t, xx.Elt)
		return types.ArrayType{Elt: t}
	case *ast.ResultExpr:
		t := e.GetType2(xx.X)
		panicIfNil(t, xx.X)
		return types.ResultType{W: t}
	case *ast.OptionExpr:
		t := e.GetType2(xx.X)
		panicIfNil(t, xx.X)
		return types.OptionType{W: t}
	case *ast.CallExpr:
		return nil
	case *ast.BasicLit:
		switch xx.Kind {
		case token.INT:
			return types.UntypedNumType{}
		case token.STRING:
			return types.UntypedStringType{}
		default:
			panic(fmt.Sprintf("%v", xx.Kind))
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
	case *ast.SetType:
		return types.SetType{K: e.GetType2(xx.Key)}
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
		return types.StructType{Pkg: ct.Pkg, Name: ct.Name}
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
	case *ast.IndexListExpr:
		return e.GetType2(xx.X) // TODO
	case *ast.StructType:
		return types.StructType{}
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
