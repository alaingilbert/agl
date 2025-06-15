package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	"path"
	"strconv"
	"strings"
)

type Inferrer struct {
	fset *token.FileSet
	Env  *Env
}

func NewInferrer(fset *token.FileSet) *Inferrer {
	return &Inferrer{fset: fset, Env: NewEnv(fset)}
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

func (infer *FileInferrer) GetType(p ast.Node) types.Type {
	t := infer.env.GetType(p)
	if t == nil {
		panic(fmt.Sprintf("%s type not found for %v %v", infer.env.fset.Position(p.Pos()), p, to(p)))
	}
	return t
}

func (infer *FileInferrer) SetTypeForce(a ast.Node, t types.Type) {
	infer.env.SetType(a, t)
}

func (infer *FileInferrer) SetType(a ast.Node, t types.Type) {
	//fmt.Println("??SETTYPE", makeKey(a), a, t)
	//printCallers(100)
	if tt := infer.env.GetType(a); tt != nil {
		if !cmpTypes(tt, t) {
			if !TryCast[types.UntypedNumType](tt) {
				//return // TODO
				panic(fmt.Sprintf("type already declared for %s %s %v %v %v %v", infer.fset.Position(a.Pos()), makeKey(a), a, to(a), infer.env.GetType(a), t))
			}
		}
	}
	infer.env.SetType(a, t)
}

func (infer *FileInferrer) Infer() {
	infer.PackageName = infer.f.Name.Name
	for _, i := range infer.f.Imports {
		var name string
		if i.Name != nil {
			name = i.Name.Name
		} else {
			name = path.Base(i.Path.Value)
		}
		name = strings.ReplaceAll(name, `"`, ``)
		if infer.env.Get(name) == nil {
			infer.env.Define(name, types.PackageType{Name: name})
		}
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
	infer.env.Define(name.Name, types.InterfaceType{Name: name.Name})
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
	infer.env.Define(name.Name, types.EnumType{Name: name.Name, Fields: fields})
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
	infer.env.Define(name.Name, types.StructType{Name: name.Name, Fields: fields})
}

func (infer *FileInferrer) funcDecl(decl *ast.FuncDecl) {
	var t types.FuncType
	outEnv := infer.env
	infer.sandboxed(func() {
		t = infer.getFuncDeclType(decl, outEnv)
	})
	infer.SetType(decl, t)
	infer.env.Define(decl.Name.Name, t)
}

func (infer *FileInferrer) funcDecl2(decl *ast.FuncDecl) {
	infer.withEnv(func() {
		if decl.Recv != nil {
			for _, recv := range decl.Recv.List {
				t := infer.env.GetType2(recv.Type)
				for _, name := range recv.Names {
					infer.env.SetType(name, t)
					infer.env.Define(name.Name, t)
				}
			}
		}
		if decl.Type.TypeParams != nil {
			for _, param := range decl.Type.TypeParams.List {
				infer.expr(param.Type)
				t := infer.env.GetType(param.Type)
				for _, name := range param.Names {
					infer.env.SetType(name, t)
					infer.env.Define(name.Name, types.GenericType{Name: name.Name, W: t})
				}
			}
		}
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				infer.expr(param.Type)
				t := infer.env.GetType(param.Type)
				for _, name := range param.Names {
					infer.env.Define(name.Name, t)
					infer.env.SetType(name, t)
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
			if len(decl.Body.List) == 1 && decl.Type.Result != nil && TryCast[*ast.ExprStmt](decl.Body.List[0]) {
				decl.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: decl.Body.List[0].(*ast.ExprStmt).X}}
			}
			infer.stmt(decl.Body)
		}
		infer.returnType = nil
	})
}

func (infer *FileInferrer) getFuncDeclType(decl *ast.FuncDecl, outEnv *Env) types.FuncType {
	var returnT types.Type
	var paramsT []types.Type
	if decl.Type.TypeParams != nil {
		for _, typeParam := range decl.Type.TypeParams.List {
			infer.expr(typeParam.Type)
			t := infer.env.GetType(typeParam.Type)
			for _, name := range typeParam.Names {
				paramsT = append(paramsT, t)
				infer.env.Define(name.Name, types.GenericType{Name: name.Name, W: t})
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
		returnT = infer.env.GetType(decl.Type.Result)
		if r, ok := returnT.(types.ResultType); ok {
			r.Bubble = true
			returnT = r
		} else if r, ok := returnT.(types.OptionType); ok {
			r.Bubble = true
			returnT = r
		}
	}
	ft := types.FuncType{
		Name:   decl.Name.Name,
		Params: paramsT,
		Return: returnT,
	}
	if decl.Recv != nil {
		r := decl.Recv.List[0]
		infer.expr(r.Type)
		structName := infer.GetType(r.Type).(types.StructType).Name
		outEnv.Define(fmt.Sprintf("%s.%s", structName, decl.Name), ft)
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
			//TryCast[types.U16Type](optType) ||
			//TryCast[types.U32Type](optType) ||
			//TryCast[types.U64Type](optType) ||
			//TryCast[types.I8Type](optType) ||
			//TryCast[types.I16Type](optType) ||
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
	default:
		panic(fmt.Sprintf("unknown basic literal %v %v", to(expr), expr.Kind))
	}
}

func (infer *FileInferrer) callExpr(expr *ast.CallExpr) {
	tmpFn := func(idT types.Type, call *ast.SelectorExpr) {
		if arr, ok := idT.(types.ArrayType); ok {
			fnName := call.Sel.Name
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
				assertf(false, "%s: method '%s' of type Vec does not exists", infer.fset.Position(call.Sel.Pos()), fnName)
			}
		}
	}

	switch call := expr.Fun.(type) {
	case *ast.SelectorExpr:
		switch id := call.X.(type) {
		case *ast.Ident:
			idT1 := infer.env.Get(id.Name)
			tmpFn(idT1, call)
			if l := infer.env.Get(id.Name); l != nil {
				infer.SetType(id, l)
				if lT, ok := l.(types.StructType); ok {
					name := fmt.Sprintf("%s.%s", lT.Name, call.Sel.Name)
					toReturn := infer.env.Get(name).(types.FuncType).Return
					toReturn = alterResultBubble(infer.returnType, toReturn)
					infer.SetType(expr, toReturn)
				} else if _, ok := l.(types.PackageType); ok {
					name := fmt.Sprintf("%s.%s", id.Name, call.Sel.Name)
					fnT := infer.env.Get(name).(types.FuncType)
					infer.SetType(expr, fnT.Return)
				} else if o, ok := l.(types.EnumType); ok {
					infer.SetType(expr, types.EnumType{Name: o.Name, SubTyp: call.Sel.Name, Fields: o.Fields})
				}
			}
			idT := infer.env.Get(id.Name)
			infer.inferVecExtensions(idT, call, expr)
		default:
			infer.expr(id)
			idT := infer.GetType(id)
			tmpFn(idT, call)
			if lT, ok := idT.(types.StructType); ok {
				name := fmt.Sprintf("%s.%s", lT.Name, call.Sel.Name)
				toReturn := infer.env.Get(name).(types.FuncType).Return
				toReturn = alterResultBubble(infer.returnType, toReturn)
				infer.SetType(expr, toReturn)
			}
			infer.inferVecExtensions(idT, call, expr)
		}
		infer.exprs(expr.Args)
	case *ast.Ident:
		if call.Name == "Some" {
			infer.SetType(call, types.SomeType{W: infer.returnType.(types.OptionType).W})
			return
		} else if call.Name == "Err" {
			if v, ok := expr.Args[0].(*ast.BasicLit); ok && v.Kind == token.STRING {
				expr.Args[0] = &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "errors"}, Sel: &ast.Ident{Name: "New"}}, Args: []ast.Expr{v}}
			}
			infer.SetType(call, types.ErrType{W: infer.returnType.(types.ResultType).W})
			return
		} else if call.Name == "Ok" {
			infer.SetType(call, types.OkType{W: infer.returnType.(types.ResultType).W})
			return
		} else {
			if call.Name == "make" {
				fnT := infer.env.Get("make").(types.FuncType)
				switch expr.Args[0].(type) {
				case *ast.ArrayType:
					fnT = fnT.ReplaceGenericParameter("T", infer.env.GetType2(expr.Args[0].(*ast.ArrayType)))
					infer.SetType(expr, fnT.Return)
				case *ast.ChanType:
					fnT = fnT.ReplaceGenericParameter("T", infer.env.GetType2(expr.Args[0].(*ast.ChanType)))
					infer.SetType(expr, fnT.Return)
				}
			}
			callT := infer.env.Get(call.Name)
			ft := callT.(types.FuncType)
			oParams := ft.Params
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
				if _, ok := arg.(*ast.ShortFuncLit); ok {
					//paramI := oParams[i].(types.FuncType)
					//ft := paramI
					//infer.SetType(f, ft)
					//infer.expr(arg)
				}
			}
			infer.SetType(call, callT)
		}
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
		} else { // Type casting
			// TODO
		}
	}
}

func alterResultBubble(fnReturn types.Type, curr types.Type) (out types.Type) {
	out = curr
	if fnReturn != nil {
		if _, ok := fnReturn.(types.ResultType); !ok {
			if tmp, ok := curr.(types.ResultType); ok {
				if fnReturnOpt, ok := fnReturn.(types.OptionType); ok {
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
		if _, ok := fnReturn.(types.OptionType); !ok {
			if tmp, ok := curr.(types.OptionType); ok {
				tmp.Bubble = false
				out = tmp
			}
		}
	}
	return
}

func (infer *FileInferrer) inferVecExtensions(idT types.Type, exprT *ast.SelectorExpr, expr *ast.CallExpr) {
	if idTArr, ok := idT.(types.ArrayType); ok {
		exprPos := infer.fset.Position(expr.Pos())
		if exprT.Sel.Name == "Filter" {
			ft := infer.env.Get("agl.Vec.Filter").(types.FuncType).GetParam(1).(types.FuncType)
			ft = ft.ReplaceGenericParameter("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetTypeForce(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			infer.SetTypeForce(expr, types.ArrayType{Elt: ft.Params[0]})

		} else if exprT.Sel.Name == "Map" {
			ft := infer.env.GetFn("agl.Vec.Map").GetParam(1).(types.FuncType).T("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetTypeForce(arg0, ft)
				infer.expr(arg0)
				infer.SetTypeForce(expr, types.ArrayType{Elt: infer.GetType(arg0).(types.FuncType).Return})
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if tmp, ok := exprArg0.(*ast.FuncLit); ok {
					infer.expr(tmp)
					infer.SetTypeForce(expr, types.ArrayType{Elt: infer.GetType(exprArg0.(*ast.FuncLit)).(types.FuncType).Return})
				}
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
		} else if exprT.Sel.Name == "Reduce" {
			exprArg0 := expr.Args[0]
			infer.expr(exprArg0)
			elTyp := idTArr.Elt
			ft := infer.env.Get("agl.Vec.Reduce").(types.FuncType).GetParam(2).(types.FuncType)
			ft = ft.ReplaceGenericParameter("T", elTyp)
			if _, ok := infer.GetType(exprArg0).(types.UntypedNumType); ok {
				ft = ft.ReplaceGenericParameter("R", elTyp)
			}
			if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
				infer.SetTypeForce(expr.Args[1], ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
		} else if exprT.Sel.Name == "Find" {
			ft := infer.env.Get("agl.Vec.Find").(types.FuncType).GetParam(1).(types.FuncType)
			ft = ft.ReplaceGenericParameter("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetTypeForce(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			infer.SetTypeForce(expr, types.OptionType{W: ft.Params[0]})
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
				infer.env.Define(fmt.Sprintf("$%d", i), param)
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
						ft = ft.ReplaceGenericParameter(t.Name, infer.env.GetType(returnStmt.X))
						infer.SetTypeForce(expr, ft)
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
}

func (infer *FileInferrer) typeAssertExpr(expr *ast.TypeAssertExpr) {
	infer.expr(expr.X)
	if expr.Type != nil {
		infer.expr(expr.Type)
	}
}

func (infer *FileInferrer) noneExpr(expr *ast.NoneExpr) {
	infer.SetType(expr, types.NoneType{W: infer.returnType.(types.OptionType).W}) // TODO
}

func (infer *FileInferrer) starExpr(expr *ast.StarExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.StarType{X: infer.env.GetType(expr.X)})
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
	if TryCast[types.BoolType](a) || TryCast[types.BoolType](b) {
		return true
	}
	if TryCast[types.GenericType](a) || TryCast[types.GenericType](b) {
		return true
	}
	if TryCast[types.StructType](a) || TryCast[types.StructType](b) {
		return true // TODO
	}
	if TryCast[types.ArrayType](a) || TryCast[types.ArrayType](b) {
		return true // TODO
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
		infer.SetType(expr, types.MapType{})
		return
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
			infer.env.Define(name.Name, t)
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
			infer.env.Define(name, types.IntType{})
		}
		if stmt.Value != nil {
			name := stmt.Value.(*ast.Ident).Name
			if _, ok := xT.(types.StringType); ok {
				infer.env.Define(name, types.I32Type{})
			} else if v, ok := xT.(types.ArrayType); ok {
				infer.env.Define(name, v.Elt)
			}
		}
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
}

func (infer *FileInferrer) assignStmt(stmt *ast.AssignStmt) {
	myDefine := func(name string, typ types.Type) {
		assertf(name != "_", "%s: No new variables on the left side of ':='", stmt.Tok)
		infer.env.Define(name, typ)
	}
	assignFn := func(name string, typ types.Type) {
		op := stmt.Tok
		f := Ternary(op == token.DEFINE, myDefine, infer.env.Assign)
		f(name, typ)
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
				assertf(TryCast[types.OptionType](rhsT), "%s: try to destructure a non-Option type into an OptionType", infer.fset.Position(lhs.Pos()))
				infer.SetTypeForce(lhs, rhsT)
			case types.ErrType, types.OkType:
				assertf(TryCast[types.ResultType](rhsT), "%s: try to destructure a non-Result type into an ResultType", infer.fset.Position(lhs.Pos()))
				infer.SetTypeForce(lhs, rhsT)
			default:
				infer.SetType(lhs, rhsT)
			}
			assignFn(lhsID.Name, infer.GetType(lhsID))
		} else {
			if rhs1, ok := rhs.(*ast.TupleExpr); ok {
				for i, x := range rhs1.Values {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*ast.Ident](lhs)
					infer.SetType(lhs, infer.GetType(x))
					assignFn(lhsID.Name, infer.GetType(lhsID))
				}
			} else if rhs2, ok := infer.env.GetType(rhs).(types.EnumType); ok {
				for i, e := range stmt.Lhs {
					lit := rhs2.SubTyp
					fields := rhs2.Fields
					// AGL: fields.Find({ $0.name == lit })
					f := Find(fields, func(f types.EnumFieldType) bool { return f.Name == lit })
					assert(f != nil)
					assignFn(e.(*ast.Ident).Name, f.Elts[i])
				}
				return
			} else if rhsId, ok := rhs.(*ast.Ident); ok {
				rhsIdT := infer.env.Get(rhsId.Name)
				if rhs3, ok := rhsIdT.(types.TupleType); ok {
					for i, x := range rhs3.Elts {
						lhs := stmt.Lhs[i]
						lhsID := MustCast[*ast.Ident](lhs)
						infer.SetType(lhs, x)
						assignFn(lhsID.Name, infer.GetType(lhsID))
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
			assignFn(lhsID.Name, infer.GetType(lhsID))
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
	if expr.Name == "None" {
		t := infer.returnType
		assert(t != nil)
		return types.NoneType{W: t.(types.OptionType).W}
	} else if expr.Name == "Some" {
		t := infer.returnType
		assert(t != nil)
		return types.SomeType{W: t.(types.OptionType).W}
	} else if expr.Name == "Ok" {
		t := infer.returnType
		assert(t != nil)
		return types.OkType{W: t.(types.ResultType).W}
	} else if expr.Name == "Err" {
		t := infer.returnType
		assert(t != nil)
		return types.ErrType{W: t.(types.ResultType).W, T: t.(types.ResultType).W}
	} else if expr.Name == "Result" {
		//t := infer.returnType
		//assert(t != nil)
		return types.ResultType{W: types.I64Type{}} // TODO
	} else if expr.Name == "Option" {
		//t := infer.returnType
		//assert(t != nil)
		return types.OptionType{W: types.I64Type{}} // TODO
	} else if expr.Name == "make" {
		return types.MakeType{}
	} else if expr.Name == "len" {
		return types.LenType{}
	} else if expr.Name == "_" {
		return types.UnderscoreType{}
	}
	v := infer.env.Get(expr.Name)
	assertf(v != nil, "%s: undefined identifier %s", infer.fset.Position(expr.Pos()), expr.Name)
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

func (infer *FileInferrer) typeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	if stmt.Init != nil {
		infer.stmt(stmt.Init)
	}
	infer.stmt(stmt.Assign)
	infer.stmt(stmt.Body)
}

func (infer *FileInferrer) labeledStmt(stmt *ast.LabeledStmt) {
	infer.env.Define(stmt.Label.Name, types.LabelType{})
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
