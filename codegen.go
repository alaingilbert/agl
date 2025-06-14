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
	//for _, b := range g.before {
	//	out += b.Content()
	//}
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

func (g *Generator) genExpr(e goast.Expr) (out string) {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *goast.Ident:
		return g.genIdent(expr)
	case *goast.ShortFuncLit:
		return g.genShortFuncLit(expr)
	case *goast.OptionExpr:
		return g.genOptionExpr(expr)
	case *goast.ResultExpr:
		return g.genResultExpr(expr)
	case *goast.BinaryExpr:
		return g.genBinaryExpr(expr)
	case *goast.BasicLit:
		return g.genBasicLit(expr)
	case *goast.CompositeLit:
		return g.genCompositeLit(expr)
	case *goast.TupleExpr:
		return g.genTupleExpr(expr)
	case *goast.KeyValueExpr:
		return g.genKeyValueExpr(expr)
	case *goast.ArrayType:
		return g.genArrayType(expr)
	case *goast.CallExpr:
		return g.genCallExpr(expr)
	case *goast.BubbleResultExpr:
		return g.genBubbleResultExpr(expr)
	case *goast.BubbleOptionExpr:
		return g.genBubbleOptionExpr(expr)
	case *goast.SelectorExpr:
		return g.genSelectorExpr(expr)
	case *goast.IndexExpr:
		return g.genIndexExpr(expr)
	case *goast.FuncType:
		return g.genFuncType(expr)
	case *goast.StructType:
		return g.genStructType(expr)
	case *goast.FuncLit:
		return g.genFuncLit(expr)
	case *goast.ParenExpr:
		return g.genParenExpr(expr)
	case *goast.Ellipsis:
		return g.genEllipsis(expr)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func (g *Generator) genIdent(expr *goast.Ident) (out string) {
	if strings.HasPrefix(expr.Name, "$") {
		expr.Name = strings.Replace(expr.Name, "$", "aglArg", 1)
	}
	t := g.env.GetType(expr)
	switch typ := t.(type) {
	//case types.BoolType:
	//return nil, Ternary(typ.V, "true", "false")
	case types.OkType:
		return "MakeResultOk"
	case types.ErrType:
		return fmt.Sprintf("MakeResultErr[%s]", typ.W.GoStr())
	case types.SomeType:
		return "MakeOptionSome"
	case types.NoneType:
		return fmt.Sprintf("MakeOptionNone[%s]()", typ.W.GoStr())
	case types.GenericType:
		return fmt.Sprintf("%s", typ.GoStr())
	}
	if expr.Name == "make" {
		return "make"
	}
	if expr.Name == "None" {
		return fmt.Sprintf("MakeOptionNone[%s]()", t.(types.OptionType).W.GoStr())
	}
	if v := g.env.Get(expr.Name); v != nil {
		return v.GoStr()
	}
	return expr.Name
}

func (g *Generator) incrPrefix(clb func() string) string {
	before := g.prefix
	g.prefix += "\t"
	out := clb()
	g.prefix = before
	return out
}

func (g *Generator) genShortFuncLit(expr *goast.ShortFuncLit) (out string) {
	t := g.env.GetType(expr).(types.FuncType)
	content1 := g.incrPrefix(func() string {
		return g.genStmt(expr.Body)
	})
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
	out += g.prefix + "}"
	return out
}

func (g *Generator) genEllipsis(expr *goast.Ellipsis) string {
	content1 := g.incrPrefix(func() string {
		return g.genExpr(expr.Elt)
	})
	return "..." + content1
}

func (g *Generator) genParenExpr(expr *goast.ParenExpr) string {
	content1 := g.incrPrefix(func() string {
		return g.genExpr(expr.X)
	})
	return "(" + content1 + ")"
}

func (g *Generator) genFuncLit(expr *goast.FuncLit) (out string) {
	content1 := g.incrPrefix(func() string {
		return g.genStmt(expr.Body)
	})
	content2 := g.genFuncType(expr.Type)
	out += content2 + " {\n"
	out += content1
	out += g.prefix + "}"
	return
}

func (g *Generator) genStructType(expr *goast.StructType) (out string) {
	out += g.prefix + "struct {\n"
	for _, field := range expr.Fields.List {
		content1 := g.genExpr(field.Type)
		var namesArr []string
		for _, name := range field.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += g.prefix + "\t" + strings.Join(namesArr, ", ") + " " + content1 + "\n"
	}
	out += g.prefix + "}"
	return
}

func (g *Generator) genFuncType(expr *goast.FuncType) string {
	content1 := g.incrPrefix(func() string {
		return g.genExpr(expr.Result)
	})
	var paramsStr, typeParamsStr string
	if typeParams := expr.TypeParams; typeParams != nil {
		typeParamsStr = g.joinList(expr.TypeParams)
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if params := expr.Params; params != nil {
		paramsStr = g.joinList(params)
	}
	if content1 != "" {
		content1 = " " + content1
	}
	return fmt.Sprintf("func%s(%s)%s", typeParamsStr, paramsStr, content1)
}

func (g *Generator) genIndexExpr(expr *goast.IndexExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExpr(expr.Index)
	return fmt.Sprintf("%s[%s]", content1, content2)
}

func (g *Generator) genSelectorExpr(expr *goast.SelectorExpr) (out string) {
	content1 := g.genExpr(expr.X)
	name := expr.Sel.Name
	switch g.env.GetType(expr.X).(type) {
	case types.TupleType:
		name = fmt.Sprintf("Arg%s", name)
	}
	return fmt.Sprintf("%s.%s", content1, name)
}

func (g *Generator) genBubbleOptionExpr(expr *goast.BubbleOptionExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("%s.Unwrap()", content1)
}

func (g *Generator) genBubbleResultExpr(expr *goast.BubbleResultExpr) (out string) {
	exprXT := MustCast[types.ResultType](g.env.GetType(expr.X))
	if exprXT.Bubble {
		content1 := g.genExpr(expr.X)
		before2 := NewBeforeStmt(addPrefix(`res := `+content1+`
if res.IsErr() {
	return res
}
`, g.prefix))
		g.before = append(g.before, before2)
		out += "res.Unwrap()"
	} else {
		if exprXT.Native {
			content1 := g.genExpr(expr.X)
			tmpl1 := "res, err := %s\nif err != nil {\n\tpanic(err)\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), g.prefix))
			out := `AglIdentity(res)`
			g.before = append(g.before, before)
			return out
		} else {
			content1 := g.genExpr(expr.X)
			out += fmt.Sprintf("%s.Unwrap()", content1)
		}
	}
	return out
}

func (g *Generator) genCallExpr(expr *goast.CallExpr) (out string) {
	switch e := expr.Fun.(type) {
	case *goast.SelectorExpr:
		if _, ok := g.env.GetType(e.X).(types.ArrayType); ok {
			if e.Sel.Name == "Filter" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				return fmt.Sprintf("AglVecFilter(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Map" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				return fmt.Sprintf("AglVecMap(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Reduce" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				content3 := g.genExpr(expr.Args[1])
				return fmt.Sprintf("AglReduce(%s, %s, %s)", content1, content2, content3)
			} else if e.Sel.Name == "Find" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				return fmt.Sprintf("AglVecFind(%s, %s)", content1, content2)
			} else if e.Sel.Name == "Sum" {
				content1 := g.genExpr(e.X)
				return fmt.Sprintf("AglVecSum(%s)", content1)
			} else if e.Sel.Name == "Joined" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				return fmt.Sprintf("AglJoined(%s, %s)", content1, content2)
			}
		}
	case *goast.Ident:
		if e.Name == "assert" {
			var contents []string
			for _, arg := range expr.Args {
				content1 := g.genExpr(arg)
				contents = append(contents, content1)
			}
			line := g.env.fset.Position(expr.Pos()).Line
			msg := fmt.Sprintf(`"assert failed line %d"`, line)
			if len(contents) == 1 {
				contents = append(contents, msg)
			} else {
				contents[1] = msg + ` + " " + ` + contents[1]
			}
			out := strings.Join(contents, ", ")
			return fmt.Sprintf("AglAssert(%s)", out)
		}
	}
	content1 := g.genExpr(expr.Fun)
	content2 := g.genExprs(expr.Args)
	return fmt.Sprintf("%s(%s)", content1, content2)
}

func (g *Generator) genArrayType(expr *goast.ArrayType) (out string) {
	content := g.genExpr(expr.Elt)
	return fmt.Sprintf("[]%s", content)
}

func (g *Generator) genKeyValueExpr(expr *goast.KeyValueExpr) (out string) {
	content1 := g.genExpr(expr.Key)
	content2 := g.genExpr(expr.Value)
	return fmt.Sprintf("%s: %s", content1, content2)
}

func (g *Generator) genResultExpr(expr *goast.ResultExpr) string {
	content := g.genExpr(expr.X)
	return fmt.Sprintf("Result[%s]", content)
}

func (g *Generator) genOptionExpr(expr *goast.OptionExpr) string {
	content := g.genExpr(expr.X)
	return fmt.Sprintf("Option[%s]", content)
}

func (g *Generator) genBasicLit(expr *goast.BasicLit) string {
	return expr.Value
}

func (g *Generator) genBinaryExpr(expr *goast.BinaryExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExpr(expr.Y)
	return fmt.Sprintf("%s %s %s", content1, expr.Op.String(), content2)
}

func (g *Generator) genCompositeLit(expr *goast.CompositeLit) (out string) {
	content1 := g.genExpr(expr.Type)
	content2 := g.genExprs(expr.Elts)
	out += fmt.Sprintf("%s{%s}", content1, content2)
	return
}

func (g *Generator) genTupleExpr(expr *goast.TupleExpr) (out string) {
	_ = g.genExprs(expr.Values)
	structName := g.env.GetType(expr).(types.TupleType).Name
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	for i, x := range expr.Values {
		structStr += fmt.Sprintf("\tArg%d %s\n", i, g.env.GetType(x).GoStr())
	}
	structStr += fmt.Sprintf("}\n")
	g.before = append(g.before, NewBeforeFn(structStr)) // TODO Add in public scope (when function output)
	var fields []string
	for i, x := range expr.Values {
		content1 := g.genExpr(x)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
}

func (g *Generator) genExprs(e []goast.Expr) (out string) {
	var tmp []string
	for _, expr := range e {
		content1 := g.genExpr(expr)
		tmp = append(tmp, content1)
	}
	return strings.Join(tmp, ", ")
}

func (g *Generator) genStmts(s []goast.Stmt) (out string) {
	for _, stmt := range s {
		content1 := g.genStmt(stmt)

		for _, b := range g.before {
			if v, ok := b.(*BeforeStmt); ok {
				out += v.Content()
			}
		}
		newBefore := make([]IBefore, 0)
		for _, b := range g.before {
			if v, ok := b.(*BeforeFn); ok {
				newBefore = append(newBefore, v)
			}
		}
		g.before = newBefore
		out += content1
	}
	return out
}

func (g *Generator) genStmt(s goast.Stmt) (out string) {
	//p("genStmt", to(s))
	switch stmt := s.(type) {
	case *goast.BlockStmt:
		return g.genBlockStmt(stmt)
	case *goast.IfStmt:
		return g.genIfStmt(stmt)
	case *goast.AssignStmt:
		return g.genAssignStmt(stmt)
	case *goast.ExprStmt:
		return g.genExprStmt(stmt)
	case *goast.ReturnStmt:
		return g.genReturnStmt(stmt)
	case *goast.RangeStmt:
		return g.genRangeStmt(stmt)
	case *goast.IncDecStmt:
		return g.genIncDecStmt(stmt)
	case *goast.DeclStmt:
		return g.genDeclStmt(stmt)
	default:
		panic(fmt.Sprintf("%v %v", s, to(s)))
	}
}

func (g *Generator) genBlockStmt(stmt *goast.BlockStmt) (out string) {
	content1 := g.genStmts(stmt.List)
	out += content1
	return
}

func (g *Generator) genSpecs(specs []goast.Spec) (out string) {
	for _, spec := range specs {
		content1 := g.genSpec(spec)
		out += content1
	}
	return
}

func (g *Generator) genSpec(s goast.Spec) (out string) {
	switch spec := s.(type) {
	case *goast.ValueSpec:
		content1 := g.genExpr(spec.Type)

		var namesArr []string
		for _, name := range spec.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += g.prefix + "var " + strings.Join(namesArr, ", ") + " " + content1 + "\n"
	case *goast.TypeSpec:
		content1 := g.genExpr(spec.Type)

		out += g.prefix + "type " + spec.Name.Name + " " + content1 + "\n"
	case *goast.ImportSpec:
		if spec.Name != nil {
			out += "import " + spec.Name.Name + "\n"
		}
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
	return
}

func (g *Generator) genDecl(d goast.Decl) (out string) {
	switch decl := d.(type) {
	case *goast.GenDecl:
		content1 := g.genGenDecl(decl)

		out += content1
		return
	case *goast.FuncDecl:
		out1 := g.genFuncDecl(decl)
		for _, b := range g.before {
			out += b.Content()
		}
		out += out1 + "\n"
		return
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
	return
}

func (g *Generator) genGenDecl(decl *goast.GenDecl) string {
	return g.genSpecs(decl.Specs)
}

func (g *Generator) genDeclStmt(stmt *goast.DeclStmt) string {
	return g.genDecl(stmt.Decl)
}

func (g *Generator) genIncDecStmt(stmt *goast.IncDecStmt) (out string) {
	content1 := g.genExpr(stmt.X)

	var op string
	switch stmt.Tok {
	case token.INC:
		op = "++"
	case token.DEC:
		op = "--"
	default:
		panic("")
	}
	out += g.prefix + content1 + op + "\n"
	return
}

func (g *Generator) genRangeStmt(stmt *goast.RangeStmt) (out string) {
	var content1, content2 string
	if stmt.Key != nil {
		content1 = g.genExpr(stmt.Key)

	}
	if stmt.Value != nil {
		content2 = g.genExpr(stmt.Value)
	}
	content3 := g.genExpr(stmt.X)
	content4 := g.incrPrefix(func() string {
		return g.genStmt(stmt.Body)
	})
	op := stmt.Tok
	if content1 == "" && content2 == "" {
		out += g.prefix + fmt.Sprintf("for range %s {\n", content3)
	} else if content2 == "" {
		out += g.prefix + fmt.Sprintf("for %s %s range %s {\n", content1, op, content3)
	} else {
		out += g.prefix + fmt.Sprintf("for %s, %s %s range %s {\n", content1, content2, op, content3)
	}
	out += fmt.Sprintf("%s", content4)
	out += g.prefix + "}\n"
	return
}

func (g *Generator) genReturnStmt(stmt *goast.ReturnStmt) (out string) {
	content1 := g.genExpr(stmt.Result)
	return g.prefix + fmt.Sprintf("return %s\n", content1)
}

func (g *Generator) genExprStmt(stmt *goast.ExprStmt) (out string) {
	content := g.genExpr(stmt.X)
	out += g.prefix + content + "\n"
	return out
}

func (g *Generator) genAssignStmt(stmt *goast.AssignStmt) (out string) {
	var lhs, after string
	if len(stmt.Rhs) == 1 && TryCast[types.TupleType](g.env.GetType(stmt.Rhs[0])) {
		rhs := stmt.Rhs[0]
		lhs = "aglVar1"
		var names []string
		var exprs []string
		if len(stmt.Lhs) == 1 {
			content1 := g.genExprs(stmt.Lhs)

			lhs = content1
		} else {
			for i := range g.env.GetType(rhs).(types.TupleType).Elts {
				name := stmt.Lhs[i].(*goast.Ident).Name
				names = append(names, name)
				exprs = append(exprs, fmt.Sprintf("%s.Arg%d", lhs, i))
			}
			after = g.prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
		}
	} else {
		content1 := g.genExprs(stmt.Lhs)

		lhs = content1
	}
	content2 := g.genExprs(stmt.Rhs)
	out = g.prefix + fmt.Sprintf("%s %s %s\n", lhs, stmt.Tok.String(), content2)
	out += after
	return out
}

func (g *Generator) genIfStmt(stmt *goast.IfStmt) (out string) {
	cond := g.genExpr(stmt.Cond)
	body := g.incrPrefix(func() string {
		return g.genStmt(stmt.Body)
	})

	var init string
	if stmt.Init != nil {
		init = g.genStmt(stmt.Init)
	}
	var initStr string
	init = strings.TrimSpace(init)
	if init != "" {
		initStr = init + "; "
	}
	out += g.prefix + "if " + initStr + cond + " {\n"
	out += body
	if stmt.Else != nil {
		if _, ok := stmt.Else.(*goast.IfStmt); ok {
			content3 := g.genStmt(stmt.Else)
			out += g.prefix + "} else " + strings.TrimSpace(content3) + "\n"
		} else {
			content3 := g.incrPrefix(func() string {
				return g.genStmt(stmt.Else)
			})
			out += g.prefix + "} else {\n"
			out += content3
			out += g.prefix + "}\n"
		}
	} else {
		out += g.prefix + "}\n"
	}
	return out
}

func (g *Generator) genDecls() (out string) {
	for _, decl := range g.a.Decls {
		g.prefix = ""
		content1 := g.genDecl(decl)
		out += content1
	}
	return
}

func (g *Generator) genFuncDecl(decl *goast.FuncDecl) (out string) {
	var name, recv, typeParamsStr, paramsStr, resultStr, bodyStr string
	if decl.Recv != nil {
		recv = g.joinList(decl.Recv)
		if recv != "" {
			recv = " (" + recv + ")"
		}
	}
	if decl.Name != nil {
		name = " " + decl.Name.Name
	}
	if typeParams := decl.Type.TypeParams; typeParams != nil {
		typeParamsStr = g.joinList(decl.Type.TypeParams)
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if params := decl.Type.Params; params != nil {
		paramsStr = g.joinList(params)
	}
	if result := decl.Type.Result; result != nil {
		resultStr = g.env.GetType(result).GoStr()
		if resultStr != "" {
			resultStr = " " + resultStr
		}
	}
	if decl.Body != nil {
		content := g.incrPrefix(func() string {
			return g.genStmt(decl.Body)
		})
		bodyStr = content
	}
	out += fmt.Sprintf("func%s%s%s(%s)%s {\n%s}", recv, name, typeParamsStr, paramsStr, resultStr, bodyStr)
	return
}

func (g *Generator) joinList(l *goast.FieldList) string {
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
		content := g.genExpr(field.Type)
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
