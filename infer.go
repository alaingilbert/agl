package main

import (
	goast "agl/ast"
	"agl/token"
	"agl/types"
	"fmt"
	"path"
	"runtime"
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
		panic(fmt.Sprintf("type not found for %v %v", p, to(p)))
	}
	return t
}

func (infer *FileInferrer) SetTypeForce(a goast.Node, t types.Type) {
	infer.env.SetType(a, t)
}

func printCallers(n int) {
	fmt.Println("--- callers ---")
	for i := 0; i < n; i++ {
		pc, _, _, ok := runtime.Caller(i + 2)
		if !ok {
			break
		}
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		fmt.Printf("%s:%d %s\n", path.Base(file), line, f.Name())
	}
}

func (infer *FileInferrer) SetType(a goast.Node, t types.Type) {
	fmt.Println("??SETTYPE", a.Pos(), a, t)
	printCallers(100)
	if t := infer.env.GetType(a); t != nil {
		//return // TODO
		panic(fmt.Sprintf("type already declared for %s %s %v", infer.fset.Position(a.Pos()), a, to(a)))
	}
	infer.env.SetType(a, t)
}

func (infer *FileInferrer) Infer() {
	infer.PackageName = infer.f.Name.Name
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			infer.funcDecl(decl)
		}
	}
	for _, d := range infer.f.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			infer.funcDecl2(decl)
		}
	}
}

func (infer *FileInferrer) funcDecl(decl *goast.FuncDecl) {
	var t types.FuncType
	infer.withEnv(func() {
		t = infer.getFuncDeclType(decl)
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
		if decl.Type.Result == nil {
			infer.returnType = types.VoidType{}
		} else {
			infer.returnType = infer.env.GetType(decl.Type.Result)
		}
		if decl.Body != nil {
			// implicit return
			if len(decl.Body.List) == 1 && decl.Type.Result != nil && TryCast[*goast.ExprStmt](decl.Body.List[0]) {
				decl.Body.List = []goast.Stmt{&goast.ReturnStmt{Result: decl.Body.List[0].(*goast.ExprStmt).X}}
			}
			infer.stmt(decl.Body)
		}
	})
}

func (infer *FileInferrer) getFuncDeclType(decl *goast.FuncDecl) types.FuncType {
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
	}
	return types.FuncType{
		Name:   decl.Name.Name,
		Params: paramsT,
		Return: returnT,
	}
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
	switch expr := e.(type) {
	case *goast.Ident:
		infer.identExpr(expr)
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
	default:
		panic(fmt.Sprintf("unknown expression %v", to(e)))
	}
}
func (infer *FileInferrer) stmt(s goast.Stmt) {
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
			if call.Sel.Name == "filter" {
				fnT := infer.env.Get("agl.Vec.filter").(types.FuncType)
				fnT = fnT.ReplaceGenericParameter("T", arr.Elt)
				infer.SetType(expr.Args[0], fnT.Params[1])
				infer.SetType(call, fnT.Return)
			} else if call.Sel.Name == "Map" {
				fnT := infer.env.Get("agl.Vec.Map").(types.FuncType)
				fnT = fnT.ReplaceGenericParameter("T", arr.Elt)
				p1 := fnT.GetParam(1)
				infer.SetType(expr.Args[0], p1)
				infer.SetType(call, fnT.Return)
			} else if call.Sel.Name == "Reduce" {
				fnT := infer.env.Get("agl.Vec.Reduce").(types.FuncType)
				fnT = fnT.ReplaceGenericParameter("R", infer.env.GetType2(expr.Args[0]))
				fnT = fnT.ReplaceGenericParameter("T", arr.Elt)
				infer.SetType(expr.Args[1], fnT.Params[2])
				p("?before", fnT.Return)
				infer.SetType(expr, fnT.Return)
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
				if _, ok := l.(types.PackageType); ok {
					name := fmt.Sprintf("%s.%s", id.Name, call.Sel.Name)
					infer.SetType(expr, infer.env.Get(name).(types.FuncType).Return)
				}
			}
			idT := infer.env.Get(id.Name)
			infer.inferVecExtensions(idT, call, expr)
		default:
			p("?HERE1", &id, id, to(id))
			infer.expr(id)
			idT := infer.GetType(id)
			tmpFn(idT, call)
			infer.inferVecExtensions(idT, call, expr)
		}
		infer.exprs(expr.Args)
	case *goast.Ident:
		callT := infer.env.Get(call.Name).(types.FuncType)
		oParams := callT.Params
		infer.expr(call)
		for i := range expr.Args {
			arg := expr.Args[i]
			if f, ok := arg.(*goast.ShortFuncLit); ok {
				paramI := oParams[i].(types.FuncType)
				ft := paramI
				infer.SetType(f, ft)
				infer.expr(arg)
			}
		}
		p("?HERE", &call, call, to(call), callT)
		infer.SetType(call, callT)
	}
	if infer.GetType(expr.Fun) != nil {
		if v, ok := infer.GetType(expr.Fun).(types.FuncType); ok {
			p("?????", expr, to(expr), v.Return)
			infer.SetType(expr, v.Return)
		} else { // Type casting
			// TODO
		}
	}
}

func (infer *FileInferrer) inferVecExtensions(idT types.Type, exprT *goast.SelectorExpr, expr *goast.CallExpr) {
	if TryCast[types.ArrayType](idT) && exprT.Sel.Name == "filter" {
		clbFnStr := "func [T any](e T) bool"
		ft := parseFuncTypeFromStringNative("", clbFnStr, infer.env)
		ft = ft.ReplaceGenericParameter("T", idT.(types.ArrayType).Elt)
		if _, ok := expr.Args[0].(*goast.ShortFuncLit); ok {
			infer.SetTypeForce(expr.Args[0], ft)
		} else if _, ok := expr.Args[0].(*goast.FuncType); ok {
			ftReal := funcTypeToFuncType("", expr.Args[0].(*goast.FuncType), infer.env)
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		} else if ftReal, ok := infer.env.GetType(expr.Args[0]).(types.FuncType); ok {
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		}
		infer.SetTypeForce(expr, types.ArrayType{Elt: ft.Params[0]})

	} else if TryCast[types.ArrayType](idT) && exprT.Sel.Name == "Map" {
		clbFnStr := "func [T, R any](e T) R"
		ft := parseFuncTypeFromStringNative("", clbFnStr, infer.env)
		ft = ft.ReplaceGenericParameter("T", idT.(types.ArrayType).Elt)
		if arg0, ok := expr.Args[0].(*goast.ShortFuncLit); ok {
			infer.SetTypeForce(arg0, ft)
			infer.expr(arg0)
			infer.SetTypeForce(expr, types.ArrayType{Elt: infer.GetType(arg0).(types.FuncType).Return})
		} else if arg0, ok := expr.Args[0].(*goast.FuncType); ok {
			ftReal := funcTypeToFuncType("", arg0, infer.env)
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		} else if ftReal, ok := infer.env.GetType(expr.Args[0]).(types.FuncType); ok {
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		}
	} else if TryCast[types.ArrayType](idT) && exprT.Sel.Name == "Reduce" {
		arg0 := expr.Args[0]
		infer.expr(arg0)
		elTyp := idT.(types.ArrayType).Elt
		clbFnStr := "func [T any, R cmp.Ordered](acc R, el T) R"
		ft := parseFuncTypeFromStringNative("", clbFnStr, infer.env)
		ft = ft.ReplaceGenericParameter("T", elTyp)
		if _, ok := infer.GetType(arg0).(types.UntypedNumType); ok {
			ft = ft.ReplaceGenericParameter("R", elTyp)
		}
		if _, ok := expr.Args[1].(*goast.ShortFuncLit); ok {
			infer.SetTypeForce(expr.Args[1], ft)
		} else if _, ok := expr.Args[0].(*goast.FuncType); ok {
			ftReal := funcTypeToFuncType("", expr.Args[0].(*goast.FuncType), infer.env)
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		} else if ftReal, ok := infer.env.GetType(expr.Args[0]).(types.FuncType); ok {
			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", infer.fset.Position(expr.Pos()), ftReal, ft)
		}
	}
}

func (infer *FileInferrer) shortFuncLit(expr *goast.ShortFuncLit) {
	infer.withEnv(func() {
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
					p("?!ANONRETURN", ft.Return)
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

func (infer *FileInferrer) funcLit(expr *goast.FuncLit) {
	ft := funcTypeToFuncType("", expr.Type, infer.env)
	infer.SetType(expr, ft)
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
	//if TryCast[InterfaceType](a) {
	//	return true
	//}
	if TryCast[types.GenericType](a) || TryCast[types.GenericType](b) {
		return true
	}
	if a == b {
		return true
	}
	if TryCast[types.OptionType](a) && TryCast[types.OptionType](b) {
		return cmpTypes(a.(types.OptionType).W, b.(types.OptionType).W)
	}
	if TryCast[types.ResultType](a) && TryCast[types.ResultType](b) {
		return cmpTypes(a.(types.ResultType).W, b.(types.ResultType).W)
	}
	return false
}

func (infer *FileInferrer) selectorExpr(expr *goast.SelectorExpr) {
}

func (infer *FileInferrer) bubbleResultExpr(expr *goast.BubbleResultExpr) {
	bubble := TryCast[types.ResultType](infer.returnType)
	infer.SetType(expr, types.BubbleResultType{Bubble: bubble})
}

func (infer *FileInferrer) bubbleOptionExpr(expr *goast.BubbleOptionExpr) {
	bubble := TryCast[types.OptionType](infer.returnType)
	infer.SetType(expr, types.BubbleOptionType{Bubble: bubble})
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
		p("?1", name, typ)
		f(name, typ)
	}
	for i := range stmt.Lhs {
		stmtLhs := stmt.Lhs[i]
		stmtRhs := stmt.Rhs[i]

		infer.exprs(stmt.Rhs)
		lhs := stmtLhs

		//if lhs, ok := lhs.(*TupleExpr); ok {
		//	if TryCast[*EnumType](stmtRhs.GetType()) {
		//		for i, e := range lhs.exprs {
		//			lit := stmtRhs.GetType().(*EnumType).subTyp
		//			fields := stmtRhs.GetType().(*EnumType).fields
		//			// AGL: fields.find({ $0.name == lit })
		//			f := Find(fields, func(f EnumFieldType) bool { return f.name == lit })
		//			assert(f != nil)
		//			assignFn(e.(*IdentExpr).lit, env.Get(f.elts[i]))
		//		}
		//		return
		//	}
		//	MustCast[*TupleExpr](stmtRhs)
		//	for i, x := range stmtRhs.(*TupleExpr).exprs {
		//		MustCast[*IdentExpr](lhs.exprs[i])
		//		lhs.exprs[i].SetType(x.GetType())
		//		assignFn(lhs.exprs[i].(*IdentExpr).lit, x.GetType())
		//	}
		//	return
		//}

		lhsID := MustCast[*goast.Ident](lhs)
		//switch rhs := stmtRhs.(type) {
		//case *BubbleResultExpr:
		//	callExpr := MustCast[*CallExpr](rhs.x)
		//	switch s := callExpr.fun.(type) {
		//	case *SelectorExpr:
		//		if id, ok := s.x.(*IdentExpr); ok {
		//			if rhs.GetType() == nil {
		//				rhs.SetType(env.Get(fmt.Sprintf("%s.%s", id.lit, s.sel.lit)))
		//				callExpr.SetType(rhs.typ.(FuncType).ret)
		//			}
		//		}
		//	}
		//}
		p("?WTF", lhsID, to(stmtRhs), infer.GetType(stmtRhs))
		infer.SetType(lhsID, infer.GetType(stmtRhs))
		assignFn(lhsID.Name, infer.GetType(lhsID))
	}
	//infer.exprs(stmt.Lhs)
	//infer.exprs(stmt.Rhs)
}

func (infer *FileInferrer) exprStmt(stmt *goast.ExprStmt) {
	infer.expr(stmt.X)
}

func (infer *FileInferrer) returnStmt(stmt *goast.ReturnStmt) {
	infer.expr(stmt.Result)
}

func (infer *FileInferrer) blockStmt(stmt *goast.BlockStmt) {
	infer.stmts(stmt.List)
}

func (infer *FileInferrer) binaryExpr(expr *goast.BinaryExpr) {
	infer.expr(expr.X)
	infer.expr(expr.Y)
	//if TryCast[types.OptionType](infer.GetType(expr.X)) &&
	//	TryCast[*NoneExpr](expr.Y) && infer.GetType(expr.Y) == nil {
	//	expr.Y.SetType(infer.GetType(expr.X))
	//}
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

func (infer *FileInferrer) identExpr(expr *goast.Ident) {
	if expr.Name == "None" {
		t := infer.returnType
		assert(t != nil)
		infer.SetType(expr, types.NoneType{W: t.(types.OptionType).W})
		return
	} else if expr.Name == "Some" {
		t := infer.returnType
		assert(t != nil)
		infer.SetType(expr, types.SomeType{W: t.(types.OptionType).W})
		return
	} else if expr.Name == "Ok" {
		t := infer.returnType
		assert(t != nil)
		infer.SetType(expr, types.OkType{W: t.(types.ResultType).W})
		return
	} else if expr.Name == "Err" {
		t := infer.returnType
		assert(t != nil)
		infer.SetType(expr, types.ErrType{W: t.(types.ResultType).W})
		return
	} else if expr.Name == "Result" {
		//t := infer.returnType
		//assert(t != nil)
		infer.SetType(expr, types.ResultType{W: types.I64Type{}}) // TODO
		return
	} else if expr.Name == "Option" {
		//t := infer.returnType
		//assert(t != nil)
		infer.SetType(expr, types.OptionType{W: types.I64Type{}}) // TODO
		return
	} else if expr.Name == "make" {
		infer.SetType(expr, types.MakeType{})
		return
	} else if expr.Name == "_" {
		infer.SetType(expr, types.UnderscoreType{})
		return
	}
	v := infer.env.Get(expr.Name)
	assertf(v != nil, "%s: undefined identifier %s", infer.fset.Position(expr.Pos()), expr.Name)
	infer.SetType(expr, v)
}

func (infer *FileInferrer) ifStmt(stmt *goast.IfStmt) {
	infer.withEnv(func() {
		infer.stmt(stmt.Body)
	})
}

//func inferFuncDecl(env *Env, decl *goast.FuncDecl) {
//	//nenv := env.Clone()
//	t := getFuncDeclType(env, decl)
//	env.Define(decl.Name.Name, t)
//}
//
//func infer(s *ast) (*ast, *Env) {
//	env := NewEnv()
//	for _, e := range s.enums {
//		inferEnumType(e, env)
//	}
//	for _, s := range s.interfaces {
//		inferInterfaceType(s, env)
//	}
//	for _, s := range s.structs {
//		inferStructType(s, env)
//	}
//	for _, f := range s.funcs {
//		newEnv := env.Clone()
//		if f.recv != nil {
//			fnName := f.name
//			if newName, ok := overloadMapping[fnName]; ok {
//				fnName = newName
//			}
//			name := f.recv.list[0].typeExpr.(*IdentExpr).lit + "." + fnName
//			env.Define(name, getFuncType(f, newEnv))
//		} else {
//			env.Define(f.name, getFuncType(f, newEnv))
//		}
//	}
//	for _, f := range s.funcs {
//		newEnv := env.Clone()
//		inferFuncType(f, newEnv)
//		fOutTyp := f.out.expr.GetType()
//		inferStmts(f.stmts, fOutTyp, newEnv)
//	}
//	return s, env
//}
//
//func inferStmts(stmts []Stmt, returnTyp Typ, env *Env) {
//	for _, ss := range stmts {
//		inferStmt(ss, returnTyp, env)
//	}
//}
//
//func inferStmt(s Stmt, returnTyp Typ, env *Env) {
//	switch stmt := s.(type) {
//	case *AssignStmt:
//		inferAssignStmt(stmt, env)
//	case *ValueSpec:
//		inferValueSpecStmt(stmt, env)
//	case *IfStmt:
//		inferExpr(stmt.cond, returnTyp, env)
//		inferStmts(stmt.body, returnTyp, env)
//		if stmt.Else != nil {
//			inferStmt(stmt.Else, returnTyp, env)
//		}
//	case *IfLetStmt:
//		// TODO fix type of variable: p(stmt.lhs, stmt.lhs.GetType())
//		nenv := env.Clone()
//		var id string
//		switch v := stmt.lhs.(type) {
//		case *SomeExpr:
//			id = v.expr.(*IdentExpr).lit
//		case *OkExpr:
//			id = v.expr.(*IdentExpr).lit
//		case *ErrExpr:
//			id = v.expr.(*IdentExpr).lit
//		case *IdentExpr:
//			id = v.lit
//		default:
//			panic("")
//		}
//		inferExpr(stmt.rhs, returnTyp, env)
//		nenv.Define(id, stmt.rhs.GetType())
//		if stmt.cond != nil {
//			inferExpr(stmt.cond, returnTyp, nenv)
//		}
//		inferStmts(stmt.body, returnTyp, nenv)
//		if stmt.Else != nil {
//			inferStmt(stmt.Else, returnTyp, nenv)
//		}
//	case *ReturnStmt:
//		inferExpr(stmt.expr, returnTyp, env)
//	case *AssertStmt:
//		inferAssertStmt(stmt, env)
//	case *ExprStmt:
//		inferExpr(stmt.x, nil, env)
//	case *InlineCommentStmt:
//	case *ForRangeStmt:
//	case *IncDecStmt:
//	case *BlockStmt:
//		inferStmts(stmt.stmts, returnTyp, env)
//	default:
//		panic(fmt.Sprintf("unexpected type %v", to(stmt)))
//	}
//}
//
//func inferAssertStmt(stmt *AssertStmt, env *Env) {
//	inferExpr(stmt.x, nil, env)
//	if stmt.msg != nil {
//		inferExpr(stmt.msg, nil, env)
//	}
//}
//
//func inferValueSpecStmt(stmt *ValueSpec, env *Env) {
//	inferExpr(stmt.typ, nil, env)
//	var rhsT Typ
//	typT := stmt.typ.GetType()
//	if len(stmt.values) > 0 {
//		inferExpr(stmt.values[0], stmt.typ.GetType(), env)
//		rhsT = stmt.values[0].GetType()
//	} else {
//		rhsT = stmt.typ.GetType()
//	}
//	lhs := stmt.names[0]
//	assertf(typT == rhsT, "%s: cannot use %s as %s value in variable declaration", stmt.Pos(), rhsT, typT)
//	env.Define(lhs.lit, rhsT)
//	lhs.SetType(rhsT)
//}
//
//func inferAssignStmt(stmt *AssignStmt, env *Env) {
//	myDefine := func(name string, typ Typ) {
//		assertf(name != "_", "%s: No new variables on the left side of ':='", stmt.tok.Pos)
//		env.Define(name, typ)
//	}
//	assignFn := func(name string, typ Typ) {
//		op := stmt.tok.typ
//		f := Ternary(op == WALRUS, myDefine, env.Assign)
//		f(name, typ)
//	}
//
//	assertf(len(stmt.lhs) == len(stmt.rhs), "%s: assignment mismatch", stmt.tok.Pos)
//	for i := range stmt.lhs {
//		stmtLhs := stmt.lhs[i]
//		stmtRhs := stmt.rhs[i]
//
//		inferExpr(stmtRhs, nil, env)
//		lhs := stmtLhs
//		if v, ok := stmtLhs.(*MutExpr); ok {
//			lhs = v.x
//		}
//
//		if lhs, ok := lhs.(*TupleExpr); ok {
//			if TryCast[*EnumType](stmtRhs.GetType()) {
//				for i, e := range lhs.exprs {
//					lit := stmtRhs.GetType().(*EnumType).subTyp
//					fields := stmtRhs.GetType().(*EnumType).fields
//					// AGL: fields.find({ $0.name == lit })
//					f := Find(fields, func(f EnumFieldType) bool { return f.name == lit })
//					assert(f != nil)
//					assignFn(e.(*IdentExpr).lit, env.Get(f.elts[i]))
//				}
//				return
//			}
//			MustCast[*TupleExpr](stmtRhs)
//			for i, x := range stmtRhs.(*TupleExpr).exprs {
//				MustCast[*IdentExpr](lhs.exprs[i])
//				lhs.exprs[i].SetType(x.GetType())
//				assignFn(lhs.exprs[i].(*IdentExpr).lit, x.GetType())
//			}
//			return
//		}
//
//		lhsID := MustCast[*IdentExpr](lhs)
//		switch rhs := stmtRhs.(type) {
//		case *BubbleResultExpr:
//			callExpr := MustCast[*CallExpr](rhs.x)
//			switch s := callExpr.fun.(type) {
//			case *SelectorExpr:
//				if id, ok := s.x.(*IdentExpr); ok {
//					if rhs.GetType() == nil {
//						rhs.SetType(env.Get(fmt.Sprintf("%s.%s", id.lit, s.sel.lit)))
//						callExpr.SetType(rhs.typ.(FuncType).ret)
//					}
//				}
//			}
//		}
//		lhsID.SetType(stmtRhs.GetType())
//		assignFn(lhsID.lit, lhsID.typ)
//	}
//}
//
//func inferExprs(e []Expr, env *Env) {
//	for _, expr := range e {
//		inferExpr(expr, nil, env)
//	}
//}
//
//func inferExpr(e Expr, optType Typ, env *Env) {
//	switch expr := e.(type) {
//	case *CallExpr:
//		inferCallExpr(expr, env)
//	case *OptionExpr:
//		inferExpr(expr.x, nil, env)
//		expr.SetType(OptionType{wrappedType: expr.x.GetType()})
//	case *BubbleOptionExpr:
//		inferExpr(expr.x, nil, env)
//		assertf(TryCast[OptionType](expr.x.GetType()), "should be Option type, got: %s", expr.x.GetType())
//		expr.SetType(expr.x.GetType().(OptionType).wrappedType)
//	case *BubbleResultExpr:
//		inferExpr(expr.x, nil, env)
//		assertf(TryCast[ResultType](expr.x.GetType()), "should be Result type, got: %s", expr.x.GetType())
//		expr.SetType(expr.x.GetType().(ResultType).wrappedType)
//	case *NumberExpr:
//		expr.SetType(UntypedNumType{})
//	case *NoneExpr:
//	case *SomeExpr:
//	case *OkExpr:
//	case *ErrExpr:
//	case *BinOpExpr:
//		inferBinOpExpr(expr, env)
//	case *SelectorExpr:
//		inferSelectorExpr(expr, env)
//	case *TupleExpr:
//		inferTupleExpr(expr, env, optType)
//	case *IdentExpr:
//		inferIdentExpr(expr, env)
//	case *MakeExpr:
//		inferExprs(expr.exprs, env)
//		expr.SetType(expr.exprs[0].GetType())
//	case *VecExpr:
//		expr.SetType(ArrayType{elt: env.Get(expr.typStr)})
//	case *StringExpr:
//		expr.SetType(StringType{})
//	case *AnonFnExpr:
//		inferAnonFnExpr(expr, env.Clone(), optType)
//	case *FuncExpr:
//		inferFuncExpr(expr, env.Clone(), optType)
//	case *TrueExpr:
//		expr.SetType(BoolType{})
//	case *FalseExpr:
//		expr.SetType(BoolType{})
//	case *CompositeLitExpr:
//		expr.SetType(env.Get(expr.typ.(*IdentExpr).lit))
//	case *ArrayTypeExpr:
//		expr.SetType(env.GetType(expr))
//	case *TypeAssertExpr:
//		inferExpr(expr.x, nil, env)
//		inferExpr(expr.typ, nil, env)
//		expr.SetType(OptionType{wrappedType: env.GetType(expr.typ)})
//	case *MatchExpr:
//		inferExpr(expr.expr, nil, env)
//		if _, ok := expr.expr.GetType().(OptionType); ok {
//			var hasSome, hasNone bool
//			for _, c := range expr.cases {
//				switch v := c.cond.(type) {
//				case *SomeExpr:
//					id := v.expr.(*IdentExpr).lit
//					nenv := env.Clone()
//					nenv.Define(id, expr.expr.GetType().(OptionType).wrappedType)
//					inferStmts(c.body, nil, nenv)
//					hasSome = true
//				case *NoneExpr:
//					hasNone = true
//				case *IdentExpr:
//					if v.lit == "_" {
//						hasSome, hasNone = true, true
//					}
//				default:
//					panic("")
//				}
//			}
//			assertf(hasSome && hasNone, "%s: match statement must be exhaustive", expr.Pos())
//		} else if _, ok := expr.expr.GetType().(ResultType); ok {
//			var hasOk, hasErr bool
//			for _, c := range expr.cases {
//				switch v := c.cond.(type) {
//				case *OkExpr:
//					id := v.expr.(*IdentExpr).lit
//					nenv := env.Clone()
//					nenv.Define(id, expr.expr.GetType().(ResultType).wrappedType)
//					inferStmts(c.body, nil, nenv)
//					hasOk = true
//				case *ErrExpr:
//					hasErr = true
//				case *IdentExpr:
//					if v.lit == "_" {
//						hasOk, hasErr = true, true
//					}
//				default:
//					panic("")
//				}
//			}
//			assertf(hasOk && hasErr, "%s: match statement must be exhaustive", expr.Pos())
//		} else {
//			panic(fmt.Sprintf("not implemented %v", expr.expr.GetType()))
//		}
//	default:
//		panic(fmt.Sprintf("unexpected type %v", to(expr)))
//	}
//	if optType != nil {
//		tryConvertType(e, optType)
//	}
//}
//
//func inferIdentExpr(expr *IdentExpr, env *Env) {
//	v := env.Get(expr.lit)
//	assertf(v != nil, "%s: undefined identifier %s", expr.Pos(), expr.lit)
//	expr.SetType(v)
//}
//
//func tryConvertType(e Expr, optType Typ) {
//	if e.GetType() == nil {
//		e.SetType(optType)
//	} else if _, ok := e.GetType().(UntypedNumType); ok {
//		if TryCast[U8Type](optType) ||
//			TryCast[U16Type](optType) ||
//			TryCast[U32Type](optType) ||
//			TryCast[U64Type](optType) ||
//			TryCast[I8Type](optType) ||
//			TryCast[I16Type](optType) ||
//			TryCast[I32Type](optType) ||
//			TryCast[I64Type](optType) ||
//			TryCast[IntType](optType) ||
//			TryCast[UintType](optType) {
//			e.SetType(optType)
//		}
//	}
//}
//
//func inferTupleExpr(expr *TupleExpr, env *Env, optType Typ) {
//	inferExprs(expr.exprs, env)
//	if optType != nil {
//		expected := optType.(TupleType).elts
//		for i, x := range expr.exprs {
//			if _, ok := x.GetType().(UntypedNumType); ok {
//				x.SetType(expected[i])
//			}
//		}
//	} else {
//		tupleTyp := TupleType{elts: make([]Typ, len(expr.exprs)), name: fmt.Sprintf("%s%d", TupleStructPrefix, env.structCounter.Add(1))}
//		for i, x := range expr.exprs {
//			if _, ok := x.GetType().(UntypedNumType); ok {
//				x.SetType(IntType{})
//				tupleTyp.elts[i] = x.(*NumberExpr).typ
//			} else {
//				tupleTyp.elts[i] = x.GetType()
//			}
//		}
//		expr.SetType(tupleTyp)
//	}
//}
//
//func inferSelectorExpr(expr *SelectorExpr, env *Env) {
//	selType := env.Get(expr.x.(*IdentExpr).lit)
//	switch v := selType.(type) {
//	case TupleType:
//		argIdx, err := strconv.Atoi(expr.sel.lit)
//		if err != nil {
//			panic("tuple arg index must be int")
//		}
//		expr.x.SetType(v)
//		expr.SetType(v.elts[argIdx])
//	case *EnumType:
//		enumName := expr.x.(*IdentExpr).lit
//		fieldName := expr.sel.lit
//		validFields := make([]string, 0, len(v.fields))
//		for _, f := range v.fields {
//			validFields = append(validFields, f.name)
//		}
//		assertf(InArray(fieldName, validFields), "%s: enum %s has no field %s", expr.sel.Pos(), enumName, fieldName)
//		expr.x.SetType(selType)
//		expr.SetType(selType)
//	case *StructType:
//		fieldName := expr.sel.lit
//		for _, f := range v.fields {
//			if f.name == fieldName {
//				expr.x.SetType(selType)
//				expr.SetType(f.typ)
//				return
//			}
//		}
//	}
//}
//
//func isNumericType(t Typ) bool {
//	return TryCast[I64Type](t) ||
//		TryCast[I32Type](t) ||
//		TryCast[I16Type](t) ||
//		TryCast[I8Type](t) ||
//		TryCast[IntType](t) ||
//		TryCast[U64Type](t) ||
//		TryCast[U32Type](t) ||
//		TryCast[U16Type](t) ||
//		TryCast[U8Type](t) ||
//		TryCast[F64Type](t) ||
//		TryCast[F32Type](t)
//}
//
//func inferBinOpExpr(expr *BinOpExpr, env *Env) {
//	inferExpr(expr.lhs, nil, env)
//	inferExpr(expr.rhs, nil, env)
//	if TryCast[OptionType](expr.lhs.GetType()) && TryCast[*NoneExpr](expr.rhs) && expr.rhs.GetType() == nil {
//		expr.rhs.SetType(expr.lhs.GetType())
//	}
//	if expr.lhs.GetType() != nil && expr.rhs.GetType() != nil {
//		if isNumericType(expr.lhs.GetType()) && TryCast[UntypedNumType](expr.rhs.GetType()) {
//			expr.rhs.SetType(expr.lhs.GetType())
//		}
//		if isNumericType(expr.rhs.GetType()) && TryCast[UntypedNumType](expr.lhs.GetType()) {
//			expr.lhs.SetType(expr.rhs.GetType())
//		}
//	}
//	switch expr.op.typ {
//	case EQL, NEQ, LOR, LAND, LTE, LT, GTE, GT:
//		expr.SetType(BoolType{})
//	case ADD, MINUS, QUO, MUL, REM:
//		expr.SetType(expr.lhs.GetType())
//	case IN:
//		MustCast[ArrayType](expr.rhs.GetType())
//		eltT := expr.rhs.GetType().(ArrayType).elt
//		assertf(cmpTypesLoose(expr.lhs.GetType(), eltT), "%s mismatched types %s and %s for 'in' operator", expr.Pos(), expr.lhs.GetType(), eltT)
//		return
//	default:
//	}
//	assertf(cmpTypes(expr.lhs.GetType(), expr.rhs.GetType()), "%s mismatched types %s and %s", expr.Pos(), expr.lhs.GetType(), expr.rhs.GetType())
//}
//
//func inferFuncExpr(expr *FuncExpr, env *Env, _ Typ) {
//	ft := funcExprToFuncType(expr, env)
//	expr.SetType(ft)
//	for i, param := range ft.params {
//		env.Define(fmt.Sprintf("%s", expr.args.list[i].names[0].lit), param)
//	}
//	inferStmts(expr.stmts, nil, env)
//	if len(expr.stmts) == 1 && TryCast[*ExprStmt](expr.stmts[0]) { // implicit return
//		if expr.stmts[0].(*ExprStmt).x.GetType() != nil {
//			if expr.GetType() != nil {
//				if t, ok := ft.ret.(*GenericType); ok {
//					ft = ft.ReplaceGenericParameter(t.name, expr.stmts[0].(*ExprStmt).x.GetType())
//					expr.SetTypeForce(ft)
//				}
//			}
//		}
//	}
//}
//
//func inferAnonFnExpr(expr *AnonFnExpr, env *Env, optType Typ) {
//	if optType != nil {
//		expr.SetType(optType)
//	}
//	if expr.GetType() != nil {
//		for i, param := range expr.typ.(FuncType).params {
//			env.Define(fmt.Sprintf("$%d", i), param)
//		}
//	}
//	inferStmts(expr.stmts, nil, env)
//	if len(expr.stmts) == 1 && TryCast[*ExprStmt](expr.stmts[0]) { // implicit return
//		returnStmt := expr.stmts[0].(*ExprStmt)
//		if returnStmt.x.GetType() != nil {
//			if expr.typ != nil {
//				ft := expr.typ.(FuncType)
//				if t, ok := ft.ret.(*GenericType); ok {
//					ft = ft.ReplaceGenericParameter(t.name, returnStmt.x.GetType())
//					expr.SetTypeForce(ft)
//				}
//			}
//		}
//	}
//}
//
//func cmpTypesLoose(a, b Typ) bool {
//	if isNumericType(a) && TryCast[UntypedNumType](b) {
//		return true
//	}
//	if isNumericType(b) && TryCast[UntypedNumType](a) {
//		return true
//	}
//	return cmpTypes(a, b)
//}
//
//func cmpTypes(a, b Typ) bool {
//	if aa, ok := a.(FuncType); ok {
//		if bb, ok := b.(FuncType); ok {
//			if aa.GoStr() == bb.GoStr() {
//				return true
//			}
//			if !cmpTypes(aa.ret, bb.ret) {
//				return false
//			}
//			if len(aa.params) != len(bb.params) {
//				return false
//			}
//			for i := range aa.params {
//				if !cmpTypes(aa.params[i], bb.params[i]) {
//					return false
//				}
//			}
//			return true
//		}
//		return false
//	}
//	if TryCast[*InterfaceType](a) {
//		return true
//	}
//	if TryCast[*GenericType](a) || TryCast[*GenericType](b) {
//		return true
//	}
//	if a == b {
//		return true
//	}
//	if TryCast[OptionType](a) && TryCast[OptionType](b) {
//		return cmpTypes(a.(OptionType).wrappedType, b.(OptionType).wrappedType)
//	}
//	if TryCast[ResultType](a) && TryCast[ResultType](b) {
//		return cmpTypes(a.(ResultType).wrappedType, b.(ResultType).wrappedType)
//	}
//	return false
//}
//
//func inferCallExpr(callExpr *CallExpr, env *Env) {
//	tmpFn := func(id Expr, idT Typ, callExprFun *SelectorExpr) {
//		if arr, ok := idT.(ArrayType); ok {
//			if callExprFun.sel.lit == "filter" {
//				filterFnType := env.Get("agl.Vec.filter").(FuncType)
//				filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
//				callExpr.args[0].SetType(filterFnType.params[1])
//				callExpr.SetType(filterFnType.ret)
//			} else if callExprFun.sel.lit == "map" {
//				filterFnType := env.Get("agl.Vec.map").(FuncType)
//				filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
//				callExpr.args[0].SetType(filterFnType.params[1])
//				callExpr.SetType(filterFnType.ret)
//			} else if callExprFun.sel.lit == "reduce" {
//				filterFnType := env.Get("agl.Vec.reduce").(FuncType)
//				filterFnType = filterFnType.ReplaceGenericParameter("R", env.GetType(callExpr.args[0]))
//				filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
//				callExpr.args[1].SetType(filterFnType.params[2])
//				callExpr.SetType(filterFnType.ret)
//			} else if callExprFun.sel.lit == "sum" {
//				filterFnType := env.Get("agl.Vec.sum").(FuncType)
//				filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
//				callExpr.SetType(filterFnType.ret)
//			} else if callExprFun.sel.lit == "find" {
//				filterFnType := env.Get("agl.Vec.find").(FuncType)
//				filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
//				callExpr.SetType(filterFnType.ret)
//			} else if callExprFun.sel.lit == "joined" {
//				filterFnType := env.Get("agl.Vec.joined").(FuncType)
//				assertf(cmpTypes(idT, filterFnType.params[0]), "%s: type mismatch, wants: %s, got: %s", id.Pos(), filterFnType.params[0], idT)
//				callExpr.SetType(filterFnType.ret)
//			}
//		}
//	}
//
//	switch callExprFun := callExpr.fun.(type) {
//	case *VecExpr:
//		callExprFun.SetType(ArrayType{elt: env.Get(callExprFun.typStr)})
//	case *IdentExpr:
//		callExprFunT := env.Get(callExprFun.lit)
//		assertf(callExprFunT != nil, "%s: undefined identifier %s", callExprFun.Pos(), callExprFun.lit)
//		ft := callExprFunT.(FuncType)
//		oParams := ft.params
//		variadic := ft.variadic
//		if variadic {
//			assertf(len(callExpr.args) >= len(oParams)-1, "%s not enough arguments in call to %s", callExpr.Pos(), callExprFun.lit)
//		} else {
//			assertf(len(oParams) == len(callExpr.args), "%s wrong number of arguments in call to %s, wants: %d, got: %d", callExpr.Pos(), callExprFun.lit, len(oParams), len(callExpr.args))
//		}
//		for i := range callExpr.args {
//			arg := callExpr.args[i]
//			var oArg Typ
//			if i >= len(oParams) {
//				oArg = oParams[len(oParams)-1]
//			} else {
//				oArg = oParams[i]
//			}
//			inferExpr(arg, oArg, env)
//			want, got := oArg, env.GetType(arg)
//			assertf(cmpTypes(want, got), "%s wrong type of argument %d in call to %s, wants: %s, got: %s", arg.Pos(), i, callExprFun.lit, want, got)
//		}
//		callExprFun.SetType(callExprFunT)
//	case *SelectorExpr:
//		switch id := callExprFun.x.(type) {
//		case *IdentExpr:
//			idT1 := env.GetType(id)
//			tmpFn(id, idT1, callExprFun)
//			if l := env.Get(id.lit); l != nil {
//				id.SetType(l)
//				if lT, ok := l.(*StructType); ok {
//					name := fmt.Sprintf("%s.%s", lT.name, callExprFun.sel.lit)
//					callExpr.SetType(env.Get(name).(FuncType).ret)
//				} else if _, ok := l.(PackageType); ok {
//					name := fmt.Sprintf("%s.%s", id.lit, callExprFun.sel.lit)
//					callExpr.SetType(env.Get(name).(FuncType).ret)
//				} else if o, ok := l.(*EnumType); ok {
//					callExpr.SetType(&EnumType{name: o.name, subTyp: callExprFun.sel.lit, fields: o.fields})
//				}
//			}
//			idT := id.GetType()
//			inferVecExtensions(env, idT, callExprFun, callExpr)
//		default:
//			inferExpr(id, nil, env)
//			idT := id.GetType()
//			tmpFn(id, idT, callExprFun)
//			if lT, ok := idT.(*StructType); ok {
//				name := fmt.Sprintf("%s.%s", lT.name, callExprFun.sel.lit)
//				callExpr.SetType(env.Get(name).(FuncType).ret)
//			}
//			inferVecExtensions(env, idT, callExprFun, callExpr)
//		}
//		inferExprs(callExpr.args, env)
//	default:
//		panic(fmt.Sprintf("unexpected type %v %v", callExpr.fun, callExpr.fun.GetType()))
//	}
//	if callExpr.fun.GetType() != nil {
//		if v, ok := callExpr.fun.GetType().(FuncType); ok {
//			callExpr.SetType(v.ret)
//		} else { // Type casting
//			// TODO
//		}
//	}
//}
//
//func compareFunctionSignatures(sig1, sig2 FuncType) bool {
//	// Compare return types
//	if !cmpTypes(sig1.ret, sig2.ret) {
//		return false
//	}
//	// Compare number of parameters
//	if len(sig1.params) != len(sig2.params) {
//		return false
//	}
//	// Compare variadic status
//	if sig1.variadic != sig2.variadic {
//		return false
//	}
//	// Compare each parameter type
//	for i := range sig1.params {
//		if !cmpTypes(sig1.params[i], sig2.params[i]) {
//			return false
//		}
//	}
//	// Compare type parameters if they exist
//	if len(sig1.typeParams) != len(sig2.typeParams) {
//		return false
//	}
//	for i := range sig1.typeParams {
//		if !cmpTypes(sig1.typeParams[i], sig2.typeParams[i]) {
//			return false
//		}
//	}
//	return true
//}
//
//func inferVecExtensions(env *Env, idT Typ, exprT *SelectorExpr, expr *CallExpr) {
//	if TryCast[ArrayType](idT) && exprT.sel.lit == "filter" {
//		clbFnStr := "fn [T any](e T) bool"
//		fs := parseFnSignatureStmt(NewTokenStream(clbFnStr))
//		ft := getFuncType(fs, NewEnv())
//		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
//		if _, ok := expr.args[0].(*AnonFnExpr); ok {
//			expr.args[0].SetTypeForce(ft)
//		} else if _, ok := expr.args[0].(*FuncExpr); ok {
//			ftReal := funcExprToFuncType(expr.args[0].(*FuncExpr), env)
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		} else if ftReal, ok := env.GetType(expr.args[0]).(FuncType); ok {
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		}
//		expr.SetTypeForce(ArrayType{elt: ft.params[0]})
//
//	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "find" {
//		clbFnStr := "fn [T any](e T) bool"
//		fs := parseFnSignatureStmt(NewTokenStream(clbFnStr))
//		ft := getFuncType(fs, NewEnv())
//		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
//		if _, ok := expr.args[0].(*AnonFnExpr); ok {
//			expr.args[0].SetTypeForce(ft)
//		} else if _, ok := expr.args[0].(*FuncExpr); ok {
//			ftReal := funcExprToFuncType(expr.args[0].(*FuncExpr), env)
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		} else if ftReal, ok := env.GetType(expr.args[0]).(FuncType); ok {
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		}
//		expr.SetTypeForce(OptionType{wrappedType: ft.params[0]})
//
//	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "map" {
//		fs := parseFnSignatureStmt(NewTokenStream("fn[T, R any](e T) R"))
//		ft := getFuncType(fs, NewEnv())
//		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
//		if arg0, ok := expr.args[0].(*AnonFnExpr); ok {
//			arg0.SetTypeForce(ft)
//			inferExpr(arg0, nil, env)
//			expr.SetTypeForce(ArrayType{elt: arg0.GetType().(FuncType).ret})
//		} else if arg0, ok := expr.args[0].(*FuncExpr); ok {
//			ftReal := funcExprToFuncType(arg0, env)
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		} else if ftReal, ok := env.GetType(expr.args[0]).(FuncType); ok {
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		}
//
//	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "reduce" {
//		arg0 := expr.args[0].(*NumberExpr)
//		inferExpr(arg0, nil, env)
//		elTyp := idT.(ArrayType).elt
//		fs := parseFnSignatureStmt(NewTokenStream("fn [T any, R cmp.Ordered](acc R, el T) R")) // TODO cmp.Ordered
//		ft := getFuncType(fs, NewEnv())
//		ft = ft.ReplaceGenericParameter("T", elTyp)
//		if _, ok := arg0.GetType().(UntypedNumType); ok {
//			ft = ft.ReplaceGenericParameter("R", elTyp)
//		}
//		if _, ok := expr.args[1].(*AnonFnExpr); ok {
//			expr.args[1].SetTypeForce(ft)
//		} else if _, ok := expr.args[0].(*FuncExpr); ok {
//			ftReal := funcExprToFuncType(expr.args[0].(*FuncExpr), env)
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		} else if ftReal, ok := env.GetType(expr.args[0]).(FuncType); ok {
//			assertf(compareFunctionSignatures(ftReal, ft), "%s: function type %s does not match inferred type %s", expr.Pos(), ftReal, ft)
//		}
//	}
//}
//
//func inferInterfaceType(e *InterfaceStmt, env *Env) {
//	for _, elt := range e.elts {
//		ft := funcExprToFuncType(elt.(*FuncExpr), env)
//		elt.SetType(ft)
//	}
//	env.Define(e.lit, &InterfaceType{name: e.lit})
//}
//
//func inferEnumType(e *EnumStmt, env *Env) {
//	var fields []EnumFieldType
//	for _, f := range e.fields {
//		var elts []string
//		for _, elt := range f.elts {
//			elts = append(elts, elt.(*IdentExpr).lit)
//		}
//		fields = append(fields, EnumFieldType{name: f.name.lit, elts: elts})
//		for _, e := range f.elts {
//			inferExpr(e, nil, env)
//		}
//	}
//	env.Define(e.lit, &EnumType{name: e.lit, fields: fields})
//}
//
//func inferStructType(s *structStmt, env *Env) {
//	inferStructTypeFieldsType(s, env)
//}
//
//func inferStructTypeFieldsType(s *structStmt, env *Env) {
//	var fields []FieldType
//	if s.fields != nil {
//		for _, field := range s.fields {
//			t := env.GetType(field.typeExpr)
//			field.typeExpr.SetType(t)
//			for _, n := range field.names {
//				fields = append(fields, FieldType{name: n.lit, typ: t})
//			}
//		}
//	}
//	env.Define(s.lit, &StructType{name: s.lit, fields: fields})
//}
//
//func getFuncDeclType(env *Env, f *goast.FuncDecl) FuncDeclType {
//	return FuncDeclType{
//		Name: f.Name.Name,
//	}
//}
//
//func getFuncType(f *FuncExpr, env *Env) FuncType {
//	getFuncTypeParamsType(f, env)
//	params, variadic := getFuncArgsType(f, env)
//	return FuncType{
//		params:   params,
//		ret:      getFuncOutType(f, env),
//		variadic: variadic,
//	}
//}
//
//func inferFuncType(f *FuncExpr, env *Env) {
//	inferFuncRecvType(f, env)
//	inferFuncTypeParamsType(f, env)
//	f.typ = FuncType{
//		params: inferFuncArgsType(f, env),
//		ret:    inferFuncOutType(f, env),
//	}
//}
//
//func getFuncTypeParamsType(f *FuncExpr, env *Env) {
//	if f.typeParams != nil {
//		for _, e := range f.typeParams.list {
//			for _, name := range e.names {
//				t := &GenericType{name: name.lit, constraints: []Typ{env.GetType(e.typeExpr)}}
//				env.Define(name.lit, t)
//			}
//		}
//	}
//}
//
//func inferFuncRecvType(f *FuncExpr, env *Env) {
//	if f.recv != nil {
//		for _, e := range f.recv.list {
//			t := env.Get(e.typeExpr.(*IdentExpr).lit)
//			e.typeExpr.SetType(t)
//		}
//	}
//}
//
//func inferFuncTypeParamsType(f *FuncExpr, env *Env) {
//	if f.typeParams != nil {
//		for _, e := range f.typeParams.list {
//			for _, name := range e.names {
//				t := &GenericType{name: name.lit, constraints: []Typ{env.GetType(e.typeExpr)}}
//				e.typeExpr.SetType(t)
//				env.Define(name.lit, t)
//			}
//		}
//	}
//}
//
//type GenericType struct {
//	BaseTyp
//	name        string
//	constraints []Typ
//}
//
//func (t *GenericType) TypeParamGoStr() string {
//	return fmt.Sprintf("%s %s", t.name, t.constraints[0].GoStr())
//}
//
//func (t *GenericType) GoStr() string {
//	return fmt.Sprintf("%s", t.name)
//}
//
//func (t *GenericType) String() string {
//	return fmt.Sprintf("%s", t.name)
//}
//
//func getFuncArgsType(f *FuncExpr, env *Env) (out []Typ, variadic bool) {
//	for _, arg := range f.args.list {
//		if _, ok := arg.typeExpr.(*EllipsisExpr); ok {
//			variadic = true
//		}
//		for range arg.names {
//			out = append(out, env.GetType(arg.typeExpr))
//		}
//	}
//	return
//}
//
//func inferFuncArgsType(f *FuncExpr, env *Env) (out []Typ) {
//	for _, arg := range f.args.list {
//		t := env.GetType(arg.typeExpr)
//		arg.typeExpr.SetType(t)
//		for _, name := range arg.names {
//			env.Define(name.lit, t)
//			out = append(out, t)
//		}
//	}
//	return
//}
//
//func getFuncOutType(f *FuncExpr, env *Env) (out Typ) {
//	if f.out.expr != nil {
//		return env.GetType(f.out.expr)
//	}
//	return VoidType{}
//}
//
//func inferFuncOutType(f *FuncExpr, env *Env) (out Typ) {
//	if f.out.expr != nil {
//		t := env.GetType(f.out.expr)
//		f.out.expr.SetType(t)
//		return t
//	}
//	f.out.expr = &VoidExpr{}
//	f.out.expr.SetType(VoidType{})
//	return VoidType{}
//}
