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
	env.Define("cmpOrdered", AnyType{})
	env.Define("strconv.Atoi", parseFuncTypeFromString("fn(string) int!", env))
	env.Define("strconv.Itoa", parseFuncTypeFromString("fn(int) string", env))
	env.Define("strconv.ParseInt", parseFuncTypeFromString("fn(s string, base int, bitSize int) i64!", env))
	env.Define("strconv.ParseUInt", parseFuncTypeFromString("fn(s string, base int, bitSize int) u64!", env))
	env.Define("strconv.ParseFloat", parseFuncTypeFromString("fn(s string, bitSize int) f64!", env))
	env.Define("os.ReadFile", parseFuncTypeFromString("fn(name string) Result[[]byte]", env))
	env.Define("agl.Vec.filter", parseFuncTypeFromString("fn filter[T any](a []T, f fn(e T) bool) []T", env))
	env.Define("agl.Vec.map", parseFuncTypeFromString("fn map[T, R any](a []T, f fn(T) R) []R", env))
	env.Define("agl.Vec.reduce", parseFuncTypeFromString("fn reduce[T any, R cmpOrdered](a []T, r R, f fn(a R, e T) R) R", env))
	env.Define("agl.Vec.sum", parseFuncTypeFromString("fn sum[T cmpOrdered](a []T) T", env))
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
	assertf(e.Get(name) != nil, "undeclared %s", name)
	e.lookupTable[name] = typ
}

func (e *Env) GetType(x Expr) Typ {
	oType := x.GetType()
	if oType != nil {
		return oType
	}
	switch v := x.(type) {
	case *IdentExpr:
		return e.strToType(v.lit)
	case *BubbleOptionExpr:
		return OptionType{wrappedType: e.GetType(v.x)}
	case *BubbleResultExpr:
		return ResultType{wrappedType: e.GetType(v.x)}
	case *ArrayType:
		return ArrayTypeTyp{elt: e.GetType(v.elt)}
	case *TrueExpr:
		return BoolType{}
	case *StringExpr:
		return StringType{}
	case *NumberExpr:
		return UntypedNumType{}
	case *EllipsisExpr:
		return e.GetType(v.x)
	case *AnonFnExpr:
		return nil
	case *VecExpr:
		return nil
	case *VoidExpr:
		return VoidType{}
	case *BinOpExpr:
		return nil
	case *TupleType:
		var elements []Typ
		for _, el := range v.exprs {
			elements = append(elements, e.GetType(el))
		}
		structName := fmt.Sprintf("%s%d", TupleStructPrefix, e.structCounter.Add(1))
		return TupleTypeTyp{elts: elements, name: structName}
	case *funcType:
		var params []Typ
		for _, field := range v.params.list {
			fieldT := e.GetType(field.typeExpr)
			n := max(len(field.names), 1)
			for i := 0; i < n; i++ {
				params = append(params, fieldT)
			}
		}
		return &FuncType{
			params: params,
			ret:    e.GetType(v.result.expr),
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
