package main

import (
	goast "agl/ast"
	"agl/token"
	"agl/types"
	"fmt"
	"strconv"
)

type Inferrer struct {
	fset *token.FileSet
	env  *Env
}

func NewInferrer(fset *token.FileSet) *Inferrer {
	return &Inferrer{fset: fset, env: NewEnv(fset)}
}

func (infer *Inferrer) InferFile(f *goast.File) {
	fileInferrer := &FileInferrer{env: infer.env, f: f, fset: infer.fset}
	fileInferrer.Infer()
}

type FileInferrer struct {
	env         *Env
	f           *goast.File
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

func (infer *FileInferrer) GetType(p goast.Node) types.Type {
	t := infer.env.GetType(p)
	if t == nil {
		panic(fmt.Sprintf("%s type not found for %v %v", infer.env.fset.Position(p.Pos()), p, to(p)))
	}
	return t
}

func (infer *FileInferrer) SetTypeForce(a goast.Node, t types.Type) {
	infer.env.SetType(a, t)
}

func (infer *FileInferrer) SetType(a goast.Node, t types.Type) {
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
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			infer.funcDecl(decl)
		case *goast.GenDecl:
			infer.genDecl(decl)
		}
	}
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			infer.funcDecl2(decl)
		}
	}
}

func (infer *FileInferrer) genDecl(decl *goast.GenDecl) {
	for _, s := range decl.Specs {
		switch spec := s.(type) {
		case *goast.TypeSpec:
			infer.typeSpec(spec)
		}
	}
}

func (infer *FileInferrer) typeSpec(spec *goast.TypeSpec) {
	switch t := spec.Type.(type) {
	case *goast.StructType:
		infer.structType(spec.Name, t)
	case *goast.EnumType:
		infer.enumType(spec.Name, t)
	case *goast.InterfaceType:
		infer.interfaceType(spec.Name, t)
	}
}

func (infer *FileInferrer) interfaceType(name *goast.Ident, e *goast.InterfaceType) {
	if e.Methods.List != nil {
		for _, f := range e.Methods.List {
			for _, n := range f.Names {
				noop(n)
			}
		}
	}
	infer.env.Define(name.Name, types.InterfaceType{Name: name.Name})
}

func (infer *FileInferrer) enumType(name *goast.Ident, e *goast.EnumType) {
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

func (infer *FileInferrer) structType(name *goast.Ident, s *goast.StructType) {
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

func (infer *FileInferrer) funcDecl(decl *goast.FuncDecl) {
	var t types.FuncType
	outEnv := infer.env
	infer.sandboxed(func() {
		t = infer.getFuncDeclType(decl, outEnv)
	})
	infer.SetType(decl, t)
	infer.env.Define(decl.Name.Name, t)
}

func (infer *FileInferrer) funcDecl2(decl *goast.FuncDecl) {
	infer.withEnv(func() {
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
			if len(decl.Body.List) == 1 && decl.Type.Result != nil && TryCast[*goast.ExprStmt](decl.Body.List[0]) {
				decl.Body.List = []goast.Stmt{&goast.ReturnStmt{Result: decl.Body.List[0].(*goast.ExprStmt).X}}
			}
			infer.stmt(decl.Body)
		}
		infer.returnType = nil
	})
}

func (infer *FileInferrer) getFuncDeclType(decl *goast.FuncDecl, outEnv *Env) types.FuncType {
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

func (infer *FileInferrer) stmts(s []goast.Stmt) {
	for _, stmt := range s {
		infer.stmt(stmt)
	}
}

func (infer *FileInferrer) exprs(s []goast.Expr) {
	for _, expr := range s {
		infer.expr(expr)
	}
}

func (infer *FileInferrer) expr(e goast.Expr) {
	//p("infer.expr", to(e))
	switch expr := e.(type) {
	case *goast.Ident:
		t := infer.identExpr(expr)
		infer.SetType(expr, t)
	case *goast.CallExpr:
		infer.callExpr(expr)
	case *goast.BinaryExpr:
		infer.binaryExpr(expr)
	case *goast.OptionExpr:
		infer.optionExpr(expr)
	case *goast.ResultExpr:
		infer.resultExpr(expr)
	case *goast.IndexExpr:
		infer.indexExpr(expr)
	case *goast.ArrayType:
		infer.arrayType(expr)
	case *goast.FuncType:
		infer.funcType(expr)
	case *goast.BasicLit:
		infer.basicLit(expr)
	case *goast.ShortFuncLit:
		infer.shortFuncLit(expr)
	case *goast.CompositeLit:
		infer.compositeLit(expr)
	case *goast.BubbleOptionExpr:
		infer.bubbleOptionExpr(expr)
	case *goast.BubbleResultExpr:
		infer.bubbleResultExpr(expr)
	case *goast.SelectorExpr:
		infer.selectorExpr(expr)
	case *goast.FuncLit:
		infer.funcLit(expr)
	case *goast.TupleExpr:
		infer.tupleExpr(expr)
	case *goast.Ellipsis:
		infer.ellipsis(expr)
	case *goast.VoidExpr:
		infer.voidExpr(expr)
	default:
		panic(fmt.Sprintf("unknown expression %v", to(e)))
	}
	if infer.optType != nil {
		infer.tryConvertType(e, infer.optType)
	}
}

func (infer *FileInferrer) tryConvertType(e goast.Expr, optType types.Type) {
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

func (infer *FileInferrer) stmt(s goast.Stmt) {
	//p("infer.stmt", to(s))
	switch stmt := s.(type) {
	case *goast.BlockStmt:
		infer.blockStmt(stmt)
	case *goast.IfStmt:
		infer.ifStmt(stmt)
	case *goast.ReturnStmt:
		infer.returnStmt(stmt)
	case *goast.ExprStmt:
		infer.exprStmt(stmt)
	case *goast.AssignStmt:
		infer.assignStmt(stmt)
	case *goast.RangeStmt:
		infer.rangeStmt(stmt)
	case *goast.IncDecStmt:
		infer.incDecStmt(stmt)
	case *goast.DeclStmt:
		infer.declStmt(stmt)
	default:
		panic(fmt.Sprintf("unknown statement %v", to(stmt)))
	}
}

func (infer *FileInferrer) basicLit(expr *goast.BasicLit) {
	switch expr.Kind {
	case token.STRING:
		infer.SetType(expr, types.StringType{})
	case token.INT:
		infer.SetType(expr, types.UntypedNumType{})
	default:
		panic(fmt.Sprintf("unknown basic literal %v %v", to(expr), expr.Kind))
	}
}

func (infer *FileInferrer) callExpr(expr *goast.CallExpr) {
	tmpFn := func(idT types.Type, call *goast.SelectorExpr) {
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
	case *goast.SelectorExpr:
		switch id := call.X.(type) {
		case *goast.Ident:
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
	case *goast.Ident:
		if call.Name == "Some" {
			infer.SetType(call, types.SomeType{W: infer.returnType.(types.OptionType).W})
			return
		} else if call.Name == "Err" {
			if v, ok := expr.Args[0].(*goast.BasicLit); ok && v.Kind == token.STRING {
				expr.Args[0] = &goast.CallExpr{Fun: &goast.SelectorExpr{X: &goast.Ident{Name: "errors"}, Sel: &goast.Ident{Name: "New"}}, Args: []goast.Expr{v}}
			}
			infer.SetType(call, types.ErrType{W: infer.returnType.(types.ResultType).W})
			return
		} else if call.Name == "Ok" {
			infer.SetType(call, types.OkType{W: infer.returnType.(types.ResultType).W})
			return
		} else {
			if call.Name == "make" {
				fnT := infer.env.Get("make").(types.FuncType)
				fnT = fnT.ReplaceGenericParameter("T", infer.env.GetType2(expr.Args[0].(*goast.ArrayType)))
				infer.SetType(expr, fnT.Return)
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
				if _, ok := arg.(*goast.ShortFuncLit); ok {
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

func (infer *FileInferrer) inferVecExtensions(idT types.Type, exprT *goast.SelectorExpr, expr *goast.CallExpr) {
	if idTArr, ok := idT.(types.ArrayType); ok {
		exprPos := infer.fset.Position(expr.Pos())
		if exprT.Sel.Name == "Filter" {
			ft := infer.env.Get("agl.Vec.Filter").(types.FuncType).GetParam(1).(types.FuncType)
			ft = ft.ReplaceGenericParameter("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*goast.ShortFuncLit); ok {
				infer.SetTypeForce(exprArg0, ft)
			} else if _, ok := exprArg0.(*goast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*goast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			infer.SetTypeForce(expr, types.ArrayType{Elt: ft.Params[0]})

		} else if exprT.Sel.Name == "Map" {
			ft := infer.env.GetFn("agl.Vec.Map").GetParam(1).(types.FuncType).T("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if arg0, ok := exprArg0.(*goast.ShortFuncLit); ok {
				infer.SetTypeForce(arg0, ft)
				infer.expr(arg0)
				infer.SetTypeForce(expr, types.ArrayType{Elt: infer.GetType(arg0).(types.FuncType).Return})
			} else if arg0, ok := exprArg0.(*goast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if tmp, ok := exprArg0.(*goast.FuncLit); ok {
					infer.expr(tmp)
					infer.SetTypeForce(expr, types.ArrayType{Elt: infer.GetType(exprArg0.(*goast.FuncLit)).(types.FuncType).Return})
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
			if _, ok := expr.Args[1].(*goast.ShortFuncLit); ok {
				infer.SetTypeForce(expr.Args[1], ft)
			} else if _, ok := exprArg0.(*goast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*goast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
		} else if exprT.Sel.Name == "Find" {
			ft := infer.env.Get("agl.Vec.Find").(types.FuncType).GetParam(1).(types.FuncType)
			ft = ft.ReplaceGenericParameter("T", idTArr.Elt)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*goast.ShortFuncLit); ok {
				infer.SetTypeForce(exprArg0, ft)
			} else if _, ok := exprArg0.(*goast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*goast.FuncType), infer.env, false)
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
			}
			infer.SetTypeForce(expr, types.OptionType{W: ft.Params[0]})
		}
	}
}

func (infer *FileInferrer) funcLit(expr *goast.FuncLit) {
	if infer.optType != nil {
		infer.SetType(expr, infer.optType)
	}
	ft := funcTypeToFuncType("", expr.Type, infer.env, false)
	// implicit return
	if len(expr.Body.List) == 1 && TryCast[*goast.ExprStmt](expr.Body.List[0]) {
		returnStmt := expr.Body.List[0].(*goast.ExprStmt)
		expr.Body.List = []goast.Stmt{&goast.ReturnStmt{Result: returnStmt.X}}
	}
	infer.SetType(expr, ft)
}

func (infer *FileInferrer) shortFuncLit(expr *goast.ShortFuncLit) {
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
		if len(expr.Body.List) == 1 && TryCast[*goast.ExprStmt](expr.Body.List[0]) {
			returnStmt := expr.Body.List[0].(*goast.ExprStmt)
			if infer.env.GetType(returnStmt.X) != nil {
				if infer.env.GetType(expr) != nil {
					ft := infer.env.GetType(expr).(types.FuncType)
					if t, ok := ft.Return.(types.GenericType); ok {
						ft = ft.ReplaceGenericParameter(t.Name, infer.env.GetType(returnStmt.X))
						infer.SetTypeForce(expr, ft)
					}
				}
			}
			expr.Body.List = []goast.Stmt{&goast.ReturnStmt{Result: returnStmt.X}}
		}
		// expr type is set in CallExpr
	})
}

func (infer *FileInferrer) funcType(expr *goast.FuncType) {
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

func (infer *FileInferrer) voidExpr(expr *goast.VoidExpr) {
	infer.SetType(expr, types.VoidType{})
}

func (infer *FileInferrer) ellipsis(expr *goast.Ellipsis) {
	infer.expr(expr.Elt)
	infer.SetType(expr, types.EllipsisType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) tupleExpr(expr *goast.TupleExpr) {
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

func (infer *FileInferrer) selectorExpr(expr *goast.SelectorExpr) {
	selType := infer.env.Get(expr.X.(*goast.Ident).Name)
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
		enumName := expr.X.(*goast.Ident).Name
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

func (infer *FileInferrer) bubbleResultExpr(expr *goast.BubbleResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X).(types.ResultType).W)
}

func (infer *FileInferrer) bubbleOptionExpr(expr *goast.BubbleOptionExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X).(types.OptionType).W)
}

func (infer *FileInferrer) compositeLit(expr *goast.CompositeLit) {
	if arr, ok := expr.Type.(*goast.ArrayType); ok {
		infer.SetType(expr, types.ArrayType{Elt: infer.env.GetType2(arr.Elt)})
		return
	}
	infer.SetType(expr, infer.env.Get(expr.Type.(*goast.Ident).Name))
}

func (infer *FileInferrer) arrayType(expr *goast.ArrayType) {
	if expr.Len != nil {
		infer.expr(expr.Len)
	}
	infer.expr(expr.Elt)
	infer.SetType(expr, types.ArrayType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) indexExpr(expr *goast.IndexExpr) {
	infer.expr(expr.X)
	infer.expr(expr.Index)
}

func (infer *FileInferrer) resultExpr(expr *goast.ResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.ResultType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) optionExpr(expr *goast.OptionExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.OptionType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) declStmt(stmt *goast.DeclStmt) {
	switch d := stmt.Decl.(type) {
	case *goast.GenDecl:
		infer.specs(d.Specs)
	}
}

func (infer *FileInferrer) specs(s []goast.Spec) {
	for _, spec := range s {
		infer.spec(spec)
	}
}

func (infer *FileInferrer) spec(s goast.Spec) {
	switch spec := s.(type) {
	case *goast.ValueSpec:
		for _, name := range spec.Names {
			infer.exprs(spec.Values)
			t := infer.env.GetType2(spec.Type)
			infer.SetType(name, t)
			infer.env.Define(name.Name, t)
		}
	}
}

func (infer *FileInferrer) incDecStmt(stmt *goast.IncDecStmt) {
}

func (infer *FileInferrer) rangeStmt(stmt *goast.RangeStmt) {
	infer.withEnv(func() {
		infer.expr(stmt.X)
		xT := infer.GetType(stmt.X)
		if stmt.Key != nil {
			// TODO find correct type for map
			name := stmt.Key.(*goast.Ident).Name
			infer.env.Define(name, types.IntType{})
		}
		if stmt.Value != nil {
			name := stmt.Value.(*goast.Ident).Name
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

func (infer *FileInferrer) assignStmt(stmt *goast.AssignStmt) {
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
			lhsID := MustCast[*goast.Ident](lhs)
			infer.SetType(lhs, infer.GetType(rhs))
			assignFn(lhsID.Name, infer.GetType(lhsID))
		} else {
			if rhs, ok := rhs.(*goast.TupleExpr); ok {
				for i, x := range rhs.Values {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*goast.Ident](lhs)
					infer.SetType(lhs, infer.GetType(x))
					assignFn(lhsID.Name, infer.GetType(lhsID))
				}
			}
			if rhs, ok := infer.env.GetType(rhs).(types.EnumType); ok {
				for i, e := range stmt.Lhs {
					lit := rhs.SubTyp
					fields := rhs.Fields
					// AGL: fields.Find({ $0.name == lit })
					f := Find(fields, func(f types.EnumFieldType) bool { return f.Name == lit })
					assert(f != nil)
					assignFn(e.(*goast.Ident).Name, f.Elts[i])
				}
				return
			}
		}
	} else {
		for i := range stmt.Lhs {
			lhs := stmt.Lhs[i]
			rhs := stmt.Rhs[i]
			infer.expr(rhs)
			lhsID := MustCast[*goast.Ident](lhs)
			infer.SetType(lhsID, infer.GetType(rhs))
			assignFn(lhsID.Name, infer.GetType(lhsID))
		}
	}
}

func (infer *FileInferrer) exprStmt(stmt *goast.ExprStmt) {
	infer.expr(stmt.X)
}

func (infer *FileInferrer) returnStmt(stmt *goast.ReturnStmt) {
	infer.optType = infer.returnType
	infer.expr(stmt.Result)
	infer.optType = nil

}

func (infer *FileInferrer) blockStmt(stmt *goast.BlockStmt) {
	infer.stmts(stmt.List)
}

func (infer *FileInferrer) binaryExpr(expr *goast.BinaryExpr) {
	infer.expr(expr.X)
	if TryCast[types.OptionType](infer.env.GetType2(expr.X)) &&
		TryCast[*goast.Ident](expr.Y) && expr.Y.(*goast.Ident).Name == "None" &&
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

func (infer *FileInferrer) identExpr(expr *goast.Ident) types.Type {
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
		return types.ErrType{W: t.(types.ResultType).W}
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

func (infer *FileInferrer) ifStmt(stmt *goast.IfStmt) {
	infer.withEnv(func() {
		infer.stmt(stmt.Body)
	})
}
