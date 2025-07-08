package agl

import (
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"
	"agl/pkg/types"
	"agl/pkg/utils"
	"embed"
	_ "embed"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
)

//go:embed pkgs/* core/*
var contentFs embed.FS

type Env struct {
	structCounter atomic.Int64
	lookupTable   map[string]*Info  // Store constants/variables/functions
	lspTable      map[NodeKey]*Info // Store type for Expr/Stmt
	parent        *Env
	NoIdxUnwrap   bool
}

func (e *Env) withEnv(clb func(*Env)) {
	clb(e.SubEnv())
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

func getGoRecv(e goast.Expr) string {
	recvTyp := e
	switch v := recvTyp.(type) {
	case *goast.Ident:
		if v.IsExported() {
			return v.Name
		}
	case *goast.StarExpr:
		return getGoRecv(v.X)
	case *goast.IndexExpr:
		return getGoRecv(v.X)
	case *goast.IndexListExpr:
		return getGoRecv(v.X)
	default:
		panic(fmt.Sprintf("%v", to(recvTyp)))
	}
	return ""
}

func goFuncDeclTypeToFuncType(name, pkgName string, expr *goast.FuncDecl, env *Env, keepRaw bool) types.FuncType {
	fT := goFuncTypeToFuncType(name, pkgName, expr.Type, env, keepRaw)
	var recvT []types.Type
	if expr.Recv != nil {
		for _, recv := range expr.Recv.List {
			for _, recvName := range recv.Names {
				recvTyp := recv.Type
				n := getGoRecv(recv.Type)
				if n == "" {
					continue
				}
				noop(recvName)
				t := env.GetGoType2(pkgName, recvTyp, keepRaw)
				recvT = append(recvT, t)
			}
		}
	}
	fT.Recv = recvT
	fT.Name = expr.Name.Name
	return fT
}

func goFuncTypeToFuncType(name, pkgName string, expr *goast.FuncType, env *Env, keepRaw bool) types.FuncType {
	native := utils.True()
	var paramsT []types.Type
	if expr.TypeParams != nil {
		for _, typeParam := range expr.TypeParams.List {
			for _, typeParamName := range typeParam.Names {
				typeParamType := typeParam.Type
				t := env.GetGoType2(pkgName, typeParamType, keepRaw)
				t = types.GenericType{W: t, Name: typeParamName.Name, IsType: true}
				//env.Define(typeParamName, typeParamName.Name, t)
				paramsT = append(paramsT, t)
			}
		}
	}
	var params []types.Type
	if expr.Params != nil {
		for _, param := range expr.Params.List {
			t := env.GetGoType2(pkgName, param.Type, keepRaw)
			n := max(len(param.Names), 1)
			for i := 0; i < n; i++ {
				params = append(params, t)
			}
		}
	}
	var result types.Type
	var results []types.Type
	if expr.Results != nil {
		for _, resultEl := range expr.Results.List {
			results = append(results, env.GetGoType2(pkgName, resultEl, keepRaw))
		}
	}
	if len(results) > 0 {
		lastResult := results[len(results)-1]
		lastResult = types.Unwrap(lastResult)
		if v, ok := lastResult.(types.InterfaceType); ok && v.Name == "error" {
			results = results[:len(results)-1]
			var inner types.Type = types.TupleType{Elts: results, KeepRaw: keepRaw}
			if len(results) == 0 {
				inner = types.VoidType{}
			} else if len(results) == 1 {
				inner = results[0]
			}
			result = types.ResultType{W: inner, Native: native, KeepRaw: keepRaw}
		} else if len(results) == 1 {
			result = results[0]
		} else {
			result = types.TupleType{Elts: results, KeepRaw: keepRaw}
		}
	} else {
		result = types.VoidType{}
	}
	parts := strings.Split(name, ".")
	name = parts[len(parts)-1]
	ft := types.FuncType{
		Name:       name,
		TypeParams: paramsT,
		Params:     params,
		Return:     result,
		IsNative:   native,
	}
	return ft
}

func funcDeclTypeToFuncType(name string, expr *ast.FuncDecl, env *Env, fset *token.FileSet, native bool) types.FuncType {
	fT := funcTypeToFuncType(name, expr.Type, env, fset, native)
	var recvT []types.Type
	if expr.Recv != nil {
		for _, recv := range expr.Recv.List {
			for _, n := range recv.Names {
				recvTyp := recv.Type
				t := env.GetType2(recvTyp, fset)
				if n.Mutable.IsValid() {
					t = types.MutType{W: t}
				}
				recvT = append(recvT, t)
			}
		}
	}
	fT.Recv = recvT
	fT.Name = expr.Name.Name
	return fT
}

func funcTypeToFuncType(name string, expr *ast.FuncType, env *Env, fset *token.FileSet, native bool) types.FuncType {
	var paramsT []types.Type
	if expr.TypeParams != nil {
		for _, typeParam := range expr.TypeParams.List {
			for _, typeParamName := range typeParam.Names {
				typeParamType := typeParam.Type
				t := env.GetType2(typeParamType, fset)
				t = types.GenericType{W: t, Name: typeParamName.Name, IsType: true}
				env.Define(typeParamName, typeParamName.Name, t)
				paramsT = append(paramsT, t)
			}
		}
	}
	var params []types.Type
	if expr.Params != nil {
		for _, param := range expr.Params.List {
			t := env.GetType2(param.Type, fset)
			n := max(len(param.Names), 1)
			for i := 0; i < n; i++ {
				if len(param.Names) > i && param.Names[i].Mutable.IsValid() {
					t = types.MutType{W: t}
				}
				params = append(params, t)
			}
		}
	}
	var result types.Type
	if expr.Result != nil {
		result = env.GetType2(expr.Result, fset)
		if result == nil {
			panic(fmt.Sprintf("%s: type not found %v %v", fset.Position(expr.Pos()), expr.Result, to(expr.Result)))
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

func parseFuncTypeFromString(name, s string, env *Env, fset *token.FileSet) types.FuncType {
	return parseFuncTypeFromStringHelper(name, s, env, fset, false)
}

func parseFuncTypeFromStringNative(name, s string, env *Env, fset *token.FileSet) types.FuncType {
	return parseFuncTypeFromStringHelper(name, s, env, fset, true)
}

func parseFuncTypeFromStringHelper(name, s string, env *Env, fset *token.FileSet, native bool) types.FuncType {
	env = env.SubEnv()
	e := Must(parser.ParseExpr(s))
	expr := e.(*ast.FuncType)
	return funcTypeToFuncType(name, expr, env, fset, native)
}

func parseFuncDeclFromStringHelper(name, s string, env *Env, fset *token.FileSet) types.FuncType {
	s = "package main\n" + s
	env = env.SubEnv()
	e := Must(parser.ParseFile(token.NewFileSet(), "", s, parser.AllErrors))
	expr := e.Decls[0].(*ast.FuncDecl)
	return funcDeclTypeToFuncType(name, expr, env, fset, true)
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
	e.Define(nil, "error", types.TypeType{W: types.InterfaceType{Name: "error"}})
	e.Define(nil, "nil", types.TypeType{W: types.NilType{}})
	e.Define(nil, "true", types.BoolValue{V: true})
	e.Define(nil, "false", types.BoolValue{V: false})

	e.Define(nil, "@LINE", types.StringType{})
}

func (e *Env) loadCoreFunctions() {
	e.DefineFn("assert", "func (pred bool, msg ...string)")
	e.DefineFn("make", "func[T, U any](t T, size ...U) T")
	e.DefineFn("recover", "func () any")
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

func defineFromSrc(env *Env, path, pkgName string, src []byte, m *PkgVisited) {
	fset := token.NewFileSet()
	node := Must(parser.ParseFile(fset, "", src, parser.AllErrors|parser.ParseComments))
	pkgName = Or(pkgName, node.Name.Name)
	if err := env.DefinePkg(pkgName, path); err != nil {
		//return
	}
	for _, d := range node.Imports {
		importName := ""
		if d.Name != nil {
			importName = d.Name.Name
		}
		if err := env.loadPkg(strings.ReplaceAll(d.Path.Value, `"`, ``), importName, m); err != nil {
			panic(err)
		}
	}
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
					switch vv := v.X.(type) {
					case *ast.Ident:
						recvName = vv.Name
					case *ast.SelectorExpr:
						recvName = vv.Sel.Name
					default:
						panic(fmt.Sprintf("%v", to(v.X)))
					}
				case *ast.SelectorExpr:
					recvName = v.Sel.Name
				default:
					panic(fmt.Sprintf("%v", to(t)))
				}
				fullName = recvName + "." + fullName
			}
			fullName = pkgName + "." + fullName
			ft := funcDeclTypeToFuncType("", decl, env, fset, true)
			if decl.Doc != nil && decl.Doc.List[0].Text == "// agl:wrapped" {
				ft.IsNative = false
				switch v := ft.Return.(type) {
				case types.ResultType:
					v.Native = false
					ft.Return = v
				case types.OptionType:
					v.Native = false
					ft.Return = v
				}
				if err := env.DefineFnNative2(fullName, ft); err != nil {
					assert(false, err.Error())
				}
			} else {
				ft.IsNative = true
				if err := env.DefineFnNative2(fullName, ft); err != nil {
					assert(false, err.Error())
				}
			}
		case *ast.GenDecl:
			for _, s := range decl.Specs {
				switch spec := s.(type) {
				case *ast.TypeSpec:
					specName := pkgName + "." + spec.Name.Name
					switch v := spec.Type.(type) {
					case *ast.Ident:
						t := env.GetType2(v, fset)
						env.Define(nil, specName, types.CustomType{Pkg: pkgName, Name: spec.Name.Name, W: t})
					case *ast.MapType:
						t := env.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.StarExpr:
						t := env.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.ArrayType:
						t := env.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.InterfaceType:
						if v.Methods != nil {
							for _, m := range v.Methods.List {
								for _, n := range m.Names {
									fullName := pkgName + "." + spec.Name.Name + "." + n.Name
									t := env.GetType2(m.Type, fset)
									env.Define(nil, fullName, t)
								}
							}
						}
						env.Define(nil, specName, types.InterfaceType{Pkg: pkgName, Name: spec.Name.Name})
					case *ast.StructType:
						env.Define(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name})
						if v.Fields != nil {
							for _, field := range v.Fields.List {
								t := env.GetType2(field.Type, fset)
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
				case *ast.ImportSpec:
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						fieldName := pkgName + "." + name.Name
						t := env.GetType2(spec.Type, fset)
						env.Define(nil, fieldName, t)
					}
				default:
					panic(fmt.Sprintf("%v", to(s)))
				}
			}
		}
	}
}

type Later struct {
	pkgName string
	s       goast.Spec
}

func defineStructsFromGoSrc(files []EntryContent, env *Env, vendorPath string, m *PkgVisited, keepRaw bool) {
	var tryLater []Later
	for _, entry := range files {
		//p("LOADING", fullPath)
		node := Must(goparser.ParseFile(gotoken.NewFileSet(), "", entry.Content, goparser.AllErrors|goparser.ParseComments))
		pkgName := node.Name.Name
		for _, d := range node.Imports {
			importName := ""
			if d.Name != nil {
				importName = d.Name.Name
			}
			if err := env.loadPkg(strings.ReplaceAll(d.Path.Value, `"`, ``), importName, m); err != nil {
				panic(err)
			}
		}
		for _, d := range node.Decls {
			switch decl := d.(type) {
			case *goast.GenDecl:
				for _, s := range decl.Specs {
					processSpec(s, env, pkgName, &tryLater, keepRaw)
				}
			}
		}
	}
	i := 0
	for len(tryLater) > 0 {
		var d Later
		d, tryLater = tryLater[0], tryLater[1:]
		processSpec(d.s, env, d.pkgName, &tryLater, keepRaw)
		i++
		if i > 10 {
			break
		}
	}
}

func processSpec(s goast.Spec, env *Env, pkgName string, tryLater *[]Later, keepRaw bool) {
	switch spec := s.(type) {
	case *goast.TypeSpec:
		if !spec.Name.IsExported() {
			return
		}
		specName := pkgName + "." + spec.Name.Name
		switch v := spec.Type.(type) {
		case *goast.StructType:
			var fields []types.FieldType
			if v.Fields != nil {
				for _, field := range v.Fields.List {
					tmp := func() types.Type {
						t := env.GetGoType2(pkgName, field.Type, keepRaw)
						t = types.Unwrap(t)
						if TryCast[types.VoidType](t) {
							fT := field.Type
							if vv, ok := field.Type.(*goast.StarExpr); ok {
								fT = vv.X
							}
							if vv, ok := fT.(*goast.Ident); ok && vv.Name == spec.Name.Name {
								t = types.StructType{Pkg: pkgName, Name: spec.Name.Name}
							} else {
								*tryLater = append(*tryLater, Later{s: s, pkgName: pkgName})
								//p("DEFSTRUCT1", pkgName, spec.Name.Name)
								env.DefineForce(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name, Fields: fields})
								return nil
							}
						}
						return t
					}
					if len(field.Names) == 0 {
						fields = append(fields, types.FieldType{Name: "", Typ: tmp()})
					}
					for _, name := range field.Names {
						if !name.IsExported() {
							continue
						}
						fields = append(fields, types.FieldType{Name: name.Name, Typ: tmp()})
					}
				}
			}
			//p("DEFSTRUCT2", pkgName, spec.Name.Name)
			env.DefineForce(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name, Fields: fields})
		case *goast.InterfaceType:
			if v.Methods != nil {
				for _, m := range v.Methods.List {
					for _, n := range m.Names {
						if !n.IsExported() {
							continue
						}
						fullName := pkgName + "." + spec.Name.Name + "." + n.Name
						t := env.GetGoType2(pkgName, m.Type, keepRaw)
						env.DefineForce(nil, fullName, t)
					}
				}
			}
			env.DefineForce(nil, specName, types.InterfaceType{Pkg: pkgName, Name: spec.Name.Name})
		case *goast.IndexListExpr:
		case *goast.ArrayType:
		case *goast.MapType:
		case *goast.Ident:
			t := env.GetGoType2(pkgName, v, keepRaw)
			if TryCast[types.VoidType](t) {
				if !v.IsExported() {
					return
				}
				*tryLater = append(*tryLater, Later{s: s, pkgName: pkgName})
				return
			}
			env.Define(nil, specName, t)
		case *goast.FuncType:
		case *goast.SelectorExpr:
		default:
			panic(fmt.Sprintf("%v", to(spec.Type)))
		}
	}
}

func defineFromGoSrc(env *Env, path string, src []byte, keepRaw bool) {
	node := Must(goparser.ParseFile(gotoken.NewFileSet(), "", src, goparser.AllErrors|goparser.ParseComments))
	pkgName := node.Name.Name
	_ = env.DefinePkg(pkgName, path) // Many files have the same "package"
	for _, d := range node.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			if !decl.Name.IsExported() {
				continue
			}
			fnT := goFuncDeclTypeToFuncType("", pkgName, decl, env, keepRaw)
			fullName := decl.Name.Name
			//p(fnT, "  |  ", fullName)
			if decl.Recv != nil {
				recvName := getGoRecv(decl.Recv.List[0].Type)
				if recvName == "" {
					continue
				}
				fullName = recvName + "." + fullName
			}
			fullName = pkgName + "." + fullName
			if err := env.DefineFnNative2(fullName, fnT); err != nil {
				continue // TODO should not skip errors
			}
		}
	}
}

func (e *Env) loadPkg(pkgPath, pkgName string, m *PkgVisited) error {
	pkgName = Or(pkgName, filepath.Base(pkgPath))
	//p("?LOADPKG", pkgPath, pkgName)
	pkgFullPath := trimPrefixPath(pkgPath)
	if err := e.loadPkgLocal(pkgFullPath, pkgPath, pkgName, m); err != nil {
		if err := e.loadPkgAglStd(pkgPath, pkgName, m); err != nil {
			if err := e.loadPkgStd(pkgPath, pkgName, m); err != nil {
				if err := e.loadPkgVendor(pkgPath, pkgName, m); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (e *Env) loadPkgLocal(pkgFullPath, pkgPath, pkgName string, m *PkgVisited) error {
	entries, err := os.ReadDir(pkgFullPath)
	if err != nil {
		return err
	}
	if err = e.DefinePkg(pkgPath, pkgName); err != nil {
		return nil
	}
	e.loadVendor2(pkgFullPath, m, entries)
	return nil
}

func (e *Env) loadPkgStd(path, pkgName string, m *PkgVisited) error {
	f := filepath.Base(path)
	stdFilePath := filepath.Join("pkgs", "std", path, f+".agl")
	by, err := contentFs.ReadFile(stdFilePath)
	if err != nil {
		return err
	}
	if m.ContainsAdd(stdFilePath) {
		return nil
	}
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "std/"))
	defineFromSrc(e, final, pkgName, by, m)
	return nil
}

func (e *Env) loadPkgAglStd(path, pkgName string, m *PkgVisited) error {
	f := filepath.Base(path)
	stdFilePath := filepath.Join("pkgs", path, f+".agl")
	by, err := contentFs.ReadFile(stdFilePath)
	if err != nil {
		return err
	}
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "pkgs/agl1/"))
	if m.ContainsAdd(stdFilePath) {
		return nil
	}
	defineFromSrc(e, final, pkgName, by, m)
	return nil
}

func (e *Env) loadPkgVendor(path, pkgName string, m *PkgVisited) error {
	f := filepath.Base(path)
	vendorPath := filepath.Join("vendor", path)
	if entries, err := os.ReadDir(vendorPath); err == nil {
		e.loadVendor2(vendorPath, m, entries)
	}
	stdFilePath := filepath.Join("pkgs", path, f+".agl")
	by, err := contentFs.ReadFile(stdFilePath)
	if err != nil {
		return nil
	}
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "pkgs/"))
	defineFromSrc(e, final, pkgName, by, m)
	return nil
}

type EntryContent struct {
	Name    string
	Content []byte
}

func (e *Env) loadVendor2(path string, m *PkgVisited, entries []os.DirEntry) {
	keepRaw := utils.True()
	files := make([]EntryContent, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		fullPath := filepath.Join(path, entry.Name())
		if m.ContainsAdd(fullPath) {
			continue
		}
		by, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}
		files = append(files, EntryContent{Name: entry.Name(), Content: by})
	}
	defineStructsFromGoSrc(files, e, path, m, keepRaw)
	for _, entry := range files {
		defineFromGoSrc(e, path, entry.Content, keepRaw)
	}
}

func (e *Env) loadPkgAgl() {
	_ = e.DefinePkg("agl1", "agl1")
	e.Define(nil, "Iterator", types.InterfaceType{Pkg: "agl1", Name: "Iterator", TypeParams: []types.Type{types.GenericType{Name: "T", W: types.AnyType{}}}})
	e.Define(nil, "agl1.Set", types.SetType{K: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.Define(nil, "agl1.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
	e.DefineFn("agl1.Set.Len", "func [T comparable](s agl1.Set[T]) int", WithDesc("The number of elements in the set."))
	e.DefineFn("agl1.Set.Insert", "func [T comparable](mut s agl1.Set[T], el T) bool", WithDesc("Inserts the given element in the set if it is not already present."))
	e.DefineFn("agl1.Set.Remove", "func [T comparable](mut s agl1.Set[T], el T) T?")
	e.DefineFn("agl1.Set.Contains", "func [T comparable](s agl1.Set[T], el T) bool")
	e.DefineFn("agl1.Set.Iter", "func [T comparable](s agl1.Set[T]) iter.Seq[T]")
	e.DefineFn("agl1.Set.Union", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
	e.DefineFn("agl1.Set.FormUnion", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
	e.DefineFn("agl1.Set.Subtracting", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
	e.DefineFn("agl1.Set.Subtract", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
	e.DefineFn("agl1.Set.Intersection", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]", WithDesc("Returns a new set with the elements that are common to both this set and the given sequence."))
	e.DefineFn("agl1.Set.FormIntersection", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
	e.DefineFn("agl1.Set.SymmetricDifference", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
	e.DefineFn("agl1.Set.FormSymmetricDifference", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
	e.DefineFn("agl1.Set.IsSubset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
	e.DefineFn("agl1.Set.IsStrictSubset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
	e.DefineFn("agl1.Set.IsSuperset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
	e.DefineFn("agl1.Set.IsStrictSuperset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
	e.DefineFn("agl1.Set.IsDisjoint", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool", WithDesc("Returns a Boolean value that indicates whether the set has no members in common with the given sequence."))
	e.DefineFn("agl1.Set.Intersects", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool", WithDesc("Returns a Boolean value that indicates whether the set has members in common with the given sequence."))
	e.DefineFn("agl1.Set.Equals", "func [T comparable](s, other agl1.Set[T]) bool")
	e.DefineFn("agl1.Set.Min", "func [T comparable](s agl1.Set[T]) T?")
	e.DefineFn("agl1.Set.Max", "func [T comparable](s agl1.Set[T]) T?")
	e.DefineFn("agl1.String.Split", "func (s, sep string) []string", WithDesc("Split slices s into all substrings separated by sep and returns a slice of\nthe substrings between those separators."))
	e.DefineFn("agl1.String.Replace", "func (s, old, new string, n int) string")
	e.DefineFn("agl1.String.ReplaceAll", "func (s, old, new string) string")
	e.DefineFn("agl1.String.TrimPrefix", "func (s, prefix string) string")
	e.DefineFn("agl1.String.TrimSpace", "func (s string) string")
	e.DefineFn("agl1.String.HasPrefix", "func (s, prefix string) bool")
	e.DefineFn("agl1.String.HasSuffix", "func (s, prefix string) bool")
	e.DefineFn("agl1.String.Lowercased", "func (s string) string")
	e.DefineFn("agl1.String.Uppercased", "func (s string) string")
	e.DefineFn("agl1.String.Int", "func (s) int?", WithDesc("Parse the string into an 'int' value.\nReturns None if the string cannot be parsed as an 'int'."))
	e.DefineFn("agl1.String.I8", "func (s) i8?", WithDesc("Parse the string into an 'i8' value.\nReturns None if the string cannot be parsed as an 'i8'."))
	e.DefineFn("agl1.String.I16", "func (s) i16?", WithDesc("Parse the string into an 'i16' value.\nReturns None if the string cannot be parsed as an 'i16'."))
	e.DefineFn("agl1.String.I32", "func (s) i32?", WithDesc("Parse the string into an 'i32' value.\nReturns None if the string cannot be parsed as an 'i32'."))
	e.DefineFn("agl1.String.I64", "func (s) i64?", WithDesc("Parse the string into an 'i64' value.\nReturns None if the string cannot be parsed as an 'i64'."))
	e.DefineFn("agl1.String.Uint", "func (s) uint?", WithDesc("Parse the string into an 'uint' value.\nReturns None if the string cannot be parsed as an 'uint'."))
	e.DefineFn("agl1.String.U8", "func (s) u8?", WithDesc("Parse the string into an 'u8' value.\nReturns None if the string cannot be parsed as an 'u8'."))
	e.DefineFn("agl1.String.U16", "func (s) u16?", WithDesc("Parse the string into an 'u16' value.\nReturns None if the string cannot be parsed as an 'u16'."))
	e.DefineFn("agl1.String.U32", "func (s) u32?", WithDesc("Parse the string into an 'u32' value.\nReturns None if the string cannot be parsed as an 'u32'."))
	e.DefineFn("agl1.String.U64", "func (s) u64?", WithDesc("Parse the string into an 'u64' value.\nReturns None if the string cannot be parsed as an 'u64'."))
	e.DefineFn("agl1.String.F32", "func (s) f32?", WithDesc("Parse the string into an 'f32' value.\nReturns None if the string cannot be parsed as an 'f32'."))
	e.DefineFn("agl1.String.F64", "func (s) f64?", WithDesc("Parse the string into an 'f64' value.\nReturns None if the string cannot be parsed as an 'f64'."))
	e.DefineFn("agl1.Vec.Iter", "func [T any](a []T) iter.Seq[T]")
	e.DefineFn("agl1.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T", WithDesc("Returns a new collection of the same type containing, in order, the elements of the original collection that satisfy the given predicate."))
	e.DefineFn("agl1.Vec.AllSatisfy", "func [T any](a []T, f func(T) bool) bool", WithDesc("Returns a Boolean value indicating whether every element of a sequence satisfies a given predicate."))
	e.DefineFn("agl1.Vec.Map", "func [T, R any](a []T, f func(T) R) []R", WithDesc("Returns an array containing the results of mapping the given closure over the sequence’s elements."))
	e.DefineFn("agl1.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R", WithDesc("Returns the result of combining the elements of the sequence using the given closure."))
	e.DefineFn("agl1.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
	e.DefineFn("agl1.Vec.Sum", "func [T cmp.Ordered](a []T) T")
	e.DefineFn("agl1.Vec.Joined", "func (a []string) string", WithDesc("Returns the elements of this sequence of sequences, concatenated."))
	e.DefineFn("agl1.Vec.Sorted", "func [E cmp.Ordered](a []E) []E", WithDesc("Returns the elements of the sequence, sorted."))
	e.DefineFn("agl1.Vec.First", "func [T any](a []T) T?")
	e.DefineFn("agl1.Vec.Last", "func [T any](a []T) T?")
	e.DefineFn("agl1.Vec.Remove", "func [T any](a []T, i int)")
	e.DefineFn("agl1.Vec.Clone", "func [T any](a []T) []T")
	e.DefineFn("agl1.Vec.Indices", "func [T any](a []T) []int")
	e.DefineFn("agl1.Vec.Contains", "func [T comparable](a []T, e T) bool")
	e.DefineFn("agl1.Vec.Any", "func [T any](a []T, f func(T) bool) bool")
	e.DefineFn("agl1.Vec.Push", "func [T any](mut a []T, els ...T)")
	e.DefineFn("agl1.Vec.PushFront", "func [T any](mut a []T, el T)")
	e.DefineFn("agl1.Vec.Pop", "func [T any](mut a []T) T?")
	e.DefineFn("agl1.Vec.PopFront", "func [T any](mut a []T) T?")
	e.DefineFn("agl1.Vec.PopIf", "func [T any](mut a []T, pred func() bool) T?")
	e.DefineFn("agl1.Vec.Insert", "func [T any](mut a []T, idx int, el T)")
	e.DefineFn("agl1.Vec.Len", "func [T any](a []T) int")
	e.DefineFn("agl1.Vec.IsEmpty", "func [T any](a []T) bool")
	e.DefineFn("agl1.Map.Get", "func [K comparable, V any](m map[K]V) V?")
	e.DefineFn("agl1.Map.Keys", "func [K comparable, V any](m map[K]V) iter.Seq[K]")
	e.DefineFn("agl1.Map.Values", "func [K comparable, V any](m map[K]V) iter.Seq[V]")
	e.DefineFn("agl1.Option.Unwrap", "func [T any]() T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or panic if None."))
	e.DefineFn("agl1.Option.UnwrapOr", "func [T any](t T) T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or a default if None."))
	e.DefineFn("agl1.Option.UnwrapOrDefault", "func [T any]() T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or the default if None."))
	e.DefineFn("agl1.Option.IsSome", "func () bool")
	e.DefineFn("agl1.Option.IsNone", "func () bool")
	e.DefineFn("agl1.Result.Unwrap", "func [T any]() T")
	e.DefineFn("agl1.Result.UnwrapOr", "func [T any](t T) T")
	e.DefineFn("agl1.Result.UnwrapOrDefault", "func [T any]() T")
	e.DefineFn("agl1.Result.IsOk", "func () bool")
	e.DefineFn("agl1.Result.IsErr", "func () bool")
}

func CoreFns() string {
	return string(Must(contentFs.ReadFile(filepath.Join("core", "core.agl"))))
}

func (e *Env) loadBaseValues() {
	m := NewPkgVisited()
	e.loadCoreTypes()
	_ = e.loadPkgAglStd("agl1/cmp", "", m)
	e.loadCoreFunctions()
	_ = e.loadPkgAglStd("agl1/iter", "", m)
	e.loadPkgAgl()
	e.Define(nil, "Option", types.OptionType{})
	e.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
}

func NewEnv() *Env {
	env := &Env{
		lookupTable: make(map[string]*Info),
		lspTable:    make(map[NodeKey]*Info),
	}
	env.loadBaseValues()
	return env
}

func (e *Env) SubEnv() *Env {
	env := &Env{
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

func (e *Env) DefineFn(name string, fnStr string, opts ...SetTypeOption) {
	fnT := parseFuncTypeFromString(name, fnStr, e, nil)
	e.Define(nil, name, fnT, opts...)
}

func (e *Env) DefineFnNative(name string, fnStr string, fset *token.FileSet) {
	fnT := parseFuncDeclFromStringHelper(name, fnStr, e, fset)
	e.Define(nil, name, fnT)
}

func (e *Env) DefineFnNative2(name string, fnT types.FuncType) error {
	return e.Define1(nil, name, fnT)
}

func (e *Env) DefinePkg(name, path string) error {
	return e.DefineErr(nil, name, types.PackageType{Name: name, Path: path})
}

func (e *Env) DefineForce(n ast.Node, name string, typ types.Type) {
	err := e.defineHelper(n, name, typ, true)
	if err != nil {
		assert(false, err.Error())
	}
}

func (e *Env) Define(n ast.Node, name string, typ types.Type, opts ...SetTypeOption) {
	if err := e.Define1(n, name, typ, opts...); err != nil {
		assert(false, err.Error())
	}
}

func (e *Env) Define1(n ast.Node, name string, typ types.Type, opts ...SetTypeOption) error {
	err := e.defineHelper(n, name, typ, false, opts...)
	if err != nil {
		return err
	}
	return nil
}

func (e *Env) DefineErr(n ast.Node, name string, typ types.Type) error {
	return e.defineHelper(n, name, typ, false)
}

func (e *Env) defineHelper(n ast.Node, name string, typ types.Type, force bool, opts ...SetTypeOption) error {
	if name == "_" {
		return nil
	}
	//p("DEF", name, typ)
	conf := &SetTypeConf{}
	for _, opt := range opts {
		opt(conf)
	}
	if !force {
		t := e.GetDirect(name)
		if t != nil {
			if strings.Contains(name, "CompareType") || strings.Contains(name, "ContextDiff") || strings.Contains(name, "yaml.") {
				return nil
			}
			//return fmt.Errorf("duplicate declaration of %s", name)
		}
	}
	info := e.GetOrCreateNameInfo(name)
	info.Type = typ
	if conf.description != "" {
		info.Message = conf.description
	}
	if n != nil {
		info.Definition = n.Pos()
		lookupInfo := e.lspNodeOrCreate(n)
		lookupInfo.Definition = n.Pos()
	}
	return nil
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

func (e *Env) Assign(parentInfo *Info, n ast.Node, name string, fset *token.FileSet, mutEnforced bool) error {
	if name == "_" {
		return nil
	}
	t := e.Get(name)
	if t == nil {
		return fmt.Errorf("%s: undeclared %s", fset.Position(n.Pos()), name)
	}
	if mutEnforced && !TryCast[types.MutType](t) {
		return fmt.Errorf("%s: cannot assign to immutable variable '%s'", fset.Position(n.Pos()), name)
	}
	e.lspNodeOrCreate(n).Definition = parentInfo.Definition
	return nil
}

func (e *Env) SetType(p *Info, x ast.Node, t types.Type, fset *token.FileSet) {
	assertf(t != nil, "%s: try to set type nil, %v %v", fset.Position(x.Pos()), x, to(x))
	info := e.lspNodeOrCreate(x)
	info.Type = t
	if p != nil {
		info.Message = p.Message
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

func (e *Env) GetType2(x ast.Node, fset *token.FileSet) types.Type {
	res := e.getType2Helper(x, fset)
	if res == nil && e.parent != nil {
		return e.parent.GetType2(x, fset)
	}
	return res
}

func (e *Env) getType2Helper(x ast.Node, fset *token.FileSet) types.Type {
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
		return funcTypeToFuncType("", xx, e, fset, false)
	case *ast.Ellipsis:
		t := e.GetType2(xx.Elt, fset)
		panicIfNil(t, xx.Elt)
		return types.EllipsisType{Elt: t}
	case *ast.ArrayType:
		t := e.GetType2(xx.Elt, fset)
		panicIfNil(t, xx.Elt)
		return types.ArrayType{Elt: t}
	case *ast.ResultExpr:
		t := e.GetType2(xx.X, fset)
		panicIfNil(t, xx.X)
		return types.ResultType{W: t}
	case *ast.OptionExpr:
		t := e.GetType2(xx.X, fset)
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
		base := e.GetType2(xx.X, fset)
		base = types.Unwrap(base)
		switch v := base.(type) {
		case types.PackageType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			return e.GetType2(&ast.Ident{Name: name}, fset)
		case types.InterfaceType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			if v.Pkg != "" {
				name = v.Pkg + "." + name
			}
			return e.GetType2(&ast.Ident{Name: name}, fset)
		case types.StructType:
			name := v.GetFieldName(xx.Sel.Name)
			return e.GetType2(&ast.Ident{Name: name}, fset)
		case types.TypeAssertType:
			return v.X
		default:
			panic(fmt.Sprintf("%v %v", xx.X, reflect.TypeOf(base)))
		}
		return nil
	case *ast.IndexExpr:
		t := e.GetType2(xx.X, fset)
		if !e.NoIdxUnwrap {
			switch v := t.(type) {
			case types.ArrayType:
				return v.Elt
			}
		}
		return t
	case *ast.ParenExpr:
		return e.GetType2(xx.X, fset)
	case *ast.VoidExpr:
		return types.VoidType{}
	case *ast.StarExpr:
		return types.StarType{X: e.GetType2(xx.X, fset)}
	case *ast.MapType:
		return types.MapType{K: e.GetType2(xx.Key, fset), V: e.GetType2(xx.Value, fset)}
	case *ast.SetType:
		return types.SetType{K: e.GetType2(xx.Key, fset)}
	case *ast.ChanType:
		return types.ChanType{W: e.GetType2(xx.Value, fset)}
	case *ast.TupleExpr:
		var elts []types.Type
		for _, v := range xx.Values { // TODO NO GOOD
			elt := e.GetType2(v, fset)
			elts = append(elts, elt)
		}
		return types.TupleType{Elts: elts}
	case *ast.BinaryExpr:
		return types.BinaryType{X: e.GetType2(xx.X, fset), Y: e.GetType2(xx.Y, fset)}
	case *ast.UnaryExpr:
		return types.UnaryType{X: e.GetType2(xx.X, fset)}
	case *ast.InterfaceType:
		return types.AnyType{}
	case *ast.CompositeLit:
		switch v := e.GetType2(xx.Type, fset).(type) {
		case types.CustomType:
			return types.StructType{Pkg: v.Pkg, Name: v.Name}
		case types.ArrayType:
			return types.ArrayType{Elt: e.GetType2(xx.Elts[0], fset)}
		case types.StructType:
			return types.StructType{Pkg: v.Pkg, Name: v.Name}
		default:
			//return nil
			panic(fmt.Sprintf("%v %v", xx.Type, reflect.TypeOf(v)))
		}
	case *ast.TypeAssertExpr:
		xT := e.GetType2(xx.X, fset)
		var typeT types.Type
		if xx.Type != nil {
			typeT = e.GetType2(xx.Type, fset)
		}
		n := types.TypeAssertType{X: xT, Type: typeT}
		info := e.lspNodeOrCreate(xx)
		info.Type = types.OptionType{W: xT} // TODO ensure xT is the right thing
		return n
	case *ast.BubbleOptionExpr:
		return e.GetType2(xx.X, fset)
	case *ast.SliceExpr:
		return e.GetType2(xx.X, fset) // TODO
	case *ast.IndexListExpr:
		return e.GetType2(xx.X, fset) // TODO
	case *ast.StructType:
		return types.StructType{}
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}

func (e *Env) GetGoType2(pkgName string, x goast.Node, keepRaw bool) types.Type {
	res := e.getGoType2Helper(pkgName, x, keepRaw)
	if res == nil && e.parent != nil {
		return e.parent.GetGoType2(pkgName, x, keepRaw)
	}
	return res
}

func (e *Env) getGoType2Helper(pkgName string, x goast.Node, keepRaw bool) types.Type {
	//if v := e.lspNode(x); v != nil {
	//	return v.Type
	//}
	switch xx := x.(type) {
	case *goast.Ident:
		if v2 := e.GetNameInfo(xx.Name); v2 != nil {
			return v2.Type
		}
		if v2 := e.GetNameInfo(pkgName + "." + xx.Name); v2 != nil {
			return v2.Type
		}
		//p("?", pkgName, xx.Name)
		return types.AnyType{}
		return nil
	case *goast.FuncType:
		return goFuncTypeToFuncType("", pkgName, xx, e, true)
	case *goast.Ellipsis:
		t := e.GetGoType2(pkgName, xx.Elt, keepRaw)
		panicIfNil(t, xx.Elt)
		return types.EllipsisType{Elt: t}
	case *goast.ArrayType:
		t := e.GetGoType2(pkgName, xx.Elt, keepRaw)
		panicIfNil(t, xx.Elt)
		return types.ArrayType{Elt: t}
	case *goast.CallExpr:
		return nil
	case *goast.BasicLit:
		switch xx.Kind {
		case gotoken.INT:
			return types.UntypedNumType{}
		case gotoken.STRING:
			return types.UntypedStringType{}
		default:
			panic(fmt.Sprintf("%v", xx.Kind))
		}
	case *goast.SelectorExpr:
		base := e.GetGoType2(pkgName, xx.X, keepRaw)
		base = types.Unwrap(base)
		switch v := base.(type) {
		case types.PackageType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			return e.GetGoType2(pkgName, &goast.Ident{Name: name}, keepRaw)
		case types.InterfaceType:
			name := fmt.Sprintf("%s.%s", v.Name, xx.Sel.Name)
			if v.Pkg != "" {
				name = v.Pkg + "." + name
			}
			return e.GetGoType2(pkgName, &goast.Ident{Name: name}, keepRaw)
		case types.StructType:
			name := v.GetFieldName(xx.Sel.Name)
			return e.GetGoType2(pkgName, &goast.Ident{Name: name}, keepRaw)
		case types.TypeAssertType:
			return v.X
		default:
			//return types.VoidType{}
			panic(fmt.Sprintf("%v %v %v", reflect.TypeOf(base), xx, xx.X))
		}
		return nil
	case *goast.IndexExpr:
		t := e.GetGoType2(pkgName, xx.X, keepRaw)
		if !e.NoIdxUnwrap {
			switch v := t.(type) {
			case types.ArrayType:
				return v.Elt
			}
		}
		return t
	case *goast.ParenExpr:
		return e.GetGoType2(pkgName, xx.X, keepRaw)
	case *goast.StarExpr:
		return types.StarType{X: e.GetGoType2(pkgName, xx.X, keepRaw)}
	case *goast.MapType:
		return types.MapType{K: e.GetGoType2(pkgName, xx.Key, keepRaw), V: e.GetGoType2(pkgName, xx.Value, keepRaw)}
	case *goast.ChanType:
		return types.ChanType{W: e.GetGoType2(pkgName, xx.Value, keepRaw)}
	case *goast.BinaryExpr:
		return types.BinaryType{X: e.GetGoType2(pkgName, xx.X, keepRaw), Y: e.GetGoType2(pkgName, xx.Y, keepRaw)}
	case *goast.UnaryExpr:
		return types.UnaryType{X: e.GetGoType2(pkgName, xx.X, keepRaw)}
	case *goast.InterfaceType:
		return types.AnyType{}
	case *goast.CompositeLit:
		ct := e.GetGoType2(pkgName, xx.Type, keepRaw).(types.CustomType)
		return types.StructType{Pkg: ct.Pkg, Name: ct.Name}
	case *goast.SliceExpr:
		return e.GetGoType2(pkgName, xx.X, keepRaw) // TODO
	case *goast.IndexListExpr:
		return e.GetGoType2(pkgName, xx.X, keepRaw) // TODO
	case *goast.StructType:
		return types.StructType{}
	case *goast.Field:
		return e.GetGoType2(pkgName, xx.Type, keepRaw)
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}
