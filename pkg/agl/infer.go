package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	goast "go/ast"
	"go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"log"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

type Inferrer struct {
	fset *token.FileSet
	Env  *Env
}

func NewInferrer(fset *token.FileSet, env *Env) *Inferrer {
	return &Inferrer{fset: fset, Env: env}
}

func (infer *Inferrer) InferFile(f *ast.File) {
	fileInferrer := &FileInferrer{env: infer.Env, f: f, fset: infer.fset}
	fileInferrer.Infer()
}

type FileInferrer struct {
	env             *Env
	f               *ast.File
	fset            *token.FileSet
	PackageName     string
	returnType      types.Type
	optType         *OptTypeTmp
	forceReturnType types.Type
	mapKT, mapVT    types.Type
}

type OptTypeTmp struct {
	Pos  token.Pos
	Type types.Type
}

func (o *OptTypeTmp) IsDefinedFor(n ast.Node) bool {
	return o != nil && o.Type != nil && o.Pos == n.Pos()
}

func (infer *FileInferrer) sandboxed(clb func()) {
	old := infer.env
	nenv := old.SubEnv()
	infer.env = nenv
	clb()
	infer.env = old
}

func (infer *FileInferrer) withMapKV(k, v types.Type, clb func()) {
	oldMapK, oldMapV := infer.mapKT, infer.mapVT
	infer.mapKT, infer.mapVT = k, v
	clb()
	infer.mapKT, infer.mapVT = oldMapK, oldMapV
}

func (infer *FileInferrer) withOptType(n ast.Node, t types.Type, clb func()) {
	prev := infer.optType
	infer.optType = &OptTypeTmp{Type: t, Pos: n.Pos()}
	clb()
	infer.optType = prev
}

func (infer *FileInferrer) withForceReturn(t types.Type, clb func()) {
	prev := infer.forceReturnType
	infer.forceReturnType = t
	clb()
	infer.forceReturnType = prev
}

func (infer *FileInferrer) withReturnType(t types.Type, clb func()) {
	prev := infer.returnType
	infer.returnType = t
	clb()
	infer.returnType = prev
}

func (infer *FileInferrer) withEnv(clb func()) {
	old := infer.env
	nenv := old.SubEnv()
	infer.env = nenv
	clb()
	infer.env = old
}

func (infer *FileInferrer) GetTypeFn(n ast.Node) types.FuncType {
	return infer.GetType(n).(types.FuncType)
}

func (infer *FileInferrer) GetType(n ast.Node) types.Type {
	t := infer.env.GetType(n)
	if t == nil {
		panic(fmt.Sprintf("%s type not found for %v %v", infer.Pos(n), n, to(n)))
	}
	return t
}

func (infer *FileInferrer) SetTypeForce(a ast.Node, t types.Type) {
	infer.env.SetType(nil, a, t)
}

type SetTypeConf struct {
	definition *Info
}

type SetTypeOption func(*SetTypeConf)

func WithDefinition(i *Info) SetTypeOption {
	return func(o *SetTypeConf) {
		o.definition = i
	}
}

func (infer *FileInferrer) SetType(a ast.Node, t types.Type, opts ...SetTypeOption) {
	conf := &SetTypeConf{}
	for _, opt := range opts {
		opt(conf)
	}
	if tt := infer.env.GetType(a); tt != nil {
		if !cmpTypes(tt, t) {
			if !TryCast[types.UntypedNumType](tt) {
				panic(fmt.Sprintf("type already declared for %s %s %v %v %v %v", infer.Pos(a), makeKey(a), a, to(a), infer.env.GetType(a), t))
			}
		}
	}
	infer.env.SetType(conf.definition, a, t)
}

func trimPrefixPath(s string) string {
	sep := string(os.PathSeparator)
	parts := strings.Split(s, sep)
	if len(parts) <= 1 {
		return s
	}
	return strings.Join(parts[1:], sep)
}

// formatFieldList formats a parameter or result list into Go-style signature string
func formatFieldList(fl *goast.FieldList) string {
	if fl == nil {
		return "()"
	}
	out := "("
	var tmp1 []string
	for _, field := range fl.List {
		var tmp []string
		for range field.Names {
			tmp = append(tmp, exprToString(field.Type))
		}
		if len(field.Names) == 0 {
			tmp = append(tmp, exprToString(field.Type))
		}
		tmp1 = append(tmp1, strings.Join(tmp, ", "))
	}
	out += strings.Join(tmp1, ", ")
	out += ")"
	return out
}

// exprToString returns a basic string representation of an expression (e.g., type)
func exprToString(expr goast.Expr) string {
	switch t := expr.(type) {
	case *goast.Ident:
		return t.Name
	case *goast.StarExpr:
		return "*" + exprToString(t.X)
	case *goast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *goast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *goast.Ellipsis:
		return "..." + exprToString(t.Elt)
	case *goast.FuncType:
		return "func" + formatFieldList(t.Params)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func (infer *FileInferrer) Infer() {
	infer.PackageName = infer.f.Name.Name
	infer.SetType(infer.f.Name, types.PackageType{Name: infer.f.Name.Name})
	for _, i := range infer.f.Imports {
		infer.inferImport(i)
	}
	// TODO do a second pass for types that used before their declaration
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *ast.GenDecl:
			infer.genDecl(decl)
		}
	}
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			infer.funcDecl(decl)
		}
	}
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			infer.funcDecl2(decl)
		}
	}
}

func (infer *FileInferrer) inferImport(i *ast.ImportSpec) {
	var pkgName string
	if i.Name != nil {
		pkgName = i.Name.Name
	} else {
		pkgName = path.Base(i.Path.Value)
	}
	pkgPath := strings.ReplaceAll(i.Path.Value, `"`, ``)
	pkgName = strings.ReplaceAll(pkgName, `"`, ``)
	pkgT := infer.env.Get(pkgName)
	if pkgT == nil {
		infer.env.Define(nil, pkgName, types.PackageType{Name: pkgName, Path: pkgPath})
		pkgFullPath := trimPrefixPath(pkgPath)
		entries, err := os.ReadDir(pkgFullPath)
		if err != nil {
			//goroot := runtime.GOROOT()
			//pkgFullPath = filepath.Join(goroot, "src", pkgPath)
			//entries, err = os.ReadDir(pkgFullPath)
			//if err != nil {
			log.Fatalf("failed to laod package %v\n", pkgPath)
			//}
		}
		for _, e := range entries {
			fName := e.Name()
			if strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			fPath := filepath.Join(pkgFullPath, fName)
			src, err := os.ReadFile(fPath)
			if err != nil {
				log.Printf("failed to load %s\n", fPath)
				continue
			}
			fset := gotoken.NewFileSet()
			node, err := parser.ParseFile(fset, fName, src, parser.AllErrors)
			if err != nil {
				log.Printf("failed to parse %s\n", fPath)
				continue
			}
			conf := gotypes.Config{Importer: nil}
			info := &gotypes.Info{Defs: make(map[*goast.Ident]gotypes.Object)}
			_, _ = conf.Check("", fset, []*goast.File{node}, info)
			for _, decl := range node.Decls {
				if fn, ok := decl.(*goast.FuncDecl); ok {
					if fn.Recv != nil {
						continue
					}
					fnStr := "func " + formatFieldList(fn.Type.Params)
					if fn.Type.Results != nil {
						fnStr += " " + formatFieldList(fn.Type.Results)
					}
					infer.env.DefineFnNative(pkgName+"."+fn.Name.Name, fnStr)
				}
			}
		}
	}
}

func (infer *FileInferrer) genDecl(decl *ast.GenDecl) {
	for _, s := range decl.Specs {
		switch spec := s.(type) {
		case *ast.TypeSpec:
			infer.typeSpec(spec)
		case *ast.ImportSpec:
		case *ast.ValueSpec:
			infer.valueSpec(spec)
		default:
			panic(fmt.Sprintf("%v", to(spec)))
		}
	}
}

func (infer *FileInferrer) valueSpec(spec *ast.ValueSpec) {
	var t types.Type
	if spec.Values != nil {
		t = infer.env.GetType2(spec.Values[0])
	}
	for _, name := range spec.Names {
		noop(name)
		infer.env.Define(name, name.Name, t)
	}
}

func (infer *FileInferrer) typeSpec(spec *ast.TypeSpec) {
	var toDef types.Type
	switch t := spec.Type.(type) {
	case *ast.Ident:
		typ := infer.env.GetType2(t)
		assertf(typ != nil, "%s: type not found '%s'", infer.Pos(spec.Name), t)
		toDef = types.TypeType{W: types.CustomType{Name: spec.Name.Name, W: typ}}
	case *ast.StructType:
		var fields []types.FieldType
		if t.Fields != nil {
			for _, f := range t.Fields.List {
				typ := infer.env.GetType2(f.Type)
				for _, n := range f.Names {
					fields = append(fields, types.FieldType{Name: n.Name, Typ: typ})
				}
			}
		}
		structT := types.StructType{Name: spec.Name.Name, Fields: fields}
		var toDef1 types.Type
		if spec.TypeParams != nil {
			var tpFields []types.FieldType
			for _, typeParam := range spec.TypeParams.List {
				for _, n := range typeParam.Names {
					typ := infer.env.GetType2(typeParam.Type)
					tpFields = append(tpFields, types.FieldType{Name: n.Name, Typ: typ})
				}
			}
			if len(tpFields) > 1 {
				toDef1 = types.IndexListType{X: structT, Indices: tpFields}
			} else {
				toDef1 = types.IndexType{X: structT, Index: tpFields}
			}
		} else {
			toDef1 = structT
		}
		toDef = toDef1
	case *ast.EnumType:
		var fields []types.EnumFieldType
		if t.Values != nil {
			for _, f := range t.Values.List {
				var elts []types.Type
				if f.Params != nil {
					for _, param := range f.Params.List {
						elts = append(elts, infer.env.GetType2(param.Type))
					}
				}
				fields = append(fields, types.EnumFieldType{Name: f.Name.Name, Elts: elts})
			}
		}
		toDef = types.EnumType{Name: spec.Name.Name, Fields: fields}
	case *ast.InterfaceType:
		if t.Methods.List != nil {
			for _, f := range t.Methods.List {
				if f.Type != nil {
					infer.expr(f.Type)
				}
				for _, n := range f.Names {
					fnT := funcTypeToFuncType("", f.Type.(*ast.FuncType), infer.env, false)
					infer.env.Define(spec.Name, spec.Name.Name+"."+n.Name, fnT)
				}
			}
		}
		toDef = types.InterfaceType{Name: spec.Name.Name}
	case *ast.ArrayType:
		toDef = types.CustomType{Name: spec.Name.Name, W: types.ArrayType{Elt: infer.env.GetType2(t.Elt)}}
	case *ast.MapType:
		kT := infer.env.GetType2(t.Key)
		vT := infer.env.GetType2(t.Value)
		mT := types.MapType{K: kT, V: vT}
		toDef = types.CustomType{Name: spec.Name.Name, W: mT}
	default:
		panic(fmt.Sprintf("%v", to(spec.Type)))
	}
	infer.env.Define(spec.Name, spec.Name.Name, toDef)
}

func (infer *FileInferrer) structType(name *ast.Ident, s *ast.StructType) {
	var fields []types.FieldType
	if s.Fields != nil {
		for _, f := range s.Fields.List {
			t := infer.env.GetType2(f.Type)
			for _, n := range f.Names {
				fields = append(fields, types.FieldType{Name: n.Name, Typ: t})
			}
		}
	}
	infer.env.Define(name, name.Name, types.StructType{Name: name.Name, Fields: fields})
}

func (infer *FileInferrer) funcDecl(decl *ast.FuncDecl) {
	var t types.FuncType
	outEnv := infer.env
	infer.sandboxed(func() {
		t = infer.getFuncDeclType(decl, outEnv)
	})
	infer.SetType(decl, t)
	fnName := decl.Name.Name
	if newName, ok := overloadMapping[fnName]; ok {
		fnName = newName
	}
	if decl.Recv != nil {
		t1 := decl.Recv.List[0].Type
		if v, ok := t1.(*ast.StarExpr); ok {
			t1 = v.X
		}
		recvTStr := infer.env.GetType2(t1).GoStr()
		fnName = recvTStr + "." + fnName
	}
	infer.env.Define(decl.Name, fnName, t)
	infer.SetType(decl.Name, t)
	infer.SetType(decl, t)
}

// mapping of "agl function name" to "go compiled function name"
var overloadMapping = map[string]string{
	"==": "__EQL",
	"!=": "__EQL",
	"+":  "__ADD",
	"-":  "__SUB",
	"*":  "__MUL",
	"/":  "__QUO",
	"%":  "__REM",
}

func (infer *FileInferrer) funcDecl2(decl *ast.FuncDecl) {
	infer.withEnv(func() {
		if decl.Recv != nil {
			for _, recv := range decl.Recv.List {
				infer.env.NoIdxUnwrap = true
				t := infer.env.GetType2(recv.Type)
				infer.env.NoIdxUnwrap = false
				for _, name := range recv.Names {
					infer.env.SetType(nil, name, t)
					infer.env.Define(name, name.Name, t)
				}
			}
		}
		if decl.Type.TypeParams != nil {
			for _, param := range decl.Type.TypeParams.List {
				infer.expr(param.Type)
				t := infer.env.GetType2(param.Type)
				for _, name := range param.Names {
					infer.env.SetType(nil, name, t)
					infer.env.Define(name, name.Name, types.GenericType{Name: name.Name, W: t, IsType: true})
				}
			}
		}
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				infer.expr(param.Type)
				t := infer.env.GetType2(param.Type)
				for _, name := range param.Names {
					infer.env.Define(name, name.Name, t)
					infer.env.SetType(nil, name, t)
				}
			}
		}

		var returnTyp types.Type = types.VoidType{}
		if decl.Type.Result != nil {
			infer.expr(decl.Type.Result)
			returnTyp = infer.env.GetType2(decl.Type.Result)
			infer.SetType(decl.Type.Result, returnTyp)
		}
		infer.withReturnType(returnTyp, func() {
			if decl.Body != nil {
				// implicit return
				cond1 := len(decl.Body.List) == 1 ||
					(len(decl.Body.List) == 2 && TryCast[*ast.EmptyStmt](decl.Body.List[1]))
				if cond1 && decl.Type.Result != nil && TryCast[*ast.ExprStmt](decl.Body.List[0]) {
					decl.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: decl.Body.List[0].(*ast.ExprStmt).X}}
				}
				infer.stmt(decl.Body)
			}
		})
	})
}

func (infer *FileInferrer) getFuncDeclType(decl *ast.FuncDecl, outEnv *Env) types.FuncType {
	var returnT types.Type
	var paramsT, typeParamsT []types.Type
	var vecExt bool
	if decl.Recv != nil {
		if len(decl.Recv.List) == 1 {
			if tmp, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if sel, ok := tmp.X.(*ast.SelectorExpr); ok {
					p1 := sel.X.(*ast.Ident).Name
					if p1 == "agl" && sel.Sel.Name == "Vec" {
						defaultName := "T" // Should not hardcode "T"
						vecExt = true
						id := tmp.Index.(*ast.Ident)
						typeName := Ternary(id.Name == defaultName, "any", id.Name)
						t := &ast.Field{Names: []*ast.Ident{{Name: defaultName}}, Type: &ast.Ident{Name: typeName}}
						if decl.Type.TypeParams == nil {
							decl.Type.TypeParams = &ast.FieldList{List: []*ast.Field{t}}
						} else {
							decl.Type.TypeParams.List = append([]*ast.Field{t}, decl.Type.TypeParams.List...)
						}
					}
				}
			}
		}
	}
	if decl.Type.TypeParams != nil {
		for _, typeParam := range decl.Type.TypeParams.List {
			infer.expr(typeParam.Type)
			t := infer.env.GetType(typeParam.Type)
			for _, name := range typeParam.Names {
				tt := types.GenericType{Name: name.Name, W: t, IsType: true}
				typeParamsT = append(typeParamsT, tt)
				infer.env.Define(name, name.Name, tt)
			}
		}
	}
	if decl.Type.Params != nil {
		for _, param := range decl.Type.Params.List {
			infer.expr(param.Type)
			t := infer.env.GetType2(param.Type)
			for range param.Names {
				paramsT = append(paramsT, t)
			}
		}
	}
	if decl.Type.Result != nil {
		infer.expr(decl.Type.Result)
		returnT = infer.env.GetType2(decl.Type.Result)
		switch r := returnT.(type) {
		case types.ResultType:
			r.Bubble = true
			returnT = r
		case types.OptionType:
			r.Bubble = true
			returnT = r
		}
	}
	if returnT == nil {
		returnT = types.VoidType{}
	}
	fnName := decl.Name.Name
	if newName, ok := overloadMapping[fnName]; ok {
		fnName = newName
	}
	ft := types.FuncType{
		Name:       fnName,
		TypeParams: typeParamsT,
		Params:     paramsT,
		Return:     returnT,
	}
	if decl.Recv != nil {
		if vecExt {
			outEnv.Define(decl.Name, fmt.Sprintf("agl.Vec.%s", fnName), ft)
		}
	}
	return ft
}

func (infer *FileInferrer) stmts(s []ast.Stmt) {
	for _, stmt := range s {
		infer.stmt(stmt)
	}
}

func (infer *FileInferrer) exprs(s []ast.Expr) {
	for _, expr := range s {
		infer.expr(expr)
	}
}

func (infer *FileInferrer) exprType(e ast.Expr) {

}

func (infer *FileInferrer) expr(e ast.Expr) {
	//p("infer.expr", to(e))
	switch expr := e.(type) {
	case *ast.Ident:
		t := infer.identExpr(expr)
		infer.SetType(expr, t)
	case *ast.CallExpr:
		infer.callExpr(expr)
	case *ast.BinaryExpr:
		infer.binaryExpr(expr)
	case *ast.OptionExpr:
		infer.optionExpr(expr)
	case *ast.ResultExpr:
		infer.resultExpr(expr)
	case *ast.IndexExpr:
		infer.indexExpr(expr)
	case *ast.ArrayType:
		infer.arrayType(expr)
	case *ast.FuncType:
		infer.funcType(expr)
	case *ast.BasicLit:
		infer.basicLit(expr)
	case *ast.ShortFuncLit:
		infer.shortFuncLit(expr)
	case *ast.CompositeLit:
		infer.compositeLit(expr)
	case *ast.BubbleOptionExpr:
		infer.bubbleOptionExpr(expr)
	case *ast.BubbleResultExpr:
		infer.bubbleResultExpr(expr)
	case *ast.SelectorExpr:
		infer.selectorExpr(expr)
	case *ast.FuncLit:
		infer.funcLit(expr)
	case *ast.TupleExpr:
		infer.tupleExpr(expr)
	case *ast.Ellipsis:
		infer.ellipsis(expr)
	case *ast.VoidExpr:
		infer.voidExpr(expr)
	case *ast.StarExpr:
		infer.starExpr(expr)
	case *ast.SomeExpr:
		infer.someExpr(expr)
	case *ast.OkExpr:
		infer.okExpr(expr)
	case *ast.ErrExpr:
		infer.errExpr(expr)
	case *ast.NoneExpr:
		infer.noneExpr(expr)
	case *ast.ChanType:
		infer.chanType(expr)
	case *ast.UnaryExpr:
		infer.unaryExpr(expr)
	case *ast.TypeAssertExpr:
		infer.typeAssertExpr(expr)
	case *ast.MapType:
		infer.mapType(expr)
	case *ast.OrBreakExpr:
		infer.orBreak(expr)
	case *ast.OrContinueExpr:
		infer.orContinue(expr)
	case *ast.OrReturnExpr:
		infer.orReturn(expr)
	case *ast.IndexListExpr:
		infer.indexListExpr(expr)
	case *ast.KeyValueExpr:
		infer.keyValueExpr(expr)
	case *ast.InterfaceType:
		infer.interfaceType(expr)
	case *ast.SliceExpr:
		infer.sliceExpr(expr)
	case *ast.DumpExpr:
		infer.dumpExpr(expr)
	default:
		panic(fmt.Sprintf("unknown expression %v", to(e)))
	}
	if infer.optType.IsDefinedFor(e) {
		infer.tryConvertType(e, infer.optType.Type)
	}
}

func isIntType(t types.Type) bool {
	return TryCast[types.I64Type](t) ||
		TryCast[types.I32Type](t) ||
		TryCast[types.I16Type](t) ||
		TryCast[types.I8Type](t) ||
		TryCast[types.IntType](t) ||
		TryCast[types.U64Type](t) ||
		TryCast[types.U32Type](t) ||
		TryCast[types.U16Type](t) ||
		TryCast[types.U8Type](t) ||
		TryCast[types.IntType](t) ||
		TryCast[types.UintType](t)
}

func isNumericType(t types.Type) bool {
	return isIntType(t) ||
		TryCast[types.F64Type](t) ||
		TryCast[types.F32Type](t)
}

func (infer *FileInferrer) tryConvertType(e ast.Expr, optType types.Type) {
	if infer.env.GetType(e) == nil {
		infer.SetType(e, optType)
	} else if _, ok := infer.GetType(e).(types.UntypedNumType); ok {
		if isIntType(optType) {
			infer.SetType(e, optType)
		}
	}
}

func (infer *FileInferrer) stmt(s ast.Stmt) {
	//p("infer.stmt", to(s))
	switch stmt := s.(type) {
	case *ast.BlockStmt:
		infer.blockStmt(stmt)
	case *ast.IfStmt:
		infer.ifStmt(stmt)
	case *ast.ReturnStmt:
		infer.returnStmt(stmt)
	case *ast.ExprStmt:
		infer.exprStmt(stmt)
	case *ast.AssignStmt:
		infer.assignStmt(stmt)
	case *ast.RangeStmt:
		infer.rangeStmt(stmt)
	case *ast.IncDecStmt:
		infer.incDecStmt(stmt)
	case *ast.DeclStmt:
		infer.declStmt(stmt)
	case *ast.ForStmt:
		infer.forStmt(stmt)
	case *ast.IfLetStmt:
		infer.ifLetStmt(stmt)
	case *ast.SendStmt:
		infer.sendStmt(stmt)
	case *ast.SelectStmt:
		infer.selectStmt(stmt)
	case *ast.CommClause:
		infer.commClause(stmt)
	case *ast.SwitchStmt:
		infer.switchStmt(stmt)
	case *ast.CaseClause:
		infer.caseClause(stmt)
	case *ast.LabeledStmt:
		infer.labeledStmt(stmt)
	case *ast.BranchStmt:
		infer.branchStmt(stmt)
	case *ast.DeferStmt:
		infer.deferStmt(stmt)
	case *ast.GoStmt:
		infer.goStmt(stmt)
	case *ast.TypeSwitchStmt:
		infer.typeSwitchStmt(stmt)
	case *ast.EmptyStmt:
		infer.emptyStmt(stmt)
	case *ast.MatchStmt:
		infer.matchStmt(stmt)
	case *ast.MatchClause:
		infer.matchClause(stmt)
	default:
		panic(fmt.Sprintf("unknown statement %v", to(stmt)))
	}
}

func (infer *FileInferrer) basicLit(expr *ast.BasicLit) {
	if infer.env.GetType(expr) != nil {
		return
	}
	switch expr.Kind {
	case token.STRING:
		infer.SetType(expr, types.StringType{})
	case token.INT:
		infer.SetType(expr, types.UntypedNumType{})
		if infer.optType.IsDefinedFor(expr) {
			infer.SetType(expr, infer.optType.Type)
		} else {
			infer.SetType(expr, types.UntypedNumType{})
		}
	case token.CHAR:
		infer.SetType(expr, types.CharType{})
	default:
		panic(fmt.Sprintf("unknown basic literal %v %v", to(expr), expr.Kind))
	}
}

func (infer *FileInferrer) getSelectorType(e ast.Expr, id *ast.Ident) types.Type {
	eTRaw := infer.env.GetType2(e)
	if v, ok := eTRaw.(types.StarType); ok {
		eTRaw = v.X
	}
	switch eT := eTRaw.(type) {
	case types.StructType:
		structPkg := eT.Pkg
		structName := eT.Name
		name := structName + "." + id.Name
		if structPkg != "" {
			name = structPkg + "." + name
		}
		return infer.env.Get(name)
	default:
		panic("")
	}
}

func (infer *FileInferrer) callExpr(expr *ast.CallExpr) {
	switch call := expr.Fun.(type) {
	case *ast.SelectorExpr:
		var exprFunT types.Type
		var callXParent *Info
		switch callXT := call.X.(type) {
		case *ast.Ident:
			exprFunT = infer.env.Get(callXT.Name)
			callXParent = infer.env.GetNameInfo(callXT.Name)
		case *ast.CallExpr, *ast.BubbleResultExpr, *ast.BubbleOptionExpr:
			infer.expr(callXT)
			exprFunT = infer.GetType(callXT)
		case *ast.SelectorExpr:
			infer.expr(callXT.X)
			if callXTXT := infer.env.GetType(callXT.X); callXTXT != nil {
				if v, ok := callXTXT.(types.StarType); ok {
					callXTXT = v.X
				}
				if v, ok := callXTXT.(types.StructType); ok {
					var t types.Type
					if f := Find(v.Fields, func(ft types.FieldType) bool { return ft.Name == callXT.Sel.Name }); f != nil {
						t = f.Typ
					} else {
						n := fmt.Sprintf("%s.%s", v.Name, callXT.Sel)
						if v.Pkg != "" {
							n = v.Pkg + "." + n
						}
						t = infer.env.Get(n)
					}
					infer.SetType(callXT.Sel, t)
					exprFunT = t
				}
			} else {
				//infer.SetType(callXT.X, )
				exprFunT = infer.getSelectorType(callXT.X, callXT.Sel)
			}
		case *ast.IndexExpr:
			exprFunT = infer.env.GetType2(callXT)
		case *ast.TypeAssertExpr:
			exprFunT = types.OptionType{W: infer.env.GetType2(callXT)}
		default:
			panic(fmt.Sprintf("%v %v", call.X, to(call.X)))
		}
		if starT, ok := exprFunT.(types.StarType); ok {
			exprFunT = starT.X
		}
		fnName := call.Sel.Name
		switch idTT := exprFunT.(type) {
		case types.ArrayType:
		case types.MapType:
		case types.CustomType:
			name := fmt.Sprintf("%s.%s", idTT.Name, fnName)
			t := infer.env.Get(name)
			tr := t.(types.FuncType).Return
			infer.SetType(call.Sel, t)
			infer.SetType(call, tr)
			infer.SetType(expr, tr)
		case types.SetType:
			fnT := infer.env.GetFn("agl.Set."+call.Sel.Name).T("T", idTT.Elt)
			fnT.Recv = []types.Type{idTT}
			fnT.Params = fnT.Params[1:]
			infer.SetType(expr, fnT.Return)
			infer.SetType(call.Sel, fnT)
		case types.StructType:
			name := fmt.Sprintf("%s.%s", idTT.Name, call.Sel.Name)
			if idTT.Pkg != "" {
				name = idTT.Pkg + "." + name
			}
			nameT := infer.env.Get(name)
			assertf(nameT != nil, "%s: method not found '%s' in struct of type '%v'", infer.Pos(call.Sel), call.Sel.Name, idTT.Name)
			fnT := infer.env.GetFn(name)
			toReturn := fnT.Return
			toReturn = alterResultBubble(infer.returnType, toReturn)
			infer.SetType(call.Sel, fnT)
			infer.SetType(expr, toReturn)
		case types.InterfaceType:
			name := fmt.Sprintf("%s.%s", idTT.Name, fnName)
			if idTT.Pkg != "" {
				name = idTT.Pkg + "." + name
			}
			t := infer.env.Get(name)
			tr := t.(types.FuncType).Return
			infer.SetType(call.Sel, t)
			infer.SetType(call, tr)
			infer.SetType(expr, tr)
		case types.EnumType:
			infer.SetType(expr, types.EnumType{Name: idTT.Name, SubTyp: call.Sel.Name, Fields: idTT.Fields})
		case types.PackageType:
			pkgT := infer.env.Get(idTT.Name)
			assertf(pkgT != nil, "package not found '%s'", idTT.Name)
			name := fmt.Sprintf("%s.%s", idTT.Name, call.Sel.Name)
			nameT := infer.env.Get(name)
			assertf(nameT != nil, "%s: not found '%s' in package '%v'", infer.Pos(call.Sel), call.Sel.Name, idTT.Name)
			fnT := nameT.(types.FuncType)
			toReturn := fnT.Return
			if toReturn != nil {
				toReturn = alterResultBubble(infer.returnType, toReturn)
			}
			infer.SetType(call.Sel, fnT)
			infer.SetType(expr.Fun, fnT)
			if toReturn != nil {
				infer.SetType(expr, toReturn)
			}
		case types.OptionType:
			assertf(InArray(fnName, []string{"IsNone", "IsSome", "Unwrap", "UnwrapOr"}),
				"Unresolved reference '%s'", fnName)
			fnT := infer.env.GetFn("agl.Option." + fnName)
			if fnName == "Unwrap" || fnName == "UnwrapOr" {
				fnT = fnT.T("T", idTT.W)
			}
			infer.SetType(expr, fnT.Return)
		case types.ResultType:
			assertf(InArray(fnName, []string{"IsOk", "IsErr", "Unwrap", "UnwrapOr", "Err"}),
				"Unresolved reference '%s'", fnName)
			fnT := infer.env.GetFn("agl.Result." + fnName)
			if fnName == "Unwrap" || fnName == "UnwrapOr" {
				fnT = fnT.T("T", idTT.W)
			} else if fnName == "Err" {
				panic("user cannot call Err")
			}
			infer.SetType(expr, fnT.Return)
		default:
			assertf(false, "%s: Unresolved reference '%s'", infer.Pos(expr.Fun), fnName)
		}
		infer.SetType(call.X, exprFunT, WithDefinition(callXParent))
		infer.inferGoExtensions(expr, exprFunT, call)
		infer.exprs(expr.Args)
	case *ast.Ident:
		if call.Name == "make" {
			fnT := infer.env.Get("make").(types.FuncType)
			assert(len(expr.Args) >= 1, "'make' must have at least 1 argument")
			arg0 := expr.Args[0]
			switch v := arg0.(type) {
			case *ast.ArrayType, *ast.ChanType, *ast.MapType:
				fnT = fnT.T("T", infer.env.GetType2(v))
				infer.SetType(expr, fnT.Return)
			default:
				panic(fmt.Sprintf("%v %v", arg0, to(arg0)))
			}
		} else if call.Name == "append" {
			fnT := infer.env.Get("append").(types.FuncType)
			arg0 := infer.env.GetType2(expr.Args[0])
			switch v := arg0.(type) {
			case types.ArrayType:
				fnT = fnT.T("T", v.Elt)
				infer.SetType(expr, fnT.Return)
			default:
				panic(fmt.Sprintf("%v %v", arg0, to(arg0)))
			}
		}
		callT := infer.env.Get(call.Name)
		assertf(callT != nil, "%s: Unresolved reference '%s'", infer.Pos(call), call.Name)
		switch callTT := callT.(type) {
		case types.TypeType:
			infer.expr(expr.Args[0])
			infer.SetType(expr, callTT.W)
		case types.FuncType:
			oParams := callTT.Params
			for i := range expr.Args {
				arg := expr.Args[i]
				var oArg types.Type
				if i >= len(oParams) {
					oArg = oParams[len(oParams)-1]
				} else {
					oArg = oParams[i]
				}
				infer.withOptType(arg, oArg, func() {
					infer.expr(arg)
				})
				if v, ok := oArg.(types.EllipsisType); ok {
					oArg = v.Elt
				}
				got := infer.GetType(arg)
				if oArgT, ok := oArg.(types.IndexType); ok {
					oArg = oArgT.X
				}
				assertf(cmpTypes(oArg, got), "%s: types not equal, %v %v", infer.Pos(arg), oArg, got)
			}
		default:
			panic(fmt.Sprintf("%v", to(callT)))
		}
		parentInfo := infer.env.GetNameInfo(call.Name)
		infer.SetType(call, callT, WithDefinition(parentInfo))
	case *ast.FuncLit:
		callT := funcTypeToFuncType("", call.Type, infer.env, false)
		infer.SetType(call, callT)
		infer.SetType(expr, callT.Return)
	case *ast.ArrayType:
		callT := infer.env.GetType2(call)
		infer.SetType(call, callT)
		infer.SetType(expr, callT)
	default:
		panic(fmt.Sprintf("%v", to(expr.Fun)))
	}
	if exprFunT := infer.env.GetType(expr.Fun); exprFunT != nil {
		if v, ok := exprFunT.(types.FuncType); ok {
			for i, arg := range expr.Args {
				if _, ok := infer.env.GetType(arg).(types.UntypedNoneType); ok {
					infer.SetTypeForce(arg, types.NoneType{W: v.Params[i].(types.OptionType).W})
				}
			}
			if infer.env.GetType2(expr) == nil {
				if v.Return != nil {
					toReturn := v.Return
					toReturn = alterResultBubble(infer.returnType, toReturn)
					infer.SetType(expr, toReturn)
				}
			}
		}
	}
}

func (infer *FileInferrer) inferGoExtensions(expr *ast.CallExpr, idT types.Type, exprT *ast.SelectorExpr) {
	switch idTT := idT.(type) {
	case types.ArrayType:
		fnName := exprT.Sel.Name
		exprPos := infer.Pos(expr)
		if fnName == "Filter" {
			filterFnT := infer.env.GetFn("agl.Vec.Filter").T("T", idTT.Elt)
			infer.SetType(expr.Args[0], filterFnT.Params[1])
			infer.SetType(expr, filterFnT.Return)
			ft := filterFnT.GetParam(1).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			filterFnT.Recv = []types.Type{idTT}
			filterFnT.Params = filterFnT.Params[1:]
			infer.SetType(expr, types.ArrayType{Elt: ft.Params[0]})
			infer.SetType(exprT.Sel, filterFnT)
		} else if fnName == "Map" {
			mapFnT := infer.env.GetFn("agl.Vec.Map").T("T", idTT.Elt)
			clbFnT := mapFnT.GetParam(1).(types.FuncType)
			exprArg0 := expr.Args[0]
			mapFnT.Recv = []types.Type{idTT}
			mapFnT.Params = mapFnT.Params[1:]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, mapFnT.Return)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, false)
				assertf(compareFunctionSignatures(ftReal, clbFnT), "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if tmp, ok := exprArg0.(*ast.FuncLit); ok {
					infer.expr(tmp)
					retT := infer.GetTypeFn(tmp).Return
					infer.SetType(expr, types.ArrayType{Elt: retT})
				}
				assertf(compareFunctionSignatures(ftReal, clbFnT), "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
			}
		} else if fnName == "Reduce" {
			infer.inferVecReduce(expr, exprT, idTT)
		} else if fnName == "Find" {
			findFnT := infer.env.GetFn("agl.Vec.Find").T("T", idTT.Elt)
			infer.SetType(expr, findFnT.Return)
			ft := findFnT.GetParam(1).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			findFnT.Recv = []types.Type{idTT}
			findFnT.Params = findFnT.Params[1:]
			infer.SetType(expr, types.OptionType{W: ft.Params[0]})
			infer.SetType(exprT.Sel, findFnT)
		} else if fnName == "Sum" || fnName == "Last" || fnName == "First" || fnName == "Push" ||
			fnName == "PushFront" || fnName == "Insert" || fnName == "Pop" || fnName == "PopFront" ||
			fnName == "Len" || fnName == "IsEmpty" {
			sumFnT := infer.env.GetFn("agl.Vec."+fnName).T("T", idTT.Elt)
			sumFnT.Recv = []types.Type{idTT}
			sumFnT.Params = sumFnT.Params[1:]
			infer.SetType(expr, sumFnT.Return)
			infer.SetType(exprT.Sel, sumFnT)
		} else if fnName == "PopIf" {
			sumFnT := infer.env.GetFn("agl.Vec.PopIf").T("T", idTT.Elt)
			clbT := sumFnT.GetParam(1).(types.FuncType)
			sumFnT.Recv = []types.Type{idTT}
			sumFnT.Params = sumFnT.Params[1:]
			if _, ok := expr.Args[0].(*ast.ShortFuncLit); ok {
				infer.SetType(expr.Args[0], clbT)
			}
			infer.SetType(expr, sumFnT.Return)
			infer.SetType(exprT.Sel, sumFnT)
		} else if fnName == "Joined" {
			joinedFnT := infer.env.GetFn("agl.Vec.Joined")
			param0 := joinedFnT.Params[0]
			assertf(cmpTypes(idT, param0), "type mismatch, wants: %s, got: %s", param0, idT)
			infer.SetType(expr, joinedFnT.Return)
			joinedFnT.Recv = []types.Type{param0}
			joinedFnT.Params = joinedFnT.Params[1:]
			infer.SetType(exprT.Sel, joinedFnT)
		} else {
			fnFullName := fmt.Sprintf("agl.Vec.%s", fnName)
			fnT := infer.env.Get(fnFullName)
			assertf(fnT != nil, "%s: method '%s' of type Vec does not exists", infer.Pos(exprT.Sel), fnName)
			fnT0 := fnT.(types.FuncType)
			assert(len(fnT0.TypeParams) >= 1, "agl.Vec should have at least one generic parameter")
			gen0 := fnT0.TypeParams[0].(types.GenericType).W
			want := types.ArrayType{Elt: gen0}
			assertf(cmpTypes(gen0, idTT.Elt), "%s: cannot use %s as %s for %s", infer.Pos(exprT.Sel), idTT, want, fnName)
			fnT1 := fnT0.T("T", idTT.Elt)
			retT := Or[types.Type](fnT1.Return, types.VoidType{})
			infer.SetType(exprT.Sel, fnT1)
			infer.SetType(expr.Fun, fnT1)
			infer.SetType(expr, retT)
			ft := infer.GetTypeFn(expr.Fun)
			// Go through the arguments and get a mapping of "generic name" to "concrete type" (eg: {"T":int})
			genericMapping := make(map[string]types.Type)
			for i, arg := range expr.Args {
				if argFn, ok := arg.(*ast.ShortFuncLit); ok {
					genFn := ft.GetParam(i)
					infer.SetType(argFn, genFn)
					infer.expr(argFn)
					concreteFn := infer.env.GetType(arg)
					m := types.FindGen(genFn, concreteFn)
					for k, v := range m {
						if el, ok := genericMapping[k]; ok {
							assertf(el == v, "generic type parameter type mismatch. want: %v, got: %v", el, v)
						}
						genericMapping[k] = v
					}
				} else if argFn, ok := arg.(*ast.FuncLit); ok {
					infer.expr(argFn)
					genFn := ft.GetParam(i)
					concreteFn := infer.env.GetType(arg)
					m := types.FindGen(genFn, concreteFn)
					for k, v := range m {
						if el, ok := genericMapping[k]; ok {
							assertf(el == v, "generic type parameter type mismatch. want: %v, got: %v", el, v)
						}
						genericMapping[k] = v
					}
				}
			}
			for k, v := range genericMapping {
				ft = ft.ReplaceGenericParameter(k, v)
			}
			ft.Recv = []types.Type{idTT}
			infer.SetType(exprT.Sel, ft)
			infer.SetType(expr.Fun, ft)
			infer.SetType(expr, ft.Return)
		}
	case types.MapType:
		fnName := exprT.Sel.Name
		if fnName == "Get" {
			getFnT := infer.env.GetFn("agl.Map.Get").T("K", idTT.K).T("V", idTT.V)
			getFnT.Recv = []types.Type{idTT}
			getFnT.Params = getFnT.Params[1:]
			infer.SetType(expr, getFnT.Return)
			infer.SetType(exprT.Sel, getFnT)
		} else if fnName == "Keys" {
			fnT := infer.env.GetFn("agl.Map.Keys").T("K", idTT.K).T("V", idTT.V)
			fnT.Recv = []types.Type{idTT}
			fnT.Params = fnT.Params[1:]
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		} else if fnName == "Values" {
			fnT := infer.env.GetFn("agl.Map.Values").T("K", idTT.K).T("V", idTT.V)
			fnT.Recv = []types.Type{idTT}
			fnT.Params = fnT.Params[1:]
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		}
	}
}

func (infer *FileInferrer) inferVecReduce(expr *ast.CallExpr, exprFun *ast.SelectorExpr, idTArr types.ArrayType) {
	eltT := idTArr.Elt
	exprPos := infer.Pos(expr)
	fnT := infer.env.GetFn("agl.Vec.Reduce").T("T", idTArr.Elt)
	if infer.forceReturnType != nil {
		fnT = fnT.T("R", infer.forceReturnType)
	} else if r, ok := infer.env.GetType2(expr.Args[0]).(types.UntypedNumType); !ok {
		noop(r) // TODO should add restriction on type R (cmp.Comparable?)
		//fnT = fnT.T("R", r)
	}
	infer.SetType(exprFun.Sel, fnT)
	infer.SetType(expr.Args[1], fnT.Params[2])
	infer.SetType(expr, fnT.Return)
	exprArg0 := expr.Args[0]
	infer.withOptType(exprArg0, infer.forceReturnType, func() {
		infer.expr(exprArg0)
	})
	arg0T := infer.GetType(exprArg0)
	reduceFnT := infer.env.GetType(exprFun.Sel).(types.FuncType)
	ft := reduceFnT.GetParam(2).(types.FuncType)
	if infer.forceReturnType != nil {
		ft = ft.T("R", infer.forceReturnType)
		reduceFnT = reduceFnT.T("R", infer.forceReturnType)
		assertf(cmpTypes(arg0T, infer.forceReturnType), "%s: type mismatch, want: %s, got: %s", exprPos, infer.forceReturnType, arg0T)
	} else if _, ok := infer.GetType(exprArg0).(types.UntypedNumType); ok {
		ft = ft.T("R", eltT)
		reduceFnT = reduceFnT.T("R", eltT)
	} else {
		ft = ft.T("R", arg0T)
		reduceFnT = reduceFnT.T("R", arg0T)
	}
	if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
		infer.SetType(expr.Args[1], ft)
	} else if _, ok := exprArg0.(*ast.FuncType); ok {
		ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
		assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
	} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
		assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
	}
	reduceFnT.Recv = []types.Type{idTArr}
	reduceFnT.Params = reduceFnT.Params[1:]
	infer.SetTypeForce(exprFun.Sel, reduceFnT)
	infer.SetType(expr.Fun, reduceFnT)
	infer.SetType(expr, reduceFnT.Return)
}

func alterResultBubble(fnReturn types.Type, curr types.Type) (out types.Type) {
	if fnReturn == nil {
		return
	}
	fnReturnIsResult := TryCast[types.ResultType](fnReturn)
	fnReturnIsOption := TryCast[types.OptionType](fnReturn)
	currIsResult := TryCast[types.ResultType](curr)
	currIsOption := TryCast[types.OptionType](curr)
	out = curr
	if fnReturnIsResult {
		if currIsResult {
			tmp := MustCast[types.ResultType](curr)
			tmp.Bubble = true
			out = tmp
		}
	} else if fnReturnIsOption && currIsOption {
		tmp := MustCast[types.OptionType](curr)
		tmp.Bubble = true
		out = tmp
	} else {
		if currIsResult {
			tmp := MustCast[types.ResultType](curr)
			if fnReturnIsOption {
				fnReturnOpt := MustCast[types.OptionType](fnReturn)
				tmp.Bubble = true
				tmp.ConvertToNone = true
				tmp.ToNoneType = fnReturnOpt.W
				out = tmp
			} else {
				tmp.Bubble = false
				out = tmp
			}
		}
	}
	if !fnReturnIsOption {
		if tmp, ok := curr.(types.OptionType); ok {
			tmp.Bubble = false
			out = tmp
		}
	}
	return
}

func (infer *FileInferrer) funcLit(expr *ast.FuncLit) {
	if infer.optType.IsDefinedFor(expr) {
		infer.SetType(expr, infer.optType.Type)
	}
	ft := funcTypeToFuncType("", expr.Type, infer.env, false)
	// implicit return
	if len(expr.Body.List) == 1 && TryCast[*ast.ExprStmt](expr.Body.List[0]) {
		returnStmt := expr.Body.List[0].(*ast.ExprStmt)
		expr.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: returnStmt.X}}
	}
	infer.SetType(expr, ft)
}

func (infer *FileInferrer) shortFuncLit(expr *ast.ShortFuncLit) {
	infer.withEnv(func() {
		if infer.optType.IsDefinedFor(expr) {
			infer.SetType(expr, infer.optType.Type)
		}
		if t := infer.env.GetType(expr); t != nil {
			for i, param := range t.(types.FuncType).Params {
				infer.env.Define(nil, fmt.Sprintf("$%d", i), param)
			}
		}
		infer.stmt(expr.Body)
		// implicit return
		if len(expr.Body.List) == 1 && TryCast[*ast.ExprStmt](expr.Body.List[0]) {
			returnStmt := expr.Body.List[0].(*ast.ExprStmt)
			if infer.env.GetType(returnStmt.X) != nil {
				if infer.env.GetType(expr) != nil {
					ft := infer.env.GetType(expr).(types.FuncType)
					if t, ok := ft.Return.(types.GenericType); ok {
						ft = ft.T(t.Name, infer.env.GetType(returnStmt.X))
						infer.SetType(expr, ft)
					}
				}
			}
			expr.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: returnStmt.X}}
		}
		// expr type is set in CallExpr
	})
}

func (infer *FileInferrer) funcType(expr *ast.FuncType) {
	//infer.expr(expr.TypeParams)
	//infer.expr(expr.Params)
	var paramsT []types.Type
	if expr.Params != nil {
		for _, param := range expr.Params.List {
			infer.expr(param.Type)
			t := infer.env.GetType(param.Type)
			n := max(len(param.Names), 1)
			for i := 0; i < n; i++ {
				paramsT = append(paramsT, t)
			}
		}
	}
	var returnT types.Type = types.VoidType{}
	if expr.Result != nil {
		infer.expr(expr.Result)
		returnT = infer.env.GetType(expr.Result)
		if returnT == nil {
			returnT = types.VoidType{}
		}
	}
	ft := types.FuncType{
		Params: paramsT,
		Return: returnT,
	}
	infer.SetType(expr, ft)
}

func (infer *FileInferrer) voidExpr(expr *ast.VoidExpr) {
	infer.SetType(expr, types.VoidType{})
}

func (infer *FileInferrer) someExpr(expr *ast.SomeExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.SomeType{W: infer.env.GetType(expr.X)})
}

func (infer *FileInferrer) noneExpr(expr *ast.NoneExpr) {
	infer.SetType(expr, types.UntypedNoneType{})
}

func (infer *FileInferrer) okExpr(expr *ast.OkExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.OkType{W: infer.env.GetType(expr.X)})
}

func (infer *FileInferrer) errExpr(expr *ast.ErrExpr) {
	infer.expr(expr.X)
	// turn `Err("error")` into `Err(errors.New("error"))`
	if v, ok := expr.X.(*ast.BasicLit); ok && v.Kind == token.STRING {
		expr.X = &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "errors"}, Sel: &ast.Ident{Name: "New"}}, Args: []ast.Expr{v}}
	}
	var t types.Type
	if infer.optType != nil {
		t = infer.optType.Type
	} else {
		t = infer.returnType
	}
	infer.SetType(expr, types.ErrType{W: infer.env.GetType(expr.X), T: t.(types.ResultType).W})
}

func (infer *FileInferrer) chanType(expr *ast.ChanType) {
	infer.expr(expr.Value)
}

func (infer *FileInferrer) unaryExpr(expr *ast.UnaryExpr) {
	infer.expr(expr.X)
	if expr.Op == token.AND {
		infer.SetType(expr, types.StarType{X: infer.GetType(expr.X)})
	} else {
		infer.SetType(expr, infer.env.GetType2(expr.X))
	}
}

func (infer *FileInferrer) typeAssertExpr(expr *ast.TypeAssertExpr) {
	infer.expr(expr.X)
	if expr.Type != nil {
		infer.expr(expr.Type)
	}
	if expr.Type != nil {
		_, bubble := infer.returnType.(types.OptionType)
		t := types.OptionType{W: infer.env.GetType2(expr.Type), Bubble: bubble}
		infer.SetType(expr, t)
	}
	//infer.SetType(expr, types.OptionType{W: infer.env.GetType2(expr.Type)})
}

func (infer *FileInferrer) orBreak(expr *ast.OrBreakExpr) {
	infer.expr(expr.X)
	var t types.Type
	switch v := infer.GetType(expr.X).(type) {
	case types.OptionType:
		t = v.W
	case types.ResultType:
		t = v.W
	default:
		panic("")
	}
	infer.SetType(expr, t)
}

func (infer *FileInferrer) orContinue(expr *ast.OrContinueExpr) {
	infer.expr(expr.X)
	var t types.Type
	switch v := infer.GetType(expr.X).(type) {
	case types.OptionType:
		t = v.W
	case types.ResultType:
		t = v.W
	default:
		panic("")
	}
	infer.SetType(expr, t)
}

func (infer *FileInferrer) orReturn(expr *ast.OrReturnExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X))
}

func (infer *FileInferrer) mapType(expr *ast.MapType) {
	infer.expr(expr.Key)
	infer.expr(expr.Value)
	kT := infer.GetType(expr.Key)
	vT := infer.GetType(expr.Value)
	infer.SetType(expr.Key, kT)
	infer.SetType(expr.Value, vT)
	infer.SetType(expr, types.MapType{K: kT, V: vT})
}

func (infer *FileInferrer) starExpr(expr *ast.StarExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.StarType{X: infer.GetType(expr.X)})
}

func (infer *FileInferrer) ellipsis(expr *ast.Ellipsis) {
	infer.expr(expr.Elt)
	infer.SetType(expr, types.EllipsisType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) tupleExpr(expr *ast.TupleExpr) {
	infer.exprs(expr.Values)
	if infer.optType.IsDefinedFor(expr) {
		expected := infer.optType.Type.(types.TupleType).Elts
		for i, x := range expr.Values {
			if _, ok := infer.GetType(x).(types.UntypedNumType); ok {
				infer.SetType(x, expected[i])
			}
		}
	} else {
		var elts []types.Type
		for _, v := range expr.Values {
			elT := infer.GetType(v)
			elts = append(elts, elT)
		}
		infer.SetType(expr, types.TupleType{Elts: elts})
	}
}

func compareFunctionSignatures(sig1, sig2 types.FuncType) bool {
	// Compare return types
	if !cmpTypes(sig1.Return, sig2.Return) {
		return false
	}
	// Compare number of parameters
	if len(sig1.Params) != len(sig2.Params) {
		return false
	}
	//// Compare variadic status
	//if sig1.variadic != sig2.variadic {
	//	return false
	//}
	// Compare each parameter type
	for i := range sig1.Params {
		if !cmpTypes(sig1.Params[i], sig2.Params[i]) {
			return false
		}
	}
	// Compare type parameters if they exist
	if len(sig1.TypeParams) != len(sig2.TypeParams) { // TODO
		return false
	}
	for i := range sig1.TypeParams {
		if !cmpTypes(sig1.TypeParams[i], sig2.TypeParams[i]) {
			return false
		}
	}
	return true
}

func cmpTypesLoose(a, b types.Type) bool {
	if v, ok := a.(types.StarType); ok {
		a = v.X
	}
	if v, ok := b.(types.StarType); ok {
		b = v.X
	}
	if v, ok := a.(types.CustomType); ok {
		a = v.W
	}
	if v, ok := b.(types.CustomType); ok {
		b = v.W
	}
	if v, ok := a.(types.TypeType); ok {
		a = v.W
	}
	if v, ok := b.(types.TypeType); ok {
		b = v.W
	}
	if isNumericType(a) && TryCast[types.UntypedNumType](b) {
		return true
	}
	if isNumericType(b) && TryCast[types.UntypedNumType](a) {
		return true
	}
	return cmpTypes(a, b)
}

func cmpTypes(a, b types.Type) bool {
	if tt, ok := a.(types.TypeType); ok {
		a = tt.W
	}
	if tt, ok := b.(types.TypeType); ok {
		b = tt.W
	}
	if aa, ok := a.(types.FuncType); ok {
		if bb, ok := b.(types.FuncType); ok {
			if aa.GoStr() == bb.GoStr() {
				return true
			}
			if !cmpTypes(aa.Return, bb.Return) {
				return false
			}
			if len(aa.Params) != len(bb.Params) {
				return false
			}
			for i := range aa.Params {
				if !cmpTypes(aa.Params[i], bb.Params[i]) {
					return false
				}
			}
			return true
		}
		return false
	}
	if aa, ok := a.(types.TupleType); ok {
		if bb, ok := b.(types.TupleType); ok {
			if len(aa.Elts) != len(bb.Elts) {
				return false
			}
			for i := range aa.Elts {
				return cmpTypesLoose(aa.Elts[i], bb.Elts[i])
			}
			return true
		}
		return false
	}
	if TryCast[types.AnyType](a) || TryCast[types.AnyType](b) {
		return true
	}
	if TryCast[types.BoolType](a) && TryCast[types.BoolType](b) {
		return a == b
	}
	if TryCast[types.MapType](a) && TryCast[types.MapType](b) {
		aa := MustCast[types.MapType](a)
		bb := MustCast[types.MapType](b)
		return cmpTypes(aa.K, bb.K) && cmpTypes(aa.V, bb.V)
	}
	if TryCast[types.GenericType](a) || TryCast[types.GenericType](b) {
		return true
	}
	if TryCast[types.StructType](a) || TryCast[types.StructType](b) {
		return true // TODO
	}
	if TryCast[types.InterfaceType](a) || TryCast[types.InterfaceType](b) {
		return true // TODO
	}
	if TryCast[types.ArrayType](a) && TryCast[types.ArrayType](b) {
		aa := MustCast[types.ArrayType](a)
		bb := MustCast[types.ArrayType](b)
		return cmpTypes(aa.Elt, bb.Elt)
	}
	if TryCast[types.EnumType](a) || TryCast[types.EnumType](b) {
		return true // TODO
	}
	if TryCast[types.StarType](a) && TryCast[types.StarType](b) {
		return cmpTypes(a.(types.StarType).X, b.(types.StarType).X)
	}
	if TryCast[types.OptionType](a) && TryCast[types.OptionType](b) {
		return cmpTypes(a.(types.OptionType).W, b.(types.OptionType).W)
	}
	if TryCast[types.ResultType](a) && TryCast[types.ResultType](b) {
		return cmpTypes(a.(types.ResultType).W, b.(types.ResultType).W)
	}
	if TryCast[types.IndexListType](a) && TryCast[types.IndexListType](b) {
		return cmpTypes(a.(types.IndexListType).X, b.(types.IndexListType).X)
	}
	if TryCast[types.IndexType](a) && TryCast[types.IndexType](b) {
		return cmpTypes(a.(types.IndexType).X, b.(types.IndexType).X)
	}
	if a == b {
		return true
	}
	return false
}

func (infer *FileInferrer) selectorExpr(expr *ast.SelectorExpr) {
	infer.expr(expr.X)
	exprXT := infer.env.GetType2(expr.X)
	exprXIdTRaw := exprXT
	if v, ok := exprXIdTRaw.(types.StarType); ok {
		exprXIdTRaw = v.X
	}
	switch exprXIdT := exprXIdTRaw.(type) {
	case types.StructType:
		fieldName := expr.Sel.Name
		if f := Find(exprXIdT.Fields, func(f types.FieldType) bool { return f.Name == fieldName }); f != nil {
			infer.SetType(expr.Sel, f.Typ)
			infer.SetType(expr.X, exprXT)
			infer.SetType(expr, f.Typ)
		} else {
			infer.SetType(expr.X, exprXT)
		}
		return
	case types.InterfaceType:
		return
	case types.EnumType:
		enumName := expr.X.(*ast.Ident).Name
		fieldName := expr.Sel.Name
		validFields := make([]string, 0, len(exprXIdT.Fields))
		for _, f := range exprXIdT.Fields {
			validFields = append(validFields, f.Name)
		}
		assertf(InArray(fieldName, validFields), "%d: enum %s has no field %s", expr.Sel.Pos(), enumName, fieldName)
		infer.SetType(expr.X, exprXIdT)
		infer.SetType(expr, exprXIdT)
	case types.TupleType:
		argIdx, err := strconv.Atoi(expr.Sel.Name)
		if err != nil {
			panic("tuple arg index must be int")
		}
		infer.SetType(expr.X, exprXIdT)
		infer.SetType(expr, exprXIdT.Elts[argIdx])
	case types.PackageType:
		pkg := expr.X.(*ast.Ident).Name
		sel := expr.Sel.Name
		selT := infer.env.Get(pkg + "." + sel)
		assertf(selT != nil, "%s: '%s' not found in package '%s'", infer.Pos(expr.Sel), sel, pkg)
		infer.SetType(expr.Sel, selT)
		infer.SetType(expr, selT)
	case types.OptionType:
		infer.SetType(expr.Sel, exprXIdT.W)
		infer.SetType(expr.X, exprXIdT)
		infer.SetType(expr, exprXIdT.W)
	case types.TypeAssertType:
		if v, ok := exprXIdT.Type.(types.StarType); ok {
			exprXIdT.Type = v.X
		}
		if v, ok := exprXIdT.Type.(types.StructType); ok {
			fieldName := expr.Sel.Name
			if f := Find(v.Fields, func(f types.FieldType) bool { return f.Name == fieldName }); f != nil {
				infer.SetType(expr.Sel, f.Typ)
				infer.SetType(expr.X, v)
				infer.SetType(expr, f.Typ)
				return
			}
		}
		infer.SetType(expr.X, exprXIdT.X)
		infer.SetType(expr, exprXIdT.Type)
	default:
		panic(fmt.Sprintf("%v", to(exprXIdTRaw)))
	}
}

func (infer *FileInferrer) bubbleResultExpr(expr *ast.BubbleResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X).(types.ResultType).W)
}

func (infer *FileInferrer) bubbleOptionExpr(expr *ast.BubbleOptionExpr) {
	infer.expr(expr.X)
	tmp := infer.GetType(expr.X)
	infer.SetType(expr, tmp.(types.OptionType).W)
}

func (infer *FileInferrer) compositeLit(expr *ast.CompositeLit) {
	switch v := expr.Type.(type) {
	case *ast.IndexExpr:
		t := infer.env.Get(v.X.(*ast.Ident).Name)
		infer.SetType(v.X, t)
		infer.SetType(expr, t)
		return
	case *ast.IndexListExpr:
		t := infer.env.Get(v.X.(*ast.Ident).Name)
		infer.SetType(v.X, t)
		infer.SetType(expr, t)
		return
	case *ast.ArrayType:
		t := infer.env.GetType2(v.Elt)
		infer.exprs(expr.Elts)
		infer.SetType(v.Elt, t)
		infer.SetType(expr, types.ArrayType{Elt: t})
		return
	case *ast.Ident:
		infer.SetType(expr, infer.env.Get(v.Name))
		return
	case *ast.MapType:
		keyT := infer.env.GetType2(v.Key)
		valT := infer.env.GetType2(v.Value)
		infer.withMapKV(keyT, valT, func() {
			infer.exprs(expr.Elts)
		})
		infer.SetType(expr, types.MapType{K: keyT, V: valT})
		return
	case *ast.SelectorExpr:
		idName := v.X.(*ast.Ident).Name
		xT := infer.env.Get(idName)
		selT := infer.env.Get(fmt.Sprintf("%s.%s", idName, v.Sel.Name))
		infer.SetType(v.X, xT)
		infer.SetType(v.Sel, selT)
		infer.SetType(expr, selT)
		return
	default:
		panic(fmt.Sprintf("%s: %v", infer.Pos(expr), to(expr.Type)))
	}
}

func (infer *FileInferrer) arrayType(expr *ast.ArrayType) {
	if expr.Len != nil {
		infer.expr(expr.Len)
	}
	infer.expr(expr.Elt)
	infer.SetType(expr, types.ArrayType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) indexListExpr(expr *ast.IndexListExpr) {
	infer.expr(expr.X)
	infer.exprs(expr.Indices)
	var indices []types.FieldType
	//for _, e := range expr.Indices {
	//	indices = append(indices)
	//}
	//fmt.Println("???", e)
	//Indices: infer.GetType(expr.Indices)
	infer.SetType(expr, types.IndexListType{X: infer.GetType(expr.X), Indices: indices})
}

func (infer *FileInferrer) dumpExpr(expr *ast.DumpExpr) {
	infer.expr(expr.X)
}

func (infer *FileInferrer) sliceExpr(expr *ast.SliceExpr) {
	// TODO
}

func (infer *FileInferrer) interfaceType(expr *ast.InterfaceType) {
	// TODO
}

func (infer *FileInferrer) keyValueExpr(expr *ast.KeyValueExpr) {
	infer.expr(expr.Key)
	switch v := expr.Value.(type) {
	case *ast.CompositeLit:
		if v.Type == nil && infer.mapVT == nil {
			panic("")
		}
	default:
		infer.expr(expr.Value)
	}
	//infer.SetType(expr,) // TODO
}

func (infer *FileInferrer) indexExpr(expr *ast.IndexExpr) {
	infer.expr(expr.X)
	infer.expr(expr.Index)
	exprXT := infer.env.GetType2(expr.X)
	switch v := exprXT.(type) {
	case types.MapType:
		infer.SetType(expr, v.V) // TODO should return an Option[T] ?
	case types.ArrayType:
		infer.SetType(expr, v.Elt)
	default:
		infer.SetType(expr, infer.GetType(expr.X))
	}
}

func (infer *FileInferrer) resultExpr(expr *ast.ResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.ResultType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) optionExpr(expr *ast.OptionExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.OptionType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) declStmt(stmt *ast.DeclStmt) {
	switch d := stmt.Decl.(type) {
	case *ast.GenDecl:
		infer.specs(d.Specs)
	}
}

func (infer *FileInferrer) specs(s []ast.Spec) {
	for _, spec := range s {
		infer.spec(spec)
	}
}

func (infer *FileInferrer) spec(s ast.Spec) {
	switch spec := s.(type) {
	case *ast.ValueSpec:
		t := infer.env.GetType2(spec.Type)
		for i, name := range spec.Names {
			if len(spec.Values) > 0 {
				infer.exprs(spec.Values)
				value := spec.Values[i]
				valueT := infer.env.GetType(value)
				assertf(cmpTypes(t, valueT), "%s: type mismatch, want: %s, got: %s", infer.Pos(name), t, valueT)
			}
			infer.SetType(name, t)
			infer.env.Define(name, name.Name, t)
		}
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
}

func (infer *FileInferrer) incDecStmt(stmt *ast.IncDecStmt) {
	infer.expr(stmt.X)
}

func (infer *FileInferrer) forStmt(stmt *ast.ForStmt) {
	infer.withEnv(func() {
		if stmt.Init != nil {
			infer.stmt(stmt.Init)
		}
		if stmt.Cond != nil {
			infer.expr(stmt.Cond)
		}
		if stmt.Post != nil {
			infer.stmt(stmt.Post)
		}
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
}

func (infer *FileInferrer) rangeStmt(stmt *ast.RangeStmt) {
	infer.withEnv(func() {
		infer.expr(stmt.X)
		xT := infer.env.GetType2(stmt.X)
		if stmt.Key != nil {
			// TODO find correct type for map
			name := stmt.Key.(*ast.Ident).Name
			t := types.IntType{}
			infer.env.Define(stmt.Key, name, t)
			infer.SetType(stmt.Key, t)
		}
		if stmt.Value != nil {
			name := stmt.Value.(*ast.Ident).Name
			switch v := xT.(type) {
			case types.StringType:
				infer.env.Define(stmt.Value, name, types.I32Type{})
			case types.ArrayType:
				infer.env.Define(stmt.Value, name, v.Elt)
				infer.SetType(stmt.Value, v.Elt)
			case types.MapType:
				infer.env.Define(stmt.Value, name, v.V)
				infer.SetType(stmt.Value, v.V)
			default:
				panic(fmt.Sprintf("%v", to(xT)))
			}
		}
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
}

func (infer *FileInferrer) Pos(n ast.Node) token.Position {
	return infer.fset.Position(n.Pos())
}

func (infer *FileInferrer) assignStmt(stmt *ast.AssignStmt) {
	type AssignStruct struct {
		n    ast.Node
		name string
		typ  types.Type
	}
	myDefine := func(parentInfo *Info, n ast.Node, name string, typ types.Type) {
		infer.env.DefineForce(n, name, typ)
	}
	myAssign := func(parentInfo *Info, n ast.Node, name string, _ types.Type) {
		infer.env.Assign(parentInfo, n, name)
	}
	var assigns []AssignStruct
	assignFn := func(n ast.Node, name string, typ types.Type) {
		op := stmt.Tok
		f := Ternary(op == token.DEFINE, myDefine, myAssign)
		var parentInfo *Info
		if op != token.DEFINE {
			parentInfo = infer.env.GetNameInfo(name)
		}
		f(parentInfo, n, name, typ)
	}
	assignsFn := func() {
		op := stmt.Tok
		if op == token.DEFINE {
			var hasNewVar bool
			for _, ass := range assigns {
				if ass.name != "_" && infer.env.GetDirect(ass.name) == nil {
					hasNewVar = true
				}
			}
			assertf(hasNewVar, "%s: No new variables on the left side of ':='", infer.Pos(stmt))
		}
		for _, ass := range assigns {
			assignFn(ass.n, ass.name, ass.typ)
		}
	}
	if len(stmt.Rhs) == 1 {
		rhs := stmt.Rhs[0]
		if len(stmt.Lhs) == 1 {
			lhs := stmt.Lhs[0]
			var lhsWantedT types.Type
			switch v := lhs.(type) {
			case *ast.Ident:
				lhsIdName := v.Name
				lhsWantedT = infer.env.Get(lhsIdName)
			case *ast.IndexExpr:
				lhsIdName := v.X.(*ast.Ident).Name
				switch vv := infer.env.Get(lhsIdName).(type) {
				case types.MapType:
					lhsWantedT = vv.V
				case types.ArrayType:
					lhsWantedT = vv.Elt
				default:
					panic(fmt.Sprintf("%v", to(v.X)))
				}
			case *ast.SelectorExpr:
				lhsIdName := v.X.(*ast.Ident).Name
				xT := infer.env.Get(lhsIdName)
				if tup, ok := xT.(types.TupleType); ok {
					argIdx, _ := strconv.Atoi(v.Sel.Name)
					lhsWantedT = tup.Elts[argIdx]
				}
			default:
				panic(fmt.Sprintf("%v", to(lhs)))
			}
			if lhsT := lhsWantedT; lhsT != nil {
				infer.withForceReturn(lhsT, func() {
					infer.expr(rhs)
					rhsT := infer.env.GetType2(rhs)
					assertf(cmpTypesLoose(rhsT, lhsT), "%s: return type %s does not match expected type %s", infer.Pos(lhs), rhsT, lhsT)
				})
			} else {
				infer.expr(rhs)
			}
			var lhsID *ast.Ident
			switch v := lhs.(type) {
			case *ast.Ident:
				lhsID = v
			case *ast.IndexExpr:
				lhsID = v.X.(*ast.Ident)
				return // tODO
			case *ast.SelectorExpr:
				lhsID = v.X.(*ast.Ident)
				lhsIdName := lhsID.Name
				xT := infer.env.Get(lhsIdName)
				if xTv, ok := xT.(types.StarType); ok {
					xT = xTv.X
				}
				switch xTv := xT.(type) {
				case types.TupleType:
					argIdx, _ := strconv.Atoi(v.Sel.Name)
					infer.SetType(v.X, xT)
					want, got := xTv.Elts[argIdx], infer.GetType(rhs)
					assertf(cmpTypesLoose(want, got), "type mismatch, wants: %s, got: %s", want, got)
					return
				case types.StructType:
					infer.SetType(v.X, xT)
				default:
					panic(fmt.Sprintf("%v", to(xT)))
				}
			default:
				panic(fmt.Sprintf("%v", to(lhs)))
			}
			var rhsT types.Type
			var tmp ast.Expr
			if ta, ok := rhs.(*ast.TypeAssertExpr); ok {
				tmp = Ternary(ta.Type == nil, ta.X, ta.Type)
				rhsT = infer.env.GetType2(tmp)
				rhsT = types.OptionType{W: rhsT}
			} else {
				tmp = rhs
				rhsT = infer.env.GetType2(tmp)
			}
			assertf(!TryCast[types.VoidType](rhsT), "cannot assign void type to a variable")
			lhsT := infer.env.GetType(lhs)
			switch lhsT.(type) {
			case types.SomeType, types.NoneType:
				assertf(TryCast[types.OptionType](rhsT), "%s: try to destructure a non-Option type into an OptionType", infer.Pos(lhs))
				infer.SetTypeForce(lhs, rhsT)
			case types.ErrType, types.OkType:
				assertf(TryCast[types.ResultType](rhsT), "%s: try to destructure a non-Result type into an ResultType", infer.Pos(lhs))
				infer.SetTypeForce(lhs, rhsT)
			default:
				infer.SetType(lhs, rhsT)
			}
			assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, infer.GetType(lhsID)})
		} else { // len(stmt.Lhs) != 1
			infer.expr(rhs)
			if rhs1, ok := rhs.(*ast.TupleExpr); ok {
				for i, x := range rhs1.Values {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*ast.Ident](lhs)
					infer.SetType(lhs, infer.GetType(x))
					assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, infer.GetType(lhsID)})
				}
			} else if rhs2, ok := infer.env.GetType(rhs).(types.EnumType); ok {
				for i, e := range stmt.Lhs {
					lit := rhs2.SubTyp
					fields := rhs2.Fields
					// AGL: fields.Find({ $0.name == lit })
					f := Find(fields, func(f types.EnumFieldType) bool { return f.Name == lit })
					assert(f != nil)
					assigns = append(assigns, AssignStruct{e, e.(*ast.Ident).Name, f.Elts[i]})
				}
			} else if rhsId, ok := rhs.(*ast.Ident); ok {
				rhsIdT := infer.env.Get(rhsId.Name)
				if rhs3, ok := rhsIdT.(types.TupleType); ok {
					for i, x := range rhs3.Elts {
						lhs := stmt.Lhs[i]
						lhsID := MustCast[*ast.Ident](lhs)
						infer.SetType(lhs, x)
						assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, infer.GetType(lhsID)})
					}
				}
			} else if rhsId1, ok := rhs.(*ast.IndexExpr); ok {
				rhsId1XT := infer.GetType(rhsId1.X)
				if v, ok := rhsId1XT.(types.CustomType); ok {
					rhsId1XT = v.W
				}
				switch rhsId1XTT := rhsId1XT.(type) {
				case types.MapType:
					switch len(stmt.Lhs) {
					case 2:
						lhs1 := stmt.Lhs[1].(*ast.Ident)
						lhs1T := types.BoolType{}
						infer.SetType(lhs1, lhs1T)
						assigns = append(assigns, AssignStruct{lhs1, lhs1.Name, lhs1T})
						fallthrough
					case 1:
						lhs0 := stmt.Lhs[0].(*ast.Ident)
						lhs0T := rhsId1XTT.V
						infer.SetType(lhs0, rhsId1XTT.V)
						assigns = append(assigns, AssignStruct{lhs0, lhs0.Name, lhs0T})
					default:
						panic("can only have 1 or 2 args on the left side")
					}
				default:
					panic(fmt.Sprintf("%v", to(rhsId1XT)))
				}
			} else {
				panic(fmt.Sprintf("%s: %v", infer.Pos(rhs), to(rhs)))
			}
		}
	} else {
		assertf(len(stmt.Lhs) == len(stmt.Rhs), "%s: Assignment count mismatch: %d = %d", infer.Pos(stmt), len(stmt.Lhs), len(stmt.Rhs))
		for i := range stmt.Lhs {
			lhs := stmt.Lhs[i]
			rhs := stmt.Rhs[i]
			infer.expr(rhs)
			lhsID := MustCast[*ast.Ident](lhs)
			infer.SetType(lhsID, infer.GetType(rhs))
			assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, infer.GetType(lhsID)})
		}
	}
	assignsFn()
}

func (infer *FileInferrer) exprStmt(stmt *ast.ExprStmt) {
	infer.expr(stmt.X)
}

func (infer *FileInferrer) returnStmt(stmt *ast.ReturnStmt) {
	if stmt.Result != nil {
		infer.withOptType(stmt.Result, infer.returnType, func() {
			infer.expr(stmt.Result)
			if _, ok := infer.GetType(stmt.Result).(types.UntypedNoneType); ok {
				if v, ok := infer.returnType.(types.OptionType); ok {
					infer.SetTypeForce(stmt.Result, types.NoneType{W: v.W})
				}
			}
		})
	}
}

func (infer *FileInferrer) blockStmt(stmt *ast.BlockStmt) {
	infer.stmts(stmt.List)
}

func (infer *FileInferrer) binaryExpr(expr *ast.BinaryExpr) {
	infer.expr(expr.X)
	if TryCast[types.OptionType](infer.env.GetType2(expr.X)) &&
		TryCast[*ast.Ident](expr.Y) && expr.Y.(*ast.Ident).Name == "None" &&
		infer.env.GetType(expr.Y) == nil {
		infer.SetType(expr.Y, infer.env.GetType2(expr.X))
	}
	if t := infer.env.GetType(expr.Y); t == nil || TryCast[types.UntypedNumType](t) {
		infer.expr(expr.Y)
	}
	if infer.env.GetType2(expr.X) != nil && infer.env.GetType2(expr.Y) != nil {
		if isNumericType(infer.env.GetType2(expr.X)) && TryCast[types.UntypedNumType](infer.env.GetType2(expr.Y)) {
			infer.SetType(expr.Y, infer.env.GetType2(expr.X))
		}
		if isNumericType(infer.env.GetType2(expr.Y)) && TryCast[types.UntypedNumType](infer.env.GetType2(expr.X)) {
			infer.SetType(expr.X, infer.env.GetType2(expr.Y))
		}
	}
	switch expr.Op {
	case token.EQL, token.NEQ, token.LOR, token.LAND, token.LEQ, token.LSS, token.GEQ, token.GTR:
		infer.SetType(expr, types.BoolType{})
	case token.ADD, token.SUB, token.QUO, token.MUL, token.REM:
		infer.SetType(expr, infer.GetType(expr.X))
	default:
	}
}

func (infer *FileInferrer) identExpr(expr *ast.Ident) types.Type {
	v := infer.env.Get(expr.Name)
	assertf(v != nil, "%s: undefined identifier %s", infer.Pos(expr), expr.Name)
	if expr.Name == "true" || expr.Name == "false" {
		return types.BoolType{}
	}
	return v
}

func (infer *FileInferrer) sendStmt(stmt *ast.SendStmt) {
	infer.expr(stmt.Chan)
	infer.expr(stmt.Value)
}

func (infer *FileInferrer) selectStmt(stmt *ast.SelectStmt) {
	infer.stmt(stmt.Body)
}

func (infer *FileInferrer) commClause(stmt *ast.CommClause) {
	if stmt.Comm != nil {
		infer.stmt(stmt.Comm)
	}
	if stmt.Body != nil {
		infer.stmts(stmt.Body)
	}
}

func (infer *FileInferrer) typeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	infer.withEnv(func() {
		if stmt.Init != nil {
			infer.stmt(stmt.Init)
		}
		if ass, ok := stmt.Assign.(*ast.AssignStmt); ok && len(ass.Lhs) == 1 {
			rhs := ass.Rhs[0]
			t := infer.env.GetType2(rhs)
			infer.SetType(ass.Lhs[0], t.(types.TypeAssertType).X)
		}
		infer.stmt(stmt.Assign)
		for _, el := range stmt.Body.List {
			c := el.(*ast.CaseClause)
			if c.List != nil {
				infer.exprs(c.List)
			}
			if c.Body != nil {
				infer.withEnv(func() {
					switch ass := stmt.Assign.(type) {
					case *ast.AssignStmt:
						if len(ass.Lhs) == 1 && len(c.List) == 1 {
							id := ass.Lhs[0].(*ast.Ident).Name
							idT := infer.env.GetType2(c.List[0])
							infer.env.Define(nil, id, idT)
						}
					case *ast.ExprStmt:
					}
					infer.stmts(c.Body)
				})
			}
		}
	})
}

func (infer *FileInferrer) switchStmt(stmt *ast.SwitchStmt) {
	infer.withEnv(func() {
		if stmt.Tag != nil {
			infer.expr(stmt.Tag)
		}
		for _, el := range stmt.Body.List {
			c := el.(*ast.CaseClause)
			if c.List != nil {
				infer.exprs(c.List)
			}
			if c.Body != nil {
				infer.stmts(c.Body)
			}
		}
	})
}

func (infer *FileInferrer) caseClause(stmt *ast.CaseClause) {
	if stmt.List != nil {
		infer.exprs(stmt.List)
	}
	if stmt.Body != nil {
		infer.stmts(stmt.Body)
	}
}

func (infer *FileInferrer) branchStmt(stmt *ast.BranchStmt) {
}

func (infer *FileInferrer) deferStmt(stmt *ast.DeferStmt) {
	infer.expr(stmt.Call)
}

func (infer *FileInferrer) goStmt(stmt *ast.GoStmt) {
}

func (infer *FileInferrer) emptyStmt(stmt *ast.EmptyStmt) {
}

func (infer *FileInferrer) matchStmt(stmt *ast.MatchStmt) {
	infer.stmt(stmt.Init)
	infer.withOptType(stmt.Init, infer.env.GetType2(stmt.Init), func() {
		infer.stmt(stmt.Body)
	})
}

func (infer *FileInferrer) matchClause(stmt *ast.MatchClause) {
	if v, ok := stmt.Expr.(*ast.OkExpr); ok {
		t := infer.optType.Type.(types.ResultType).W
		infer.env.Define(v.X, v.X.(*ast.Ident).Name, t)
		infer.SetType(v.X, t)
	} else if v1, ok := stmt.Expr.(*ast.ErrExpr); ok {
		t := infer.env.Get("error")
		infer.env.Define(v1.X, v1.X.(*ast.Ident).Name, t)
		infer.SetType(v1.X, t)
	}
	infer.expr(stmt.Expr)
	infer.stmts(stmt.Body)
}

func (infer *FileInferrer) labeledStmt(stmt *ast.LabeledStmt) {
	infer.env.Define(stmt.Label, stmt.Label.Name, types.LabelType{})
	infer.stmt(stmt.Stmt)
}

func (infer *FileInferrer) ifLetStmt(stmt *ast.IfLetStmt) {
	infer.withEnv(func() {
		lhs := stmt.Ass.Lhs[0]
		var lhsT types.Type
		switch stmt.Op {
		case token.NONE:
			lhsT = types.NoneType{}
		case token.OK:
			lhsT = types.OkType{}
		case token.ERR:
			lhsT = types.ErrType{}
		case token.SOME:
			lhsT = types.SomeType{}
		default:
			panic("unreachable")
		}
		infer.SetType(lhs, lhsT)
		infer.stmt(stmt.Ass)
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	if stmt.Else != nil {
		infer.withEnv(func() {
			infer.stmt(stmt.Else)
		})
	}
}

func (infer *FileInferrer) ifStmt(stmt *ast.IfStmt) {
	infer.withEnv(func() {
		if stmt.Init != nil {
			infer.stmt(stmt.Init)
		}
		infer.expr(stmt.Cond)
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	if stmt.Else != nil {
		infer.withEnv(func() {
			infer.stmt(stmt.Else)
		})
	}
}
