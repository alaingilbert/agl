package main

import (
	goast "agl/ast"
	"agl/parser"
	"agl/token"
	"agl/types"
	"fmt"
	"maps"
	"reflect"
	"sync/atomic"
)

type Env struct {
	fset          *token.FileSet
	structCounter atomic.Int64
	lookupTable   map[string]types.Type    // Store constants/variables/functions
	lookupTable2  map[token.Pos]types.Type // Store type for Expr/Stmt
}

func funcTypeToFuncType(name string, expr *goast.FuncType, env *Env) types.FuncType {
	var paramsT []types.Type
	if expr.TypeParams != nil {
		for _, typeParam := range expr.TypeParams.List {
			for _, typeParamName := range typeParam.Names {
				typeParamType := typeParam.Type
				t := env.GetType2(typeParamType)
				t = types.GenericType{W: t, Name: typeParamName.Name}
				env.Define(typeParamName.Name, t)
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
	}
	ft := types.FuncType{
		Name:       name,
		TypeParams: paramsT,
		Params:     params,
		Return:     result,
	}
	return ft
}

func parseFuncTypeFromStringNative(name, s string, env *Env) types.FuncType {
	env = env.Clone()
	e, err := parser.ParseExpr(s)
	if err != nil {
		panic(err)
	}
	expr := e.(*goast.FuncType)
	return funcTypeToFuncType(name, expr, env)
}

func NewEnv(fset *token.FileSet) *Env {
	env := &Env{fset: fset, lookupTable2: make(map[token.Pos]types.Type), lookupTable: make(map[string]types.Type)}
	env.Define("any", types.AnyType{})
	env.Define("i64", types.I64Type{})
	env.Define("u8", types.U8Type{})
	env.Define("int", types.IntType{})
	env.Define("string", types.StringType{})
	env.Define("bool", types.BoolType{})
	env.Define("cmp.Ordered", types.AnyType{})
	env.Define("assert", parseFuncTypeFromStringNative("assert", "func (pred bool, msg ...string)", env))
	env.Define("Some", parseFuncTypeFromStringNative("Some", "func[T any]()", env))
	env.Define("Ok", parseFuncTypeFromStringNative("Some", "func[T any]()", env))
	env.Define("Err", parseFuncTypeFromStringNative("Some", "func[T any]()", env))
	env.Define("make", parseFuncTypeFromStringNative("make", "func[T, U any](t T, size ...U) T", env))
	env.Define("fmt.Println", parseFuncTypeFromStringNative("Println", "func(a ...any) int!", env))
	env.Define("strconv.Itoa", parseFuncTypeFromStringNative("Itoa", "func(int) string", env))
	env.Define("agl.Vec.filter", parseFuncTypeFromStringNative("Filter", "func [T any](a []T, f func(e T) bool) []T", env))
	env.Define("agl.Vec.Map", parseFuncTypeFromStringNative("Map", "func [T, R any](a []T, f func(T) R) []R", env))
	env.Define("agl.Vec.Reduce", parseFuncTypeFromStringNative("Reduce", "func [T any, R cmp.Ordered](a []T, r R, f func(a R, e T) R) R", env)) // Fix R cmp.Ordered
	return env
}

func (e *Env) Clone() *Env {
	env := &Env{
		fset:         e.fset,
		lookupTable:  maps.Clone(e.lookupTable),
		lookupTable2: e.lookupTable2,
	}
	return env
}

func (e *Env) Get(name string) types.Type {
	return e.lookupTable[name]
}

func (e *Env) Define(name string, typ types.Type) {
	//p("Define", name, typ)
	assertf(e.Get(name) == nil, "duplicate declaration of %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) Assign(name string, typ types.Type) {
	if name == "_" {
		return
	}
	assertf(e.Get(name) != nil, "undeclared %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) SetType(x goast.Node, t types.Type) {
	assertf(t != nil, "%s: try to set type nil, %v %v", e.fset.Position(x.Pos()), x, to(x))
	e.lookupTable2[x.Pos()] = t
}

func (e *Env) GetType(x goast.Node) types.Type {
	if v, ok := e.lookupTable2[x.Pos()]; ok {
		return v
	}
	return nil
}

func (e *Env) GetType2(x goast.Node) types.Type {
	if v, ok := e.lookupTable2[x.Pos()]; ok {
		return v
	}
	switch xx := x.(type) {
	case *goast.Ident:
		if v2, ok := e.lookupTable[xx.Name]; ok {
			return v2
		}
		return nil
	case *goast.FuncType:
		return funcTypeToFuncType("", xx, e)
	case *goast.Ellipsis:
		return types.EllipsisType{}
	case *goast.ArrayType:
		return types.ArrayType{Elt: e.GetType2(xx.Elt)}
	case *goast.ResultExpr:
		return types.ResultType{}
	case *goast.BasicLit:
		switch xx.Kind {
		case token.INT:
			return types.UntypedNumType{}
		default:
			panic("")
		}
	case *goast.SelectorExpr:
		return e.GetType2(&goast.Ident{Name: fmt.Sprintf("%s.%s", xx.X.(*goast.Ident).Name, xx.Sel.Name)})
	default:
		panic(fmt.Sprintf("unhandled type %v %v", xx, reflect.TypeOf(xx)))
	}
}

//func (e *Env) GetType(x Expr) Typ {
//	return e.getType(x, false)
//}
//
//func (e *Env) getType(x Expr, native bool) Typ {
//	oType := x.GetType()
//	if oType != nil {
//		return oType
//	}
//	switch v := x.(type) {
//	case *IdentExpr:
//		return e.strToType(v.lit)
//	case *OptionExpr:
//		return OptionType{wrappedType: e.getType(v.x, native)}
//	case *ResultExpr:
//		return ResultType{wrappedType: e.getType(v.x, native), native: native}
//	case *BubbleOptionExpr: // TODO
//		return OptionType{wrappedType: e.getType(v.x, native)}
//	case *BubbleResultExpr: // TODO
//		return ResultType{wrappedType: e.getType(v.x, native), native: native}
//	case *ArrayTypeExpr:
//		return ArrayType{elt: e.getType(v.elt, native)}
//	case *SelectorExpr:
//		return e.getType(&IdentExpr{lit: fmt.Sprintf("%s.%s", v.x.(*IdentExpr).lit, v.sel.lit)}, native)
//	case *TrueExpr:
//		return BoolType{}
//	case *StringExpr:
//		return StringType{}
//	case *NumberExpr:
//		return UntypedNumType{}
//	case *EllipsisExpr:
//		return e.getType(v.x, native)
//	case *AnonFnExpr:
//		return nil
//	case *VecExpr:
//		return nil
//	case *VoidExpr:
//		return VoidType{}
//	case *BinOpExpr:
//		return nil
//	case *TupleTypeExpr:
//		var elements []Typ
//		for _, el := range v.exprs {
//			elements = append(elements, e.getType(el, native))
//		}
//		structName := fmt.Sprintf("%s%d", TupleStructPrefix, e.structCounter.Add(1))
//		return TupleType{elts: elements, name: structName}
//	case *FuncExpr:
//		var params []Typ
//		for _, field := range v.args.list {
//			fieldT := e.getType(field.typeExpr, native)
//			n := max(len(field.names), 1)
//			for i := 0; i < n; i++ {
//				params = append(params, fieldT)
//			}
//		}
//		return FuncType{
//			params: params,
//			ret:    e.getType(v.out.expr, native),
//		}
//	default:
//		panic(fmt.Sprintf("unexpected type %v, %v", reflect.TypeOf(v), v))
//	}
//}
//
//func (e *Env) strToType(s string) (out Typ) {
//	if t := e.Get(s); t != nil {
//		out = t
//	}
//	if out == nil {
//		panic(fmt.Sprintf("not found %s", s))
//	}
//	return
//}
