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
	GoStrType() string
	String() string
}

type BaseType struct {
}

type VoidType struct{}

func (v VoidType) GoStr() string     { return "AglVoid{}" }
func (v VoidType) GoStrType() string { return "AglVoid" }
func (v VoidType) String() string    { return "void" }

type StarType struct {
	X Type
}

func (s StarType) GoStr() string     { return fmt.Sprintf("*%s", s.X.GoStr()) }
func (s StarType) GoStrType() string { return s.GoStr() }
func (s StarType) String() string {
	if s.X == nil {
		return "*UNKNOWN"
	}
	return fmt.Sprintf("*%s", s.X.String())
}

type MapType struct {
	K, V Type
}

func (m MapType) GoStr() string { return fmt.Sprintf("map[%s]%s", m.K.GoStr(), m.V.GoStr()) }
func (m MapType) GoStrType() string {
	return fmt.Sprintf("map[%s]%s", m.K.GoStrType(), m.V.GoStrType())
}
func (m MapType) String() string { return fmt.Sprintf("map[%s]%s", m.K, m.V) }

type RangeType struct {
	Typ Type
}

func (r RangeType) GoStr() string     { return "RangeType" }
func (r RangeType) GoStrType() string { return "RangeType" }
func (r RangeType) String() string    { return fmt.Sprintf("Range[%s]", r.Typ.String()) }

type ChanType struct{ W Type }

func (m ChanType) GoStr() string     { return fmt.Sprintf("chan %s", m.W) }
func (m ChanType) GoStrType() string { return m.GoStr() }
func (m ChanType) String() string    { return fmt.Sprintf("chan %s", m.W) }

type BinaryType struct {
	X, Y Type
}

func (b BinaryType) GoStr() string     { return "BinaryType" }
func (b BinaryType) GoStrType() string { return "BinaryType" }
func (b BinaryType) String() string    { return "BinaryType" }

type UnaryType struct {
	X Type
}

func (u UnaryType) GoStr() string     { return "UnaryType" }
func (u UnaryType) GoStrType() string { return "UnaryType" }
func (u UnaryType) String() string    { return "UnaryType" }

type LabelType struct{ W Type }

func (l LabelType) GoStr() string     { return "LabelType" }
func (l LabelType) GoStrType() string { return "LabelType" }
func (l LabelType) String() string    { return "LabelType" }

type IndexType struct {
	X     Type
	Index []FieldType
}

func (i IndexType) GoStr() string     { return i.X.GoStr() }
func (i IndexType) GoStrType() string { return i.X.GoStr() }
func (i IndexType) String() string {
	var tmp []string
	for _, e := range i.Index {
		tmp = append(tmp, e.Name)
	}
	return fmt.Sprintf("%s[%s]", i.X.String(), strings.Join(tmp, ","))
}

type IndexListType struct {
	X       Type
	Indices []FieldType
}

func (i IndexListType) GoStr() string     { return i.X.GoStr() }
func (i IndexListType) GoStrType() string { return i.X.GoStr() }
func (i IndexListType) String() string    { return i.X.String() }

type ResultType struct {
	W             Type
	Native        bool
	KeepRaw       bool
	Bubble        bool
	ConvertToNone bool
	ToNoneType    Type
}

func (r ResultType) GoStr() string     { return fmt.Sprintf("Result[%s]", r.W.GoStrType()) }
func (r ResultType) GoStrType() string { return fmt.Sprintf("Result[%s]", r.W.GoStrType()) }
func (r ResultType) String() string {
	switch r.W.(type) {
	case ArrayType, StarType, StructType, InterfaceType, CustomType:
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

func (o OptionType) GoStr() string     { return fmt.Sprintf("Option[%s]", o.W.GoStrType()) }
func (o OptionType) GoStrType() string { return fmt.Sprintf("Option[%s]", o.W.GoStrType()) }
func (o OptionType) String() string {
	switch o.W.(type) {
	case ArrayType, StarType, StructType, InterfaceType, CustomType:
		return fmt.Sprintf("(%s)?", o.W.String())
	default:
		if o.W == nil {
			return "None"
		}
		return fmt.Sprintf("%s?", o.W.String())
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
func (c CustomType) GoStrType() string { return c.GoStr() }
func (c CustomType) String() string    { return c.GoStr() }

type LabelledType struct {
	Label string
	W     Type
}

func (l LabelledType) GoStr() string     { return l.W.GoStr() }
func (l LabelledType) GoStrType() string { return l.W.GoStrType() }
func (l LabelledType) String() string {
	return fmt.Sprintf("%s: %s", l.Label, l.W.String())
}

type TypeType struct{ W Type }

func (t TypeType) GoStr() string     { return t.W.GoStr() }
func (t TypeType) GoStrType() string { return t.GoStr() }
func (t TypeType) String() string    { return t.W.String() }

type StringType struct{ W Type }

func (s StringType) GoStr() string     { return "string" }
func (s StringType) GoStrType() string { return "string" }
func (s StringType) String() string    { return "string" }

type CharType struct{ W Type }

func (s CharType) GoStr() string     { return "char" }
func (s CharType) GoStrType() string { return "char" }
func (s CharType) String() string    { return "char" }

type BoolValue struct{ V bool }

func (b BoolValue) GoStr() string {
	if b.V {
		return "true"
	} else {
		return "false"
	}
}
func (b BoolValue) GoStrType() string { return b.GoStr() }
func (b BoolValue) String() string    { return b.GoStr() }

type BoolType struct{}

func (b BoolType) GoStr() string     { return "bool" }
func (b BoolType) GoStrType() string { return "bool" }
func (b BoolType) String() string    { return "bool" }

type RuneType struct{ W Type }

func (r RuneType) GoStr() string     { return "RuneType" }
func (r RuneType) GoStrType() string { return "RuneType" }
func (r RuneType) String() string    { return "RuneType" }

type ByteType struct{ W Type }

func (b ByteType) GoStr() string     { return "byte" }
func (b ByteType) GoStrType() string { return "byte" }
func (b ByteType) String() string    { return "byte" }

type MakeType struct{ W Type }

func (o MakeType) GoStr() string     { return "make" }
func (o MakeType) GoStrType() string { return "make" }
func (o MakeType) String() string    { return "make" }

type LenType struct{ W Type }

func (l LenType) GoStr() string     { return "len" }
func (l LenType) GoStrType() string { return "len" }
func (l LenType) String() string    { return "len" }

type UnderscoreType struct{ W Type }

func (u UnderscoreType) GoStr() string     { return "_" }
func (u UnderscoreType) GoStrType() string { return "_" }
func (u UnderscoreType) String() string    { return "_" }

type OkType struct{ W Type }

func (o OkType) GoStr() string     { return "OkType" }
func (o OkType) GoStrType() string { return "OkType" }
func (o OkType) String() string    { return fmt.Sprintf("Ok[%s]", o.W.String()) }

type ErrType struct {
	W Type
	T Type
}

func (e ErrType) GoStr() string     { return "ErrType" }
func (e ErrType) GoStrType() string { return "ErrType" }
func (e ErrType) String() string    { return fmt.Sprintf("Err[%s]", e.T.String()) }

type PackageType struct{ Name, Path string }

func (p PackageType) GoStr() string     { return p.Name }
func (p PackageType) GoStrType() string { return p.Name }
func (p PackageType) String() string    { return "package " + p.Name }

type AnyType struct{}

func (a AnyType) GoStr() string     { return "any" }
func (a AnyType) GoStrType() string { return "any" }
func (a AnyType) String() string    { return "any" }

type NilType struct{}

func (n NilType) GoStr() string     { return "nil" }
func (n NilType) GoStrType() string { return "nil" }
func (n NilType) String() string    { return "nil" }

type SetType struct{ K Type }

func (s SetType) GoStr() string     { return fmt.Sprintf("AglSet[%s]", s.K.GoStr()) }
func (s SetType) GoStrType() string { return fmt.Sprintf("AglSet[%s]", s.K.GoStr()) }
func (s SetType) String() string    { return fmt.Sprintf("set[%s]", s.K.String()) }

type ArrayType struct{ Elt Type }

func (a ArrayType) GoStr() string     { return fmt.Sprintf("[]%s", a.Elt.GoStr()) }
func (a ArrayType) GoStrType() string { return fmt.Sprintf("[]%s", a.Elt.GoStr()) }
func (a ArrayType) String() string    { return fmt.Sprintf("[]%s", a.Elt.String()) }

type EllipsisType struct{ Elt Type }

func (e EllipsisType) GoStr() string     { return fmt.Sprintf("...%s", e.Elt.GoStr()) }
func (e EllipsisType) GoStrType() string { return fmt.Sprintf("...%s", e.Elt.GoStr()) }
func (e EllipsisType) String() string    { return fmt.Sprintf("...%s", e.Elt.String()) }

type FieldType struct {
	Name string
	Typ  Type
}

func (f FieldType) GoStr() string     { return "FieldType" }
func (f FieldType) GoStrType() string { return "FieldType" }
func (f FieldType) String() string    { return "FieldType" }

type TypeAssertType struct {
	X    Type
	Type Type
}

func (t TypeAssertType) GoStr() string     { return "TypeAssertType" }
func (t TypeAssertType) GoStrType() string { return "TypeAssertType" }
func (t TypeAssertType) String() string    { return "TypeAssertType" }

type StructType struct {
	Name       string
	Pkg        string
	TypeParams []GenericType
	Fields     []FieldType
}

func (s StructType) GetFieldName(field string) (out string) {
	return s.String1() + "." + field
}

func (s StructType) GoStr() string {
	if s.String() == "" {
		return "struct{}"
	}
	out := s.String1()
	if len(s.TypeParams) > 0 {
		tmp := utils.MapJoin(s.TypeParams, func(t GenericType) string { return t.W.GoStrType() }, ", ")
		out += fmt.Sprintf("[%s]", tmp)
	}
	return out
}

func (s StructType) GoStrType() string {
	if s.String() == "" {
		return "struct{}"
	}
	out := s.String1()
	if len(s.TypeParams) > 0 {
		tmp := utils.MapJoin(s.TypeParams, func(t GenericType) string { return t.W.GoStrType() }, ", ")
		out += fmt.Sprintf("[%s]", tmp)
	}
	return out
}

func (s StructType) String() string {
	out := s.String1()
	if len(s.TypeParams) > 0 {
		tmp := utils.MapJoin(s.TypeParams, func(t GenericType) string { return t.W.String() }, ", ")
		out += fmt.Sprintf("[%s]", tmp)
	}
	return out
}

func (s StructType) String1() string {
	out := s.Name
	if s.Pkg != "" {
		out = s.Pkg + "." + out
	}
	return out
}

type InterfaceMethod struct {
	Name string
	Typ  Type
}

type InterfaceType struct {
	Name       string
	Pkg        string
	TypeParams []Type
	Methods    []InterfaceMethod
}

func (i InterfaceType) Concrete(typs []Type) InterfaceType {
	var newParams []Type
	for idx, p := range i.TypeParams {
		if _, ok := p.(GenericType); ok {
			newParams = append(newParams, typs[idx])
		}
	}
	i.TypeParams = newParams
	return i
}

func (i InterfaceType) GetMethodByName(name string) Type {
	for _, m := range i.Methods {
		if m.Name == name {
			tmp := m.Typ.(FuncType)
			tmp.Name = m.Name
			return tmp
		}
	}
	panic(fmt.Sprintf("%s", name))
	return nil
}

func (i InterfaceType) GoStr() string     { return i.String() }
func (i InterfaceType) GoStrType() string { return i.String() }
func (i InterfaceType) String() string {
	out := i.Name
	if i.Pkg != "" {
		out = i.Pkg + "." + out
	}
	if len(i.TypeParams) > 0 {
		var tmp []string
		for _, t := range i.TypeParams {
			tmp = append(tmp, t.String())
		}
		out += fmt.Sprintf("[%s]", strings.Join(tmp, ", "))
	}
	return out
}

type EnumType struct {
	Name   string
	Fields []EnumFieldType
	SubTyp string
}

func (e EnumType) GoStr() string     { return e.Name }
func (e EnumType) GoStrType() string { return e.Name }
func (e EnumType) String() string    { return fmt.Sprintf("%s", e.Name) }

type EnumFieldType struct {
	Name string
	Elts []Type
}

func (e EnumFieldType) GoStr() string     { return "EnumFieldType" }
func (e EnumFieldType) GoStrType() string { return "EnumFieldType" }
func (e EnumFieldType) String() string    { return "EnumFieldType" }

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
func (g GenericType) GoStr() string     { return fmt.Sprintf("%s", g.Name) }
func (g GenericType) GoStrType() string { return fmt.Sprintf("%s", g.Name) }
func (g GenericType) String() string    { return fmt.Sprintf("%s", g.Name) }

type BubbleOptionType struct {
	Elt    Type
	Bubble bool
}

func (b BubbleOptionType) GoStr() string     { return "BubbleOptionType" }
func (b BubbleOptionType) GoStrType() string { return "BubbleOptionType" }
func (b BubbleOptionType) String() string    { return "BubbleOptionType" }

type BubbleResultType struct {
	Elt    Type
	Bubble bool
}

func (r BubbleResultType) GoStr() string     { return "BubbleResultType" }
func (r BubbleResultType) GoStrType() string { return "BubbleResultType" }
func (r BubbleResultType) String() string    { return "BubbleResultType" }

type UintptrType struct{}

func (u UintptrType) GoStr() string     { return "uintptr" }
func (u UintptrType) GoStrType() string { return "uintptr" }
func (u UintptrType) String() string    { return "uintptr" }

type Complex128Type struct{}

func (c Complex128Type) GoStr() string     { return "complex128" }
func (c Complex128Type) GoStrType() string { return "complex128" }
func (c Complex128Type) String() string    { return "complex128" }

type Complex64Type struct{}

func (c Complex64Type) GoStr() string     { return "complex64" }
func (c Complex64Type) GoStrType() string { return "complex64" }
func (c Complex64Type) String() string    { return "complex64" }

type I64Type struct{}

func (i I64Type) GoStr() string     { return "int64" }
func (i I64Type) GoStrType() string { return "int64" }
func (i I64Type) String() string    { return "i64" }

type U8Type struct{}

func (u U8Type) GoStr() string     { return "uint8" }
func (u U8Type) GoStrType() string { return "uint8" }
func (u U8Type) String() string    { return "u8" }

type U16Type struct{}

func (u U16Type) GoStr() string     { return "uint16" }
func (u U16Type) GoStrType() string { return "uint16" }
func (u U16Type) String() string    { return "u16" }

type U32Type struct{}

func (u U32Type) GoStr() string     { return "uint32" }
func (u U32Type) GoStrType() string { return "uint32" }
func (u U32Type) String() string    { return "u32" }

type U64Type struct{}

func (u U64Type) GoStr() string     { return "uint64" }
func (u U64Type) GoStrType() string { return "uint64" }
func (u U64Type) String() string    { return "u64" }

type I8Type struct{}

func (i I8Type) GoStr() string     { return "i8" }
func (i I8Type) GoStrType() string { return "i8" }
func (i I8Type) String() string    { return "int8" }

type I16Type struct{}

func (i I16Type) GoStr() string     { return "int16" }
func (i I16Type) GoStrType() string { return "int16" }
func (i I16Type) String() string    { return "i16" }

type I32Type struct{}

func (i I32Type) GoStr() string     { return "int32" }
func (i I32Type) GoStrType() string { return "int32" }
func (i I32Type) String() string    { return "i32" }

type UntypedNumType struct{}

func (i UntypedNumType) GoStr() string     { return "int" }
func (i UntypedNumType) GoStrType() string { return "int" }
func (i UntypedNumType) String() string    { return "UntypedNumType" }

type UntypedStringType struct{}

func (i UntypedStringType) GoStr() string     { return "string" }
func (i UntypedStringType) GoStrType() string { return "string" }
func (i UntypedStringType) String() string    { return "UntypedStringType" }

type IntType struct{}

func (i IntType) GoStr() string     { return "int" }
func (i IntType) GoStrType() string { return "int" }
func (i IntType) String() string    { return "int" }

type UintType struct{}

func (u UintType) GoStr() string     { return "uint" }
func (u UintType) GoStrType() string { return "uint" }
func (u UintType) String() string    { return "uint" }

type F32Type struct{}

func (f F32Type) GoStr() string     { return "float32" }
func (f F32Type) GoStrType() string { return "float32" }
func (f F32Type) String() string    { return "f32" }

type F64Type struct{}

func (f F64Type) GoStr() string     { return "float64" }
func (f F64Type) GoStrType() string { return "float64" }
func (f F64Type) String() string    { return "f64" }

type MutType struct{ W Type }

func (m MutType) Unwrap() Type      { return m.W }
func (m MutType) GoStr() string     { return m.W.GoStr() }
func (m MutType) GoStrType() string { return m.W.GoStrType() }
func (m MutType) String() string    { return "mut " + m.W.String() }

func Unwrap(t Type) Type {
	if v, ok := t.(LabelledType); ok {
		t = v.W
	}
	if v, ok := t.(MutType); ok {
		t = v.Unwrap()
	}
	if starT, ok := t.(StarType); ok {
		t = starT.X
	}
	if v, ok := t.(TypeType); ok {
		t = v.W
	}
	return t
}

type TupleType struct {
	KeepRaw bool
	Elts    []Type
}

func (t TupleType) GoStr1() string {
	name := "AglTupleStruct_"
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := utils.MapJoin(t.Elts, func(t Type) string { return t.GoStr() }, "_")
	tmpName = r.Replace(tmpName)
	return name + tmpName
}

func (t TupleType) GoStr() string {
	name := "AglTupleStruct_"
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := utils.MapJoin(t.Elts, func(t Type) string { return t.GoStr() }, "_")
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
	r := strings.NewReplacer(
		"[", "_",
		"]", "_",
	)
	tmpName := utils.MapJoin(t.Elts, func(t Type) string { return t.GoStr() }, "_")
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

func (t TupleType) GoStrType() string { return t.GoStr() }

func (t TupleType) String() string {
	var tmp []string
	for _, el := range t.Elts {
		tmp = append(tmp, el.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(tmp, ", "))
}

type FuncType struct {
	Pub        bool
	Name       string
	Recv       []Type
	TypeParams []Type
	Params     []Type
	Return     Type
	IsNative   bool
}

func (f FuncType) IsGeneric() bool {
	return len(f.TypeParams) > 0
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

func (f FuncType) Concrete(typs []Type) FuncType {
	var newParams []Type
	for i, p := range f.TypeParams {
		if _, ok := p.(GenericType); ok {
			newParams = append(newParams, typs[i])
		}
	}
	f.TypeParams = newParams
	return f
}

func (f FuncType) RenameGenericParameter(name, newName string) FuncType {
	ff := f
	newParams := make([]Type, 0)
	newTypeParams := make([]Type, 0)
	for _, p := range ff.TypeParams {
		newTypeParams = append(newTypeParams, RenameGen(p, name, newName))
	}
	for _, p := range ff.Params {
		newParams = append(newParams, RenameGen(p, name, newName))
	}
	if ff.Return != nil {
		ff.Return = RenameGen(ff.Return, name, newName)
	}
	if v, ok := ff.Return.(GenericType); ok {
		if v.Name == name {
			ff.Name = newName
		}
	} else if v, ok := ff.Return.(FuncType); ok {
		ff.Return = v.RenameGenericParameter(name, newName)
	}
	for i, p := range ff.Params {
		if v, ok := p.(GenericType); ok {
			if v.Name == name {
				v.Name = newName
				newParams[i] = v
			}
		} else if v, ok := p.(FuncType); ok {
			newParams[i] = v.RenameGenericParameter(name, newName)
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

func (f FuncType) ReplaceGenericParameter(name string, typ Type) FuncType {
	ff := f
	newParams := make([]Type, 0)
	newTypeParams := make([]Type, 0)
	for _, p := range ff.TypeParams {
		newTypeParams = append(newTypeParams, ReplGen(p, name, typ))
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
	currTyp = Unwrap(currTyp)
	newTyp = Unwrap(newTyp)
	switch currT := currTyp.(type) {
	case ArrayType:
		return ReplGen2(t, currT.Elt, newTyp.(ArrayType).Elt)
	case GenericType:
		t = t.(FuncType).ReplaceGenericParameter(currT.Name, newTyp)
		return t
	case InterfaceType:
		if currT.Name == "Iterator" {
			switch v := newTyp.(type) {
			case ArrayType:
				newTyp = v.Elt
			case MapType:
				newTyp = v.K
			case SetType:
				newTyp = v.K
			default:
				panic("")
			}
			for _, p := range currT.TypeParams {
				t = t.(FuncType).ReplaceGenericParameter(p.String(), newTyp)
			}
			return t
		}
		return t
	default:
		return t
		panic(fmt.Sprintf("%v", reflect.TypeOf(currT)))
	}
}

func RenameGen(t Type, name, newName string) (out Type) {
	switch t1 := t.(type) {
	case StarType:
		t1.X = RenameGen(t1.X, name, newName)
		return t1
	case MutType:
		t1.W = RenameGen(t1.W, name, newName)
		return t1
	case ArrayType:
		t1.Elt = RenameGen(t1.Elt, name, newName)
		return t1
	case SetType:
		t1.K = RenameGen(t1.K, name, newName)
		return t1
	case MapType:
		t1.K = RenameGen(t1.K, name, newName)
		t1.V = RenameGen(t1.V, name, newName)
		return t1
	case AnyType:
		return t
	case GenericType:
		if t1.Name == name {
			t1.Name = newName
			return t1
		}
		t1.W = RenameGen(t1.W, name, newName)
		return t1
	case TypeType:
		return t
	case FuncType:
		var params []Type
		for _, p := range t1.Params {
			p = RenameGen(p, name, newName)
			params = append(params, p)
		}
		return FuncType{
			Name:       t1.Name,
			Recv:       t1.Recv,
			TypeParams: t1.TypeParams,
			Params:     params,
			Return:     RenameGen(t1.Return, name, newName),
			IsNative:   t1.IsNative,
		}
	case OptionType:
		return OptionType{W: RenameGen(t1.W, name, newName)}
	case ResultType:
		return ResultType{W: RenameGen(t1.W, name, newName)}
	case EllipsisType:
		return RenameGen(t1.Elt, name, newName)
	case I8Type, I16Type, I32Type, I64Type, U8Type, U16Type, U32Type, U64Type, UintType, IntType:
		return t
	case StructType:
		var typeParams []GenericType
		for _, p := range t1.TypeParams {
			if p.Name == name {
				p.Name = newName
			}
			typeParams = append(typeParams, p)
		}
		var fields []FieldType
		for _, f := range t1.Fields {
			fields = append(fields, FieldType{Name: f.Name, Typ: RenameGen(f.Typ, name, newName)})
		}
		return StructType{Pkg: t1.Pkg, Name: t1.Name, TypeParams: typeParams, Fields: fields}
	case InterfaceType:
		var params []Type
		for _, p := range t1.TypeParams {
			p = RenameGen(p, name, newName)
			params = append(params, p)
		}
		return InterfaceType{Name: t1.Name, Pkg: t1.Pkg, TypeParams: params}
	case TupleType:
		var params []Type
		for _, p := range t1.Elts {
			p = RenameGen(p, name, newName)
			params = append(params, p)
		}
		return TupleType{Elts: params}
	case ErrType:
		return RenameGen(t1.W, name, newName)
	default:
		return t
		panic(fmt.Sprintf("%v", reflect.TypeOf(t)))
	}
}

func ReplGen(t Type, name string, newTyp Type) (out Type) {
	switch t1 := t.(type) {
	case StarType:
		t1.X = ReplGen(t1.X, name, newTyp)
		return t1
	case MutType:
		t1.W = ReplGen(t1.W, name, newTyp)
		return t1
	case ArrayType:
		t1.Elt = ReplGen(t1.Elt, name, newTyp)
		return t1
	case SetType:
		t1.K = ReplGen(t1.K, name, newTyp)
		return t1
	case MapType:
		t1.K = ReplGen(t1.K, name, newTyp)
		t1.V = ReplGen(t1.V, name, newTyp)
		return t1
	case AnyType:
		return t
	case GenericType:
		if t1.Name == name {
			return newTyp
		}
		t1.W = ReplGen(t1.W, name, newTyp)
		return t1
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
	case StructType:
		var typeParams []GenericType
		for _, p := range t1.TypeParams {
			if p.Name == name {
				p.W = newTyp
			}
			typeParams = append(typeParams, p)
		}
		var fields []FieldType
		for _, f := range t1.Fields {
			fields = append(fields, FieldType{Name: f.Name, Typ: ReplGen(f.Typ, name, newTyp)})
		}
		return StructType{Pkg: t1.Pkg, Name: t1.Name, TypeParams: typeParams, Fields: fields}
	case InterfaceType:
		var params []Type
		for _, p := range t1.TypeParams {
			p = ReplGen(p, name, newTyp)
			params = append(params, p)
		}
		return InterfaceType{Name: t1.Name, Pkg: t1.Pkg, TypeParams: params}
	case TupleType:
		var params []Type
		for _, p := range t1.Elts {
			p = ReplGen(p, name, newTyp)
			params = append(params, p)
		}
		return TupleType{Elts: params}
	case ErrType:
		return ReplGen(t1.W, name, newTyp)
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
		b = Unwrap(b)
		findGenHelper(m, t1.Elt, b.(ArrayType).Elt)
	case TupleType:
		for i, elt := range t1.Elts {
			findGenHelper(m, elt, b.(TupleType).Elts[i])
		}
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
	case BoolType:
	case EllipsisType:
	case StarType:
		findGenHelper(m, t1.X, b.(StarType).X)
	case OptionType:
		findGenHelper(m, t1.W, b.(OptionType).W)
	case TypeType:
		findGenHelper(m, t1.W, b.(TypeType).W)
	default:
		panic(fmt.Sprintf("%v", reflect.TypeOf(a)))
	}
}

func (f FuncType) GoStr() string {
	var typeParamsStr string
	if f.TypeParams != nil {
		typeParamsStr = utils.MapJoin(f.TypeParams, func(t Type) string {
			switch v := t.(type) {
			case GenericType:
				return v.TypeParamGoStr()
			default:
				return t.GoStrType()
			}
		}, ", ")
		typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
	}
	return f.Name + typeParamsStr
}

func (f FuncType) GoStrType() string {
	var recvStr, nameStr, resultStr, paramsStr, typeParamsStr string
	if f.Name != "" {
		nameStr = " " + f.Name
	}
	if f.Recv != nil {
		recvStr = utils.MapJoin(f.Recv, func(t Type) string { return t.GoStr() }, ", ")
		recvStr = utils.WrapIf(recvStr, "(", ")")
	}
	if f.TypeParams != nil {
		typeParamsStr = utils.MapJoin(f.TypeParams, func(t Type) string { return t.(GenericType).TypeParamGoStr() }, ", ")
		typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
	}
	if f.Params != nil {
		paramsStr = utils.MapJoin(f.Params, func(p Type) string { return p.GoStr() }, ", ")
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

func (f FuncType) String() string {
	out := f.String1()
	var recvStr string
	if f.Recv != nil {
		recvStr = utils.MapJoin(f.Recv, func(t Type) string {
			if t == nil {
				return "NIL"
			}
			return t.String()
		}, ", ")
		recvStr = utils.WrapIf(recvStr, " (", ")")
	}
	return out[0:4] + recvStr + out[4:]
}

func (f FuncType) String1() string {
	var nameStr, resultStr, paramsStr, typeParamsStr string
	if f.Name != "" {
		nameStr = " " + f.Name
	}
	if f.TypeParams != nil {
		typeParamsStr = utils.MapJoin(f.TypeParams, func(t Type) string { return t.(GenericType).TypeParamGoStr() }, ", ")
		typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
	}
	if f.Params != nil {
		paramsStr = utils.MapJoin(f.Params, func(p Type) string {
			var val string
			if utils.TryCast[MutType](p) {
				val += "mut "
				p = p.(MutType).W
			}
			if p == nil {
				val += "nil"
			} else {
				val += p.String()
			}
			return val
		}, ", ")
	}
	if result := f.Return; result != nil {
		if _, ok := result.(VoidType); !ok {
			if v, ok := result.(ResultType); ok && utils.TryCast[VoidType](v.W) {
				resultStr = " !"
			} else {
				val := result.String()
				if val != "" {
					resultStr = " " + val
				}
			}
		}
	}
	return fmt.Sprintf("func%s%s(%s)%s", nameStr, typeParamsStr, paramsStr, resultStr)
}

type ShortFuncLitType struct {
	Return Type
}

func (f ShortFuncLitType) String() string    { return "" }
func (f ShortFuncLitType) GoStr() string     { return "" }
func (f ShortFuncLitType) GoStrType() string { return "" }
