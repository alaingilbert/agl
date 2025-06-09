package main

import (
	"fmt"
	"reflect"
	"strings"
)

func codegen(a *ast) (out string) {
	var before []IBefore
	out += genPackage(a)
	out += genImports(a)
	out += genEnums(a)
	out += genStructs(a)
	before1, content1 := genFunctions(a)
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

func genEnums(a *ast) (out string) {
	for _, s := range a.enums {
		_, content := genStmt(s, "", nil)
		out += content
	}
	return
}

func genStructs(a *ast) (out string) {
	for _, s := range a.structs {
		_, content := genStmt(s, "", nil)
		out += content
	}
	return
}

func genFunctions(a *ast) (before []IBefore, out string) {
	for _, f := range a.funcs {
		before1, content1 := genStmt(f, "", nil)
		before = append(before, before1...)
		out += content1
	}
	return
}

//type Generator struct {
//	buf bufio.Writer
//}
//
//func NewGenerator() *Generator {
//	return &Generator{}
//}
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

func genStmts(stmts []Stmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	for _, stmt := range stmts {
		before1, content1 := genStmt(stmt, prefix, retTyp)
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

func genStmt(stmt Stmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	switch s := stmt.(type) {
	case *EnumStmt:
		return genEnumStmt(s)
	case *structStmt:
		return genStructStmt(s)
	case *funcStmt:
		return genFuncStmt(s)
	case *IfStmt:
		return genIfStmt(s, prefix, retTyp)
	case *ReturnStmt:
		return genReturnStmt(s, prefix, retTyp)
	case *AssignStmt:
		return genAssignStmt(s, prefix, retTyp)
	case *InlineCommentStmt:
		return genInlineCommentStmt(s, prefix, retTyp)
	case *ExprStmt:
		return genExprStmt(s, prefix, retTyp)
	case *ForRangeStmt:
		return genForRangeStmt(s, prefix, retTyp)
	case *IncDecStmt:
		return genIncDecStmt(s, prefix, retTyp)
	case *AssertStmt:
		return genAssertStmt(s, prefix, retTyp)
	case *BlockStmt:
		return genBlockStmt(s, prefix, retTyp)
	default:
		panic(fmt.Sprintf("unknown statement: %s, %v", reflect.TypeOf(s), s))
	}
	return
}

func genExpr(e Expr, prefix string, retTyp Typ) ([]IBefore, string) {
	switch expr := e.(type) {
	case *BinOpExpr:
		return genBinOpExpr(expr, prefix, retTyp)
	case *NumberExpr:
		return genNumberExpr(expr, prefix, retTyp)
	case *IdentExpr:
		return genIdentExpr(expr, prefix, retTyp)
	case *VoidExpr:
		return genVoidExpr(expr, prefix, retTyp)
	case *TrueExpr:
		return genTrueExpr(expr, prefix, retTyp)
	case *FalseExpr:
		return genFalseExpr(expr, prefix, retTyp)
	case *NoneExpr:
		return genNoneExpr(expr, prefix, retTyp)
	case *MemberExpr:
		return genMemberExpr(expr, prefix, retTyp)
	case *ParenExpr:
		return genParenExpr(expr, prefix, retTyp)
	case *StringExpr:
		return genStringExpr(expr, prefix, retTyp)
	case *SomeExpr:
		return genSomeExpr(expr, prefix, retTyp)
	case *OkExpr:
		return genOkExpr(expr, prefix, retTyp)
	case *ErrExpr:
		return genErrExpr(expr, prefix, retTyp)
	case *TupleExpr:
		return genTupleExpr(expr, prefix, retTyp)
	case *CallExpr:
		return genCallExpr(expr, prefix, retTyp)
	case *BubbleOptionExpr:
		return genBubbleOptionExpr(expr, prefix, retTyp)
	case *BubbleResultExpr:
		return genBubbleResultExpr(expr, prefix, retTyp)
	case *SelectorExpr:
		return genSelectorExpr(expr, prefix, retTyp)
	case *VecExpr:
		return genVecExpr(expr, prefix, retTyp)
	case *MakeExpr:
		return genMakeExpr(expr, prefix, retTyp)
	case *MutExpr:
		return genMutExpr(expr, prefix, retTyp)
	case *Field:
		return genField(expr, prefix, retTyp)
	case *AnonFnExpr:
		return genAnonFnExpr(expr, prefix, retTyp)
	case *CompositeLitExpr:
		return genCompositeLit(expr, prefix, retTyp)
	default:
		panic(fmt.Sprintf("unknown expression type: %s %v", reflect.TypeOf(e), expr))
	}
	return nil, ""
}

func genEnumStmt(s *EnumStmt) (before []IBefore, out string) {
	out += fmt.Sprintf("type %s int\n", s.lit)
	out += "const (\n"
	for i, name := range s.fields {
		if i == 0 {
			out += fmt.Sprintf("\tAglEnum_%s_%s %s = iota + 1\n", s.lit, name.lit, s.lit)
		} else {
			out += fmt.Sprintf("\tAglEnum_%s_%s\n", s.lit, name.lit)
		}
	}
	out += ")\n"
	out += `func (c Color) String() string {
	switch c {
`
	for _, name := range s.fields {
		out += fmt.Sprintf("\tcase AglEnum_%s_%s:\n\t\treturn \"%s\"\n", s.lit, name.lit, name.lit)
	}
	out += "\tdefault:\n\t\tpanic(\"\")\n\t}\n}\n"
	return
}

func genStructStmt(s *structStmt) (before []IBefore, out string) {
	var namePrefix string
	if !s.pub {
		namePrefix = "aglPriv"
	}
	var fields string
	for _, f := range s.fields {
		_, ft := genExpr(f, "\t", nil)
		fields += fmt.Sprintf("\t%s\n", ft)
	}
	out += fmt.Sprintf("type %s%s struct {\n%s}\n", namePrefix, s.lit, fields)
	return
}

func genFuncStmt(f *funcStmt) (before []IBefore, out string) {
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
		o += f.out.expr.GetType().GoStr()
		o = Ternary(o != "", " "+o, "")
	}

	// If the function body only has 1 stmt, auto add "return" of the only expression value
	if len(f.stmts) == 1 && !TryCast[*VoidExpr](f.out.expr) && f.out.expr.GetType().GoStr() != "" {
		if e, ok := f.stmts[0].(*ExprStmt); ok {
			f.stmts[0] = &ReturnStmt{expr: e.x}
		}
	}
	before1, stmtsStr := genStmts(f.stmts, "\t", f.out.expr.GetType())
	before = append(before, before1...)
	out += fmt.Sprintf("func %s%s%s(%s)%s {\n%s}\n", recv, f.name, typeParamsStr, strings.Join(args, ", "), o, stmtsStr)
	return
}

func genInlineCommentStmt(stmt *InlineCommentStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	out += prefix + fmt.Sprintf("%s", stmt.lit) + "\n"
	return
}

func genExprStmt(stmt *ExprStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(stmt.x, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + content + "\n"
	return
}

func genIncDecStmt(stmt *IncDecStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(stmt.x, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + content + stmt.tok.lit + "\n"
	return
}

func genForRangeStmt(stmt *ForRangeStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(stmt.id1, prefix, retTyp)
	before2, content2 := genExpr(stmt.id2, prefix, retTyp)
	before3, content3 := genExpr(stmt.expr, prefix, retTyp)
	before4, content4 := genStmts(stmt.stmts, prefix+"\t", retTyp)
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

func genAssignStmt(stmt *AssignStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
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
		before1, content1 := genExpr(stmt.lhs, prefix, retTyp)
		before = append(before, before1...)
		lhs = content1
	}

	// RHS
	before2, content2 := genExpr(stmt.rhs, prefix, retTyp)
	before = append(before, before2...)

	// Output
	out += prefix + fmt.Sprintf("%s %s %s\n", lhs, stmt.tok.lit, content2)
	out += after
	return
}

func genReturnStmt(stmt *ReturnStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content := genExpr(stmt.expr, prefix, retTyp)
	before = append(before, before1...)
	out += prefix + "return " + content + "\n"
	return
}

func genIfStmt(stmt *IfStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(stmt.cond, prefix, retTyp)
	before2, content2 := genStmts(stmt.body, prefix+"\t", retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	out += prefix + "if " + content1 + " {\n"
	out += content2
	if stmt.Else != nil {
		before3, content3 := genStmt(stmt.Else, prefix, retTyp)
		before = append(before, before3...)
		out += prefix + "} else " + strings.TrimSpace(content3) + "\n"
	} else {
		out += prefix + "}\n"
	}
	return
}

func genBlockStmt(stmt *BlockStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genStmts(stmt.stmts, prefix, retTyp)
	before = append(before, before1...)
	out += "{\n"
	if content1 != "" {
		out += prefix + content1
	}
	out += prefix + "}\n"
	return
}

func genAssertStmt(stmt *AssertStmt, prefix string, retTyp Typ) (before []IBefore, out string) {
	before1, content1 := genExpr(stmt.x, prefix, retTyp)
	before = append(before, before1...)
	var content2 string
	if stmt.msg != nil {
		var before2 []IBefore
		before2, content2 = genExpr(stmt.msg, prefix, retTyp)
		before = append(before, before2...)
	}
	aug := fmt.Sprintf(`"assert failed '%s' line %v"`, stmt.x.AglStr(), stmt.Pos().Row)
	if content2 != "" {
		aug += ` + " " + ` + content2
	}
	out += prefix + fmt.Sprintf("AglAssert(%s, %s)", content1, aug) + "\n"
	return
}

func genExprs(e []Expr, prefix string, retTyp Typ) (before []IBefore, out string) {
	var tmp []string
	for _, expr := range e {
		before1, content1 := genExpr(expr, prefix, retTyp)
		before = append(before, before1...)
		tmp = append(tmp, content1)
	}
	return before, strings.Join(tmp, ", ")
}

func genVoidExpr(expr *VoidExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, ""
}

func genTrueExpr(expr *TrueExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "true"
}

func genFalseExpr(expr *FalseExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "false"
}

func genMutExpr(expr *MutExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return genExpr(expr.x, prefix, retTyp)
}

func genField(expr *Field, prefix string, retTyp Typ) ([]IBefore, string) {
	before1, content1 := genExpr(expr.names[0], prefix, retTyp)
	return before1, fmt.Sprintf("%s %s", content1, expr.typeExpr.GetType().GoStr())
}

func genNumberExpr(expr *NumberExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genMemberExpr(expr *MemberExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, "." + expr.name
}

func genParenExpr(expr *ParenExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(expr.subExpr, prefix, retTyp)
	return before, fmt.Sprintf("(%s)", content)
}

func genStringExpr(expr *StringExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genSomeExpr(expr *SomeExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(expr.expr, prefix, retTyp)
	return before, fmt.Sprintf("MakeOptionSome(%s)", content)
}

func genNoneExpr(expr *NoneExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("MakeOptionNone[%s]()", expr.typ.(OptionType).wrappedType.GoStr())
}

func genOkExpr(expr *OkExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(expr.expr, prefix, retTyp)
	return before, fmt.Sprintf("MakeResultOk(%s)", content)
}

func genErrExpr(expr *ErrExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, content := genExpr(expr.expr, prefix, retTyp)
	return before, fmt.Sprintf("MakeResultErr[%s](%s)", expr.typ.(ResultType).wrappedType.GoStr(), content)
}

func genMakeExpr(expr *MakeExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before1, content1 := genExprs(expr.exprs, prefix, retTyp)
	return before1, fmt.Sprintf("make(%s)", content1)
}

func genBinOpExpr(expr *BinOpExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	before1, content1 := genExpr(expr.lhs, prefix, retTyp)
	before2, content2 := genExpr(expr.rhs, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, fmt.Sprintf("%s %s %s", content1, expr.op.lit, content2)
}

func genIdentExpr(expr *IdentExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	if strings.HasPrefix(expr.lit, "$") {
		expr.lit = strings.Replace(expr.lit, "$", "aglArg", 1)
	}
	return nil, fmt.Sprintf("%s", expr.lit)
}

func genCompositeLit(expr *CompositeLitExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	return nil, fmt.Sprintf("%v{}", expr.typ.(*IdentExpr).lit)
}

func genAnonFnExpr(expr *AnonFnExpr, prefix string, retTyp Typ) ([]IBefore, string) {
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
	before1, content1 := genStmts(expr.stmts, prefix+"\t", retTyp)
	return before1, fmt.Sprintf("func(%s)%s {\n%s%s}", argsT, retT, content1, prefix) // TODO
}

func genSelectorExpr(expr *SelectorExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	before1, content1 := genExpr(expr.x, prefix, retTyp)
	before2, content2 := genExpr(expr.sel, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	// Rename tuple .0 .1 .2 ... to .Arg0 .Arg1 .Arg2 ...
	switch expr.x.GetType().(type) {
	case TupleTypeTyp:
		content2 = fmt.Sprintf("Arg%s", content2)
	case *EnumType:
		return before, fmt.Sprintf("AglEnum_%s_%s", content1, content2)
	}
	return before, fmt.Sprintf("%s.%s", content1, content2)
}

func genVecExpr(expr *VecExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	out := fmt.Sprintf("%s", expr.GetType().GoStr())
	if expr.exprs != nil {
		out += "{"
		var tmp []string
		for _, e := range expr.exprs {
			before1, content1 := genExpr(e, prefix, retTyp)
			before = append(before, before1...)
			tmp = append(tmp, content1)
		}
		out += strings.Join(tmp, ", ")
		out += "}"
	}
	return before, out
}

func genTupleExpr(expr *TupleExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	before, _ := genExprs(expr.exprs, prefix, retTyp)
	structName := expr.GetType().(TupleTypeTyp).name
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	for i, x := range expr.exprs {
		structStr += fmt.Sprintf("\tArg%d %s\n", i, x.GetType().GoStr())
	}
	structStr += fmt.Sprintf("}\n")
	before = append(before, NewBeforeFn(structStr)) // TODO Add in public scope (when function output)
	var fields []string
	for i, x := range expr.exprs {
		before1, content1 := genExpr(x, prefix, retTyp)
		before = append(before, before1...)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return before, fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
}

func genBubbleOptionExpr(e *BubbleOptionExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	if TryCast[OptionType](e.x.GetType()) {
		if _, ok := retTyp.(ResultType); ok {
			before1, content1 := genExpr(e.x, prefix, retTyp)
			// TODO: res should be an incrementing tmp numbered variable
			before := NewBeforeStmt(addPrefix(`res := `+content1+`
if res.IsNone() {
	return res
}
`, prefix))
			out := `res.Unwrap()`
			return append(before1, before), out
		} else {
			before1, content1 := genExpr(e.x, prefix, retTyp)
			out := fmt.Sprintf("%s.Unwrap()", content1)
			return before1, out
		}
	} else {
		panic("")
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

func genBubbleResultExpr(e *BubbleResultExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	// TODO: res should be an incrementing tmp numbered variable
	fmt.Println("?", e.x, e.x.GetType())
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
	if TryCast[ResultType](e.x.GetType()) {
		if TryCast[VoidType](retTyp) && TryCast[*FuncType](e.GetType()) && e.GetType().(*FuncType).isNative {
			before1, content1 := genExpr(e.x, prefix, retTyp)
			if e.GetType().(*FuncType).isNative {
				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), prefix))
				out := `res`
				return append(before1, before), out
			}
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl2, content1), prefix))
			out := `res.Unwrap()`
			return append(before1, before), out
		}

		if retTypType, ok := retTyp.(ResultType); ok {
			before1, content1 := genExpr(e.x, prefix, retTyp)
			if e.GetType().(ResultType).native {
				before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl3, content1, retTypType.wrappedType.GoStr()), prefix))
				out := `res`
				return append(before1, before), out
			}

			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl4, content1), prefix))
			out := `res.Unwrap()`
			return append(before1, before), out
		} else {
			before1, content1 := genExpr(e.x, prefix, retTyp)
			out := fmt.Sprintf("%s.Unwrap()", content1)
			return before1, out
		}
	} else {
		panic("")
	}
}

func genCallExpr(e *CallExpr, prefix string, retTyp Typ) ([]IBefore, string) {
	var before []IBefore
	switch expr := e.fun.(type) {
	case *SelectorExpr:
		if TryCast[ArrayTypeTyp](expr.x.GetType()) && expr.sel.lit == "filter" {
			before1, content1 := genExpr(expr.x, prefix, retTyp)
			before2, content2 := genExpr(e.args[0], prefix, retTyp) // TODO resolve generic parameter to concrete type
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
		} else if TryCast[ArrayTypeTyp](expr.x.GetType()) && expr.sel.lit == "map" {
			before1, content1 := genExpr(expr.x, prefix, retTyp)
			before2, content2 := genExpr(e.args[0], prefix, retTyp)
			before = append(before, before1...)
			before = append(before, before2...)
			return before, fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
		} else if TryCast[ArrayTypeTyp](expr.x.GetType()) && expr.sel.lit == "reduce" {
			before1, content1 := genExpr(expr.x, prefix, retTyp)
			before2, content2 := genExpr(e.args[0], prefix, retTyp)
			before3, content3 := genExpr(e.args[1], prefix, retTyp)
			before = append(before, before1...)
			before = append(before, before2...)
			before = append(before, before3...)
			return before, fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
		} else if TryCast[ArrayTypeTyp](expr.x.GetType()) && expr.sel.lit == "sum" {
			before1, content1 := genExpr(expr.x, prefix, retTyp)
			before = append(before, before1...)
			return before, fmt.Sprintf("AglVecSum(%s)", content1)
		}
		if TryCast[OptionType](expr.x.GetType()) ||
			TryCast[ResultType](expr.x.GetType()) {
			if expr.sel.lit == "unwrap" {
				_, content := genExpr(expr.x, prefix, retTyp)
				return nil, fmt.Sprintf(`%s.Unwrap()`, content)
			}
		}
	case *IdentExpr:
		//default:
		//panic(fmt.Sprintf("unknown function type: %s %v", reflect.TypeOf(expr.fun), expr.fun))
	}
	before1, content := genExpr(e.fun, prefix, retTyp)
	before2, content2 := genExprs(e.args, prefix, retTyp)
	before = append(before, before1...)
	before = append(before, before2...)
	return before, fmt.Sprintf("%s(%s)", content, content2)
}

func genCore() string {
	return `
package main

import "cmp"

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
`
}
