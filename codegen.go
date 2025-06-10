package main

import (
	"bufio"
	"fmt"
	"reflect"
	"strings"
)

func codegen(a *ast, env *Env) (out string) {
	var before []IBefore
	out += genPackage(a)
	out += genImports(a)
	out += genEnums(env, a)
	out += genInterface(env, a)
	out += genStructs(env, a)
	before1, content1 := genFunctions(a, env)
	before = append(before, before1...)
	for _, b := range before {
		out += b.Content()
	}
	out += content1
	return
}

func genPackage(a *ast) (out string) {
	if a.packageStmt != nil {
		out += fmt.Sprintf("package %s\n", a.packageStmt.lit)
	}
	return
}

func genImports(a *ast) (out string) {
	for _, i := range a.imports {
		out += fmt.Sprintf("import %s\n", i.lit)
	}
	return
}

func genInterface(env *Env, a *ast) (out string) {
	for _, s := range a.interfaces {
		_, content := genStmt(env, s, "", nil)
		out += content
	}
	return
}

func genEnums(env *Env, a *ast) (out string) {
	for _, s := range a.enums {
		_, content := genStmt(env, s, "", nil)
		out += content
	}
	return
}

func genStructs(env *Env, a *ast) (out string) {
	for _, s := range a.structs {
		_, content := genStmt(env, s, "", nil)
		out += content
	}
	return
}

func genFunctions(a *ast, env *Env) (before []IBefore, out string) {
	for _, f := range a.funcs {
		before1, content1 := genStmt(env, f, "", nil)
		before = append(before, before1...)
		out += content1
	}
	return
}

type Generator struct {
	buf bufio.Writer
	a   *ast
	env *Env
}

func NewGenerator(a *ast, env *Env) *Generator {
	return &Generator{a: a, env: env}
}

func (g *Generator) Generate() string {
	return codegen(g.a, g.env)
}

//
//func (g *Generator) GenStmts(stmts []Stmt) {
//	for _, stmt := range stmts {
//		before, content := genStmt(stmt, prefix, retTyp)
//		_, _ = g.buf.WriteString(strings.Join(before, ""))
//		_, _ = g.buf.WriteString(content)
//	}
//	return
//}

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

func genStmts(env *Env, stmts []Stmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	for _, stmt := range stmts {
		before1, content1 := genStmt(env, stmt, prefix, retTyp)
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
	return
}

func genStmt(env *Env, stmt Stmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	switch s := stmt.(type) {
	case *EnumStmt:
		return genEnumStmt(env, s)
	case *structStmt:
		return genStructStmt(env, s)
	case *FuncExpr:
		return genFuncStmt(env, s)
	case *IfStmt:
		return genIfStmt(env, s, prefix, retTyp)
	case *ReturnStmt:
		return genReturnStmt(env, s, prefix, retTyp)
	case *AssignStmt:
		return genAssignStmt(env, s, prefix, retTyp)
	case *InlineCommentStmt:
		return genInlineCommentStmt(env, s, prefix, retTyp)
	case *ExprStmt:
		return genExprStmt(env, s, prefix, retTyp)
	case *ForRangeStmt:
		return genForRangeStmt(env, s, prefix, retTyp)
	case *IncDecStmt:
		return genIncDecStmt(env, s, prefix, retTyp)
	case *AssertStmt:
		return genAssertStmt(env, s, prefix, retTyp)
	case *BlockStmt:
		return genBlockStmt(env, s, prefix, retTyp)
	case *InterfaceStmt:
		return genInterfaceStmt(env, s, prefix, retTyp)
	default:
		panic(fmt.Sprintf("unknown statement: %s, %v", reflect.TypeOf(s), s))
	}
	return
}

func genExpr(env *Env, e Expr, prefix string, retTyp Typ) ([]IBefore, string) {
	switch expr := e.(type) {
	case *BinOpExpr:
		return genBinOpExpr(env, expr, prefix, retTyp)
	case *NumberExpr:
		return genNumberExpr(env, expr, prefix, retTyp)
	case *IdentExpr:
		return genIdentExpr(env, expr, prefix, retTyp)
	case *VoidExpr:
		return genVoidExpr(env, expr, prefix, retTyp)
	case *TrueExpr:
		return genTrueExpr(env, expr, prefix, retTyp)
	case *FalseExpr:
		return genFalseExpr(env, expr, prefix, retTyp)
	case *NoneExpr:
		return genNoneExpr(env, expr, prefix, retTyp)
	case *MemberExpr:
		return genMemberExpr(env, expr, prefix, retTyp)
	case *ParenExpr:
		return genParenExpr(env, expr, prefix, retTyp)
	case *StringExpr:
		return genStringExpr(env, expr, prefix, retTyp)
	case *SomeExpr:
		return genSomeExpr(env, expr, prefix, retTyp)
	case *OkExpr:
		return genOkExpr(env, expr, prefix, retTyp)
	case *ErrExpr:
		return genErrExpr(env, expr, prefix, retTyp)
	case *TupleExpr:
		return genTupleExpr(env, expr, prefix, retTyp)
	case *CallExpr:
		return genCallExpr(env, expr, prefix, retTyp)
	case *BubbleOptionExpr:
		return genBubbleOptionExpr(env, expr, prefix, retTyp)
	case *BubbleResultExpr:
		return genBubbleResultExpr(env, expr, prefix, retTyp)
	case *SelectorExpr:
		return genSelectorExpr(env, expr, prefix, retTyp)
	case *VecExpr:
		return genVecExpr(env, expr, prefix, retTyp)
	case *MakeExpr:
		return genMakeExpr(env, expr, prefix, retTyp)
	case *MutExpr:
		return genMutExpr(env, expr, prefix, retTyp)
	case *Field:
		return genField(env, expr, prefix, retTyp)
	case *AnonFnExpr:
		return genAnonFnExpr(env, expr, prefix, retTyp)
	case *CompositeLitExpr:
		return genCompositeLit(env, expr, prefix, retTyp)
	case *KeyValueExpr:
		return genKeyValueExpr(env, expr, prefix, retTyp)
	case *TypeAssertExpr:
		return genTypeAssertExpr(env, expr, prefix, retTyp)
	default:
		panic(fmt.Sprintf("unknown expression type: %s %v", reflect.TypeOf(e), expr))
	}
	return nil, ""
}

func genEnumStmt(env *Env, s *EnumStmt) (before []IBefore, out string) {
	out += fmt.Sprintf("type %sTag int\n", s.lit)
	out += "const (\n"
	for i, field := range s.fields {
		if i == 0 {
			out += fmt.Sprintf("\t%s_%s %sTag = iota + 1\n", s.lit, field.name.lit, s.lit)
		} else {
			out += fmt.Sprintf("\t%s_%s\n", s.lit, field.name.lit)
		}
	}
	out += ")\n"
	out += fmt.Sprintf("type %s struct {\n", s.lit)
	out += fmt.Sprintf("\ttag %sTag\n", s.lit)
	for _, field := range s.fields {
		for i, el := range field.elts {
			out += fmt.Sprintf("\t%s%d %s\n", field.name.lit, i, el.GetType().GoStr())
		}
	}
	out += "}\n"
	out += fmt.Sprintf("func (v %s) String() string {\n\tswitch v.tag {\n", s.lit)
	for _, field := range s.fields {
		out += fmt.Sprintf("\tcase %s_%s:\n\t\treturn \"%s\"\n", s.lit, field.name.lit, field.name.lit)
	}
	out += "\tdefault:\n\t\tpanic(\"\")\n\t}\n}\n"
	for _, field := range s.fields {
		var tmp []string
		var tmp1 []string
		for i, el := range field.elts {
			tmp = append(tmp, fmt.Sprintf("arg%d %s", i, el.GetType().GoStr()))
			tmp1 = append(tmp1, fmt.Sprintf("%s%d: arg%d", field.name.lit, i, i))
		}
		var tmp1Out string
		if len(tmp1) > 0 {
			tmp1Out = ", " + strings.Join(tmp1, ", ")
		}
		out += fmt.Sprintf("func Make_%s_%s(%s) %s {\n\treturn %s{tag: %s_%s%s}\n}\n",
			s.lit, field.name.lit, strings.Join(tmp, ", "), s.lit, s.lit, s.lit, field.name.lit, tmp1Out)
	}
	return
}

func genStructStmt(env *Env, s *structStmt) (before []IBefore, out string) {
	var namePrefix string
	if !s.pub {
		namePrefix = "aglPriv"
	}
	var fields string
	for _, f := range s.fields {
		_, ft := genExpr(env, f, "\t", nil)
		fields += fmt.Sprintf("\t%s\n", ft)
	}
	out += fmt.Sprintf("type %s%s struct {\n%s}\n", namePrefix, s.lit, fields)
	return
}

func genFuncStmt(env *Env, f *FuncExpr) (before []IBefore, out string) {
	var recv string
	if f.recv != nil {
		var args1 []string
		for _, e := range f.recv.list {
			var tmp []string
			for _, name := range e.names {
				tmp = append(tmp, name.lit)
			}
			args1 = append(args1, strings.Join(tmp, ", ")+" "+e.typeExpr.GetType().GoStr())
		}
		recv = strings.Join(args1, ", ")
		if recv != "" {
			recv = "(" + recv + ") "
		}
	}

	var typeParamsStr string
	if f.typeParams != nil {
		var args1 []string
		for _, e := range f.typeParams.list {
			var tmp []string
			for _, name := range e.names {
				tmp = append(tmp, name.lit)
			}
			args1 = append(args1, strings.Join(tmp, ", ")+" "+e.typeExpr.GetType().(*GenericType).constraints[0].GoStr())
		}
		typeParamsStr = strings.Join(args1, ", ")
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}

	var args []string
	for _, arg := range f.args.list {
		var tmp []string
		for _, name := range arg.names {
			tmp = append(tmp, name.lit)
		}
		argT := arg.typeExpr.GetType().GoStr()
		if TryCast[*EllipsisExpr](arg.typeExpr) {
			argT = "..." + argT
		}
		args = append(args, strings.Join(tmp, ", ")+" "+argT)
	}
	var o string
	if f.out.expr != nil {
		if _, ok := f.out.expr.GetType().(VoidType); !ok {
			o += f.out.expr.GetType().GoStr()
			o = Ternary(o != "", " "+o, "")
		}
	}

	// If the function body only has 1 stmt, auto add "return" of the only expression value
	if len(f.stmts) == 1 && !TryCast[*VoidExpr](f.out.expr) && f.out.expr.GetType().GoStr() != "" {
		if e, ok := f.stmts[0].(*ExprStmt); ok {
			f.stmts[0] = &ReturnStmt{expr: e.x}
		}
	}
	before1, stmtsStr := genStmts(env, f.stmts, "\t", f.out.expr.GetType())
	before = append(before, before1...)
	name := f.name
	if newName, ok := overloadMapping[name]; ok {
		name = newName
	}

	out += fmt.Sprintf("func %s%s%s(%s)%s {\n%s}\n", recv, name, typeParamsStr, strings.Join(args, ", "), o, stmtsStr)
	return
}

func genInlineCommentStmt(env *Env, stmt *InlineCommentStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	out += prefix + fmt.Sprintf("%s", stmt.lit) + "\n"
	return
}

func genExprStmt(env *Env, stmt *ExprStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(env, stmt.x, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + content + "\n"
	return
}

func genIncDecStmt(env *Env, stmt *IncDecStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(env, stmt.x, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + content + stmt.tok.lit + "\n"
	return
}

func genForRangeStmt(env *Env, stmt *ForRangeStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(env, stmt.id1, prefix, retTyp)
	before2, content2 := genExpr(env, stmt.id2, prefix, retTyp)
	before3, content3 := genExpr(env, stmt.expr, prefix, retTyp)
	before4, content4 := genStmts(env, stmt.stmts, prefix+"\t", retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	before = append(before, before3...)
	before = append(before, before4...)
	out += prefix + fmt.Sprintf("for %s, %s := range %s {\n%s%s}\n",
		content1,
		content2,
		content3,
		content4,
		prefix)
	return
}

func genAssignStmt(env *Env, stmt *AssignStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	// LHS
	var lhs string
	var after string
	if TryCast[*TupleExpr](stmt.lhs) {
		lhs = AglVariablePrefix + "1"
		var names []string
		var exprs []string
		for i, x := range stmt.lhs.(*TupleExpr).exprs {
			names = append(names, x.(*IdentExpr).lit)
			exprs = append(exprs, fmt.Sprintf("%s.Arg%d", lhs, i))
		}
		after = prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
	} else {
		before1, content1 := genExpr(env, stmt.lhs, prefix, retTyp)
		before = append(before, before1...)
		lhs = content1
	}

	// RHS
	before2, content2 := genExpr(env, stmt.rhs, prefix, retTyp)
	before = append(before, before2...)

	// Output
	out += prefix + fmt.Sprintf("%s %s %s\n", lhs, stmt.tok.lit, content2)
	out += after
	return
}

func genReturnStmt(env *Env, stmt *ReturnStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(env, stmt.expr, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + "return " + content + "\n"
	return
}

func genIfStmt(env *Env, stmt *IfStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(env, stmt.cond, prefix, retTyp)
	before2, content2 := genStmts(env, stmt.body, prefix+"\t", retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	out += prefix + "if " + content1 + " {\n"
	out += content2
	if stmt.Else != nil {
		before3, content3 := genStmt(env, stmt.Else, prefix, retTyp)
		before = append(before, before3...)
		out += prefix + "} else " + strings.TrimSpace(content3) + "\n"
	} else {
		out += prefix + "}\n"
	}
	return
}

func genInterfaceStmt(env *Env, stmt *InterfaceStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	out += fmt.Sprintf("type %s interface {\n}\n", stmt.lit)
	return
}

func genBlockStmt(env *Env, stmt *BlockStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genStmts(env, stmt.stmts, prefix, retTyp)
	before = append(before, before1...)
	out += "{\n"
	if content1 != "" {
		out += prefix + content1
	}
	out += prefix + "}\n"
	return
}

func genAssertStmt(env *Env, stmt *AssertStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(env, stmt.x, prefix, retTyp)
	before = append(before, before1...)
	var content2 string
	if stmt.msg != nil {
		var before2 []IBefore
		before2, content2 = genExpr(env, stmt.msg, prefix, retTyp)
		before = append(before, before2...)
	}
	aug := fmt.Sprintf(`"assert failed '%s' line %v"`, stmt.x.AglStr(), stmt.Pos().Row)
	if content2 != "" {
		aug += ` + " " + ` + content2
	}
	out += prefix + fmt.Sprintf("AglAssert(%s, %s)", content1, aug) + "\n"
	return
}

func genExprs(env *Env, e []Expr, prefix string, retTyp Typ) (before []IBefore, out string) {
	var tmp []string
	for _, expr := range e {
		before1, content1 := genExpr(env, expr, prefix, retTyp)
		before = append(before, before1...)
		tmp = append(tmp, content1)
	}
	return before, strings.Join(tmp, ", ")
}

func genVoidExpr(env *Env, expr *VoidExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, ""
}

func genTrueExpr(env *Env, expr *TrueExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "true"
}

func genFalseExpr(env *Env, expr *FalseExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "false"
}

func genMutExpr(env *Env, expr *MutExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return genExpr(env, expr.x, prefix, retTyp)
}

func genField(env *Env, expr *Field, prefix string, retTyp Typ) ([]IBefore, string) {
	before1, content1 := genExpr(env, expr.names[0], prefix, retTyp)
	return before1, fmt.Sprintf("%s %s", content1, expr.typeExpr.GetType().GoStr())
}

func genNumberExpr(env *Env, expr *NumberExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genMemberExpr(env *Env, expr *MemberExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "." + expr.name
}

func genParenExpr(env *Env, expr *ParenExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(env, expr.subExpr, prefix, retTyp)
	return before, fmt.Sprintf("(%s)", content)
}

func genStringExpr(env *Env, expr *StringExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genSomeExpr(env *Env, expr *SomeExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(env, expr.expr, prefix, retTyp)
	return before, fmt.Sprintf("MakeOptionSome(%s)", content)
}

func genNoneExpr(env *Env, expr *NoneExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("MakeOptionNone[%s]()", expr.typ.(OptionType).wrappedType.GoStr())
}

func genOkExpr(env *Env, expr *OkExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(env, expr.expr, prefix, retTyp)
	return before, fmt.Sprintf("MakeResultOk(%s)", content)
}

func genErrExpr(env *Env, expr *ErrExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	newExpr := expr.expr
	if v, ok := expr.expr.(*StringExpr); ok {
		newExpr = &CallExpr{fun: &SelectorExpr{x: &IdentExpr{lit: "errors"}, sel: &IdentExpr{lit: "New"}}, args: []Expr{v}}
	}
	before, content := genExpr(env, newExpr, prefix, retTyp)
	return before, fmt.Sprintf("MakeResultErr[%s](%s)", expr.typ.(*ResultType).wrappedType.GoStr(), content)
}

func genMakeExpr(env *Env, expr *MakeExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before1, content1 := genExprs(env, expr.exprs, prefix, retTyp)
	return before1, fmt.Sprintf("make(%s)", content1)
}

func genBinOpExpr(env *Env, expr *BinOpExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	op := expr.op.lit
	before1, content1 := genExpr(env, expr.lhs, prefix, retTyp)
	before2, content2 := genExpr(env, expr.rhs, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	if op == "in" {
		return before, fmt.Sprintf("AglVecIn(%s, %s)", content2, content1)
	}
	if expr.lhs.GetType() != nil && expr.rhs.GetType() != nil {
		if TryCast[*StructType](expr.lhs.GetType()) && TryCast[*StructType](expr.rhs.GetType()) {
			lhsName := expr.lhs.GetType().(*StructType).name
			rhsName := expr.rhs.GetType().(*StructType).name
			if lhsName == rhsName {
				if (op == "==" || op == "!=") && env.Get(lhsName+".__EQL") != nil {
					if op == "==" {
						return before, fmt.Sprintf("%s.__EQL(%s)", content1, content2)
					} else {
						return before, fmt.Sprintf("!%s.__EQL(%s)", content1, content2)
					}
				}
			}
		}
	}
	return before, fmt.Sprintf("%s %s %s", content1, op, content2)
}

func genIdentExpr(env *Env, expr *IdentExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	if strings.HasPrefix(expr.lit, "$") {
		expr.lit = strings.Replace(expr.lit, "$", "aglArg", 1)
	}
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genTypeAssertExpr(env *Env, expr *TypeAssertExpr, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.x, prefix, retTyp)
	before2, content2 := genExpr(env, expr.typ, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	out += fmt.Sprintf("AglTypeAssert[%s](%s)", content2, content1)
	return
}

func genKeyValueExpr(env *Env, expr *KeyValueExpr, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.key, prefix, retTyp)
	before2, content2 := genExpr(env, expr.value, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	out += fmt.Sprintf("%s: %s", content1, content2)
	return
}

func genCompositeLit(env *Env, expr *CompositeLitExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var tmp []string
	var before []IBefore
	for _, elt := range expr.elts {
		before1, content1 := genExpr(env, elt, prefix, retTyp)
		before = append(before, before1...)
		tmp = append(tmp, content1)
	}
	return before, fmt.Sprintf("%v{%s}", expr.typ.(*IdentExpr).lit, strings.Join(tmp, ", "))
}

func genAnonFnExpr(env *Env, expr *AnonFnExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	t := expr.typ.(*FuncType)
	var argsT string
	if len(t.params) > 0 {
		var tmp []string
		for i, arg := range t.params {
			tmp = append(tmp, fmt.Sprintf("aglArg%d %s", i, arg.GoStr()))
		}
		argsT = strings.Join(tmp, ", ")
	}
	var retT string
	if t.ret != nil {
		retT = " " + t.ret.GoStr()
	}

	// If the function body only has 1 stmt, auto add "return" of the only expression value
	if len(expr.stmts) == 1 && t.ret != nil && t.ret.GoStr() != "" {
		if e, ok := expr.stmts[0].(*ExprStmt); ok {
			expr.stmts[0] = &ReturnStmt{expr: e.x}
		}
	}
	before1, content1 := genStmts(env, expr.stmts, prefix+"\t", retTyp)
	return before1, fmt.Sprintf("func(%s)%s {\n%s%s}", argsT, retT, content1, prefix) // TODO
}

func genSelectorExpr(env *Env, expr *SelectorExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	before1, content1 := genExpr(env, expr.x, prefix, retTyp)
	before2, content2 := genExpr(env, expr.sel, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	// Rename tuple .0 .1 .2 ... to .Arg0 .Arg1 .Arg2 ...
	switch expr.x.GetType().(type) {
	case TupleType:
		content2 = fmt.Sprintf("Arg%s", content2)
	case *EnumType:
		out := fmt.Sprintf("Make_%s_%s", content1, content2)
		if _, ok := expr.GetType().(*EnumType); ok { // TODO
			out += "()"
		}
		return before, out
	}
	return before, fmt.Sprintf("%s.%s", content1, content2)
}

func genVecExpr(env *Env, expr *VecExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	out := fmt.Sprintf("%s", expr.GetType().GoStr())
	if expr.exprs != nil {
		out += "{"
		var tmp []string
		for _, e := range expr.exprs {
			before1, content1 := genExpr(env, e, prefix, retTyp)
			before = append(before, before1...)
			tmp = append(tmp, content1)
		}
		out += strings.Join(tmp, ", ")
		out += "}"
	}
	return before, out
}

func genTupleExpr(env *Env, expr *TupleExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, _ := genExprs(env, expr.exprs, prefix, retTyp)
	structName := expr.GetType().(TupleType).name
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	for i, x := range expr.exprs {
		structStr += fmt.Sprintf("\tArg%d %s\n", i, x.GetType().GoStr())
	}
	structStr += fmt.Sprintf("}\n")
	before = append(before, NewBeforeFn(structStr)) // TODO Add in public scope (when function output)
	var fields []string
	for i, x := range expr.exprs {
		before1, content1 := genExpr(env, x, prefix, retTyp)
		before = append(before, before1...)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return before, fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
}

func genBubbleOptionExpr(env *Env, e *BubbleOptionExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	if TryCast[OptionType](e.x.GetType()) {
		if _, ok := retTyp.(*ResultType); ok {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			// TODO: res should be an incrementing tmp numbered variable
			before := NewBeforeStmt(addPrefix(`res := `+content1+`
if res.IsNone() {
	return res
}
`, prefix))
			out := `res.Unwrap()`
			return append(before1, before), out
		} else {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			out := fmt.Sprintf("%s.Unwrap()", content1)
			return before1, out
		}
	} else {
		panic(fmt.Sprintf("BubbleOptionExpr: %v", e.x))
	}
}

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

func genBubbleResultExpr(env *Env, e *BubbleResultExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	// TODO: res should be an incrementing tmp numbered variable
	// Native void unwrapping
	tmpl1 := `res, err := %s
if err != nil {
	panic(err)
}
`
	// Non-native void unwrapping
	tmpl2 := `res := %s
if res.IsErr() {
	return res
}
`
	// Native error propagation
	tmpl3 := `res, err := %s
if err != nil {
	return MakeResultErr[%s](err)
}
`
	// Non-native error propagation
	tmpl4 := `res := %s
if res.IsErr() {
	return res
}
`

	// Non-native error propagation in function that returns Option
	tmpl5 := `res := %s
if res.IsErr() {
	return MakeOptionNone[%s]()
}
`
	if TryCast[*ResultType](e.x.GetType()) {
		if TryCast[VoidType](retTyp) && TryCast[*FuncType](e.GetType()) && e.GetType().(*FuncType).isNative {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			if e.GetType().(*FuncType).isNative {
				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), prefix))
				out := `res`
				return append(before1, before), out
			}
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl2, content1), prefix))
			out := `res.Unwrap()`
			return append(before1, before), out
		}

		if retTypType, ok := retTyp.(*ResultType); ok {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			if e.GetType().(*ResultType).native {
				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl3, content1, retTypType.wrappedType.GoStr()), prefix))
				out := `res`
				return append(before1, before), out
			}

			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl4, content1), prefix))
			out := `res.Unwrap()`
			return append(before1, before), out

		} else if _, ok := retTyp.(OptionType); ok {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl5, content1, "int"), prefix))
			out := `res.Unwrap()`
			return append(before1, before), out

		} else {
			before1, content1 := genExpr(env, e.x, prefix, retTyp)
			if e.x.GetType().(*ResultType).native {
				tmpl := tmpl1
				if _, ok := e.GetType().(*ResultType).wrappedType.(VoidType); ok {
					tmpl = "err := %s\nif err != nil {\n\tpanic(err)\n}\n"
					before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), prefix))
					out := `AglNoop[struct{}]()`
					return append(before1, before), out
				}
				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), prefix))
				out := `AglIdentity(res)`
				return append(before1, before), out
			}

			out := fmt.Sprintf("%s.Unwrap()", content1)
			return before1, out
		}
	} else {
		panic("")
	}
}

func genCallExpr(env *Env, e *CallExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	switch expr := e.fun.(type) {
	case *SelectorExpr:
		if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "filter" {
			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
			before2, content2 := genExpr(env, e.args[0], prefix, retTyp) // TODO resolve generic parameter to concrete type
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "map" {
			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "reduce" {
			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
			before2, content2 := genExpr(env, e.args[0], prefix, retTyp)
			before3, content3 := genExpr(env, e.args[1], prefix, retTyp)
			before = append(before, before1...)
			before = append(before, before2...)
			before = append(before, before3...)
			return before, fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
		} else if TryCast[ArrayType](expr.x.GetType()) && expr.sel.lit == "sum" {
			before1, content1 := genExpr(env, expr.x, prefix, retTyp)
			before = append(before, before1...)
			return before, fmt.Sprintf("AglVecSum(%s)", content1)
		}
		if TryCast[OptionType](expr.x.GetType()) ||
			TryCast[*ResultType](expr.x.GetType()) {
			if expr.sel.lit == "unwrap" {
				_, content := genExpr(env, expr.x, prefix, retTyp)
				return nil, fmt.Sprintf(`%s.Unwrap()`, content)
			} else if expr.sel.lit == "is_some" {
				_, content := genExpr(env, expr.x, prefix, retTyp)
				return nil, fmt.Sprintf(`%s.IsSome()`, content)
			}
		}
	case *IdentExpr:
		//default:
		//panic(fmt.Sprintf("unknown function type: %s %v", reflect.TypeOf(expr.fun), expr.fun))
	}
	before1, content := genExpr(env, e.fun, prefix, retTyp)
	before2, content2 := genExprs(env, e.args, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, fmt.Sprintf("%s(%s)", content, content2)
}

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
`
}
