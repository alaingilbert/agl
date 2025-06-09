package main

import (
	"fmt"
	"path"
	"runtime"
	"strings"
)

func parser(ts *TokenStream) *ast {
	s := &ast{}
	for {
		if ts.Peek().typ == EOF {
			break
		}
		if ts.Peek().typ == PACKAGE {
			ts.Next()
			s.packageStmt = &packageStmt{lit: ts.Next().lit}
		} else if ts.Peek().typ == IMPORT {
			ts.Next()
			s.imports = append(s.imports, &importStmt{lit: ts.Next().lit})
		} else if ts.Peek().typ == TYPE {
			s.structs = append(s.structs, parseStructTypeDecl(ts))
		} else if ts.Peek().typ == FN {
			s.funcs = append(s.funcs, parseFnStmt(ts, false))
		} else if ts.Peek().typ == INLINECOMMENT {
			ts.Next()
		}
	}
	return s
}

func parseFnSignatureStmt(ts *TokenStream) *funcStmt {
	ft := parseFnSignature(ts)
	return &funcStmt{
		recv:       ft.recv,
		name:       ft.name,
		typeParams: ft.typeParams,
		args:       ft.params,
		out:        ft.result,
	}
}

func parseFnSignature(ts *TokenStream) *funcType {
	out := &funcType{}
	assert(ts.Next().typ == FN)

	var recv *FieldList
	if ts.Peek().typ == LPAREN {
		assert(ts.Next().typ == LPAREN)
		if ts.Peek().typ == IDENT { // can be both fn/method

			params := parseParametersList(ts, RPAREN)
			fields1 := params
			assert(ts.Next().typ == RPAREN)

			out.params = fields1

			if ts.Peek().typ == IDENT { // fn/methods
				tok := ts.Next()
				if ts.Peek().typ == LBRACKET || ts.Peek().typ == LPAREN { // method

					// Optional function name
					if tok.typ == IDENT {
						out.name = tok.lit
						tok = ts.Next()
					} else if tok.typ != LPAREN && tok.typ != LBRACKET { // Allow use of reserved words as function names (map)
						out.name = tok.lit
						tok = ts.Next()
					}

					// Type parameters
					if tok.typ == LBRACKET {
						typeParams := parseParametersList(ts, RBRACKET)
						out.typeParams = typeParams
						assert(ts.Next().typ == RBRACKET)
						tok = ts.Next()
					}

					assert(tok.typ == LPAREN)
					params := parseParametersList(ts, RPAREN)
					out.params = params
					assert(ts.Next().typ == RPAREN)

					// Output
					if ts.Peek().typ != EOF && ts.Peek().typ != LBRACE {
						out.result.expr = parseType(ts)
					}

					out.recv = fields1
					return out
				}
				ts.Back()
			}

			// Output
			if ts.Peek().typ != LBRACE {
				out.result.expr = parseType(ts)
			}

			return out
		}
		ts.Back()
	}
	out.recv = recv

	// Optional function name
	tok := ts.Next()
	if tok.typ == IDENT {
		out.name = tok.lit
		tok = ts.Next()
	} else if tok.typ != LPAREN && tok.typ != LBRACKET { // Allow use of reserved words as function names (map)
		out.name = tok.lit
		tok = ts.Next()
	}

	// Type parameters
	if tok.typ == LBRACKET {
		typeParams := parseParametersList(ts, RBRACKET)
		out.typeParams = typeParams
		assert(ts.Next().typ == RBRACKET)
		tok = ts.Next()
	}

	assert(tok.typ == LPAREN)
	params := parseParametersList(ts, RPAREN)
	out.params = params
	assert(ts.Next().typ == RPAREN)

	// Output
	if ts.Peek().typ != EOF && ts.Peek().typ != LBRACE {
		out.result.expr = parseType(ts)
	}

	return out
}

func parseParametersList(ts *TokenStream, closing int) *FieldList {
	out := &FieldList{}
	var names []*IdentExpr
	for {
		if ts.Peek().typ == closing || ts.Peek().typ == EOF {
			return out
		}
		if ts.Peek().typ == IDENT {
			tok := ts.Next()
			if ts.Peek().typ == closing {
				out.list = append(out.list, &Field{typeExpr: NewIdentExpr(tok), names: names})
				return out
			} else if ts.Peek().typ == COMMA {
				names = append(names, NewIdentExpr(tok))
				ts.Next()
			} else {
				names = append(names, NewIdentExpr(tok))
				expr := parseType(ts)
				out.list = append(out.list, &Field{typeExpr: expr, names: names})
				names = make([]*IdentExpr, 0)
				if ts.Peek().typ == closing {
					return out
				}
			}
		} else if ts.Peek().typ == COMMA {
			ts.Next()
			continue
		} else {
			expr := parseType(ts)
			ts.Next()
			out.list = append(out.list, &Field{typeExpr: expr, names: names})
		}
	}
}

func parseTypes(ts *TokenStream, closing int) []Expr {
	out := make([]Expr, 0)
	for {
		if ts.Peek().typ == closing || ts.Peek().typ == EOF {
			return out
		}
		if ts.Peek().typ == COMMA {
			ts.Next()
			continue
		}
		e := parseType(ts)
		out = append(out, e)
	}
}

func parseType(ts *TokenStream) Expr {
	if ts.Peek().typ == IDENT {
		e := NewIdentExpr(ts.Next())
		if ts.Peek().typ == QUESTION {
			ts.Next()
			return &BubbleOptionExpr{x: e}
		} else if ts.Peek().typ == BANG {
			ts.Next()
			return &BubbleResultExpr{x: e}
		}
		return e
	} else if ts.Peek().typ == ELLIPSIS {
		ts.Next()
		return &EllipsisExpr{x: parseType(ts)}
	} else if ts.Peek().typ == LBRACKET {
		ts.Next()
		assert(ts.Next().typ == RBRACKET)
		return &ArrayType{elt: parseType(ts)}
	} else if ts.Peek().typ == FN {
		return parseFnSignature(ts)
	} else if ts.Peek().typ == OPTION {
		ts.Next()
		assert(ts.Next().typ == LBRACKET)
		e := &BubbleOptionExpr{x: parseType(ts)}
		assert(ts.Next().typ == RBRACKET)
		return e
	} else if ts.Peek().typ == RESULT {
		ts.Next()
		assert(ts.Next().typ == LBRACKET)
		e := &BubbleResultExpr{x: parseType(ts)}
		assert(ts.Next().typ == RBRACKET)
		return e
	} else if ts.Peek().typ == LPAREN {
		assert(ts.Next().typ == LPAREN)
		exprs := parseTypes(ts, RPAREN)
		assert(ts.Next().typ == RPAREN)
		if len(exprs) == 1 {
			return exprs[0]
		} else if len(exprs) > 1 {
			return &TupleType{exprs: exprs}
		} else {
			panic("not implemented")
		}
	}
	panic(fmt.Sprintf("unknown type %v", ts.Peek()))
}

func parseFnStmt(ts *TokenStream, isPub bool) *funcStmt {
	ft := parseFnSignature(ts)
	assert(ts.Next().typ == LBRACE)
	fs := &funcStmt{
		recv:       ft.recv,
		name:       ft.name,
		typeParams: ft.typeParams,
		args:       ft.params,
		out:        ft.result,
	}
	fs.stmts = parseStmts(ts)
	assert(ts.Next().typ == RBRACE)
	return fs
}

func parseStructTypeDecl(ts *TokenStream) (out *structStmt) {
	//if isPub {
	//	assert(tokens[*i].typ == PUB)
	//	*i++
	//}
	assert(ts.Next().typ == TYPE)
	lit := ts.Next().lit
	assert(ts.Next().typ == STRUCT)
	assert(ts.Next().typ == LBRACE)
	fields := make([]*Field, 0)
	for {
		if ts.Peek().typ == RBRACE {
			break
		}
		id := parseIdentExpr(ts)
		typ := parseType(ts)
		fields = append(fields, &Field{names: []*IdentExpr{id}, typeExpr: typ})
	}
	assert(ts.Next().typ == RBRACE)
	return &structStmt{lit: lit, pub: true, fields: fields}
}

func parseStmts(ts *TokenStream) (out []Stmt) {
	for {
		if ts.Peek().typ == RBRACE {
			break
		}
		out = append(out, parseStmt(ts))
	}
	return
}

func parseStmt(ts *TokenStream) (out Stmt) {
	var x []Expr
	x = parseExprs2(ts, 1)
	switch ts.Peek().typ {
	case WALRUS, ADD_ASSIGN, SUB_ASSIGN, MUL_ASSIGN, QUO_ASSIGN, REM_ASSIGN, AND_ASSIGN, OR_ASSIGN, XOR_ASSIGN, SHL_ASSIGN, SHR_ASSIGN, AND_NOT_ASSIGN, ASSIGN:
		tok := ts.Next()
		expr := parseExpr(ts, 1)
		return &AssignStmt{lhs: x[0], rhs: expr, tok: tok}
	case INC, DEC:
		tok := ts.Next()
		return &IncDecStmt{x: x[0], tok: tok}
	case IF:
		return parseIfStmt(ts)
	case RETURN:
		return parseReturnStmt(ts)
	case MATCH:
		return parseMatchStmt(ts)
	case INLINECOMMENT:
		return parseInlineComment(ts)
	case FOR:
		return parseForStmt(ts)
	case ASSERT:
		return parseAssertStmt(ts)
	default:
		if len(x) == 1 {
			return &ExprStmt{x: x[0]}
		}
		panic(fmt.Sprintf("unknown stmt type %v", ts.Peek()))
	}
}

func parseAssertStmt(ts *TokenStream) *AssertStmt {
	assert(ts.Next().typ == ASSERT)
	assert(ts.Next().typ == LPAREN)
	expr := parseExpr(ts, 1)
	var msg Expr
	if ts.Peek().typ == COMMA {
		ts.Next()
		msg = parseExpr(ts, 1)
	}
	assert(ts.Next().typ == RPAREN)
	return &AssertStmt{x: expr, msg: msg}
}

func parseForStmt(ts *TokenStream) *ForRangeStmt {
	assert(ts.Next().typ == FOR)
	if ts.Peek().typ == IDENT {
		id1 := parseIdentExpr(ts)
		if ts.Peek().typ == COMMA {
			ts.Next()
			if ts.Peek().typ == IDENT {
				id2 := parseIdentExpr(ts)
				if ts.Peek().typ == WALRUS {
					ts.Next()
					if ts.Peek().typ == RANGE {
						ts.Next()
						expr := parsePrimaryExpr(ts, 1)
						assert(ts.Next().typ == LBRACE)
						stmts := parseStmts(ts)
						assert(ts.Next().typ == RBRACE)
						return &ForRangeStmt{id1: id1, id2: id2, expr: expr, stmts: stmts}
					}
				}
			}
		}
	}
	panic("not implemented")
}

func parseReturnStmt(ts *TokenStream) *ReturnStmt {
	assert(ts.Next().typ == RETURN)
	expr := parseExpr(ts, 1)
	return &ReturnStmt{expr: expr}
}

func parseMatchStmt(ts *TokenStream) *MatchStmt {
	assert(ts.Next().typ == MATCH)
	expr := parseExpr(ts, 1)
	assert(ts.Next().typ == LBRACE)
	return &MatchStmt{expr: expr}
}

func parseInlineComment(ts *TokenStream) *InlineCommentStmt {
	stmt := &InlineCommentStmt{lit: ts.Peek().lit}
	ts.Next()
	return stmt
}

func parseIfStmt(ts *TokenStream) *IfStmt {
	ts.Next() // if
	expr := parseExpr(ts, 1)
	ts.Next() // {
	stmts := parseStmts(ts)
	ts.Next() // }
	var elseStmt Stmt
	if ts.Peek().typ == ELSE {
		ts.Next() // else
		if ts.Peek().typ == IF {
			elseStmt = parseIfStmt(ts)
		} else if ts.Peek().typ == LBRACE {
			elseStmt = parseBlockStmt(ts)
		}
	}
	return &IfStmt{cond: expr, body: stmts, Else: elseStmt}
}

func parseBlockStmt(ts *TokenStream) *BlockStmt {
	tok := ts.Next()
	assert(tok.typ == LBRACE)
	stmts := parseStmts(ts)
	assert(ts.Next().typ == RBRACE)
	return &BlockStmt{stmts: stmts, lbrace: tok.Pos}
}

func parseExprs(ts *TokenStream, exitTok, prec int) (out []Expr) {
	for {
		if ts.Peek().typ == exitTok {
			return
		} else if ts.Peek().typ == COMMA {
			ts.Next()
			continue
		}
		expr := parseExpr(ts, prec)
		out = append(out, expr)
	}
}

func parseExprs2(ts *TokenStream, prec int) (out []Expr) {
	expr := parseExpr(ts, prec)
	if expr != nil {
		out = append(out, expr)
	}
	for ts.Peek().typ == COMMA {
		ts.Next()
		expr := parseExpr(ts, prec)
		if expr != nil {
			out = append(out, expr)
		}
	}
	return
}

func parsePrimaryExpr(ts *TokenStream, prec int) Expr {
	switch ts.Peek().typ {
	case IDENT:
		ident := parseIdentExpr(ts)
		return ident
	case STRING:
		tok := ts.Next()
		return &StringExpr{lit: tok.lit, BaseExpr: BaseExpr{BaseNode: BaseNode{pos: tok.Pos}}}
	default:
		panic("")
	}
}

func parseExpr(ts *TokenStream, prec int) Expr {
	var x Expr
	x = parseExpr2(ts, prec)
loop:
	for {
		switch ts.Peek().typ {
		case EOF:
			break loop
		case QUESTION:
			ts.Next()
			x = &BubbleOptionExpr{x: x}
		case BANG:
			ts.Next()
			x = &BubbleResultExpr{x: x}
		case DOT:
			ts.Next()
			id := parseIdentExpr(ts)
			assert(ts.Next().typ == LPAREN)
			args := parseExprs(ts, RPAREN, prec)
			assert(ts.Next().typ == RPAREN)
			x = &CallExpr{fun: &SelectorExpr{x: x, sel: id}, args: args}
		case EQL, LOR, LAND, REM, ADD, NEQ, MINUS, MUL, QUO, LTE, LT, GTE, GT:
			x = parseBinOpExpr(ts, x, prec)
			break loop
		default:
			break loop
		}
	}
	return x
}

func parseExpr2(ts *TokenStream, prec int) Expr {
	token := ts.Peek()
	switch token.typ {
	case LBRACE:
		return parseAnonFn(ts)
	case LPAREN:
		assert(ts.Next().typ == LPAREN)
		expr1 := parseExpr(ts, prec)
		switch ts.Peek().typ {
		case RPAREN:
			assert(ts.Next().typ == RPAREN)
			return &ParenExpr{subExpr: expr1}
		case COMMA:
			assert(ts.Next().typ == COMMA)
			exprs := parseExprs(ts, RPAREN, prec)
			assert(ts.Next().typ == RPAREN)
			return &TupleExpr{exprs: append([]Expr{expr1}, exprs...)}
		default:
			panic("")
		}
	case LBRACKET:
		return parseVecExpr(ts, prec)
	case MUT:
		ts.Next()
		return &MutExpr{x: parseExpr(ts, prec)}
	case IDENT:
		ident := parseIdentExpr(ts)
		switch ts.Peek().typ {
		case DOT:
			assert(ts.Next().typ == DOT)
			ident2 := parseIdentExpr(ts)
			sel := &SelectorExpr{x: ident, sel: ident2}
			if ts.Peek().typ == LPAREN {
				return parseCallExpr(ts, sel, prec)
			}
			return sel
		case LBRACE:
			assert(ts.Next().typ == LBRACE)
			assert(ts.Next().typ == RBRACE)
			return &CompositeLitExpr{typ: ident}
		case LPAREN:
			return parseCallExpr(ts, ident, prec)
		default:
			return ident
		}
	case TRUE:
		return &TrueExpr{BaseExpr{BaseNode: BaseNode{pos: ts.Next().Pos}}}
	case FALSE:
		return &FalseExpr{BaseExpr{BaseNode: BaseNode{pos: ts.Next().Pos}}}
	case NUMBER:
		return parseNumberExpr(ts)
	case NONE:
		assert(ts.Next().typ == NONE)
		return &NoneExpr{}
	case OK:
		assert(ts.Next().typ == OK)
		assert(ts.Next().typ == LPAREN)
		expr2 := parseExpr(ts, prec)
		assert(ts.Next().typ == RPAREN)
		return &OkExpr{expr: expr2}
	case ERR:
		assert(ts.Next().typ == ERR)
		assert(ts.Next().typ == LPAREN)
		expr2 := parseExpr(ts, prec)
		assert(ts.Next().typ == RPAREN)
		return &ErrExpr{expr: expr2}
	case SOME:
		ts.Next()
		assert(ts.Next().typ == LPAREN)
		expr2 := parseExpr(ts, prec)
		assert(ts.Next().typ == RPAREN)
		return &SomeExpr{expr: expr2}
	case STRING:
		tok := ts.Next()
		return &StringExpr{lit: tok.lit, BaseExpr: BaseExpr{BaseNode: BaseNode{pos: tok.Pos}}}
	case MAKE:
		assert(ts.Next().typ == MAKE)
		assert(ts.Next().typ == LPAREN)
		exprs := parseExprs(ts, RPAREN, prec)
		assert(len(exprs) >= 1 && len(exprs) <= 3)
		assert(ts.Next().typ == RPAREN)
		expr := &MakeExpr{exprs: exprs}
		return expr
	default:
		return nil
		// panic(fmt.Sprintf("unknown expr %v", token))
	}
}

func precForToken(op Tok) int {
	switch op.typ {
	case LOR:
		return 1
	case LAND:
		return 2
	case EQL, NEQ:
		return 3
	case ADD, MINUS:
		return 4
	case REM, MUL, QUO:
		return 5
	default:
		return 0
	}
}

func parseUnaryExpr(ts *TokenStream, prec int) Expr {
	return parseExpr(ts, prec)
}

func parseBinOpExpr(ts *TokenStream, x Expr, prec int) Expr {
	if x == nil {
		x = parseUnaryExpr(ts, prec)
	}
	for {
		op := ts.Peek()
		opPrec := precForToken(op)
		if opPrec < prec {
			return x
		}
		ts.Next()
		y := parseBinOpExpr(ts, nil, opPrec+1)
		x = &BinOpExpr{lhs: x, rhs: y, op: op}
	}
}

func parseVecExpr(ts *TokenStream, prec int) *VecExpr {
	assert(ts.Next().typ == LBRACKET)
	assert(ts.Next().typ == RBRACKET)
	assert(ts.Peek().typ == IDENT)
	tok := ts.Next()
	expr := &VecExpr{typStr: tok.lit, BaseExpr: BaseExpr{BaseNode: BaseNode{pos: tok.Pos}}}
	if ts.Peek().typ == LBRACE {
		assert(ts.Next().typ == LBRACE)
		expr.exprs = parseExprs(ts, RBRACE, prec)
		assert(ts.Next().typ == RBRACE)
	}
	return expr
}

func parseCallExpr(ts *TokenStream, x Expr, prec int) *CallExpr {
	assert(ts.Next().typ == LPAREN)
	exprs := parseExprs(ts, RPAREN, prec)
	assert(ts.Next().typ == RPAREN)
	return &CallExpr{fun: x, args: exprs}
}

func parseAnonFn(ts *TokenStream) *AnonFnExpr {
	if ts.Peek().typ == LBRACE {
		lbrackTok := ts.Next()
		stmts := parseStmts(ts)
		rbrackTok := ts.Next()
		assert(rbrackTok.typ == RBRACE)
		return &AnonFnExpr{stmts: stmts, lbrack: lbrackTok.Pos, rbrack: rbrackTok.Pos}
	}
	panic("")
}

func parseNumberExpr(ts *TokenStream) *NumberExpr {
	tok := ts.Next()
	return &NumberExpr{lit: tok.lit, BaseExpr: BaseExpr{BaseNode: BaseNode{pos: tok.Pos}}}
}

func parseIdentExpr(ts *TokenStream) *IdentExpr {
	return NewIdentExpr(ts.Next())
}

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

type VoidType struct{ BaseTyp }

func (v VoidType) String() string { return "VoidType" }

func (v VoidType) GoStr() string { return "" }

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

func (r ResultType) GoStr() string {
	return fmt.Sprintf("Result[%s]", r.wrappedType.GoStr())
}

func (r ResultType) String() string {
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

type TupleTypeTyp struct {
	BaseTyp
	name string
	elts []Typ
}

func (t TupleTypeTyp) GoStr() string {
	return t.name
}

func (t TupleTypeTyp) String() string {
	var strs []string
	for _, e := range t.elts {
		strs = append(strs, e.GoStr())
	}
	return fmt.Sprintf("TupleTypeTyp(%s)", strings.Join(strs, ", "))
}

type ArrayTypeTyp struct {
	BaseTyp
	elt Typ
}

func (a ArrayTypeTyp) GoStr() string {
	return "[]" + a.elt.GoStr()
}

func (a ArrayTypeTyp) String() string {
	return fmt.Sprintf("ArrayType(%s)", a.elt)
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
	typeParams []Typ
	params     []Typ
	ret        Typ
	variadic   bool
	isNative   bool
}

func (f *FuncType) ReplaceGenericParameter(name string, typ Typ) {
	if v, ok := f.ret.(*GenericType); ok {
		if v.name == name {
			f.ret = typ
		}
	} else if v, ok := f.ret.(*FuncType); ok {
		v.ReplaceGenericParameter(name, typ)
	}
	for i, p := range f.params {
		if v, ok := p.(*GenericType); ok {
			if v.name == name {
				f.params[i] = typ
			}
		} else if v, ok := p.(*FuncType); ok {
			v.ReplaceGenericParameter(name, typ)
		}
	}
}

func (f *FuncType) GoStr() string {
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
	return fmt.Sprintf("func%s(%s)%s", typeParamsStr, strings.Join(paramsStr, ", "), retStr)
}

func (f *FuncType) String() string {
	return fmt.Sprintf("FuncType(...)")
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
			if v, ok := b.typ.(*FuncType); ok {
				fmt.Printf("%v\n", v.GoStr())
			}
			if v, ok := typ.(*FuncType); ok {
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

type TupleType struct {
	BaseExpr
	exprs []Expr
}

func (t TupleType) String() string {
	return fmt.Sprintf("TupleType(%v)", t.exprs)
}

type ArrayType struct {
	BaseExpr
	elt Expr
}

func (a ArrayType) String() string {
	return fmt.Sprintf("ArrayType(%v)", a.elt)
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

type ErrExpr struct {
	BaseExpr
	expr Expr
}

type SomeExpr struct {
	BaseExpr
	expr Expr
}

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

type structStmt struct {
	BaseStmt
	pub    bool
	lit    string
	fields []*Field
}

type importStmt struct {
	lit string
}

type funcType struct {
	BaseExpr
	name       string
	recv       *FieldList
	typeParams *FieldList
	params     *FieldList
	result     FuncOut
}

func (f *funcType) String() string {
	return fmt.Sprintf("funcType(...)")
}

type funcStmt struct {
	BaseStmt
	name       string
	recv       *FieldList
	typeParams *FieldList
	args       *FieldList
	out        FuncOut
	stmts      []Stmt
	typ        Typ
}

type IfStmt struct {
	BaseStmt
	cond Expr
	body []Stmt
	Else Stmt
}

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

type CompositeLitExpr struct {
	BaseExpr
	typ  Expr
	elts []Expr
}

func (c CompositeLitExpr) String() string {
	return fmt.Sprintf("CompositeLitExpr(%v)", c.typ)
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
