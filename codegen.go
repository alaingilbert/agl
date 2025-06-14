package main

import (
	goast "agl/ast"
	"agl/token"
	"agl/types"
	"fmt"
	"strings"
)

type Generator struct {
	env    *Env
	a      *goast.File
	prefix string
	before []IBefore
}

func NewGenerator(env *Env, a *goast.File) *Generator {
	return &Generator{env: env, a: a}
}

func (g *Generator) Generate() (out string) {
	out1 := g.genPackage()
	out2 := g.genImports()
	out3 := g.genDecls()
	for _, b := range g.before {
		out += b.Content()
	}
	return out + out1 + out2 + out3
}

func (g *Generator) genPackage() string {
	return fmt.Sprintf("package %s\n", g.a.Name.Name)
}

func (g *Generator) genImports() (out string) {
	for _, spec := range g.a.Imports {
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
	case *goast.StructType:
		return genStructType(env, expr, prefix)
	case *goast.FuncLit:
		return genFuncLit(env, expr, prefix)
	case *goast.ParenExpr:
		return genParenExpr(env, expr, prefix)
	case *goast.Ellipsis:
		return genEllipsis(env, expr, prefix)
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
	//case types.BoolType:
	//return nil, Ternary(typ.V, "true", "false")
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
	if expr.Name == "make" {
		return nil, "make"
	}
	if expr.Name == "None" {
		return nil, fmt.Sprintf("MakeOptionNone[%s]()", t.(types.OptionType).W.GoStr())
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

func genEllipsis(env *Env, expr *goast.Ellipsis, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.Elt, prefix+"\t")
	before = append(before, before1...)
	out += "..." + content1
	return
}

func genParenExpr(env *Env, expr *goast.ParenExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.X, prefix+"\t")
	before = append(before, before1...)
	out += "(" + content1 + ")"
	return
}

func genFuncLit(env *Env, expr *goast.FuncLit, prefix string) (before []IBefore, out string) {
	before1, content1 := genStmt(env, expr.Body, prefix+"\t")
	before2, content2 := genFuncType(env, expr.Type, prefix)
	before = append(before, before1...)
	before = append(before, before2...)
	out += content2 + " {\n"
	out += content1
	out += prefix + "}"
	return
}

func genStructType(env *Env, expr *goast.StructType, prefix string) (before []IBefore, out string) {
	out += prefix + "struct {\n"
	for _, field := range expr.Fields.List {
		before1, content1 := genExpr(env, field.Type, prefix)
		before = append(before, before1...)
		var namesArr []string
		for _, name := range field.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += prefix + "\t" + strings.Join(namesArr, ", ") + " " + content1 + "\n"
	}
	out += prefix + "}"
	return
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
	name := expr.Sel.Name
	switch env.GetType(expr.X).(type) {
	case types.TupleType:
		name = fmt.Sprintf("Arg%s", name)
	}
	return before1, fmt.Sprintf("%s.%s", content1, name)
}

func genBubbleOptionExpr(env *Env, expr *goast.BubbleOptionExpr, prefix string) (before []IBefore, out string) {
	before1, content1 := genExpr(env, expr.X, prefix)
	out += fmt.Sprintf("%s.Unwrap()", content1)
	return before1, out
}

func genBubbleResultExpr(env *Env, expr *goast.BubbleResultExpr, prefix string) (before []IBefore, out string) {
	exprXT := MustCast[types.ResultType](env.GetType(expr.X))
	if exprXT.Bubble {
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
		if exprXT.Native {
			before1, content1 := genExpr(env, expr.X, prefix)
			before1 = append(before, before1...)
			tmpl1 := "res, err := %s\nif err != nil {\n\tpanic(err)\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), prefix))
			out := `AglIdentity(res)`
			return append(before1, before), out
		} else {
			before1, content1 := genExpr(env, expr.X, prefix)
			before1 = append(before, before1...)
			out += fmt.Sprintf("%s.Unwrap()", content1)
		}
	}
	return before, out
}

func genCallExpr(env *Env, expr *goast.CallExpr, prefix string) (before []IBefore, out string) {
	switch e := expr.Fun.(type) {
	case *goast.SelectorExpr:
		if _, ok := env.GetType(e.X).(types.ArrayType); ok {
			if e.Sel.Name == "Filter" {
				before1, content1 := genExpr(env, e.X, prefix)
				before2, content2 := genExpr(env, expr.Args[0], prefix)
				before = append(before, before1...)
				before = append(before, before2...)
				return before, fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Map" {
				before1, content1 := genExpr(env, e.X, prefix)
				before2, content2 := genExpr(env, expr.Args[0], prefix)
				before = append(before, before1...)
				before = append(before, before2...)
				return before, fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Reduce" {
				before1, content1 := genExpr(env, e.X, prefix)
				before2, content2 := genExpr(env, expr.Args[0], prefix)
				before3, content3 := genExpr(env, expr.Args[1], prefix)
				before = append(before, before1...)
				before = append(before, before2...)
				before = append(before, before3...)
				return before, fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
			} else if e.Sel.Name == "Find" {
				before1, content1 := genExpr(env, e.X, prefix)
				before2, content2 := genExpr(env, expr.Args[0], prefix)
				before = append(before, before1...)
				before = append(before, before2...)
				return before, fmt.Sprintf("AglVecFind(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Sum" {
				before1, content1 := genExpr(env, e.X, prefix)
				before = append(before, before1...)
				return before, fmt.Sprintf("AglVecSum(%s)", content1)
			} else if e.Sel.Name == "Joined" {
				before1, content1 := genExpr(env, e.X, prefix)
				before2, content2 := genExpr(env, expr.Args[0], prefix)
				before = append(before, before1...)
				before = append(before, before2...)
				return before, fmt.Sprintf("AglJoined(%s, %s)", content1, content2)
			}
		}
	case *goast.Ident:
		if e.Name == "assert" {
			var contents []string
			for _, arg := range expr.Args {
				before1, content1 := genExpr(env, arg, prefix)
				before = append(before, before1...)
				contents = append(contents, content1)
			}
			line := env.fset.Position(expr.Pos()).Line
			msg := fmt.Sprintf(`"assert failed line %d"`, line)
			if len(contents) == 1 {
				contents = append(contents, msg)
			} else {
				contents[1] = msg + ` + " " + ` + contents[1]
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
	before, _ = genExprs(env, expr.Values, prefix)
	structName := env.GetType(expr).(types.TupleType).Name
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	for i, x := range expr.Values {
		structStr += fmt.Sprintf("\tArg%d %s\n", i, env.GetType(x).GoStr())
	}
	structStr += fmt.Sprintf("}\n")
	before = append(before, NewBeforeFn(structStr)) // TODO Add in public scope (when function output)
	var fields []string
	for i, x := range expr.Values {
		before1, content1 := genExpr(env, x, prefix)
		before = append(before, before1...)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return before, fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
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
		return genIncDecStmt(env, stmt, prefix)
	case *goast.DeclStmt:
		return genDeclStmt(env, stmt, prefix)
	default:
		panic(fmt.Sprintf("%v %v", s, to(s)))
	}
}

func genBlockStmt(env *Env, stmt *goast.BlockStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genStmts(env, stmt.List, prefix)
	before = append(before, before1...)
	out += content1
	return
}

func genSpecs(env *Env, specs []goast.Spec, prefix string) (before []IBefore, out string) {
	for _, spec := range specs {
		before1, content1 := genSpec(env, spec, prefix)
		before = append(before, before1...)
		out += content1
	}
	return
}

func genSpec(env *Env, s goast.Spec, prefix string) (before []IBefore, out string) {
	switch spec := s.(type) {
	case *goast.ValueSpec:
		before1, content1 := genExpr(env, spec.Type, prefix)
		before = append(before, before1...)
		var namesArr []string
		for _, name := range spec.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += prefix + "var " + strings.Join(namesArr, ", ") + " " + content1 + "\n"
	case *goast.TypeSpec:
		before1, content1 := genExpr(env, spec.Type, prefix)
		before = append(before, before1...)
		out += prefix + "type " + spec.Name.Name + " " + content1 + "\n"
	case *goast.ImportSpec:
		if spec.Name != nil {
			out += "import " + spec.Name.Name + "\n"
		}
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
	return
}

func genDecl(env *Env, d goast.Decl, prefix string) (before []IBefore, out string) {
	switch decl := d.(type) {
	case *goast.GenDecl:
		before1, content1 := genGenDecl(env, decl, prefix)
		before = append(before, before1...)
		out += content1
		return
	case *goast.FuncDecl:
		before1, out1 := genFuncDecl(env, decl, prefix)
		for _, b := range before1 {
			out += b.Content()
		}
		out += out1 + "\n"
		return
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
	return
}

func genGenDecl(env *Env, decl *goast.GenDecl, prefix string) (before []IBefore, out string) {
	before1, content1 := genSpecs(env, decl.Specs, prefix)
	before = append(before, before1...)
	out += content1
	return
}

func genDeclStmt(env *Env, stmt *goast.DeclStmt, prefix string) (before []IBefore, out string) {
	before1, content1 := genDecl(env, stmt.Decl, prefix)
	before = append(before, before1...)
	out += content1
	return
}

func genIncDecStmt(env *Env, stmt *goast.IncDecStmt, prefix string) (before []IBefore, out string) {
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
	var lhs, after string
	if len(stmt.Rhs) == 1 && TryCast[types.TupleType](env.GetType(stmt.Rhs[0])) {
		rhs := stmt.Rhs[0]
		lhs = "aglVar1"
		var names []string
		var exprs []string
		if len(stmt.Lhs) == 1 {
			before1, content1 := genExprs(env, stmt.Lhs, prefix)
			before = append(before, before1...)
			lhs = content1
		} else {
			for i := range env.GetType(rhs).(types.TupleType).Elts {
				name := stmt.Lhs[i].(*goast.Ident).Name
				names = append(names, name)
				exprs = append(exprs, fmt.Sprintf("%s.Arg%d", lhs, i))
			}
			after = prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
		}
	} else {
		before1, content1 := genExprs(env, stmt.Lhs, prefix)
		before = append(before, before1...)
		lhs = content1
	}
	before2, content2 := genExprs(env, stmt.Rhs, prefix)
	before = append(before, before2...)
	out = prefix + fmt.Sprintf("%s %s %s\n", lhs, stmt.Tok.String(), content2)
	out += after
	return before, out
}

func genIfStmt(env *Env, stmt *goast.IfStmt, prefix string) (before []IBefore, out string) {
	before1, cond := genExpr(env, stmt.Cond, prefix)
	before2, body := genStmt(env, stmt.Body, prefix+"\t")
	before = append(before, before1...)
	before = append(before, before2...)
	var init string
	if stmt.Init != nil {
		var before3 []IBefore
		before3, init = genStmt(env, stmt.Init, prefix)
		before = append(before, before3...)
	}
	var initStr string
	init = strings.TrimSpace(init)
	if init != "" {
		initStr = init + "; "
	}
	out += prefix + "if " + initStr + cond + " {\n"
	out += body
	if stmt.Else != nil {
		if _, ok := stmt.Else.(*goast.IfStmt); ok {
			before3, content3 := genStmt(env, stmt.Else, prefix)
			before = append(before, before3...)
			out += prefix + "} else " + strings.TrimSpace(content3) + "\n"
		} else {
			before3, content3 := genStmt(env, stmt.Else, prefix+"\t")
			before = append(before, before3...)
			out += prefix + "} else {\n"
			out += content3
			out += prefix + "}\n"
		}
	} else {
		out += prefix + "}\n"
	}
	return before, out
}

func (g *Generator) genDecls() (out string) {
	for _, decl := range g.a.Decls {
		before1, content1 := genDecl(g.env, decl, "")
		g.before = append(g.before, before1...)
		out += content1
	}
	return
}

func genFuncDecl(env *Env, decl *goast.FuncDecl, prefix string) (before []IBefore, out string) {
	var name, recv, typeParamsStr, paramsStr, resultStr, bodyStr string
	if decl.Recv != nil {
		recv = joinList(env, decl.Recv, prefix)
		if recv != "" {
			recv = " (" + recv + ")"
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
		resultStr = env.GetType(result).GoStr()
		if resultStr != "" {
			resultStr = " " + resultStr
		}
	}
	if decl.Body != nil {
		before1, content := genStmt(env, decl.Body, prefix+"\t")
		before = append(before, before1...)
		bodyStr = content
	}
	out += fmt.Sprintf("func%s%s%s(%s)%s {\n%s}", recv, name, typeParamsStr, paramsStr, resultStr, bodyStr)
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

func AglVecSum[T cmp.Ordered](a []T) (out T) {
	for _, el := range a {
		out += el
	}
	return
}

`
}
