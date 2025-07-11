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
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

//go:embed pkgs/* core/*
var contentFs embed.FS

var envIDCounter atomic.Int64

type Env struct {
	ID            int64
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
	Message     string
	Definition  token.Pos
	Definition1 DefinitionProvider
	Type        types.Type
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
				env.Define(nil, typeParamName.Name, t)
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

func (e *Env) loadCoreFunctions(m *PkgVisited) {
	e.withEnv(func(nenv *Env) {
		_ = e.loadPkgAglStd(0, nil, nenv, "agl1/cmp", "", m)
		e.DefineFn(nenv, "assert", "func (pred bool, msg ...string)")
		e.DefineFn(nenv, "make", "func[T, U any](t T, size ...U) T")
		e.DefineFn(nenv, "recover", "func () any")
		e.DefineFn(nenv, "len", "func [T any](v T) int")
		e.DefineFn(nenv, "cap", "func [T any](v T) int")
		e.DefineFn(nenv, "min", "func [T cmp.Ordered](x T, y ...T) T")
		e.DefineFn(nenv, "max", "func [T cmp.Ordered](x T, y ...T) T")
		e.DefineFn(nenv, "abs", "func [T AglNumber](x T) T")
		//e.DefineFn("zip", "func [T, U any](x []T, y []U) [](T, U)")
		//e.DefineFn("clear", "func [T ~[]Type | ~map[Type]Type1](t T)")
		e.DefineFn(nenv, "append", "func [T any](slice []T, elems ...T) []T")
		e.DefineFn(nenv, "close", "func (c chan<- Type)")
		e.DefineFn(nenv, "panic", "func (v any)")
		e.DefineFn(nenv, "new", "func [T any](T) *T")
	})
}

func loadAglImports(fileName string, depth int, t *TreeDrawer, env, nenv *Env, node *ast.File, m *PkgVisited) {
	var imports [][]string
	for _, d := range node.Imports {
		importName := ""
		if d.Name != nil {
			importName = d.Name.Name
		}
		path := strings.ReplaceAll(d.Path.Value, `"`, ``)
		imports = append(imports, []string{path, importName})
	}
	loadImports(fileName, depth, t, env, nenv, imports, m)
}

func loadGoImports(fileName string, depth int, t *TreeDrawer, env, nenv *Env, node *goast.File, m *PkgVisited) {
	var imports [][]string
	for _, d := range node.Imports {
		importName := ""
		if d.Name != nil {
			importName = d.Name.Name
		}
		path := strings.ReplaceAll(d.Path.Value, `"`, ``)
		imports = append(imports, []string{path, importName})
	}
	loadImports(fileName, depth, t, env, nenv, imports, m)
}

type TreeDrawer struct {
	rows []string
}

func (t *TreeDrawer) Add(depth int, lbl string) {
	t.rows = append(t.rows, fmt.Sprintf("%s├─ %s\n", strings.Repeat("│ ", depth), lbl))
}

func (t *TreeDrawer) Draw() {
	fmt.Print(strings.Join(t.rows, ""))
}

func loadImports(fileName string, depth int, t *TreeDrawer, env, nenv *Env, imports [][]string, m *PkgVisited) {
	if t != nil {
		t.Add(depth, fileName)
	}
	for _, d := range imports {
		path, importName := d[0], d[1]
		if t != nil {
			t.Add(depth+1, path)
		}
		if err := env.loadPkg(depth+2, t, nenv, path, importName, m); err != nil {
			panic(err)
		}
	}
}

func defineFromSrc(depth int, t *TreeDrawer, env, nenv *Env, path, pkgName string, src []byte, m *PkgVisited) {
	fset := token.NewFileSet()
	node := Must(parser.ParseFile(fset, "", src, parser.AllErrors|parser.ParseComments))
	origPkgName := filepath.Base(path)
	pkgName = Or(pkgName, node.Name.Name)
	if err := env.DefinePkg(origPkgName, path); err != nil {
		panic(err)
	}
	if err := env.DefinePkg(pkgName, path); err != nil {
		panic(err)
	}
	loadAglImports(path, depth, t, nenv, nenv, node, m)
	loadDecls(env, nenv, node, path, pkgName, fset)
}

func loadDecls(env, nenv *Env, node *ast.File, path, pkgName string, fset *token.FileSet) {
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
			ft := funcDeclTypeToFuncType("", decl, nenv, fset, true)
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
			} else {
				ft.IsNative = true
			}
			var opts []SetTypeOption
			if decl.Doc != nil {
				doc := decl.Doc.List[0].Text
				r := regexp.MustCompile(`// ([^:]+):(\d+):(\d+)$`)
				if r.MatchString(doc) {
					goroot := runtime.GOROOT()
					parts := strings.Split(doc, ":")
					absPath, _ := filepath.Abs(filepath.Join(goroot, "src", path))
					absPath = filepath.Join(absPath, strings.TrimPrefix(parts[0], "// "))
					line, _ := strconv.Atoi(parts[1])
					character, _ := strconv.Atoi(parts[2])
					opts = append(opts, WithDefinition1(DefinitionProvider{URI: absPath, Line: line, Character: character}))
				}
			}
			if err := env.DefineFnNative2(fullName, ft, opts...); err != nil {
				assert(false, err.Error())
			}
		case *ast.GenDecl:
			for _, s := range decl.Specs {
				switch spec := s.(type) {
				case *ast.TypeSpec:
					specName := pkgName + "." + spec.Name.Name
					switch v := spec.Type.(type) {
					case *ast.Ident:
						t := nenv.GetType2(v, fset)
						env.Define(nil, specName, types.CustomType{Pkg: pkgName, Name: spec.Name.Name, W: t})
					case *ast.MapType:
						t := nenv.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.StarExpr:
						t := nenv.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.ArrayType:
						t := nenv.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.FuncType:
						t := nenv.GetType2(v, fset)
						env.Define(nil, specName, t)
					case *ast.InterfaceType:
						var methodsT []types.InterfaceMethod
						if v.Methods != nil {
							for _, m := range v.Methods.List {
								for _, n := range m.Names {
									fullName := pkgName + "." + spec.Name.Name + "." + n.Name
									t := nenv.GetType2(m.Type, fset)
									tmp := t.(types.FuncType)
									tmp.Name = n.Name
									env.Define(nil, fullName, tmp)
									methodsT = append(methodsT, types.InterfaceMethod{Name: n.Name, Typ: tmp})
								}
							}
						}
						env.Define(nil, specName, types.InterfaceType{Pkg: pkgName, Name: spec.Name.Name, Methods: methodsT})
					case *ast.StructType:
						env.Define(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name})
						if v.Fields != nil {
							for _, field := range v.Fields.List {
								t := nenv.GetType2(field.Type, fset)
								for _, name := range field.Names {
									fieldName := pkgName + "." + spec.Name.Name + "." + name.Name
									switch vv := t.(type) {
									case types.InterfaceType:
										env.Define(nil, fieldName, types.InterfaceType{Pkg: vv.Pkg, Name: vv.Name, Methods: vv.Methods})
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
						t := nenv.GetType2(spec.Type, fset)
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
	pkgName   string
	s         goast.Spec
	node      *goast.File
	fset      *gotoken.FileSet
	path      string
	entryName string
}

func defineStructsFromGoSrc(path string, depth int, t *TreeDrawer, files []EntryContent, env, nenv *Env, m *PkgVisited, keepRaw bool) {
	var tryLater []Later
	for _, entry := range files {
		env.withEnv(func(nenv *Env) {
			//p("LOADING", fullPath)
			fset := gotoken.NewFileSet()
			node := Must(goparser.ParseFile(fset, "", entry.Content, goparser.AllErrors|goparser.ParseComments))
			pkgName := node.Name.Name
			loadGoImports(filepath.Join(path, entry.Name), depth, t, env, nenv, node, m)
			for _, d := range node.Decls {
				switch decl := d.(type) {
				case *goast.GenDecl:
					for _, s := range decl.Specs {
						processSpec(path, entry.Name, node, fset, s, env, pkgName, &tryLater, keepRaw)
					}
				}
			}
		})
	}
	i := 0
	for len(tryLater) > 0 {
		var d Later
		d, tryLater = tryLater[0], tryLater[1:]
		processSpec(d.path, d.entryName, d.node, d.fset, d.s, env, d.pkgName, &tryLater, keepRaw)
		i++
		if i > 10 {
			break
		}
	}
}

func processSpec(path, entryName string, node *goast.File, fset *gotoken.FileSet, s goast.Spec, env *Env, pkgName string, tryLater *[]Later, keepRaw bool) {
	switch spec := s.(type) {
	//case *goast.ValueSpec:
	//	for _, name := range spec.Names {
	//		fieldName := pkgName + "." + name.Name
	//		t := env.GetGoType2(pkgName, spec.Type, keepRaw)
	//		p(fieldName, spec.Type, t)
	//		env.DefineForce(nil, fieldName, t)
	//	}
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
								*tryLater = append(*tryLater, Later{s: s, pkgName: pkgName, node: node, fset: fset, path: path, entryName: entryName})
								//p("DEFSTRUCT1", pkgName, spec.Name.Name)
								env.Define(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name, Fields: fields})
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
			//p(path, entryName, node, fset)
			absPath, _ := filepath.Abs(filepath.Join(path, entryName))
			pos := fset.Position(spec.Pos())
			env.Define(nil, specName, types.StructType{Pkg: pkgName, Name: spec.Name.Name, Fields: fields},
				WithDefinition1(DefinitionProvider{URI: absPath, Line: pos.Line, Character: pos.Column}))
		case *goast.InterfaceType:
			var methodsT []types.InterfaceMethod
			if v.Methods != nil {
				for _, m := range v.Methods.List {
					for _, n := range m.Names {
						if !n.IsExported() {
							continue
						}
						fullName := pkgName + "." + spec.Name.Name + "." + n.Name
						t := env.GetGoType2(pkgName, m.Type, keepRaw)
						env.Define(nil, fullName, t)
						methodsT = append(methodsT, types.InterfaceMethod{Name: n.Name, Typ: t})
					}
				}
			}
			env.Define(nil, specName, types.InterfaceType{Pkg: pkgName, Name: spec.Name.Name, Methods: methodsT})
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

func defineFromGoSrc(env, nenv *Env, path string, src []byte, keepRaw bool) {
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

func (e *Env) loadPkg(depth int, t *TreeDrawer, nenv *Env, pkgPath, pkgName string, m *PkgVisited) error {
	pkgName = Or(pkgName, filepath.Base(pkgPath))
	if m.ContainsAdd(pkgPath + "_" + pkgName + "_" + strconv.FormatInt(e.ID, 10)) {
		return nil
	}
	//p("?LOADPKG", pkgPath, pkgName)
	if err := e.loadPkgLocal(depth, t, nenv, pkgPath, pkgName, m); err != nil {
		if err := e.loadPkgAglStd(depth, t, nenv, pkgPath, pkgName, m); err != nil {
			if err := e.loadPkgVendor(depth, t, nenv, pkgPath, pkgName, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Env) loadPkgLocal(depth int, t *TreeDrawer, nenv *Env, pkgPath, pkgName string, m *PkgVisited) error {
	pkgFullPath := trimPrefixPath(pkgPath)
	if err := e.DefinePkg(pkgPath, pkgName); err != nil {
		return nil
	}
	if err := e.loadVendor2(depth, t, nenv, pkgFullPath, m); err != nil {
		return err
	}
	return nil
}

func (e *Env) loadPkgAglStd(depth int, t *TreeDrawer, nenv *Env, path, pkgName string, m *PkgVisited) error {
	var prefix string
	if strings.HasPrefix(path, "agl1/") {
		prefix = "agl1/"
	} else {
		prefix = "std/"
		if !strings.HasPrefix(path, "std/") {
			path = filepath.Join("std", path)
		}
	}
	//if path == "std/fmt" {
	//	goroot := runtime.GOROOT()
	//	absPath, _ := filepath.Abs(filepath.Join(goroot, "src", "fmt"))
	//	if err := e.loadVendor2(nenv, absPath, m); err != nil {
	//		return err
	//	}
	//}
	return e.loadAglFile(depth, t, nenv, prefix, path, pkgName, m)
}

func (e *Env) loadPkgVendor(depth int, t *TreeDrawer, nenv *Env, path, pkgName string, m *PkgVisited) error {
	vendorPath := filepath.Join("vendor", path)
	if err := e.loadVendor2(depth, t, nenv, vendorPath, m); err != nil {
		if err := e.loadAglFile(depth, t, nenv, "", path, pkgName, m); err != nil {
			return err
		}
	}
	return nil
}

func (e *Env) loadAglFile(depth int, t *TreeDrawer, nenv *Env, prefix, path, pkgName string, m *PkgVisited) error {
	stdFilePath := filepath.Join("pkgs", path, filepath.Base(path)+".agl")
	by, err := contentFs.ReadFile(stdFilePath)
	if err != nil {
		return err
	}
	final := filepath.Dir(strings.TrimPrefix(stdFilePath, "pkgs/"+prefix))
	defineFromSrc(depth, t, e, nenv, final, pkgName, by, m)
	return nil
}

func (e *Env) loadVendor2(depth int, t *TreeDrawer, nenv *Env, path string, m *PkgVisited) error {
	keepRaw := utils.True()
	files, err := readDir(path, m)
	if err != nil {
		return err
	}
	defineStructsFromGoSrc(path, depth, t, files, e, nenv, m, keepRaw)
	for _, entry := range files {
		defineFromGoSrc(e, nenv, path, entry.Content, keepRaw)
	}
	return nil
}

type EntryContent struct {
	Name    string
	Content []byte
}

// Read `*.go` files from a given directory
func readDir(path string, m *PkgVisited) ([]EntryContent, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
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
	return files, nil
}

func (e *Env) loadPkgAgl(m *PkgVisited) {
	_ = e.DefinePkg("agl1", "agl1")
	e.withEnv(func(nenv *Env) {
		_ = e.loadPkgAglStd(0, nil, nenv, "agl1/cmp", "", m)
		_ = e.loadPkgAglStd(0, nil, nenv, "agl1/iter", "", m)
		e.Define(nil, "Iterator", types.InterfaceType{Pkg: "agl1", Name: "Iterator", TypeParams: []types.Type{types.GenericType{Name: "T", W: types.AnyType{}}}})
		e.Define(nil, "agl1.Set", types.SetType{K: types.GenericType{Name: "T", W: types.AnyType{}}})
		e.Define(nil, "agl1.Vec", types.ArrayType{Elt: types.GenericType{Name: "T", W: types.AnyType{}}})
		e.DefineFn(nenv, "agl1.Set.Len", "func [T comparable](s agl1.Set[T]) int", WithDesc("The number of elements in the set."))
		e.DefineFn(nenv, "agl1.Set.Insert", "func [T comparable](mut s agl1.Set[T], el T) bool", WithDesc("Inserts the given element in the set if it is not already present."))
		e.DefineFn(nenv, "agl1.Set.Remove", "func [T comparable](mut s agl1.Set[T], el T) T?")
		e.DefineFn(nenv, "agl1.Set.Contains", "func [T comparable](s agl1.Set[T], el T) bool")
		e.DefineFn(nenv, "agl1.Set.Iter", "func [T comparable](s agl1.Set[T]) iter.Seq[T]")
		e.DefineFn(nenv, "agl1.Set.Union", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
		e.DefineFn(nenv, "agl1.Set.FormUnion", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
		e.DefineFn(nenv, "agl1.Set.Subtracting", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
		e.DefineFn(nenv, "agl1.Set.Subtract", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
		e.DefineFn(nenv, "agl1.Set.Intersection", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]", WithDesc("Returns a new set with the elements that are common to both this set and the given sequence."))
		e.DefineFn(nenv, "agl1.Set.FormIntersection", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
		e.DefineFn(nenv, "agl1.Set.SymmetricDifference", "func [T comparable](s agl1.Set[T], other Iterator[T]) agl1.Set[T]")
		e.DefineFn(nenv, "agl1.Set.FormSymmetricDifference", "func [T comparable](mut s agl1.Set[T], other Iterator[T])")
		e.DefineFn(nenv, "agl1.Set.IsSubset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
		e.DefineFn(nenv, "agl1.Set.IsStrictSubset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
		e.DefineFn(nenv, "agl1.Set.IsSuperset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
		e.DefineFn(nenv, "agl1.Set.IsStrictSuperset", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool")
		e.DefineFn(nenv, "agl1.Set.IsDisjoint", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool", WithDesc("Returns a Boolean value that indicates whether the set has no members in common with the given sequence."))
		e.DefineFn(nenv, "agl1.Set.Intersects", "func [T comparable](s agl1.Set[T], other Iterator[T]) bool", WithDesc("Returns a Boolean value that indicates whether the set has members in common with the given sequence."))
		e.DefineFn(nenv, "agl1.Set.Equals", "func [T comparable](s, other agl1.Set[T]) bool")
		e.DefineFn(nenv, "agl1.Set.Min", "func [T comparable](s agl1.Set[T]) T?")
		e.DefineFn(nenv, "agl1.Set.Max", "func [T comparable](s agl1.Set[T]) T?")
		e.DefineFn(nenv, "agl1.String.Split", "func (s, sep string) []string", WithDesc("Split slices s into all substrings separated by sep and returns a slice of\nthe substrings between those separators."))
		e.DefineFn(nenv, "agl1.String.Replace", "func (s, old, new string, n int) string")
		e.DefineFn(nenv, "agl1.String.ReplaceAll", "func (s, old, new string) string")
		e.DefineFn(nenv, "agl1.String.TrimPrefix", "func (s, prefix string) string")
		e.DefineFn(nenv, "agl1.String.TrimSpace", "func (s string) string")
		e.DefineFn(nenv, "agl1.String.HasPrefix", "func (s, prefix string) bool")
		e.DefineFn(nenv, "agl1.String.HasSuffix", "func (s, prefix string) bool")
		e.DefineFn(nenv, "agl1.String.Lowercased", "func (s string) string")
		e.DefineFn(nenv, "agl1.String.Uppercased", "func (s string) string")
		e.DefineFn(nenv, "agl1.String.AsBytes", "func (s string) []byte")
		e.DefineFn(nenv, "agl1.String.Int", "func (s) int?", WithDesc("Parse the string into an 'int' value.\nReturns None if the string cannot be parsed as an 'int'."))
		e.DefineFn(nenv, "agl1.String.I8", "func (s) i8?", WithDesc("Parse the string into an 'i8' value.\nReturns None if the string cannot be parsed as an 'i8'."))
		e.DefineFn(nenv, "agl1.String.I16", "func (s) i16?", WithDesc("Parse the string into an 'i16' value.\nReturns None if the string cannot be parsed as an 'i16'."))
		e.DefineFn(nenv, "agl1.String.I32", "func (s) i32?", WithDesc("Parse the string into an 'i32' value.\nReturns None if the string cannot be parsed as an 'i32'."))
		e.DefineFn(nenv, "agl1.String.I64", "func (s) i64?", WithDesc("Parse the string into an 'i64' value.\nReturns None if the string cannot be parsed as an 'i64'."))
		e.DefineFn(nenv, "agl1.String.Uint", "func (s) uint?", WithDesc("Parse the string into an 'uint' value.\nReturns None if the string cannot be parsed as an 'uint'."))
		e.DefineFn(nenv, "agl1.String.U8", "func (s) u8?", WithDesc("Parse the string into an 'u8' value.\nReturns None if the string cannot be parsed as an 'u8'."))
		e.DefineFn(nenv, "agl1.String.U16", "func (s) u16?", WithDesc("Parse the string into an 'u16' value.\nReturns None if the string cannot be parsed as an 'u16'."))
		e.DefineFn(nenv, "agl1.String.U32", "func (s) u32?", WithDesc("Parse the string into an 'u32' value.\nReturns None if the string cannot be parsed as an 'u32'."))
		e.DefineFn(nenv, "agl1.String.U64", "func (s) u64?", WithDesc("Parse the string into an 'u64' value.\nReturns None if the string cannot be parsed as an 'u64'."))
		e.DefineFn(nenv, "agl1.String.F32", "func (s) f32?", WithDesc("Parse the string into an 'f32' value.\nReturns None if the string cannot be parsed as an 'f32'."))
		e.DefineFn(nenv, "agl1.String.F64", "func (s) f64?", WithDesc("Parse the string into an 'f64' value.\nReturns None if the string cannot be parsed as an 'f64'."))
		e.DefineFn(nenv, "agl1.Vec.Iter", "func [T any](a []T) iter.Seq[T]")
		e.DefineFn(nenv, "agl1.Vec.Filter", "func [T any](a []T, f func(e T) bool) []T", WithDesc("Returns a new collection of the same type containing, in order, the elements of the original collection that satisfy the given predicate."))
		e.DefineFn(nenv, "agl1.Vec.AllSatisfy", "func [T any](a []T, f func(T) bool) bool", WithDesc("Returns a Boolean value indicating whether every element of a sequence satisfies a given predicate."))
		e.DefineFn(nenv, "agl1.Vec.Map", "func [T, R any](a []T, f func(T) R) []R", WithDesc("Returns an array containing the results of mapping the given closure over the sequence’s elements."))
		e.DefineFn(nenv, "agl1.Vec.Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R", WithDesc("Returns the result of combining the elements of the sequence using the given closure."))
		e.DefineFn(nenv, "agl1.Vec.Find", "func [T any](a []T, f func(e T) bool) T?")
		e.DefineFn(nenv, "agl1.Vec.Sum", "func [T cmp.Ordered](a []T) T")
		e.DefineFn(nenv, "agl1.Vec.Joined", "func (a []string) string", WithDesc("Returns the elements of this sequence of sequences, concatenated."))
		e.DefineFn(nenv, "agl1.Vec.Sorted", "func [E cmp.Ordered](a []E) []E", WithDesc("Returns the elements of the sequence, sorted."))
		e.DefineFn(nenv, "agl1.Vec.Get", "func [T any](a []T, i int) T?")
		e.DefineFn(nenv, "agl1.Vec.First", "func [T any](a []T) T?")
		e.DefineFn(nenv, "agl1.Vec.Last", "func [T any](a []T) T?")
		e.DefineFn(nenv, "agl1.Vec.Remove", "func [T any](a []T, i int)")
		e.DefineFn(nenv, "agl1.Vec.Clone", "func [T any](a []T) []T")
		e.DefineFn(nenv, "agl1.Vec.Indices", "func [T any](a []T) []int")
		e.DefineFn(nenv, "agl1.Vec.Contains", "func [T comparable](a []T, e T) bool")
		e.DefineFn(nenv, "agl1.Vec.Any", "func [T any](a []T, f func(T) bool) bool")
		e.DefineFn(nenv, "agl1.Vec.Push", "func [T any](mut a []T, els ...T)")
		e.DefineFn(nenv, "agl1.Vec.PushFront", "func [T any](mut a []T, el T)")
		e.DefineFn(nenv, "agl1.Vec.Pop", "func [T any](mut a []T) T?")
		e.DefineFn(nenv, "agl1.Vec.PopFront", "func [T any](mut a []T) T?")
		e.DefineFn(nenv, "agl1.Vec.PopIf", "func [T any](mut a []T, pred func() bool) T?")
		e.DefineFn(nenv, "agl1.Vec.Insert", "func [T any](mut a []T, idx int, el T)")
		e.DefineFn(nenv, "agl1.Vec.Len", "func [T any](a []T) int")
		e.DefineFn(nenv, "agl1.Vec.IsEmpty", "func [T any](a []T) bool")
		e.DefineFn(nenv, "agl1.Map.ContainsKey", "func [K comparable, V any](m map[K]V, k K) bool")
		e.DefineFn(nenv, "agl1.Map.Len", "func [K comparable, V any]() int")
		e.DefineFn(nenv, "agl1.Map.Get", "func [K comparable, V any](m map[K]V) V?")
		e.DefineFn(nenv, "agl1.Map.Keys", "func [K comparable, V any](m map[K]V) iter.Seq[K]")
		e.DefineFn(nenv, "agl1.Map.Values", "func [K comparable, V any](m map[K]V) iter.Seq[V]")
		e.DefineFn(nenv, "agl1.Option.Unwrap", "func [T any]() T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or panic if None."))
		e.DefineFn(nenv, "agl1.Option.UnwrapOr", "func [T any](t T) T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or a default if None."))
		e.DefineFn(nenv, "agl1.Option.UnwrapOrDefault", "func [T any]() T", WithDesc("Unwraps an Option value, yielding the content of a Some(x), or the default if None."))
		e.DefineFn(nenv, "agl1.Option.IsSome", "func () bool")
		e.DefineFn(nenv, "agl1.Option.IsNone", "func () bool")
		e.DefineFn(nenv, "agl1.Result.Unwrap", "func [T any]() T")
		e.DefineFn(nenv, "agl1.Result.UnwrapOr", "func [T any](t T) T")
		e.DefineFn(nenv, "agl1.Result.UnwrapOrDefault", "func [T any]() T")
		e.DefineFn(nenv, "agl1.Result.IsOk", "func () bool")
		e.DefineFn(nenv, "agl1.Result.IsErr", "func () bool")
	})
}

func CoreFns() string {
	return string(Must(contentFs.ReadFile(filepath.Join("core", "core.agl"))))
}

func (e *Env) loadBaseValues() {
	m := NewPkgVisited()
	e.loadCoreTypes()
	e.loadCoreFunctions(m)
	e.loadPkgAgl(m)
	e.Define(nil, "Option", types.OptionType{})
	e.Define(nil, "comparable", types.TypeType{W: types.CustomType{Name: "comparable", W: types.AnyType{}}})
}

func NewEnv() *Env {
	env := &Env{
		ID:          envIDCounter.Add(1),
		lookupTable: make(map[string]*Info),
		lspTable:    make(map[NodeKey]*Info),
	}
	env.loadBaseValues()
	return env
}

func (e *Env) SubEnv() *Env {
	env := &Env{
		ID:          envIDCounter.Add(1),
		lookupTable: make(map[string]*Info),
		lspTable:    e.lspTable,
		parent:      e,
		NoIdxUnwrap: e.NoIdxUnwrap,
	}
	//p("SubEnv", e.ID, env.ID)
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

func (e *Env) DefineFn(nenv *Env, name string, fnStr string, opts ...SetTypeOption) {
	fnT := parseFuncTypeFromString(name, fnStr, nenv, nil)
	e.Define(nil, name, fnT, opts...)
}

func (e *Env) DefineFnNative(name string, fnStr string, fset *token.FileSet) {
	fnT := parseFuncDeclFromStringHelper(name, fnStr, e, fset)
	e.Define(nil, name, fnT)
}

func (e *Env) DefineFnNative2(name string, fnT types.FuncType, opts ...SetTypeOption) error {
	return e.Define1(nil, name, fnT, opts...)
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

type DefinitionProvider struct {
	URI             string
	Line, Character int
}

func (e *Env) defineHelper(n ast.Node, name string, typ types.Type, force bool, opts ...SetTypeOption) error {
	if name == "_" {
		return nil
	}
	//p("DEF", name, typ, e.ID)
	conf := &SetTypeConf{}
	for _, opt := range opts {
		opt(conf)
	}
	if !force {
		t := e.GetDirect(name)
		if t != nil {
			//p("?", name, t, e.ID)
			//return fmt.Errorf("duplicate declaration of %s", name)
		}
	}
	info := e.GetOrCreateNameInfo(name)
	info.Type = typ
	if conf.definition1 != nil {
		info.Definition1 = *conf.definition1
	}
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

func (e *Env) SetType(p *Info, def1 *DefinitionProvider, x ast.Node, t types.Type, fset *token.FileSet) {
	assertf(t != nil, "%s: try to set type nil, %v %v", fset.Position(x.Pos()), x, to(x))
	info := e.lspNodeOrCreate(x)
	info.Type = t
	if def1 != nil {
		info.Definition1 = *def1
	}
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
	switch xx := x.(type) {
	case *goast.Ident:
		if v2 := e.GetNameInfo(xx.Name); v2 != nil {
			return v2.Type
		}
		if v2 := e.GetNameInfo(pkgName + "." + xx.Name); v2 != nil {
			return v2.Type
		}
		return nil
	case *goast.FuncType:
		return goFuncTypeToFuncType("", pkgName, xx, e, true)
	case *goast.Ellipsis:
		t := e.GetGoType2(pkgName, xx.Elt, keepRaw)
		if t == nil {
			return nil
		}
		return types.EllipsisType{Elt: t}
	case *goast.ArrayType:
		t := e.GetGoType2(pkgName, xx.Elt, keepRaw)
		if t == nil {
			return nil
		}
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
