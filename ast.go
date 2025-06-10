package main

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

type ast struct {
	packageStmt *packageStmt
	imports     []*importStmt
	funcs       []*FuncExpr
	structs     []*structStmt
	interfaces  []*InterfaceStmt
	enums       []*EnumStmt
}

type Node interface {
	Pos() Pos
}

type BaseNode struct {
	pos Pos
}

func (n BaseNode) Pos() Pos {
	panic("not implemented")
	//return Pos{}
}

type Expr interface {
	Node
	GetType() Typ
	SetType(Typ)
	SetTypeForce(Typ)
	AglStr() string
}

type BaseExpr struct {
	BaseNode
	typ Typ
}

func (b *BaseExpr) SetType(typ Typ) {
	b.setType(typ, false)
}

func (b *BaseExpr) SetTypeForce(typ Typ) {
	b.setType(typ, true)
}

func (b *BaseExpr) setType(typ Typ, force bool) {
	//printCallers(100)
	if !force && b.typ != nil {
		if cmpTypes(b.typ, typ) {
			return
		}
		if !TryCast[UntypedNumType](b.typ) {
			if v, ok := b.typ.(FuncType); ok {
				fmt.Printf("%v\n", v.GoStr())
			}
			if v, ok := typ.(FuncType); ok {
				fmt.Printf("%v\n", v.GoStr())
			}
			panic(fmt.Sprintf("set type twice: curr: %v, new: %v", b.typ, typ))
		}
	}
	b.typ = typ
}

func (b *BaseExpr) AglStr() string {
	return "<not implemented>"
}

func (b *BaseExpr) GetType() Typ {
	return b.typ
}

type Stmt interface {
	Node
}

type BaseStmt struct {
	BaseNode
}

type Typ interface {
	GoStr() string
}

type BaseTyp struct{}

func (BaseTyp) GoStr() string { panic("not implemented") }

type Field struct {
	BaseExpr
	names    []*IdentExpr
	typeExpr Expr
}

func (f Field) String() string {
	return fmt.Sprintf("Field(%v %v)", f.names, f.typeExpr)
}

type FieldList struct {
	list []*Field
}

type FuncArg struct {
	lit string
	typ string
}

type VoidType struct{ BaseTyp }

func (v VoidType) String() string { return "VoidType" }

func (v VoidType) GoStr() string { return "AglVoid" }

type IntType struct{ BaseTyp }

func (i IntType) GoStr() string { return "int" }

func (i IntType) String() string { return "int" }

type ByteType struct{ BaseTyp }

func (b ByteType) GoStr() string { return "byte" }

func (b ByteType) String() string { return "ByteType" }

type BoolType struct{ BaseTyp }

func (b BoolType) GoStr() string { return "bool" }

func (b BoolType) String() string { return "bool" }

type AnyType struct{ BaseTyp }

func (a AnyType) GoStr() string { return "any" }

func (a AnyType) String() string { return "any" }

type StringType struct{ BaseTyp }

func (s StringType) GoStr() string { return "string" }

func (s StringType) String() string { return "string" }

type UntypedNumType struct{ BaseTyp }

func (i UntypedNumType) GoStr() string { return "<untyped number>" }

func (i UntypedNumType) String() string { return "UntypedNumType" }

type I64Type struct{ BaseTyp }

func (i I64Type) GoStr() string { return "int64" }

func (i I64Type) String() string { return "i64" }

type I32Type struct{ BaseTyp }

type I16Type struct{ BaseTyp }

type I8Type struct{ BaseTyp }

type U64Type struct{ BaseTyp }

func (u U64Type) GoStr() string { return "uint64" }

func (u U64Type) String() string { return "u64" }

type U32Type struct{ BaseTyp }

func (u U32Type) GoStr() string { return "uint32" }

func (u U32Type) String() string { return "u32" }

type U16Type struct{ BaseTyp }

func (u U16Type) GoStr() string { return "uint16" }

func (u U16Type) String() string { return "u16" }

type U8Type struct{ BaseTyp }

func (u U8Type) GoStr() string { return "uint8" }

func (u U8Type) String() string { return "u8" }

type UintType struct{ BaseTyp }

func (u UintType) GoStr() string { return "uint" }

func (u UintType) String() string { return "uint" }

type PackageType struct{ BaseTyp }

func (p PackageType) String() string { return "PackageType" }

type F64Type struct{ BaseTyp }

type F32Type struct{ BaseTyp }

type ResultType struct {
	BaseTyp
	wrappedType Typ
	native      bool
}

func (r *ResultType) GoStr() string {
	return fmt.Sprintf("Result[%s]", r.wrappedType.GoStr())
}

func (r *ResultType) String() string {
	return fmt.Sprintf("ResultType(%s)", r.wrappedType)
}

type OptionType struct {
	BaseTyp
	wrappedType Typ
}

func (o OptionType) Unwrap() Typ { return o.wrappedType }

func (o OptionType) GoStr() string {
	return fmt.Sprintf("Option[%s]", o.wrappedType.GoStr())
}

func (o OptionType) String() string {
	return fmt.Sprintf("OptionType(%s)", o.wrappedType)
}

type TupleType struct {
	BaseTyp
	name string
	elts []Typ
}

func (t TupleType) GoStr() string {
	return t.name
}

func (t TupleType) String() string {
	var strs []string
	for _, e := range t.elts {
		strs = append(strs, e.GoStr())
	}
	return fmt.Sprintf("TupleType(%s)", strings.Join(strs, ", "))
}

type ArrayType struct {
	BaseTyp
	elt Typ
}

func (a ArrayType) GoStr() string {
	return "[]" + a.elt.GoStr()
}

func (a ArrayType) String() string {
	return fmt.Sprintf("ArrayTypeExpr(%s)", a.elt)
}

type InterfaceType struct {
	BaseTyp
	name string
}

func (e InterfaceType) String() string { return fmt.Sprintf("InterfaceType(%s)", e.name) }

func (e InterfaceType) GoStr() string { return e.name }

type EnumType struct {
	BaseTyp
	name   string
	fields []EnumFieldType
}

func (e EnumType) String() string { return fmt.Sprintf("EnumType(%s)", e.name) }

func (e EnumType) GoStr() string { return fmt.Sprintf("%s", e.name) }

type EnumFieldType struct {
	name string
	elts []string
}

type StructType struct {
	BaseTyp
	name   string
	fields []FieldType
}

func (s StructType) String() string { return fmt.Sprintf("StructType(%s)", s.name) }

func (s StructType) GoStr() string { return s.name }

type FieldType struct {
	name string
	typ  Typ
}

type FuncType struct {
	BaseTyp
	name       string
	typeParams []Typ
	params     []Typ
	ret        Typ
	variadic   bool
	isNative   bool
}

func (f FuncType) ReplaceGenericParameter(name string, typ Typ) FuncType {
	if v, ok := f.ret.(*GenericType); ok {
		if v.name == name {
			f.ret = typ
		}
	} else if v, ok := f.ret.(FuncType); ok {
		f.ret = v.ReplaceGenericParameter(name, typ)
	}
	for i, p := range f.params {
		if v, ok := p.(*GenericType); ok {
			if v.name == name {
				f.params[i] = typ
			}
		} else if v, ok := p.(FuncType); ok {
			f.params[i] = v.ReplaceGenericParameter(name, typ)
		}
	}
	return f
}

func (f FuncType) GoStr() string {
	return f.goStr(true)
}

func (f FuncType) InterfaceStr() string {
	return f.goStr(false)
}

func (f FuncType) goStr(includeFunc bool) string {
	var typeParamsStr string
	var typeParams []string
	for _, p := range f.typeParams {
		typeParams = append(typeParams, p.(*GenericType).TypeParamGoStr())
	}
	if len(typeParams) > 0 {
		typeParamsStr = fmt.Sprintf(" [%s]", strings.Join(typeParams, ", "))
	}
	var paramsStr []string
	for _, p := range f.params {
		paramsStr = append(paramsStr, p.GoStr())
	}
	var retStr string
	if f.ret != nil {
		retStr = " " + f.ret.GoStr()
	}
	name := f.name
	if name != "" && includeFunc {
		name = " " + name
	}

	funcStr := "func"
	if !includeFunc {
		funcStr = ""
	}

	return fmt.Sprintf("%s%s%s(%s)%s", funcStr, typeParamsStr, name, strings.Join(paramsStr, ", "), retStr)
}

func (f FuncType) String() string {
	return fmt.Sprintf("FuncType(...)")
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

type NumberExpr struct {
	BaseExpr
	lit string
}

func (n NumberExpr) Pos() Pos { return n.pos }

func (n NumberExpr) AglStr() string {
	return fmt.Sprintf("%s", n.lit)
}

func (n NumberExpr) String() string {
	return fmt.Sprintf("NumberExpr(%s)", n.lit)
}

type AnonFnExpr struct {
	BaseExpr
	lbrack Pos
	rbrack Pos
	stmts  []Stmt
}

func (a *AnonFnExpr) Pos() Pos {
	return a.lbrack
}

func (a *AnonFnExpr) String() string {
	return fmt.Sprintf("AnonFnExpr(...)")
}

type KeyValueExpr struct {
	BaseExpr
	key   Expr
	value Expr
}

func (k KeyValueExpr) String() string {
	return fmt.Sprintf("KeyValueExpr(%v, %v)", k.key, k.value)
}

type EllipsisExpr struct {
	BaseExpr
	x Expr
}

func (e *EllipsisExpr) String() string {
	return fmt.Sprintf("EllipsisExpr(%v)", e.x)
}

type IdentExpr struct {
	BaseExpr
	tok Tok
	lit string
}

func NewIdentExpr(tok Tok) *IdentExpr {
	return &IdentExpr{tok: tok, lit: tok.lit}
}

func (i IdentExpr) Pos() Pos {
	return i.tok.Pos
}

func (i IdentExpr) AglStr() string {
	return fmt.Sprintf("%s", i.lit)
}

func (i IdentExpr) String() string {
	return fmt.Sprintf("IdentExpr(%s)", i.lit)
}

func (i IdentExpr) GetType() Typ {
	return i.typ
}

type TupleTypeExpr struct {
	BaseExpr
	exprs []Expr
}

func (t TupleTypeExpr) String() string {
	return fmt.Sprintf("TupleTypeExpr(%v)", t.exprs)
}

type ArrayTypeExpr struct {
	BaseExpr
	elt Expr
}

func (a ArrayTypeExpr) String() string {
	return fmt.Sprintf("ArrayTypeExpr(%v)", a.elt)
}

type ParenExpr struct {
	BaseExpr
	subExpr Expr
}

type MemberExpr struct {
	BaseExpr
	expr Expr
	name string
}

type BinOpExpr struct {
	BaseExpr
	op  Tok
	lhs Expr
	rhs Expr
}

func (b BinOpExpr) Pos() Pos {
	return b.lhs.Pos()
}

func (b BinOpExpr) AglStr() string {
	return fmt.Sprintf("%v %v %v", b.lhs.AglStr(), b.op.lit, b.rhs.AglStr())
}

func (b BinOpExpr) String() string {
	return fmt.Sprintf("BinOpExpr(%v, %v, %v)", b.op.lit, b.lhs, b.rhs)
}

type MutExpr struct {
	BaseExpr
	x Expr
}

type StringExpr struct {
	BaseExpr
	lit string
}

func (s StringExpr) Pos() Pos { return s.pos }

func (s StringExpr) AglStr() string { return s.lit }

func (s StringExpr) String() string { return fmt.Sprintf("StringExpr(%s)", s.lit) }

type MakeExpr struct {
	BaseExpr
	exprs []Expr
}

func (m MakeExpr) String() string {
	return fmt.Sprintf("MakeExpr(%v)", m.exprs)
}

type NoneExpr struct {
	BaseExpr
}

func (n NoneExpr) AglStr() string {
	return "None"
}

func (n NoneExpr) String() string { return "NoneExpr" }

type OkExpr struct {
	BaseExpr
	expr Expr
}

func (o OkExpr) String() string { return fmt.Sprintf("OkExpr(%v)", o.expr) }

type ErrExpr struct {
	BaseExpr
	expr Expr
}

func (e ErrExpr) String() string { return fmt.Sprintf("ErrExpr(%v)", e.expr) }

type SomeExpr struct {
	BaseExpr
	expr Expr
}

func (s SomeExpr) String() string { return fmt.Sprintf("SomeExpr(%v)", s.expr) }

type VoidExpr struct {
	BaseExpr
}

func (v VoidExpr) String() string { return "VoidExpr" }

type FuncOut struct {
	expr Expr
}

type TupleExpr struct {
	BaseExpr
	exprs []Expr
}

func (t TupleExpr) String() string {
	return fmt.Sprintf("TupleExpr(%v)", t.exprs)
}

type VecExpr struct {
	BaseExpr
	typStr string
	exprs  []Expr
}

func (v VecExpr) Pos() Pos {
	return v.pos
}

func (v VecExpr) String() string {
	return fmt.Sprintf("VecExpr(%s)", v.typStr)
}

type TupleExpr1 struct {
	BaseExpr
	FuncArgs *FieldList
}

type packageStmt struct {
	lit string
}

type InterfaceStmt struct {
	BaseStmt
	lit  string
	elts []Expr
}

func (i InterfaceStmt) String() string { return fmt.Sprintf("InterfaceStmt(%s)", i.lit) }

type EnumStmt struct {
	BaseStmt
	pub    bool
	lit    string
	fields []*EnumField
}

func (e EnumStmt) String() string { return fmt.Sprintf("EnumStmt(%v)", e.lit) }

type EnumField struct {
	name *IdentExpr
	elts []Expr
}

type structStmt struct {
	BaseStmt
	pub    bool
	lit    string
	fields []*Field
}

type importStmt struct {
	lit string
}

type FuncExpr struct {
	BaseExpr
	name       string
	recv       *FieldList
	typeParams *FieldList
	args       *FieldList
	out        FuncOut
	stmts      []Stmt
	typ        Typ
}

func (f *FuncExpr) String() string {
	return fmt.Sprintf("FuncExpr(...)")
}

type IfLetStmt struct {
	BaseStmt
	lhs  Expr
	rhs  Expr
	body []Stmt
	Else Stmt
}

func (i IfLetStmt) String() string { return "IfLetStmt(...)" }

type IfStmt struct {
	BaseStmt
	cond Expr
	body []Stmt
	Else Stmt
}

func (i IfStmt) String() string { return "IfStmt(...)" }

func (i IfStmt) Pos() Pos { return i.cond.Pos() }

type BlockStmt struct {
	BaseStmt
	lbrace Pos
	stmts  []Stmt
}

func (b BlockStmt) Pos() Pos { return b.lbrace }

type ReturnStmt struct {
	BaseStmt
	expr Expr
}

type MatchStmt struct {
	BaseStmt
	expr Expr
}

type InlineCommentStmt struct {
	BaseStmt
	lit string
}

type AssertStmt struct {
	BaseStmt
	x   Expr
	msg Expr
}

func (a AssertStmt) Pos() Pos {
	return a.x.Pos()
}

func (a AssertStmt) String() string {
	return fmt.Sprintf("AssertStmt(%v, %v)", a.x, a.msg)
}

type ForRangeStmt struct {
	BaseStmt
	id1   Expr
	id2   Expr
	expr  Expr
	stmts []Stmt
}

type IncDecStmt struct {
	BaseStmt
	x   Expr
	tok Tok
}

type ExprStmt struct {
	BaseStmt
	x Expr
}

func (e ExprStmt) Pos() Pos {
	return e.x.Pos()
}

func (e ExprStmt) String() string {
	return fmt.Sprintf("ExprStmt(%v)", e.x)
}

type AssignStmt struct {
	BaseStmt
	lhs Expr
	rhs Expr
	tok Tok
}

func (a AssignStmt) String() string {
	return fmt.Sprintf("AssignStmt(%v %v)", a.lhs, a.tok)
}

type CompositeLitExpr struct {
	BaseExpr
	typ  Expr
	elts []Expr
}

func (c CompositeLitExpr) String() string {
	return fmt.Sprintf("CompositeLitExpr(%v)", c.typ)
}

type TypeAssertExpr struct {
	BaseExpr
	x   Expr
	typ Expr // asserted type
}

type SelectorExpr struct {
	BaseExpr
	x   Expr
	sel *IdentExpr
}

func (s SelectorExpr) Pos() Pos {
	return s.x.Pos()
}

func (s SelectorExpr) AglStr() string {
	return fmt.Sprintf("%s.%s", s.x.AglStr(), s.sel.AglStr())
}

func (s SelectorExpr) String() string {
	return fmt.Sprintf("SelectorExpr(%s, %s)", s.x, s.sel)
}

type BubbleOptionExpr struct {
	BaseExpr
	x Expr
}

func (o BubbleOptionExpr) String() string { return fmt.Sprintf("BubbleOptionExpr(%v)", o.x) }

type ChainExpr struct {
	BaseExpr
	x  Expr
	x2 Expr
}

type BubbleResultExpr struct {
	BaseExpr
	x Expr
}

func (b BubbleResultExpr) String() string {
	return fmt.Sprintf("BubbleResultExpr(%v)", b.x)
}

type TrueExpr struct {
	BaseExpr
}

func (t TrueExpr) Pos() Pos {
	return t.BaseExpr.pos
}

func (t TrueExpr) String() string {
	return "TrueExpr"
}

func (t TrueExpr) AglStr() string { return "true" }

type FalseExpr struct {
	BaseExpr
}

func (f FalseExpr) String() string {
	return "FalseExpr"
}

func (f FalseExpr) AglStr() string { return "false" }

type CallExpr struct {
	BaseExpr
	fun  Expr
	args []Expr
}

func (c CallExpr) Pos() Pos {
	return c.fun.Pos()
}

func (c CallExpr) String() string {
	return fmt.Sprintf("CallExpr(%s)", c.fun)
}

func (c CallExpr) AglStr() string {
	args := make([]string, len(c.args))
	for i, arg := range c.args {
		args[i] = arg.AglStr()
	}
	return fmt.Sprintf("%s(%s)", c.fun.AglStr(), strings.Join(args, ", "))
}
