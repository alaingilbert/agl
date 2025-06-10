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

func funcExprToFuncType(fe *FuncExpr, env *Env) FuncType {
	var typeParams []Typ
	if fe.typeParams != nil {
		for _, p := range fe.typeParams.list {
			t := env.GetType(p.typeExpr)
			for _, n := range p.names {
				gn := &GenericType{name: n.lit, constraints: []Typ{t}}
				env.Define(n.lit, gn)
				typeParams = append(typeParams, gn)
			}
		}
	}
	var params []Typ
	for _, field := range fe.args.list {
		t := env.GetType(field.typeExpr)
		n := max(len(field.names), 1)
		for i := 0; i < n; i++ {
			params = append(params, t)
		}
	}
	ret := env.GetType(fe.out.expr)
	if v, ok := ret.(*ResultType); ok {
		v.native = true
	}
	return FuncType{
		name:       fe.name,
		typeParams: typeParams,
		params:     params,
		ret:        ret,
		isNative:   true,
	}
}

func parseFuncTypeFromString(s string, env *Env) FuncType {
	nenv := env.Clone()
	ft := parseFnSignature(NewTokenStream(s))
	return funcExprToFuncType(ft, nenv)
}

func infer(s *ast) (*ast, *Env) {
	env := NewEnv()
	for _, e := range s.enums {
		inferEnumType(e, env)
	}
	for _, s := range s.interfaces {
		inferInterfaceType(s, env)
	}
	for _, s := range s.structs {
		inferStructType(s, env)
	}
	for _, f := range s.funcs {
		newEnv := env.Clone()
		if f.recv != nil {
			fnName := f.name
			if newName, ok := overloadMapping[fnName]; ok {
				fnName = newName
			}
			name := f.recv.list[0].typeExpr.(*IdentExpr).lit + "." + fnName
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
	switch stmt := s.(type) {
	case *AssignStmt:
		inferAssignStmt(stmt, env)
	case *ValueSpec:
		inferValueSpecStmt(stmt, env)
	case *IfStmt:
		inferExpr(stmt.cond, returnTyp, env)
		inferStmts(stmt.body, returnTyp, env)
		if stmt.Else != nil {
			inferStmt(stmt.Else, returnTyp, env)
		}
	case *IfLetStmt:
		inferExpr(stmt.lhs, returnTyp, env)
		inferExpr(stmt.rhs, returnTyp, env)
		// TODO fix type of variable: p(stmt.lhs, stmt.lhs.GetType())
		nenv := env.Clone()
		var id string
		switch v := stmt.lhs.(type) {
		case *SomeExpr:
			id = v.expr.(*IdentExpr).lit
		case *OkExpr:
			id = v.expr.(*IdentExpr).lit
		case *ErrExpr:
			id = v.expr.(*IdentExpr).lit
		default:
			panic("")
		}
		nenv.Define(id, stmt.rhs.GetType())
		inferStmts(stmt.body, returnTyp, nenv)
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

func inferValueSpecStmt(stmt *ValueSpec, env *Env) {
	inferExpr(stmt.typ, nil, env)
	var rhsT Typ
	if len(stmt.values) > 0 {
		inferExpr(stmt.values[0], stmt.typ.GetType(), env)
		rhsT = stmt.values[0].GetType()
	} else {
		rhsT = stmt.typ.GetType()
	}
	lhs := stmt.names[0]
	env.Define(lhs.lit, rhsT)
	lhs.SetType(rhsT)
}

func inferAssignStmt(stmt *AssignStmt, env *Env) {
	assignFn := Ternary(stmt.tok.typ == WALRUS, env.Define, env.Assign)
	inferExpr(stmt.rhs, nil, env)
	lhs := stmt.lhs
	if v, ok := stmt.lhs.(*MutExpr); ok {
		lhs = v.x
	}

	if lhs, ok := lhs.(*TupleExpr); ok {
		if TryCast[*EnumType](stmt.rhs.GetType()) {
			for i, e := range lhs.exprs {
				lit := stmt.rhs.(*CallExpr).fun.(*SelectorExpr).sel.lit
				fields := stmt.rhs.GetType().(*EnumType).fields
				// AGL: fields.find({ $0.name == lit })
				f := Find(fields, func(f EnumFieldType) bool { return f.name == lit })
				assert(f != nil)
				assignFn(e.(*IdentExpr).lit, env.Get(f.elts[i]))
			}
			return
		}
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
					callExpr.SetType(rhs.typ.(FuncType).ret)
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
	switch expr := e.(type) {
	case *CallExpr:
		inferCallExpr(expr, env)
	case *BubbleOptionExpr:
		inferExpr(expr.x, nil, env)
		expr.SetType(OptionType{wrappedType: expr.x.GetType()})
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
		expr.SetType(expr.exprs[0].GetType())
	case *VecExpr:
		expr.SetType(ArrayType{elt: env.Get(expr.typStr)})
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
	case *TypeAssertExpr:
		inferExpr(expr.x, nil, env)
		inferExpr(expr.typ, nil, env)
		expr.SetType(OptionType{wrappedType: env.GetType(expr.typ)})
	default:
		panic(fmt.Sprintf("unexpected type %v", to(expr)))
	}
	if optType != nil {
		tryConvertType(e, optType)
	}
}

func inferIdentExpr(expr *IdentExpr, env *Env) {
	v := env.Get(expr.lit)
	assertf(v != nil, "%s: undefined identifier %s", expr.Pos(), expr.lit)
	expr.SetType(v)
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
		expected := optType.(TupleType).elts
		for i, x := range expr.exprs {
			if _, ok := x.GetType().(UntypedNumType); ok {
				x.SetType(expected[i])
			}
		}
	} else {
		tupleTyp := TupleType{elts: make([]Typ, len(expr.exprs)), name: fmt.Sprintf("%s%d", TupleStructPrefix, env.structCounter.Add(1))}
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
	case TupleType:
		argIdx, err := strconv.Atoi(expr.sel.lit)
		if err != nil {
			panic("tuple arg index must be int")
		}
		expr.x.SetType(v)
		expr.SetType(v.elts[argIdx])
	case *EnumType:
		enumName := expr.x.(*IdentExpr).lit
		fieldName := expr.sel.lit
		validFields := make([]string, 0, len(v.fields))
		for _, f := range v.fields {
			validFields = append(validFields, f.name)
		}
		assertf(InArray(fieldName, validFields), "%s: enum %s has no field %s", expr.sel.Pos(), enumName, fieldName)
		expr.x.SetType(selType)
		expr.SetType(selType)
	}
}

func isNumericType(t Typ) bool {
	return TryCast[I64Type](t) ||
		TryCast[I32Type](t) ||
		TryCast[I16Type](t) ||
		TryCast[I8Type](t) ||
		TryCast[IntType](t) ||
		TryCast[U64Type](t) ||
		TryCast[U32Type](t) ||
		TryCast[U16Type](t) ||
		TryCast[U8Type](t) ||
		TryCast[F64Type](t) ||
		TryCast[F32Type](t)
}

func inferBinOpExpr(expr *BinOpExpr, env *Env) {
	inferExpr(expr.lhs, nil, env)
	inferExpr(expr.rhs, nil, env)
	if TryCast[OptionType](expr.lhs.GetType()) && TryCast[*NoneExpr](expr.rhs) && expr.rhs.GetType() == nil {
		expr.rhs.SetType(expr.lhs.GetType())
	}
	if expr.lhs.GetType() != nil && expr.rhs.GetType() != nil {
		if isNumericType(expr.lhs.GetType()) && TryCast[UntypedNumType](expr.rhs.GetType()) {
			expr.rhs.SetType(expr.lhs.GetType())
		}
		if isNumericType(expr.rhs.GetType()) && TryCast[UntypedNumType](expr.lhs.GetType()) {
			expr.lhs.SetType(expr.rhs.GetType())
		}
	}
	switch expr.op.typ {
	case EQL, NEQ, LOR, LAND, LTE, LT, GTE, GT:
		expr.SetType(BoolType{})
	case ADD, MINUS, QUO, MUL, REM:
		expr.SetType(expr.lhs.GetType())
	case IN:
		MustCast[ArrayType](expr.rhs.GetType())
		eltT := expr.rhs.GetType().(ArrayType).elt
		assertf(cmpTypesLoose(expr.lhs.GetType(), eltT), "%s mismatched types %s and %s for 'in' operator", expr.Pos(), expr.lhs.GetType(), eltT)
		return
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
		for i, p := range expr.typ.(FuncType).params {
			env.Define(fmt.Sprintf("$%d", i), p)
		}
	}
	inferStmts(expr.stmts, nil, env)
	if len(expr.stmts) == 1 && TryCast[*ExprStmt](expr.stmts[0]) { // implicit return
		if expr.stmts[0].(*ExprStmt).x.GetType() != nil {
			if expr.typ != nil {
				ft := expr.typ.(FuncType)
				if t, ok := ft.ret.(*GenericType); ok {
					ft = ft.ReplaceGenericParameter(t.name, expr.stmts[0].(*ExprStmt).x.GetType())
					expr.SetTypeForce(ft)
				}
			}
		}
	}
}

func cmpTypesLoose(a, b Typ) bool {
	if isNumericType(a) && TryCast[UntypedNumType](b) {
		return true
	}
	if isNumericType(b) && TryCast[UntypedNumType](a) {
		return true
	}
	return cmpTypes(a, b)
}

func cmpTypes(a, b Typ) bool {
	if aa, ok := a.(FuncType); ok {
		if bb, ok := b.(FuncType); ok {
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
	if TryCast[*InterfaceType](a) {
		return true
	}
	if a == b {
		return true
	}
	if TryCast[OptionType](a) && TryCast[OptionType](b) {
		return cmpTypes(a.(OptionType).wrappedType, b.(OptionType).wrappedType)
	}
	if TryCast[*ResultType](a) && TryCast[*ResultType](b) {
		return cmpTypes(a.(*ResultType).wrappedType, b.(*ResultType).wrappedType)
	}
	return false
}

func inferCallExpr(expr *CallExpr, env *Env) {
	switch exprT := expr.fun.(type) {
	case *VecExpr:
		expr.fun.SetType(ArrayType{elt: env.Get(exprT.typStr)})
	case *IdentExpr:
		if exprTT := env.Get(exprT.lit); exprTT != nil {
			ft := exprTT.(FuncType)
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
			if arr, ok := env.GetType(id).(ArrayType); ok {
				if exprT.sel.lit == "filter" {
					filterFnType := env.Get("agl.Vec.filter").(FuncType)
					filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[0].SetType(filterFnType.params[1])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "map" {
					filterFnType := env.Get("agl.Vec.map").(FuncType)
					filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[0].SetType(filterFnType.params[1])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "reduce" {
					filterFnType := env.Get("agl.Vec.reduce").(FuncType)
					filterFnType = filterFnType.ReplaceGenericParameter("R", env.GetType(expr.args[0]))
					filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.args[1].SetType(filterFnType.params[2])
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "sum" {
					filterFnType := env.Get("agl.Vec.sum").(FuncType)
					filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.SetType(filterFnType.ret)
				} else if exprT.sel.lit == "find" {
					filterFnType := env.Get("agl.Vec.find").(FuncType)
					filterFnType = filterFnType.ReplaceGenericParameter("T", arr.elt)
					expr.SetType(filterFnType.ret)
				}
			}
			if l := env.Get(id.lit); l != nil {
				id.SetType(l)
				if lT, ok := l.(*StructType); ok {
					name := fmt.Sprintf("%s.%s", lT.name, exprT.sel.lit)
					expr.SetType(env.Get(name).(FuncType).ret)
				} else if _, ok := l.(PackageType); ok {
					name := fmt.Sprintf("%s.%s", id.lit, exprT.sel.lit)
					expr.SetType(env.Get(name).(FuncType).ret)
				} else if _, ok := l.(*EnumType); ok {
					expr.SetType(l)
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
		panic(fmt.Sprintf("unexpected type %v %v", expr.fun, expr.fun.GetType()))
	}
	if expr.fun.GetType() != nil {
		if v, ok := expr.fun.GetType().(FuncType); ok {
			expr.SetType(v.ret)
		} else { // Type casting
			// TODO
		}
	}
}

func inferVecExtensions(env *Env, idT Typ, exprT *SelectorExpr, expr *CallExpr) {
	if TryCast[ArrayType](idT) && exprT.sel.lit == "filter" {
		clbFnStr := "fn [T any](e T) bool"
		fs := parseFnSignatureStmt(NewTokenStream(clbFnStr))
		ft := getFuncType(fs, NewEnv())
		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
		expr.args[0].SetTypeForce(ft)
		expr.SetTypeForce(ArrayType{elt: ft.params[0]})

	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "find" {
		clbFnStr := "fn [T any](e T) bool"
		fs := parseFnSignatureStmt(NewTokenStream(clbFnStr))
		ft := getFuncType(fs, NewEnv())
		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
		expr.args[0].SetTypeForce(ft)
		expr.SetTypeForce(ArrayType{elt: ft.params[0]})

	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "map" {
		fs := parseFnSignatureStmt(NewTokenStream("fn[T, R any](e T) R"))
		ft := getFuncType(fs, NewEnv())
		ft = ft.ReplaceGenericParameter("T", idT.(ArrayType).elt)
		switch arg0 := expr.args[0].(type) {
		case *AnonFnExpr:
			arg0.SetType(ft)
		case *SelectorExpr: // TODO
		default:
			panic(fmt.Sprintf("unexpected type %v", reflect.TypeOf(arg0)))
		}
		expr.SetTypeForce(ArrayType{elt: ft.params[0]})

	} else if TryCast[ArrayType](idT) && exprT.sel.lit == "reduce" {
		arg0 := expr.args[0].(*NumberExpr)
		inferExpr(arg0, nil, env)
		elTyp := idT.(ArrayType).elt
		fs := parseFnSignatureStmt(NewTokenStream("fn [T any, R cmp.Ordered](acc R, el T) R")) // TODO cmp.Ordered
		ft := getFuncType(fs, NewEnv())
		ft = ft.ReplaceGenericParameter("T", elTyp)
		if _, ok := arg0.GetType().(UntypedNumType); ok {
			ft = ft.ReplaceGenericParameter("R", elTyp)
		}
		expr.args[1].SetTypeForce(ft)
	}
}

func inferInterfaceType(e *InterfaceStmt, env *Env) {
	for _, elt := range e.elts {
		ft := funcExprToFuncType(elt.(*FuncExpr), env)
		elt.SetType(ft)
	}
	env.Define(e.lit, &InterfaceType{name: e.lit})
}

func inferEnumType(e *EnumStmt, env *Env) {
	var fields []EnumFieldType
	for _, f := range e.fields {
		var elts []string
		for _, elt := range f.elts {
			elts = append(elts, elt.(*IdentExpr).lit)
		}
		fields = append(fields, EnumFieldType{name: f.name.lit, elts: elts})
		for _, e := range f.elts {
			inferExpr(e, nil, env)
		}
	}
	env.Define(e.lit, &EnumType{name: e.lit, fields: fields})
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

func getFuncType(f *FuncExpr, env *Env) FuncType {
	getFuncTypeParamsType(f, env)
	params, variadic := getFuncArgsType(f, env)
	return FuncType{
		params:   params,
		ret:      getFuncOutType(f, env),
		variadic: variadic,
	}
}

func inferFuncType(f *FuncExpr, env *Env) {
	inferFuncRecvType(f, env)
	inferFuncTypeParamsType(f, env)
	f.typ = FuncType{
		params: inferFuncArgsType(f, env),
		ret:    inferFuncOutType(f, env),
	}
}

func getFuncTypeParamsType(f *FuncExpr, env *Env) {
	if f.typeParams != nil {
		for _, e := range f.typeParams.list {
			for _, name := range e.names {
				t := &GenericType{name: name.lit, constraints: []Typ{env.GetType(e.typeExpr)}}
				env.Define(name.lit, t)
			}
		}
	}
}

func inferFuncRecvType(f *FuncExpr, env *Env) {
	if f.recv != nil {
		for _, e := range f.recv.list {
			t := env.Get(e.typeExpr.(*IdentExpr).lit)
			e.typeExpr.SetType(t)
		}
	}
}

func inferFuncTypeParamsType(f *FuncExpr, env *Env) {
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

func getFuncArgsType(f *FuncExpr, env *Env) (out []Typ, variadic bool) {
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

func inferFuncArgsType(f *FuncExpr, env *Env) (out []Typ) {
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

func getFuncOutType(f *FuncExpr, env *Env) (out Typ) {
	if f.out.expr != nil {
		return env.GetType(f.out.expr)
	}
	return VoidType{}
}

func inferFuncOutType(f *FuncExpr, env *Env) (out Typ) {
	if f.out.expr != nil {
		t := env.GetType(f.out.expr)
		f.out.expr.SetType(t)
		return t
	}
	f.out.expr = &VoidExpr{}
	f.out.expr.SetType(VoidType{})
	return VoidType{}
}
