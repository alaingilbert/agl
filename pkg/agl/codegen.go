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

const GeneratedFilePrefix = "// agl:generated\n"

type Generator struct {
	fset             *token.FileSet
	env              *Env
	a, b             *ast.File
	prefix           string
	genFuncDecls2    map[string]string
	tupleStructs     map[string]string
	genFuncDecls     map[string]*ast.FuncDecl
	varCounter       atomic.Int64
	returnType       types.Type
	extensions       map[string]Extension
	extensionsString map[string]ExtensionString
	genMap           map[string]types.Type
	allowUnused      bool
	inlineStmt       bool
	fragments        Frags
}

func (g *Generator) WithGenMapping(m map[string]types.Type, clb func()) {
	prev := g.genMap
	g.genMap = m
	clb()
	g.genMap = prev
}

func (g *Generator) WithInlineStmt(clb func()) {
	prev := g.inlineStmt
	g.inlineStmt = true
	clb()
	g.inlineStmt = prev
}

type Extension struct {
	decl *ast.FuncDecl
	gen  map[string]ExtensionTest
}

type ExtensionString struct {
	decl *ast.FuncDecl
	gen  map[string]ExtensionTest
}

type ExtensionTest struct {
	raw      types.Type
	concrete types.Type
}

type GeneratorConf struct {
	AllowUnused bool
}

type GeneratorOption func(*GeneratorConf)

func AllowUnused() GeneratorOption {
	return func(c *GeneratorConf) {
		c.AllowUnused = true
	}
}

func NewGenerator(env *Env, a, b *ast.File, fset *token.FileSet, opts ...GeneratorOption) *Generator {
	conf := &GeneratorConf{}
	for _, opt := range opts {
		opt(conf)
	}
	genFns := make(map[string]*ast.FuncDecl)
	return &Generator{
		fset:             fset,
		env:              env,
		a:                a,
		b:                b,
		extensions:       make(map[string]Extension),
		extensionsString: make(map[string]ExtensionString),
		tupleStructs:     make(map[string]string),
		genFuncDecls2:    make(map[string]string),
		genFuncDecls:     genFns,
		allowUnused:      conf.AllowUnused}
}

type Frag struct {
	n ast.Node
	s string
}

type Frags []Frag

type EmitConf struct {
	n ast.Node
}

type EmitOption func(c *EmitConf)

func WithNode(n ast.Node) EmitOption {
	return func(c *EmitConf) {
		c.n = n
	}
}

func (g *Generator) Emit(s string, opts ...EmitOption) string {
	c := &EmitConf{}
	for _, opt := range opts {
		opt(c)
	}
	//printCallers(1)
	g.fragments = append(g.fragments, Frag{s: s, n: c.n})
	return s
}

func (g *Generator) genExtensionString(e ExtensionString) (out string) {
	for _, _ = range slices.Sorted(maps.Keys(e.gen)) {
		decl := e.decl
		var name, typeParamsStr, paramsStr, resultStr, bodyStr string
		if decl.Name != nil {
			name = decl.Name.Name
		}
		var paramsClone []ast.Field
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				paramsClone = append(paramsClone, *param)
			}
		}
		recv := decl.Recv.List[0]
		var recvName string
		if len(recv.Names) >= 1 {
			recvName = recv.Names[0].Name
		}
		recvT := recv.Type.(*ast.SelectorExpr).Sel.Name
		var recvTName string
		recvTName = recvT
		firstArg := ast.Field{Names: []*ast.LabelledIdent{{Ident: &ast.Ident{Name: recvName}, Label: nil}}, Type: &ast.Ident{Name: recvTName}}
		paramsClone = append([]ast.Field{firstArg}, paramsClone...)
		if params := paramsClone; params != nil {
			var fieldsItems []string
			for i, field := range params {
				var content string
				if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
					if _, ok := v.W.(types.FuncType); ok {
						content = types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStr1()
					} else {
						content = g.genExpr(field.Type).F()
					}
				} else {
					switch field.Type.(type) {
					case *ast.TupleExpr:
						content = g.env.GetType(field.Type).GoStr()
					default:
						content = g.genExpr(field.Type).F()
					}
				}
				if i == 0 {
					if content != "String" {
						panic("")
					}
					content = "string"
				}
				namesStr := utils.MapJoin(field.Names, func(n *ast.LabelledIdent) string { return n.Name }, ", ")
				namesStr = utils.SuffixIf(namesStr, " ")
				fieldsItems = append(fieldsItems, namesStr+content)
			}
			paramsStr = strings.Join(fieldsItems, ", ")
		}
		if result := decl.Type.Result; result != nil {
			resT := g.env.GetType(result)
			resultStr = utils.PrefixIf(resT.GoStr(), " ")
		}
		if decl.Body != nil {
			content := g.incrPrefix(func() string {
				return g.genStmt(decl.Body).F()
			})
			bodyStr = content
		}
		out += fmt.Sprintf("func AglString%s%s(%s)%s {\n%s}", name, typeParamsStr, paramsStr, resultStr, bodyStr)
		out += "\n"
	}
	return out
}

func (g *Generator) genExtension(e Extension) (out string) {
	for _, key := range slices.Sorted(maps.Keys(e.gen)) {
		ge := e.gen[key]
		m := types.FindGen(ge.raw, ge.concrete)
		decl := e.decl
		if decl == nil {
			return ""
		}
		typeParamsStr := func() string { return "" }
		paramsStr := func() string { return "" }
		resultStr := func() string { return "" }
		bodyStr := func() string { return "" }
		var name string
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

		firstArg := ast.Field{Names: []*ast.LabelledIdent{{Ident: &ast.Ident{Name: recvName}, Label: nil}}, Type: &ast.ArrayType{Elt: &ast.Ident{Name: recvTName}}}
		var paramsClone []ast.Field
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				paramsClone = append(paramsClone, *param)
			}
		}
		paramsClone = append([]ast.Field{firstArg}, paramsClone...)
		g.WithGenMapping(m, func() {
			if params := paramsClone; params != nil {
				paramsStr = func() string {
					var out string
					for i, field := range params {
						for j, n := range field.Names {
							out += g.Emit(n.Name)
							if j < len(field.Names)-1 {
								out += g.Emit(", ")
							}
						}
						if len(field.Names) > 0 {
							out += g.Emit(" ")
						}
						if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
							if _, ok := v.W.(types.FuncType); ok {
								out += g.Emit(types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStr1())
							} else {
								out += g.genExpr(field.Type).F()
							}
						} else {
							switch field.Type.(type) {
							case *ast.TupleExpr:
								out += g.Emit(g.env.GetType(field.Type).GoStr())
							default:
								out += g.genExpr(field.Type).F()
							}
						}
						if i < len(params)-1 {
							out += ", "
						}
					}
					return out
				}
			}
			if result := decl.Type.Result; result != nil {
				resultStr = func() string {
					resT := g.env.GetType(result)
					for k, v := range m {
						resT = types.ReplGen(resT, k, v)
					}
					if v := resT.GoStr(); v != "" {
						return g.Emit(" " + v)
					}
					return ""
				}
			}
			if decl.Body != nil {
				bodyStr = func() string {
					return g.incrPrefix(func() string {
						return g.genStmt(decl.Body).F()
					})
				}
			}
			out += g.Emit("func AglVec"+name+"_"+strings.Join(elts, "_")) + typeParamsStr() + g.Emit("(") + paramsStr() + g.Emit(")") + resultStr() + g.Emit(" {\n")
			out += bodyStr()
			out += g.Emit("}\n")
		})
	}
	return
}

func (g *Generator) Generate2() (out1, out2 string) {
	out1 = g.Generate()
	for _, f := range g.fragments {
		out2 += f.s
	}
	return
}

func (g *Generator) Generate() (out string) {
	out += g.Emit(GeneratedFilePrefix)
	out += g.genPackage()
	out += g.genImports(g.a)
	out += g.genImports(g.b)
	out4 := g.genDecls(g.b)
	out5 := g.genDecls(g.a)
	var extStringStr string
	for _, extKey := range slices.Sorted(maps.Keys(g.extensionsString)) {
		extStringStr += g.genExtensionString(g.extensionsString[extKey])
	}
	var extStr string
	for _, extKey := range slices.Sorted(maps.Keys(g.extensions)) {
		extStr += g.genExtension(g.extensions[extKey])
	}
	var tupleStr string
	for _, k := range slices.Sorted(maps.Keys(g.tupleStructs)) {
		tupleStr += g.tupleStructs[k]
	}
	var genFuncDeclStr string
	for _, k := range slices.Sorted(maps.Keys(g.genFuncDecls2)) {
		genFuncDeclStr += g.genFuncDecls2[k]
	}
	return out + g.Emit(tupleStr) + genFuncDeclStr + out4.F() + out5.F() + extStr + extStringStr
}

func (g *Generator) PkgName() string {
	return g.a.Name.Name
}

func (g *Generator) genPackage() string {
	return g.Emit("package " + g.a.Name.Name + "\n")
}

func (g *Generator) genImports(f *ast.File) (out string) {
	genRow := func(spec *ast.ImportSpec) (out string) {
		if spec.Name != nil {
			out += spec.Name.Name + " "
		}
		pathValue := spec.Path.Value
		if strings.HasPrefix(pathValue, `"agl1/`) {
			pathValue = `"` + pathValue[6:]
		}
		return out + pathValue + "\n"
	}
	if len(f.Imports) == 1 {
		spec := f.Imports[0]
		out += g.Emit("import " + genRow(spec))
	} else if len(f.Imports) > 1 {
		out += g.Emit("import (\n")
		for _, spec := range f.Imports {
			out += g.Emit("\t" + genRow(spec))
		}
		out += g.Emit(")\n")
	}
	return
}

func (g *Generator) genStmt(s ast.Stmt) (out SomethingTest) {
	//p("genStmt", to(s))
	switch stmt := s.(type) {
	case *ast.BlockStmt:
		return g.genBlockStmt(stmt)
	case *ast.IfStmt:
		return g.genIfStmt(stmt)
	case *ast.IfLetStmt:
		return g.genIfLetStmt(stmt)
	case *ast.GuardStmt:
		return g.genGuardStmt(stmt)
	case *ast.GuardLetStmt:
		return g.genGuardLetStmt(stmt)
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

func (g *Generator) genExpr(e ast.Expr) (out SomethingTest) {
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
	case *ast.LabelledArg:
		return g.genLabelledArg(expr)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func (g *Generator) genIdent(expr *ast.Ident) (out SomethingTest) {
	return SomethingTest{F: func() string {
		if strings.HasPrefix(expr.Name, "$") {
			beforeT := g.env.GetType(expr)
			expr.Name = strings.Replace(expr.Name, "$", "aglArg", 1)
			g.env.SetType(nil, nil, expr, beforeT, g.fset)
		}
		if strings.HasPrefix(expr.Name, "@") {
			expr.Name = strings.Replace(expr.Name, "@LINE", fmt.Sprintf(`"%d"`, g.fset.Position(expr.Pos()).Line), 1)
			expr.Name = strings.Replace(expr.Name, "@COLUMN", fmt.Sprintf(`"%d"`, g.fset.Position(expr.Pos()).Column), 1)
		}
		t := g.env.GetType(expr)
		if v, ok := t.(types.TypeType); ok {
			t = v.W
			switch typ := t.(type) {
			case types.GenericType:
				if typ.IsType {
					for k, v := range g.genMap {
						if typ.Name == k {
							typ.Name = v.GoStr()
							typ.W = v
						}
					}
					return g.Emit(typ.GoStr())
				}
			}
		}
		switch expr.Name {
		case "make":
			return g.Emit("make")
		case "abs":
			return g.Emit("AglAbs")
		}
		if v := g.env.Get(expr.Name); v != nil {
			switch vv := v.(type) {
			case types.TypeType:
				return g.Emit(v.GoStr())
			case types.FuncType:
				name := expr.Name
				if vv.Pub {
					name = "AglPub_" + name
				}
				return g.Emit(name)
			default:
				return g.Emit(expr.Name)
			}
		}
		return g.Emit(expr.Name)
	}}
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
	if len(g.prefix) > 0 {
		g.prefix = g.prefix[:len(g.prefix)-1]
	}
	out := clb()
	g.prefix = before
	return out
}

func (g *Generator) genShortFuncLit(expr *ast.ShortFuncLit) SomethingTest {
	c1 := g.genStmt(expr.Body)
	return SomethingTest{F: func() string {
		var out string
		t := g.env.GetType(expr).(types.FuncType)
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
		out = g.Emit(fmt.Sprintf("func(%s)%s {\n", argsStr, returnStr))
		out += g.incrPrefix(func() string {
			return c1.F()
		})
		out += g.Emit(g.prefix + "}")
		return out
	}}
}

func (g *Generator) genEnumType(enumName string, expr *ast.EnumType) string {
	out := fmt.Sprintf("type %sTag int\n", enumName)
	out += fmt.Sprintf("const (\n")
	for i, v := range expr.Values.List {
		if i == 0 {
			out += fmt.Sprintf("\t%s_%s %sTag = iota\n", enumName, v.Name.Name, enumName)
		} else {
			out += fmt.Sprintf("\t%s_%s\n", enumName, v.Name.Name)
		}
	}
	out += fmt.Sprintf(")\n")
	out += fmt.Sprintf("type %s struct {\n", enumName)
	out += fmt.Sprintf("\tTag %sTag\n", enumName)
	for _, field := range expr.Values.List {
		if field.Params != nil {
			for i, el := range field.Params.List {
				out += fmt.Sprintf("\t%s_%d %s\n", field.Name.Name, i, g.env.GetType2(el.Type, g.fset).GoStr())
			}
		}
	}
	out += "}\n"
	out += fmt.Sprintf("func (v %s) String() string {\n\tswitch v.Tag {\n", enumName)
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
	out += fmt.Sprintf("func (v %s) RawValue() int {\n\treturn int(v.Tag)\n}\n", enumName)
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
		out += fmt.Sprintf("func Make_%s_%s(%s) %s {\n\treturn %s{Tag: %s_%s%s}\n}\n",
			enumName, field.Name.Name, strings.Join(tmp, ", "), enumName, enumName, enumName, field.Name.Name, tmp1Out)
	}
	return out
}

func (g *Generator) genTypeAssertExpr(expr *ast.TypeAssertExpr) SomethingTest {
	if expr.Type != nil {
		c1 := g.genExpr(expr.X)
		c2 := g.genExpr(expr.Type)
		return SomethingTest{F: func() string { return c1.F() + g.Emit(".(") + c2.F() + g.Emit(")") }}
	} else {
		c1 := g.genExpr(expr.X)
		return SomethingTest{F: func() string { return c1.F() + g.Emit(".(type)") }}
	}
}

func (g *Generator) genStarExpr(expr *ast.StarExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("*") + c1.F() }}
}

func (g *Generator) genMapType(expr *ast.MapType) SomethingTest {
	t := g.env.GetType2(expr.Value, g.fset).GoStr()
	c1 := g.genExpr(expr.Key)
	return SomethingTest{F: func() string { return g.Emit("map[") + c1.F() + g.Emit("]"+t) }}
}

func (g *Generator) genSomeExpr(expr *ast.SomeExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("MakeOptionSome(") + c1.F() + g.Emit(")") }}
}

func (g *Generator) genOkExpr(expr *ast.OkExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("MakeResultOk(") + c1.F() + g.Emit(")") }}
}

func (g *Generator) genErrExpr(expr *ast.ErrExpr) SomethingTest {
	t := g.env.GetType(expr).(types.ErrType).T.GoStrType()
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("MakeResultErr["+t+"](") + c1.F() + g.Emit(")") }}
}

func (g *Generator) genChanType(expr *ast.ChanType) SomethingTest {
	c1 := g.genExpr(expr.Value)
	return SomethingTest{F: func() string { return g.Emit("chan ") + c1.F() }}
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

func (g *Generator) genOrBreakExpr(expr *ast.OrBreakExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	gPrefix := g.prefix
	return SomethingTest{F: func() string {
		return g.Emit("AglIdentity(" + varName + ").Unwrap()")
	}, B: []func() string{func() string {
		check := getCheck(g.env.GetType(expr.X))
		out := g.Emit(gPrefix+varName+" := ") + c1.F() + g.Emit("\n")
		out += g.Emit(gPrefix + "if " + varName + "." + check + " {\n")
		out += g.Emit(gPrefix + "\tbreak")
		if expr.Label != nil {
			out += g.Emit(" " + expr.Label.String())
		}
		out += g.Emit("\n")
		out += g.Emit(gPrefix + "}\n")
		return out
	}}}
}

func (g *Generator) genOrContinueExpr(expr *ast.OrContinueExpr) (out SomethingTest) {
	content1 := g.genExpr(expr.X)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	gPrefix := g.prefix
	return SomethingTest{F: func() string {
		return g.Emit("AglIdentity(" + varName + ").Unwrap()")
	}, B: []func() string{func() string {
		before := ""
		before += g.Emit(gPrefix+varName+" := ") + content1.F() + g.Emit("\n")
		before += g.Emit(gPrefix + fmt.Sprintf("if %s.%s {\n", varName, check))
		before += g.Emit(gPrefix + "\tcontinue")
		if expr.Label != nil {
			before += g.Emit(" " + expr.Label.String())
		}
		before += g.Emit("\n")
		before += g.Emit(gPrefix + "}\n")
		return before
	}}}
}

func (g *Generator) genOrReturn(expr *ast.OrReturnExpr) (out SomethingTest) {
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string {
		return g.Emit("AglIdentity(" + varName + ")")
	}, B: []func() string{func() string {
		out := ""
		out += g.Emit(g.prefix+varName+" := ") + c1.F() + g.Emit("\n")
		out += g.Emit(g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check))
		if g.returnType == nil {
			out += g.Emit(g.prefix + "\treturn\n")
		} else {
			switch retT := g.returnType.(type) {
			case types.ResultType:
				out += g.Emit(g.prefix + fmt.Sprintf("\treturn MakeResultErr[%s](%s.Err())\n", retT.W.GoStrType(), varName))
			case types.OptionType:
				out += g.Emit(g.prefix + fmt.Sprintf("\treturn MakeOptionNone[%s]()\n", retT.W.GoStrType()))
			case types.VoidType:
				out += g.Emit(g.prefix + fmt.Sprintf("\treturn\n"))
			default:
				assert(false, "cannot use or_return in a function that does not return void/Option/Result")
			}
		}
		out += g.Emit(g.prefix + "}\n")
		return out
	}}}
}

func (g *Generator) genUnaryExpr(expr *ast.UnaryExpr) SomethingTest {
	return SomethingTest{F: func() string {
		return g.Emit(expr.Op.String()) + g.genExpr(expr.X).F()
	}}
}

func (g *Generator) genSendStmt(expr *ast.SendStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		if !g.inlineStmt {
			out += g.Emit(g.prefix)
		}
		out += g.genExpr(expr.Chan).F() + g.Emit(" <- ") + g.genExpr(expr.Value).F()
		if !g.inlineStmt {
			out += g.Emit("\n")
		}
		return out
	}}
}

func (g *Generator) genSelectStmt(expr *ast.SelectStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix + "select {\n")
		out += g.genStmt(expr.Body).F()
		out += g.Emit(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genLabeledStmt(expr *ast.LabeledStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix + expr.Label.Name + ":\n")
		out += g.genStmt(expr.Stmt).F()
		return out
	}}
}

func (g *Generator) genBranchStmt(expr *ast.BranchStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix + expr.Tok.String())
		if expr.Label != nil {
			out += g.Emit(" ") + g.genExpr(expr.Label).F()
		}
		out += g.Emit("\n")
		return out
	}}
}

func (g *Generator) genDeferStmt(expr *ast.DeferStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix+"defer ") + g.genExpr(expr.Call).F() + g.Emit("\n")
		return out
	}}
}

func (g *Generator) genGoStmt(expr *ast.GoStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix+"go ") + g.genExpr(expr.Call).F() + g.Emit("\n")
		return out
	}}
}

func (g *Generator) genEmptyStmt(expr *ast.EmptyStmt) (out SomethingTest) {
	return SomethingTest{F: func() string {
		return ""
	}}
}

func (g *Generator) genMatchClause(expr *ast.MatchClause) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		switch v := expr.Expr.(type) {
		case *ast.ErrExpr:
			out += g.Emit(g.prefix+"case Err(") + g.genExpr(v.X).F() + g.Emit("):\n")
		case *ast.OkExpr:
			out += g.Emit(g.prefix+"case Ok(") + g.genExpr(v.X).F() + g.Emit("):\n")
		case *ast.SomeExpr:
			out += g.Emit(g.prefix+"case Some(") + g.genExpr(v.X).F() + g.Emit("):\n")
		default:
			panic("")
		}
		out += g.incrPrefix(func() string {
			return g.genStmts(expr.Body).F()
		})
		return out
	}}
}

func (g *Generator) genMatchExpr(expr *ast.MatchExpr) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		content1 := g.genExpr(expr.Init).F()
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
				for i, c := range expr.Body.List {
					c := c.(*ast.MatchClause)
					if v.Native {
						switch v := c.Expr.(type) {
						case *ast.OkExpr:
							binding := g.genExpr(v.X).F()
							assignOp := utils.Ternary(binding == "_", "=", ":=")
							out += gPrefix + fmt.Sprintf("if %s == nil {\n%s\t%s %s *%s\n", errName, gPrefix, binding, assignOp, varName)
						case *ast.ErrExpr:
							binding := g.genExpr(v.X).F()
							assignOp := utils.Ternary(binding == "_", "=", ":=")
							out += gPrefix + fmt.Sprintf("if %s != nil {\n%s\t%s %s %s\n", errName, gPrefix, binding, assignOp, errName)
						default:
							panic("")
						}
					} else {
						switch v := c.Expr.(type) {
						case *ast.OkExpr:
							out += gPrefix + fmt.Sprintf("if %s.IsOk() {\n%s\t%s := %s.Unwrap()\n", varName, gPrefix, g.genExpr(v.X).F(), varName)
						case *ast.ErrExpr:
							out += gPrefix + fmt.Sprintf("if %s.IsErr() {\n%s\t%s := %s.Err()\n", varName, gPrefix, g.genExpr(v.X).F(), varName)
						default:
							panic("")
						}
					}
					content3 := g.incrPrefix(func() string {
						return g.genStmts(c.Body).F()
					})
					out += content3
					out += gPrefix + "}"
					if i < len(expr.Body.List)-1 {
						out += "\n"
					}
				}
			}
		case types.OptionType:
			out += fmt.Sprintf("%s := %s\n", varName, content1)
			if expr.Body != nil {
				for i, c := range expr.Body.List {
					c := c.(*ast.MatchClause)
					switch v := c.Expr.(type) {
					case *ast.SomeExpr:
						out += gPrefix + fmt.Sprintf("if %s.IsSome() {\n%s\t%s := %s.Unwrap()\n", varName, gPrefix, g.genExpr(v.X).F(), varName)
					case *ast.NoneExpr:
						out += gPrefix + fmt.Sprintf("if %s.IsNone() {\n", varName)
					default:
						panic("")
					}
					content3 := g.incrPrefix(func() string {
						return g.genStmts(c.Body).F()
					})
					out += content3
					out += gPrefix + "}"
					if i < len(expr.Body.List)-1 {
						out += "\n"
					}
				}
			}
		case types.EnumType:
			if expr.Body != nil {
				for i, cc := range expr.Body.List {
					c := cc.(*ast.MatchClause)
					if i > 0 {
						out += gPrefix
						out += "} else "
					}
					switch cv := c.Expr.(type) {
					case *ast.CallExpr:
						sel := cv.Fun.(*ast.SelectorExpr)
						out += fmt.Sprintf("if %s.Tag == %s_%s {\n", expr.Init, v.Name, sel.Sel.Name)
						for j, id := range cv.Args {
							rhs := fmt.Sprintf("%s.%s_%d", expr.Init, v.Fields[i].Name, j)
							if id.(*ast.Ident).Name == "_" {
								out += gPrefix + fmt.Sprintf("\t_ = %s\n", rhs)
							} else {
								out += gPrefix + fmt.Sprintf("\t%s := %s\n", g.genExpr(id).F(), rhs)
							}
						}
						out += gPrefix + g.genStmts(c.Body).F()
					case *ast.SelectorExpr:
						out += fmt.Sprintf("if %s.Tag == %s_%s {\n", expr.Init, v.Name, cv.Sel.Name)
						out += gPrefix + g.genStmts(c.Body).F()
					default:
						panic(fmt.Sprintf("%v", to(c.Expr)))
					}
				}
				out += gPrefix + fmt.Sprintf("} else {\n")
				out += gPrefix + fmt.Sprintf("\tpanic(\"match on enum should be exhaustive\")\n")
				out += gPrefix + fmt.Sprintf("}")
			}
		default:
			panic(fmt.Sprintf("%v", to(initT)))
		}
		return out
	}}
}

func (g *Generator) genTypeSwitchStmt(expr *ast.TypeSwitchStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		e := func(s string) string { return g.Emit(s, WithNode(expr)) }
		out += e(g.prefix + "switch ")
		if expr.Init != nil {
			out += g.genStmt(expr.Init).F()
		}
		var n string
		g.WithInlineStmt(func() {
			if v, ok := expr.Assign.(*ast.AssignStmt); ok && len(v.Lhs) == 1 {
				if vv, ok := v.Lhs[0].(*ast.Ident); ok {
					n = vv.Name
				}
			}
			out += g.genStmt(expr.Assign).F()
		})
		out += e(" {\n")

		for _, ccr := range expr.Body.List {
			cc := ccr.(*ast.CaseClause)
			out += e(g.prefix)
			if cc.List != nil {
				out += e("case ")
				for i, el := range cc.List {
					out += g.genExpr(el).F()
					if i < len(cc.List)-1 {
						out += e(", ")
					}
				}
				out += e(":\n")
			} else {
				out += e("default:\n")
			}
			if g.allowUnused && n != "" {
				out += e(g.prefix + fmt.Sprintf("\tAglNoop(%s)\n", n))
			}
			if cc.Body != nil {
				out += g.incrPrefix(func() string {
					return g.genStmts(cc.Body).F()
				})
			}
		}

		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genCaseClause(expr *ast.CaseClause) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		var listStr string
		if expr.List != nil {
			tmp := utils.MapJoin(expr.List, func(el ast.Expr) string { return g.genExpr(el).F() }, ", ")
			listStr = "case " + tmp + ":\n"
		} else {
			listStr = "default:\n"
		}
		var content1 string
		if expr.Body != nil {
			content1 = g.incrPrefix(func() string {
				return g.genStmts(expr.Body).F()
			})
		}
		out += g.prefix + listStr
		out += content1
		return out
	}}
}

func (g *Generator) genSwitchStmt(expr *ast.SwitchStmt) SomethingTest {
	e := func(s string) string { return g.Emit(s, WithNode(expr)) }
	return SomethingTest{F: func() string {
		var out string
		content1 := func() string {
			var out string
			if expr.Init != nil {
				if init := g.genStmt(expr.Init).F(); init != "" {
					out = init + e(" ")
				}
			}
			return out
		}
		var tagIsEnum bool
		content2 := func() string {
			var out string
			if expr.Tag != nil {
				tagT := g.env.GetType(expr.Tag)
				switch tagT.(type) {
				case types.EnumType:
					tagIsEnum = true
					out += g.genExpr(expr.Tag).F() + e(".Tag"+" ")
				default:
					if v := g.genExpr(expr.Tag).F(); v != "" {
						out += v + e(" ")
					}
				}
			}
			return out
		}
		out += e(g.prefix+"switch ") + content1() + content2() + e("{\n")
		for _, cc := range expr.Body.List {
			expr1 := cc.(*ast.CaseClause)
			listStr := func() string {
				var out string
				if expr1.List != nil {
					tmp := func() string {
						var out string
						if tagIsEnum {
							tagT := g.env.GetType(expr.Tag).(types.EnumType)
							out += utils.MapJoin(expr1.List, func(el ast.Expr) string {
								if sel, ok := el.(*ast.SelectorExpr); ok {
									return fmt.Sprintf("%s_%s", tagT.Name, sel.Sel.Name) // TODO: validate Sel.Name is an actual field
								} else {
									return g.genExpr(el).F()
								}
							}, ", ")
						} else {
							for i, el := range expr1.List {
								out += g.genExpr(el).F()
								if i < len(expr1.List)-1 {
									out += e(", ")
								}
							}
						}
						return out
					}
					out = e("case ") + tmp() + e(":\n")
				} else {
					out = e("default:\n")
				}
				return out
			}
			content3 := func() string {
				var out string
				if expr1.Body != nil {
					out = g.incrPrefix(func() string {
						return g.genStmts(expr1.Body).F()
					})
				}
				return out
			}
			out += e(g.prefix) + listStr()
			out += content3()
		}
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genCommClause(expr *ast.CommClause) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit(g.prefix)
		if expr.Comm != nil {
			g.WithInlineStmt(func() {
				out += g.Emit("case ") + g.genStmt(expr.Comm).F() + g.Emit(":")
			})
		} else {
			out += g.Emit("default:")
		}
		out += g.Emit("\n")
		if expr.Body != nil {
			out += g.genStmts(expr.Body).F()
		}
		return out
	}}
}

func (g *Generator) genNoneExpr(expr *ast.NoneExpr) SomethingTest {
	return SomethingTest{F: func() string {
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
		return g.Emit("MakeOptionNone[" + typeStr + "]()")
	}}
}

func (g *Generator) genInterfaceType(expr *ast.InterfaceType) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		if expr.Methods == nil || len(expr.Methods.List) == 0 {
			return g.Emit("interface{}")
		}
		out += g.Emit("interface {\n")
		if expr.Methods != nil {
			for _, m := range expr.Methods.List {
				content1 := g.env.GetType(m.Type).(types.FuncType).GoStr1()
				out += g.Emit(g.prefix + "\t" + m.Names[0].Name + strings.TrimPrefix(content1, "func") + "\n")
			}
		}
		out += g.Emit("}")
		return out
	}}
}

func (g *Generator) genEllipsis(expr *ast.Ellipsis) SomethingTest {
	return SomethingTest{F: func() string {
		content1 := g.incrPrefix(func() string {
			return g.genExpr(expr.Elt).F()
		})
		return "..." + content1
	}}
}

func (g *Generator) genParenExpr(expr *ast.ParenExpr) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit("(")
		out += g.incrPrefix(func() string {
			return g.genExpr(expr.X).F()
		})
		out += g.Emit(")")
		return out
	}}
}

func (g *Generator) genFuncLit(expr *ast.FuncLit) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.genFuncType(expr.Type).F() + g.Emit(" {\n")
		out += g.incrPrefix(func() string {
			return g.genStmt(expr.Body).F()
		})
		out += g.Emit(g.prefix + "}")
		return out
	}}
}

func (g *Generator) genStructType(expr *ast.StructType) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		e := func(s string) string { return g.Emit(s, WithNode(expr)) }
		gPrefix := g.prefix
		if expr.Fields == nil || len(expr.Fields.List) == 0 {
			return e("struct{}")
		}
		out += e("struct {\n")
		for _, field := range expr.Fields.List {
			out += e(gPrefix + "\t")
			for i, name := range field.Names {
				out += e(name.Name)
				if i < len(field.Names)-1 {
					out += e(", ")
				}
			}
			out += e(" ")
			out += g.genExpr(field.Type).F()
			if field.Tag != nil {
				out += utils.PrefixIf(g.genExpr(field.Tag).F(), " ")
			}
			out += e("\n")
		}
		out += e(gPrefix + "}")
		return out
	}}
}

func (g *Generator) genFuncType(expr *ast.FuncType) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out = g.Emit("func")
		var typeParamsStr string
		if typeParams := expr.TypeParams; typeParams != nil {
			typeParamsStr = g.joinList(expr.TypeParams)
			typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
		}
		out += typeParamsStr
		out += g.Emit("(")
		var paramsStr string
		if params := expr.Params; params != nil {
			for i, field := range params.List {
				for j, n := range field.Names {
					paramsStr += g.Emit(n.Name)
					if j < len(field.Names)-1 {
						paramsStr += g.Emit(", ")
					}
				}
				if len(field.Names) > 0 {
					paramsStr += g.Emit(" ")
				}
				if _, ok := field.Type.(*ast.TupleExpr); ok {
					paramsStr += g.Emit(g.env.GetType(field.Type).GoStr())
				} else {
					paramsStr += g.genExpr(field.Type).F()
				}
				if i < len(params.List)-1 {
					paramsStr += g.Emit(", ")
				}
			}
		}
		out += paramsStr
		out += g.Emit(")")
		content1 := g.incrPrefix(func() string {
			if expr.Result != nil {
				return g.Emit(" ") + g.genExpr(expr.Result).F()
			} else {
				return ""
			}
		})
		out += content1
		return out
	}}
}

func (g *Generator) genIndexExpr(expr *ast.IndexExpr) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.genExpr(expr.X).F()
		out += g.Emit("[")
		out += g.genExpr(expr.Index).F()
		out += g.Emit("]")
		return out
	}}
}

func (g *Generator) genLabelledArg(expr *ast.LabelledArg) SomethingTest {
	return g.genExpr(expr.X)
}

func (g *Generator) genDumpExpr(expr *ast.DumpExpr) SomethingTest {
	content1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string {
		return content1.F()
	}, B: []func() string{func() string {
		varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
		safeContent1 := strconv.Quote(content1.F())
		before := g.prefix + fmt.Sprintf("%s := %s\n", varName, content1.F())
		before += g.prefix + fmt.Sprintf("fmt.Printf(\"%s: %%s: %%v\\n\", %s, %s)\n", g.fset.Position(expr.X.Pos()), safeContent1, varName)
		return before
	}}}
}

func (g *Generator) genSliceExpr(expr *ast.SliceExpr) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.genExpr(expr.X).F()
		out += g.Emit("[")
		if expr.Low != nil {
			out += g.genExpr(expr.Low).F()
		}
		out += g.Emit(":")
		if expr.High != nil {
			out += g.genExpr(expr.High).F()
		}
		if expr.Max != nil {
			out += g.Emit(":")
			out += g.genExpr(expr.Max).F()
		}
		out += g.Emit("]")
		return out
	}}
}

func (g *Generator) genIndexListType(expr *ast.IndexListExpr) SomethingTest {
	return SomethingTest{F: func() string {
		return g.genExpr(expr.X).F() + g.Emit("[") + g.genExprs(expr.Indices).F() + g.Emit("]")
	}}
}

func (g *Generator) genSelectorExpr(expr *ast.SelectorExpr) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		content1 := func() string { return g.genExpr(expr.X).F() }
		name := expr.Sel.Name
		t := types.Unwrap(g.env.GetType(expr.X))
		switch t.(type) {
		case types.TupleType:
			name = "Arg" + name
		case types.EnumType:
			content2 := func() string { return g.genExpr(expr.Sel).F() }
			var out string
			if expr.Sel.Name == "RawValue" {
				out = content1() + g.Emit(".") + content2()
			} else {
				out = g.Emit("Make_") + content1() + g.Emit("_") + content2()
				if _, ok := g.env.GetType(expr).(types.EnumType); ok { // TODO
					out += g.Emit("()")
				}
			}
			return out
		}
		out = content1() + g.Emit("."+name)
		return out
	}}
}

func (g *Generator) genBubbleOptionExpr(expr *ast.BubbleOptionExpr) SomethingTest {
	var out string
	switch exprXT := g.env.GetInfo(expr.X).Type.(type) {
	case types.OptionType:
		if exprXT.Bubble {
			content1 := g.genExpr(expr.X)
			if exprXT.Native {
				varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
				return SomethingTest{F: func() string { return g.Emit("AglIdentity(" + varName + ")") }, B: []func() string{func() string {
					out := g.Emit(g.prefix+varName+", ok := ") + content1.F() + g.Emit("\n")
					out += g.Emit(g.prefix + "if !ok {\n")
					out += g.Emit(g.prefix + "\treturn MakeOptionNone[" + exprXT.W.GoStr() + "]()\n")
					out += g.Emit(g.prefix + "}\n")
					return out
				}}}
			} else {
				varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
				returnType := g.returnType
				return SomethingTest{F: func() string { return g.Emit(varName + ".Unwrap()") }, B: []func() string{func() string {
					out := g.Emit(g.prefix+varName+" := ") + content1.F() + g.Emit("\n")
					out += g.Emit(g.prefix + "if " + varName + ".IsNone() {\n")
					out += g.Emit(g.prefix + "\treturn MakeOptionNone[" + returnType.(types.OptionType).W.String() + "]()\n")
					out += g.Emit(g.prefix + "}\n")
					return out
				}}}
			}
		} else {
			if exprXT.Native {
				content1 := g.genExpr(expr.X)
				id := g.varCounter.Add(1)
				varName := fmt.Sprintf("aglTmpVar%d", id)
				errName := fmt.Sprintf("aglTmpErr%d", id)
				out = fmt.Sprintf(`AglIdentity(%s)`, varName)
				return SomethingTest{F: func() string { return g.Emit(out) }, B: []func() string{func() string {
					out := g.Emit(g.prefix+varName+", "+errName+" := ") + content1.F() + g.Emit("\n")
					out += g.Emit(g.prefix + "if " + errName + " != nil {\n")
					out += g.Emit(g.prefix + "\tpanic(" + errName + ")\n")
					out += g.Emit(g.prefix + "}\n")
					return out
				}}}
			} else {
				return SomethingTest{F: func() string { return g.genExpr(expr.X).F() + g.Emit(".Unwrap()") }}
			}
		}
	case types.TypeAssertType:
		content1 := g.genExpr(expr.X)
		id := g.varCounter.Add(1)
		varName := fmt.Sprintf("aglTmpVar%d", id)
		okName := fmt.Sprintf("aglTmpOk%d", id)
		returnType := g.returnType
		return SomethingTest{F: func() string { return g.Emit(varName) }, B: []func() string{func() string {
			tmp := g.prefix + fmt.Sprintf("%s, %s := %s\n", varName, okName, content1.F())
			tmp += g.prefix + fmt.Sprintf("if !%s {\n", okName)
			if v, ok := returnType.(types.OptionType); ok {
				tmp += g.prefix + fmt.Sprintf("\tMakeOptionNone[%s]()\n", v.W.GoStrType())
			} else {
				tmp += g.prefix + fmt.Sprintf("\tpanic(\"type assert failed\")\n")
			}
			tmp += g.prefix + fmt.Sprintf("}\n")
			return tmp
		}}}
	default:
		panic("")
	}
}

func (g *Generator) genBubbleResultExpr(expr *ast.BubbleResultExpr) (out SomethingTest) {
	exprXT := MustCast[types.ResultType](g.env.GetType(expr.X))
	if exprXT.Bubble {
		content1 := g.genExpr(expr.X)
		if _, ok := exprXT.W.(types.VoidType); ok && exprXT.Native {
			errName := fmt.Sprintf("aglTmpErr%d", g.varCounter.Add(1))
			return SomethingTest{F: func() string { return g.Emit(`AglNoop()`) }, B: []func() string{func() string {
				out := g.Emit(g.prefix+"if "+errName+" := ") + content1.F() + g.Emit("; "+errName+" != nil {\n")
				out += g.Emit(g.prefix + "\treturn MakeResultErr[" + exprXT.W.GoStrType() + "](" + errName + ")\n")
				out += g.Emit(g.prefix + "}\n")
				return out
			}}}
		} else if exprXT.Native {
			id := g.varCounter.Add(1)
			varName := fmt.Sprintf("aglTmpVar%d", id)
			errName := fmt.Sprintf("aglTmpErr%d", id)
			returnType := g.returnType
			return SomethingTest{F: func() string {
				return g.Emit("AglIdentity(" + varName + ")")
			}, B: []func() string{func() string {
				out := g.Emit(g.prefix+varName+", "+errName+" := ") + content1.F() + g.Emit("\n")
				out += g.Emit(g.prefix + "if " + errName + " != nil {\n")
				out += g.Emit(g.prefix + "\treturn MakeResultErr[" + returnType.(types.ResultType).W.GoStrType() + "](" + errName + ")\n")
				out += g.Emit(g.prefix + "}\n")
				return out
			}}}
		} else if exprXT.ConvertToNone {
			varName := fmt.Sprintf("aglTmpVar%d", g.varCounter.Add(1))
			return SomethingTest{F: func() string { return g.Emit(varName + ".Unwrap()") }, B: []func() string{func() string {
				out := g.Emit(g.prefix+varName+" := ") + content1.F() + g.Emit("\n")
				out += g.Emit(g.prefix + "if " + varName + ".IsErr() {\n")
				out += g.Emit(g.prefix + "\treturn MakeOptionNone[" + exprXT.ToNoneType.GoStrType() + "]()\n")
				out += g.Emit(g.prefix + "}\n")
				return out
			}}}
		} else {
			varName := fmt.Sprintf("aglTmpVar%d", g.varCounter.Add(1))
			return SomethingTest{F: func() string { return g.Emit(varName + ".Unwrap()") }, B: []func() string{func() string {
				out := g.Emit(g.prefix+varName+" := ") + content1.F() + g.Emit("\n")
				out += g.Emit(g.prefix + "if " + varName + ".IsErr() {\n")
				out += g.Emit(g.prefix + "\treturn " + varName + "\n")
				out += g.Emit(g.prefix + "}\n")
				return out
			}}}
		}
	} else {
		if exprXT.Native {
			if _, ok := exprXT.W.(types.VoidType); ok {
				c1 := g.genExpr(expr.X)
				tmpErrVar := fmt.Sprintf("aglTmpErr%d", g.varCounter.Add(1))
				return SomethingTest{F: func() string { return g.Emit(`AglNoop()`) }, B: []func() string{func() string {
					out := g.Emit(g.prefix+tmpErrVar+" := ") + c1.F() + g.Emit("\n")
					out += g.Emit(g.prefix + "if " + tmpErrVar + " != nil {\n")
					out += g.Emit(g.prefix + "\tpanic(" + tmpErrVar + ")\n")
					out += g.Emit(g.prefix + "}\n")
					return out
				}}}
			} else {
				c1 := g.genExpr(expr.X)
				id := g.varCounter.Add(1)
				varName := fmt.Sprintf("aglTmp%d", id)
				errName := fmt.Sprintf("aglTmpErr%d", id)
				return SomethingTest{F: func() string { return g.Emit("AglIdentity(" + varName + ")") }, B: []func() string{func() string {
					out := g.Emit(g.prefix+varName+", "+errName+" := ") + c1.F() + g.Emit("\n")
					out += g.Emit(g.prefix + "if " + errName + " != nil {\n")
					out += g.Emit(g.prefix + "\tpanic(" + errName + ")\n")
					out += g.Emit(g.prefix + "}\n")
					return out
				}}}
			}
		} else {
			return SomethingTest{F: func() string { return g.genExpr(expr.X).F() + g.Emit(".Unwrap()") }}
		}
	}
}

func (g *Generator) genCallExpr(expr *ast.CallExpr) SomethingTest {
	switch e := expr.Fun.(type) {
	case *ast.SelectorExpr:
		oeXT := g.env.GetType(e.X)
		eXT := types.Unwrap(oeXT)
		switch eXTT := eXT.(type) {
		case types.ArrayType:
			genEX := func() string { return g.genExpr(e.X).F() }
			genArgFn := func(i int) string { return g.genExpr(expr.Args[i]).F() }
			eltT := types.ReplGenM(eXTT.Elt, g.genMap)
			eltTStr := eltT.GoStr()
			fnName := e.Sel.Name
			switch fnName {
			case "Sum", "Last", "First", "Len", "IsEmpty", "Clone", "Indices", "Sorted", "Iter":
				return SomethingTest{F: func() string { return g.Emit("AglVec"+fnName+"(") + genEX() + g.Emit(")") }}
			case "Filter", "AllSatisfy", "Contains", "ContainsWhere", "Any", "Map", "Find", "Joined", "Get", "FirstIndex", "FirstIndexWhere", "FirstWhere", "__ADD":
				return SomethingTest{F: func() string { return g.Emit("AglVec"+fnName+"(") + genEX() + g.Emit(", ") + genArgFn(0) + g.Emit(")") }}
			case "Reduce", "ReduceInto":
				return SomethingTest{F: func() string {
					return g.Emit("AglVec"+fnName+"(") + genEX() + g.Emit(", ") + genArgFn(0) + g.Emit(", ") + genArgFn(1) + g.Emit(")")
				}}
			case "Insert":
				return SomethingTest{F: func() string {
					return g.Emit("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + g.Emit("), ") + genArgFn(0) + g.Emit(", ") + genArgFn(1) + g.Emit(")")
				}}
			case "PopIf", "PushFront", "Remove":
				return SomethingTest{F: func() string {
					return g.Emit("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + g.Emit("), ") + genArgFn(0) + g.Emit(")")
				}}
			case "Pop", "PopFront":
				return SomethingTest{F: func() string {
					return g.Emit("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + g.Emit(")") + g.Emit(")")
				}}
			case "Push":
				paramsStr := utils.MapJoin(expr.Args, func(arg ast.Expr) string { return g.genExpr(arg).F() }, ", ")
				ellipsis := utils.TernaryOrZero(expr.Ellipsis.IsValid(), "...")
				tmpoeXT := oeXT
				if v, ok := tmpoeXT.(types.MutType); ok {
					tmpoeXT = v.W
				}

				// Push into a value of a mut map
				if v, ok := e.X.(*ast.IndexExpr); ok {
					ot := g.env.GetType(v.X)
					t := types.Unwrap(ot)
					if vv, ok := t.(types.MapType); ok {
						if _, ok := vv.V.(types.StarType); !ok {
							varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
							return SomethingTest{F: func() string {
								var out string
								out += fmt.Sprintf("%s := %s\n", varName, genEX()) // temp variable to store the map value
								out += g.prefix + fmt.Sprintf("AglVec%s(&%s, %s%s)\n", fnName, varName, paramsStr, ellipsis)
								out += g.prefix + fmt.Sprintf("%s = %s", genEX(), varName) // put the temp value back in the map
								return out
							}}
						}
					}
				}

				if _, ok := tmpoeXT.(types.StarType); ok {
					return SomethingTest{F: func() string { return fmt.Sprintf("AglVec%s(%s, %s%s)", fnName, genEX(), paramsStr, ellipsis) }}
				} else {
					return SomethingTest{F: func() string {
						return fmt.Sprintf("AglVec%s((*[]%s)(&%s), %s%s)", fnName, eltTStr, genEX(), paramsStr, ellipsis)
					}}
				}
			default:
				extName := "agl1.Vec." + fnName
				rawFnT := g.env.Get(extName)
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
				c1 := g.genExprs(expr.Args)
				return SomethingTest{F: func() string {
					out := g.Emit("AglVec"+fnName+"_"+elsStr+"(") + genEX()
					if len(expr.Args) > 0 {
						out += ", "
					}
					out += c1.F()
					out += g.Emit(")")
					return out
				}}
			}
		case types.SetType:
			fnName := e.Sel.Name
			switch fnName {
			case "Union", "FormUnion", "Intersects", "Subtracting", "Subtract", "Intersection", "FormIntersection",
				"SymmetricDifference", "FormSymmetricDifference", "IsSubset", "IsStrictSubset", "IsSuperset", "IsStrictSuperset", "IsDisjoint":
				arg0 := expr.Args[0]
				content2 := func() string {
					switch v := g.env.GetType(arg0).(type) {
					case types.ArrayType:
						return g.Emit("AglVec["+v.Elt.GoStrType()+"](") + g.genExpr(arg0).F() + g.Emit(")")
					default:
						return g.genExpr(arg0).F()
					}
				}
				return SomethingTest{F: func() string {
					return g.Emit("AglSet"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + content2() + g.Emit(")")
				}}
			case "Insert", "Remove", "Contains", "Equals":
				return SomethingTest{F: func() string {
					return g.Emit("AglSet"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit(")")
				}}
			case "Len", "Min", "Max", "Iter":
				return SomethingTest{F: func() string { return g.Emit("AglSet"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(")") }}
			}
		case types.I64Type:
			fnName := e.Sel.Name
			switch fnName {
			case "String":
				return SomethingTest{F: func() string { return g.Emit("AglI64String(") + g.genExpr(e.X).F() + g.Emit(")") }}
			}
		case types.StringType, types.UntypedStringType:
			fnName := e.Sel.Name
			switch fnName {
			case "Replace":
				return SomethingTest{F: func() string {
					return g.Emit("AglString"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit(", ") + g.genExpr(expr.Args[1]).F() + g.Emit(", ") + g.genExpr(expr.Args[2]).F() + g.Emit(")")
				}}
			case "ReplaceAll":
				return SomethingTest{F: func() string {
					return g.Emit("AglString"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit(", ") + g.genExpr(expr.Args[1]).F() + g.Emit(")")
				}}
			case "Split", "TrimPrefix", "HasPrefix", "HasSuffix":
				return SomethingTest{F: func() string {
					return g.Emit("AglString"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit(")")
				}}
			case "TrimSpace", "Uppercased", "AsBytes", "Lines", "Int", "I8", "I16", "I32", "I64", "Uint", "U8", "U16", "U32", "U64", "F32", "F64":
				return SomethingTest{F: func() string { return g.Emit("AglString"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(")") }}
			default:
				extName := "agl1.String." + fnName
				rawFnT := g.env.Get(extName)
				tmp := g.extensionsString[extName]
				if tmp.gen == nil {
					tmp.gen = make(map[string]ExtensionTest)
				}
				tmp.gen[rawFnT.String()] = ExtensionTest{raw: rawFnT}
				g.extensionsString[extName] = tmp
				return SomethingTest{F: func() string { return g.Emit("AglString"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(")") }}
			}
		case types.MapType:
			fnName := e.Sel.Name
			switch fnName {
			case "Len":
				return SomethingTest{F: func() string { return g.Emit("AglIdentity(AglMapLen(") + g.genExpr(e.X).F() + "))" }}
			case "Get":
				return SomethingTest{F: func() string {
					return g.Emit("AglIdentity(AglMapIndex(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit("))")
				}}
			case "ContainsKey":
				return SomethingTest{F: func() string {
					return g.Emit("AglIdentity(AglMapContainsKey(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit("))")
				}}
			case "Keys", "Values":
				return SomethingTest{F: func() string { return g.Emit("AglIdentity(AglMap"+fnName+"(") + g.genExpr(e.X).F() + g.Emit("))") }}
			case "Filter":
				return SomethingTest{F: func() string {
					return g.Emit("AglIdentity(AglMapFilter(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit("))")
				}}
			case "Map":
				return SomethingTest{F: func() string {
					return g.Emit("AglIdentity(AglMapMap(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit("))")
				}}
			case "Reduce", "ReduceInto":
				return SomethingTest{F: func() string {
					return g.Emit("AglMap"+fnName+"(") + g.genExpr(e.X).F() + g.Emit(", ") + g.genExpr(expr.Args[0]).F() + g.Emit(", ") + g.genExpr(expr.Args[1]).F() + g.Emit(")")
				}}
			}
		default:
			if v, ok := e.X.(*ast.Ident); ok && v.Name == "agl" && e.Sel.Name == "NewSet" {
				return SomethingTest{F: func() string { return g.Emit("AglNewSet(") + g.genExprs(expr.Args).F() + g.Emit(")") }}
			} else if v, ok := e.X.(*ast.Ident); ok && v.Name == "http" && e.Sel.Name == "NewRequest" {
				return SomethingTest{F: func() string { return g.Emit("AglHttpNewRequest(") + g.genExprs(expr.Args).F() + g.Emit(")") }}
			}
		}
	case *ast.Ident:
		if e.Name == "assert" {
			return SomethingTest{F: func() string {
				var out string
				out = g.Emit("AglAssert(")
				var contents []func() string
				for _, arg := range expr.Args {
					contents = append(contents, func() string { return g.genExpr(arg).F() })
				}
				line := g.fset.Position(expr.Pos()).Line
				msg := fmt.Sprintf(`"assert failed line %d"`, line)
				if len(contents) == 1 {
					contents = append(contents, func() string { return g.Emit(msg) })
				} else {
					tmp := contents[1]
					contents[1] = func() string { return g.Emit(msg+` + " " + `) + tmp() }
				}
				for i, c := range contents {
					out += c()
					if i < len(contents)-1 {
						out += g.Emit(", ")
					}
				}
				out += g.Emit(")")
				return out
			}}
		} else if e.Name == "panic" {
			return SomethingTest{F: func() string { return g.Emit("panic(nil)") }}
		} else if e.Name == "panicWith" {
			return SomethingTest{F: func() string { return g.Emit("panic(") + g.genExpr(expr.Args[0]).F() + g.Emit(")") }}
		}
	}
	content1 := func() string { return "" }
	content2 := func() string { return "" }
	switch v := expr.Fun.(type) {
	case *ast.Ident:
		t1 := g.env.Get(v.Name)
		if t2, ok := t1.(types.TypeType); ok && TryCast[types.CustomType](t2.W) {
			c2 := g.genExprs(expr.Args)
			content1 = func() string { return v.Name }
			content2 = func() string { return c2.F() }
		} else {
			if fnT, ok := t1.(types.FuncType); ok {
				if !InArray(v.Name, []string{"make", "append", "len", "new", "AglAbs", "min", "max"}) && fnT.IsGeneric() {
					oFnT := g.env.Get(v.Name)
					newFnT := g.env.GetType(v)
					fnDecl := g.genFuncDecls[oFnT.String()]
					m := types.FindGen(oFnT, newFnT)
					var outFnDecl string
					g.WithGenMapping(m, func() {
						outFnDecl = g.decrPrefix(func() string {
							return g.genFuncDecl(fnDecl).F()
						})
					})
					out := g.genExpr(v).F()
					for _, k := range slices.Sorted(maps.Keys(m)) {
						out += fmt.Sprintf("_%s_%s", k, m[k].GoStr())
					}
					g.genFuncDecls2[out] = outFnDecl
					content1 = func() string {
						return out
					}
					content2 = func() string {
						var out string
						g.WithGenMapping(m, func() {
							out += g.genExprs(expr.Args).F()
						})
						return out
					}
				} else if v.Name == "make" {
					c1 := g.genExpr(v)
					content1 = func() string { return c1.F() }
					content2 = func() string {
						var out string
						if g.genMap != nil {
							out = g.Emit(types.ReplGenM(g.env.GetType(expr.Args[0]), g.genMap).GoStr())
						} else {
							out = g.genExpr(expr.Args[0]).F()
						}
						if len(expr.Args) > 1 {
							out += g.Emit(", ")
							for i, arg := range expr.Args[1:] {
								out += g.genExpr(arg).F()
								if i < len(expr.Args[1:])-1 {
									out += g.Emit(", ")
								}
							}
						}
						return out
					}
				} else {
					c1 := g.genExpr(expr.Fun)
					c2 := g.genExprs(expr.Args)
					content1 = func() string { return c1.F() }
					content2 = func() string { return c2.F() }
				}
			} else {
				c1 := g.genExpr(expr.Fun)
				c2 := g.genExprs(expr.Args)
				content1 = func() string { return c1.F() }
				content2 = func() string { return c2.F() }
			}
		}
	default:
		c1 := g.genExpr(expr.Fun)
		c2 := g.genExprs(expr.Args)
		content1 = func() string { return c1.F() }
		content2 = func() string { return c2.F() }
	}
	return SomethingTest{F: func() string {
		var out string
		out = content1() + g.Emit("(")
		out += content2()
		if expr.Ellipsis != 0 {
			out += g.Emit("...")
		}
		out += g.Emit(")")
		return out
	}}
}

func (g *Generator) genSetType(expr *ast.SetType) SomethingTest {
	return SomethingTest{F: func() string {
		return g.Emit("AglSet[") + g.genExpr(expr.Key).F() + g.Emit("]")
	}}
}

func (g *Generator) genArrayType(expr *ast.ArrayType) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit("[]")
		switch v := expr.Elt.(type) {
		case *ast.TupleExpr:
			out += g.Emit(types.ReplGenM(g.env.GetType(v), g.genMap).GoStr())
		default:
			out += g.genExpr(expr.Elt).F()
		}
		return out
	}}
}

func (g *Generator) genKeyValueExpr(expr *ast.KeyValueExpr) SomethingTest {
	var bs []func() string
	c1 := g.genExpr(expr.Key)
	c2 := g.genExpr(expr.Value)
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return SomethingTest{F: func() string {
		var out string
		out += c1.F()
		out += g.Emit(": ")
		out += c2.F()
		return out
	}, B: bs}
}

func (g *Generator) genResultExpr(expr *ast.ResultExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("Result[") + c1.F() + g.Emit("]") }}
}

func (g *Generator) genOptionExpr(expr *ast.OptionExpr) SomethingTest {
	c1 := g.genExpr(expr.X)
	return SomethingTest{F: func() string { return g.Emit("Option[") + c1.F() + g.Emit("]") }}
}

func (g *Generator) genBasicLit(expr *ast.BasicLit) SomethingTest {
	return SomethingTest{F: func() string { return g.Emit(expr.Value) }}
}

func (g *Generator) genBinaryExpr(expr *ast.BinaryExpr) SomethingTest {
	return SomethingTest{F: func() string {
		content1 := func() string { return g.genExpr(expr.X).F() }
		content2 := func() string { return g.genExpr(expr.Y).F() }
		op := expr.Op.String()
		xT := g.env.GetType(expr.X)
		yT := g.env.GetType(expr.Y)
		if xT != nil && yT != nil {
			if op == "in" {
				t := g.env.GetType(expr.Y)
				switch t.(type) {
				case types.ArrayType:
					t = t.(types.ArrayType).Elt
					content2 = func() string { return fmt.Sprintf("AglVec[%s](%s)", t.GoStrType(), g.genExpr(expr.Y).F()) }
				case types.MapType:
					kT := t.(types.MapType).K
					vT := t.(types.MapType).V
					content2 = func() string {
						return fmt.Sprintf("AglMap[%s, %s](%s)", kT.GoStrType(), vT.GoStrType(), g.genExpr(expr.Y).F())
					}
				case types.SetType:
				default:
					panic(fmt.Sprintf("%v", to(t)))
				}
				return g.Emit("AglIn(") + content1() + g.Emit(", ") + content2() + g.Emit(")")
			}
			if TryCast[types.SetType](xT) && TryCast[types.SetType](yT) {
				if (op == "==" || op == "!=") && g.env.Get("agl1.Set.Equals") != nil {
					var out string
					if op == "!=" {
						out += g.Emit("!")
					}
					out += g.Emit("AglSetEquals(") + content1() + g.Emit(", ") + content2() + g.Emit(")")
					return out
				}
			} else if TryCast[types.ArrayType](xT) && TryCast[types.ArrayType](yT) {
				lhsT := types.Unwrap(xT.(types.ArrayType).Elt)
				rhsT := types.Unwrap(yT.(types.ArrayType).Elt)
				if op == "+" && g.env.Get("agl1.Vec.__ADD") != nil {
					return g.Emit("AglVec__ADD(") + content1() + g.Emit(", ") + content2() + g.Emit(")")
				}
				if TryCast[types.ByteType](lhsT) && TryCast[types.ByteType](rhsT) {
					if op == "==" || op == "!=" {
						var out string
						if op == "!=" {
							out += g.Emit("!")
						}
						out += g.Emit("AglBytesEqual(") + content1() + g.Emit(", ") + content2() + g.Emit(")")
						return out
					}
				}
			} else if TryCast[types.StructType](xT) && TryCast[types.StructType](yT) {
				lhsName := xT.(types.StructType).Name
				rhsName := yT.(types.StructType).Name
				if lhsName == rhsName {
					if (op == "==" || op == "!=") && g.env.Get(lhsName+".__EQL") != nil {
						var out string
						if op == "!=" {
							out += g.Emit("!")
						}
						out += content1() + g.Emit(".__EQL(") + content2() + g.Emit(")")
						return out
					} else if op == "+" && g.env.Get(lhsName+".__ADD") != nil {
						return content1() + g.Emit(".__ADD(") + content2() + g.Emit(")")
					} else if op == "-" && g.env.Get(lhsName+".__SUB") != nil {
						return content1() + g.Emit(".__SUB(") + content2() + g.Emit(")")
					} else if op == "*" && g.env.Get(lhsName+".__MUL") != nil {
						return content1() + g.Emit(".__MUL(") + content2() + g.Emit(")")
					} else if op == "/" && g.env.Get(lhsName+".__QUO") != nil {
						return content1() + g.Emit(".__QUO(") + content2() + g.Emit(")")
					} else if op == "%" && g.env.Get(lhsName+".__REM") != nil {
						return content1() + g.Emit(".__REM(") + content2() + g.Emit(")")
					}
				}
			}
		}
		return content1() + g.Emit(" "+expr.Op.String()+" ") + content2()
	}}
}

func (g *Generator) genCompositeLit(expr *ast.CompositeLit) SomethingTest {
	c1 := g.genExprs(expr.Elts)
	return SomethingTest{F: func() string {
		var out string
		if expr.Type != nil {
			out += g.genExpr(expr.Type).F()
		}
		if out == "AglVoid{}" {
			return out
		}
		out += g.Emit("{")
		if expr.Type != nil && TryCast[types.SetType](g.env.GetType(expr.Type)) {
			for i, el := range expr.Elts {
				out += g.genExpr(el).F()
				out += g.Emit(": {}")
				if i < len(expr.Elts)-1 {
					out += g.Emit(", ")
				}
			}
		} else {
			out += c1.F()
		}
		out += g.Emit("}")
		return out
	}}
}

func (g *Generator) genTupleExpr(expr *ast.TupleExpr) SomethingTest {
	t := g.env.GetType(expr)
	var isType bool
	if v, ok := t.(types.TypeType); ok {
		isType = true
		t = v.W
	}
	tup := types.ReplGenM(t, g.genMap).(types.TupleType)
	structName := tup.GoStr()
	structName1 := tup.GoStr2()
	var args []string
	for i, x := range expr.Values {
		xT := g.env.GetType2(x, g.fset)
		args = append(args, fmt.Sprintf("\tArg%d %s\n", i, types.ReplGenM(xT, g.genMap).GoStr()))
	}
	structStr := fmt.Sprintf("type %s struct {\n", structName1)
	structStr += strings.Join(args, "")
	structStr += "}\n"
	g.tupleStructs[structName] = structStr
	return SomethingTest{F: func() string {
		if isType {
			return g.Emit(structName)
		}
		out := g.Emit(structName) + g.Emit("{")
		for i, x := range expr.Values {
			out += g.Emit(fmt.Sprintf("Arg%d: ", i))
			out += g.genExpr(x).F()
			if i < len(expr.Values)-1 {
				out += g.Emit(", ")
			}
		}
		out += g.Emit("}")
		return out
	}}
}

func (g *Generator) genExprs(e []ast.Expr) SomethingTest {
	arr := make([]func() string, len(e))
	var bs []func() string
	for i, x := range e {
		tmp := g.genExpr(x)
		arr[i] = tmp.F
		bs = append(bs, tmp.B...)
	}
	return SomethingTest{F: func() string {
		var out string
		for i, el := range arr {
			out += el()
			if i < len(e)-1 {
				out += g.Emit(", ")
			}
		}
		return out
	}, B: bs}
}

func (g *Generator) genStmts(s []ast.Stmt) SomethingTest {
	var stmts []SomethingTest
	for _, stmt := range s {
		stmts = append(stmts, g.genStmt(stmt))
	}
	return SomethingTest{F: func() string {
		var out string
		for _, stmt := range stmts {
			for _, b := range stmt.B {
				out += b()
			}
			out += stmt.F()
		}
		return out
	}}
}

func (g *Generator) genBlockStmt(stmt *ast.BlockStmt) SomethingTest {
	c1 := g.genStmts(stmt.List)
	return SomethingTest{F: func() string {
		return c1.F()
	}}
}

func (g *Generator) genSpecs(specs []ast.Spec, tok token.Token) SomethingTest {
	return SomethingTest{F: func() string {
		var tmp string
		for _, spec := range specs {
			tmp += g.genSpec(spec, tok).F()
		}
		return tmp
	}}
}

func (g *Generator) genSpec(s ast.Spec, tok token.Token) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		switch spec := s.(type) {
		case *ast.ValueSpec:
			e := func(s string) string { return g.Emit(s, WithNode(spec)) }
			var namesArr []string
			for _, name := range spec.Names {
				namesArr = append(namesArr, name.Name)
			}
			out += e(g.prefix + tok.String() + " ")
			out += e(strings.Join(namesArr, ", "))
			if spec.Type != nil {
				out += e(" ") + g.genExpr(spec.Type).F()
			}
			if spec.Values != nil {
				out += e(" = ") + g.genExprs(spec.Values).F()
			}
			out += e("\n")
			return out
		case *ast.TypeSpec:
			e := func(s string) string { return g.Emit(s, WithNode(spec)) }
			if v, ok := spec.Type.(*ast.EnumType); ok {
				out += e(g.prefix) + e(g.genEnumType(spec.Name.Name, v)) + e("\n")
			} else {
				out += e(g.prefix + "type " + spec.Name.Name)
				if typeParams := spec.TypeParams; typeParams != nil {
					if len(typeParams.List) > 0 {
						out += e("[")
					}
					for i, field := range typeParams.List {
						namesStr := utils.MapJoin(field.Names, func(n *ast.LabelledIdent) string {
							return g.genIdent(n.Ident).F()
						}, ", ")
						namesStr = utils.SuffixIf(namesStr, " ")
						out += e(namesStr) + g.genExpr(field.Type).F()
						if i < len(typeParams.List)-1 {
							out += e(", ")
						}
					}
					if len(typeParams.List) > 0 {
						out += e("]")
					}
				}
				out += e(" ") + g.genExpr(spec.Type).F() + e("\n")
			}
			return out
		case *ast.ImportSpec:
			return out
		default:
			panic(fmt.Sprintf("%v", to(s)))
		}
	}}
}

func (g *Generator) genDecl(d ast.Decl) SomethingTest {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return g.genGenDecl(decl)
	case *ast.FuncDecl:
		return g.genFuncDecl(decl)
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
}

func (g *Generator) genGenDecl(decl *ast.GenDecl) SomethingTest {
	return g.genSpecs(decl.Specs, decl.Tok)
}

func (g *Generator) genDeclStmt(stmt *ast.DeclStmt) SomethingTest {
	return g.genDecl(stmt.Decl)
}

func (g *Generator) genIncDecStmt(stmt *ast.IncDecStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		e := func(s string) string { return g.Emit(s, WithNode(stmt)) }
		var op string
		switch stmt.Tok {
		case token.INC:
			op = "++"
		case token.DEC:
			op = "--"
		default:
			panic("")
		}
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += g.genExpr(stmt.X).F() + e(op)
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}}
}

func (g *Generator) genForStmt(stmt *ast.ForStmt) SomethingTest {
	e := func(s string) string { return g.Emit(s, WithNode(stmt)) }
	body := func() string { return g.incrPrefix(func() string { return g.genStmt(stmt.Body).F() }) }
	if stmt.Init == nil && stmt.Post == nil && stmt.Cond != nil {
		if v, ok := stmt.Cond.(*ast.BinaryExpr); ok {
			if v.Op == token.IN {
				xT := g.env.GetType(v.X)
				yT := g.env.GetType(v.Y)
				c1 := g.genExpr(v.Y)
				return SomethingTest{F: func() string {
					var out string
					if tup, ok := xT.(types.TupleType); ok && !TryCast[types.MapType](yT) {
						varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
						out += e(g.prefix+"for _, "+varName+" := range ") + c1.F() + e(" {\n")
						xTup := v.X.(*ast.TupleExpr)
						out += e(g.prefix + "\t")
						for i := range tup.Elts {
							out += g.genExpr(xTup.Values[i]).F()
							if i < len(tup.Elts)-1 {
								out += e(", ")
							}
						}
						out += e(" := ")
						for i := range tup.Elts {
							out += e(fmt.Sprintf("%s.Arg%d", varName, i))
							if i < len(tup.Elts)-1 {
								out += e(", ")
							}
						}
						out += e("\n")
					} else {
						switch yT.(type) {
						case types.ArrayType:
							out += e(g.prefix+"for _, ") + g.genExpr(v.X).F() + e(" := range ") + c1.F() + e(" {\n")
						case types.SetType:
							out += e(g.prefix+"for ") + g.genExpr(v.X).F() + e(" := range (") + c1.F() + e(").Iter() {\n")
						case types.MapType:
							xTup := v.X.(*ast.TupleExpr)
							key := func() string { return g.genExpr(xTup.Values[0]).F() }
							val := func() string { return g.genExpr(xTup.Values[1]).F() }
							y := func() string { return c1.F() }
							out += e(g.prefix+"for ") + key() + e(", ") + val() + e(" := range ") + y() + e(" {\n")
						default:
							panic("")
						}
					}
					out += body()
					out += e(g.prefix + "}\n")
					return out
				}}
			}
		}
	}
	tmp := func() string {
		var init, cond, post func() string
		var els []func() string
		if stmt.Init != nil {
			init = func() string {
				var out string
				g.WithInlineStmt(func() {
					out = g.genStmt(stmt.Init).F()
				})
				return out
			}
			els = append(els, init)
		}
		if stmt.Cond != nil {
			cond = func() string { return g.genExpr(stmt.Cond).F() }
			els = append(els, cond)
		}
		if stmt.Post != nil {
			post = func() string {
				var out string
				g.WithInlineStmt(func() {
					out = g.genStmt(stmt.Post).F()
				})
				return out
			}
			els = append(els, post)
		}
		var out string
		for i, el := range els {
			out += el()
			if i < len(els)-1 {
				out += e("; ")
			}
		}
		if out != "" {
			out += e(" ")
		}
		return out
	}
	return SomethingTest{F: func() string {
		var out string
		out += e(g.prefix+"for ") + tmp() + e("{\n")
		out += body()
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genRangeStmt(stmt *ast.RangeStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		e := func(s string) string { return g.Emit(s, WithNode(stmt)) }
		content3 := func() string {
			isCompLit := TryCast[*ast.CompositeLit](stmt.X)
			var out string
			if isCompLit {
				out += "("
			}
			out += g.genExpr(stmt.X).F()
			if isCompLit {
				out += ")"
			}
			return out
		}
		op := stmt.Tok
		if stmt.Key == nil && stmt.Value == nil {
			out += e(g.prefix+"for range ") + content3() + e(" {\n")
		} else if stmt.Value == nil {
			out += e(g.prefix+"for ") + g.genExpr(stmt.Key).F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		} else {
			out += e(g.prefix+"for ") + g.genExpr(stmt.Key).F() + e(", ") + g.genExpr(stmt.Value).F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		}
		out += g.incrPrefix(func() string {
			return g.genStmt(stmt.Body).F()
		})
		out += e(g.prefix + "}\n")
		return out
	}}
}

type SomethingTest struct {
	F func() string
	B []func() string
}

func (g *Generator) genReturnStmt(stmt *ast.ReturnStmt) SomethingTest {
	var bs []func() string
	var tmp SomethingTest
	if stmt.Result != nil {
		tmp = g.genExpr(stmt.Result)
		bs = append(bs, tmp.B...)
	}
	return SomethingTest{F: func() string {
		if stmt.Result == nil {
			return g.Emit(g.prefix + "return\n")
		}
		return g.Emit(g.prefix+"return ") + tmp.F() + g.Emit("\n")
	}, B: bs}
}

func (g *Generator) genExprStmt(stmt *ast.ExprStmt) SomethingTest {
	tmp := g.genExpr(stmt.X)
	bs := tmp.B
	return SomethingTest{F: func() string {
		var out string
		if !g.inlineStmt {
			out += g.Emit(g.prefix)
		}
		out += tmp.F()
		if !g.inlineStmt {
			out += g.Emit("\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) genAssignStmt(stmt *ast.AssignStmt) SomethingTest {
	e := func(s string) string { return g.Emit(s, WithNode(stmt)) }
	lhs := func() string { return "" }
	var after string
	rhsT := g.env.GetType(stmt.Rhs[0])
	if v, ok := rhsT.(types.CustomType); ok {
		rhsT = v.W
	}
	if len(stmt.Rhs) == 1 && TryCast[types.EnumType](rhsT) {
		enumT := rhsT.(types.EnumType)
		if len(stmt.Lhs) == 1 {
			lhs = func() string { return g.genExprs(stmt.Lhs).F() }
		} else {
			varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
			lhs = func() string { return e(varName) }
			var names []string
			var exprs []string
			for i, x := range stmt.Lhs {
				names = append(names, x.(*ast.Ident).Name)
				exprs = append(exprs, fmt.Sprintf("%s.%s_%d", varName, enumT.SubTyp, i))
			}
			if !g.inlineStmt {
				after += g.prefix
			}
			after += strings.Join(names, ", ") + " := " + strings.Join(exprs, ", ")
		}
	} else if len(stmt.Rhs) == 1 && TryCast[types.TupleType](rhsT) {
		if len(stmt.Lhs) == 1 {
			lhs = func() string { return g.genExprs(stmt.Lhs).F() }
		} else {
			if v, ok := rhsT.(types.TupleType); ok && v.KeepRaw {
				lhs = func() string { return g.genExprs(stmt.Lhs).F() }
			} else {
				varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				lhs = func() string { return e(varName) }
				rhs := stmt.Rhs[0]
				var names []string
				var exprs []string
				for i := range g.env.GetType(rhs).(types.TupleType).Elts {
					name := stmt.Lhs[i].(*ast.Ident).Name
					names = append(names, name)
					exprs = append(exprs, fmt.Sprintf("%s.Arg%d", varName, i))
				}
				if !g.inlineStmt {
					after += g.prefix
				}
				after += fmt.Sprintf("%s := %s", strings.Join(names, ", "), strings.Join(exprs, ", "))
			}
		}
	} else if len(stmt.Lhs) == 1 && TryCast[*ast.IndexExpr](stmt.Lhs[0]) && TryCast[*ast.MapType](stmt.Lhs[0].(*ast.IndexExpr).X) {
		lhs = func() string { return g.genExprs(stmt.Lhs).F() }
	} else {
		isMutStarMap := func() bool {
			if len(stmt.Lhs) == 1 {
				if v, ok := stmt.Lhs[0].(*ast.IndexExpr); ok {
					if vv, ok := g.env.GetType(v.X).(types.MutType); ok {
						if vvv, ok := vv.W.(types.StarType); ok {
							_, ok := vvv.X.(types.MapType)
							return ok
						}
					}
				}
			}
			return false
		}
		if isMutStarMap() {
			v := stmt.Lhs[0].(*ast.IndexExpr)
			lhs = func() string { return "(*" + g.genExpr(v.X).F() + ")[" + g.genExpr(v.Index).F() + "]" }
		} else {
			lhs = func() string { return g.genExprs(stmt.Lhs).F() }
		}
	}
	content2 := g.genExprs(stmt.Rhs)
	if len(stmt.Rhs) == 1 {
		if v, ok := g.env.GetType(stmt.Rhs[0]).(types.ResultType); ok && v.Native {
			switch tup := v.W.(type) {
			case types.TupleType:
				panic(fmt.Sprintf("need to implement AglWrapNative for tuple len %d", len(tup.Elts)))
			default:
				if !v.KeepRaw {
					if _, ok := v.W.(types.VoidType); ok {
						content2 = SomethingTest{F: func() string { return "AglWrapNative1(" + g.genExprs(stmt.Rhs).F() + ")" }}
					} else {
						content2 = SomethingTest{F: func() string { return "AglWrapNative2(" + g.genExprs(stmt.Rhs).F() + ")" }}
					}
				}
			}
		}
	}
	var bs []func() string
	bs = append(bs, content2.B...)
	return SomethingTest{F: func() string {
		var out string
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += lhs() + e(" "+stmt.Tok.String()+" ") + content2.F()
		if after != "" {
			out += e("\n")
		}
		if !g.inlineStmt && g.allowUnused {
			out += e("\n"+g.prefix+"AglNoop(") + lhs() + e(")") // Allow to have "declared and not used" variables
		}
		if after != "" {
			out += e(g.prefix + after)
		}
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) genIfLetStmt(stmt *ast.IfLetStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		gPrefix := g.prefix
		ass := stmt.Ass
		lhs := func() string { return g.genExpr(ass.Lhs[0]).F() }
		rhs := func() string { return g.incrPrefix(func() string { return g.genExpr(ass.Rhs[0]).F() }) }
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
		if !g.inlineStmt {
			out += g.Emit(gPrefix)
		}
		if _, ok := ass.Rhs[0].(*ast.TypeAssertExpr); ok {
			out += g.Emit("if ") + lhs() + g.Emit(", ok := ") + rhs() + g.Emit("; ok {\n")
			g.inlineStmt = false
		} else {
			out += g.Emit("if "+varName+" := ") + rhs() + g.Emit("; "+cond+" {\n")
			g.inlineStmt = false
			out += g.Emit(gPrefix+"\t") + lhs() + g.Emit(" := "+varName+"."+unwrapFn+"()\n")
		}
		if g.allowUnused {
			out += g.Emit(gPrefix+"\tAglNoop(") + lhs() + g.Emit(")\n")
		}
		out += g.incrPrefix(func() string { return g.genStmt(stmt.Body).F() })
		if stmt.Else != nil {
			switch stmt.Else.(type) {
			case *ast.IfStmt, *ast.IfLetStmt:
				out += g.Emit(gPrefix + "} else ")
				g.WithInlineStmt(func() {
					out += g.genStmt(stmt.Else).F()
				})
			default:
				out += g.Emit(gPrefix + "} else {\n")
				out += g.incrPrefix(func() string {
					return g.genStmt(stmt.Else).F()
				})
				out += g.Emit(gPrefix + "}\n")
			}
		} else {
			out += g.Emit(gPrefix + "}\n")
		}
		return out
	}}
}

func (g *Generator) genGuardLetStmt(stmt *ast.GuardLetStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		gPrefix := g.prefix
		ass := stmt.Ass
		lhs0, rhs0 := ass.Lhs[0], ass.Rhs[0]
		lhs := g.genExpr(lhs0).F()
		rhs := g.incrPrefix(func() string { return g.genExpr(rhs0).F() })
		body := g.incrPrefix(func() string { return g.genStmt(stmt.Body).F() })
		varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
		var cond string
		unwrapFn := "Unwrap"
		switch stmt.Op {
		case token.SOME:
			cond = fmt.Sprintf("%s.IsNone()", varName)
		case token.OK:
			cond = fmt.Sprintf("%s.IsErr()", varName)
			unwrapFn = "Err"
		case token.ERR:
			cond = fmt.Sprintf("%s.IsOk()", varName)
		default:
			panic("")
		}
		if _, ok := rhs0.(*ast.TypeAssertExpr); ok {
			out += gPrefix + fmt.Sprintf("%s, %s := %s\n", lhs, varName, rhs)
			out += gPrefix + fmt.Sprintf("if !%s {\n", varName)
			out += body
			out += gPrefix + "}\n"
		} else {
			out += gPrefix + fmt.Sprintf("%s := %s\n", varName, rhs)
			out += gPrefix + fmt.Sprintf("if %s {\n", cond)
			out += body
			out += gPrefix + "}\n"
			out += gPrefix + fmt.Sprintf("%s := %s.%s()\n", lhs, varName, unwrapFn)
		}
		if g.allowUnused {
			out += gPrefix + fmt.Sprintf("AglNoop(%s)\n", lhs)
		}
		return out
	}}
}

func (g *Generator) genIfStmt(stmt *ast.IfStmt) SomethingTest {
	var bs []func() string
	cond := g.genExpr(stmt.Cond)
	bs = append(bs, cond.B...)
	return SomethingTest{F: func() string {
		var out string
		gPrefix := g.prefix
		if !g.inlineStmt {
			out += g.Emit(gPrefix, WithNode(stmt))
		}
		out += g.Emit("if ", WithNode(stmt))
		if stmt.Init != nil {
			g.WithInlineStmt(func() {
				out += g.genStmt(stmt.Init).F() + g.Emit("; ")
			})
		}
		out += cond.F() + g.Emit(" {\n")
		g.inlineStmt = false
		out += g.incrPrefix(func() string {
			return g.genStmt(stmt.Body).F()
		})
		if stmt.Else != nil {
			switch stmt.Else.(type) {
			case *ast.IfStmt, *ast.IfLetStmt:
				out += g.Emit(gPrefix + "} else ")
				g.WithInlineStmt(func() {
					out += g.genStmt(stmt.Else).F()
				})
			default:

				out += g.Emit(gPrefix + "} else {\n")
				out += g.incrPrefix(func() string {
					return g.genStmt(stmt.Else).F()
				})
				out += g.Emit(gPrefix + "}\n")
			}
		} else {
			out += g.Emit(gPrefix + "}\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) genGuardStmt(stmt *ast.GuardStmt) SomethingTest {
	return SomethingTest{F: func() string {
		var out string
		cond := func() string { return g.genExpr(stmt.Cond).F() }
		gPrefix := g.prefix
		out += g.Emit(gPrefix+"if !(") + cond() + g.Emit(") {\n")
		out += g.incrPrefix(func() string {
			return g.genStmt(stmt.Body).F()
		})
		out += g.Emit(gPrefix + "}\n")
		return out
	}}
}

func (g *Generator) genDecls(f *ast.File) SomethingTest {
	var decls []func() string
	for _, decl := range f.Decls {
		switch declT := decl.(type) {
		case *ast.FuncDecl:
			fnT := g.env.GetType(declT)
			if fnT.(types.FuncType).IsGeneric() {
				g.genFuncDecls[fnT.String()] = declT
			}
		}
		decls = append(decls, g.genDecl(decl).F)
	}
	return SomethingTest{F: func() string {
		var out string
		for _, decl := range decls {
			out += decl()
		}
		return out
	}}
}

func (g *Generator) genFuncDecl(decl *ast.FuncDecl) SomethingTest {
	g.returnType = g.env.GetType(decl).(types.FuncType).Return
	c1 := g.genStmt(decl.Body)
	recv := func() string { return "" }
	var name, typeParamsStr, paramsStr, resultStr string
	if decl.Recv != nil {
		if len(decl.Recv.List) >= 1 {
			if tmp1, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if tmp2, ok := tmp1.X.(*ast.SelectorExpr); ok {
					if tmp2.Sel.Name == "Vec" {
						fnName := fmt.Sprintf("agl1.Vec.%s", decl.Name.Name)
						tmp := g.extensions[fnName]
						tmp.decl = decl
						g.extensions[fnName] = tmp
						return SomethingTest{F: func() string { return "" }}
					}
				}
			} else if tmp2, ok := decl.Recv.List[0].Type.(*ast.SelectorExpr); ok {
				if tmp2.Sel.Name == "String" {
					fnName := fmt.Sprintf("agl1.String.%s", decl.Name.Name)
					tmp := g.extensionsString[fnName]
					tmp.decl = decl
					g.extensionsString[fnName] = tmp
					return SomethingTest{F: func() string { return "" }}
				}
			}
		}
		recv = func() string {
			var out string
			if decl.Recv != nil {
				out += g.Emit(" (")
				out += g.joinList(decl.Recv)
				out += g.Emit(")")
			}
			return out
		}
	}
	fnT := g.env.GetType(decl)
	if g.genMap == nil && fnT.(types.FuncType).IsGeneric() {
		g.genFuncDecls[fnT.String()] = decl
		return SomethingTest{F: func() string { return "" }}
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
					content = g.genExpr(field.Type).F()
				}
			}
			namesStr := utils.MapJoin(field.Names, func(n *ast.LabelledIdent) string { return n.Name }, ", ")
			namesStr = utils.SuffixIf(namesStr, " ")
			fieldsItems = append(fieldsItems, namesStr+content)
		}
		paramsStr = strings.Join(fieldsItems, ", ")
	}
	if result := decl.Type.Result; result != nil {
		resultStr = types.ReplGenM(g.env.GetType(result), g.genMap).GoStr()
		resultStr = utils.PrefixIf(resultStr, " ")
	}
	if g.genMap != nil {
		typeParamsStr = ""
		for _, k := range slices.Sorted(maps.Keys(g.genMap)) {
			v := g.genMap[k]
			name += fmt.Sprintf("_%v_%v", k, v.GoStr())
		}
	}
	return SomethingTest{F: func() string {
		var out string
		out += g.Emit("func")
		out += recv()
		out += g.Emit(fmt.Sprintf("%s%s(%s)%s {\n", name, typeParamsStr, paramsStr, resultStr), WithNode(decl))
		if decl.Body != nil {
			out += g.incrPrefix(func() string {
				return c1.F()
			})
		}
		out += g.Emit("}\n", WithNode(decl))
		return out
	}}
}

func (g *Generator) joinList(l *ast.FieldList) (out string) {
	if l == nil {
		return ""
	}
	for i, field := range l.List {
		for j, n := range field.Names {
			out += g.genIdent(n.Ident).F()
			if j < len(field.Names)-1 {
				out += g.Emit(", ")
			}
		}
		if out != "" {
			out += g.Emit(" ")
		}
		out += g.genExpr(field.Type).F()
		if i < len(l.List)-1 {
			out += g.Emit(", ")
		}
	}
	return
}

type BeforeStmt struct {
	w SomethingTest
}

func (b *BeforeStmt) Content() string {
	return b.w.F()
}

func NewBeforeStmt(content SomethingTest) *BeforeStmt {
	return &BeforeStmt{w: content}
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
	aglImportBytes "bytes"
	aglImportCmp "cmp"
	aglImportFmt "fmt"
	aglImportIo "io"
	aglImportIter "iter"
	aglImportMaps "maps"
	aglImportMath "math"
	aglImportHttp "net/http"
	aglImportSlices "slices"
	aglImportStrconv "strconv"
	aglImportStrings "strings"
)`
}

func GenCore(packageName string) string {
	out := GeneratedFilePrefix
	out += fmt.Sprintf("package %s\n", packageName)
	out += GenHeaders()
	out += GenContent()
	return out
}

func GenContent() string {
	return `
func AglBytesEqual(a, b []byte) bool {
	return aglImportBytes.Equal(a, b)
}

func AglWrapNative1(err error) Result[AglVoid] {
	if err != nil {
		return MakeResultErr[AglVoid](err)
	}
	return MakeResultOk(AglVoid{})
}

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

func AglVec__ADD[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
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

func AglVecFirstIndex[T comparable](a []T, e T) Option[int] {
	for i, v := range a {
		if v == e {
			return MakeOptionSome(i)
		}
	}
	return MakeOptionNone[int]()
}

func AglVecFirstIndexWhere[T any](a []T, p func(T) bool) Option[int] {
	for i, v := range a {
		if p(v) {
			return MakeOptionSome(i)
		}
	}
	return MakeOptionNone[int]()
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

func AglVecContainsWhere[T comparable](a []T, p func(T) bool) bool {
	for _, v := range a {
		if p(v) {
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

func AglVecReduce[T, R any](a []T, acc R, f func(R, T) R) R {
	for _, v := range a {
		acc = f(acc, v)
	}
	return acc
}

func AglVecReduceInto[T, R any](a []T, acc R, f func(*R, T) AglVoid) R {
	for _, v := range a {
		f(&acc, v)
	}
	return acc
}

func AglMapReduce[K comparable, V, R any](m map[K]V, acc R, f func(R, DictEntry[K, V]) R) R {
	for k, v := range m {
		acc = f(acc, DictEntry[K, V]{Key: k, Value: v})
	}
	return acc
}

func AglMapReduceInto[K comparable, V, R any](m map[K]V, acc R, f func(*R, DictEntry[K, V]) AglVoid) R {
	for k, v := range m {
		f(&acc, DictEntry[K, V]{Key: k, Value: v})
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

func AglNoop(_ ...any) Result[AglVoid] { return MakeResultOk(AglVoid{}) }

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

func (a AglVec[T]) Len() int { return len(a) }

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

type DictEntry[K comparable, V any] struct {
	Key K
	Value V
}

type AglMap[K comparable, V any] map[K]V

func (m AglMap[K, V]) Iter() aglImportIter.Seq[K] { return AglMapKeys(m) }

func (m AglMap[K, V]) Len() int { return len(m) }

type AglSet[T comparable] map[T]struct{}

func (s AglSet[T]) Len() int { return len(s) }

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

func AglSetIter[T comparable](s AglSet[T]) aglImportIter.Seq[T] {
	return s.Iter()
}

func (s AglSet[T]) String() string {
	var tmp []string
	for k := range s {
		tmp = append(tmp, aglImportFmt.Sprintf("%v", k))
	}
	return aglImportFmt.Sprintf("set(%s)", aglImportStrings.Join(tmp, " "))
}

func (s AglSet[T]) Intersects(other Iterator[T]) bool {
	return AglSetIntersects(s, other)
}

func (s AglSet[T]) Contains(el T) bool {
	return AglSetContains(s, el)
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

// AglSetFormIntersection removes the elements of the set that aren't also in the given sequence.
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

func AglStringLines(s string) []string {
	s = aglImportStrings.ReplaceAll(s, "\r\n", "\n")
	return aglImportStrings.Split(s, "\n")
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

func AglStringAsBytes(s string) []byte {
	return []byte(s)
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

func AglVecJoined(a []string, s string) string {
	return aglImportStrings.Join(a, s)
}

func AglVecSum[T aglImportCmp.Ordered](a []T) (out T) {
	var zero T
	return AglVecReduce(a, zero, func(acc, el T) T { return acc + el })
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

func AglVecFirstWhere[T any](a []T, predicate func(T) bool) (out Option[T]) {
	if len(a) == 0 {
		return MakeOptionNone[T]()
	}
	for _, el := range a {
		if predicate(el) {
			return MakeOptionSome(el)
		}
	}
	return MakeOptionNone[T]()
}

func AglVecGet[T any](a []T, i int) (out Option[T]) {
	if i >= 0 && i <= len(a)-1 {
		return MakeOptionSome(a[i])
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

func AglMapLen[K comparable, V any](m map[K]V) int {
	return len(m)
}

func AglMapIndex[K comparable, V any](m map[K]V, index K) Option[V] {
	if el, ok := m[index]; ok {
		return MakeOptionSome(el)
	}
	return MakeOptionNone[V]()
}

func AglMapContainsKey[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

func AglMapFilter[K comparable, V any](m map[K]V, f func(DictEntry[K, V]) bool) map[K]V {
	out := make(map[K]V)
	for k, v := range m {
		if f(DictEntry[K, V]{Key: k, Value: v}) {
			out[k] = v
		}
	}
	return out
}

func AglMapMap[K comparable, V, R any](m map[K]V, f func(DictEntry[K, V]) R) []R {
	var out []R
	for k, v := range m {
		out = append(out, f(DictEntry[K, V]{Key: k, Value: v}))
	}
	return out
}

func AglMapKeys[K comparable, V any](m map[K]V) aglImportIter.Seq[K] {
	return aglImportMaps.Keys(m)
}

func AglMapValues[K comparable, V any](m map[K]V) aglImportIter.Seq[V] {
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

func AglIntString(v int) string { return aglImportStrconv.FormatInt(int64(v), 10) }
func AglI8String(v int8) string { return aglImportStrconv.FormatInt(int64(v), 10) }
func AglI16String(v int16) string { return aglImportStrconv.FormatInt(int64(v), 10) }
func AglI32String(v int32) string { return aglImportStrconv.FormatInt(int64(v), 10) }
func AglI64String(v int64) string { return aglImportStrconv.FormatInt(int64(v), 10) }

func AglIn[T comparable](e T, it Iterator[T]) bool {
	for el := range it.Iter() {
		if el == e {
			return true
		}
	}
	return false
}
`
}
