package main

import (
	goast "agl/ast"
	"agl/token"
	"agl/types"
	"fmt"
	"strings"
)

var fset *token.FileSet

func codegen(fset1 *token.FileSet, env *Env, a *goast.File) (out string) {
	fset = fset1
	before1, out1 := genPackage(a)
	before2, out2 := genImports(a)
	out3 := genDecls(env, a, "")
	for _, b := range before1 {
		out += b.Content()
	}
	for _, b := range before2 {
		out += b.Content()
	}
	return out + out1 + out2 + out3
}

func genPackage(a *goast.File) (before []IBefore, out string) {
	out += fmt.Sprintf("package %s\n", a.Name.Name)
	return
}

func genImports(a *goast.File) (before []IBefore, out string) {
	for _, spec := range a.Imports {
		out += "import "
		if spec.Name != nil {
			out += spec.Name.Name
		}
		out += spec.Path.Value + "\n"
	}
	return
}

func genExpr(env *Env, e goast.Expr, prefix string) (before []IBefore, out string) {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *goast.Ident:
		return genIdent(env, expr, prefix)
	case *goast.ShortFuncLit:
		return genShortFuncLit(env, expr, prefix)
	case *goast.OptionExpr:
		before1, content := genExpr(env, expr.X, prefix)
		return before1, fmt.Sprintf("Option[%s]", content)
	case *goast.ResultExpr:
		before1, content := genExpr(env, expr.X, prefix)
		return before1, fmt.Sprintf("Result[%s]", content)
	case *goast.BinaryExpr:
		before1, content1 := genExpr(env, expr.X, prefix)
		before2, content2 := genExpr(env, expr.Y, prefix)
		before = append(before, before1...)
		before = append(before, before2...)
		return before, fmt.Sprintf("%s %s %s", content1, expr.Op.String(), content2)
	case *goast.BasicLit:
		return nil, expr.Value
	case *goast.CompositeLit:
		return genCompositeLit(env, expr, prefix)
	case *goast.TupleExpr:
		return genTupleExpr(env, expr, prefix)
	case *goast.KeyValueExpr:
		return genKeyValueExpr(env, expr, prefix)
	case *goast.ArrayType:
		return genArrayType(env, expr, prefix)
	case *goast.CallExpr:
		return genCallExpr(env, expr, prefix)
	case *goast.BubbleResultExpr:
		return genBubbleResultExpr(env, expr, prefix)
	case *goast.BubbleOptionExpr:
		return genBubbleOptionExpr(env, expr, prefix)
	case *goast.SelectorExpr:
		return genSelectorExpr(env, expr, prefix)
	case *goast.IndexExpr:
		return genIndexExpr(env, expr, prefix)
	case *goast.FuncType:
		return genFuncType(env, expr, prefix)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func genIdent(env *Env, expr *goast.Ident, prefix string) (before []IBefore, out string) {
	if strings.HasPrefix(expr.Name, "$") {
		expr.Name = strings.Replace(expr.Name, "$", "aglArg", 1)
	}
	t := env.GetType(expr)
	switch typ := t.(type) {
	case types.OkType:
		return nil, "MakeResultOk"
	case types.ErrType:
		return nil, fmt.Sprintf("MakeResultErr[%s]", typ.W.GoStr())
	case types.SomeType:
		return nil, "MakeOptionSome"
	case types.NoneType:
		return nil, fmt.Sprintf("MakeOptionNone[%s]()", typ.W.GoStr())
	case types.GenericType:
		return nil, fmt.Sprintf("%s", typ.GoStr())
	}
	if v := env.Get(expr.Name); v != nil {
		return nil, v.GoStr()
	}
	return nil, expr.Name
}

func genShortFuncLit(env *Env, expr *goast.ShortFuncLit, prefix string) (before []IBefore, out string) {
	t := env.GetType(expr).(types.FuncType)
	before1, content1 := genStmt(env, expr.Body, prefix+"\t")
	var returnStr, argsStr string
	if len(t.Params) > 0 {
		var tmp []string
		for i, arg := range t.Params {
			tmp = append(tmp, fmt.Sprintf("aglArg%d %s", i, arg.GoStr()))
		}
		argsStr = strings.Join(tmp, ", ")
	}
	ret := t.Return
	if ret != nil {
		returnStr = " " + ret.GoStr()
	}
	out += fmt.Sprintf("func(%s)%s {\n", argsStr, returnStr)
	out += content1
	out += prefix + "}"
	return before1, out
}

func genFuncType(env *Env, expr *goast.FuncType, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.Result, prefix+"\t")
	before = append(before, before1...)
	var paramsStr, typeParamsStr string
	if typeParams := expr.TypeParams; typeParams != nil {
		typeParamsStr = joinList(env, expr.TypeParams, prefix)
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if params := expr.Params; params != nil {
		paramsStr = joinList(env, params, prefix)
	}
	if content1 != "" {
		content1 = " " + content1
	}
	out += fmt.Sprintf("func%s(%s)%s", typeParamsStr, paramsStr, content1)
	return before1, out
}

func genIndexExpr(env *Env, expr *goast.IndexExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.X, prefix)
	before2, content2 := genExpr(env, expr.Index, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	out += fmt.Sprintf("%s[%s]", content1, content2)
	return
}

func genSelectorExpr(env *Env, expr *goast.SelectorExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.X, prefix)
	return before1, fmt.Sprintf("%s.%s", content1, expr.Sel.Name)
}

func genBubbleOptionExpr(env *Env, expr *goast.BubbleOptionExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.X, prefix)
	out += fmt.Sprintf("%s.Unwrap()", content1)
	return before1, out
}

func genBubbleResultExpr(env *Env, expr *goast.BubbleResultExpr, prefix string) (before []IBefore, out string) {
	t := env.GetType(expr).(types.BubbleResultType)
	if t.Bubble {
		before1, content1 := genExpr(env, expr.X, prefix)
		before2 := NewBeforeStmt(addPrefix(`res := `+content1+`
if res.IsErr() {
	return res
}
`, prefix))
		before = append(before, before1...)
		before = append(before, before2)
		out += "res.Unwrap()"
	} else {
		before1, content1 := genExpr(env, expr.X, prefix)
		before1 = append(before, before1...)
		out += fmt.Sprintf("%s.Unwrap()", content1)
	}
	return before, out
}

func genCallExpr(env *Env, expr *goast.CallExpr, prefix string) (before []IBefore, out string) {
	switch e := expr.Fun.(type) {
	case *goast.SelectorExpr:
		if TryCast[types.ArrayType](env.GetType(e.X)) && e.Sel.Name == "filter" {
			before1, content1 := genExpr(env, e.X, prefix)
			before2, content2 := genExpr(env, expr.Args[0], prefix)
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
		} else if TryCast[types.ArrayType](env.GetType(e.X)) && e.Sel.Name == "Map" {
			before1, content1 := genExpr(env, e.X, prefix)
			before2, content2 := genExpr(env, expr.Args[0], prefix)
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
		} else if TryCast[types.ArrayType](env.GetType(e.X)) && e.Sel.Name == "Reduce" {
			before1, content1 := genExpr(env, e.X, prefix)
			before2, content2 := genExpr(env, expr.Args[0], prefix)
			before3, content3 := genExpr(env, expr.Args[1], prefix)
			before = append(before, before1...)
			before = append(before, before2...)
			before = append(before, before3...)
			return before, fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
		}
	case *goast.Ident:
		if e.Name == "assert" {
			var contents []string
			var assertExprStr string
			for i, arg := range expr.Args {
				before1, content1 := genExpr(env, arg, prefix)
				if i == 0 {
					assertExprStr = content1
				} else if i == 1 {
					content1 = fmt.Sprintf(`"assert failed '%s' line %d" + " " + `, assertExprStr, env.fset.Position(arg.Pos()).Line) + content1
				}
				before = append(before, before1...)
				contents = append(contents, content1)
			}
			out := strings.Join(contents, ", ")
			return before, fmt.Sprintf("AglAssert(%s)", out)
		}
	}
	before1, content1 := genExpr(env, expr.Fun, prefix)
	before2, content2 := genExprs(env, expr.Args, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, fmt.Sprintf("%s(%s)", content1, content2)
}

func genArrayType(env *Env, expr *goast.ArrayType, prefix string) (before []IBefore, out string) {
	before1, content := genExpr(env, expr.Elt, prefix)
	return before1, fmt.Sprintf("[]%s", content)
}

func genKeyValueExpr(env *Env, expr *goast.KeyValueExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.Key, prefix)
	before2, content2 := genExpr(env, expr.Value, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, fmt.Sprintf("%s: %s", content1, content2)
}

func genCompositeLit(env *Env, expr *goast.CompositeLit, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.Type, prefix)
	before2, content2 := genExprs(env, expr.Elts, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	out += fmt.Sprintf("%s{%s}", content1, content2)
	return
}

func genTupleExpr(env *Env, expr *goast.TupleExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExprs(env, expr.Values, prefix)
	out += fmt.Sprintf("(%s)\n", content1)
	return before1, out
}

func genExprs(env *Env, e []goast.Expr, prefix string) (before []IBefore, out string) {
	var tmp []string
	for _, expr := range e {
		before1, content1 := genExpr(env, expr, prefix)
		before = append(before, before1...)
		tmp = append(tmp, content1)
	}
	return before, strings.Join(tmp, ", ")
}

func genStmts(env *Env, s []goast.Stmt, prefix string) (before []IBefore, out string) {
	for _, stmt := range s {
		before1, content1 := genStmt(env, stmt, prefix)
		before = append(before, before1...)
		for _, b := range before {
			if v, ok := b.(*BeforeStmt); ok {
				out += v.Content()
			}
		}
		newBefore := make([]IBefore, 0)
		for _, b := range before {
			if v, ok := b.(*BeforeFn); ok {
				newBefore = append(newBefore, v)
			}
		}
		before = newBefore
		out += content1
	}
	return before, out
}

func genStmt(env *Env, s goast.Stmt, prefix string) (before []IBefore, out string) {
	//p("genStmt", to(s))
	switch stmt := s.(type) {
	case *goast.BlockStmt:
		return genBlockStmt(env, stmt, prefix)
	case *goast.IfStmt:
		return genIfStmt(env, stmt, prefix)
	case *goast.AssignStmt:
		return genAssignStmt(env, stmt, prefix)
	case *goast.ExprStmt:
		return genExprStmt(env, stmt, prefix)
	case *goast.ReturnStmt:
		return genReturnStmt(env, stmt, prefix)
	case *goast.RangeStmt:
		return genRangeStmt(env, stmt, prefix)
	case *goast.IncDecStmt:
		return getIncDecStmt(env, stmt, prefix)
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
}

func genBlockStmt(env *Env, stmt *goast.BlockStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genStmts(env, stmt.List, prefix)
	before = append(before, before1...)
	out += content1
	return
}

func getIncDecStmt(env *Env, stmt *goast.IncDecStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, stmt.X, prefix)
	before = append(before, before1...)
	var op string
	switch stmt.Tok {
	case token.INC:
		op = "++"
	case token.DEC:
		op = "--"
	}
	out += prefix + content1 + op + "\n"
	return
}

func genRangeStmt(env *Env, stmt *goast.RangeStmt, prefix string) (before []IBefore, out string) {
	var content1, content2 string
	if stmt.Key != nil {
		var before1 []IBefore
		before1, content1 = genExpr(env, stmt.Key, prefix)
		before = append(before, before1...)
	}
	if stmt.Value != nil {
		var before2 []IBefore
		before2, content2 = genExpr(env, stmt.Value, prefix)
		before = append(before, before2...)
	}
	before3, content3 := genExpr(env, stmt.X, prefix)
	before = append(before, before3...)
	before4, content4 := genStmt(env, stmt.Body, prefix+"\t")
	before = append(before, before4...)
	op := stmt.Tok
	if content1 == "" && content2 == "" {
		out += prefix + fmt.Sprintf("for range %s {\n", content3)
	} else if content2 == "" {
		out += prefix + fmt.Sprintf("for %s %s range %s {\n", content1, op, content3)
	} else {
		out += prefix + fmt.Sprintf("for %s, %s %s range %s {\n", content1, content2, op, content3)
	}
	out += fmt.Sprintf("%s", content4)
	out += prefix + "}\n"
	return
}

func genReturnStmt(env *Env, stmt *goast.ReturnStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, stmt.Result, prefix)
	return before1, prefix + fmt.Sprintf("return %s\n", content1)
}

func genExprStmt(env *Env, stmt *goast.ExprStmt, prefix string) (before []IBefore, out string) {
	before1, content := genExpr(env, stmt.X, prefix)
	out += prefix + content + "\n"
	return before1, out
}

func genAssignStmt(env *Env, stmt *goast.AssignStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genExprs(env, stmt.Lhs, prefix)
	before2, content2 := genExprs(env, stmt.Rhs, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, prefix + fmt.Sprintf("%s %s %s\n", content1, stmt.Tok.String(), content2)
}

func genIfStmt(env *Env, stmt *goast.IfStmt, prefix string) (before []IBefore, out string) {
	before1, cond := genExpr(env, stmt.Cond, prefix)
	before2, body := genStmt(env, stmt.Body, prefix+"\t")
	before = append(before, before1...)
	before = append(before, before2...)
	out += prefix + "if " + cond + " {\n"
	out += body
	out += prefix + "}\n"
	return before, out
}

func genDecls(env *Env, a *goast.File, prefix string) (out string) {
	for _, d := range a.Decls {
		switch decl := d.(type) {
		case *goast.FuncDecl:
			before1, out1 := genFuncDecl(env, decl, prefix)
			for _, b := range before1 {
				out += b.Content()
			}
			out += out1
		}
	}
	return out
}

func genFuncDecl(env *Env, decl *goast.FuncDecl, prefix string) (before []IBefore, out string) {
	var name, recv, typeParamsStr, paramsStr, resultStr, bodyStr string
	if decl.Recv != nil {
		recv = joinList(env, decl.Recv, prefix)
		if recv != "" {
			recv = " [" + recv + "]"
		}
	}
	if decl.Name != nil {
		name = " " + decl.Name.Name
	}
	if typeParams := decl.Type.TypeParams; typeParams != nil {
		typeParamsStr = joinList(env, decl.Type.TypeParams, prefix)
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if params := decl.Type.Params; params != nil {
		paramsStr = joinList(env, params, prefix)
	}
	if result := decl.Type.Result; result != nil {
		before1, content := genExpr(env, result, prefix)
		before = append(before, before1...)
		if content != "" {
			resultStr = " " + content
		}
	}
	if decl.Body != nil {
		before1, content := genStmt(env, decl.Body, prefix+"\t")
		before = append(before, before1...)
		bodyStr = content
	}
	out += fmt.Sprintf("func%s%s%s(%s)%s {\n%s}\n", recv, name, typeParamsStr, paramsStr, resultStr, bodyStr)
	return
}

func joinList(env *Env, l *goast.FieldList, prefix string) string {
	if l == nil {
		return ""
	}
	var fieldsItems []string
	for _, field := range l.List {
		var namesItems []string
		for _, n := range field.Names {
			namesItems = append(namesItems, n.Name)
		}
		tmp2Str := strings.Join(namesItems, ", ")
		_, content := genExpr(env, field.Type, prefix)
		if tmp2Str != "" {
			tmp2Str = tmp2Str + " "
		}
		fieldsItems = append(fieldsItems, tmp2Str+content)
	}
	return strings.Join(fieldsItems, ", ")
}

type IBefore interface {
	Content() string
}

type BaseBefore struct {
	w string
}

func (b *BaseBefore) Content() string {
	return b.w
}

type BeforeStmt struct {
	BaseBefore
}

func NewBeforeStmt(content string) *BeforeStmt {
	return &BeforeStmt{BaseBefore{w: content}}
}

type BeforeFn struct {
	BaseBefore
}

func NewBeforeFn(content string) *BeforeFn {
	return &BeforeFn{BaseBefore{w: content}}
}

//
//func genStmts(env *Env, stmts []Stmt, prefix string, retTyp Typ) (before []IBefore, out string) {
//	for _, stmt := range stmts {
//		before1, content1 := genStmt(env, stmt, prefix, retTyp)
//		before = append(before, before1...)
//		for _, b := range before {
//			if v, ok := b.(*BeforeStmt); ok {
//				out += v.Content()
//			}
//		}
//		newBefore := make([]IBefore, 0)
//		for _, b := range before {
//			if v, ok := b.(*BeforeFn); ok {
//				newBefore = append(newBefore, v)
//			}
//		}
//		before = newBefore
//		out += content1
//	}
//	return
//}

//func genEnumStmt(_ *Env, s *EnumStmt) (before []IBefore, out string) {
//	out += fmt.Sprintf("type %sTag int\n", s.lit)
//	out += "const (\n"
//	for i, field := range s.fields {
//		if i == 0 {
//			out += fmt.Sprintf("\t%s_%s %sTag = iota + 1\n", s.lit, field.name.lit, s.lit)
//		} else {
//			out += fmt.Sprintf("\t%s_%s\n", s.lit, field.name.lit)
//		}
//	}
//	out += ")\n"
//	out += fmt.Sprintf("type %s struct {\n", s.lit)
//	out += fmt.Sprintf("\ttag %sTag\n", s.lit)
//	for _, field := range s.fields {
//		for i, el := range field.elts {
//			out += fmt.Sprintf("\t%s%d %s\n", field.name.lit, i, el.GetType().GoStr())
//		}
//	}
//	out += "}\n"
//	out += fmt.Sprintf("func (v %s) String() string {\n\tswitch v.tag {\n", s.lit)
//	for _, field := range s.fields {
//		out += fmt.Sprintf("\tcase %s_%s:\n\t\treturn \"%s\"\n", s.lit, field.name.lit, field.name.lit)
//	}
//	out += "\tdefault:\n\t\tpanic(\"\")\n\t}\n}\n"
//	for _, field := range s.fields {
//		var tmp []string
//		var tmp1 []string
//		for i, el := range field.elts {
//			tmp = append(tmp, fmt.Sprintf("arg%d %s", i, el.GetType().GoStr()))
//			tmp1 = append(tmp1, fmt.Sprintf("%s%d: arg%d", field.name.lit, i, i))
//		}
//		var tmp1Out string
//		if len(tmp1) > 0 {
//			tmp1Out = ", " + strings.Join(tmp1, ", ")
//		}
//		out += fmt.Sprintf("func Make_%s_%s(%s) %s {\n\treturn %s{tag: %s_%s%s}\n}\n",
//			s.lit, field.name.lit, strings.Join(tmp, ", "), s.lit, s.lit, s.lit, field.name.lit, tmp1Out)
//	}
//	return
//}

//func genStructStmt(env *Env, s *structStmt) (before []IBefore, out string) {
//	var namePrefix string
//	if !s.pub {
//		namePrefix = "aglPriv"
//	}
//	var fields string
//	for _, f := range s.fields {
//		_, ft := genExpr(env, f, "\t", nil)
//		fields += fmt.Sprintf("\t%s\n", ft)
//	}
//	out += fmt.Sprintf("type %s%s struct {\n%s}\n", namePrefix, s.lit, fields)
//	return
//}
//
//func genAssignStmt1(env *Env, stmtLhs, stmtRhs Expr, tok Tok, prefix string, retTyp Typ) (before []IBefore, out string) {
//	// LHS
//	var lhs string
//	var after string
//	if TryCast[*TupleExpr](stmtLhs) {
//		if TryCast[*EnumType](stmtRhs.GetType()) {
//			lhs = AglVariablePrefix + "1"
//			var sel string
//			switch v := stmtRhs.(type) {
//			case *CallExpr:
//				sel = v.fun.(*SelectorExpr).sel.lit
//			case *IdentExpr:
//				sel = v.lit
//			}
//
//			var names []string
//			var exprs []string
//			for i, x := range stmtLhs.(*TupleExpr).exprs {
//				names = append(names, x.(*IdentExpr).lit)
//				exprs = append(exprs, fmt.Sprintf("%s.%s%d", lhs, sel, i))
//			}
//			after = prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
//		} else {
//			lhs = AglVariablePrefix + "1"
//			var names []string
//			var exprs []string
//			for i, x := range stmtLhs.(*TupleExpr).exprs {
//				names = append(names, x.(*IdentExpr).lit)
//				exprs = append(exprs, fmt.Sprintf("%s.Arg%d", lhs, i))
//			}
//			after = prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
//		}
//	} else {
//		before1, content1 := genExpr(env, stmtLhs, prefix, retTyp)
//		before = append(before, before1...)
//		lhs = content1
//	}
//
//	// RHS
//	before2, content2 := genExpr(env, stmtRhs, prefix, retTyp)
//	before = append(before, before2...)
//
//	// Output
//	out += prefix + fmt.Sprintf("%s %s %s\n", lhs, tok.lit, content2)
//	out += after
//	return
//}
//
//
//func genIfLetStmt(env *Env, stmt *IfLetStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
//	var tmp string
//	if TryCast[*SomeExpr](stmt.lhs) {
//		id := stmt.lhs.(*SomeExpr).expr.(*IdentExpr).lit
//		_, content2 := genExpr(env, stmt.rhs, prefix, retTyp)
//		tmp += "if res := " + content2 + "; res.IsSome() {\n"
//		tmp += addPrefix("\t"+id+" := res.Unwrap()\n", prefix)
//	} else if TryCast[*OkExpr](stmt.lhs) {
//		id := stmt.lhs.(*OkExpr).expr.(*IdentExpr).lit
//		_, content2 := genExpr(env, stmt.rhs, prefix, retTyp)
//		tmp += "if res := " + content2 + "; res.IsOk() {\n"
//		tmp += addPrefix("\t"+id+" := res.Unwrap()\n", prefix)
//	} else if TryCast[*ErrExpr](stmt.lhs) {
//		id := stmt.lhs.(*ErrExpr).expr.(*IdentExpr).lit
//		_, content2 := genExpr(env, stmt.rhs, prefix, retTyp)
//		tmp += "if res := " + content2 + "; res.IsErr() {\n"
//		tmp += addPrefix("\t"+id+" := res.Err()\n", prefix)
//	} else if TryCast[*IdentExpr](stmt.lhs) {
//		id := stmt.lhs.(*IdentExpr).lit
//		before2, content2 := genExpr(env, stmt.rhs, prefix, retTyp)
//		before3, content3 := genExpr(env, stmt.cond, prefix, retTyp)
//		before = append(before, before2...)
//		before = append(before, before3...)
//		tmp += "if " + id + " := " + content2 + "; " + content3 + " {\n"
//	}
//
//	before3, content3 := genStmts(env, stmt.body, prefix+"\t", retTyp)
//	before = append(before, before3...)
//	out += prefix + tmp
//	out += content3
//	if stmt.Else != nil {
//		before4, content4 := genStmt(env, stmt.Else, prefix, retTyp)
//		before = append(before, before4...)
//		out += prefix + "} else " + strings.TrimSpace(content4) + "\n"
//	} else {
//		out += prefix + "}\n"
//	}
//	return
//}
//
//func genMatchExpr(env *Env, stmt *MatchExpr, prefix string, retTyp Typ) (before []IBefore, out string) {
//	if TryCast[OptionType](stmt.expr.GetType()) {
//		before1, content1 := genExpr(env, stmt.expr, prefix, retTyp)
//		before = append(before, before1...)
//		out += fmt.Sprintf("res := %s\n", content1)
//		cases := stmt.cases
//		sort.Slice(cases, func(i, j int) bool {
//			iSome := TryCast[*SomeExpr](cases[i].cond)
//			iNone := TryCast[*NoneExpr](cases[i].cond)
//			iAll := TryCast[*IdentExpr](cases[i].cond) && cases[i].cond.(*IdentExpr).lit == "_"
//			jSome := TryCast[*SomeExpr](cases[j].cond)
//			jNone := TryCast[*NoneExpr](cases[j].cond)
//			jAll := TryCast[*IdentExpr](cases[j].cond) && cases[j].cond.(*IdentExpr).lit == "_"
//			// If i is Some and j is not Some, i should come first
//			if iSome && !jSome {
//				return true
//			}
//			// If j is Some and i is not Some, j should come first
//			if jSome && !iSome {
//				return false
//			}
//			// If i is None and j is All, i should come first
//			if iNone && jAll {
//				return true
//			}
//			// If j is None and i is All, j should come first
//			if jNone && iAll {
//				return false
//			}
//			return false // Keep original order for equal types
//		})
//		for i, e := range stmt.cases {
//			var condition string
//			var setup string
//			switch cond := e.cond.(type) {
//			case *NoneExpr:
//				condition = "res.IsNone()"
//			case *SomeExpr:
//				id := cond.expr.(*IdentExpr).lit
//				condition = "res.IsSome()"
//				setup = fmt.Sprintf("%s := res.Unwrap()\n", id)
//			case *IdentExpr:
//				if cond.lit == "_" {
//					condition = "res.IsSome() || res.IsNone()"
//				}
//			}
//			if i == 0 {
//				out += prefix + fmt.Sprintf("if %s {\n", condition)
//			} else {
//				out += fmt.Sprintf(" else if %s {\n", condition)
//			}
//			if setup != "" {
//				out += prefix + "\t" + setup
//			}
//			for _, s := range e.body {
//				before2, content2 := genStmt(env, s, prefix+"\t", retTyp)
//				before = append(before, before2...)
//				out += content2
//			}
//			out += prefix + "}"
//		}
//	} else if TryCast[ResultType](stmt.expr.GetType()) {
//		before1, content1 := genExpr(env, stmt.expr, prefix, retTyp)
//		before = append(before, before1...)
//		out += fmt.Sprintf("res := %s\n", content1)
//		cases := stmt.cases
//		sort.Slice(cases, func(i, j int) bool {
//			iOk := TryCast[*OkExpr](cases[i].cond)
//			iErr := TryCast[*ErrExpr](cases[i].cond)
//			iAll := TryCast[*IdentExpr](cases[i].cond) && cases[i].cond.(*IdentExpr).lit == "_"
//			jOk := TryCast[*OkExpr](cases[j].cond)
//			jErr := TryCast[*ErrExpr](cases[j].cond)
//			jAll := TryCast[*IdentExpr](cases[j].cond) && cases[j].cond.(*IdentExpr).lit == "_"
//			// If i is Some and j is not Some, i should come first
//			if iOk && !jOk {
//				return true
//			}
//			// If j is Some and i is not Some, j should come first
//			if jOk && !iOk {
//				return false
//			}
//			// If i is None and j is All, i should come first
//			if iErr && jAll {
//				return true
//			}
//			// If j is None and i is All, j should come first
//			if jErr && iAll {
//				return false
//			}
//			return false // Keep original order for equal types
//		})
//		for i, e := range stmt.cases {
//			var condition string
//			var setup string
//			switch cond := e.cond.(type) {
//			case *ErrExpr:
//				condition = "res.IsErr()"
//			case *OkExpr:
//				id := cond.expr.(*IdentExpr).lit
//				condition = "res.IsOk()"
//				setup = fmt.Sprintf("%s := res.Unwrap()\n", id)
//			case *IdentExpr:
//				if cond.lit == "_" {
//					condition = "res.IsOk() || res.IsErr()"
//				}
//			}
//			if i == 0 {
//				out += prefix + fmt.Sprintf("if %s {\n", condition)
//			} else {
//				out += fmt.Sprintf(" else if %s {\n", condition)
//			}
//			if setup != "" {
//				out += prefix + "\t" + setup
//			}
//			for _, s := range e.body {
//				before2, content2 := genStmt(env, s, prefix+"\t", retTyp)
//				before = append(before, before2...)
//				out += content2
//			}
//			out += prefix + "}"
//		}
//	}
//	return
//}

//func genSomeExpr(env *Env, expr *SomeExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	before, content := genExpr(env, expr.expr, prefix, retTyp)
//	return before, fmt.Sprintf("MakeOptionSome(%s)", content)
//}
//
//func genNoneExpr(_ *Env, expr *NoneExpr, _ string, _ Typ) ([]IBefore, string) {
//	return nil, fmt.Sprintf("MakeOptionNone[%s]()", expr.typ.(OptionType).wrappedType.GoStr())
//}
//
//func genOkExpr(env *Env, expr *OkExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	before, content := genExpr(env, expr.expr, prefix, retTyp)
//	return before, fmt.Sprintf("MakeResultOk(%s)", content)
//}
//
//func genErrExpr(env *Env, expr *ErrExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	newExpr := expr.expr
//	if v, ok := expr.expr.(*StringExpr); ok {
//		newExpr = &CallExpr{fun: &SelectorExpr{x: &IdentExpr{lit: "errors"}, sel: &IdentExpr{lit: "New"}}, args: []Expr{v}}
//	}
//	before, content := genExpr(env, newExpr, prefix, retTyp)
//	return before, fmt.Sprintf("MakeResultErr[%s](%s)", expr.typ.(ResultType).wrappedType.GoStr(), content)
//}
//
//
//func genBinOpExpr(env *Env, expr *BinOpExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	var before []IBefore
//	op := expr.op.lit
//	before1, content1 := genExpr(env, expr.lhs, prefix, retTyp)
//	before2, content2 := genExpr(env, expr.rhs, prefix, retTyp)
//	before = append(before, before1...)
//	before = append(before, before2...)
//	if op == "in" {
//		return before, fmt.Sprintf("AglVecIn(%s, %s)", content2, content1)
//	}
//	if expr.lhs.GetType() != nil && expr.rhs.GetType() != nil {
//		if TryCast[*StructType](expr.lhs.GetType()) && TryCast[*StructType](expr.rhs.GetType()) {
//			lhsName := expr.lhs.GetType().(*StructType).name
//			rhsName := expr.rhs.GetType().(*StructType).name
//			if lhsName == rhsName {
//				if (op == "==" || op == "!=") && env.Get(lhsName+".__EQL") != nil {
//					if op == "==" {
//						return before, fmt.Sprintf("%s.__EQL(%s)", content1, content2)
//					} else {
//						return before, fmt.Sprintf("!%s.__EQL(%s)", content1, content2)
//					}
//				}
//			}
//		}
//	}
//	return before, fmt.Sprintf("%s %s %s", content1, op, content2)
//}
//
//func genIdentExpr(_ *Env, expr *IdentExpr, _ string, _ Typ) ([]IBefore, string) {
//	if strings.HasPrefix(expr.lit, "$") {
//		expr.lit = strings.Replace(expr.lit, "$", "aglArg", 1)
//	}
//	return nil, fmt.Sprintf("%s", expr.lit)
//}
//
//func genTypeAssertExpr(env *Env, expr *TypeAssertExpr, prefix string, retTyp Typ) (before []IBefore, out string) {
//	before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//	before2, content2 := genExpr(env, expr.typ, prefix, retTyp)
//	before = append(before, before1...)
//	before = append(before, before2...)
//	out += fmt.Sprintf("AglTypeAssert[%s](%s)", content2, content1)
//	return
//}
//
//
//func genAnonFnExpr(env *Env, expr *AnonFnExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	t := expr.typ.(FuncType)
//	var argsT string
//	if len(t.params) > 0 {
//		var tmp []string
//		for i, arg := range t.params {
//			tmp = append(tmp, fmt.Sprintf("aglArg%d %s", i, arg.GoStr()))
//		}
//		argsT = strings.Join(tmp, ", ")
//	}
//	var retT string
//	if t.ret != nil {
//		retT = " " + t.ret.GoStr()
//	}
//
//	// If the function body only has 1 stmt, auto add "return" of the only expression value
//	if len(expr.stmts) == 1 && t.ret != nil && t.ret.GoStr() != "" {
//		if e, ok := expr.stmts[0].(*ExprStmt); ok {
//			expr.stmts[0] = &ReturnStmt{expr: e.x}
//		}
//	}
//	before1, content1 := genStmts(env, expr.stmts, prefix+"\t", retTyp)
//	return before1, fmt.Sprintf("func(%s)%s {\n%s%s}", argsT, retT, content1, prefix) // TODO
//}
//
//func genSelectorExpr(env *Env, expr *SelectorExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	var before []IBefore
//	before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//	before2, content2 := genExpr(env, expr.sel, prefix, retTyp)
//	before = append(before, before1...)
//	before = append(before, before2...)
//	// Rename tuple .0 .1 .2 ... to .Arg0 .Arg1 .Arg2 ...
//	switch expr.x.GetType().(type) {
//	case TupleType:
//		content2 = fmt.Sprintf("Arg%s", content2)
//	case *EnumType:
//		out := fmt.Sprintf("Make_%s_%s", content1, content2)
//		if _, ok := expr.GetType().(*EnumType); ok { // TODO
//			out += "()"
//		}
//		return before, out
//	}
//	return before, fmt.Sprintf("%s.%s", content1, content2)
//}
//
//func genBubbleOptionExpr(env *Env, e *BubbleOptionExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	assert(TryCast[OptionType](e.x.GetType()), fmt.Sprintf("BubbleOptionExpr: %v", e.x))
//	if _, ok := retTyp.(ResultType); ok {
//		before1, content1 := genExpr(env, e.x, prefix, retTyp)
//		// TODO: res should be an incrementing tmp numbered variable
//		before := NewBeforeStmt(addPrefix(`res := `+content1+`
//if res.IsNone() {
//	return res
//}
//`, prefix))
//		out := `res.Unwrap()`
//		return append(before1, before), out
//	} else {
//		before1, content1 := genExpr(env, e.x, prefix, retTyp)
//		out := fmt.Sprintf("%s.Unwrap()", content1)
//		return before1, out
//	}
//}

func addPrefix(s, prefix string) string {
	var newArr []string
	arr := strings.Split(s, "\n")
	for i := 0; i < len(arr); i++ {
		line := arr[i]
		if i < len(arr)-1 {
			line = prefix + line
		}
		newArr = append(newArr, line)
	}
	return strings.Join(newArr, "\n")
}

//
//func genBubbleResultExpr(env *Env, e *BubbleResultExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	// TODO: res should be an incrementing tmp numbered variable
//	// Native void unwrapping
//	tmpl1 := `res, err := %s
//if err != nil {
//	panic(err)
//}
//`
//	// Non-native void unwrapping
//	tmpl2 := `res := %s
//if res.IsErr() {
//	return res
//}
//`
//	// Native error propagation
//	tmpl3 := `res, err := %s
//if err != nil {
//	return MakeResultErr[%s](err)
//}
//`
//	// Non-native error propagation
//	tmpl4 := `res := %s
//if res.IsErr() {
//	return res
//}
//`
//
//	// Non-native error propagation in function that returns Option
//	tmpl5 := `res := %s
//if res.IsErr() {
//	return MakeOptionNone[%s]()
//}
//`
//	if TryCast[ResultType](e.x.GetType()) {
//		if TryCast[VoidType](retTyp) && TryCast[FuncType](e.GetType()) && e.GetType().(FuncType).isNative {
//			before1, content1 := genExpr(env, e.x, prefix, retTyp)
//			if e.GetType().(FuncType).isNative {
//				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), prefix))
//				out := `res`
//				return append(before1, before), out
//			}
//			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl2, content1), prefix))
//			out := `res.Unwrap()`
//			return append(before1, before), out
//		}
//
//		if retTypType, ok := retTyp.(ResultType); ok {
//			before1, content1 := genExpr(env, e.x, prefix, retTyp)
//			if e.x.GetType().(ResultType).native {
//				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl3, content1, retTypType.wrappedType.GoStr()), prefix))
//				out := `res`
//				return append(before1, before), out
//			}
//
//			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl4, content1), prefix))
//			out := `res.Unwrap()`
//			return append(before1, before), out
//
//		} else if _, ok := retTyp.(OptionType); ok {
//			before1, content1 := genExpr(env, e.x, prefix, retTyp)
//			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl5, content1, "int"), prefix))
//			out := `res.Unwrap()`
//			return append(before1, before), out
//
//		} else {
//			before1, content1 := genExpr(env, e.x, prefix, retTyp)
//			if e.x.GetType().(ResultType).native {
//				tmpl := tmpl1
//				if _, ok := e.x.GetType().(ResultType).wrappedType.(VoidType); ok {
//					tmpl = "err := %s\nif err != nil {\n\tpanic(err)\n}\n"
//					before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), prefix))
//					out := `AglNoop[struct{}]()`
//					return append(before1, before), out
//				}
//				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), prefix))
//				out := `AglIdentity(res)`
//				return append(before1, before), out
//			}
//
//			out := fmt.Sprintf("%s.Unwrap()", content1)
//			return before1, out
//		}
//	} else {
//		panic("")
//	}
//}

//func genCallExpr(env *Env, e *CallExpr, prefix string, retTyp Typ) ([]IBefore, string) {
//	var before []IBefore
//	switch expr := e.fun.(type) {
//	case *SelectorExpr:
//		if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "filter" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before2, content2 := genExpr(env, e.args[0], prefix, retTyp) // TODO resolve generic parameter to concrete type
//			before = append(before, before1...)
//			before = append(before, before2...)
//			return before, fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
//		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "find" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
//			before = append(before, before1...)
//			before = append(before, before2...)
//			return before, fmt.Sprintf("AglVecFind(%s, %s)", content1, content2)
//		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "map" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
//			before = append(before, before1...)
//			before = append(before, before2...)
//			return before, fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
//		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "reduce" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
//			before3, content3 := genExpr(env, e.args[1], prefix, retTyp)
//			before = append(before, before1...)
//			before = append(before, before2...)
//			before = append(before, before3...)
//			return before, fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
//		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "joined" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
//			before = append(before, before1...)
//			before = append(before, before2...)
//			return before, fmt.Sprintf("AglJoined(%s, %s)", content1, content2)
//		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "sum" {
//			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
//			before = append(before, before1...)
//			return before, fmt.Sprintf("AglVecSum(%s)", content1)
//		}
//		if TryCast[OptionType](expr.x.GetType()) ||
//			TryCast[ResultType](expr.x.GetType()) {
//			if expr.sel.lit == "unwrap" {
//				_, content := genExpr(env, expr.x, prefix, retTyp)
//				return nil, fmt.Sprintf(`%s.Unwrap()`, content)
//			} else if expr.sel.lit == "is_some" {
//				_, content := genExpr(env, expr.x, prefix, retTyp)
//				return nil, fmt.Sprintf(`%s.IsSome()`, content)
//			}
//		}
//	case *IdentExpr:
//		//default:
//		//panic(fmt.Sprintf("unknown function type: %s %v", reflect.TypeOf(expr.fun), expr.fun))
//	}
//	before1, content := genExpr(env, e.fun, prefix, retTyp)
//	before2, content2 := genExprs(env, e.args, prefix, retTyp)
//	before = append(before, before1...)
//	before = append(before, before2...)
//	return before, fmt.Sprintf("%s(%s)", content, content2)
//}

func genCore() string {
	return `
package main

import "cmp"

type AglVoid struct{}

type Option[T any] struct {
	t *T
}

func (o Option[T]) String() string {
	if o.IsNone() {
		return "None"
	}
	return fmt.Sprintf("Some(%v)", *o.t)
}

func (o Option[T]) IsSome() bool {
	return o.t != nil
}

func (o Option[T]) IsNone() bool {
	return o.t == nil
}

func (o Option[T]) Unwrap() T {
	if o.IsNone() {
		panic("unwrap on a None value")
	}
	return *o.t
}

func MakeOptionSome[T any](t T) Option[T] {
	return Option[T]{t: &t}
}

func MakeOptionNone[T any]() Option[T] {
	return Option[T]{t: nil}
}

type Result[T any] struct {
	t *T
	e error
}

func (r Result[T]) String() string {
	if r.IsErr() {
		return fmt.Sprintf("Err(%v)", r.e)
	}
	return fmt.Sprintf("Ok(%v)", *r.t)
}

func (r Result[T]) IsErr() bool {
	return r.e != nil
}

func (r Result[T]) IsOk() bool {
	return r.e == nil
}

func (r Result[T]) Unwrap() T {
	if r.IsErr() {
		panic(fmt.Sprintf("unwrap on an Err value: %s", r.e))
	}
	return *r.t
}

func (r Result[T]) Err() error {
	return r.e
}

func MakeResultOk[T any](t T) Result[T] {
	return Result[T]{t: &t, e: nil}
}

func MakeResultErr[T any](err error) Result[T] {
	return Result[T]{t: nil, e: err}
}

func AglVecMap[T, R any](a []T, f func(T) R) []R {
	var out []R
	for _, v := range a {
		out = append(out, f(v))
	}
	return out
}

func AglVecFilter[T any](a []T, f func(T) bool) []T {
	var out []T
	for _, v := range a {
		if f(v) {
			out = append(out, v)
		}
	}
	return out
}

func AglReduce[T any, R cmp.Ordered](a []T, r R, f func(R, T) R) R {
	var acc R
	for _, v := range a {
		acc = f(acc, v)
	}
	return acc
}

func AglAssert(pred bool, msg ...string) {
	if !pred {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		panic(m)
	}
}

func AglSum[T cmp.Ordered](a []T) (out T) {
	for _, el := range a {
		out += el
	}
	return
}

func AglVecIn[T cmp.Ordered](a []T, v T) bool {
	for _, el := range a {
		if el == v {
			return true
		}
	}
	return false
}

func AglNoop[T any](_ ...T) {}

func AglTypeAssert[T any](v any) Option[T] {
	if v, ok := v.(T); ok {
		return MakeOptionSome(v)
	}
	return MakeOptionNone[T]()
}

func AglIdentity[T any](v T) T { return v }

func AglVecFind[T any](a []T, f func(T) bool) Option[T] {
	for _, v := range a {
		if f(v) {
			return MakeOptionSome(v)
		}
	}
	return MakeOptionNone[T]()
}

func AglJoined(a []string, s string) string {
	return strings.Join(a, s)
}
`
}
