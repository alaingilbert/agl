package main

import (
	"fmt"
	"reflect"
	"strconv"
)

const (
	TupleStructPrefix = "AglTupleStruct"
	AglVariablePrefix = "aglVar"
)

func parseFuncTypeFromString(s string, env *Env) *FuncType {
	nenv := env.Clone()
	ft := parseFnSignature(NewTokenStream(s))
	var typeParams []Typ
	if ft.typeParams != nil {
		for _, p := range ft.typeParams.list {
			t := nenv.GetType(p.typeExpr)
			for _, n := range p.names {
				gn := &GenericType{name: n.lit, constraints: []Typ{t}}
				nenv.Define(n.lit, gn)
				typeParams = append(typeParams, gn)
			}
		}
	}
	var params []Typ
	for _, field := range ft.params.list {
		t := nenv.GetType(field.typeExpr)
		n := max(len(field.names), 1)
		for i := 0; i < n; i++ {
			params = append(params, t)
		}
	}
	ret := nenv.GetType(ft.result.expr)
	if v, ok := ret.(ResultType); ok {
		v.native = true
	}
	return &FuncType{
		typeParams: typeParams,
		params:     params,
		ret:        ret,
		isNative:   true,
	}
}

func infer(s *ast) (*ast, *Env) {
	env := NewEnv()
	for _, e := range s.enums {
		inferEnumType(e, env)
	}
	for _, s := range s.structs {
		inferStructType(s, env)
	}
	for _, f := range s.funcs {
		newEnv := env.Clone()
		if f.recv != nil {
			name := f.recv.list[0].typeExpr.(*IdentExpr).lit + "." + f.name
			env.Define(name, getFuncType(f, newEnv))
		} else {
			env.Define(f.name, getFuncType(f, newEnv))
		}
	}
	for _, f := range s.funcs {
		newEnv := env.Clone()
		inferFuncType(f, newEnv)
		fOutTyp := f.out.expr.GetType()
		inferStmts(f.stmts, fOutTyp, newEnv)
	}
	return s, env
}

func inferStmts(stmts []Stmt, returnTyp Typ, env *Env) {
	for _, ss := range stmts {
		inferStmt(ss, returnTyp, env)
	}
}

func inferStmt(s Stmt, returnTyp Typ, env *Env) {
	//fmt.Println("inferStmt:", s)
	switch stmt := s.(type) {
	case *AssignStmt:
		inferAssignStmt(stmt, env)
	case *IfStmt:
		inferExpr(stmt.cond, returnTyp, env)
		inferStmts(stmt.body, returnTyp, env)
		if stmt.Else != nil {
			inferStmt(stmt.Else, returnTyp, env)
		}
	case *ReturnStmt:
		inferExpr(stmt.expr, returnTyp, env)
	case *AssertStmt:
		inferAssertStmt(stmt, env)
	case *ExprStmt:
		inferExpr(stmt.x, nil, env)
	case *InlineCommentStmt:
	case *ForRangeStmt:
	case *IncDecStmt:
	case *BlockStmt:
		inferStmts(stmt.stmts, returnTyp, env)
	default:
		panic(fmt.Sprintf("unexpected type %v", to(stmt)))
	}
}

func inferAssertStmt(stmt *AssertStmt, env *Env) {
	inferExpr(stmt.x, nil, env)
	if stmt.msg != nil {
		inferExpr(stmt.msg, nil, env)
	}
}

func inferAssignStmt(stmt *AssignStmt, env *Env) {
	assignFn := Ternary(stmt.tok.typ == WALRUS, env.Define, env.Assign)
	inferExpr(stmt.rhs, nil, env)
	lhs := stmt.lhs
	if v, ok := stmt.lhs.(*MutExpr); ok {
		lhs = v.x
	}

	if lhs, ok := lhs.(*TupleExpr); ok {
		MustCast[*TupleExpr](stmt.rhs)
		for i, x := range stmt.rhs.(*TupleExpr).exprs {
			MustCast[*IdentExpr](lhs.exprs[i])
			lhs.exprs[i].SetType(x.GetType())
			assignFn(lhs.exprs[i].(*IdentExpr).lit, x.GetType())
		}
		return
	}

	lhsID := MustCast[*IdentExpr](lhs)
	switch rhs := stmt.rhs.(type) {
	case *BubbleResultExpr:
		callExpr := MustCast[*CallExpr](rhs.x)
		switch s := callExpr.fun.(type) {
		case *SelectorExpr:
			if id, ok := s.x.(*IdentExpr); ok {
				if rhs.GetType() == nil {
					rhs.SetType(env.Get(fmt.Sprintf("%s.%s", id.lit, s.sel.lit)))
					callExpr.SetType(rhs.typ.(*FuncType).ret)
				}
			}
		}
	}
	lhsID.SetType(stmt.rhs.GetType())
	assignFn(lhsID.lit, lhsID.typ)
}

func inferExprs(e []Expr, env *Env) {
	for _, expr := range e {
		inferExpr(expr, nil, env)
	}
}

func inferExpr(e Expr, optType Typ, env *Env) {
	//fmt.Println("inferExpr:", e)
	switch expr := e.(type) {
	case *CallExpr:
		inferCallExpr(expr, env)
	case *BubbleOptionExpr:
		inferExpr(expr.x, nil, env)
	case *BubbleResultExpr:
		inferExpr(expr.x, nil, env)
		expr.SetType(expr.x.GetType())
	case *NumberExpr:
		expr.SetType(UntypedNumType{})
	case *NoneExpr:
	case *SomeExpr:
	case *OkExpr:
	case *ErrExpr:
	case *BinOpExpr:
		inferBinOpExpr(expr, env)
	case *SelectorExpr:
		inferSelectorExpr(expr, env)
	case *TupleExpr:
		inferTupleExpr(expr, env, optType)
	case *IdentExpr:
		inferIdentExpr(expr, env)
	case *MakeExpr:
		inferExprs(expr.exprs, env)
	case *VecExpr:
		expr.SetType(ArrayTypeTyp{elt: env.Get(expr.typStr)})
	case *StringExpr:
		expr.SetType(StringType{})
	case *AnonFnExpr:
		inferAnonFnExpr(expr, env.Clone(), optType)
	case *TrueExpr:
		expr.SetType(BoolType{})
	case *FalseExpr:
		expr.SetType(BoolType{})
	case *CompositeLitExpr:
		expr.SetType(env.Get(expr.typ.(*IdentExpr).lit))
	default:
		panic(fmt.Sprintf("unexpected type %v", to(expr)))
	}
	if optType != nil {
		tryConvertType(e, optType)
	}
}

func inferIdentExpr(expr *IdentExpr, env *Env) {
	expr.SetType(env.Get(expr.lit))
}

func tryConvertType(e Expr, optType Typ) {
	if e.GetType() == nil {
		e.SetType(optType)
	} else if _, ok := e.GetType().(UntypedNumType); ok {
		if TryCast[U8Type](optType) ||
			TryCast[U16Type](optType) ||
			TryCast[U32Type](optType) ||
			TryCast[U64Type](optType) ||
			TryCast[I8Type](optType) ||
			TryCast[I16Type](optType) ||
			TryCast[I32Type](optType) ||
			TryCast[I64Type](optType) ||
			TryCast[IntType](optType) ||
			TryCast[UintType](optType) {
			e.SetType(optType)
		}
	}
}

func inferTupleExpr(expr *TupleExpr, env *Env, optType Typ) {
	inferExprs(expr.exprs, env)
	if optType != nil {
		expected := optType.(TupleTypeTyp).elts
		for i, x := range expr.exprs {
			if _, ok := x.GetType().(UntypedNumType); ok {
				x.SetType(expected[i])
			}
		}
	} else {
		tupleTyp := TupleTypeTyp{elts: make([]Typ, len(expr.exprs)), name: fmt.Sprintf("%s%d", TupleStructPrefix, env.structCounter.Add(1))}
		for i, x := range expr.exprs {
			if _, ok := x.GetType().(UntypedNumType); ok {
				x.SetType(IntType{})
				tupleTyp.elts[i] = x.(*NumberExpr).typ
			} else {
				tupleTyp.elts[i] = x.GetType()
			}
		}
		expr.SetType(tupleTyp)
	}
}

func inferSelectorExpr(expr *SelectorExpr, env *Env) {
	selType := env.Get(expr.x.(*IdentExpr).lit)
	switch v := selType.(type) {
	case TupleTypeTyp:
		argIdx, err := strconv.Atoi(expr.sel.lit)
		if err != nil {
			panic("tuple arg index must be int")
		}
		expr.x.SetType(v)
		expr.SetType(v.elts[argIdx])
	case *EnumType:
		expr.x.SetType(selType)
		expr.SetType(selType)
	}
}

func inferBinOpExpr(expr *BinOpExpr, env *Env) {
	inferExpr(expr.lhs, nil, env)
	inferExpr(expr.rhs, nil, env)
	if TryCast[OptionType](expr.lhs.GetType()) && TryCast[*NoneExpr](expr.rhs) && expr.rhs.GetType() == nil {
		expr.rhs.SetType(expr.lhs.GetType())
	}
	if expr.lhs.GetType() != nil && expr.rhs.GetType() != nil {
		// TODO
		if (TryCast[I64Type](expr.lhs.GetType()) ||
			TryCast[I32Type](expr.lhs.GetType()) ||
			TryCast[I16Type](expr.lhs.GetType()) ||
			TryCast[I8Type](expr.lhs.GetType()) ||
			TryCast[IntType](expr.lhs.GetType()) ||
			TryCast[U64Type](expr.lhs.GetType()) ||
			TryCast[U32Type](expr.lhs.GetType()) ||
			TryCast[U16Type](expr.lhs.GetType()) ||
			TryCast[U8Type](expr.lhs.GetType()) ||
			TryCast[F64Type](expr.lhs.GetType()) ||
			TryCast[F32Type](expr.lhs.GetType())) &&
			TryCast[UntypedNumType](expr.rhs.GetType()) {
			expr.rhs.SetType(expr.lhs.GetType())
		}
		if (TryCast[I64Type](expr.rhs.GetType()) ||
			TryCast[I32Type](expr.rhs.GetType()) ||
			TryCast[I16Type](expr.rhs.GetType()) ||
			TryCast[I8Type](expr.rhs.GetType()) ||
			TryCast[IntType](expr.rhs.GetType()) ||
			TryCast[U64Type](expr.rhs.GetType()) ||
			TryCast[U32Type](expr.rhs.GetType()) ||
			TryCast[U16Type](expr.rhs.GetType()) ||
			TryCast[U8Type](expr.rhs.GetType()) ||
			TryCast[F64Type](expr.rhs.GetType()) ||
			TryCast[F32Type](expr.rhs.GetType())) &&
			TryCast[UntypedNumType](expr.lhs.GetType()) {
			expr.lhs.SetType(expr.rhs.GetType())
		}
	}
	switch expr.op.typ {
	case EQL, NEQ, LOR, LAND, LTE, LT, GTE, GT:
		expr.SetType(BoolType{})
	case ADD, MINUS, QUO, MUL, REM:
		expr.SetType(expr.lhs.GetType())
	default:
	}
	//fmt.Println("ASSERT", expr.lhs, "|||||", expr.rhs)
	assertf(cmpTypes(expr.lhs.GetType(), expr.rhs.GetType()), "%s mismatched types %s and %s", expr.Pos(), expr.lhs.GetType(), expr.rhs.GetType())
}

func inferAnonFnExpr(expr *AnonFnExpr, env *Env, optType Typ) {
	if optType != nil {
		expr.SetType(optType)
	}
	if expr.GetType() != nil {
		for i, p := range expr.typ.(*FuncType).params {
			env.Define(fmt.Sprintf("$%d", i), p)
		}
	}
	inferStmts(expr.stmts, nil, env)
	if len(expr.stmts) == 1 && TryCast[*ExprStmt](expr.stmts[0]) { // implicit return
		if expr.stmts[0].(*ExprStmt).x.GetType() != nil {
			if expr.typ != nil {
				ft := expr.typ.(*FuncType)
				if t, ok := ft.ret.(*GenericType); ok {
					ft.ReplaceGenericParameter(t.name, expr.stmts[0].(*ExprStmt).x.GetType())
				}
			}
		}
	}
}

func cmpTypes(a, b Typ) bool {
	if a == b {
		return true
	}
	if aa, ok := a.(*FuncType); ok {
		if bb, ok := b.(*FuncType); ok {
			if aa.GoStr() == bb.GoStr() {
				return true
			}
			if !cmpTypes(aa.ret, bb.ret) {
				return false
			}
			if len(aa.params) != len(bb.params) {
				return false
			}
			for i := range aa.params {
				if !cmpTypes(aa.params[i], bb.params[i]) {
					return false
				}
			}
			return true
		}
		return false
	}
	if TryCast[OptionType](a) && TryCast[OptionType](b) {
		return cmpTypes(a.(OptionType).wrappedType, b.(OptionType).wrappedType)
	}
	if TryCast[ResultType](a) && TryCast[ResultType](b) {
		return cmpTypes(a.(ResultType).wrappedType, b.(ResultType).wrappedType)
	}
	return false
}

func inferCallExpr(expr *CallExpr, env *Env) {
	switch exprT := expr.fun.(type) {
	case *IdentExpr:
		if exprTT := env.Get(exprT.lit); exprTT != nil {
			ft := exprTT.(*FuncType)
			oParams := ft.params
			variadic := ft.variadic
			if variadic {
				assertf(len(expr.args) >= len(oParams)-1, "%s not enough arguments in call to %s", expr.Pos(), exprT.lit)
			} else {
				assertf(len(oParams) == len(expr.args), "%s wrong number of arguments in call to %s, wants: %d, got: %d", expr.Pos(), exprT.lit, len(oParams), len(expr.args))
			}
			for i := range expr.args {
				arg := expr.args[i]
				var oArg Typ
				if i >= len(oParams) {
					oArg = oParams[len(oParams)-1]
				} else {
					oArg = oParams[i]
				}
				inferExpr(arg, oArg, env)
				want := oArg
				got := env.GetType(arg)
				assertf(cmpTypes(want, got), "%s wrong type of argument %d in call to %s, wants: %s, got: %s", arg.Pos(), i, exprT.lit, want, got)
			}
			exprT.SetType(exprTT)
		}
	case *SelectorExpr:
		switch id := exprT.x.(type) {
		case *IdentExpr:
			if arr, ok := env.GetType(id).(ArrayTypeTyp); ok {
				if exprT.sel.lit == "filter" {
					filterFnType := env.Get("agl.Vec.filter").(*FuncType)
					filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[0].SetType(filterFnType.params[1])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "map" {
					filterFnType := env.Get("agl.Vec.map").(*FuncType)
					filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[0].SetType(filterFnType.params[1])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "reduce" {
					filterFnType := env.Get("agl.Vec.reduce").(*FuncType)
					filterFnType.ReplaceGenericParameter("R", env.GetType(expr.args[0]))
					filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[1].SetType(filterFnType.params[2])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "sum" {
					filterFnType := env.Get("agl.Vec.sum").(*FuncType)
					filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.SetType(filterFnType.ret)
				}
			}
			if l := env.Get(id.lit); l != nil {
				id.SetType(l)
				if lT, ok := l.(*StructType); ok {
					name := fmt.Sprintf("%s.%s", lT.name, exprT.sel.lit)
					expr.SetType(env.Get(name).(*FuncType).ret)
				}
			}
			idT := id.GetType()
			inferVecExtensions(env, idT, exprT, expr)
		default:
			inferExpr(id, nil, env)
			idT := id.GetType()
			expr.SetType(idT)
			inferVecExtensions(env, idT, exprT, expr)
		}
		inferExprs(expr.args, env)
	default:
		panic(fmt.Sprintf("unexpected type %v", to(expr)))
	}
	if expr.fun.GetType() != nil {
		expr.SetType(expr.fun.GetType().(*FuncType).ret)
	}
}

func inferVecExtensions(env *Env, idT Typ, exprT *SelectorExpr, expr *CallExpr) {
	if TryCast[ArrayTypeTyp](idT) && exprT.sel.lit == "filter" {
		clbFnStr := "fn [T any](e T) bool"
		fs := parseFnSignatureStmt(NewTokenStream(clbFnStr))
		fs.typ = getFuncType(fs, NewEnv())
		fs.typ.(*FuncType).params[0] = idT.(ArrayTypeTyp).elt
		expr.args[0].SetType(fs.typ)
		expr.SetTypeForce(ArrayTypeTyp{elt: fs.typ.(*FuncType).params[0]})

	} else if TryCast[ArrayTypeTyp](idT) && exprT.sel.lit == "map" {
		fs := parseFnSignatureStmt(NewTokenStream("fn[T, R any](e T) R"))
		fs.typ = getFuncType(fs, NewEnv())
		fs.typ.(*FuncType).params[0] = idT.(ArrayTypeTyp).elt
		switch arg0 := expr.args[0].(type) {
		case *AnonFnExpr:
			arg0.SetType(fs.typ)
		case *SelectorExpr:
			t := env.Get(fmt.Sprintf("%s.%s", arg0.x.(*IdentExpr).lit, arg0.sel.lit))
			p("????", arg0.x.(*IdentExpr).lit, arg0.sel.lit, t.(*FuncType).params[0], t.(*FuncType).ret)
		default:
			panic(fmt.Sprintf("unexpected type %v", reflect.TypeOf(arg0)))
		}
		//inferExprs(expr.args, env)
		expr.SetTypeForce(ArrayTypeTyp{elt: fs.typ.(*FuncType).params[0]})

	} else if TryCast[ArrayTypeTyp](idT) && exprT.sel.lit == "reduce" {
		fs := parseFnSignatureStmt(NewTokenStream("fn [T any, R cmpOrdered](acc R, el T) R")) // TODO cmp.Ordered
		fs.typ = getFuncType(fs, NewEnv())
		inferExpr(expr.args[0], nil, env)
		elTyp := idT.(ArrayTypeTyp).elt
		fs.typ.(*FuncType).params[1] = elTyp
		if _, ok := expr.args[0].(*NumberExpr).typ.(UntypedNumType); ok {
			fs.typ.(*FuncType).params[0] = elTyp
			fs.typ.(*FuncType).ReplaceGenericParameter("R", fs.typ.(*FuncType).params[0])
		}
		expr.args[1].SetTypeForce(fs.typ)
	}
}

func inferEnumType(e *EnumStmt, env *Env) {
	env.Define(e.lit, &EnumType{name: e.lit})
}

func inferStructType(s *structStmt, env *Env) {
	inferStructTypeFieldsType(s, env)
}

func inferStructTypeFieldsType(s *structStmt, env *Env) {
	env.Define(s.lit, &StructType{name: s.lit})
	if s.fields != nil {
		for _, field := range s.fields {
			field.typeExpr.SetType(env.GetType(field.typeExpr))
		}
	}
}

func getFuncType(f *funcStmt, env *Env) *FuncType {
	getFuncTypeParamsType(f, env)
	params, variadic := getFuncArgsType(f, env)
	return &FuncType{
		params:   params,
		ret:      getFuncOutType(f, env),
		variadic: variadic,
	}
}

func inferFuncType(f *funcStmt, env *Env) {
	inferFuncRecvType(f, env)
	inferFuncTypeParamsType(f, env)
	f.typ = &FuncType{
		params: inferFuncArgsType(f, env),
		ret:    inferFuncOutType(f, env),
	}
}

func getFuncTypeParamsType(f *funcStmt, env *Env) {
	if f.typeParams != nil {
		for _, e := range f.typeParams.list {
			for _, name := range e.names {
				t := &GenericType{name: name.lit, constraints: []Typ{env.GetType(e.typeExpr)}}
				env.Define(name.lit, t)
			}
		}
	}
}

func inferFuncRecvType(f *funcStmt, env *Env) {
	if f.recv != nil {
		for _, e := range f.recv.list {
			t := env.Get(e.typeExpr.(*IdentExpr).lit)
			e.typeExpr.SetType(t)
		}
	}
}

func inferFuncTypeParamsType(f *funcStmt, env *Env) {
	if f.typeParams != nil {
		for _, e := range f.typeParams.list {
			for _, name := range e.names {
				t := &GenericType{name: name.lit, constraints: []Typ{env.GetType(e.typeExpr)}}
				e.typeExpr.SetType(t)
				env.Define(name.lit, t)
			}
		}
	}
}

type GenericType struct {
	BaseTyp
	name        string
	constraints []Typ
}

func (t *GenericType) TypeParamGoStr() string {
	return fmt.Sprintf("%s %s", t.name, t.constraints[0].GoStr())
}

func (t *GenericType) GoStr() string {
	return fmt.Sprintf("%s", t.name)
}

func (t *GenericType) String() string {
	return fmt.Sprintf("GenType(%s %s)", t.name, t.constraints[0].GoStr())
}

func getFuncArgsType(f *funcStmt, env *Env) (out []Typ, variadic bool) {
	for _, arg := range f.args.list {
		if _, ok := arg.typeExpr.(*EllipsisExpr); ok {
			variadic = true
		}
		for range arg.names {
			out = append(out, env.GetType(arg.typeExpr))
		}
	}
	return
}

func inferFuncArgsType(f *funcStmt, env *Env) (out []Typ) {
	for _, arg := range f.args.list {
		t := env.GetType(arg.typeExpr)
		arg.typeExpr.SetType(t)
		for _, name := range arg.names {
			env.Define(name.lit, t)
			out = append(out, t)
		}
	}
	return
}

func getFuncOutType(f *funcStmt, env *Env) (out Typ) {
	if f.out.expr != nil {
		return env.GetType(f.out.expr)
	}
	return VoidType{}
}

func inferFuncOutType(f *funcStmt, env *Env) (out Typ) {
	if f.out.expr != nil {
		t := env.GetType(f.out.expr)
		f.out.expr.SetType(t)
		return t
	}
	f.out.expr = &VoidExpr{}
	f.out.expr.SetType(VoidType{})
	return VoidType{}
}
