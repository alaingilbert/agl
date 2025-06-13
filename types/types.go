package types

import (
	"fmt"
	"slices"
	"strings"
)

type Type interface {
	GoStr() string
	String() string
}

type BaseType struct {
}

type VoidType struct{}

func (v VoidType) GoStr() string  { return "" }
func (v VoidType) String() string { return "" }

type ResultType struct{ W Type }

func (r ResultType) GoStr() string  { return fmt.Sprintf("Result[%s]", r.W.String()) }
func (r ResultType) String() string { return r.W.String() + "!" }

type OptionType struct{ W Type }

func (o OptionType) GoStr() string  { return fmt.Sprintf("Option[%s]", o.W.String()) }
func (o OptionType) String() string { return "OptionType" }

type StringType struct{ W Type }

func (s StringType) GoStr() string  { return "string" }
func (s StringType) String() string { return "string" }

type BoolType struct{ W Type }

func (b BoolType) GoStr() string  { return "bool" }
func (b BoolType) String() string { return "bool" }

type MakeType struct{ W Type }

func (o MakeType) GoStr() string  { return "make" }
func (o MakeType) String() string { return "make" }

type UnderscoreType struct{ W Type }

func (u UnderscoreType) GoStr() string  { return "_" }
func (u UnderscoreType) String() string { return "_" }

type NoneType struct{ W Type }

func (n NoneType) GoStr() string  { return "" }
func (n NoneType) String() string { return "" }

type SomeType struct{ W Type }

func (s SomeType) GoStr() string  { return "" }
func (s SomeType) String() string { return "" }

type OkType struct{ W Type }

func (o OkType) GoStr() string { return "" }

func (o OkType) String() string { return "" }

type ErrType struct{ W Type }

func (e ErrType) GoStr() string { return "" }

func (e ErrType) String() string { return "" }

type PackageType struct{ Name string }

func (p PackageType) GoStr() string { return p.Name }

func (p PackageType) String() string { return p.Name }

type AnyType struct{}

func (a AnyType) GoStr() string { return "any" }

func (a AnyType) String() string { return "any" }

type ArrayType struct{ Elt Type }

func (a ArrayType) GoStr() string { return fmt.Sprintf("[]%s", a.Elt.String()) }

func (a ArrayType) String() string { return fmt.Sprintf("[]%s", a.Elt.String()) }

type EllipsisType struct{ Elt Type }

func (e EllipsisType) GoStr() string { return "..." }

func (e EllipsisType) String() string { return "..." }

type GenericType struct {
	W    Type
	Name string
}

func (g GenericType) TypeParamGoStr() string { return fmt.Sprintf("%s %s", g.Name, g.W.String()) }

func (g GenericType) GoStr() string { return fmt.Sprintf("%s", g.Name) }

func (g GenericType) String() string { return fmt.Sprintf("%s", g.Name) }

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

type I64Type struct{}

func (i I64Type) GoStr() string { return "int64" }

func (i I64Type) String() string { return "i64" }

type U8Type struct{}

func (u U8Type) GoStr() string { return "uint8" }

func (u U8Type) String() string { return "u8" }

type I32Type struct{}

func (i I32Type) GoStr() string { return "int32" }

func (i I32Type) String() string { return "int32" }

type UntypedNumType struct{}

func (i UntypedNumType) GoStr() string { return "UntypedNumType" }

func (i UntypedNumType) String() string { return "UntypedNumType" }

type IntType struct{}

func (i IntType) GoStr() string { return "int" }

func (i IntType) String() string { return "int" }

type UintType struct{}

func (u UintType) GoStr() string { return "uint" }

func (u UintType) String() string { return "uint" }

type FuncType struct {
	Name       string
	TypeParams []Type
	Params     []Type
	Return     Type
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

func (f FuncType) ReplaceGenericParameter(name string, typ Type) FuncType {
	if v, ok := f.Return.(GenericType); ok {
		if v.Name == name {
			f.Return = typ
		}
	} else if v, ok := f.Return.(FuncType); ok {
		f.Return = v.ReplaceGenericParameter(name, typ)
	}
	for i, p := range f.Params {
		if v, ok := p.(GenericType); ok {
			if v.Name == name {
				f.Params[i] = typ
			}
		} else if v, ok := p.(FuncType); ok {
			f.Params[i] = v.ReplaceGenericParameter(name, typ)
		}
	}
	for i, p := range f.TypeParams {
		if v, ok := p.(GenericType); ok {
			if v.Name == name {
				f.TypeParams = slices.Delete(f.TypeParams, i, i+1)
			}
		}
	}
	return f
}

func (f FuncType) GoStr() string { return f.Name }

func (f FuncType) String() string {
	var nameStr, resultStr, paramsStr, typeParamsStr string
	if f.Name != "" {
		nameStr = " " + f.Name
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
			tmp1 = append(tmp1, param.String())
		}
		paramsStr = strings.Join(tmp1, ", ")
	}
	if result := f.Return; result != nil {
		if result.String() != "" {
			resultStr = " " + result.String()
		}
	}
	return fmt.Sprintf("func%s%s(%s)%s", nameStr, typeParamsStr, paramsStr, resultStr)
}

type ShortFuncLitType struct {
	Return Type
}

func (f ShortFuncLitType) String() string { return "" }

func (f ShortFuncLitType) GoStr() string { return "" }
