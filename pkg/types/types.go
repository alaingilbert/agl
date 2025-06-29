package types

import (
	"agl/pkg/utils"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

type Type interface {
	GoStr() string
	String() string
	StringFull() string
}

type BaseType struct {
}

type VoidType struct{}

func (v VoidType) GoStr() string      { return "AglVoid" }
func (v VoidType) String() string     { return "void" }
func (v VoidType) StringFull() string { return v.String() }

type StarType struct {
	X Type
}

func (s StarType) GoStr() string {
	return fmt.Sprintf("*%s", s.X.GoStr())
}
func (s StarType) String() string {
	if s.X == nil {
		return "*UNKNOWN"
	}
	return fmt.Sprintf("*%s", s.X.String())
}
func (s StarType) StringFull() string {
	if s.X == nil {
		return "*UNKNOWN"
	}
	return fmt.Sprintf("*%s", s.X.StringFull())
}

type MapType struct {
	K, V Type
}

func (m MapType) GoStr() string  { return "MapType" }
func (m MapType) String() string { return fmt.Sprintf("map[%s]%s", m.K, m.V) }
func (m MapType) StringFull() string {
	return fmt.Sprintf("map[%s]%s", m.K.StringFull(), m.V.StringFull())
}

type ChanType struct{ W Type }

func (m ChanType) GoStr() string      { return "ChanType" }
func (m ChanType) String() string     { return fmt.Sprintf("chan %s", m.W) }
func (m ChanType) StringFull() string { return fmt.Sprintf("chan %s", m.W.StringFull()) }

type BinaryType struct {
	X, Y Type
}

func (b BinaryType) GoStr() string      { return "BinaryType" }
func (b BinaryType) String() string     { return "BinaryType" }
func (b BinaryType) StringFull() string { return "BinaryType" }

type UnaryType struct {
	X Type
}

func (u UnaryType) GoStr() string      { return "UnaryType" }
func (u UnaryType) String() string     { return "UnaryType" }
func (u UnaryType) StringFull() string { return "UnaryType" }

type LabelType struct{ W Type }

func (l LabelType) GoStr() string      { return "LabelType" }
func (l LabelType) String() string     { return "LabelType" }
func (l LabelType) StringFull() string { return "LabelType" }

type IndexType struct {
	X     Type
	Index []FieldType
}

func (i IndexType) GoStr() string  { return i.X.GoStr() }
func (i IndexType) String() string { return i.X.String() }
func (i IndexType) StringFull() string {
	var tmp []string
	for _, e := range i.Index {
		tmp = append(tmp, e.Name)
	}
	return fmt.Sprintf("%s[%s]", i.X.StringFull(), strings.Join(tmp, ","))
}

type IndexListType struct {
	X       Type
	Indices []FieldType
}

func (i IndexListType) GoStr() string      { return i.X.GoStr() }
func (i IndexListType) String() string     { return i.X.String() }
func (i IndexListType) StringFull() string { return i.X.StringFull() }

type ResultType struct {
	W             Type
	Native        bool
	Bubble        bool
	ConvertToNone bool
	ToNoneType    Type
}

func (r ResultType) GoStr() string { return fmt.Sprintf("Result[%s]", r.W.GoStr()) }
func (r ResultType) StringFull() string {
	switch r.W.(type) {
	case ArrayType, StarType:
		return fmt.Sprintf("(%s)!", r.W.StringFull())
	default:
		return fmt.Sprintf("%s!", r.W.StringFull())
	}
}
func (r ResultType) String() string {
	switch r.W.(type) {
	case ArrayType, StarType:
		return fmt.Sprintf("(%s)!", r.W.String())
	default:
		return fmt.Sprintf("%s!", r.W.String())
	}
}

type OptionType struct {
	W      Type
	Native bool
	Bubble bool
}

func (o OptionType) GoStr() string { return fmt.Sprintf("Option[%s]", o.W.GoStr()) }
func (o OptionType) String() string {
	switch o.W.(type) {
	case ArrayType, StarType:
		return fmt.Sprintf("(%s)?", o.W.String())
	default:
		return fmt.Sprintf("(%s)?", o.W.String())
	}
}
func (o OptionType) StringFull() string {
	switch o.W.(type) {
	case ArrayType, StarType:
		return fmt.Sprintf("(%s)?", o.W.StringFull())
	default:
		return fmt.Sprintf("(%s)?", o.W.StringFull())
	}
}

type CustomType struct {
	Pkg  string
	Name string
	W    Type
}

func (c CustomType) GoStr() string {
	out := c.Name
	if c.Pkg != "" {
		out = c.Pkg + "." + out
	}
	return out
}
func (c CustomType) String() string     { return c.GoStr() }
func (c CustomType) StringFull() string { return c.GoStr() }

type TypeType struct{ W Type }

func (t TypeType) GoStr() string      { return t.W.GoStr() }
func (t TypeType) String() string     { return t.W.String() }
func (t TypeType) StringFull() string { return t.String() }

type StringType struct{ W Type }

func (s StringType) GoStr() string      { return "string" }
func (s StringType) String() string     { return "string" }
func (s StringType) StringFull() string { return s.String() }

type CharType struct{ W Type }

func (s CharType) GoStr() string      { return "char" }
func (s CharType) String() string     { return "char" }
func (s CharType) StringFull() string { return "char" }

type BoolValue struct{ V bool }

func (b BoolValue) GoStr() string {
	if b.V {
		return "true"
	} else {
		return "false"
	}
}
func (b BoolValue) String() string     { return b.GoStr() }
func (b BoolValue) StringFull() string { return b.GoStr() }

type BoolType struct{}

func (b BoolType) GoStr() string      { return "bool" }
func (b BoolType) String() string     { return "bool" }
func (b BoolType) StringFull() string { return b.String() }

type RuneType struct{ W Type }

func (r RuneType) GoStr() string      { return "RuneType" }
func (r RuneType) String() string     { return "RuneType" }
func (r RuneType) StringFull() string { return "RuneType" }

type ByteType struct{ W Type }

func (b ByteType) GoStr() string      { return "byte" }
func (b ByteType) String() string     { return "byte" }
func (b ByteType) StringFull() string { return b.String() }

type MakeType struct{ W Type }

func (o MakeType) GoStr() string      { return "make" }
func (o MakeType) String() string     { return "make" }
func (o MakeType) StringFull() string { return o.String() }

type LenType struct{ W Type }

func (l LenType) GoStr() string      { return "len" }
func (l LenType) String() string     { return "len" }
func (l LenType) StringFull() string { return l.String() }

type UnderscoreType struct{ W Type }

func (u UnderscoreType) GoStr() string      { return "_" }
func (u UnderscoreType) String() string     { return "_" }
func (u UnderscoreType) StringFull() string { return u.String() }

type NoneType struct{ W Type }

func (n NoneType) GoStr() string      { return "NoneType" }
func (n NoneType) String() string     { return fmt.Sprintf("None[%s]", n.W.String()) }
func (n NoneType) StringFull() string { return fmt.Sprintf("None[%s]", n.W.StringFull()) }

type UntypedNoneType struct{}

func (n UntypedNoneType) GoStr() string      { return "UntypedNoneType" }
func (n UntypedNoneType) String() string     { return "UntypedNoneType" }
func (n UntypedNoneType) StringFull() string { return "UntypedNoneType" }

type SomeType struct{ W Type }

func (s SomeType) GoStr() string      { return "SomeType" }
func (s SomeType) String() string     { return fmt.Sprintf("Some[%s]", s.W.String()) }
func (s SomeType) StringFull() string { return fmt.Sprintf("Some[%s]", s.W.StringFull()) }

type OkType struct{ W Type }

func (o OkType) GoStr() string      { return "OkType" }
func (o OkType) String() string     { return fmt.Sprintf("Ok[%s]", o.W.String()) }
func (o OkType) StringFull() string { return fmt.Sprintf("Ok[%s]", o.W.StringFull()) }

type ErrType struct {
	W Type
	T Type
}

func (e ErrType) GoStr() string      { return "ErrType" }
func (e ErrType) String() string     { return fmt.Sprintf("Err[%s]", e.T.String()) }
func (e ErrType) StringFull() string { return fmt.Sprintf("Err[%s]", e.T.StringFull()) }

type PackageType struct{ Name, Path string }

func (p PackageType) GoStr() string      { return p.Name }
func (p PackageType) String() string     { return "package " + p.Name }
func (p PackageType) StringFull() string { return "package " + p.Name }

type AnyType struct{}

func (a AnyType) GoStr() string      { return "any" }
func (a AnyType) String() string     { return "any" }
func (a AnyType) StringFull() string { return "any" }

type NilType struct{}

func (n NilType) GoStr() string      { return "nil" }
func (n NilType) String() string     { return "nil" }
func (n NilType) StringFull() string { return n.String() }

type SetType struct{ Elt Type }

func (s SetType) GoStr() string      { return fmt.Sprintf("Set[%s]", s.Elt.GoStr()) }
func (s SetType) String() string     { return fmt.Sprintf("Set[%s]", s.Elt.String()) }
func (s SetType) StringFull() string { return fmt.Sprintf("Set[%s]", s.Elt.StringFull()) }

type ArrayType struct{ Elt Type }

func (a ArrayType) GoStr() string      { return fmt.Sprintf("[]%s", a.Elt.GoStr()) }
func (a ArrayType) String() string     { return fmt.Sprintf("[]%s", a.Elt.String()) }
func (a ArrayType) StringFull() string { return fmt.Sprintf("[]%s", a.Elt.StringFull()) }

type EllipsisType struct{ Elt Type }

func (e EllipsisType) GoStr() string      { return fmt.Sprintf("...%s", e.Elt.GoStr()) }
func (e EllipsisType) String() string     { return fmt.Sprintf("...%s", e.Elt.String()) }
func (e EllipsisType) StringFull() string { return fmt.Sprintf("...%s", e.Elt.StringFull()) }

type FieldType struct {
	Name string
	Typ  Type
}

func (f FieldType) GoStr() string      { return "FieldType" }
func (f FieldType) String() string     { return "FieldType" }
func (f FieldType) StringFull() string { return "FieldType" }

type TypeAssertType struct {
	X    Type
	Type Type
}

func (t TypeAssertType) GoStr() string      { return "TypeAssertType" }
func (t TypeAssertType) String() string     { return "TypeAssertType" }
func (t TypeAssertType) StringFull() string { return "TypeAssertType" }

type StructType struct {
	Name   string
	Pkg    string
	Fields []FieldType
}

func (s StructType) GetFieldName(field string) (out string) {
	out = s.Name + "." + field
	if s.Pkg != "" {
		out = s.Pkg + "." + out
	}
	return out
}

func (s StructType) GoStr() string {
	out := s.Name
	if s.Pkg != "" {
		out = s.Pkg + "." + out
	}
	return out
}

func (s StructType) String() string { return s.Name }

func (s StructType) StringFull() string {
	out := s.Name
	if s.Pkg != "" {
		out = s.Pkg + "." + out
	}
	return out
}

type InterfaceType struct {
	Name string
	Pkg  string
}

func (i InterfaceType) GoStr() string      { return i.String() }
func (i InterfaceType) StringFull() string { return i.String() }
func (i InterfaceType) String() string {
	out := i.Name
	if i.Pkg != "" {
		out = i.Pkg + "." + out
	}
	return out
}

type EnumType struct {
	Name   string
	Fields []EnumFieldType
	SubTyp string
}

func (e EnumType) GoStr() string      { return e.Name }
func (e EnumType) String() string     { return e.Name }
func (e EnumType) StringFull() string { return e.Name }

type EnumFieldType struct {
	Name string
	Elts []Type
}

func (e EnumFieldType) GoStr() string { return "EnumFieldType" }

func (e EnumFieldType) String() string { return "EnumFieldType" }

type GenericType struct {
	W      Type
	Name   string
	IsType bool
}

func (g GenericType) TypeParamGoStr() string {
	if g.W == nil {
		return fmt.Sprintf("%s UNKNOWN", g.Name)
	}
	return fmt.Sprintf("%s %s", g.Name, g.W.String())
}
func (g GenericType) GoStr() string      { return fmt.Sprintf("%s", g.Name) }
func (g GenericType) String() string     { return fmt.Sprintf("%s", g.Name) }
func (g GenericType) StringFull() string { return g.String() }

type BubbleOptionType struct {
	Elt    Type
	Bubble bool
}

func (b BubbleOptionType) GoStr() string { return "BubbleOptionType" }

func (b BubbleOptionType) String() string { return "BubbleOptionType" }

type BubbleResultType struct {
	Elt    Type
	Bubble bool
}

func (r BubbleResultType) GoStr() string { return "BubbleResultType" }

func (r BubbleResultType) String() string { return "BubbleResultType" }

type UintptrType struct{}

func (u UintptrType) GoStr() string      { return "uintptr" }
func (u UintptrType) String() string     { return "uintptr" }
func (u UintptrType) StringFull() string { return u.String() }

type Complex128Type struct{}

func (c Complex128Type) GoStr() string      { return "complex128" }
func (c Complex128Type) String() string     { return "complex128" }
func (c Complex128Type) StringFull() string { return c.String() }

type Complex64Type struct{}

func (c Complex64Type) GoStr() string      { return "complex64" }
func (c Complex64Type) String() string     { return "complex64" }
func (c Complex64Type) StringFull() string { return c.String() }

type I64Type struct{}

func (i I64Type) GoStr() string      { return "int64" }
func (i I64Type) String() string     { return "i64" }
func (i I64Type) StringFull() string { return i.String() }

type U8Type struct{}

func (u U8Type) GoStr() string      { return "uint8" }
func (u U8Type) String() string     { return "u8" }
func (u U8Type) StringFull() string { return u.String() }

type U16Type struct{}

func (u U16Type) GoStr() string      { return "uint16" }
func (u U16Type) String() string     { return "u16" }
func (u U16Type) StringFull() string { return u.String() }

type U32Type struct{}

func (u U32Type) GoStr() string      { return "uint32" }
func (u U32Type) String() string     { return "u32" }
func (u U32Type) StringFull() string { return u.String() }

type U64Type struct{}

func (u U64Type) GoStr() string      { return "uint64" }
func (u U64Type) String() string     { return "u64" }
func (u U64Type) StringFull() string { return u.String() }

type I8Type struct{}

func (i I8Type) GoStr() string      { return "i8" }
func (i I8Type) String() string     { return "int8" }
func (i I8Type) StringFull() string { return i.String() }

type I16Type struct{}

func (i I16Type) GoStr() string      { return "int16" }
func (i I16Type) String() string     { return "i16" }
func (i I16Type) StringFull() string { return i.String() }

type I32Type struct{}

func (i I32Type) GoStr() string      { return "int32" }
func (i I32Type) String() string     { return "i32" }
func (i I32Type) StringFull() string { return i.String() }

type UntypedNumType struct{}

func (i UntypedNumType) GoStr() string      { return "int" }
func (i UntypedNumType) String() string     { return "UntypedNumType" }
func (i UntypedNumType) StringFull() string { return i.String() }

type UntypedStringType struct{}

func (i UntypedStringType) GoStr() string      { return "string" }
func (i UntypedStringType) String() string     { return "UntypedStringType" }
func (i UntypedStringType) StringFull() string { return i.String() }

type IntType struct{}

func (i IntType) GoStr() string      { return "int" }
func (i IntType) String() string     { return "int" }
func (i IntType) StringFull() string { return i.String() }

type UintType struct{}

func (u UintType) GoStr() string      { return "uint" }
func (u UintType) String() string     { return "uint" }
func (u UintType) StringFull() string { return u.String() }

type F32Type struct{}

func (f F32Type) GoStr() string      { return "float32" }
func (f F32Type) String() string     { return "f32" }
func (f F32Type) StringFull() string { return f.String() }

type F64Type struct{}

func (f F64Type) GoStr() string      { return "float64" }
func (f F64Type) String() string     { return "f64" }
func (f F64Type) StringFull() string { return f.String() }

type TupleType struct {
	Elts []Type
}

func (t TupleType) GoStr1() string {
	name := "AglTupleStruct_"
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.GoStr())
	}
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := strings.Join(tmp, "_")
	tmpName = r.Replace(tmpName)
	return name + tmpName
}

func (t TupleType) GoStr() string {
	name := "AglTupleStruct_"
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.GoStr())
	}
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := strings.Join(tmp, "_")
	tmpName = r.Replace(tmpName)
	var typeParams []string
	for _, el := range t.Elts {
		if v, ok := el.(GenericType); ok {
			typeParams = append(typeParams, v.Name)
		}
	}
	if len(typeParams) > 0 {
		tmpName += "[" + strings.Join(typeParams, ", ") + "]"
	}
	return name + tmpName
}

func (t TupleType) GoStr2() string {
	name := "AglTupleStruct_"
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.GoStr())
	}
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := strings.Join(tmp, "_")
	tmpName = r.Replace(tmpName)
	var typeParams []string
	for _, el := range t.Elts {
		if v, ok := el.(GenericType); ok {
			typeParams = append(typeParams, fmt.Sprintf("%s %s", v.Name, v.W.GoStr()))
		}
	}
	if len(typeParams) > 0 {
		tmpName += "[" + strings.Join(typeParams, ", ") + "]"
	}
	return name + tmpName
}

func (t TupleType) String() string {
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(tmp, ", "))
}
func (t TupleType) StringFull() string {
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.StringFull())
	}
	return fmt.Sprintf("(%s)", strings.Join(tmp, ", "))
}

type FuncType struct {
	Name       string
	Recv       []Type
	TypeParams []Type
	Params     []Type
	Return     Type
	IsNative   bool
}

func (f FuncType) GetParam(i int) Type {
	if len(f.Params) >= i {
		param := f.Params[i]
		ptMap := make(map[GenericType]bool)
		if other, ok := param.(FuncType); ok {
			for _, op := range other.Params {
				if opG, ok := op.(GenericType); ok {
					ptMap[opG] = true
				}
			}
			if other.Return != nil {
				if opG, ok := other.Return.(GenericType); ok {
					ptMap[opG] = true
				}
			}
			for k := range ptMap {
				other.TypeParams = append(other.TypeParams, k)
			}
			param = other
		}
		return param
	}
	return nil
}

func (f FuncType) T(name string, typ Type) FuncType {
	return f.ReplaceGenericParameter(name, typ)
}

func (f FuncType) ReplaceGenericParameter(name string, typ Type) FuncType {
	ff := f
	newParams := make([]Type, 0)
	newTypeParams := make([]Type, 0)
	for _, p := range ff.TypeParams {
		newTypeParams = append(newTypeParams, p)
	}
	for _, p := range ff.Params {
		newParams = append(newParams, ReplGen(p, name, typ))
	}
	if ff.Return != nil {
		ff.Return = ReplGen(ff.Return, name, typ)
	}
	if v, ok := ff.Return.(GenericType); ok {
		if v.Name == name {
			ff.Return = typ
		}
	} else if v, ok := ff.Return.(FuncType); ok {
		ff.Return = v.ReplaceGenericParameter(name, typ)
	}
	for i, p := range ff.Params {
		if v, ok := p.(GenericType); ok {
			if v.Name == name {
				newParams[i] = typ
			}
		} else if v, ok := p.(FuncType); ok {
			newParams[i] = v.ReplaceGenericParameter(name, typ)
		}
	}
	for i, p := range f.TypeParams {
		if v, ok := p.(GenericType); ok {
			if v.Name == name {
				newTypeParams = slices.Delete(newTypeParams, i, i+1)
			}
		}
	}

	ff.Params = newParams
	ff.TypeParams = newTypeParams
	return ff
}

func ReplGenM(t Type, m map[string]Type) (out Type) {
	for k, v := range m {
		t = ReplGen(t, k, v)
	}
	return t
}

func ReplGen2(t Type, currTyp, newTyp Type) (out Type) {
	switch currT := currTyp.(type) {
	case ArrayType:
		return ReplGen2(t, currT.Elt, newTyp.(ArrayType).Elt)
	case GenericType:
		t = t.(FuncType).ReplaceGenericParameter(currT.Name, newTyp)
		return t
	default:
		return t
		panic(fmt.Sprintf("%v", reflect.TypeOf(currT)))
	}
}

func ReplGen(t Type, name string, newTyp Type) (out Type) {
	switch t1 := t.(type) {
	case ArrayType:
		t1.Elt = ReplGen(t1.Elt, name, newTyp)
		return t1
	case AnyType:
		return t
	case GenericType:
		if t1.Name == name {
			return newTyp
		}
		return t
	case TypeType:
		return t
	case FuncType:
		var params []Type
		for _, p := range t1.Params {
			p = ReplGen(p, name, newTyp)
			params = append(params, p)
		}
		return FuncType{
			Name:       t1.Name,
			Recv:       t1.Recv,
			TypeParams: t1.TypeParams,
			Params:     params,
			Return:     ReplGen(t1.Return, name, newTyp),
			IsNative:   t1.IsNative,
		}
	case OptionType:
		return OptionType{W: ReplGen(t1.W, name, newTyp)}
	case ResultType:
		return ResultType{W: ReplGen(t1.W, name, newTyp)}
	case EllipsisType:
		return ReplGen(t1.Elt, name, newTyp)
	case I8Type, I16Type, I32Type, I64Type, U8Type, U16Type, U32Type, U64Type, UintType, IntType:
		return t
	case StructType: // TODO
		return t
	case TupleType:
		var params []Type
		for _, p := range t1.Elts {
			p = ReplGen(p, name, newTyp)
			params = append(params, p)
		}
		return TupleType{Elts: params}
	default:
		return t
		panic(fmt.Sprintf("%v", reflect.TypeOf(t)))
	}
}

func FindGen(a, b Type) map[string]Type {
	m := make(map[string]Type)
	findGenHelper(m, a, b)
	return m
}

func findGenHelper(m map[string]Type, a, b Type) {
	switch t1 := a.(type) {
	case ArrayType:
		findGenHelper(m, t1.Elt, b.(ArrayType).Elt)
	case GenericType:
		m[t1.Name] = b
	case FuncType:
		for i, rawParam := range t1.Params {
			findGenHelper(m, rawParam, b.(FuncType).Params[i])
		}
		if t1.Return != nil {
			findGenHelper(m, t1.Return, b.(FuncType).Return)
		}
	case VoidType:
	case StringType:
	case IntType:
	case U8Type:
	case I64Type:
	case TypeType:
		findGenHelper(m, t1.W, b.(TypeType).W)
	default:
		panic(fmt.Sprintf("%v", reflect.TypeOf(a)))
	}
}

func (f FuncType) GoStr() string { return f.Name }

func (f FuncType) GoStr1() string { // TODO
	var recvStr, nameStr, resultStr, paramsStr, typeParamsStr string
	if f.Name != "" {
		nameStr = " " + f.Name
	}
	if f.Recv != nil {
		var tmp []string
		for _, recv := range f.Recv {
			tmp = append(tmp, recv.GoStr())
		}
		recvStr = strings.Join(tmp, ", ")
		if recvStr != "" {
			recvStr = " (" + recvStr + ")"
		}
	}
	if f.TypeParams != nil {
		var tmp []string
		for _, typeParam := range f.TypeParams {
			tmp = append(tmp, typeParam.(GenericType).TypeParamGoStr())
		}
		typeParamsStr = strings.Join(tmp, ", ")
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if f.Params != nil {
		var tmp1 []string
		for _, param := range f.Params {
			var val string
			if param == nil {
				val = "nil"
			} else {
				val = param.GoStr()
			}
			tmp1 = append(tmp1, val)
		}
		paramsStr = strings.Join(tmp1, ", ")
	}
	if result := f.Return; result != nil {
		if _, ok := result.(VoidType); !ok {
			if v, ok := result.(ResultType); ok && utils.TryCast[VoidType](v.W) {
				resultStr = " !"
			} else {
				val := result.GoStr()
				if val != "" {
					resultStr = " " + val
				}
			}
		}
	}
	return fmt.Sprintf("func%s%s%s(%s)%s", recvStr, nameStr, typeParamsStr, paramsStr, resultStr)
}

func (f FuncType) StringFull() string { return f.String() }

func (f FuncType) String() string {
	var recvStr, nameStr, resultStr, paramsStr, typeParamsStr string
	if f.Name != "" {
		nameStr = " " + f.Name
	}
	if f.Recv != nil {
		var tmp []string
		for _, recv := range f.Recv {
			tmp = append(tmp, recv.String())
		}
		recvStr = strings.Join(tmp, ", ")
		if recvStr != "" {
			recvStr = " (" + recvStr + ")"
		}
	}
	if f.TypeParams != nil {
		var tmp []string
		for _, typeParam := range f.TypeParams {
			tmp = append(tmp, typeParam.(GenericType).TypeParamGoStr())
		}
		typeParamsStr = strings.Join(tmp, ", ")
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if f.Params != nil {
		var tmp1 []string
		for _, param := range f.Params {
			var val string
			if param == nil {
				val = "nil"
			} else {
				val = param.StringFull()
			}
			tmp1 = append(tmp1, val)
		}
		paramsStr = strings.Join(tmp1, ", ")
	}
	if result := f.Return; result != nil {
		if _, ok := result.(VoidType); !ok {
			if v, ok := result.(ResultType); ok && utils.TryCast[VoidType](v.W) {
				resultStr = " !"
			} else {
				val := result.StringFull()
				if val != "" {
					resultStr = " " + val
				}
			}
		}
	}
	return fmt.Sprintf("func%s%s%s(%s)%s", recvStr, nameStr, typeParamsStr, paramsStr, resultStr)
}

type ShortFuncLitType struct {
	Return Type
}

func (f ShortFuncLitType) String() string { return "" }

func (f ShortFuncLitType) GoStr() string { return "" }
