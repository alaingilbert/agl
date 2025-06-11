package main

import (
	"fmt"
	"reflect"
	"sync/atomic"
)

type Env struct {
	structCounter atomic.Int64
	lookupTable   map[string]Typ // Store constants/variables/functions
}

func NewEnv() *Env {
	env := &Env{lookupTable: make(map[string]Typ)}
	env.Define("time", PackageType{})
	env.Define("fmt", PackageType{})
	env.Define("os", PackageType{})
	env.Define("f64", F64Type{})
	env.Define("f32", F32Type{})
	env.Define("i64", I64Type{})
	env.Define("i32", I32Type{})
	env.Define("i16", I16Type{})
	env.Define("i8", I8Type{})
	env.Define("int", IntType{})
	env.Define("u64", U64Type{})
	env.Define("u32", U32Type{})
	env.Define("u16", U16Type{})
	env.Define("u8", U8Type{})
	env.Define("uint", UintType{})
	env.Define("string", StringType{})
	env.Define("bool", BoolType{})
	env.Define("any", AnyType{})
	env.Define("byte", ByteType{})
	env.Define("cmp.Ordered", AnyType{})
	env.Define("fmt.Println", parseFuncTypeFromStringNative("fn(a ...any) int!", env))
	env.Define("time.Now", parseFuncTypeFromStringNative("fn() time.Time", env))
	env.Define("strconv.Atoi", parseFuncTypeFromStringNative("fn(string) int!", env))
	env.Define("strconv.Itoa", parseFuncTypeFromStringNative("fn(int) string", env))
	env.Define("strconv.ParseInt", parseFuncTypeFromStringNative("fn(s string, base int, bitSize int) i64!", env))
	env.Define("strconv.ParseUInt", parseFuncTypeFromStringNative("fn(s string, base int, bitSize int) u64!", env))
	env.Define("strconv.ParseFloat", parseFuncTypeFromStringNative("fn(s string, bitSize int) f64!", env))
	env.Define("os.ReadFile", parseFuncTypeFromStringNative("fn(name string) Result[[]byte]", env))
	env.Define("os.WriteFile", parseFuncTypeFromStringNative("fn(name string, data []byte, perm os.FileMode) !", env))
	env.Define("agl.Vec.filter", parseFuncTypeFromString("fn filter[T any](a []T, f fn(e T) bool) []T", env))
	env.Define("agl.Vec.map", parseFuncTypeFromString("fn map[T, R any](a []T, f fn(T) R) []R", env))
	env.Define("agl.Vec.reduce", parseFuncTypeFromString("fn reduce[T any, R cmp.Ordered](a []T, r R, f fn(a R, e T) R) R", env))
	env.Define("agl.Vec.find", parseFuncTypeFromString("fn find[T any](a []T, f fn(e T) bool) T?", env))
	env.Define("agl.Vec.sum", parseFuncTypeFromString("fn sum[T cmp.Ordered](a []T) T", env))
	env.Define("agl.Option.is_some", parseFuncTypeFromString("fn is_some() bool", env))
	env.Define("agl.Option.is_none", parseFuncTypeFromString("fn is_none() bool", env))
	env.Define("agl.Result.is_ok", parseFuncTypeFromString("fn is_ok() bool", env))
	env.Define("agl.Result.is_err", parseFuncTypeFromString("fn is_err() bool", env))
	return env
}

func (e *Env) Clone() *Env {
	env := &Env{lookupTable: make(map[string]Typ)}
	for k, v := range e.lookupTable {
		env.lookupTable[k] = v
	}
	return env
}

func (e *Env) Get(name string) Typ {
	return e.lookupTable[name]
}

func (e *Env) Define(name string, typ Typ) {
	assertf(e.Get(name) == nil, "duplicate declaration of %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) Assign(name string, typ Typ) {
	if name == "_" {
		return
	}
	assertf(e.Get(name) != nil, "undeclared %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) GetTypeNative(x Expr) Typ {
	return e.getType(x, true)
}

func (e *Env) GetType(x Expr) Typ {
	return e.getType(x, false)
}

func (e *Env) getType(x Expr, native bool) Typ {
	oType := x.GetType()
	if oType != nil {
		return oType
	}
	switch v := x.(type) {
	case *IdentExpr:
		return e.strToType(v.lit)
	case *OptionExpr:
		return OptionType{wrappedType: e.getType(v.x, native)}
	case *ResultExpr:
		return ResultType{wrappedType: e.getType(v.x, native), native: native}
	case *BubbleOptionExpr: // TODO
		return OptionType{wrappedType: e.getType(v.x, native)}
	case *BubbleResultExpr: // TODO
		return ResultType{wrappedType: e.getType(v.x, native), native: native}
	case *ArrayTypeExpr:
		return ArrayType{elt: e.getType(v.elt, native)}
	case *SelectorExpr:
		return nil
	case *TrueExpr:
		return BoolType{}
	case *StringExpr:
		return StringType{}
	case *NumberExpr:
		return UntypedNumType{}
	case *EllipsisExpr:
		return e.getType(v.x, native)
	case *AnonFnExpr:
		return nil
	case *VecExpr:
		return nil
	case *VoidExpr:
		return VoidType{}
	case *BinOpExpr:
		return nil
	case *TupleTypeExpr:
		var elements []Typ
		for _, el := range v.exprs {
			elements = append(elements, e.getType(el, native))
		}
		structName := fmt.Sprintf("%s%d", TupleStructPrefix, e.structCounter.Add(1))
		return TupleType{elts: elements, name: structName}
	case *FuncExpr:
		var params []Typ
		for _, field := range v.args.list {
			fieldT := e.getType(field.typeExpr, native)
			n := max(len(field.names), 1)
			for i := 0; i < n; i++ {
				params = append(params, fieldT)
			}
		}
		return FuncType{
			params: params,
			ret:    e.getType(v.out.expr, native),
		}
	default:
		panic(fmt.Sprintf("unexpected type %v, %v", reflect.TypeOf(v), v))
	}
}

func (e *Env) strToType(s string) (out Typ) {
	if t := e.Get(s); t != nil {
		out = t
	}
	if out == nil {
		panic(fmt.Sprintf("not found %s", s))
	}
	return
}
