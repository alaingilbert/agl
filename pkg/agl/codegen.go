package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync/atomic"
)

type Generator struct {
	env          *Env
	a            *ast.File
	prefix       string
	before       []IBefore
	tupleStructs map[string]string
	varCounter   atomic.Int64
	returnType   types.Type
	extensions   map[string]Extension
	swapGen      bool
	genMap       map[string]types.Type
}

type Extension struct {
	decl *ast.FuncDecl
	gen  []ExtensionTest
}

type ExtensionTest struct {
	raw      types.Type
	concrete types.Type
}

func NewGenerator(env *Env, a *ast.File) *Generator {
	return &Generator{env: env, a: a, extensions: make(map[string]Extension), tupleStructs: make(map[string]string)}
}

func (g *Generator) genExtension(e Extension) (out string) {
	for _, ge := range e.gen {
		m := types.FindGen(ge.raw, ge.concrete)
		decl := e.decl
		var name, typeParamsStr, paramsStr, resultStr, bodyStr string
		if decl.Name != nil {
			name = decl.Name.Name
		}
		assert(len(decl.Recv.List) == 1)
		recv := decl.Recv.List[0]
		var recvName string
		if len(recv.Names) >= 1 {
			recvName = recv.Names[0].Name
		}
		recvT := recv.Type.(*ast.IndexExpr).Index.(*ast.Ident).Name
		var recvTName string
		if el, ok := m[recvT]; ok {
			recvTName = el.GoStr()
		} else {
			recvTName = recvT
		}

		var elts []string
		for _, k := range slices.Sorted(maps.Keys(m)) {
			elts = append(elts, fmt.Sprintf("%s_%s", k, m[k].GoStr()))
		}
		if _, ok := m["T"]; !ok {
			elts = append(elts, fmt.Sprintf("%s_%s", "T", recvTName))
		}

		firstArg := ast.Field{Names: []*ast.Ident{{Name: recvName}}, Type: &ast.ArrayType{Elt: &ast.Ident{Name: recvTName}}}
		var paramsClone []ast.Field
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				paramsClone = append(paramsClone, *param)
			}
		}
		paramsClone = append([]ast.Field{firstArg}, paramsClone...)
		g.swapGen = true
		g.genMap = m
		if params := paramsClone; params != nil {
			var fieldsItems []string
			for _, field := range params {
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
			paramsStr = strings.Join(fieldsItems, ", ")
		}
		if result := decl.Type.Result; result != nil {
			resT := g.env.GetType(result)
			for k, v := range m {
				resT = types.ReplGen(resT, k, v)
			}
			resultStr = prefixIf(resT.GoStr(), " ")
		}
		if decl.Body != nil {
			content := g.incrPrefix(func() string {
				return g.genStmt(decl.Body)
			})
			bodyStr = content
		}
		g.swapGen = false
		out += fmt.Sprintf("func AglVec%s_%s%s(%s)%s {\n%s}", name, strings.Join(elts, "_"), typeParamsStr, paramsStr, resultStr, bodyStr)
		out += "\n"
	}
	return
}

func (g *Generator) Generate() (out string) {
	out1 := g.genPackage()
	out2 := g.genImports()
	out3 := g.genDecls()
	var extStr string
	for _, extKey := range slices.Sorted(maps.Keys(g.extensions)) {
		extStr += g.genExtension(g.extensions[extKey])
	}
	var tupleStr string
	for _, k := range slices.Sorted(maps.Keys(g.tupleStructs)) {
		tupleStr += g.tupleStructs[k]
	}
	clear(g.tupleStructs)
	return out + out1 + out2 + tupleStr + out3 + extStr
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

func (g *Generator) genStmt(s ast.Stmt) (out string) {
	//p("genStmt", to(s))
	switch stmt := s.(type) {
	case *ast.BlockStmt:
		return g.genBlockStmt(stmt)
	case *ast.IfStmt:
		return g.genIfStmt(stmt)
	case *ast.AssignStmt:
		return g.genAssignStmt(stmt)
	case *ast.ExprStmt:
		return g.genExprStmt(stmt)
	case *ast.ReturnStmt:
		return g.genReturnStmt(stmt)
	case *ast.RangeStmt:
		return g.genRangeStmt(stmt)
	case *ast.ForStmt:
		return g.genForStmt(stmt)
	case *ast.IncDecStmt:
		return g.genIncDecStmt(stmt)
	case *ast.DeclStmt:
		return g.genDeclStmt(stmt)
	case *ast.IfLetStmt:
		return g.genIfLetStmt(stmt)
	case *ast.SendStmt:
		return g.genSendStmt(stmt)
	case *ast.SelectStmt:
		return g.genSelectStmt(stmt)
	case *ast.CommClause:
		return g.genCommClause(stmt)
	case *ast.SwitchStmt:
		return g.genSwitchStmt(stmt)
	case *ast.LabeledStmt:
		return g.genLabeledStmt(stmt)
	case *ast.CaseClause:
		return g.genCaseClause(stmt)
	case *ast.BranchStmt:
		return g.genBranchStmt(stmt)
	case *ast.DeferStmt:
		return g.genDeferStmt(stmt)
	case *ast.GoStmt:
		return g.genGoStmt(stmt)
	case *ast.TypeSwitchStmt:
		return g.genTypeSwitchStmt(stmt)
	case *ast.EmptyStmt:
		return g.genEmptyStmt(stmt)
	case *ast.MatchStmt:
		return g.genMatchStmt(stmt)
	case *ast.MatchClause:
		return g.genMatchClause(stmt)
	default:
		panic(fmt.Sprintf("%v %v", s, to(s)))
	}
}

func (g *Generator) genExpr(e ast.Expr) (out string) {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *ast.Ident:
		return g.genIdent(expr)
	case *ast.ShortFuncLit:
		return g.genShortFuncLit(expr)
	case *ast.OptionExpr:
		return g.genOptionExpr(expr)
	case *ast.ResultExpr:
		return g.genResultExpr(expr)
	case *ast.BinaryExpr:
		return g.genBinaryExpr(expr)
	case *ast.BasicLit:
		return g.genBasicLit(expr)
	case *ast.CompositeLit:
		return g.genCompositeLit(expr)
	case *ast.TupleExpr:
		return g.genTupleExpr(expr)
	case *ast.KeyValueExpr:
		return g.genKeyValueExpr(expr)
	case *ast.ArrayType:
		return g.genArrayType(expr)
	case *ast.CallExpr:
		return g.genCallExpr(expr)
	case *ast.BubbleResultExpr:
		return g.genBubbleResultExpr(expr)
	case *ast.BubbleOptionExpr:
		return g.genBubbleOptionExpr(expr)
	case *ast.SelectorExpr:
		return g.genSelectorExpr(expr)
	case *ast.IndexExpr:
		return g.genIndexExpr(expr)
	case *ast.FuncType:
		return g.genFuncType(expr)
	case *ast.StructType:
		return g.genStructType(expr)
	case *ast.FuncLit:
		return g.genFuncLit(expr)
	case *ast.ParenExpr:
		return g.genParenExpr(expr)
	case *ast.Ellipsis:
		return g.genEllipsis(expr)
	case *ast.InterfaceType:
		return g.genInterfaceType(expr)
	case *ast.TypeAssertExpr:
		return g.genTypeAssertExpr(expr)
	case *ast.StarExpr:
		return g.genStarExpr(expr)
	case *ast.MapType:
		return g.genMapType(expr)
	case *ast.SomeExpr:
		return g.genSomeExpr(expr)
	case *ast.OkExpr:
		return g.genOkExpr(expr)
	case *ast.ErrExpr:
		return g.genErrExpr(expr)
	case *ast.NoneExpr:
		return g.genNoneExpr(expr)
	case *ast.ChanType:
		return g.genChanType(expr)
	case *ast.UnaryExpr:
		return g.genUnaryExpr(expr)
	case *ast.OrBreakExpr:
		return g.genOrBreakExpr(expr)
	case *ast.OrContinueExpr:
		return g.genOrContinueExpr(expr)
	case *ast.OrReturnExpr:
		return g.genOrReturn(expr)
	case *ast.IndexListExpr:
		return g.genIndexListType(expr)
	case *ast.SliceExpr:
		return g.genSliceExpr(expr)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func (g *Generator) genIdent(expr *ast.Ident) (out string) {
	if strings.HasPrefix(expr.Name, "$") {
		expr.Name = strings.Replace(expr.Name, "$", "aglArg", 1)
	}
	t := g.env.GetType(expr)
	switch typ := t.(type) {
	case types.GenericType:
		if g.swapGen && typ.IsType {
			for k, v := range g.genMap {
				if typ.Name == k {
					typ.Name = v.GoStr()
					typ.W = v
				}
			}
			return fmt.Sprintf("%s", typ.GoStr())
		}
	}
	if expr.Name == "make" {
		return "make"
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

func (g *Generator) genShortFuncLit(expr *ast.ShortFuncLit) (out string) {
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

func (g *Generator) genEnumType(enumName string, expr *ast.EnumType) string {
	out := fmt.Sprintf("type %sTag int\n", enumName)
	out += fmt.Sprintf("const (\n")
	for i, v := range expr.Values.List {
		if i == 0 {
			out += fmt.Sprintf("\t%s_%s %sTag = iota + 1\n", enumName, v.Name.Name, enumName)
		} else {
			out += fmt.Sprintf("\t%s_%s\n", enumName, v.Name.Name)
		}
	}
	out += fmt.Sprintf(")\n")
	out += fmt.Sprintf("type %s struct {\n", enumName)
	out += fmt.Sprintf("\ttag %sTag\n", enumName)
	for _, field := range expr.Values.List {
		if field.Params != nil {
			for i, el := range field.Params.List {
				out += fmt.Sprintf("\t%s_%d %s\n", field.Name.Name, i, g.env.GetType2(el.Type).GoStr())
			}
		}
	}
	out += "}\n"
	out += fmt.Sprintf("func (v %s) String() string {\n\tswitch v.tag {\n", enumName)
	for _, field := range expr.Values.List {
		out += fmt.Sprintf("\tcase %s_%s:\n\t\treturn \"%s\"\n", enumName, field.Name.Name, field.Name.Name)
	}
	out += "\tdefault:\n\t\tpanic(\"\")\n\t}\n}\n"
	for _, field := range expr.Values.List {
		var tmp []string
		var tmp1 []string
		if field.Params != nil {
			for i, el := range field.Params.List {
				tmp = append(tmp, fmt.Sprintf("arg%d %s", i, g.env.GetType2(el.Type).GoStr()))
				tmp1 = append(tmp1, fmt.Sprintf("%s_%d: arg%d", field.Name.Name, i, i))
			}
		}
		var tmp1Out string
		if len(tmp1) > 0 {
			tmp1Out = ", " + strings.Join(tmp1, ", ")
		}
		out += fmt.Sprintf("func Make_%s_%s(%s) %s {\n\treturn %s{tag: %s_%s%s}\n}\n",
			enumName, field.Name.Name, strings.Join(tmp, ", "), enumName, enumName, enumName, field.Name.Name, tmp1Out)
	}
	return out
}

func (g *Generator) genTypeAssertExpr(expr *ast.TypeAssertExpr) string {
	var content1 string
	content2 := g.genExpr(expr.X)
	if expr.Type != nil {
		content1 = g.genExpr(expr.Type)
	} else {
		return content2 + ".(type)"
	}
	return fmt.Sprintf("AglTypeAssert[%s](%s)", content1, content2)
}

func (g *Generator) genStarExpr(expr *ast.StarExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("*%s", content1)
}

func (g *Generator) genMapType(expr *ast.MapType) string {
	content1 := g.genExpr(expr.Key)
	content2 := g.env.GetType2(expr.Value).GoStr()
	return fmt.Sprintf("map[%s]%s", content1, content2)
}

func (g *Generator) genSomeExpr(expr *ast.SomeExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("MakeOptionSome(%s)", content1)
}

func (g *Generator) genOkExpr(expr *ast.OkExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("MakeResultOk(%s)", content1)
}

func (g *Generator) genErrExpr(expr *ast.ErrExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("MakeResultErr[%s](%s)", g.env.GetType(expr).(types.ErrType).T.GoStr(), content1)
}

func (g *Generator) genChanType(expr *ast.ChanType) string {
	return fmt.Sprintf("chan %s", g.genExpr(expr.Value))
}

func (g *Generator) genOrBreakExpr(expr *ast.OrBreakExpr) (out string) {
	content1 := g.genExpr(expr.X)
	var check string
	if TryCast[types.ResultType](g.env.GetType(expr.X)) {
		check = "IsErr()"
	} else if TryCast[types.OptionType](g.env.GetType(expr.X)) {
		check = "IsNone()"
	}
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := ""
	before += g.prefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	before += g.prefix + "\tbreak"
	if expr.Label != nil {
		before += " " + expr.Label.String()
	}
	before += "\n"
	before += g.prefix + "}\n"
	g.before = append(g.before, NewBeforeStmt(before))
	return fmt.Sprintf("AglIdentity(%s).Unwrap()", varName)
}

func (g *Generator) genOrContinueExpr(expr *ast.OrContinueExpr) (out string) {
	content1 := g.genExpr(expr.X)
	var check string
	if TryCast[types.ResultType](g.env.GetType(expr.X)) {
		check = "IsErr()"
	} else if TryCast[types.OptionType](g.env.GetType(expr.X)) {
		check = "IsNone()"
	}
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := ""
	before += g.prefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	before += g.prefix + "\tcontinue"
	if expr.Label != nil {
		before += " " + expr.Label.String()
	}
	before += "\n"
	before += g.prefix + "}\n"
	g.before = append(g.before, NewBeforeStmt(before))
	return fmt.Sprintf("AglIdentity(%s).Unwrap()", varName)
}

func (g *Generator) genOrReturn(expr *ast.OrReturnExpr) (out string) {
	content1 := g.genExpr(expr.X)
	var check string
	if TryCast[types.ResultType](g.env.GetType(expr.X)) {
		check = "IsErr()"
	} else if TryCast[types.OptionType](g.env.GetType(expr.X)) {
		check = "IsNone()"
	}
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := ""
	before += g.prefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	if g.returnType == nil {
		before += g.prefix + "\treturn\n"
	} else {
		switch retT := g.returnType.(type) {
		case types.ResultType:
			before += g.prefix + fmt.Sprintf("\treturn MakeResultErr[%s](%s.Err())\n", retT.W, varName)
		case types.OptionType:
			before += g.prefix + fmt.Sprintf("\treturn MakeOptionNone[%s]()\n", retT.W)
		case types.VoidType:
			before += g.prefix + fmt.Sprintf("\treturn\n")
		default:
			assert(false, "cannot use or_return in a function that does not return void/Option/Result")
		}
	}
	before += g.prefix + "}\n"
	g.before = append(g.before, NewBeforeStmt(before))
	return fmt.Sprintf("AglIdentity(%s)", varName)
}

func (g *Generator) genUnaryExpr(expr *ast.UnaryExpr) string {
	return fmt.Sprintf("%s%s", expr.Op.String(), g.genExpr(expr.X))
}

func (g *Generator) genSendStmt(expr *ast.SendStmt) string {
	content1 := g.genExpr(expr.Chan)
	content2 := g.genExpr(expr.Value)
	return g.prefix + fmt.Sprintf("%s <- %s\n", content1, content2)
}

func (g *Generator) genSelectStmt(expr *ast.SelectStmt) (out string) {
	content1 := g.genStmt(expr.Body)
	out += g.prefix + "select {\n"
	out += content1
	out += g.prefix + "}\n"
	return
}

func (g *Generator) genLabeledStmt(expr *ast.LabeledStmt) (out string) {
	out += g.prefix + fmt.Sprintf("%s:\n", expr.Label.Name)
	out += g.genStmt(expr.Stmt)
	return
}

func (g *Generator) genBranchStmt(expr *ast.BranchStmt) (out string) {
	out += g.prefix + expr.Tok.String()
	if expr.Label != nil {
		out += " " + g.genExpr(expr.Label)
	}
	out += "\n"
	return
}

func (g *Generator) genDeferStmt(expr *ast.DeferStmt) (out string) {
	out += g.prefix + fmt.Sprintf("defer %s\n", g.genExpr(expr.Call))
	return
}

func (g *Generator) genGoStmt(expr *ast.GoStmt) (out string) {
	out += g.prefix + fmt.Sprintf("go %s\n", g.genExpr(expr.Call))
	return
}

func (g *Generator) genEmptyStmt(expr *ast.EmptyStmt) (out string) {
	return
}

func (g *Generator) genMatchClause(expr *ast.MatchClause) (out string) {
	switch v := expr.Expr.(type) {
	case *ast.ErrExpr:
		out += g.prefix + fmt.Sprintf("case Err(%s):\n", g.genExpr(v.X))
	case *ast.OkExpr:
		out += g.prefix + fmt.Sprintf("case Ok(%s):\n", g.genExpr(v.X))
	case *ast.SomeExpr:
		out += g.prefix + fmt.Sprintf("case Some(%s):\n", g.genExpr(v.X))
	default:
		panic("")
	}
	content1 := g.incrPrefix(func() string {
		return g.genStmts(expr.Body)
	})
	return out + content1
}

func (g *Generator) genMatchStmt(expr *ast.MatchStmt) (out string) {
	content1 := strings.TrimSpace(g.genStmt(expr.Init))
	initT := g.env.GetType(expr.Init)
	switch v := initT.(type) {
	case types.ResultType:
		if v.Native {
			out += g.prefix + fmt.Sprintf("tmp, tmpErr := %s\n", content1)
		} else {
			out += g.prefix + fmt.Sprintf("tmp := %s\n", content1)
		}
		if expr.Body != nil {
			for _, c := range expr.Body.List {
				c := c.(*ast.MatchClause)
				if v.Native {
					switch v := c.Expr.(type) {
					case *ast.OkExpr:
						out += g.prefix + fmt.Sprintf("if tmpErr == nil {\n%s\t%s := tmp\n", g.prefix, g.genExpr(v.X))
					case *ast.ErrExpr:
						out += g.prefix + fmt.Sprintf("if tmpErr != nil {\n%s\t%s := tmpErr\n", g.prefix, g.genExpr(v.X))
					default:
						panic("")
					}
				} else {
					switch v := c.Expr.(type) {
					case *ast.OkExpr:
						out += g.prefix + fmt.Sprintf("if tmp.IsOk() {\n%s\t%s := tmp.Unwrap()\n", g.prefix, g.genExpr(v.X))
					case *ast.ErrExpr:
						out += g.prefix + fmt.Sprintf("if tmp.IsErr() {\n%s\t%s := tmp.Err()\n", g.prefix, g.genExpr(v.X))
					default:
						panic("")
					}
				}
				content3 := g.incrPrefix(func() string {
					return g.genStmts(c.Body)
				})
				out += content3
				out += g.prefix + "}\n"
			}
		}
	case types.OptionType:
		out += g.prefix + fmt.Sprintf("tmp := %s\n", content1)
		if expr.Body != nil {
			for _, c := range expr.Body.List {
				c := c.(*ast.MatchClause)
				switch v := c.Expr.(type) {
				case *ast.SomeExpr:
					out += g.prefix + fmt.Sprintf("if tmp.IsSome() {\n%s\t%s := tmp.Unwrap()\n", g.prefix, g.genExpr(v.X))
				case *ast.NoneExpr:
					out += g.prefix + fmt.Sprintf("if tmp.IsNone() {\n")
				default:
					panic("")
				}
				content3 := g.incrPrefix(func() string {
					return g.genStmts(c.Body)
				})
				out += content3
				out += g.prefix + "}\n"
			}
		}
	default:
		panic("")
	}
	return
}

func (g *Generator) genTypeSwitchStmt(expr *ast.TypeSwitchStmt) (out string) {
	content1 := strings.TrimSpace(g.genStmt(expr.Assign))
	var content2 string
	if expr.Init != nil {
		content2 = strings.TrimSpace(g.genStmt(expr.Init))
	}
	content3 := g.genStmt(expr.Body)
	out += g.prefix + fmt.Sprintf("switch %s%s {\n", content2, content1)
	out += content3
	out += g.prefix + "}\n"
	return
}

func (g *Generator) genCaseClause(expr *ast.CaseClause) (out string) {
	var listStr string
	if expr.List != nil {
		var els []string
		for _, el := range expr.List {
			els = append(els, g.genExpr(el))
		}
		listStr = "case " + strings.Join(els, ", ") + ":\n"
	} else {
		listStr = "default:\n"
	}
	var content1 string
	if expr.Body != nil {
		content1 = g.genStmts(expr.Body)
	}
	out += g.prefix + listStr
	out += content1
	return
}

func (g *Generator) genSwitchStmt(expr *ast.SwitchStmt) (out string) {
	var content1 string
	if expr.Init != nil {
		content1 = strings.TrimSpace(g.genStmt(expr.Init))
		if content1 != "" {
			content1 = content1 + " "
		}
	}
	var content2 string
	if expr.Tag != nil {
		content2 = g.genExpr(expr.Tag)
		if content2 != "" {
			content2 = content2 + " "
		}
	}
	content3 := g.genStmt(expr.Body)
	out += g.prefix + fmt.Sprintf("switch %s%s{\n", content1, content2)
	out += content3
	out += g.prefix + "}\n"
	return
}

func (g *Generator) genCommClause(expr *ast.CommClause) (out string) {
	var content1, content2 string
	if expr.Comm != nil {
		content1 = "case " + strings.TrimSpace(g.genStmt(expr.Comm)) + ":"
	} else {
		content1 = "default:"
	}
	if expr.Body != nil {
		content2 = g.genStmts(expr.Body)
	}
	out += g.prefix + fmt.Sprintf("%s\n", content1)
	out += content2
	return
}

func (g *Generator) genNoneExpr(expr *ast.NoneExpr) string {
	//content1 := g.genExpr(expr)
	return fmt.Sprintf("MakeOptionNone[%s]()", g.env.GetType(expr).(types.NoneType).W.GoStr())
}

func (g *Generator) genInterfaceType(expr *ast.InterfaceType) (out string) {
	out += "interface {\n"
	if expr.Methods != nil {
		for _, m := range expr.Methods.List {
			content1 := g.genExpr(m.Type)
			out += g.prefix + "\t" + m.Names[0].Name + strings.TrimPrefix(content1, "func") + "\n"
		}
	}
	out += "}"
	return
}

func (g *Generator) genEllipsis(expr *ast.Ellipsis) string {
	content1 := g.incrPrefix(func() string {
		return g.genExpr(expr.Elt)
	})
	return "..." + content1
}

func (g *Generator) genParenExpr(expr *ast.ParenExpr) string {
	content1 := g.incrPrefix(func() string {
		return g.genExpr(expr.X)
	})
	return "(" + content1 + ")"
}

func (g *Generator) genFuncLit(expr *ast.FuncLit) (out string) {
	content1 := g.incrPrefix(func() string {
		return g.genStmt(expr.Body)
	})
	content2 := g.genFuncType(expr.Type)
	out += content2 + " {\n"
	out += content1
	out += g.prefix + "}"
	return
}

func (g *Generator) genStructType(expr *ast.StructType) (out string) {
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

func (g *Generator) genFuncType(expr *ast.FuncType) string {
	content1 := g.incrPrefix(func() string {
		if expr.Result != nil {
			return g.genExpr(expr.Result)
		} else {
			return ""
		}
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

func (g *Generator) genIndexExpr(expr *ast.IndexExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExpr(expr.Index)
	return fmt.Sprintf("%s[%s]", content1, content2)
}

func (g *Generator) genSliceExpr(expr *ast.SliceExpr) string {
	content1 := g.genExpr(expr.X)
	var content2, content3, content4 string
	if expr.Low != nil {
		content2 = g.genExpr(expr.Low)
	}
	if expr.High != nil {
		content3 = g.genExpr(expr.High)
	}
	if expr.Max != nil {
		content4 = g.genExpr(expr.Max)
	}
	out := fmt.Sprintf("%s[%s:%s]", content1, content2, content3)
	if content4 != "" {
		out += ":" + content4
	}
	return out
}

func (g *Generator) genIndexListType(expr *ast.IndexListExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExprs(expr.Indices)
	return fmt.Sprintf("%s[%s]", content1, content2)
}

func (g *Generator) genSelectorExpr(expr *ast.SelectorExpr) (out string) {
	content1 := g.genExpr(expr.X)
	name := expr.Sel.Name
	switch g.env.GetType(expr.X).(type) {
	case types.TupleType:
		name = fmt.Sprintf("Arg%s", name)
	case types.EnumType:
		content2 := g.genExpr(expr.Sel)
		out := fmt.Sprintf("Make_%s_%s", content1, content2)
		if _, ok := g.env.GetType(expr).(types.EnumType); ok { // TODO
			out += "()"
		}
		return out
	}
	return fmt.Sprintf("%s.%s", content1, name)
}

func (g *Generator) genBubbleOptionExpr(expr *ast.BubbleOptionExpr) (out string) {
	exprXT := MustCast[types.OptionType](g.env.GetType(expr.X))
	if exprXT.Bubble {
		content1 := g.genExpr(expr.X)
		if exprXT.Native {
			varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
			tmpl := varName + ", ok := %s\nif !ok {\n\treturn MakeOptionNone[%s]()\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.W.GoStr()), g.prefix))
			g.before = append(g.before, before)
			return fmt.Sprintf(`AglIdentity(%s)`, varName)
		} else {
			varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
			tmpl := fmt.Sprintf("%s := %%s\nif %s.IsNone() {\n\treturn %s\n}\n", varName, varName, varName)
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), g.prefix))
			g.before = append(g.before, before2)
			out += fmt.Sprintf("%s.Unwrap()", varName)
		}
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
	return
}

func (g *Generator) genBubbleResultExpr(expr *ast.BubbleResultExpr) (out string) {
	exprXT := MustCast[types.ResultType](g.env.GetType(expr.X))
	if exprXT.Bubble {
		content1 := g.genExpr(expr.X)
		if _, ok := exprXT.W.(types.VoidType); ok && exprXT.Native {
			tmpl := "if err := %s; err != nil {\n\treturn MakeResultErr[%s](err)\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.W.GoStr()), g.prefix))
			g.before = append(g.before, before)
			return `AglNoop()`
		} else if exprXT.Native {
			tmpl := "tmp, err := %s\nif err != nil {\n\treturn MakeResultErr[%s](err)\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, g.returnType.(types.ResultType).W), g.prefix))
			g.before = append(g.before, before)
			return `AglIdentity(tmp)`
		} else if exprXT.ConvertToNone {
			tmpl := "res := %s\nif res.IsErr() {\n\treturn MakeOptionNone[%s]()\n}\n"
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.ToNoneType.GoStr()), g.prefix))
			g.before = append(g.before, before2)
			out += "res.Unwrap()"
		} else {
			tmpl := "res := %s\nif res.IsErr() {\n\treturn res\n}\n"
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), g.prefix))
			g.before = append(g.before, before2)
			out += "res.Unwrap()"
		}
	} else {
		if exprXT.Native {
			var tmpl1 string
			if _, ok := exprXT.W.(types.VoidType); ok {
				tmpl1 = "err := %s\nif err != nil {\n\tpanic(err)\n}\n"
				out = `AglNoop()`
			} else {
				id := g.varCounter.Add(1)
				varName := fmt.Sprintf("aglTmp%d", id)
				tmpl1 = varName + ", err := %s\nif err != nil {\n\tpanic(err)\n}\n"
				out = fmt.Sprintf(`AglIdentity(%s)`, varName)
			}
			content1 := g.genExpr(expr.X)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), g.prefix))
			g.before = append(g.before, before)
			return out
		} else {
			content1 := g.genExpr(expr.X)
			out += fmt.Sprintf("%s.Unwrap()", content1)
		}
	}
	return out
}

func (g *Generator) genCallExpr(expr *ast.CallExpr) (out string) {
	switch e := expr.Fun.(type) {
	case *ast.SelectorExpr:
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
			} else {
				extName := "agl.Vec." + e.Sel.Name
				t := g.env.Get(extName)
				rawFnT := t
				concreteT := g.env.GetType(expr.Fun)
				m := types.FindGen(rawFnT, concreteT)
				tmp := g.extensions[extName]
				tmp.gen = append(tmp.gen, ExtensionTest{raw: rawFnT, concrete: concreteT})
				g.extensions[extName] = tmp
				content1 := g.genExpr(e.X)
				var els []string
				for _, k := range slices.Sorted(maps.Keys(m)) {
					els = append(els, fmt.Sprintf("%s_%s", k, m[k].GoStr()))
				}
				if _, ok := m["T"]; !ok {
					recvTName := rawFnT.(types.FuncType).TypeParams[0].(types.GenericType).W.GoStr()
					els = append(els, fmt.Sprintf("%s_%s", "T", recvTName))
				}
				elsStr := strings.Join(els, "_")
				content2 := prefixIf(g.genExprs(expr.Args), ", ")
				return fmt.Sprintf("AglVec%s_%s(%s%s)", e.Sel.Name, elsStr, content1, content2)
			}
		}
	case *ast.Ident:
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
	var content1 string
	switch v := expr.Fun.(type) {
	case *ast.Ident:
		t1 := g.env.Get(v.Name)
		if t2, ok := t1.(types.TypeType); ok && TryCast[types.CustomType](t2.W) {
			content1 = expr.Fun.(*ast.Ident).Name
		} else {
			content1 = g.genExpr(expr.Fun)
		}
	default:
		content1 = g.genExpr(expr.Fun)
	}
	content2 := g.genExprs(expr.Args)
	return fmt.Sprintf("%s(%s)", content1, content2)
}

func prefixIf(s, prefix string) string {
	if s != "" {
		return prefix + s
	}
	return s
}

func suffixIf(s, suffix string) string {
	if s != "" {
		return s + suffix
	}
	return s
}

func (g *Generator) genArrayType(expr *ast.ArrayType) (out string) {
	var content string
	switch v := expr.Elt.(type) {
	case *ast.TupleExpr:
		content = g.env.GetType(v).GoStr()
	default:
		content = g.genExpr(expr.Elt)
	}
	return fmt.Sprintf("[]%s", content)
}

func (g *Generator) genKeyValueExpr(expr *ast.KeyValueExpr) (out string) {
	content1 := g.genExpr(expr.Key)
	content2 := g.genExpr(expr.Value)
	return fmt.Sprintf("%s: %s", content1, content2)
}

func (g *Generator) genResultExpr(expr *ast.ResultExpr) string {
	content := g.genExpr(expr.X)
	return fmt.Sprintf("Result[%s]", content)
}

func (g *Generator) genOptionExpr(expr *ast.OptionExpr) string {
	content := g.genExpr(expr.X)
	return fmt.Sprintf("Option[%s]", content)
}

func (g *Generator) genBasicLit(expr *ast.BasicLit) string {
	return expr.Value
}

func (g *Generator) genBinaryExpr(expr *ast.BinaryExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExpr(expr.Y)
	op := expr.Op.String()
	if g.env.GetType(expr.X) != nil && g.env.GetType(expr.Y) != nil {
		if TryCast[types.StructType](g.env.GetType(expr.X)) && TryCast[types.StructType](g.env.GetType(expr.Y)) {
			lhsName := g.env.GetType(expr.X).(types.StructType).Name
			rhsName := g.env.GetType(expr.Y).(types.StructType).Name
			if lhsName == rhsName {
				if (op == "==" || op == "!=") && g.env.Get(lhsName+".__EQL") != nil {
					if op == "==" {
						return fmt.Sprintf("%s.__EQL(%s)", content1, content2)
					} else {
						return fmt.Sprintf("!%s.__EQL(%s)", content1, content2)
					}
				} else if op == "+" && g.env.Get(lhsName+".__ADD") != nil {
					return fmt.Sprintf("%s.__ADD(%s)", content1, content2)
				} else if op == "-" && g.env.Get(lhsName+".__SUB") != nil {
					return fmt.Sprintf("%s.__SUB(%s)", content1, content2)
				} else if op == "*" && g.env.Get(lhsName+".__MUL") != nil {
					return fmt.Sprintf("%s.__MUL(%s)", content1, content2)
				} else if op == "/" && g.env.Get(lhsName+".__QUO") != nil {
					return fmt.Sprintf("%s.__QUO(%s)", content1, content2)
				} else if op == "%" && g.env.Get(lhsName+".__REM") != nil {
					return fmt.Sprintf("%s.__REM(%s)", content1, content2)
				}
			}
		}
	}
	return fmt.Sprintf("%s %s %s", content1, expr.Op.String(), content2)
}

func (g *Generator) genCompositeLit(expr *ast.CompositeLit) (out string) {
	var content1 string
	if expr.Type != nil {
		content1 = g.genExpr(expr.Type)
	}
	content2 := g.genExprs(expr.Elts)
	return fmt.Sprintf("%s{%s}", content1, content2)
}

func (g *Generator) genTupleExpr(expr *ast.TupleExpr) (out string) {
	_ = g.genExprs(expr.Values)
	structName := g.env.GetType(expr).(types.TupleType).GoStr()
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	for i, x := range expr.Values {
		structStr += fmt.Sprintf("\tArg%d %s\n", i, g.env.GetType2(x).GoStr())
	}
	structStr += fmt.Sprintf("}\n")
	//structStr += fmt.Sprintf("func (s %s) String() string {\n", structName)
	//var tmp []string
	//for _, x := range expr.Values {
	//	tmp = append(tmp, g.genExpr(x))
	//}
	//structStr += fmt.Sprintf("\treturn \"(%s)\"\n", strings.Join(tmp, ", "))
	//structStr += "}\n"
	g.tupleStructs[structName] = structStr
	var fields []string
	for i, x := range expr.Values {
		content1 := g.genExpr(x)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
}

func (g *Generator) genExprs(e []ast.Expr) (out string) {
	var tmp []string
	for _, expr := range e {
		content1 := g.genExpr(expr)
		tmp = append(tmp, content1)
	}
	return strings.Join(tmp, ", ")
}

func (g *Generator) genStmts(s []ast.Stmt) (out string) {
	for _, stmt := range s {
		content1 := g.genStmt(stmt)
		var beforeStmtStr string
		newBefore := make([]IBefore, 0)
		for _, b := range g.before {
			switch v := b.(type) {
			case *BeforeStmt:
				beforeStmtStr += v.Content()
			case *BeforeFn:
				newBefore = append(newBefore, v)
			}
		}
		g.before = newBefore
		out += beforeStmtStr + content1
	}
	return out
}

func (g *Generator) genBlockStmt(stmt *ast.BlockStmt) (out string) {
	return g.genStmts(stmt.List)
}

func (g *Generator) genSpecs(specs []ast.Spec) (out string) {
	for _, spec := range specs {
		out += g.genSpec(spec)
	}
	return
}

func (g *Generator) genSpec(s ast.Spec) (out string) {
	switch spec := s.(type) {
	case *ast.ValueSpec:
		content1 := g.genExpr(spec.Type)
		var namesArr []string
		for _, name := range spec.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += g.prefix + "var " + strings.Join(namesArr, ", ") + " " + content1
		if spec.Values != nil {
			out += " = " + g.genExprs(spec.Values)
		}
		out += "\n"
	case *ast.TypeSpec:
		if v, ok := spec.Type.(*ast.EnumType); ok {
			content1 := g.genEnumType(spec.Name.Name, v)
			out += g.prefix + content1 + "\n"
		} else {
			var typeParamsStr string
			if typeParams := spec.TypeParams; typeParams != nil {
				typeParamsStr = g.joinList(spec.TypeParams)
				if typeParamsStr != "" {
					typeParamsStr = "[" + typeParamsStr + "]"
				}
			}
			content1 := g.genExpr(spec.Type)
			out += g.prefix + "type " + spec.Name.Name + typeParamsStr + " " + content1 + "\n"
		}
	case *ast.ImportSpec:
		if spec.Name != nil {
			out += "import " + spec.Name.Name + "\n"
		}
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
	return
}

func (g *Generator) genDecl(d ast.Decl) (out string) {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return g.genGenDecl(decl)
	case *ast.FuncDecl:
		out1 := g.genFuncDecl(decl)
		for _, b := range g.before {
			out += b.Content()
		}
		clear(g.before)
		out += suffixIf(out1, "\n")
		return
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
	return
}

func (g *Generator) genGenDecl(decl *ast.GenDecl) string {
	return g.genSpecs(decl.Specs)
}

func (g *Generator) genDeclStmt(stmt *ast.DeclStmt) string {
	return g.genDecl(stmt.Decl)
}

func (g *Generator) genIncDecStmt(stmt *ast.IncDecStmt) (out string) {
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

func (g *Generator) genForStmt(stmt *ast.ForStmt) (out string) {
	var init, cond, post string
	var els []string
	if stmt.Init != nil {
		init = strings.TrimSpace(g.genStmt(stmt.Init))
		els = append(els, init)
	}
	if stmt.Cond != nil {
		cond = g.genExpr(stmt.Cond)
		els = append(els, cond)
	}
	if stmt.Post != nil {
		post = strings.TrimSpace(g.genStmt(stmt.Post))
		els = append(els, post)
	}
	tmp := strings.Join(els, "; ")
	tmp = suffixIf(tmp, " ")
	body := g.incrPrefix(func() string { return g.genStmt(stmt.Body) })
	out += g.prefix + fmt.Sprintf("for %s{\n", tmp)
	out += body
	out += g.prefix + "}\n"
	return
}

func (g *Generator) genRangeStmt(stmt *ast.RangeStmt) (out string) {
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

func (g *Generator) genReturnStmt(stmt *ast.ReturnStmt) (out string) {
	if stmt.Result == nil {
		return g.prefix + "return\n"
	}
	content1 := g.genExpr(stmt.Result)
	return g.prefix + fmt.Sprintf("return %s\n", content1)
}

func (g *Generator) genExprStmt(stmt *ast.ExprStmt) (out string) {
	content := g.genExpr(stmt.X)
	out += g.prefix + content + "\n"
	return out
}

func (g *Generator) genAssignStmt(stmt *ast.AssignStmt) (out string) {
	var lhs, after string
	if len(stmt.Rhs) == 1 && TryCast[types.EnumType](g.env.GetType(stmt.Rhs[0])) {
		rhsT := g.env.GetType(stmt.Rhs[0]).(types.EnumType)
		if len(stmt.Lhs) == 1 {
			content1 := g.genExprs(stmt.Lhs)
			lhs = content1
		} else {
			lhs = fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
			var names []string
			var exprs []string
			for i, x := range stmt.Lhs {
				names = append(names, x.(*ast.Ident).Name)
				exprs = append(exprs, fmt.Sprintf("%s.%s_%d", lhs, rhsT.SubTyp, i))
			}
			after = g.prefix + fmt.Sprintf("%s := %s\n", strings.Join(names, ", "), strings.Join(exprs, ", "))
		}
	} else if len(stmt.Rhs) == 1 && TryCast[types.TupleType](g.env.GetType(stmt.Rhs[0])) {
		if len(stmt.Lhs) == 1 {
			content1 := g.genExprs(stmt.Lhs)
			lhs = content1
		} else {
			lhs = fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
			rhs := stmt.Rhs[0]
			var names []string
			var exprs []string
			for i := range g.env.GetType(rhs).(types.TupleType).Elts {
				name := stmt.Lhs[i].(*ast.Ident).Name
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

func (g *Generator) genIfLetStmt(stmt *ast.IfLetStmt) (out string) {
	ass := stmt.Ass
	lhs := g.genExpr(ass.Lhs[0])
	rhs := g.incrPrefix(func() string { return g.genExpr(ass.Rhs[0]) })
	body := g.incrPrefix(func() string { return g.genStmt(stmt.Body) })
	var cond string
	unwrapFn := "Unwrap"
	switch stmt.Op {
	case token.SOME:
		cond = "tmp.IsSome()"
	case token.OK:
		cond = "tmp.IsOk()"
	case token.ERR:
		cond = "tmp.IsErr()"
		unwrapFn = "Err"
	default:
		panic("")
	}
	out += g.prefix + fmt.Sprintf("if tmp := %s; %s {\n", rhs, cond)
	out += g.prefix + fmt.Sprintf("\t%s := tmp.%s()\n", lhs, unwrapFn)
	out += body
	out += g.prefix + "}\n"
	return out
}

func (g *Generator) genIfStmt(stmt *ast.IfStmt) (out string) {
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
		if _, ok := stmt.Else.(*ast.IfStmt); ok {
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

func (g *Generator) genFuncDecl(decl *ast.FuncDecl) (out string) {
	g.returnType = g.env.GetType(decl).(types.FuncType).Return
	var name, recv, typeParamsStr, paramsStr, resultStr, bodyStr string
	if decl.Recv != nil {
		if len(decl.Recv.List) >= 1 {
			if tmp1, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if tmp2, ok := tmp1.X.(*ast.SelectorExpr); ok {
					if tmp2.Sel.Name == "Vec" {
						fnName := fmt.Sprintf("agl.Vec.%s", decl.Name.Name)
						tmp := g.extensions[fnName]
						tmp.decl = decl
						g.extensions[fnName] = tmp
						return
					}
				}
			}
		}
		recv = g.joinList(decl.Recv)
		if recv != "" {
			recv = " (" + recv + ")"
		}
	}
	if decl.Name != nil {
		fnName := decl.Name.Name
		if newName, ok := overloadMapping[fnName]; ok {
			fnName = newName
		}
		name = " " + fnName
	}
	if typeParams := decl.Type.TypeParams; typeParams != nil {
		typeParamsStr = g.joinList(decl.Type.TypeParams)
		if typeParamsStr != "" {
			typeParamsStr = "[" + typeParamsStr + "]"
		}
	}
	if params := decl.Type.Params; params != nil {
		var fieldsItems []string
		for _, field := range params.List {
			var namesItems []string
			for _, n := range field.Names {
				namesItems = append(namesItems, n.Name)
			}
			tmp2Str := strings.Join(namesItems, ", ")
			var content string
			switch field.Type.(type) {
			case *ast.TupleExpr:
				content = g.env.GetType(field.Type).GoStr()
			default:
				content = g.genExpr(field.Type)
			}
			if tmp2Str != "" {
				tmp2Str = tmp2Str + " "
			}
			fieldsItems = append(fieldsItems, tmp2Str+content)
		}
		paramsStr = strings.Join(fieldsItems, ", ")
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

func (g *Generator) joinList(l *ast.FieldList) string {
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

func GenCore() string {
	return `
package main

import (
	"cmp"
	"fmt"
	"strings"
)

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

func (o Option[T]) UnwrapOr(d T) T {
	if o.IsNone() {
		return d
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

func (r Result[T]) UnwrapOr(d T) T {
	if r.IsErr() {
		return d
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

func AglNoop(_ ...any) {}

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
