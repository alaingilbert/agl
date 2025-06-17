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
	env         *Env
	f           *ast.File
	fset        *token.FileSet
	PackageName string
	returnType  types.Type
	optType     types.Type
}

func (infer *FileInferrer) sandboxed(clb func()) {
	old := infer.env
	nenv := old.CloneFull()
	infer.env = nenv
	clb()
	infer.env = old
}

func (infer *FileInferrer) withEnv(clb func()) {
	old := infer.env
	nenv := old.Clone()
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
		panic(fmt.Sprintf("%s type not found for %v %v", infer.env.fset.Position(n.Pos()), n, to(n)))
	}
	return t
}

func (infer *FileInferrer) SetTypeForce(a ast.Node, t types.Type) {
	infer.env.SetType(nil, a, t)
}

func (infer *FileInferrer) SetType(a ast.Node, t types.Type) {
	infer.SetType2(nil, a, t)
}

func (infer *FileInferrer) SetType2(p *Info, a ast.Node, t types.Type) {
	//fmt.Println("??SETTYPE", makeKey(a), a, t)
	//printCallers(100)
	if tt := infer.env.GetType(a); tt != nil {
		if !cmpTypes(tt, t) {
			if !TryCast[types.UntypedNumType](tt) {
				//return // TODO
				panic(fmt.Sprintf("type already declared for %s %s %v %v %v %v", infer.Pos(a), makeKey(a), a, to(a), infer.env.GetType(a), t))
			}
		}
	}
	infer.env.SetType(p, a, t)
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
	for _, i := range infer.f.Imports {
		infer.inferImport(i)
	}
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			infer.funcDecl(decl)
		case *ast.GenDecl:
			infer.genDecl(decl)
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
		}
	}
}

func (infer *FileInferrer) typeSpec(spec *ast.TypeSpec) {
	switch t := spec.Type.(type) {
	case *ast.StructType:
		infer.structType(spec.Name, t)
	case *ast.EnumType:
		infer.enumType(spec.Name, t)
	case *ast.InterfaceType:
		infer.interfaceType(spec.Name, t)
	}
}

func (infer *FileInferrer) interfaceType(name *ast.Ident, e *ast.InterfaceType) {
	if e.Methods.List != nil {
		for _, f := range e.Methods.List {
			for _, n := range f.Names {
				noop(n)
			}
		}
	}
	infer.env.Define(name, name.Name, types.InterfaceType{Name: name.Name})
}

func (infer *FileInferrer) enumType(name *ast.Ident, e *ast.EnumType) {
	var fields []types.EnumFieldType
	if e.Values != nil {
		for _, f := range e.Values.List {
			var elts []types.Type
			if f.Params != nil {
				for _, param := range f.Params.List {
					elts = append(elts, infer.env.GetType2(param.Type))
				}
			}
			fields = append(fields, types.EnumFieldType{Name: f.Name.Name, Elts: elts})
		}
	}
	infer.env.Define(name, name.Name, types.EnumType{Name: name.Name, Fields: fields})
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
	infer.env.Define(decl.Name, fnName, t)
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
				t := infer.env.GetType2(recv.Type)
				for _, name := range recv.Names {
					infer.env.SetType(nil, name, t)
					infer.env.Define(name, name.Name, t)
				}
			}
		}
		if decl.Type.TypeParams != nil {
			for _, param := range decl.Type.TypeParams.List {
				infer.expr(param.Type)
				t := infer.env.GetType(param.Type)
				for _, name := range param.Names {
					infer.env.SetType(nil, name, t)
					infer.env.Define(name, name.Name, types.GenericType{Name: name.Name, W: t, IsType: true})
				}
			}
		}
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				infer.expr(param.Type)
				t := infer.env.GetType(param.Type)
				for _, name := range param.Names {
					infer.env.Define(name, name.Name, t)
					infer.env.SetType(nil, name, t)
				}
			}
		}

		var returnTyp types.Type
		if decl.Type.Result == nil {
			returnTyp = types.VoidType{}
		} else {
			infer.expr(decl.Type.Result)
			returnTyp = infer.env.GetType2(decl.Type.Result)
			infer.SetType(decl.Type.Result, returnTyp)
		}
		infer.returnType = returnTyp
		if decl.Body != nil {
			// implicit return
			cond1 := len(decl.Body.List) == 1 ||
				(len(decl.Body.List) == 2 && TryCast[*ast.EmptyStmt](decl.Body.List[1]))
			if cond1 && decl.Type.Result != nil && TryCast[*ast.ExprStmt](decl.Body.List[0]) {
				decl.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: decl.Body.List[0].(*ast.ExprStmt).X}}
			}
			infer.stmt(decl.Body)
		}
		infer.returnType = nil
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
			t := infer.env.GetType(param.Type)
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
		} else {
			r := decl.Recv.List[0]
			infer.expr(r.Type)
			structName := infer.GetType(r.Type).(types.StructType).Name
			outEnv.Define(decl.Name, fmt.Sprintf("%s.%s", structName, fnName), ft)
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
	default:
		panic(fmt.Sprintf("unknown expression %v", to(e)))
	}
	if infer.optType != nil {
		infer.tryConvertType(e, infer.optType)
	}
}

func (infer *FileInferrer) tryConvertType(e ast.Expr, optType types.Type) {
	if infer.env.GetType(e) == nil {
		infer.SetType(e, optType)
	} else if _, ok := infer.GetType(e).(types.UntypedNumType); ok {
		if TryCast[types.U8Type](optType) ||
			TryCast[types.U16Type](optType) ||
			TryCast[types.U32Type](optType) ||
			TryCast[types.U64Type](optType) ||
			TryCast[types.I8Type](optType) ||
			TryCast[types.I16Type](optType) ||
			TryCast[types.I32Type](optType) ||
			TryCast[types.I64Type](optType) ||
			TryCast[types.IntType](optType) ||
			TryCast[types.UintType](optType) {
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
	default:
		panic(fmt.Sprintf("unknown statement %v", to(stmt)))
	}
}

func (infer *FileInferrer) basicLit(expr *ast.BasicLit) {
	switch expr.Kind {
	case token.STRING:
		infer.SetType(expr, types.StringType{})
	case token.INT:
		infer.SetType(expr, types.UntypedNumType{})
	case token.CHAR:
		infer.SetType(expr, types.CharType{})
	default:
		panic(fmt.Sprintf("unknown basic literal %v %v", to(expr), expr.Kind))
	}
}

func (infer *FileInferrer) callExpr(expr *ast.CallExpr) {
	tmpFn := func(idT types.Type, call *ast.SelectorExpr) {
		fnName := call.Sel.Name
		switch idTT := idT.(type) {
		case types.ArrayType:
			arr := idTT
			if fnName == "Filter" {
				fnT := infer.env.GetFn("agl.Vec.Filter").T("T", arr.Elt)
				infer.SetType(expr.Args[0], fnT.Params[1])
				infer.SetType(expr, fnT.Return)
			} else if fnName == "Map" {
				fnT := infer.env.GetFn("agl.Vec.Map").T("T", arr.Elt)
				infer.SetType(expr.Args[0], fnT.GetParam(1))
				infer.SetType(expr, fnT.Return)
			} else if fnName == "Reduce" {
				r := infer.env.GetType2(expr.Args[0])
				fnT := infer.env.GetFn("agl.Vec.Reduce").T("T", arr.Elt).T("R", r)
				infer.SetType(expr.Args[1], fnT.Params[2])
				infer.SetType(expr, fnT.Return)
			} else if fnName == "Find" {
				fnT := infer.env.GetFn("agl.Vec.Find").T("T", arr.Elt)
				infer.SetType(expr, fnT.Return)
			} else if fnName == "Sum" {
				fnT := infer.env.GetFn("agl.Vec.Sum").T("T", arr.Elt)
				infer.SetType(expr, fnT.Return)
			} else if fnName == "Joined" {
				fnT := infer.env.GetFn("agl.Vec.Joined")
				assertf(cmpTypes(idT, fnT.Params[0]), "type mismatch, wants: %s, got: %s", fnT.Params[0], idT)
				infer.SetType(expr, fnT.Return)
			} else {
				fnFullName := fmt.Sprintf("agl.Vec.%s", fnName)
				if fnT := infer.env.Get(fnFullName); fnT != nil {
					fnT1 := fnT.(types.FuncType).T("T", arr.Elt)
					retT := Or[types.Type](fnT1.Return, types.VoidType{})
					infer.SetType(expr.Fun, fnT1)
					infer.SetType(expr, retT)
				} else {
					assertf(false, "%s: method '%s' of type Vec does not exists", infer.Pos(call.Sel), fnName)
				}
			}
			return
		case types.StructType:
			name := fmt.Sprintf("%s.%s", idTT.Name, call.Sel.Name)
			if idTT.Pkg != "" {
				name = idTT.Pkg + "." + name
			}
			nameT := infer.env.Get(name)
			assertf(nameT != nil, "method not found '%s' in struct of type '%v'", call.Sel.Name, idTT.Name)
			toReturn := infer.env.Get(name).(types.FuncType).Return
			toReturn = alterResultBubble(infer.returnType, toReturn)
			infer.SetType(expr, toReturn)
			return
		case types.InterfaceType:
			return
		case types.EnumType:
			infer.SetType(expr, types.EnumType{Name: idTT.Name, SubTyp: call.Sel.Name, Fields: idTT.Fields})
			return
		case types.PackageType:
			pkgT := infer.env.Get(idTT.Name)
			assertf(pkgT != nil, "package not found '%s'", idTT.Name)
			name := fmt.Sprintf("%s.%s", idTT.Name, call.Sel.Name)
			nameT := infer.env.Get(name)
			assertf(nameT != nil, "not found '%s' in package '%v'", call.Sel.Name, idTT.Name)
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
			return
		case types.OptionType:
			if InArray(fnName, []string{"IsNone", "IsSome", "Unwrap", "UnwrapOr"}) {
				fnT := infer.env.GetFn("agl.Option." + fnName)
				if fnName == "UnwrapOr" {
					fnT = fnT.T("T", idTT.W)
				} else if fnName == "Unwrap" {
					fnT = fnT.T("T", idTT.W)
				}
				infer.SetType(expr, fnT.Return)
				return
			}
		case types.ResultType:
			if InArray(fnName, []string{"IsOk", "IsErr", "Unwrap", "UnwrapOr", "Err"}) {
				fnT := infer.env.GetFn("agl.Result." + fnName)
				if fnName == "UnwrapOr" {
					fnT = fnT.T("T", idTT.W)
				} else if fnName == "Unwrap" {
					fnT = fnT.T("T", idTT.W)
				} else if fnName == "Err" {
					panic("user cannot call Err")
				}
				infer.SetType(expr, fnT.Return)
				return
			}
		}
		assertf(false, "Unresolved reference '%s'", fnName)
	}

	switch call := expr.Fun.(type) {
	case *ast.SelectorExpr:
		var exprFunT types.Type
		switch callXT := call.X.(type) {
		case *ast.Ident:
			exprFunT = infer.env.Get(callXT.Name)
		case *ast.CallExpr, *ast.BubbleResultExpr, *ast.BubbleOptionExpr:
			infer.expr(callXT)
			exprFunT = infer.GetType(callXT)
		default:
			panic(fmt.Sprintf("%v %v", call.X, to(call.X)))
		}
		tmpFn(exprFunT, call)
		infer.SetType(call.X, exprFunT)
		infer.inferVecExtensions(exprFunT, call, expr)
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
		}
		callT := infer.env.Get(call.Name)
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
				infer.optType = oArg
				infer.expr(arg)
				infer.optType = nil
				if v, ok := oArg.(types.EllipsisType); ok {
					oArg = v.Elt
				}
				got := infer.GetType(arg)
				assertf(cmpTypes(oArg, got), "types not equal, %v %v", oArg, got)
			}
		default:
			panic(fmt.Sprintf("%v %v", expr.Fun, to(expr.Fun)))
		}
		parentInfo := infer.env.GetNameInfo(call.Name)
		infer.SetType2(parentInfo, call, callT)
	}
	if exprFunT := infer.env.GetType(expr.Fun); exprFunT != nil {
		if v, ok := exprFunT.(types.FuncType); ok {
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

func (infer *FileInferrer) inferVecExtensions(idT types.Type, exprT *ast.SelectorExpr, expr *ast.CallExpr) {
	if idTArr, ok := idT.(types.ArrayType); ok {
		eltT := idTArr.Elt
		fnName := exprT.Sel.Name
		exprPos := infer.Pos(expr)
		if fnName == "Filter" {
			filterFnT := infer.env.GetFn("agl.Vec.Filter").T("T", eltT)
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
			infer.SetType(expr, types.ArrayType{Elt: ft.Params[0]})
			infer.SetType(exprT.Sel, filterFnT)
		} else if fnName == "Map" {
			mapFnT := infer.env.GetFn("agl.Vec.Map").T("T", eltT)
			ft := mapFnT.GetParam(1).(types.FuncType)
			exprArg0 := expr.Args[0]
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(arg0, ft)
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if tmp, ok := exprArg0.(*ast.FuncLit); ok {
					infer.expr(tmp)
					retT := infer.GetTypeFn(tmp).Return
					infer.SetType(expr, types.ArrayType{Elt: retT})
				}
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
		} else if fnName == "Reduce" {
			exprArg0 := expr.Args[0]
			infer.expr(exprArg0)
			reduceFnT := infer.env.GetFn("agl.Vec.Reduce").T("T", eltT)
			infer.SetType(exprT.Sel, reduceFnT)
			ft := reduceFnT.GetParam(2).(types.FuncType)
			if _, ok := infer.GetType(exprArg0).(types.UntypedNumType); ok {
				ft = ft.T("R", eltT)
			}
			if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
				infer.SetType(expr.Args[1], ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
		} else if fnName == "Find" {
			findFnT := infer.env.GetFn("agl.Vec.Find").T("T", eltT)
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
			infer.SetType(expr, types.OptionType{W: ft.Params[0]})
			infer.SetType(exprT.Sel, findFnT)
		} else if fnName == "Sum" {
			sumFnT := infer.env.GetFn("agl.Vec.Sum").T("T", eltT)
			infer.SetType(exprT.Sel, sumFnT)
		} else if fnName == "Joined" {
			joinedFnT := infer.env.GetFn("agl.Vec.Joined")
			infer.SetType(exprT.Sel, joinedFnT)
		} else {
			funT := infer.GetTypeFn(expr.Fun)
			ft := infer.env.GetFn("agl.Vec." + fnName)
			assert(len(ft.TypeParams) >= 1, "agl.Vec should have at least one generic parameter")
			want, got := types.ArrayType{Elt: ft.TypeParams[0].(types.GenericType).W}, idTArr
			assertf(cmpTypes(want, got), "%s: cannot use %v as %v for %s", infer.Pos(exprT.Sel), got.GoStr(), want.GoStr(), fnName)
			// Go through the arguments and get a mapping of "generic name" to "concrete type" (eg: {"T":int})
			genericMapping := make(map[string]types.Type)
			for i, arg := range expr.Args {
				if argFn, ok := arg.(*ast.ShortFuncLit); ok { // TODO not working at all
					infer.SetType(argFn, ft.GetParam(i))
					infer.expr(argFn)
					//ftt := infer.GetTypeFn(argFn)
					//rT := ftt.Return
					//infer.SetType(expr, types.ArrayType{Elt: rT})
					//infer.SetType(exprT.Sel, funT.T("R", rT))
					//infer.SetType(argFn, ftt)
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
				funT = funT.ReplaceGenericParameter(k, v)
			}
			infer.SetType(expr.Fun, funT)
			infer.SetType(exprT.Sel, ft)
		}
	}
}

func (infer *FileInferrer) funcLit(expr *ast.FuncLit) {
	if infer.optType != nil {
		infer.SetType(expr, infer.optType)
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
		if infer.optType != nil {
			infer.SetType(expr, infer.optType)
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
	infer.expr(expr.Result)
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
	ft := types.FuncType{
		Params: paramsT,
		Return: infer.env.GetType(expr.Result),
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

func (infer *FileInferrer) okExpr(expr *ast.OkExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.OkType{W: infer.env.GetType(expr.X)})
}

func (infer *FileInferrer) errExpr(expr *ast.ErrExpr) {
	infer.expr(expr.X)
	if v, ok := expr.X.(*ast.BasicLit); ok && v.Kind == token.STRING {
		expr.X = &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "errors"}, Sel: &ast.Ident{Name: "New"}}, Args: []ast.Expr{v}}
	}
	infer.SetType(expr, types.ErrType{W: infer.env.GetType(expr.X), T: infer.returnType.(types.ResultType).W})
}

func (infer *FileInferrer) chanType(expr *ast.ChanType) {
	infer.expr(expr.Value)
}

func (infer *FileInferrer) unaryExpr(expr *ast.UnaryExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.StarType{X: infer.GetType(expr.X)})
}

func (infer *FileInferrer) typeAssertExpr(expr *ast.TypeAssertExpr) {
	infer.expr(expr.X)
	if expr.Type != nil {
		infer.expr(expr.Type)
	}
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

func (infer *FileInferrer) noneExpr(expr *ast.NoneExpr) {
	infer.SetType(expr, types.NoneType{W: infer.returnType.(types.OptionType).W}) // TODO
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
	if infer.optType != nil {
		expected := infer.optType.(types.TupleType).Elts
		for i, x := range expr.Values {
			if _, ok := infer.GetType(x).(types.UntypedNumType); ok {
				infer.SetType(x, expected[i])
			}
		}
	} else {
		var elts []types.Type
		for _, v := range expr.Values {
			elts = append(elts, infer.GetType(v))
		}
		infer.SetType(expr, types.TupleType{Name: "AglTupleStruct1", Elts: elts})
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

func isNumericType(t types.Type) bool {
	return TryCast[types.I64Type](t) ||
		TryCast[types.I32Type](t) ||
		//TryCast[types.I16Type](t) ||
		//TryCast[types.I8Type](t) ||
		TryCast[types.IntType](t) ||
		//TryCast[types.U64Type](t) ||
		//TryCast[types.U32Type](t) ||
		//TryCast[types.U16Type](t) ||
		TryCast[types.U8Type](t)
	//TryCast[types.F64Type](t) ||
	//TryCast[types.F32Type](t)
}

func cmpTypesLoose(a, b types.Type) bool {
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
				return cmpTypes(aa.Elts[i], bb.Elts[i])
			}
			return true
		}
		return false
	}
	if TryCast[types.AnyType](a) || TryCast[types.AnyType](b) {
		return true
	}
	if TryCast[types.BoolType](a) || TryCast[types.BoolType](b) {
		return true
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
	if TryCast[types.ArrayType](a) && TryCast[types.ArrayType](b) {
		aa := MustCast[types.ArrayType](a)
		bb := MustCast[types.ArrayType](b)
		return cmpTypes(aa.Elt, bb.Elt)
	}
	if TryCast[types.EnumType](a) || TryCast[types.EnumType](b) {
		return true // TODO
	}
	if TryCast[types.OptionType](a) && TryCast[types.OptionType](b) {
		return cmpTypes(a.(types.OptionType).W, b.(types.OptionType).W)
	}
	if TryCast[types.ResultType](a) && TryCast[types.ResultType](b) {
		return cmpTypes(a.(types.ResultType).W, b.(types.ResultType).W)
	}
	if a == b {
		return true
	}
	return false
}

func (infer *FileInferrer) selectorExpr(expr *ast.SelectorExpr) {
	selType := infer.env.Get(expr.X.(*ast.Ident).Name)
	switch v := selType.(type) {
	case types.StructType:
		fieldName := expr.Sel.Name
		for _, f := range v.Fields {
			if f.Name == fieldName {
				infer.SetType(expr.X, selType)
				infer.SetType(expr, f.Typ)
				return
			}
		}
	case types.EnumType:
		enumName := expr.X.(*ast.Ident).Name
		fieldName := expr.Sel.Name
		validFields := make([]string, 0, len(v.Fields))
		for _, f := range v.Fields {
			validFields = append(validFields, f.Name)
		}
		assertf(InArray(fieldName, validFields), "%d: enum %s has no field %s", expr.Sel.Pos(), enumName, fieldName)
		infer.SetType(expr.X, selType)
		infer.SetType(expr, selType)
	case types.TupleType:
		argIdx, err := strconv.Atoi(expr.Sel.Name)
		if err != nil {
			panic("tuple arg index must be int")
		}
		infer.SetType(expr.X, v)
		infer.SetType(expr, v.Elts[argIdx])
	}
}

func (infer *FileInferrer) bubbleResultExpr(expr *ast.BubbleResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X).(types.ResultType).W)
}

func (infer *FileInferrer) bubbleOptionExpr(expr *ast.BubbleOptionExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X).(types.OptionType).W)
}

func (infer *FileInferrer) compositeLit(expr *ast.CompositeLit) {
	switch v := expr.Type.(type) {
	case *ast.ArrayType:
		infer.SetType(expr, types.ArrayType{Elt: infer.env.GetType2(v.Elt)})
		return
	case *ast.Ident:
		infer.SetType(expr, infer.env.Get(v.Name))
		return
	case *ast.MapType:
		infer.SetType(expr, types.MapType{K: infer.env.GetType2(v.Key), V: infer.env.GetType2(v.Value)})
		return
	case *ast.SelectorExpr:
		infer.SetType(expr, infer.env.Get(fmt.Sprintf("%s.%s", v.X.(*ast.Ident).Name, v.Sel.Name)))
		return
	default:
		panic(fmt.Sprintf("%v", to(expr.Type)))
	}
}

func (infer *FileInferrer) arrayType(expr *ast.ArrayType) {
	if expr.Len != nil {
		infer.expr(expr.Len)
	}
	infer.expr(expr.Elt)
	infer.SetType(expr, types.ArrayType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) indexExpr(expr *ast.IndexExpr) {
	infer.expr(expr.X)
	infer.expr(expr.Index)
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
		for _, name := range spec.Names {
			infer.exprs(spec.Values)
			t := infer.env.GetType2(spec.Type)
			infer.SetType(name, t)
			infer.env.Define(name, name.Name, t)
		}
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
		xT := infer.GetType(stmt.X)
		if stmt.Key != nil {
			// TODO find correct type for map
			name := stmt.Key.(*ast.Ident).Name
			infer.env.Define(stmt.Key, name, types.IntType{})
		}
		if stmt.Value != nil {
			name := stmt.Value.(*ast.Ident).Name
			if _, ok := xT.(types.StringType); ok {
				infer.env.Define(stmt.Value, name, types.I32Type{})
			} else if v, ok := xT.(types.ArrayType); ok {
				infer.env.Define(stmt.Value, name, v.Elt)
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
	myDefine := func(parentInfo *Info, n ast.Node, name string, typ types.Type) {
		assertf(name != "_", "%s: No new variables on the left side of ':='", stmt.Tok)
		infer.env.Define(n, name, typ)
	}
	assignFn := func(n ast.Node, name string, typ types.Type) {
		op := stmt.Tok
		f := Ternary(op == token.DEFINE, myDefine, infer.env.Assign)
		var parentInfo *Info
		if op != token.DEFINE {
			parentInfo = infer.env.GetNameInfo(name)
		}
		f(parentInfo, n, name, typ)
	}
	if len(stmt.Rhs) == 1 {
		rhs := stmt.Rhs[0]
		infer.expr(rhs)
		if len(stmt.Lhs) == 1 {
			lhs := stmt.Lhs[0]
			var lhsID *ast.Ident
			switch v := lhs.(type) {
			case *ast.Ident:
				lhsID = v
			case *ast.IndexExpr:
				lhsID = v.X.(*ast.Ident)
				return // tODO
			}
			rhsT := infer.GetType(rhs)
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
			assignFn(lhsID, lhsID.Name, infer.GetType(lhsID))
		} else {
			if rhs1, ok := rhs.(*ast.TupleExpr); ok {
				for i, x := range rhs1.Values {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*ast.Ident](lhs)
					infer.SetType(lhs, infer.GetType(x))
					assignFn(lhsID, lhsID.Name, infer.GetType(lhsID))
				}
			} else if rhs2, ok := infer.env.GetType(rhs).(types.EnumType); ok {
				for i, e := range stmt.Lhs {
					lit := rhs2.SubTyp
					fields := rhs2.Fields
					// AGL: fields.Find({ $0.name == lit })
					f := Find(fields, func(f types.EnumFieldType) bool { return f.Name == lit })
					assert(f != nil)
					assignFn(e, e.(*ast.Ident).Name, f.Elts[i])
				}
				return
			} else if rhsId, ok := rhs.(*ast.Ident); ok {
				rhsIdT := infer.env.Get(rhsId.Name)
				if rhs3, ok := rhsIdT.(types.TupleType); ok {
					for i, x := range rhs3.Elts {
						lhs := stmt.Lhs[i]
						lhsID := MustCast[*ast.Ident](lhs)
						infer.SetType(lhs, x)
						assignFn(lhsID, lhsID.Name, infer.GetType(lhsID))
					}
				}
			}
		}
	} else {
		for i := range stmt.Lhs {
			lhs := stmt.Lhs[i]
			rhs := stmt.Rhs[i]
			infer.expr(rhs)
			lhsID := MustCast[*ast.Ident](lhs)
			infer.SetType(lhsID, infer.GetType(rhs))
			assignFn(lhsID, lhsID.Name, infer.GetType(lhsID))
		}
	}
}

func (infer *FileInferrer) exprStmt(stmt *ast.ExprStmt) {
	infer.expr(stmt.X)
}

func (infer *FileInferrer) returnStmt(stmt *ast.ReturnStmt) {
	infer.optType = infer.returnType
	infer.expr(stmt.Result)
	infer.optType = nil
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
	if infer.GetType(expr.X) != nil && infer.GetType(expr.Y) != nil {
		if isNumericType(infer.GetType(expr.X)) && TryCast[types.UntypedNumType](infer.GetType(expr.Y)) {
			infer.SetType(expr.Y, infer.GetType(expr.X))
		}
		if isNumericType(infer.GetType(expr.Y)) && TryCast[types.UntypedNumType](infer.GetType(expr.X)) {
			infer.SetType(expr.X, infer.GetType(expr.Y))
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

func (infer *FileInferrer) switchStmt(stmt *ast.SwitchStmt) {
	if stmt.Init != nil {
		infer.stmt(stmt.Init)
	}
	if stmt.Tag != nil {
		infer.expr(stmt.Tag)
	}
	infer.stmt(stmt.Body)
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
}

func (infer *FileInferrer) goStmt(stmt *ast.GoStmt) {
}

func (infer *FileInferrer) emptyStmt(stmt *ast.EmptyStmt) {
}

func (infer *FileInferrer) typeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	if stmt.Init != nil {
		infer.stmt(stmt.Init)
	}
	infer.stmt(stmt.Assign)
	infer.stmt(stmt.Body)
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
