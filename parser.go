package main

import (
	"fmt"
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
			ss := parseTypeDecl(ts)
			switch sss := ss.(type) {
			case *EnumStmt:
				s.enums = append(s.enums, sss)
			case *structStmt:
				s.structs = append(s.structs, sss)
			case *InterfaceStmt:
				s.interfaces = append(s.interfaces, sss)
			}
		} else if ts.Peek().typ == FN {
			s.funcs = append(s.funcs, parseFnStmt(ts, false))
		} else if ts.Peek().typ == INLINECOMMENT {
			ts.Next()
		} else {
			assertf(false, "%s: syntax error: non-declaration statement outside function body", ts.Peek().Pos)
		}
	}
	return s
}

func parseFnSignatureStmt(ts *TokenStream) *FuncExpr {
	ft := parseFnSignature(ts)
	return &FuncExpr{
		recv:       ft.recv,
		name:       ft.name,
		typeParams: ft.typeParams,
		args:       ft.args,
		out:        ft.out,
	}
}

var overloadOps = []int{EQL, NEQ, ADD, MINUS, MUL, QUO, REM}

var overloadMapping = map[string]string{
	"==": "__EQL",
	"!=": "__EQL",
	"+":  "__ADD",
	"-":  "__SUB",
	"*":  "__MUL",
	"/":  "__QUO",
	"%":  "__REM",
}

func parseFnSignature(ts *TokenStream) *FuncExpr {
	out := &FuncExpr{}
	assert(ts.Next().typ == FN)

	var recv *FieldList
	if ts.Peek().typ == LPAREN {
		assert(ts.Next().typ == LPAREN)
		if ts.Peek().typ == IDENT { // can be both fn/method

			params := parseParametersList(ts, RPAREN)
			fields1 := params
			assert(ts.Next().typ == RPAREN)

			out.args = fields1

			if ts.Peek().typ == IDENT || // fn/methods
				InArray(ts.Peek().typ, overloadOps) { // Op overloading
				tok := ts.Next()
				if ts.Peek().typ == LBRACKET || ts.Peek().typ == LPAREN { // method

					out.recv = fields1

					// Optional function name
					var name string
					if tok.typ == IDENT {
						name = tok.lit
						tok = ts.Next()
					} else if tok.typ != LPAREN && tok.typ != LBRACKET { // Allow use of reserved words as function names (map)
						name = tok.lit
						tok = ts.Next()
					} else if InArray(ts.Peek().typ, overloadOps) { // Op overloading
						name = tok.lit
						tok = ts.Next()
					}
					out.name = name

					// Type parameters
					if tok.typ == LBRACKET {
						typeParams := parseParametersList(ts, RBRACKET)
						out.typeParams = typeParams
						assert(ts.Next().typ == RBRACKET)
						tok = ts.Next()
					}
					assert(out.typeParams == nil, "Method cannot have type parameters")

					assert(tok.typ == LPAREN)
					out.args = parseParametersList(ts, RPAREN)
					assert(ts.Next().typ == RPAREN)

					// Output
					if ts.Peek().typ != EOF && ts.Peek().typ != LBRACE {
						out.out.expr = parseType(ts)
					}

					return out
				}
				ts.Back()
			}

			// Output
			if ts.Peek().typ != LBRACE {
				out.out.expr = parseType(ts)
			}

			return out
		}
		ts.Back()
	}
	out.recv = recv

	// Optional function name
	tok := ts.Next()
	var name string
	if tok.typ == IDENT {
		name = tok.lit
		tok = ts.Next()
	} else if tok.typ != LPAREN && tok.typ != LBRACKET { // Allow use of reserved words as function names (map)
		name = tok.lit
		tok = ts.Next()
	}
	out.name = name

	// Type parameters
	if tok.typ == LBRACKET {
		typeParams := parseParametersList(ts, RBRACKET)
		out.typeParams = typeParams
		assert(ts.Next().typ == RBRACKET)
		tok = ts.Next()
	}

	assert(tok.typ == LPAREN)
	params := parseParametersList(ts, RPAREN)
	out.args = params
	assert(ts.Next().typ == RPAREN)

	// Output
	if ts.Peek().typ != EOF && ts.Peek().typ != LBRACE {
		out.out.expr = parseType(ts)
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
		switch ts.Peek().typ {
		case DOT:
			ts.Next()
			assert(ts.Peek().typ == IDENT)
			return &SelectorExpr{x: e, sel: NewIdentExpr(ts.Next())}
		case QUESTION:
			ts.Next()
			return &OptionExpr{x: e}
		case BANG:
			ts.Next()
			return &ResultExpr{x: e}
		default:
			return e
		}
	} else if ts.Peek().typ == ELLIPSIS {
		ts.Next()
		return &EllipsisExpr{x: parseType(ts)}
	} else if ts.Peek().typ == LBRACKET {
		ts.Next()
		assert(ts.Next().typ == RBRACKET)
		return &ArrayTypeExpr{elt: parseType(ts)}
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
			return &TupleTypeExpr{exprs: exprs}
		} else {
			panic("not implemented")
		}
	} else if ts.Peek().typ == BANG {
		ts.Next()
		return &BubbleResultExpr{x: &VoidExpr{}}
	}
	panic(fmt.Sprintf("unknown type %v", ts.Peek()))
}

func parseFnStmt(ts *TokenStream, isPub bool) *FuncExpr {
	ft := parseFnSignature(ts)
	assert(ts.Next().typ == LBRACE)
	fs := &FuncExpr{
		recv:       ft.recv,
		name:       ft.name,
		typeParams: ft.typeParams,
		args:       ft.args,
		out:        ft.out,
	}
	fs.stmts = parseStmts(ts)
	assert(ts.Next().typ == RBRACE)
	return fs
}

func parseTypeDecl(ts *TokenStream) (out Stmt) {
	assert(ts.Next().typ == TYPE)
	lit := ts.Next().lit
	if ts.Peek().typ == STRUCT {
		return parseStructTypeDecl(ts, lit)
	} else if ts.Peek().typ == ENUM {
		return parseEnumTypeDecl(ts, lit)
	} else if ts.Peek().typ == INTERFACE {
		return parseInterfaceTypeDecl(ts, lit)
	}
	panic("")
}

func parseInterfaceTypeDecl(ts *TokenStream, lit string) (out *InterfaceStmt) {
	assert(ts.Next().typ == INTERFACE)
	assert(ts.Next().typ == LBRACE)
	elts := parseTypes(ts, RBRACE)
	assert(ts.Next().typ == RBRACE)
	return &InterfaceStmt{lit: lit, elts: elts}
}

func parseEnumTypeDecl(ts *TokenStream, lit string) (out *EnumStmt) {
	assert(ts.Next().typ == ENUM)
	assert(ts.Next().typ == LBRACE)
	fields := make([]*EnumField, 0)
	for {
		if ts.Peek().typ == RBRACE {
			break
		}
		id := parseIdentExpr(ts)
		var args []Expr
		if ts.Peek().typ == LPAREN {
			assert(ts.Next().typ == LPAREN)
			args = parseTypes(ts, RPAREN)
			assert(ts.Next().typ == RPAREN)
		}
		assert(ts.Next().typ == COMMA)
		fields = append(fields, &EnumField{name: id, elts: args})
	}
	assert(ts.Next().typ == RBRACE)
	return &EnumStmt{lit: lit, fields: fields}
}

func parseStructTypeDecl(ts *TokenStream, lit string) (out *structStmt) {
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
	default:
		if len(x) == 1 {
			return &ExprStmt{x: x[0]}
		} else if len(x) > 1 {
			panic(fmt.Sprintf("unknown stmt type %v", ts.Peek()))
		}
	}
	switch ts.Peek().typ {
	case VAR:
		pos := ts.Peek().Pos
		assert(ts.Next().typ == VAR)
		id := parseIdentExpr(ts)
		t := parseType(ts)
		var values []Expr
		if ts.Peek().typ == ASSIGN {
			assert(ts.Next().typ == ASSIGN)
			value := parseExpr(ts, 1)
			values = []Expr{value}
		}
		return &ValueSpec{names: []*IdentExpr{id}, typ: t, values: values, opPos: pos}
	case IF:
		return parseIfStmt(ts)
	case RETURN:
		return parseReturnStmt(ts)
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

func parseMatchExpr(ts *TokenStream) *MatchExpr {
	assert(ts.Next().typ == MATCH)
	expr := parseExpr(ts, 1)
	assert(ts.Next().typ == LBRACE)
	var cases []*MatchCase
	for ts.Peek().typ != RBRACE {
		x1 := parseExpr(ts, 1)
		assert(ts.Next().typ == FATARROWRIGHT)
		if ts.Peek().typ == LBRACE {
			assert(ts.Next().typ == LBRACE)
			stmts := parseStmts(ts)
			assert(ts.Next().typ == RBRACE)
			cases = append(cases, &MatchCase{cond: x1, body: stmts})
		} else {
			x := parseExpr(ts, 1)
			cases = append(cases, &MatchCase{cond: x1, body: []Stmt{&ExprStmt{x: x}}})
		}
		if ts.Peek().typ == RBRACE {
			break
		}
		assert(ts.Next().typ == COMMA)
	}
	assert(ts.Next().typ == RBRACE)
	return &MatchExpr{expr: expr, cases: cases}
}

func parseInlineComment(ts *TokenStream) *InlineCommentStmt {
	stmt := &InlineCommentStmt{lit: ts.Peek().lit}
	ts.Next()
	return stmt
}

func parseIfStmt(ts *TokenStream) Stmt {
	assert(ts.Next().typ == IF)
	expr := parseExpr(ts, 1)
	var isLet bool
	var expr2 Expr
	if ts.Peek().typ == IN {
		tok := ts.Next()
		expr2 := parsePrimaryExpr(ts, 1)
		expr = &BinOpExpr{lhs: expr, rhs: expr2, op: tok}
	} else if ts.Peek().typ == WALRUS {
		assert(ts.Next().typ == WALRUS)
		expr2 = parseExpr(ts, 1)
		isLet = true
	}
	assert(ts.Next().typ == LBRACE)
	stmts := parseStmts(ts)
	assert(ts.Next().typ == RBRACE)
	var elseStmt Stmt
	if ts.Peek().typ == ELSE {
		ts.Next() // else
		if ts.Peek().typ == IF {
			elseStmt = parseIfStmt(ts)
		} else if ts.Peek().typ == LBRACE {
			elseStmt = parseBlockStmt(ts)
		}
	}
	if isLet {
		return &IfLetStmt{lhs: expr, rhs: expr2, body: stmts, Else: elseStmt}
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
			assertf(x != nil, "%s: syntax error", ts.Peek().Pos)
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
		var x Expr = parseVecExpr(ts, prec)
		if ts.Peek().typ == LPAREN {
			assert(ts.Next().typ == LPAREN)
			e := parseExpr(ts, 1)
			assert(ts.Next().typ == RPAREN)
			x = &CallExpr{fun: x, args: []Expr{e}}
		}
		return x
	case MUT:
		ts.Next()
		return &MutExpr{x: parseExpr(ts, prec)}
	case MATCH:
		return parseMatchExpr(ts)
	case IDENT:
		ident := parseIdentExpr(ts)
		switch ts.Peek().typ {
		case DOT:
			assert(ts.Next().typ == DOT)
			if ts.Peek().typ == LPAREN { // Type casting
				assert(ts.Next().typ == LPAREN)
				t := parseType(ts)
				assert(ts.Next().typ == RPAREN)
				return &TypeAssertExpr{x: ident, typ: t}
			}
			ident2 := parseIdentExpr(ts)
			sel := &SelectorExpr{x: ident, sel: ident2}
			if ts.Peek().typ == LPAREN {
				return parseCallExpr(ts, sel, prec)
			}
			return sel
		case LBRACE:
			assert(ts.Next().typ == LBRACE)
			var elts []Expr
			if ts.Peek().typ != RBRACE {
				elts = parseElementList(ts, prec)
			}
			assert(ts.Next().typ == RBRACE)
			return &CompositeLitExpr{typ: ident, elts: elts}
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

func parseElementList(ts *TokenStream, prec int) (elts []Expr) {
	for ts.Peek().typ != RBRACE {
		elts = append(elts, parseElement(ts, prec))
		if ts.Peek().typ == RBRACE {
			break
		}
		assert(ts.Next().typ == COMMA)
	}
	return
}

func parseElement(ts *TokenStream, prec int) Expr {
	key := parseExpr(ts, prec)
	assert(ts.Next().typ == COLON)
	return &KeyValueExpr{key: key, value: parseExpr(ts, prec)}
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
