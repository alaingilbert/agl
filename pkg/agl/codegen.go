package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"agl/pkg/utils"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

type Generator struct {
	fset          *token.FileSet
	env           *Env
	a             *ast.File
	prefix        string
	before        []IBefore
	beforeStmt    []IBefore
	genFuncDecls2 map[string]string
	tupleStructs  map[string]string
	genFuncDecls  map[string]*ast.FuncDecl
	varCounter    atomic.Int64
	returnType    types.Type
	extensions    map[string]Extension
	swapGen       bool
	genMap        map[string]types.Type
	parent        *Generator
	isType        bool
}

func (g *Generator) withType(clb func()) {
	prev := g.isType
	g.isType = true
	clb()
	g.isType = prev
}

func (g *Generator) WithSub(clb func()) {
	prev := g.beforeStmt
	g.beforeStmt = make([]IBefore, 0)
	clb()
	g.beforeStmt = prev
}

func (g *Generator) WithGenMapping(m map[string]types.Type, clb func()) {
	prevSwapGen := g.swapGen
	prev := g.genMap
	g.swapGen = true
	g.genMap = m
	clb()
	g.swapGen = prevSwapGen
	g.genMap = prev
}

type Extension struct {
	decl *ast.FuncDecl
	gen  map[string]ExtensionTest
}

type ExtensionTest struct {
	raw      types.Type
	concrete types.Type
}

func NewGenerator(env *Env, a *ast.File, fset *token.FileSet) *Generator {
	genFns := make(map[string]*ast.FuncDecl)
	return &Generator{
		fset:          fset,
		env:           env,
		a:             a,
		extensions:    make(map[string]Extension),
		tupleStructs:  make(map[string]string),
		genFuncDecls2: make(map[string]string),
		genFuncDecls:  genFns}
}

func (g *Generator) genExtension(e Extension) (out string) {
	for _, key := range slices.Sorted(maps.Keys(e.gen)) {
		ge := e.gen[key]
		m := types.FindGen(ge.raw, ge.concrete)
		decl := e.decl
		if decl == nil {
			return ""
		}
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
		g.WithGenMapping(m, func() {
			if params := paramsClone; params != nil {
				var fieldsItems []string
				for _, field := range params {
					var content string
					if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
						if _, ok := v.W.(types.FuncType); ok {
							content = types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStr1()
						} else {
							content = g.genExpr(field.Type)
						}
					} else {
						switch field.Type.(type) {
						case *ast.TupleExpr:
							content = g.env.GetType(field.Type).GoStr()
						default:
							content = g.genExpr(field.Type)
						}
					}
					namesStr := utils.MapJoin(field.Names, func(n *ast.Ident) string { return n.Name }, ", ")
					namesStr = utils.SuffixIf(namesStr, " ")
					fieldsItems = append(fieldsItems, namesStr+content)
				}
				paramsStr = strings.Join(fieldsItems, ", ")
			}
			if result := decl.Type.Result; result != nil {
				resT := g.env.GetType(result)
				for k, v := range m {
					resT = types.ReplGen(resT, k, v)
				}
				resultStr = utils.PrefixIf(resT.GoStr(), " ")
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
		})
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
	var genFuncDeclStr string
	for _, k := range slices.Sorted(maps.Keys(g.genFuncDecls2)) {
		genFuncDeclStr += g.genFuncDecls2[k]
	}
	clear(g.genFuncDecls2)
	return out + out1 + out2 + tupleStr + genFuncDeclStr + out3 + extStr
}

func (g *Generator) genPackage() string {
	return fmt.Sprintf("package %s\n", g.a.Name.Name)
}

func (g *Generator) genImports() (out string) {
	for _, spec := range g.a.Imports {
		out += "import "
		if spec.Name != nil {
			out += spec.Name.Name + " "
		}
		pathValue := spec.Path.Value
		if strings.HasPrefix(pathValue, `"agl1/`) {
			pathValue = `"` + pathValue[6:]
		}
		out += pathValue + "\n"
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
	case *ast.MatchClause:
		return g.genMatchClause(stmt)
	default:
		panic(fmt.Sprintf("%v %v", s, to(s)))
	}
}

func (g *Generator) genExpr(e ast.Expr) (out string) {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *ast.MatchExpr:
		return g.genMatchExpr(expr)
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
	case *ast.SetType:
		return g.genSetType(expr)
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
	case *ast.DumpExpr:
		return g.genDumpExpr(expr)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func (g *Generator) genIdent(expr *ast.Ident) (out string) {
	if strings.HasPrefix(expr.Name, "$") {
		beforeT := g.env.GetType(expr)
		expr.Name = strings.Replace(expr.Name, "$", "aglArg", 1)
		g.env.SetType(nil, expr, beforeT, g.fset)
	}
	if strings.HasPrefix(expr.Name, "@") {
		expr.Name = strings.Replace(expr.Name, "@LINE", fmt.Sprintf(`"%d"`, g.fset.Position(expr.Pos()).Line), 1)
	}
	t := g.env.GetType(expr)
	if v, ok := t.(types.TypeType); ok {
		t = v.W
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
	}
	switch expr.Name {
	case "make":
		return "make"
	case "abs":
		return "AglAbs"
	}
	if v := g.env.Get(expr.Name); v != nil {
		switch vv := v.(type) {
		case types.TypeType:
			return v.GoStr()
		case types.FuncType:
			name := expr.Name
			if vv.Pub {
				name = "AglPub_" + name
			}
			return name
		default:
			return expr.Name
		}
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

func (g *Generator) decrPrefix(clb func() string) string {
	before := g.prefix
	g.prefix = g.prefix[:len(g.prefix)-1]
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
			tmp = append(tmp, fmt.Sprintf("aglArg%d %s", i, types.ReplGenM(arg, g.genMap).GoStr()))
		}
		argsStr = strings.Join(tmp, ", ")
	}
	ret := types.ReplGenM(t.Return, g.genMap)
	if ret != nil {
		returnStr = " " + ret.GoStrType()
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
				out += fmt.Sprintf("\t%s_%d %s\n", field.Name.Name, i, g.env.GetType2(el.Type, g.fset).GoStr())
			}
		}
	}
	out += "}\n"
	out += fmt.Sprintf("func (v %s) String() string {\n\tswitch v.tag {\n", enumName)
	for _, field := range expr.Values.List {
		tmp := fmt.Sprintf("%s", field.Name.Name)
		if field.Params != nil {
			var placeholders []string
			var args []string
			for i := range field.Params.List {
				placeholders = append(placeholders, "%v")
				args = append(args, fmt.Sprintf("v.%s_%d", field.Name.Name, i))
			}
			tmp = fmt.Sprintf("fmt.Sprintf(\"%s(%s)\", %s)", tmp, strings.Join(placeholders, ", "), strings.Join(args, ", "))
		} else {
			tmp = `"` + tmp + `"`
		}
		out += fmt.Sprintf("\tcase %s_%s:\n\t\treturn %s\n", enumName, field.Name.Name, tmp)
	}
	out += "\tdefault:\n\t\tpanic(\"\")\n\t}\n}\n"
	for _, field := range expr.Values.List {
		var tmp []string
		var tmp1 []string
		if field.Params != nil {
			for i, el := range field.Params.List {
				tmp = append(tmp, fmt.Sprintf("arg%d %s", i, g.env.GetType2(el.Type, g.fset).GoStr()))
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
	return fmt.Sprintf("%s.(%s)", content2, content1)
	//return fmt.Sprintf("AglTypeAssert[%s](%s)", content1, content2)
}

func (g *Generator) genStarExpr(expr *ast.StarExpr) string {
	content1 := g.genExpr(expr.X)
	return fmt.Sprintf("*%s", content1)
}

func (g *Generator) genMapType(expr *ast.MapType) string {
	content1 := g.genExpr(expr.Key)
	var content2 string
	if g.isType {
		content2 = g.env.GetType2(expr.Value, g.fset).GoStrType()
	} else {
		content2 = g.env.GetType2(expr.Value, g.fset).GoStr()
	}
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
	return fmt.Sprintf("MakeResultErr[%s](%s)", g.env.GetType(expr).(types.ErrType).T.GoStrType(), content1)
}

func (g *Generator) genChanType(expr *ast.ChanType) string {
	return fmt.Sprintf("chan %s", g.genExpr(expr.Value))
}

func getCheck(t types.Type) string {
	switch t.(type) {
	case types.ResultType:
		return "IsErr()"
	case types.OptionType:
		return "IsNone()"
	default:
		panic("")
	}
}

func (g *Generator) genOrBreakExpr(expr *ast.OrBreakExpr) (out string) {
	content1 := g.genExpr(expr.X)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	gPrefix := g.prefix
	before := ""
	before += gPrefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += gPrefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	before += gPrefix + "\tbreak"
	if expr.Label != nil {
		before += " " + expr.Label.String()
	}
	before += "\n"
	before += gPrefix + "}\n"
	g.beforeStmt = append(g.beforeStmt, NewBeforeStmt(before))
	return fmt.Sprintf("AglIdentity(%s).Unwrap()", varName)
}

func (g *Generator) genOrContinueExpr(expr *ast.OrContinueExpr) (out string) {
	content1 := g.genExpr(expr.X)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := ""
	gPrefix := g.prefix
	before += gPrefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += gPrefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	before += gPrefix + "\tcontinue"
	if expr.Label != nil {
		before += " " + expr.Label.String()
	}
	before += "\n"
	before += gPrefix + "}\n"
	g.beforeStmt = append(g.beforeStmt, NewBeforeStmt(before))
	return fmt.Sprintf("AglIdentity(%s).Unwrap()", varName)
}

func (g *Generator) genOrReturn(expr *ast.OrReturnExpr) (out string) {
	content1 := g.genExpr(expr.X)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := ""
	before += g.prefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check)
	if g.returnType == nil {
		before += g.prefix + "\treturn\n"
	} else {
		switch retT := g.returnType.(type) {
		case types.ResultType:
			before += g.prefix + fmt.Sprintf("\treturn MakeResultErr[%s](%s.Err())\n", retT.W.GoStrType(), varName)
		case types.OptionType:
			before += g.prefix + fmt.Sprintf("\treturn MakeOptionNone[%s]()\n", retT.W.GoStrType())
		case types.VoidType:
			before += g.prefix + fmt.Sprintf("\treturn\n")
		default:
			assert(false, "cannot use or_return in a function that does not return void/Option/Result")
		}
	}
	before += g.prefix + "}\n"
	g.beforeStmt = append(g.beforeStmt, NewBeforeStmt(before))
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

func (g *Generator) genMatchExpr(expr *ast.MatchExpr) (out string) {
	content1 := strings.TrimSpace(g.genExpr(expr.Init))
	initT := g.env.GetType(expr.Init)
	id := g.varCounter.Add(1)
	varName := fmt.Sprintf(`aglTmp%d`, id)
	errName := fmt.Sprintf(`aglTmpErr%d`, id)
	gPrefix := g.prefix
	switch v := initT.(type) {
	case types.ResultType:
		if v.Native {
			out += fmt.Sprintf("%s, %s := AglWrapNative2(%s).NativeUnwrap()\n", varName, errName, content1)
		} else {
			out += fmt.Sprintf("%s := %s\n", varName, content1)
		}
		if expr.Body != nil {
			for _, c := range expr.Body.List {
				c := c.(*ast.MatchClause)
				if v.Native {
					switch v := c.Expr.(type) {
					case *ast.OkExpr:
						binding := g.genExpr(v.X)
						assignOp := utils.Ternary(binding == "_", "=", ":=")
						out += gPrefix + fmt.Sprintf("if %s == nil {\n%s\t%s %s *%s\n", errName, gPrefix, binding, assignOp, varName)
					case *ast.ErrExpr:
						binding := g.genExpr(v.X)
						assignOp := utils.Ternary(binding == "_", "=", ":=")
						out += gPrefix + fmt.Sprintf("if %s != nil {\n%s\t%s %s %s\n", errName, gPrefix, binding, assignOp, errName)
					default:
						panic("")
					}
				} else {
					switch v := c.Expr.(type) {
					case *ast.OkExpr:
						out += gPrefix + fmt.Sprintf("if %s.IsOk() {\n%s\t%s := %s.Unwrap()\n", varName, gPrefix, g.genExpr(v.X), varName)
					case *ast.ErrExpr:
						out += gPrefix + fmt.Sprintf("if %s.IsErr() {\n%s\t%s := %s.Err()\n", varName, gPrefix, g.genExpr(v.X), varName)
					default:
						panic("")
					}
				}
				content3 := g.incrPrefix(func() string {
					return g.genStmts(c.Body)
				})
				out += content3
				out += gPrefix + "}\n"
			}
		}
	case types.OptionType:
		out += gPrefix + fmt.Sprintf("%s := %s\n", varName, content1)
		if expr.Body != nil {
			for _, c := range expr.Body.List {
				c := c.(*ast.MatchClause)
				switch v := c.Expr.(type) {
				case *ast.SomeExpr:
					out += gPrefix + fmt.Sprintf("if %s.IsSome() {\n%s\t%s := %s.Unwrap()\n", varName, gPrefix, g.genExpr(v.X), varName)
				case *ast.NoneExpr:
					out += gPrefix + fmt.Sprintf("if %s.IsNone() {\n", varName)
				default:
					panic("")
				}
				content3 := g.incrPrefix(func() string {
					return g.genStmts(c.Body)
				})
				out += content3
				out += gPrefix + "}\n"
			}
		}
	case types.EnumType:
		if expr.Body != nil {
			for i, cc := range expr.Body.List {
				out += gPrefix + fmt.Sprintf("if %s.tag == %s_%s {\n", expr.Init, v.Name, v.Fields[i].Name)
				c := cc.(*ast.MatchClause)
				switch cv := c.Expr.(type) {
				case *ast.CallExpr:
					for j, id := range cv.Args {
						rhs := fmt.Sprintf("%s.%s_%d", expr.Init, v.Fields[i].Name, j)
						if id.(*ast.Ident).Name == "_" {
							out += gPrefix + fmt.Sprintf("\t_ = %s\n", rhs)
						} else {
							out += gPrefix + fmt.Sprintf("\t%s := %s\n", g.genExpr(id), rhs)
						}
					}
					out += gPrefix + g.genStmts(c.Body)
				default:
					panic("")
				}
				out += gPrefix + fmt.Sprintf("}\n")
			}
		}
	default:
		panic(fmt.Sprintf("%v", to(initT)))
	}
	out = strings.TrimSpace(out)
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
		tmp := utils.MapJoin(expr.List, func(el ast.Expr) string { return g.genExpr(el) }, ", ")
		listStr = "case " + tmp + ":\n"
	} else {
		listStr = "default:\n"
	}
	var content1 string
	if expr.Body != nil {
		content1 = g.incrPrefix(func() string {
			return g.genStmts(expr.Body)
		})
	}
	out += g.prefix + listStr
	out += content1
	return
}

func (g *Generator) genSwitchStmt(expr *ast.SwitchStmt) (out string) {
	var content1 string
	if expr.Init != nil {
		content1 = strings.TrimSpace(g.genStmt(expr.Init))
		content1 = utils.SuffixIf(content1, " ")
	}
	var content2 string
	if expr.Tag != nil {
		content2 = g.genExpr(expr.Tag)
		content2 = utils.SuffixIf(content2, " ")
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
		content1 = fmt.Sprintf("case %s:", strings.TrimSpace(g.genStmt(expr.Comm)))
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
	nT := types.ReplGenM(g.env.GetType(expr), g.genMap)
	var typeStr string
	switch v := nT.(type) {
	case types.NoneType:
		typeStr = v.W.GoStrType()
	case types.TypeType:
		typeStr = v.GoStrType()
	default:
		panic("")
	}
	return fmt.Sprintf("MakeOptionNone[%s]()", typeStr)
}

func (g *Generator) genInterfaceType(expr *ast.InterfaceType) (out string) {
	if expr.Methods == nil || len(expr.Methods.List) == 0 {
		return "interface{}"
	}
	out += "interface {\n"
	if expr.Methods != nil {
		for _, m := range expr.Methods.List {
			content1 := g.env.GetType(m.Type).(types.FuncType).GoStr1()
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
	return fmt.Sprintf("(%s)", content1)
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
	gPrefix := g.prefix
	out += gPrefix + "struct {\n"
	for _, field := range expr.Fields.List {
		content1 := g.genExpr(field.Type)
		var namesArr []string
		for _, name := range field.Names {
			namesArr = append(namesArr, name.Name)
		}
		var content2 string
		if field.Tag != nil {
			content2 = g.genExpr(field.Tag)
			content2 = utils.PrefixIf(content2, " ")
		}
		names := strings.Join(namesArr, ", ")
		out += gPrefix + "\t" + names + " " + content1 + content2 + "\n"
	}
	out += gPrefix + "}"
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
		typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
	}
	if params := expr.Params; params != nil {
		var fieldsItems []string
		for _, field := range params.List {
			var content string
			if _, ok := field.Type.(*ast.TupleExpr); ok {
				content = g.env.GetType(field.Type).GoStr()
			} else {
				content = g.genExpr(field.Type)
			}
			namesStr := utils.MapJoin(field.Names, func(n *ast.Ident) string { return n.Name }, ", ")
			namesStr = utils.SuffixIf(namesStr, " ")
			fieldsItems = append(fieldsItems, namesStr+content)
		}
		paramsStr = strings.Join(fieldsItems, ", ")
	}
	content1 = utils.PrefixIf(content1, " ")
	return fmt.Sprintf("func%s(%s)%s", typeParamsStr, paramsStr, content1)
}

func (g *Generator) genIndexExpr(expr *ast.IndexExpr) string {
	content1 := g.genExpr(expr.X)
	content2 := g.genExpr(expr.Index)
	//switch g.env.GetType(expr.X).(type) {
	//case types.MapType:
	//	return fmt.Sprintf("AglMapIndex(%s, %s)", content1, content2)
	//}
	return fmt.Sprintf("%s[%s]", content1, content2)
}

func (g *Generator) genDumpExpr(expr *ast.DumpExpr) string {
	content1 := g.genExpr(expr.X)
	safeContent1 := strconv.Quote(content1)
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	before := g.prefix + fmt.Sprintf("%s := %s\n", varName, content1)
	before += g.prefix + fmt.Sprintf("fmt.Printf(\"%s: %%s: %%v\\n\", %s, %s)\n", g.fset.Position(expr.X.Pos()), safeContent1, varName)
	g.beforeStmt = append(g.beforeStmt, NewBeforeStmt(before))
	return content1
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
	exprXT := MustCast[types.OptionType](g.env.GetInfo(expr.X).Type)
	if exprXT.Bubble {
		content1 := g.genExpr(expr.X)
		if exprXT.Native {
			varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
			tmpl := varName + ", ok := %s\nif !ok {\n\treturn MakeOptionNone[%s]()\n}\n"
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.W.GoStr()), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before)
			return fmt.Sprintf(`AglIdentity(%s)`, varName)
		} else {
			varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
			tmpl := fmt.Sprintf("%s := %%s\nif %s.IsNone() {\n\treturn MakeOptionNone[%s]()\n}\n", varName, varName, g.returnType.(types.OptionType).W)
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before2)
			out += fmt.Sprintf("%s.Unwrap()", varName)
		}
	} else {
		if exprXT.Native {
			content1 := g.genExpr(expr.X)
			id := g.varCounter.Add(1)
			varName := fmt.Sprintf("aglTmpVar%d", id)
			errName := fmt.Sprintf("aglTmpErr%d", id)
			tmpl1 := fmt.Sprintf("%s, %s := %%s\nif %s != nil {\n\tpanic(%s)\n}\n", varName, errName, errName, errName)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), g.prefix))
			out := fmt.Sprintf(`AglIdentity(%s)`, varName)
			g.beforeStmt = append(g.beforeStmt, before)
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
			errName := fmt.Sprintf("aglTmpErr%d", g.varCounter.Add(1))
			tmpl := fmt.Sprintf("if %s := %%s; %s != nil {\n\treturn MakeResultErr[%%s](%s)\n}\n", errName, errName, errName)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.W.GoStrType()), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before)
			return `AglNoop()`
		} else if exprXT.Native {
			id := g.varCounter.Add(1)
			varName := fmt.Sprintf("aglTmpVar%d", id)
			errName := fmt.Sprintf("aglTmpErr%d", id)
			tmpl := fmt.Sprintf("%s, %s := %%s\nif %s != nil {\n\treturn MakeResultErr[%%s](%s)\n}\n", varName, errName, errName, errName)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, g.returnType.(types.ResultType).W.GoStrType()), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before)
			return fmt.Sprintf(`AglIdentity(%s)`, varName)
		} else if exprXT.ConvertToNone {
			varName := fmt.Sprintf("aglTmpVar%d", g.varCounter.Add(1))
			tmpl := fmt.Sprintf("%s := %%s\nif %s.IsErr() {\n\treturn MakeOptionNone[%%s]()\n}\n", varName, varName)
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1, exprXT.ToNoneType.GoStrType()), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before2)
			out += varName + ".Unwrap()"
		} else {
			varName := fmt.Sprintf("aglTmpVar%d", g.varCounter.Add(1))
			tmpl := fmt.Sprintf("%s := %%s\nif %s.IsErr() {\n\treturn %s\n}\n", varName, varName, varName)
			before2 := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl, content1), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before2)
			out += varName + ".Unwrap()"
		}
	} else {
		if exprXT.Native {
			var tmpl1 string
			if _, ok := exprXT.W.(types.VoidType); ok {
				tmpErrVar := fmt.Sprintf("aglTmpErr%d", g.varCounter.Add(1))
				tmpl1 = fmt.Sprintf("%s := %%s\nif %s != nil {\n\tpanic(%s)\n}\n", tmpErrVar, tmpErrVar, tmpErrVar)
				out = `AglNoop()`
			} else {
				id := g.varCounter.Add(1)
				varName := fmt.Sprintf("aglTmp%d", id)
				errName := fmt.Sprintf("aglTmpErr%d", id)
				tmpl1 = fmt.Sprintf("%s, %s := %%s\nif %s != nil {\n\tpanic(%s)\n}\n", varName, errName, errName, errName)
				out = fmt.Sprintf(`AglIdentity(%s)`, varName)
			}
			content1 := g.genExpr(expr.X)
			before := NewBeforeStmt(addPrefix(fmt.Sprintf(tmpl1, content1), g.prefix))
			g.beforeStmt = append(g.beforeStmt, before)
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
		eXT := g.env.GetType(e.X)
		eXT = types.Unwrap(eXT)
		switch eXT.(type) {
		case types.ArrayType:
			fnName := e.Sel.Name
			if fnName == "Filter" {
				return fmt.Sprintf("AglVecFilter(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "AllSatisfy" {
				return fmt.Sprintf("AglVecAllSatisfy(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Contains" {
				return fmt.Sprintf("AglVecContains(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Any" {
				return fmt.Sprintf("AglVecAny(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Map" {
				return fmt.Sprintf("AglVecMap(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Reduce" {
				return fmt.Sprintf("AglReduce(%s, %s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]), g.genExpr(expr.Args[1]))
			} else if fnName == "Find" {
				return fmt.Sprintf("AglVecFind(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Sum" {
				return fmt.Sprintf("AglVecSum(%s)", g.genExpr(e.X))
			} else if fnName == "Last" {
				return fmt.Sprintf("AglVecLast(%s)", g.genExpr(e.X))
			} else if fnName == "First" {
				return fmt.Sprintf("AglVecFirst(%s)", g.genExpr(e.X))
			} else if fnName == "Len" {
				return fmt.Sprintf("AglVecLen(%s)", g.genExpr(e.X))
			} else if fnName == "IsEmpty" {
				return fmt.Sprintf("AglVecIsEmpty(%s)", g.genExpr(e.X))
			} else if fnName == "Insert" {
				return fmt.Sprintf("AglVecInsert(%s, %s ,%s)", g.genExpr(e.X), g.genExpr(expr.Args[0]), g.genExpr(expr.Args[1]))
			} else if fnName == "Remove" {
				return fmt.Sprintf("AglVecRemove(&%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Clone" {
				return fmt.Sprintf("AglVecClone(%s)", g.genExpr(e.X))
			} else if fnName == "Indices" {
				return fmt.Sprintf("AglVecIndices(%s)", g.genExpr(e.X))
			} else if fnName == "Pop" {
				return fmt.Sprintf("AglVecPop(&%s)", g.genExpr(e.X))
			} else if fnName == "PopFront" {
				return fmt.Sprintf("AglVecPopFront(&%s)", g.genExpr(e.X))
			} else if fnName == "PopIf" {
				return fmt.Sprintf("AglVecPopIf(&%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Push" {
				var params []string
				for _, el := range expr.Args {
					params = append(params, g.genExpr(el))
				}
				return fmt.Sprintf("AglVecPush(&%s, %s)", g.genExpr(e.X), strings.Join(params, ", "))
			} else if fnName == "PushFront" {
				return fmt.Sprintf("AglVecPushFront(&%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Joined" {
				return fmt.Sprintf("AglJoined(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			} else if fnName == "Sorted" {
				return fmt.Sprintf("AglVecSorted(%s)", g.genExpr(e.X))
			} else if fnName == "Iter" {
				return fmt.Sprintf("AglVecIter(%s)", g.genExpr(e.X))
			} else {
				extName := "agl1.Vec." + fnName
				t := g.env.Get(extName)
				rawFnT := t
				concreteT := g.env.GetType(expr.Fun)
				m := types.FindGen(rawFnT, concreteT)
				tmp := g.extensions[extName]
				if tmp.gen == nil {
					tmp.gen = make(map[string]ExtensionTest)
				}
				tmp.gen[rawFnT.String()+"_"+concreteT.String()] = ExtensionTest{raw: rawFnT, concrete: concreteT}
				g.extensions[extName] = tmp
				var els []string
				for _, k := range slices.Sorted(maps.Keys(m)) {
					els = append(els, fmt.Sprintf("%s_%s", k, m[k].GoStr()))
				}
				if _, ok := m["T"]; !ok {
					recvTName := rawFnT.(types.FuncType).TypeParams[0].(types.GenericType).W.GoStr()
					els = append(els, fmt.Sprintf("%s_%s", "T", recvTName))
				}
				elsStr := strings.Join(els, "_")
				content2 := utils.PrefixIf(g.genExprs(expr.Args), ", ")
				return fmt.Sprintf("AglVec%s_%s(%s%s)", fnName, elsStr, g.genExpr(e.X), content2)
			}
		case types.SetType:
			switch e.Sel.Name {
			case "Insert":
				return fmt.Sprintf("AglSetInsert(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Remove":
				return fmt.Sprintf("AglSetRemove(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Contains":
				return fmt.Sprintf("AglSetContains(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Union":
				return fmt.Sprintf("AglSetUnion(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "FormUnion":
				return fmt.Sprintf("AglSetFormUnion(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Subtracting":
				return fmt.Sprintf("AglSetSubtracting(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Subtract":
				return fmt.Sprintf("AglSetSubtract(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Intersection":
				return fmt.Sprintf("AglSetIntersection(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "FormIntersection":
				return fmt.Sprintf("AglSetFormIntersection(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "SymmetricDifference":
				return fmt.Sprintf("AglSetSymmetricDifference(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "FormSymmetricDifference":
				return fmt.Sprintf("AglSetFormSymmetricDifference(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "IsSubset":
				return fmt.Sprintf("AglSetIsSubset(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "IsStrictSubset":
				return fmt.Sprintf("AglSetIsStrictSubset(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "IsSuperset":
				return fmt.Sprintf("AglSetIsSuperset(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "IsStrictSuperset":
				return fmt.Sprintf("AglSetIsStrictSuperset(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Equals":
				return fmt.Sprintf("AglSetEquals(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "IsDisjoint":
				return fmt.Sprintf("AglSetIsDisjoint(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Intersects":
				return fmt.Sprintf("AglSetIntersects(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Len":
				return fmt.Sprintf("AglSetLen(%s)", g.genExpr(e.X))
			case "Min":
				return fmt.Sprintf("AglSetMin(%s)", g.genExpr(e.X))
			case "Max":
				return fmt.Sprintf("AglSetMax(%s)", g.genExpr(e.X))
			case "Iter":
				return fmt.Sprintf("AglSetIter(%s)", g.genExpr(e.X))
			}
		case types.StringType, types.UntypedStringType:
			switch e.Sel.Name {
			case "Split":
				return fmt.Sprintf("AglStringSplit(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Replace":
				return fmt.Sprintf("AglStringReplace(%s, %s, %s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]), g.genExpr(expr.Args[1]), g.genExpr(expr.Args[2]))
			case "ReplaceAll":
				return fmt.Sprintf("AglStringReplaceAll(%s, %s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]), g.genExpr(expr.Args[1]))
			case "TrimSpace":
				return fmt.Sprintf("AglStringTrimSpace(%s)", g.genExpr(e.X))
			case "TrimPrefix":
				return fmt.Sprintf("AglStringTrimPrefix(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "HasPrefix":
				return fmt.Sprintf("AglStringHasPrefix(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "HasSuffix":
				return fmt.Sprintf("AglStringHasSuffix(%s, %s)", g.genExpr(e.X), g.genExpr(expr.Args[0]))
			case "Lowercased":
				return fmt.Sprintf("AglStringLowercased(%s)", g.genExpr(e.X))
			case "Uppercased":
				return fmt.Sprintf("AglStringUppercased(%s)", g.genExpr(e.X))
			case "Int":
				return fmt.Sprintf("AglStringInt(%s)", g.genExpr(e.X))
			case "I8":
				return fmt.Sprintf("AglStringI8(%s)", g.genExpr(e.X))
			case "I16":
				return fmt.Sprintf("AglStringI16(%s)", g.genExpr(e.X))
			case "I32":
				return fmt.Sprintf("AglStringI32(%s)", g.genExpr(e.X))
			case "I64":
				return fmt.Sprintf("AglStringI64(%s)", g.genExpr(e.X))
			case "Uint":
				return fmt.Sprintf("AglStringUint(%s)", g.genExpr(e.X))
			case "U8":
				return fmt.Sprintf("AglStringU8(%s)", g.genExpr(e.X))
			case "U16":
				return fmt.Sprintf("AglStringU16(%s)", g.genExpr(e.X))
			case "U32":
				return fmt.Sprintf("AglStringU32(%s)", g.genExpr(e.X))
			case "U64":
				return fmt.Sprintf("AglStringU64(%s)", g.genExpr(e.X))
			case "F32":
				return fmt.Sprintf("AglStringF32(%s)", g.genExpr(e.X))
			case "F64":
				return fmt.Sprintf("AglStringF64(%s)", g.genExpr(e.X))
			}
		case types.MapType:
			if e.Sel.Name == "Get" {
				content1 := g.genExpr(e.X)
				content2 := g.genExpr(expr.Args[0])
				return fmt.Sprintf("AglIdentity(AglMapIndex(%s, %s))", content1, content2)
			} else if e.Sel.Name == "Keys" {
				content1 := g.genExpr(e.X)
				return fmt.Sprintf("AglIdentity(AglMapKeys(%s))", content1)
			} else if e.Sel.Name == "Values" {
				content1 := g.genExpr(e.X)
				return fmt.Sprintf("AglIdentity(AglMapValues(%s))", content1)
			}
		default:
			if v, ok := e.X.(*ast.Ident); ok && v.Name == "agl" && e.Sel.Name == "NewSet" {
				content1 := g.genExprs(expr.Args)
				return fmt.Sprintf("AglNewSet(%s)", content1)
			} else if v, ok := e.X.(*ast.Ident); ok && v.Name == "http" && e.Sel.Name == "NewRequest" {
				content1 := g.genExprs(expr.Args)
				return fmt.Sprintf("AglHttpNewRequest(%s)", content1)
			}
		}
	case *ast.Ident:
		if e.Name == "assert" {
			var contents []string
			for _, arg := range expr.Args {
				content1 := g.genExpr(arg)
				contents = append(contents, content1)
			}
			line := g.fset.Position(expr.Pos()).Line
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
	var content1, content2 string
	switch v := expr.Fun.(type) {
	case *ast.Ident:
		t1 := g.env.Get(v.Name)
		if t2, ok := t1.(types.TypeType); ok && TryCast[types.CustomType](t2.W) {
			content1 = expr.Fun.(*ast.Ident).Name
			content2 = g.genExprs(expr.Args)
		} else {
			content1 = g.genExpr(expr.Fun)
			if fnT, ok := t1.(types.FuncType); ok {
				if !InArray(content1, []string{"make", "append", "len", "new", "AglAbs", "min", "max"}) && fnT.IsGeneric() {
					oFnT := g.env.Get(v.Name)
					newFnT := g.env.GetType(expr.Fun)
					fnDecl := g.genFuncDecls[oFnT.String()]
					m := types.FindGen(oFnT, newFnT)
					var outFnDecl string
					g.WithGenMapping(m, func() {
						outFnDecl = g.decrPrefix(func() string {
							return g.genFuncDecl(fnDecl, false) + "\n"
						})
					})
					for _, k := range slices.Sorted(maps.Keys(m)) {
						content1 += fmt.Sprintf("_%s_%s", k, m[k].GoStr())
					}
					g.genFuncDecls2[content1] = outFnDecl
					g.WithGenMapping(m, func() {
						content2 = g.genExprs(expr.Args)
					})
				} else if content1 == "make" {
					content1 = g.genExpr(expr.Fun)
					if g.genMap != nil {
						g.withType(func() { content2 = types.ReplGenM(g.env.GetType(expr.Args[0]), g.genMap).GoStr() })
					} else {
						g.withType(func() { content2 = g.genExpr(expr.Args[0]) })
					}
					if len(expr.Args) > 1 {
						content2 += ", " + utils.MapJoin(expr.Args[1:], func(expr ast.Expr) string { return g.genExpr(expr) }, ", ")
					}
				} else {
					content1 = g.genExpr(expr.Fun)
					content2 = g.genExprs(expr.Args)
				}
			} else {
				content2 = g.genExprs(expr.Args)
			}
		}
	default:
		content1 = g.genExpr(expr.Fun)
		content2 = g.genExprs(expr.Args)
	}
	if expr.Ellipsis != 0 {
		content2 += "..."
	}
	return fmt.Sprintf("%s(%s)", content1, content2)
}

func (g *Generator) genSetType(expr *ast.SetType) string {
	content1 := g.genExpr(expr.Key)
	return fmt.Sprintf("AglSet[%s]", content1)
}

func (g *Generator) genArrayType(expr *ast.ArrayType) (out string) {
	var content string
	switch v := expr.Elt.(type) {
	case *ast.TupleExpr:
		content = types.ReplGenM(g.env.GetType(v), g.genMap).GoStr()
	default:
		content = g.genExpr(expr.Elt)
	}
	return fmt.Sprintf("AglVec[%s]", content)
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
		if TryCast[types.SetType](g.env.GetType(expr.X)) && TryCast[types.SetType](g.env.GetType(expr.Y)) {
			if (op == "==" || op == "!=") && g.env.Get("agl1.Set.Equals") != nil {
				out := fmt.Sprintf("AglSetEquals(%s, %s)", content1, content2)
				if op == "!=" {
					out = "!" + out
				}
				return out
			}
		} else if TryCast[types.StructType](g.env.GetType(expr.X)) && TryCast[types.StructType](g.env.GetType(expr.Y)) {
			lhsName := g.env.GetType(expr.X).(types.StructType).Name
			rhsName := g.env.GetType(expr.Y).(types.StructType).Name
			if lhsName == rhsName {
				if (op == "==" || op == "!=") && g.env.Get(lhsName+".__EQL") != nil {
					out := fmt.Sprintf("%s.__EQL(%s)", content1, content2)
					if op == "!=" {
						out = "!" + out
					}
					return out
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
	var content1, content2 string
	if expr.Type != nil {
		content1 = g.genExpr(expr.Type)
	}
	if content1 == "AglVoid{}" {
		return content1
	}
	if expr.Type != nil && TryCast[types.SetType](g.env.GetType(expr.Type)) {
		var tmp []string
		for _, el := range expr.Elts {
			tmp = append(tmp, fmt.Sprintf("%s: {}", g.genExpr(el)))
		}
		content2 = strings.Join(tmp, ", ")
	} else {
		content2 = g.genExprs(expr.Elts)
	}
	return fmt.Sprintf("%s{%s}", content1, content2)
}

func (g *Generator) genTupleExpr(expr *ast.TupleExpr) (out string) {
	_ = g.genExprs(expr.Values)
	structName := types.ReplGenM(g.env.GetType(expr), g.genMap).(types.TupleType).GoStr()
	structName1 := types.ReplGenM(g.env.GetType(expr), g.genMap).(types.TupleType).GoStr2()
	var args []string
	for i, x := range expr.Values {
		xT := g.env.GetType2(x, g.fset)
		args = append(args, fmt.Sprintf("\tArg%d %s\n", i, types.ReplGenM(xT, g.genMap).GoStr()))
	}
	structStr := fmt.Sprintf("type %s struct {\n", structName1)
	structStr += strings.Join(args, "")
	structStr += fmt.Sprintf("}\n")
	g.tupleStructs[structName] = structStr
	if g.isType {
		return fmt.Sprintf("%s", structName)
	}
	var fields []string
	for i, x := range expr.Values {
		content1 := g.genExpr(x)
		fields = append(fields, fmt.Sprintf("Arg%d: %s", i, content1))
	}
	return fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
}

func (g *Generator) genExprs(e []ast.Expr) (out string) {
	return utils.MapJoin(e, func(expr ast.Expr) string { return g.genExpr(expr) }, ", ")
}

func (g *Generator) genStmts(s []ast.Stmt) (out string) {
	for _, stmt := range s {
		g.WithSub(func() {
			content1 := g.genStmt(stmt)
			var beforeStmtStr string
			for _, b := range g.beforeStmt {
				switch v := b.(type) {
				case *BeforeStmt:
					beforeStmtStr += v.Content()
				}
			}
			out += beforeStmtStr + content1
		})
	}
	return out
}

func (g *Generator) genBlockStmt(stmt *ast.BlockStmt) (out string) {
	return g.genStmts(stmt.List)
}

func (g *Generator) genSpecs(specs []ast.Spec, tok token.Token) (out string) {
	for _, spec := range specs {
		out += g.genSpec(spec, tok)
	}
	return
}

func (g *Generator) genSpec(s ast.Spec, tok token.Token) (out string) {
	switch spec := s.(type) {
	case *ast.ValueSpec:
		var content1 string
		if spec.Type != nil {
			g.withType(func() {
				content1 = g.genExpr(spec.Type)
			})
		}
		var namesArr []string
		for _, name := range spec.Names {
			namesArr = append(namesArr, name.Name)
		}
		out += g.prefix + tok.String() + " " + strings.Join(namesArr, ", ")
		if content1 != "" {
			out += " " + content1
		}
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
				typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
			}
			content1 := g.genExpr(spec.Type)
			out += g.prefix + "type " + spec.Name.Name + typeParamsStr + " " + content1 + "\n"
		}
	case *ast.ImportSpec:
		//if spec.Name != nil {
		//	out += "import " + spec.Name.Name + "\n"
		//}
		//p("?!", spec.Name)
	default:
		panic(fmt.Sprintf("%v", to(s)))
	}
	return
}

func (g *Generator) genDecl(d ast.Decl, first bool) (out string) {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return g.genGenDecl(decl)
	case *ast.FuncDecl:
		out1 := g.genFuncDecl(decl, first)
		for _, b := range g.before {
			if b != nil {
				out += b.Content()
			}
		}
		clear(g.before)
		out += utils.SuffixIf(out1, "\n")
		return
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
	return
}

func (g *Generator) genGenDecl(decl *ast.GenDecl) string {
	return g.genSpecs(decl.Specs, decl.Tok)
}

func (g *Generator) genDeclStmt(stmt *ast.DeclStmt) string {
	return g.genDecl(stmt.Decl, false)
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
	tmp = utils.SuffixIf(tmp, " ")
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
	t := g.env.GetType(stmt.Rhs[0])
	if v, ok := t.(types.CustomType); ok {
		t = v.W
	}
	if len(stmt.Rhs) == 1 && TryCast[types.EnumType](t) {
		rhsT := t.(types.EnumType)
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
	} else if len(stmt.Rhs) == 1 && TryCast[types.TupleType](t) {
		if len(stmt.Lhs) == 1 {
			content1 := g.genExprs(stmt.Lhs)
			lhs = content1
		} else {
			if v, ok := t.(types.TupleType); ok && v.KeepRaw {
				lhs = g.genExprs(stmt.Lhs)
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
		}
	} else {
		lhs = g.genExprs(stmt.Lhs)
	}
	content2 := g.genExprs(stmt.Rhs)
	if len(stmt.Rhs) == 1 {
		if v, ok := g.env.GetType(stmt.Rhs[0]).(types.ResultType); ok && v.Native {
			switch tup := v.W.(type) {
			case types.TupleType:
				panic(fmt.Sprintf("need to implement AglWrapNative for tuple len %d", len(tup.Elts)))
			default:
				if v.KeepRaw {
					content2 = fmt.Sprintf("%s", content2)
				} else {
					content2 = fmt.Sprintf("AglWrapNative2(%s)", content2)
				}
			}
		}
	}
	out = g.prefix + fmt.Sprintf("%s %s %s\n", lhs, stmt.Tok.String(), content2)
	//out += g.prefix + fmt.Sprintf("AglNoop(%s)\n", lhs) // Allow to have "declared and not used" variables
	out += after
	return out
}

func (g *Generator) genIfLetStmt(stmt *ast.IfLetStmt) (out string) {
	gPrefix := g.prefix
	ass := stmt.Ass
	lhs := g.genExpr(ass.Lhs[0])
	rhs := g.incrPrefix(func() string { return g.genExpr(ass.Rhs[0]) })
	body := g.incrPrefix(func() string { return g.genStmt(stmt.Body) })
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	var cond string
	unwrapFn := "Unwrap"
	switch stmt.Op {
	case token.SOME:
		cond = fmt.Sprintf("%s.IsSome()", varName)
	case token.OK:
		cond = fmt.Sprintf("%s.IsOk()", varName)
	case token.ERR:
		cond = fmt.Sprintf("%s.IsErr()", varName)
		unwrapFn = "Err"
	default:
		panic("")
	}
	out += gPrefix + fmt.Sprintf("if %s := %s; %s {\n", varName, rhs, cond)
	out += gPrefix + fmt.Sprintf("\t%s := %s.%s()\n", lhs, varName, unwrapFn)
	out += body
	if stmt.Else != nil {
		switch stmt.Else.(type) {
		case *ast.IfStmt, *ast.IfLetStmt:
			content3 := g.genStmt(stmt.Else)
			out += gPrefix + "} else " + strings.TrimSpace(content3) + "\n"
		default:
			content3 := g.incrPrefix(func() string {
				return g.genStmt(stmt.Else)
			})
			out += gPrefix + "} else {\n"
			out += content3
			out += gPrefix + "}\n"
		}
	} else {
		out += gPrefix + "}\n"
	}
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
	gPrefix := g.prefix
	out += gPrefix + "if " + initStr + cond + " {\n"
	out += body
	if stmt.Else != nil {
		switch stmt.Else.(type) {
		case *ast.IfStmt, *ast.IfLetStmt:
			content3 := g.genStmt(stmt.Else)
			out += gPrefix + "} else " + strings.TrimSpace(content3) + "\n"
		default:
			content3 := g.incrPrefix(func() string {
				return g.genStmt(stmt.Else)
			})
			out += gPrefix + "} else {\n"
			out += content3
			out += gPrefix + "}\n"
		}
	} else {
		out += gPrefix + "}\n"
	}
	return out
}

func (g *Generator) genDecls() (out string) {
	for _, decl := range g.a.Decls {
		g.genDecl(decl, true)
	}
	for _, decl := range g.a.Decls {
		g.prefix = ""
		content1 := g.genDecl(decl, false)
		out += content1
	}
	return
}

func (g *Generator) genFuncDecl(decl *ast.FuncDecl, first bool) (out string) {
	g.returnType = g.env.GetType(decl).(types.FuncType).Return
	var name, recv, typeParamsStr, paramsStr, resultStr, bodyStr string
	if decl.Recv != nil {
		if len(decl.Recv.List) >= 1 {
			if tmp1, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if tmp2, ok := tmp1.X.(*ast.SelectorExpr); ok {
					if tmp2.Sel.Name == "Vec" {
						fnName := fmt.Sprintf("agl1.Vec.%s", decl.Name.Name)
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
	fnT := g.env.GetType(decl)
	if g.genMap == nil && fnT.(types.FuncType).IsGeneric() {
		g.genFuncDecls[fnT.String()] = decl
		return ""
	}
	if first {
		return ""
	}
	if decl.Name != nil {
		fnName := decl.Name.Name
		if newName, ok := overloadMapping[fnName]; ok {
			fnName = newName
		}
		if decl.Pub.IsValid() {
			fnName = "AglPub_" + fnName
		}
		name = " " + fnName
	}
	if typeParams := decl.Type.TypeParams; typeParams != nil {
		typeParamsStr = g.joinList(decl.Type.TypeParams)
		typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
	}
	if params := decl.Type.Params; params != nil {
		var fieldsItems []string
		for _, field := range params.List {
			var content string
			if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
				if _, ok := v.W.(types.FuncType); ok {
					content = types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStr1()
				} else {
					content = types.ReplGenM(v.W, g.genMap).GoStr()
				}
			} else {
				switch field.Type.(type) {
				case *ast.TupleExpr:
					content = g.env.GetType(field.Type).GoStr()
				default:
					content = g.genExpr(field.Type)
				}
			}
			namesStr := utils.MapJoin(field.Names, func(n *ast.Ident) string { return n.Name }, ", ")
			namesStr = utils.SuffixIf(namesStr, " ")
			fieldsItems = append(fieldsItems, namesStr+content)
		}
		paramsStr = strings.Join(fieldsItems, ", ")
	}
	if result := decl.Type.Result; result != nil {
		resultStr = types.ReplGenM(g.env.GetType(result), g.genMap).GoStr()
		resultStr = utils.PrefixIf(resultStr, " ")
	}
	if decl.Body != nil {
		content := g.incrPrefix(func() string {
			return g.genStmt(decl.Body)
		})
		bodyStr = content
	}
	if g.genMap != nil {
		typeParamsStr = ""
		for _, k := range slices.Sorted(maps.Keys(g.genMap)) {
			v := g.genMap[k]
			name += fmt.Sprintf("_%v_%v", k, v.GoStr())
		}
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
		content := g.genExpr(field.Type)
		namesStr := utils.MapJoin(field.Names, func(n *ast.Ident) string {
			return g.genIdent(n)
		}, ", ")
		namesStr = utils.SuffixIf(namesStr, " ")
		fieldsItems = append(fieldsItems, namesStr+content)
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

func GenHeaders() string {
	return `import (
	aglImportCmp "cmp"
	aglImportFmt "fmt"
	aglImportIo "io"
	aglImportHttp "net/http"
	aglImportStrings "strings"
	aglImportIter "iter"
	aglImportMaps "maps"
	aglImportSlices "slices"
	aglImportMath "math"
	aglImportStrconv "strconv"
)`
}

func GenCore() string {
	out := "package main\n"
	out += GenHeaders()
	out += GenContent()
	return out
}

func GenContent() string {
	return `
func AglWrapNative2[T any](v1 T, err error) Result[T] {
	if err != nil {
		return MakeResultErr[T](err)
	}
	return MakeResultOk(v1)
}

type AglVoid struct{}

type Option[T any] struct {
	t *T
}

func (o Option[T]) String() string {
	if o.IsNone() {
		return "None"
	}
	return aglImportFmt.Sprintf("Some(%v)", *o.t)
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

func (o Option[T]) UnwrapOrDefault() T {
	var zero T
	if o.IsNone() {
		return zero
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
		return aglImportFmt.Sprintf("Err(%v)", r.e)
	}
	return aglImportFmt.Sprintf("Ok(%v)", *r.t)
}

func (r Result[T]) IsErr() bool {
	return r.e != nil
}

func (r Result[T]) IsOk() bool {
	return r.e == nil
}

func (r Result[T]) Unwrap() T {
	if r.IsErr() {
		panic(aglImportFmt.Sprintf("unwrap on an Err value: %s", r.e))
	}
	return *r.t
}

func (r Result[T]) NativeUnwrap() (*T, error) {
	return r.t, r.e
}

func (r Result[T]) UnwrapOr(d T) T {
	if r.IsErr() {
		return d
	}
	return *r.t
}

func (r Result[T]) UnwrapOrDefault() T {
	var zero T
	if r.IsErr() {
		return zero
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

func AglVecAllSatisfy[T any](a []T, f func(T) bool) bool {
	for _, v := range a {
		if !f(v) {
			return false
		}
	}
	return true
}

func AglVecContains[T comparable](a []T, e T) bool {
	for _, v := range a {
		if v == e {
			return true
		}
	}
	return false
}

func AglVecAny[T any](a []T, f func(T) bool) bool {
	for _, v := range a {
		if f(v) {
			return true
		}
	}
	return false
}

func AglReduce[T any, R aglImportCmp.Ordered](a []T, acc R, f func(R, T) R) R {
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

func AglVecIn[T aglImportCmp.Ordered](a []T, v T) bool {
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

type AglVec[T any] []T

func (v AglVec[T]) Iter() aglImportIter.Seq[T] {
	return func(yield func(T) bool) {
		for _, e := range v {
			if !yield(e) {
				return
			}
		}
	}
}

func AglVecIter[T any](v AglVec[T]) aglImportIter.Seq[T] {
	return v.Iter()
}

type AglSet[T comparable] map[T]struct{}

type Iterator[T any] interface {
	Iter() aglImportIter.Seq[T]
}

func (s AglSet[T]) Iter() aglImportIter.Seq[T] {
	return func(yield func(T) bool) {
		for k := range s {
			if !yield(k) {
				return
			}
		}
	}
}

func AglSetIter[T aglImportCmp.Ordered](s AglSet[T]) aglImportIter.Seq[T] {
	return s.Iter()
}

func (s AglSet[T]) String() string {
	var tmp []string
	for k := range s {
		tmp = append(tmp, aglImportFmt.Sprintf("%v", k))
	}
	return aglImportFmt.Sprintf("set(%s)", aglImportStrings.Join(tmp, " "))
}

func AglSetLen[T comparable](s AglSet[T]) int {
	return len(s)
}

func AglSetMin[T aglImportCmp.Ordered](s AglSet[T]) Option[T] {
	if len(s) == 0 {
		return MakeOptionNone[T]()
	}
	keys := aglImportSlices.Sorted(aglImportMaps.Keys(s))
	out := keys[0]
	for _, k := range keys {
		out = min(out, k)
	}
	return MakeOptionSome(out) 
}

func AglSetMax[T aglImportCmp.Ordered](s AglSet[T]) Option[T] {
	if len(s) == 0 {
		return MakeOptionNone[T]()
	}
	keys := aglImportSlices.Sorted(aglImportMaps.Keys(s))
	out := keys[0]
	for _, k := range keys {
		out = max(out, k)
	}
	return MakeOptionSome(out) 
}

// AglSetEquals returns a Boolean value indicating whether two sets have equal elements.
func AglSetEquals[T comparable](s, other AglSet[T]) bool {
	if len(s) != len(other) {
	    return false
	}
	for k := range s {
	    if _, ok := other[k]; !ok {
	        return false
	    }
	}
	return true
}

func AglSetInsert[T comparable](s AglSet[T], el T) bool {
	if _, ok := s[el]; ok {
		return false
	}
	s[el] = struct{}{}
	return true
}

// AglSetContains returns a Boolean value that indicates whether the given element exists in the set.
func AglSetContains[T comparable](s AglSet[T], el T) bool {
	_, ok := s[el]
	return ok
}

// AglSetRemove removes the specified element from the set.
// Return: The value of the member parameter if it was a member of the set; otherwise, nil.
func AglSetRemove[T comparable](s AglSet[T], el T) Option[T] {
	if _, ok := s[el]; ok {
		delete(s, el)
		return MakeOptionSome(el)	 
	}
	return MakeOptionNone[T]()
}

// AglSetUnion returns a new set with the elements of both this set and the given sequence.
func AglSetUnion[T comparable](s AglSet[T], other Iterator[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		newSet[k] = struct{}{}
	}
	for k := range other.Iter() {
		newSet[k] = struct{}{}	
	}
	return newSet
}


// AglSetFormUnion inserts the elements of the given sequence into the set.
func AglSetFormUnion[T comparable](s AglSet[T], other Iterator[T]) {
	for k := range other.Iter() {
		s[k] = struct{}{}	
	}
}

func AglSetSubtracting[T comparable](s AglSet[T], other Iterator[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		newSet[k] = struct{}{}
	}
	for k := range other.Iter() {
		delete(newSet, k)	
	}
	return newSet
}

func AglSetSubtract[T comparable](s AglSet[T], other Iterator[T]) {
	for k := range other.Iter() {
		delete(s, k)
	}
}

// AglSetIntersection returns a new set with the elements that are common to both this set and the given sequence.
func AglSetIntersection[T comparable](s, other AglSet[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		if _, ok := other[k]; ok {
			newSet[k] = struct{}{}
		}
	}
	return newSet
}

// AglSetFormIntersection removes the elements of the set that aren’t also in the given sequence.
func AglSetFormIntersection[T comparable](s, other AglSet[T]) {
	for k := range s {
		if _, ok := other[k]; !ok {
			delete(s, k)
		}
	}
}

// AglSetSymmetricDifference returns a new set with the elements that are either in this set or in the given sequence, but not in both.
func AglSetSymmetricDifference[T comparable](s, other AglSet[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		if _, ok := other[k]; !ok {
			newSet[k] = struct{}{}
		}
	}
	for k := range other {
		if _, ok := s[k]; !ok {
			newSet[k] = struct{}{}
		}
	}
	return newSet
}

// AglSetFormSymmetricDifference removes the elements of the set that are also in the given sequence and adds the members of the sequence that are not already in the set.
func AglSetFormSymmetricDifference[T comparable](s AglSet[T], other Iterator[T]) {
	for k := range other.Iter() {
		if _, ok := s[k]; !ok {
			s[k] = struct{}{}
		} else {
			delete(s, k)
		}
	}
}

// AglSetIsSubset returns a Boolean value that indicates whether the set is a subset of the given sequence.
// Return: true if the set is a subset of possibleSuperset; otherwise, false.
// Set A is a subset of another set B if every member of A is also a member of B.
func AglSetIsSubset[T comparable](s, other AglSet[T]) bool {
	for k := range s {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	return true
}

// AglSetIsStrictSubset returns a Boolean value that indicates whether the set is a strict subset of the given sequence.
// Set A is a strict subset of another set B if every member of A is also a member of B and B contains at least one element that is not a member of A.
func AglSetIsStrictSubset[T comparable](s, other AglSet[T]) bool {
	for k := range s {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	for k := range other {
		if _, ok := s[k]; !ok {
			return true
		}
	}
	return false
}

// AglSetIsSuperset returns a Boolean value that indicates whether this set is a superset of the given set.
// Return: true if the set is a superset of other; otherwise, false.
// Set A is a superset of another set B if every member of B is also a member of A.
func AglSetIsSuperset[T comparable](s AglSet[T], other Iterator[T]) bool {
	for k := range other.Iter() {
		if _, ok := s[k]; !ok {
			return false
		}
	}
	return true
}

// AglSetIsStrictSuperset returns a Boolean value that indicates whether the set is a strict superset of the given sequence.
// Set A is a strict superset of another set B if every member of B is also a member of A and A contains at least one element that is not a member of B.
func AglSetIsStrictSuperset[T comparable](s, other AglSet[T]) bool {
	for k := range other {
		if _, ok := s[k]; !ok {
			return false
		}
	}
	for k := range s {
		if _, ok := other[k]; !ok {
			return true
		}
	}
	return false
}

// AglSetIsDisjoint returns a Boolean value that indicates whether the set has no members in common with the given sequence.
// Return: true if the set has no elements in common with other; otherwise, false.
func AglSetIsDisjoint[T comparable](s AglSet[T], other Iterator[T]) bool {
	var otherSet AglSet[T]	
	if v, ok := other.(AglSet[T]); ok {
		otherSet = v
	} else {
		otherSet = make(AglSet[T])
		for e := range other.Iter() {
			otherSet[e] = struct{}{}
		}
	}
	for k := range s {
		if _, ok := otherSet[k]; ok {
			return false
		}
	}
	return true
}

// AglSetIntersects ...
func AglSetIntersects[T comparable](s AglSet[T], other Iterator[T]) bool {
	return !AglSetIsDisjoint(s, other)
}

func AglStringReplace(s string, old, new string, n int) string {
	return aglImportStrings.Replace(s, old, new, n)
}

func AglStringReplaceAll(s string, old, new string) string {
	return aglImportStrings.ReplaceAll(s, old, new)
}

func AglStringTrimSpace(s string) string {
	return aglImportStrings.TrimSpace(s)
}

func AglStringTrimPrefix(s string, prefix string) string {
	return aglImportStrings.TrimPrefix(s, prefix)
}

func AglStringHasPrefix(s string, prefix string) bool {
	return aglImportStrings.HasPrefix(s, prefix)
}

func AglStringHasSuffix(s string, suffix string) bool {
	return aglImportStrings.HasSuffix(s, suffix)
}

func AglStringSplit(s string, sep string) []string {
	return aglImportStrings.Split(s, sep)
}

func AglStringLowercased(s string) string {
	return aglImportStrings.ToLower(s)
}

func AglStringUppercased(s string) string {
	return aglImportStrings.ToUpper(s)
}

func AglCleanupIntString(s string) (string, int) {
	s = aglImportStrings.ReplaceAll(s, "_", "")
	var base int
	switch {
	case aglImportStrings.HasPrefix(s, "0b"):
		s, base = s[2:], 2
	case aglImportStrings.HasPrefix(s, "0o"):
		s, base = s[2:], 8
	case aglImportStrings.HasPrefix(s, "0x"):
		s, base = s[2:], 16
	case aglImportStrings.HasPrefix(s, "0") && len(s) > 1:
		base = 8 // legacy octal (e.g., 0755)
	default:
		base = 10
	}
	return s, base
}

func AglStringInt(s string) Option[int] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseInt(s, base, 0)
	if err != nil {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(int(v))
}

func AglStringI8(s string) Option[int8] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseInt(s, base, 8)
	if err != nil {
		return MakeOptionNone[int8]()
	}
	return MakeOptionSome(int8(v))
}

func AglStringI16(s string) Option[int16] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseInt(s, base, 16)
	if err != nil {
		return MakeOptionNone[int16]()
	}
	return MakeOptionSome(int16(v))
}

func AglStringI32(s string) Option[int32] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseInt(s, base, 32)
	if err != nil {
		return MakeOptionNone[int32]()
	}
	return MakeOptionSome(int32(v))
}

func AglStringI64(s string) Option[int64] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseInt(s, base, 64)
	if err != nil {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(v)
}

func AglStringUint(s string) Option[uint] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseUint(s, base, 0)
	if err != nil {
		return MakeOptionNone[uint]()
	}
	return MakeOptionSome(uint(v))
}

func AglStringU8(s string) Option[uint8] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseUint(s, base, 8)
	if err != nil {
		return MakeOptionNone[uint8]()
	}
	return MakeOptionSome(uint8(v))
}

func AglStringU16(s string) Option[uint16] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseUint(s, base, 16)
	if err != nil {
		return MakeOptionNone[uint16]()
	}
	return MakeOptionSome(uint16(v))
}

func AglStringU32(s string) Option[uint32] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseUint(s, base, 32)
	if err != nil {
		return MakeOptionNone[uint32]()
	}
	return MakeOptionSome(uint32(v))
}

func AglStringU64(s string) Option[uint64] {
	s, base := AglCleanupIntString(s)
	v, err := aglImportStrconv.ParseUint(s, base, 64)
	if err != nil {
		return MakeOptionNone[uint64]()
	}
	return MakeOptionSome(uint64(v))
}

func AglStringF32(s string) Option[float32] {
	v, err := aglImportStrconv.ParseFloat(s, 32)
	if err != nil {
		return MakeOptionNone[float32]()
	}
	return MakeOptionSome(float32(v))
}

func AglStringF64(s string) Option[float64] {
	v, err := aglImportStrconv.ParseFloat(s, 64)
	if err != nil {
		return MakeOptionNone[float64]()
	}
	return MakeOptionSome(v)
}

func AglVecSorted[E aglImportCmp.Ordered](a []E) []E {
	return aglImportSlices.Sorted(aglImportSlices.Values(a))
}

func AglJoined(a []string, s string) string {
	return aglImportStrings.Join(a, s)
}

func AglVecSum[T aglImportCmp.Ordered](a []T) (out T) {
	var zero T
	return AglReduce(a, zero, func(acc, el T) T { return acc + el })
}

type AglNumber interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

func AglAbs[T AglNumber](e T) (out T) {
	return T(aglImportMath.Abs(float64(e)))
}

func AglVecLast[T any](a []T) (out Option[T]) {
	if len(a) > 0 {
		return MakeOptionSome(a[len(a)-1])
	}
	return MakeOptionNone[T]()
}

func AglVecFirst[T any](a []T) (out Option[T]) {
	if len(a) > 0 {
		return MakeOptionSome(a[0])
	}
	return MakeOptionNone[T]()
}

func AglVecLen[T any](a []T) int {
	return len(a)
}

func AglVecIsEmpty[T any](a []T) bool {
	return len(a) == 0
}

func AglVecPush[T any](a *[]T, els ...T) {
	*a = append(*a, els...)
}

// AglVecPushFront ...
func AglVecPushFront[T any](a *[]T, el T) {
	*a = append([]T{el}, *a...)
}

// AglVecPopFront ...
func AglVecPopFront[T any](a *[]T) Option[T] {
	if len(*a) == 0 {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[0], (*a)[1:]
	return MakeOptionSome(el)
}

// AglVecInsert ...
func AglVecInsert[T any](a *[]T, idx int, el T) {
	*a = append((*a)[:idx], append([]T{el}, (*a)[idx:]...)...)
}

// AglVecRemove ...
func AglVecRemove[T any](a *[]T, idx int) {
	*a = append((*a)[:idx], (*a)[idx+1:]...)
}

// AglVecClone ...
func AglVecClone[S ~[]E, E any](a S) S {
	return aglImportSlices.Clone(a)
}

// AglVecIndices ...
func AglVecIndices[T any](a []T) []int {
	out := make([]int, len(a))
	for i := range a {
		out[i] = i
	}
	return out
}

// AglVecPop removes the last element from a vector and returns it, or None if it is empty.
func AglVecPop[T any](a *[]T) Option[T] {
	if len(*a) == 0 {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[len(*a)-1], (*a)[:len(*a)-1]
	return MakeOptionSome(el)
}

// AglVecPopIf Removes and returns the last element from a vector if the predicate returns true,
// or None if the predicate returns false or the vector is empty (the predicate will not be called in that case).
func AglVecPopIf[T any](a *[]T, pred func() bool) Option[T] {
	if len(*a) == 0 {
		return MakeOptionNone[T]()
	}
	if !pred() {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[len(*a)-1], (*a)[:len(*a)-1]
	return MakeOptionSome(el)
}

func AglMapIndex[K comparable, V any](m map[K]V, index K) Option[V] {
	if el, ok := m[index]; ok {
		return MakeOptionSome(el)
	}
	return MakeOptionNone[V]()
}

func AglMapKeys[K comparable, V any](m map[K]V, index K) aglImportIter.Seq[K] {
	return aglImportMaps.Keys(m)
}

func AglMapValues[K comparable, V any](m map[K]V, index K) aglImportIter.Seq[V] {
	return aglImportMaps.Values(m)
}

func AglHttpNewRequest(method, url string, b Option[aglImportIo.Reader]) Result[*aglImportHttp.Request] {
	var body aglImportIo.Reader
	if b.IsSome() {
		body = b.Unwrap()
	}
	req, err := aglImportHttp.NewRequest(method, url, body)
	if err != nil {
		return MakeResultErr[*aglImportHttp.Request](err)
	}
	return MakeResultOk(req)
}

type Set [T comparable]struct {
	values map[T]struct{}
}

func (s *Set[T]) String() string {
	var vals []string
	for k := range s.values {
		vals = append(vals, aglImportFmt.Sprintf("%v", k))
	}
	return "{" + aglImportStrings.Join(vals, " ") + "}"
}

func (s *Set[T]) Len() int {
	return len(s.values)
}

// Insert Adds a value to the set.
//
// Returns whether the value was newly inserted. That is:
//
// - If the set did not previously contain this value, true is returned.
// - If the set already contained this value, false is returned, and the set is not modified: original value is not replaced, and the value passed as argument is dropped.
func (s *Set[T]) Insert(value T) bool {
	if _, ok := s.values[value]; ok {
		return false
	}
	s.values[value] = struct{}{}
	return true
}

func AglNewSet[T comparable](els ...T) *Set[T] {
	s := &Set[T]{values: make(map[T]struct{})}
	for _, el := range els {
		s.values[el] = struct{}{}
	}
	return s
}
`
}
