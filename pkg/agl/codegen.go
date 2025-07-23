package agl

import (
	"agl/pkg/ast"
	"agl/pkg/token"
	"agl/pkg/types"
	"agl/pkg/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
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
	genFuncDecls2    map[string]func() string
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
	emitEnabled      bool
	asType           bool
	ifVarName        string
	imports          map[string]*ast.ImportSpec
}

func (g *Generator) withAsType(clb func()) {
	prev := g.asType
	g.asType = true
	clb()
	g.asType = prev
}

func (g *Generator) WithIfVarName(n string, clb func()) {
	prev := g.ifVarName
	g.ifVarName = n
	clb()
	g.ifVarName = prev
}

func (g *Generator) WithoutEmit(clb func()) {
	g.emitEnabled = false
	clb()
	g.emitEnabled = true
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

func NewGenerator(env *Env, a, b *ast.File, imports map[string]*ast.ImportSpec, fset *token.FileSet, opts ...GeneratorOption) *Generator {
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
		genFuncDecls2:    make(map[string]func() string),
		genFuncDecls:     genFns,
		allowUnused:      conf.AllowUnused,
		emitEnabled:      true,
		imports:          imports,
	}
}

// SourceMapEntry represents a mapping from Go output to Agl source.
type SourceMapEntry struct {
	GoStartLine int    `json:"go_start_line"`
	GoStartCol  int    `json:"go_start_col"`
	GoEndLine   int    `json:"go_end_line"`
	GoEndCol    int    `json:"go_end_col"`
	AglFile     string `json:"agl_file"`
	AglLine     int    `json:"agl_line"`
	AglCol      int    `json:"agl_col"`
	NodeType    string `json:"node_type,omitempty"`
}

// nodeTypeName returns the type name of the ast.Node for debugging.
func nodeTypeName(n ast.Node) string {
	if n == nil {
		return ""
	}
	return fmt.Sprintf("%T", n)
}

// VLQ encoding for source maps (see https://sourcemaps.info/spec.html)
func encodeVLQ(value int) string {
	const (
		VLQ_BASE_SHIFT       = 5
		VLQ_BASE             = 1 << VLQ_BASE_SHIFT
		VLQ_BASE_MASK        = VLQ_BASE - 1
		VLQ_CONTINUATION_BIT = VLQ_BASE
	)
	vlq := value << 1
	if value < 0 {
		vlq = (-value << 1) + 1
	}
	result := ""
	for {
		digit := vlq & VLQ_BASE_MASK
		vlq >>= VLQ_BASE_SHIFT
		if vlq > 0 {
			digit |= VLQ_CONTINUATION_BIT
		}
		result += base64VLQChar(digit)
		if vlq == 0 {
			break
		}
	}
	return result
}

func base64VLQChar(digit int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	return string(chars[digit])
}

// GenerateStandardSourceMap outputs a standard-compliant source map (version 3) as a JSON string.
// It uses VLQ encoding for the mappings field, and assumes a single source file (the main Agl file).
// https://sokra.github.io/source-map-visualization
func (g *Generator) GenerateStandardSourceMap(goFile string) (string, error) {
	// Collect all fragments with source info
	entries := []SourceMapEntry{}
	line, col := 1, 1
	for _, frag := range g.fragments {
		fragText := frag.s
		startLine, startCol := line, col
		for _, r := range fragText {
			if r == '\n' {
				line++
				col = 1
			} else {
				col++
			}
		}
		endLine, endCol := line, col
		if frag.n != nil {
			pos := frag.n.Pos()
			aglPos := g.fset.Position(pos)
			entries = append(entries, SourceMapEntry{
				GoStartLine: startLine,
				GoStartCol:  startCol,
				GoEndLine:   endLine,
				GoEndCol:    endCol,
				AglFile:     aglPos.Filename,
				AglLine:     aglPos.Line,
				AglCol:      aglPos.Column,
				NodeType:    nodeTypeName(frag.n),
			})
		}
	}
	if len(entries) == 0 {
		return "", nil
	}
	// For standard source maps, we need to build the mappings string.
	// We'll map Go output lines to Agl source lines, using the first entry for each Go line.
	mappings := ""
	prevGenCol := 0
	prevSrcIdx := 0 // always 0, since we only use one source file
	prevSrcLine := 0
	prevSrcCol := 0
	//curLine := 1
	entryIdx := 0
	for lineNum := 1; lineNum <= entries[len(entries)-1].GoEndLine; lineNum++ {
		if lineNum > 1 {
			mappings += ";"
		}
		first := true
		for entryIdx < len(entries) && entries[entryIdx].GoStartLine == lineNum {
			entry := entries[entryIdx]
			genCol := entry.GoStartCol - 1 // 0-based
			srcLine := entry.AglLine - 1   // 0-based
			srcCol := entry.AglCol - 1     // 0-based
			if !first {
				mappings += ","
			}
			first = false
			// [generatedColumn, sourceIndex, sourceLine, sourceColumn]
			mappings += encodeVLQ(genCol - prevGenCol)
			prevGenCol = genCol
			mappings += encodeVLQ(prevSrcIdx) // always 0
			mappings += encodeVLQ(srcLine - prevSrcLine)
			prevSrcLine = srcLine
			mappings += encodeVLQ(srcCol - prevSrcCol)
			prevSrcCol = srcCol
			// names field omitted (no symbol names)
			entryIdx++
		}
		prevGenCol = 0 // reset at each new line
	}
	// Compose the source map object
	smap := map[string]interface{}{
		"version":  3,
		"file":     goFile,
		"sources":  []string{entries[0].AglFile},
		"names":    []string{},
		"mappings": mappings,
	}
	b, err := json.MarshalIndent(smap, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
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
	if !g.emitEnabled {
		return s
	}
	c := &EmitConf{}
	for _, opt := range opts {
		opt(c)
	}
	g.fragments = append(g.fragments, Frag{s: s, n: c.n})
	return s
}

func (g *Generator) genExtensionString(ext ExtensionString) (out string) {
	e := EmitWith(g, ext.decl)
	for _, _ = range slices.Sorted(maps.Keys(ext.gen)) {
		decl := ext.decl
		var name, resultStr string
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
		paramsStr := func() (out string) {
			firstArg := ast.Field{Names: []*ast.LabelledIdent{{Ident: &ast.Ident{Name: recvName}, Label: nil}}, Type: &ast.Ident{Name: recvTName}}
			paramsClone = append([]ast.Field{firstArg}, paramsClone...)
			if params := paramsClone; params != nil {
				for i, field := range params {
					out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return e(n.Name) }, ", ")
					if len(field.Names) > 0 {
						out += e(" ")
					}
					content := func() string {
						var content string
						if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
							if _, ok := v.W.(types.FuncType); ok {
								content = types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStrType()
							} else {
								content = g.genExpr(field.Type).FNoEmit(g)
							}
						} else {
							switch field.Type.(type) {
							case *ast.TupleExpr:
								content = g.env.GetType(field.Type).GoStr()
							default:
								content = g.genExpr(field.Type).FNoEmit(g)
							}
						}
						if i == 0 {
							if content != "String" {
								panic("")
							}
							content = e("string")
						}
						return content
					}
					out += content()
					if i < len(params)-1 {
						out += e(", ")
					}
				}
			}
			return
		}
		if result := decl.Type.Result; result != nil {
			resT := g.env.GetType(result)
			resultStr = utils.PrefixIf(resT.GoStrType(), " ")
		}
		out += e("func AglString"+name+"(") + paramsStr() + e(")"+resultStr+" {\n")
		if decl.Body != nil {
			out += g.incrPrefix(func() string {
				return g.genStmt(decl.Body).F()
			})
		}
		out += e("}\n")
	}
	return out
}

func (g *Generator) genExtension(ext Extension) (out string) {
	e := EmitWith(g, ext.decl)
	for _, key := range slices.Sorted(maps.Keys(ext.gen)) {
		ge := ext.gen[key]
		m := types.FindGen(ge.raw, ge.concrete)
		decl := ext.decl
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

		r := strings.NewReplacer(
			"[", "_",
			"]", "_",
		)

		var elts []string
		for _, k := range slices.Sorted(maps.Keys(m)) {
			elts = append(elts, fmt.Sprintf("%s_%s", k, r.Replace(m[k].GoStr())))
		}
		if _, ok := m["T"]; !ok {
			elts = append(elts, fmt.Sprintf("%s_%s", "T", r.Replace(recvTName)))
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
					out += MapJoin(e, params, func(field ast.Field) (out string) {
						out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return e(n.Name) }, ", ")
						if len(field.Names) > 0 {
							out += e(" ")
						}
						if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
							if _, ok := v.W.(types.FuncType); ok {
								out += e(types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStrType())
							} else {
								out += g.genExpr(field.Type).F()
							}
						} else {
							switch field.Type.(type) {
							case *ast.TupleExpr:
								out += e(g.env.GetType(field.Type).GoStr())
							default:
								out += g.genExpr(field.Type).F()
							}
						}
						return
					}, ", ")
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
						return e(" " + v)
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
			out += e("func AglVec"+name+"_"+strings.Join(elts, "_")) + typeParamsStr() + e("(") + paramsStr() + e(")") + resultStr() + e(" {\n")
			out += bodyStr()
			out += e("}\n")
		})
	}
	return
}

func (g *Generator) GenerateFrags(line int) (n ast.Node) {
	var out string
	for _, f := range g.fragments {
		out += f.s
		if len(strings.Split(out, "\n")) > line {
			return f.n
		}
	}
	return nil
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
	imports := make(map[string]*ast.ImportSpec)
	addImport := func(i *ast.ImportSpec) {
		key := i.Path.Value
		if i.Name != nil {
			key = i.Name.Name + "_" + key
		}
		imports[key] = i
	}
	for _, i := range g.imports {
		addImport(i)
	}
	for _, i := range g.a.Imports {
		addImport(i)
	}
	importsArr := make([]*ast.ImportSpec, len(imports))
	for i, k := range slices.Sorted(maps.Keys(imports)) {
		importsArr[i] = imports[k]
	}
	out += g.genPackage()
	out += g.genImports(importsArr)
	out4 := g.genDecls(g.b)
	out5 := g.genDecls(g.a)
	out += out4.F() + out5.F()
	var genFuncDeclStr string
	for _, k := range slices.Sorted(maps.Keys(g.genFuncDecls2)) {
		genFuncDeclStr += g.genFuncDecls2[k]()
	}
	out += genFuncDeclStr
	var extStr string
	for _, extKey := range slices.Sorted(maps.Keys(g.extensions)) {
		extStr += g.genExtension(g.extensions[extKey])
	}
	out += extStr
	var extStringStr string
	for _, extKey := range slices.Sorted(maps.Keys(g.extensionsString)) {
		extStringStr += g.genExtensionString(g.extensionsString[extKey])
	}
	out += extStringStr
	var tupleStr string
	for _, k := range slices.Sorted(maps.Keys(g.tupleStructs)) {
		tupleStr += g.tupleStructs[k]
	}
	out += g.Emit(tupleStr)
	return
}

func (g *Generator) PkgName() string {
	return g.a.Name.Name
}

func (g *Generator) genPackage() string {
	return g.Emit("package "+g.a.Name.Name+"\n", WithNode(g.a.Name))
}

func (g *Generator) genImports(imports []*ast.ImportSpec) (out string) {
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
	if len(imports) == 1 {
		spec := imports[0]
		out += g.Emit("import "+genRow(spec), WithNode(spec))
	} else if len(imports) > 1 {
		out += g.Emit("import (\n")
		for _, spec := range imports {
			out += g.Emit("\t"+genRow(spec), WithNode(spec))
		}
		out += g.Emit(")\n")
	}
	return
}

func (g *Generator) genStmt(s ast.Stmt) (out GenFrag) {
	//p("genStmt", to(s))
	switch stmt := s.(type) {
	case *ast.BlockStmt:
		return g.genBlockStmt(stmt)
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
	default:
		panic(fmt.Sprintf("%v %v", s, to(s)))
	}
}

func (g *Generator) genExpr(e ast.Expr) (out GenFrag) {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *ast.IfExpr:
		return g.genIfStmt(expr)
	case *ast.IfLetExpr:
		return g.genIfLetStmt(expr)
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
	case *ast.RangeExpr:
		return g.genRangeExpr(expr)
	default:
		panic(fmt.Sprintf("%v", to(e)))
	}
}

func (g *Generator) genIdent(expr *ast.Ident) (out GenFrag) {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
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
		switch v := t.(type) {
		case types.TypeType:
			t = v.W
			switch typ := t.(type) {
			case types.GenericType:
				if typ.IsType {
					for k, vv := range g.genMap {
						if typ.Name == k {
							typ.Name = vv.GoStr()
							typ.W = vv
						}
					}
					return e(typ.GoStr())
				}
			}
		}
		switch expr.Name {
		case "make":
			return e("make")
		case "abs":
			return e("AglAbs")
		}
		if v := g.env.Get(expr.Name); v != nil {
			switch vv := v.(type) {
			case types.TypeType:
				return e(v.GoStr())
			case types.FuncType:
				name := expr.Name
				if vv.Pub {
					name = "AglPub_" + name
				}
				return e(name)
			default:
				return e(expr.Name)
			}
		}
		return e(expr.Name)
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

func (g *Generator) genShortFuncLit(expr *ast.ShortFuncLit) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Body)
	return GenFrag{F: func() string {
		var out string
		t := g.env.GetType(expr).(types.FuncType)
		var returnStr, argsStr string
		if len(t.Params) > 0 {
			var tmp []string
			for i, arg := range t.Params {
				n := fmt.Sprintf("aglArg%d", i)
				if len(expr.Args) > 0 {
					switch v := expr.Args[i].(type) {
					case *ast.Ident:
						n = v.Name
					}
				}
				tmp = append(tmp, fmt.Sprintf("%s %s", n, types.ReplGenM(arg, g.genMap).GoStr()))
			}
			argsStr = strings.Join(tmp, ", ")
		}
		ret := types.ReplGenM(t.Return, g.genMap)
		if ret != nil {
			returnStr = " "
			val := ret.GoStrType()
			if val == "AglVoid{}" {
				val = "AglVoid"
			}
			returnStr += val
		}
		out = e(fmt.Sprintf("func(%s)%s {\n", argsStr, returnStr))
		for i, _ := range t.Params {
			n := fmt.Sprintf("aglArg%d", i)
			if len(expr.Args) > 0 {
				switch v := expr.Args[i].(type) {
				case *ast.TupleExpr:
					for j, val := range v.Values {
						switch vv := val.(type) {
						case *ast.Ident:
							if vv.Name != "_" {
								out += e(g.prefix + "\t" + vv.Name + " := " + n + fmt.Sprintf(".Arg%d", j) + "\n")
							}
						case *ast.TupleExpr: // TODO should be recursive
							for k, ee := range vv.Values {
								id := ee.(*ast.Ident)
								if id.Name != "_" {
									out += e(g.prefix + "\t" + id.Name + " := " + n + fmt.Sprintf(".Arg%d", j) + fmt.Sprintf(".Arg%d", k) + "\n")
								}
							}
						}
					}
				}
			}
		}
		out += g.incrPrefix(func() string {
			return c1.F()
		})
		out += e(g.prefix + "}")
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

func (g *Generator) genTypeAssertExpr(expr *ast.TypeAssertExpr) GenFrag {
	e := EmitWith(g, expr)
	if expr.Type != nil {
		c1 := g.genExpr(expr.X)
		c2 := g.genExpr(expr.Type)
		return GenFrag{F: func() string { return c1.F() + e(".(") + c2.F() + e(")") }}
	} else {
		c1 := g.genExpr(expr.X)
		return GenFrag{F: func() string { return c1.F() + e(".(type)") }}
	}
}

func (g *Generator) genStarExpr(expr *ast.StarExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("*") + c1.F() }}
}

func (g *Generator) genMapType(expr *ast.MapType) GenFrag {
	e := EmitWith(g, expr)
	t := g.env.GetType2(expr.Value, g.fset).GoStr()
	c1 := g.genExpr(expr.Key)
	return GenFrag{F: func() string { return e("map[") + c1.F() + e("]"+t) }}
}

func (g *Generator) genSomeExpr(expr *ast.SomeExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("MakeOptionSome(") + c1.F() + e(")") }}
}

func (g *Generator) genOkExpr(expr *ast.OkExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("MakeResultOk(") + c1.F() + e(")") }}
}

func (g *Generator) genErrExpr(expr *ast.ErrExpr) GenFrag {
	e := EmitWith(g, expr)
	t := g.env.GetType(expr).(types.ErrType).T.GoStrType()
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("MakeResultErr["+t+"](") + c1.F() + e(")") }}
}

func (g *Generator) genChanType(expr *ast.ChanType) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Value)
	return GenFrag{F: func() string { return e("chan ") + c1.F() }}
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

func (g *Generator) genOrBreakExpr(expr *ast.OrBreakExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	return GenFrag{F: func() string {
		return e("AglIdentity(" + varName + ").Unwrap()")
	}, B: []func() string{func() string {
		gPrefix := g.prefix
		check := getCheck(g.env.GetType(expr.X))
		out := e(gPrefix+varName+" := ") + c1.F() + e("\n")
		out += e(gPrefix + "if " + varName + "." + check + " {\n")
		out += e(gPrefix + "\tbreak")
		if expr.Label != nil {
			out += e(" " + expr.Label.String())
		}
		out += e("\n")
		out += e(gPrefix + "}\n")
		return out
	}}}
}

func (g *Generator) genOrContinueExpr(expr *ast.OrContinueExpr) (out GenFrag) {
	e := EmitWith(g, expr)
	content1 := g.genExpr(expr.X)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	return GenFrag{F: func() string {
		return e("AglIdentity(" + varName + ").Unwrap()")
	}, B: []func() string{func() string {
		gPrefix := g.prefix
		before := ""
		before += e(gPrefix+varName+" := ") + content1.F() + e("\n")
		before += e(gPrefix + fmt.Sprintf("if %s.%s {\n", varName, check))
		before += e(gPrefix + "\tcontinue")
		if expr.Label != nil {
			before += e(" " + expr.Label.String())
		}
		before += e("\n")
		before += e(gPrefix + "}\n")
		return before
	}}}
}

func (g *Generator) genOrReturn(expr *ast.OrReturnExpr) (out GenFrag) {
	e := EmitWith(g, expr)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	c1 := g.genExpr(expr.X)
	returnType := g.returnType
	return GenFrag{F: func() string {
		return e("AglIdentity(" + varName + ")")
	}, B: []func() string{func() string {
		out := ""
		out += e(g.prefix+varName+" := ") + c1.F() + e("\n")
		out += e(g.prefix + fmt.Sprintf("if %s.%s {\n", varName, check))
		if returnType == nil {
			out += e(g.prefix + "\treturn\n")
		} else {
			switch retT := returnType.(type) {
			case types.ResultType:
				out += e(g.prefix + fmt.Sprintf("\treturn MakeResultErr[%s](%s.Err())\n", retT.W.GoStrType(), varName))
			case types.OptionType:
				out += e(g.prefix + fmt.Sprintf("\treturn MakeOptionNone[%s]()\n", retT.W.GoStrType()))
			case types.VoidType:
				out += e(g.prefix + fmt.Sprintf("\treturn\n"))
			default:
				assert(false, "cannot use or_return in a function that does not return void/Option/Result")
			}
		}
		out += e(g.prefix + "}\n")
		return out
	}}}
}

func (g *Generator) genUnaryExpr(expr *ast.UnaryExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		return e(expr.Op.String()) + c1.F()
	}}
}

func (g *Generator) genSendStmt(expr *ast.SendStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Chan)
	c2 := g.genExpr(expr.Value)
	return GenFrag{F: func() string {
		var out string
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += c1.F() + e(" <- ") + c2.F()
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}}
}

func (g *Generator) genSelectStmt(expr *ast.SelectStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Body)
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix + "select {\n")
		out += c1.F()
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genLabeledStmt(expr *ast.LabeledStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Stmt)
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix + expr.Label.Name + ":\n")
		out += c1.F()
		return out
	}}
}

func (g *Generator) genBranchStmt(expr *ast.BranchStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := GenFrag{F: func() string { return "" }}
	if expr.Label != nil {
		c1 = g.genExpr(expr.Label)
	}
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix + expr.Tok.String())
		if expr.Label != nil {
			out += e(" ") + c1.F()
		}
		out += e("\n")
		return out
	}}
}

func (g *Generator) genDeferStmt(expr *ast.DeferStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Call)
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix+"defer ") + c1.F() + e("\n")
		return out
	}}
}

func (g *Generator) genGoStmt(expr *ast.GoStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Call)
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix+"go ") + c1.F() + e("\n")
		return out
	}}
}

func (g *Generator) genEmptyStmt(expr *ast.EmptyStmt) (out GenFrag) {
	return GenFrag{F: func() string {
		return ""
	}}
}

type Emitter interface {
	Emit(string) string
}

type EmitterFunc func(string) string

func (e EmitterFunc) Emit(s string) string {
	return e(s)
}

func MapJoin[T any](e Emitter, a []T, clb func(T) string, sep string) (out string) {
	for i, el := range a {
		out += clb(el)
		if i < len(a)-1 {
			out += e.Emit(sep)
		}
	}
	return
}

func EmitWith(g *Generator, n ast.Node) EmitterFunc {
	return func(s string) string { return g.Emit(s, WithNode(n)) }
}

func (g *Generator) genMatchExpr(expr *ast.MatchExpr) GenFrag {
	e := EmitWith(g, expr)
	content1 := g.genExpr(expr.Init)
	initT := g.env.GetType(expr.Init)
	return GenFrag{F: func() string {
		var out string
		id := g.varCounter.Add(1)
		varName := fmt.Sprintf(`aglTmp%d`, id)
		errName := fmt.Sprintf(`aglTmpErr%d`, id)
		gPrefix := g.prefix
		switch v := initT.(type) {
		case types.ResultType:
			if v.Native {
				out += e(varName+", "+errName+" := AglWrapNative2(") + content1.F() + e(").NativeUnwrap()\n")
			} else {
				out += e(varName+" := ") + content1.F() + e("\n")
			}
			if expr.Body != nil {
				out += MapJoin(e, expr.Body.List, func(cc ast.Stmt) (out string) {
					c := cc.(*ast.MatchClause)
					if v.Native {
						assignOp := func(op string) string { return utils.Ternary(op == "_", "=", ":=") }
						switch v := c.Expr.(type) {
						case *ast.OkExpr:
							binding := func() string { return g.genExpr(v.X).F() }
							out += e(gPrefix + "if " + errName + " == nil {\n" + gPrefix + "\t")
							op := binding()
							out += op + e(" "+assignOp(op)+" *"+varName+"\n")
						case *ast.ErrExpr:
							binding := func() string { return g.genExpr(v.X).F() }
							out += e(gPrefix + "if " + errName + " != nil {\n" + gPrefix + "\t")
							op := binding()
							out += op + e(" "+assignOp(op)+" "+errName+"\n")
						default:
							panic("")
						}
					} else {
						switch v := c.Expr.(type) {
						case *ast.OkExpr:
							out += e(gPrefix+"if "+varName+".IsOk() {\n"+gPrefix+"\t") + g.genExpr(v.X).F() + e(" := "+varName+".Unwrap()\n")
						case *ast.ErrExpr:
							out += e(gPrefix+"if "+varName+".IsErr() {\n"+gPrefix+"\t") + g.genExpr(v.X).F() + e(" := "+varName+".Err()\n")
						default:
							panic("")
						}
					}
					content3 := g.incrPrefix(func() string {
						return g.genStmts(c.Body).F()
					})
					out += content3
					out += e(gPrefix + "}")
					return
				}, "\n")
			}
		case types.OptionType:
			out += e(varName+" := ") + content1.F() + e("\n")
			if expr.Body != nil {
				out += MapJoin(e, expr.Body.List, func(cc ast.Stmt) (out string) {
					c := cc.(*ast.MatchClause)
					switch v := c.Expr.(type) {
					case *ast.SomeExpr:
						out += e(gPrefix+"if "+varName+".IsSome() {\n"+gPrefix+"\t") + g.genExpr(v.X).F() + e(" := "+varName+".Unwrap()\n")
					case *ast.NoneExpr:
						out += e(gPrefix + "if " + varName + ".IsNone() {\n")
					default:
						panic("")
					}
					content3 := g.incrPrefix(func() string {
						return g.genStmts(c.Body).F()
					})
					out += content3
					out += e(gPrefix + "}")
					return
				}, "\n")
			}
		case types.EnumType:
			if expr.Body != nil {
				for i, cc := range expr.Body.List {
					c := cc.(*ast.MatchClause)
					if i > 0 {
						out += e(gPrefix + "} else ")
					}
					switch cv := c.Expr.(type) {
					case *ast.CallExpr:
						sel := cv.Fun.(*ast.SelectorExpr)
						out += e("if ") + g.genExpr(expr.Init).F() + e(".Tag == "+v.Name+"_"+sel.Sel.Name+" {\n")
						for j, id := range cv.Args {
							rhs := func() string { return g.genExpr(expr.Init).F() + e("."+v.Fields[i].Name+"_"+strconv.Itoa(j)) }
							if id.(*ast.Ident).Name == "_" {
								out += e(gPrefix+"\t_ = ") + rhs() + e("\n")
							} else {
								out += e(gPrefix+"\t") + g.genExpr(id).F() + e(" := ") + rhs() + e("\n")
							}
						}
						out += e(gPrefix) + g.genStmts(c.Body).F()
					case *ast.SelectorExpr:
						out += e("if ") + g.genExpr(expr.Init).F() + e(".Tag == "+v.Name+"_"+cv.Sel.Name+" {\n")
						out += e(gPrefix) + g.genStmts(c.Body).F()
					default:
						panic(fmt.Sprintf("%v", to(c.Expr)))
					}
				}
				out += e(gPrefix + "} else {\n")
				out += e(gPrefix + "\tpanic(\"match on enum should be exhaustive\")\n")
				out += e(gPrefix + "}")
			}
		default:
			panic(fmt.Sprintf("%v", to(initT)))
		}
		return out
	}}
}

func (g *Generator) genTypeSwitchStmt(expr *ast.TypeSwitchStmt) GenFrag {
	c1 := g.genStmt(expr.Assign)
	return GenFrag{F: func() string {
		var out string
		e := EmitWith(g, expr)
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
			out += c1.F()
		})
		out += e(" {\n")

		for _, ccr := range expr.Body.List {
			cc := ccr.(*ast.CaseClause)
			out += e(g.prefix)
			if cc.List != nil {
				out += e("case ")
				out += MapJoin(e, cc.List, func(el ast.Expr) string { return g.genExpr(el).F() }, ", ")
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

func (g *Generator) genSwitchStmt(expr *ast.SwitchStmt) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
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
		c1 := GenFrag{F: func() string { return "" }}
		if expr.Tag != nil {
			c1 = g.genExpr(expr.Tag)
		}
		content2 := func() string {
			var out string
			if expr.Tag != nil {
				tagT := g.env.GetType(expr.Tag)
				switch tagT.(type) {
				case types.EnumType:
					tagIsEnum = true
					out += c1.F() + e(".Tag"+" ")
				default:
					if v := c1.F(); v != "" {
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
							out += MapJoin(e, expr1.List, func(el ast.Expr) string {
								if sel, ok := el.(*ast.SelectorExpr); ok {
									return e(fmt.Sprintf("%s_%s", tagT.Name, sel.Sel.Name)) // TODO: validate Sel.Name is an actual field
								} else {
									return g.genExpr(el).F()
								}
							}, ", ")
						} else {
							out += MapJoin(e, expr1.List, func(el ast.Expr) string { return g.genExpr(el).F() }, ", ")
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

func (g *Generator) genCommClause(expr *ast.CommClause) GenFrag {
	e := EmitWith(g, expr)
	c1 := GenFrag{F: func() string { return "" }}
	c2 := GenFrag{F: func() string { return "" }}
	if expr.Comm != nil {
		c1 = g.genStmt(expr.Comm)
	}
	if expr.Body != nil {
		c2 = g.genStmts(expr.Body)
	}
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix)
		if expr.Comm != nil {
			g.WithInlineStmt(func() {
				out += e("case ") + c1.F() + e(":")
			})
		} else {
			out += e("default:")
		}
		out += e("\n")
		if expr.Body != nil {
			out += c2.F()
		}
		return out
	}}
}

func (g *Generator) genNoneExpr(expr *ast.NoneExpr) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
		nT := types.ReplGenM(g.env.GetType(expr), g.genMap)
		var typeStr string
		switch v := nT.(type) {
		case types.OptionType:
			typeStr = v.W.GoStrType()
		case types.TypeType:
			typeStr = v.GoStrType()
		default:
			panic(fmt.Sprintf("%v", to(nT)))
		}
		return e("MakeOptionNone[" + typeStr + "]()")
	}}
}

func (g *Generator) genInterfaceType(expr *ast.InterfaceType) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
		var out string
		if expr.Methods == nil || len(expr.Methods.List) == 0 {
			return e("interface{}")
		}
		out += e("interface {\n")
		if expr.Methods != nil {
			for _, m := range expr.Methods.List {
				content1 := g.env.GetType(m.Type).(types.FuncType).GoStrType()
				out += e(g.prefix + "\t" + m.Names[0].Name + strings.TrimPrefix(content1, "func") + "\n")
			}
		}
		out += e("}")
		return out
	}}
}

func (g *Generator) genEllipsis(expr *ast.Ellipsis) GenFrag {
	c1 := g.genExpr(expr.Elt)
	return GenFrag{F: func() string {
		content1 := g.incrPrefix(func() string {
			return c1.F()
		})
		return "..." + content1
	}}
}

func (g *Generator) genParenExpr(expr *ast.ParenExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		var out string
		out += e("(")
		out += g.incrPrefix(func() string {
			return c1.F()
		})
		out += e(")")
		return out
	}}
}

func (g *Generator) genFuncLit(expr *ast.FuncLit) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Body)
	return GenFrag{F: func() string {
		var out string
		out += g.genFuncType(expr.Type).F() + e(" {\n")
		out += g.incrPrefix(func() string {
			return c1.F()
		})
		out += e(g.prefix + "}")
		return out
	}}
}

func (g *Generator) genStructType(expr *ast.StructType) GenFrag {
	return GenFrag{F: func() string {
		var out string
		e := EmitWith(g, expr)
		gPrefix := g.prefix
		if expr.Fields == nil || len(expr.Fields.List) == 0 {
			return e("struct{}")
		}
		out += e("struct {\n")
		for _, field := range expr.Fields.List {
			out += e(gPrefix + "\t")
			out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return e(n.Name) }, ", ")
			out += e(" ")
			out += g.genExpr(field.Type).F()
			if field.Tag != nil {
				out += e(" ") + g.genExpr(field.Tag).F()
			}
			out += e("\n")
		}
		out += e(gPrefix + "}")
		return out
	}}
}

func (g *Generator) genFuncType(expr *ast.FuncType) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
		var out string
		out = e("func")
		var typeParamsStr string
		if typeParams := expr.TypeParams; typeParams != nil {
			typeParamsStr = g.joinList(expr.TypeParams)
			typeParamsStr = utils.WrapIf(typeParamsStr, "[", "]")
		}
		out += typeParamsStr
		out += e("(")
		var paramsStr string
		if params := expr.Params; params != nil {
			paramsStr += MapJoin(e, params.List, func(field *ast.Field) (out string) {
				out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return e(n.Name) }, ", ")
				if len(field.Names) > 0 {
					out += e(" ")
				}
				if _, ok := field.Type.(*ast.TupleExpr); ok {
					out += e(g.env.GetType(field.Type).GoStr())
				} else if id, ok := field.Type.(*ast.Ident); ok && TryCast[types.GenericType](g.env.GetType(id)) {
					typ := g.env.GetType(id).(types.GenericType)
					if vv, ok := g.genMap[id.Name]; ok {
						typ.Name = vv.GoStr()
						typ.W = vv
						out += e(typ.GoStr())
					} else {
						out += g.genExpr(field.Type).F()
					}
				} else {
					out += g.genExpr(field.Type).F()
				}
				return
			}, ", ")
		}
		out += paramsStr
		out += e(")")
		content1 := g.incrPrefix(func() string {
			if expr.Result != nil {
				return e(" ") + g.genExpr(expr.Result).F()
			} else {
				return ""
			}
		})
		out += content1
		return out
	}}
}

func (g *Generator) genIndexExpr(expr *ast.IndexExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	c2 := g.genExpr(expr.Index)
	return GenFrag{F: func() string {
		var out string
		out += c1.F()
		out += e("[")
		out += c2.F()
		out += e("]")
		return out
	}}
}

func (g *Generator) genLabelledArg(expr *ast.LabelledArg) GenFrag {
	return g.genExpr(expr.X)
}

func (g *Generator) genRangeExpr(expr *ast.RangeExpr) GenFrag {
	e := EmitWith(g, expr)
	start := g.genExpr(expr.Start)
	end := g.genExpr(expr.End_)
	op := func() string {
		if expr.Op == token.RANGEOPEQ {
			return e("true")
		} else {
			return e("false")
		}
	}
	return GenFrag{F: func() (out string) {
		out += e("AglNewRange["+g.env.GetType(expr).(types.RangeType).Typ.GoStrType()+"](") + start.F() + e(", ") + end.F() + e(", ") + op() + e(")")
		return
	}}
}

func (g *Generator) genDumpExpr(expr *ast.DumpExpr) GenFrag {
	e := EmitWith(g, expr)
	content1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		return content1.F()
	}, B: []func() string{func() string {
		varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
		safeContent1 := strconv.Quote(content1.FNoEmit(g))
		before := e(g.prefix+varName+" := ") + content1.F() + e("\n")
		before += e(g.prefix + "fmt.Printf(\"" + g.fset.Position(expr.X.Pos()).String() + ": %s: %v\\n\", " + safeContent1 + ", " + varName + ")\n")
		return before
	}}}
}

func (g *Generator) genSliceExpr(expr *ast.SliceExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		var out string
		out += c1.F()
		out += e("[")
		if expr.Low != nil {
			out += g.genExpr(expr.Low).F()
		}
		out += e(":")
		if expr.High != nil {
			out += g.genExpr(expr.High).F()
		}
		if expr.Max != nil {
			out += e(":")
			out += g.genExpr(expr.Max).F()
		}
		out += e("]")
		return out
	}}
}

func (g *Generator) genIndexListType(expr *ast.IndexListExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	c2 := g.genExprs(expr.Indices)
	return GenFrag{F: func() string {
		return c1.F() + e("[") + c2.F() + e("]")
	}}
}

func (g *Generator) genSelectorExpr(expr *ast.SelectorExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	c2 := g.genExpr(expr.Sel)
	return GenFrag{F: func() string {
		var out string
		content1 := func() string { return c1.F() }
		name := expr.Sel.Name
		t := types.Unwrap(g.env.GetType(expr.X))
		switch t.(type) {
		case types.TupleType:
			name = "Arg" + name
		case types.EnumType:
			content2 := func() string { return c2.F() }
			var out string
			if expr.Sel.Name == "RawValue" {
				out = content1() + e(".") + content2()
			} else {
				out = e("Make_") + content1() + e("_") + content2()
				if _, ok := g.env.GetType(expr).(types.EnumType); ok { // TODO
					out += e("()")
				}
			}
			return out
		}
		out = content1() + e("."+name)
		return out
	}}
}

func (g *Generator) genBubbleOptionExpr(expr *ast.BubbleOptionExpr) GenFrag {
	e := EmitWith(g, expr)
	returnType := g.returnType
	content1 := g.genExpr(expr.X)
	var out string
	switch exprXT := g.env.GetInfo(expr.X).Type.(type) {
	case types.OptionType:
		if exprXT.Bubble {
			if exprXT.Native {
				varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
				return GenFrag{F: func() string { return e("AglIdentity(" + varName + ")") }, B: []func() string{func() string {
					out := e(g.prefix+varName+", ok := ") + content1.F() + e("\n")
					out += e(g.prefix + "if !ok {\n")
					out += e(g.prefix + "\treturn MakeOptionNone[" + exprXT.W.GoStr() + "]()\n")
					out += e(g.prefix + "}\n")
					return out
				}}}
			} else {
				varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
				return GenFrag{F: func() string { return e(varName + ".Unwrap()") }, B: []func() string{func() string {
					out := e(g.prefix+varName+" := ") + content1.F() + e("\n")
					out += e(g.prefix + "if " + varName + ".IsNone() {\n")
					out += e(g.prefix + "\treturn MakeOptionNone[" + returnType.(types.OptionType).W.String() + "]()\n")
					out += e(g.prefix + "}\n")
					return out
				}}}
			}
		} else {
			if exprXT.Native {
				id := g.varCounter.Add(1)
				varName := fmt.Sprintf("aglTmpVar%d", id)
				errName := fmt.Sprintf("aglTmpErr%d", id)
				out = fmt.Sprintf(`AglIdentity(%s)`, varName)
				return GenFrag{F: func() string { return e(out) }, B: []func() string{func() string {
					out := e(g.prefix+varName+", "+errName+" := ") + content1.F() + e("\n")
					out += e(g.prefix + "if " + errName + " != nil {\n")
					out += e(g.prefix + "\tpanic(" + errName + ")\n")
					out += e(g.prefix + "}\n")
					return out
				}}}
			} else {
				return GenFrag{F: func() string { return content1.F() + e(".Unwrap()") }}
			}
		}
	case types.TypeAssertType:
		content1 := g.genExpr(expr.X)
		id := g.varCounter.Add(1)
		varName := fmt.Sprintf("aglTmpVar%d", id)
		okName := fmt.Sprintf("aglTmpOk%d", id)
		return GenFrag{F: func() string { return e(varName) }, B: []func() string{func() string {
			out := e(g.prefix+varName+", "+okName+" := ") + content1.F() + e("\n")
			out += e(g.prefix + "if !" + okName + " {\n")
			if v, ok := returnType.(types.OptionType); ok {
				out += e(g.prefix + "\tMakeOptionNone[" + v.W.GoStrType() + "]()\n")
			} else {
				out += e(g.prefix + "\tpanic(\"type assert failed\")\n")
			}
			out += e(g.prefix + "}\n")
			return out
		}}}
	default:
		panic("")
	}
}

func (g *Generator) genBubbleResultExpr(expr *ast.BubbleResultExpr) (out GenFrag) {
	e := EmitWith(g, expr)
	returnType := g.returnType
	getVar := func() (string, string) {
		id := g.varCounter.Add(1)
		varName := fmt.Sprintf("aglTmpVar%d", id)
		errName := fmt.Sprintf("aglTmpErr%d", id)
		return varName, errName
	}
	content1 := g.genExpr(expr.X)
	exprXT := MustCast[types.ResultType](g.env.GetType(expr.X))
	if exprXT.Bubble {
		varName, errName := getVar()
		if _, ok := exprXT.W.(types.VoidType); ok && exprXT.Native {
			return GenFrag{F: func() string { return e(`AglNoop()`) }, B: []func() string{func() string {
				out := e(g.prefix+"if "+errName+" := ") + content1.F() + e("; "+errName+" != nil {\n")
				out += e(g.prefix + "\treturn MakeResultErr[" + exprXT.W.GoStrType() + "](" + errName + ")\n")
				out += e(g.prefix + "}\n")
				return out
			}}}
		} else if exprXT.Native {
			return GenFrag{F: func() string { return e("AglIdentity(" + varName + ")") }, B: []func() string{func() string {
				out := e(g.prefix+varName+", "+errName+" := ") + content1.F() + e("\n")
				out += e(g.prefix + "if " + errName + " != nil {\n")
				out += e(g.prefix + "\treturn MakeResultErr[" + returnType.(types.ResultType).W.GoStrType() + "](" + errName + ")\n")
				out += e(g.prefix + "}\n")
				return out
			}}}
		} else if exprXT.ConvertToNone {
			return GenFrag{F: func() string { return e(varName + ".Unwrap()") }, B: []func() string{func() string {
				out := e(g.prefix+varName+" := ") + content1.F() + e("\n")
				out += e(g.prefix + "if " + varName + ".IsErr() {\n")
				out += e(g.prefix + "\treturn MakeOptionNone[" + exprXT.ToNoneType.GoStrType() + "]()\n")
				out += e(g.prefix + "}\n")
				return out
			}}}
		} else {
			return GenFrag{F: func() string { return e(varName + ".Unwrap()") }, B: []func() string{func() string {
				out := e(g.prefix+varName+" := ") + content1.F() + e("\n")
				out += e(g.prefix + "if " + varName + ".IsErr() {\n")
				out += e(g.prefix + "\treturn " + varName + "\n")
				out += e(g.prefix + "}\n")
				return out
			}}}
		}
	} else {
		if exprXT.Native {
			varName, errName := getVar()
			if _, ok := exprXT.W.(types.VoidType); ok {
				return GenFrag{F: func() string { return e(`AglNoop()`) }, B: []func() string{func() string {
					out := e(g.prefix+errName+" := ") + content1.F() + e("\n")
					out += e(g.prefix + "if " + errName + " != nil {\n")
					out += e(g.prefix + "\tpanic(" + errName + ")\n")
					out += e(g.prefix + "}\n")
					return out
				}}}
			} else {
				return GenFrag{F: func() string { return e("AglIdentity(" + varName + ")") }, B: []func() string{func() string {
					out := e(g.prefix+varName+", "+errName+" := ") + content1.F() + e("\n")
					out += e(g.prefix + "if " + errName + " != nil {\n")
					out += e(g.prefix + "\tpanic(" + errName + ")\n")
					out += e(g.prefix + "}\n")
					return out
				}}}
			}
		} else {
			return GenFrag{F: func() string { return content1.F() + e(".Unwrap()") }}
		}
	}
}

func (g *Generator) genCallExprSelectorExpr(expr *ast.CallExpr, x *ast.SelectorExpr) GenFrag {
	e := EmitWith(g, expr)
	oeXT := g.env.GetType(x.X)
	eXT := types.Unwrap(oeXT)
	switch eXTT := eXT.(type) {
	case types.StructType:
		c1 := g.genExpr(x.X)
		genEX := func() string { return c1.F() }
		fnName := x.Sel.Name
		switch fnName {
		case "Sum":
			fnT := g.env.GetType(x.Sel).(types.FuncType)
			recvT := fnT.Recv[0].(types.StructType).TypeParams[0].W.GoStrType()
			retT := fnT.Return.GoStrType()
			return GenFrag{F: func() string { return e("AglSequence"+fnName+"["+recvT+", "+retT+"](") + genEX() + e(")") }}
		}
	case types.ArrayType:
		c1 := g.genExpr(x.X)
		genEX := func() string { return c1.F() }
		genArgFn := func(i int) string { return g.genExpr(expr.Args[i]).F() }
		eltT := types.ReplGenM(eXTT.Elt, g.genMap)
		eltTStr := eltT.GoStr()
		fnName := x.Sel.Name
		switch fnName {
		case "Sum", "Last", "First", "Len", "IsEmpty", "Clone", "Indices", "Sorted":
			return GenFrag{F: func() string { return e("AglVec"+fnName+"(") + genEX() + e(")") }}
		case "Filter", "AllSatisfy", "Contains", "ContainsWhere", "Any", "Map", "FilterMap", "Find", "Joined", "Get", "FirstIndex", "FirstIndexWhere", "FirstWhere", "__ADD":
			return GenFrag{F: func() string { return e("AglVec"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(")") }}
		case "Reduce", "ReduceInto":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(", ") + genArgFn(1) + e(")")
			}}
		case "Insert", "Swap":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + genArgFn(0) + e(", ") + genArgFn(1) + e(")")
			}}
		case "PopIf", "PushFront", "Remove":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + genArgFn(0) + e(")")
			}}
		case "With":
			return GenFrag{F: func() string {
				return e("AglVecWith((*[]"+eltTStr+")(&") + genEX() + e("), ") + genArgFn(0) + e(", ") + genArgFn(1) + e(")")
			}}
		case "Pop", "PopFront", "Clear":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e(")") + e(")")
			}}
		case "Push":
			paramsStr := func() (out string) {
				out += MapJoin(e, expr.Args, func(arg ast.Expr) string { return g.genExpr(arg).F() }, ", ")
				return
			}
			ellipsis := func() (out string) {
				if expr.Ellipsis.IsValid() {
					out = e("...")
				}
				return
			}
			tmpoeXT := oeXT
			if v, ok := tmpoeXT.(types.MutType); ok {
				tmpoeXT = v.W
			}

			// Push into a value of a mut map
			if v, ok := x.X.(*ast.IndexExpr); ok {
				ot := g.env.GetType(v.X)
				t := types.Unwrap(ot)
				if vv, ok := t.(types.MapType); ok {
					if _, ok := vv.V.(types.StarType); !ok {
						varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
						return GenFrag{F: func() string {
							out := e(varName+" := ") + genEX() + e("\n") // temp variable to store the map value
							out += e(g.prefix+"AglVec"+fnName+"(&"+varName+", ") + paramsStr() + ellipsis() + e(")\n")
							out += e(g.prefix) + genEX() + e(" = "+varName) // put the temp value back in the map
							return out
						}}
					}
				}
			}

			if _, ok := tmpoeXT.(types.StarType); ok {
				return GenFrag{F: func() string {
					return e("AglVec"+fnName+"(") + genEX() + e(", ") + paramsStr() + ellipsis() + e(")")
				}}
			} else {
				return GenFrag{F: func() string {
					return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + paramsStr() + ellipsis() + e(")")
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
			r := strings.NewReplacer(
				"[", "_",
				"]", "_",
			)
			var els []string
			for _, k := range slices.Sorted(maps.Keys(m)) {
				els = append(els, fmt.Sprintf("%s_%s", k, r.Replace(m[k].GoStr())))
			}
			if _, ok := m["T"]; !ok {
				recvTName := rawFnT.(types.FuncType).TypeParams[0].(types.GenericType).W.GoStr()
				els = append(els, fmt.Sprintf("%s_%s", "T", r.Replace(recvTName)))
			}
			elsStr := strings.Join(els, "_")
			c1 := g.genExprs(expr.Args)
			return GenFrag{F: func() string {
				out := e("AglVec"+fnName+"_"+elsStr+"(") + genEX()
				if len(expr.Args) > 0 {
					out += e(", ")
				}
				out += c1.F()
				out += e(")")
				return out
			}}
		}
	case types.SetType:
		fnName := x.Sel.Name
		switch fnName {
		case "Union", "FormUnion", "Intersects", "Subtracting", "Subtract", "Intersection", "FormIntersection",
			"SymmetricDifference", "FormSymmetricDifference", "IsSubset", "IsStrictSubset", "IsSuperset", "IsStrictSuperset", "IsDisjoint":
			arg0 := expr.Args[0]
			content2 := func() string {
				switch v := g.env.GetType(arg0).(type) {
				case types.ArrayType:
					return e("AglVec["+v.Elt.GoStrType()+"](") + g.genExpr(arg0).F() + e(")")
				default:
					return g.genExpr(arg0).F()
				}
			}
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string {
				return e("AglSet"+fnName+"(") + c1.F() + e(", ") + content2() + e(")")
			}}
		case "Insert", "Remove", "Contains", "Equals":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglSet"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(")")
			}}
		case "Len", "Min", "Max", "Iter":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglSet"+fnName+"(") + c1.F() + e(")") }}
		}
	case types.RangeType:
		fnName := x.Sel.Name
		switch fnName {
		case "Rev":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglDoubleEndedIteratorRev(") + c1.F() + e(")") }}
		}
	case types.I64Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI64String(") + c1.F() + e(")") }}
		}
	case types.UintType:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglUintString(") + c1.F() + e(")") }}
		}
	case types.StringType, types.UntypedStringType:
		fnName := x.Sel.Name
		switch fnName {
		case "Replace":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			c3 := g.genExpr(expr.Args[1])
			c4 := g.genExpr(expr.Args[2])
			return GenFrag{F: func() string {
				return e("AglString"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(", ") + c3.F() + e(", ") + c4.F() + e(")")
			}}
		case "ReplaceAll":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			c3 := g.genExpr(expr.Args[1])
			return GenFrag{F: func() string {
				return e("AglString"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(", ") + c3.F() + e(")")
			}}
		case "Split", "TrimPrefix", "HasPrefix", "HasSuffix":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglString"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(")")
			}}
		case "TrimSpace", "Lowercased", "Uppercased", "AsBytes", "Lines", "Int", "I8", "I16", "I32", "I64", "Uint", "U8", "U16", "U32", "U64", "F32", "F64", "Len":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglString"+fnName+"(") + c1.F() + e(")") }}
		default:
			extName := "agl1.String." + fnName
			rawFnT := g.env.Get(extName)
			tmp := g.extensionsString[extName]
			if tmp.gen == nil {
				tmp.gen = make(map[string]ExtensionTest)
			}
			tmp.gen[rawFnT.String()] = ExtensionTest{raw: rawFnT}
			g.extensionsString[extName] = tmp
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglString"+fnName+"(") + c1.F() + e(")") }}
		}
	case types.MapType:
		fnName := x.Sel.Name
		switch fnName {
		case "Len":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglIdentity(AglMapLen(") + c1.F() + "))" }}
		case "Get":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglIdentity(AglMapIndex(") + c1.F() + e(", ") + c2.F() + e("))")
			}}
		case "ContainsKey":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglIdentity(AglMapContainsKey(") + c1.F() + e(", ") + c2.F() + e("))")
			}}
		case "Keys", "Values":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglIdentity(AglMap"+fnName+"(") + c1.F() + e("))") }}
		case "Filter":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglIdentity(AglMapFilter(") + c1.F() + e(", ") + c2.F() + e("))")
			}}
		case "Map":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglIdentity(AglMapMap(") + c1.F() + e(", ") + c2.F() + e("))")
			}}
		case "Reduce", "ReduceInto":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			c3 := g.genExpr(expr.Args[1])
			return GenFrag{F: func() string {
				return e("AglMap"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(", ") + c3.F() + e(")")
			}}
		}
	case types.OptionType:
		c1 := g.genExpr(x.X)
		genEX := func() string { return c1.F() }
		genArgFn := func(i int) string { return g.genExpr(expr.Args[i]).F() }
		fnName := x.Sel.Name
		switch fnName {
		case "Map":
			return GenFrag{F: func() string { return e("AglOption"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(")") }}
		}
	default:
		c1 := g.genExprs(expr.Args)
		if v, ok := x.X.(*ast.Ident); ok && v.Name == "agl" && x.Sel.Name == "NewSet" {
			return GenFrag{F: func() string { return e("AglNewSet(") + c1.F() + e(")") }}
		} else if v, ok := x.X.(*ast.Ident); ok && v.Name == "http" && x.Sel.Name == "NewRequest" {
			return GenFrag{F: func() string { return e("AglHttpNewRequest(") + c1.F() + e(")") }}
		}
	}
	return GenFrag{}
}

func (g *Generator) genCallExpr(expr *ast.CallExpr) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	switch x := expr.Fun.(type) {
	case *ast.IndexExpr:
		switch v := x.X.(type) {
		case *ast.SelectorExpr:
			if res := g.genCallExprSelectorExpr(expr, v); res.F != nil {
				return res
			}
		}
	case *ast.SelectorExpr:
		if res := g.genCallExprSelectorExpr(expr, x); res.F != nil {
			return res
		}
	case *ast.Ident:
		if x.Name == "assert" {
			var contents []func() string
			for _, arg := range expr.Args {
				contents = append(contents, g.genExpr(arg).F)
			}
			return GenFrag{F: func() string {
				var out string
				out = e("AglAssert(")
				line := g.fset.Position(expr.Pos()).Line
				msg := fmt.Sprintf(`"assert failed line %d"`, line)
				if len(contents) == 1 {
					contents = append(contents, func() string { return e(msg) })
				} else {
					tmp := contents[1]
					contents[1] = func() string { return e(msg+` + " " + `) + tmp() }
				}
				out += MapJoin(e, contents, func(f func() string) string { return f() }, ", ")
				out += e(")")
				return out
			}}
		} else if x.Name == "panic" {
			return GenFrag{F: func() string { return e("panic(nil)") }}
		} else if x.Name == "panicWith" {
			c1 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string { return e("panic(") + c1.F() + e(")") }}
		} else if x.Name == "Set" {
			c1 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string { return e("AglBuildSet(") + c1.F() + e(")") }}
		}
	}
	content1 := func() string { return "" }
	content2 := func() string { return "" }
	switch v := expr.Fun.(type) {
	case *ast.Ident:
		t1 := g.env.Get(v.Name)
		if t2, ok := t1.(types.TypeType); ok && TryCast[types.CustomType](t2.W) {
			c2 := g.genExprs(expr.Args)
			content1 = func() string { return e(v.Name) }
			content2 = func() string { return c2.F() }
		} else {
			if fnT, ok := t1.(types.FuncType); ok {
				if !InArray(v.Name, []string{"make", "append", "len", "new", "abs", "min", "max"}) && fnT.IsGeneric() {
					oFnT := g.env.Get(v.Name)
					newFnT := g.env.GetType(v)
					fnDecl := g.genFuncDecls[oFnT.String()]
					m := types.FindGen(oFnT, newFnT)
					outFnDecl := func() (out string) {
						g.WithGenMapping(m, func() {
							out = g.decrPrefix(func() string {
								return g.genFuncDecl(fnDecl).F()
							})
						})
						return
					}
					name := g.genExpr(v).FNoEmit(g)
					for _, k := range slices.Sorted(maps.Keys(m)) {
						name += "_" + k + "_" + m[k].GoStr()
					}
					g.genFuncDecls2[name] = outFnDecl
					content1 = func() string { return e(name) }
					content2 = func() string {
						var out string
						g.WithGenMapping(m, func() {
							out += g.genExprs(expr.Args).F()
						})
						return out
					}
				} else if v.Name == "make" {
					c1 := g.genExpr(v)
					c2 := g.genExpr(expr.Args[0])
					bs = append(bs, c1.B...)
					bs = append(bs, c2.B...)
					content1 = func() string { return c1.F() }
					content2 = func() string {
						var out string
						if g.genMap != nil {
							out = e(types.ReplGenM(g.env.GetType(expr.Args[0]), g.genMap).GoStr())
						} else {
							out = c2.F()
						}
						if len(expr.Args) > 1 {
							out += e(", ")
							out += MapJoin(e, expr.Args[1:], func(e ast.Expr) string { return g.genExpr(e).F() }, ", ")
						}
						return out
					}
				} else {
					c1 := g.genExpr(expr.Fun)
					c2 := g.genExprs(expr.Args)
					bs = append(bs, c1.B...)
					bs = append(bs, c2.B...)
					content1 = func() string { return c1.F() }
					content2 = func() string { return c2.F() }
				}
			} else {
				c1 := g.genExpr(expr.Fun)
				c2 := g.genExprs(expr.Args)
				bs = append(bs, c1.B...)
				bs = append(bs, c2.B...)
				content1 = func() string { return c1.F() }
				content2 = func() string { return c2.F() }
			}
		}
	default:
		c1 := g.genExpr(expr.Fun)
		c2 := g.genExprs(expr.Args)
		bs = append(bs, c1.B...)
		bs = append(bs, c2.B...)
		content1 = func() string { return c1.F() }
		content2 = func() string { return c2.F() }
	}
	return GenFrag{F: func() string {
		var out string
		out = content1() + e("(")
		out += content2()
		if expr.Ellipsis != 0 {
			out += e("...")
		}
		out += e(")")
		return out
	}, B: bs}
}

func (g *Generator) genSetType(expr *ast.SetType) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExpr(expr.Key)
	bs = append(bs, c1.B...)
	return GenFrag{F: func() string {
		return e("AglSet[") + c1.F() + e("]")
	}, B: bs}
}

func (g *Generator) genArrayType(expr *ast.ArrayType) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
		var out string
		out += e("[]")
		switch v := expr.Elt.(type) {
		case *ast.TupleExpr:
			out += e(types.ReplGenM(g.env.GetType(v), g.genMap).GoStr())
		default:
			out += g.genExpr(expr.Elt).F()
		}
		return out
	}}
}

func (g *Generator) genKeyValueExpr(expr *ast.KeyValueExpr) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExpr(expr.Key)
	c2 := g.genExpr(expr.Value)
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return GenFrag{F: func() string {
		var out string
		out += c1.F()
		out += e(": ")
		out += c2.F()
		return out
	}, B: bs}
}

func (g *Generator) genResultExpr(expr *ast.ResultExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("Result[") + c1.F() + e("]") }}
}

func (g *Generator) genOptionExpr(expr *ast.OptionExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("Option[") + c1.F() + e("]") }}
}

func (g *Generator) genBasicLit(expr *ast.BasicLit) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string { return e(expr.Value) }}
}

func (g *Generator) genBinaryExpr(expr *ast.BinaryExpr) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExpr(expr.X)
	c2 := g.genExpr(expr.Y)
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return GenFrag{F: func() string {
		content1 := func() string { return c1.F() }
		content2 := func() string { return c2.F() }
		op := expr.Op.String()
		xT := g.env.GetType(expr.X)
		yT := g.env.GetType(expr.Y)
		if xT != nil && yT != nil {
			if op == "in" {
				t := g.env.GetType(expr.Y)
				switch t.(type) {
				case types.ArrayType:
					t = t.(types.ArrayType).Elt
					content2 = func() string { return e("AglVec["+t.GoStrType()+"](") + c2.F() + e(")") }
				case types.MapType:
					kT := t.(types.MapType).K
					vT := t.(types.MapType).V
					content2 = func() string {
						return e("AglMap["+kT.GoStrType()+", "+vT.GoStrType()+"](") + c2.F() + e(")")
					}
				case types.SetType:
				default:
					panic(fmt.Sprintf("%v", to(t)))
				}
				return e("AglIn(") + content1() + e(", ") + content2() + e(")")
			}
			if TryCast[types.SetType](xT) && TryCast[types.SetType](yT) {
				if (op == "==" || op == "!=") && g.env.Get("agl1.Set.Equals") != nil {
					var out string
					if op == "!=" {
						out += e("!")
					}
					out += e("AglSetEquals(") + content1() + e(", ") + content2() + e(")")
					return out
				}
			} else if TryCast[types.ArrayType](xT) && TryCast[types.ArrayType](yT) {
				lhsT := types.Unwrap(xT.(types.ArrayType).Elt)
				rhsT := types.Unwrap(yT.(types.ArrayType).Elt)
				if op == "+" && g.env.Get("agl1.Vec.__ADD") != nil {
					return e("AglVec__ADD(") + content1() + e(", ") + content2() + e(")")
				}
				if TryCast[types.ByteType](lhsT) && TryCast[types.ByteType](rhsT) {
					if op == "==" || op == "!=" {
						var out string
						if op == "!=" {
							out += e("!")
						}
						out += e("AglBytesEqual(") + content1() + e(", ") + content2() + e(")")
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
							out += e("!")
						}
						out += content1() + e(".__EQL(") + content2() + e(")")
						return out
					} else if op == "+" && g.env.Get(lhsName+".__ADD") != nil {
						return content1() + e(".__ADD(") + content2() + e(")")
					} else if op == "-" && g.env.Get(lhsName+".__SUB") != nil {
						return content1() + e(".__SUB(") + content2() + e(")")
					} else if op == "*" && g.env.Get(lhsName+".__MUL") != nil {
						return content1() + e(".__MUL(") + content2() + e(")")
					} else if op == "/" && g.env.Get(lhsName+".__QUO") != nil {
						return content1() + e(".__QUO(") + content2() + e(")")
					} else if op == "%" && g.env.Get(lhsName+".__REM") != nil {
						return content1() + e(".__REM(") + content2() + e(")")
					}
				}
			}
		}
		return content1() + e(" "+expr.Op.String()+" ") + content2()
	}, B: bs}
}

func (g *Generator) genCompositeLit(expr *ast.CompositeLit) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExprs(expr.Elts)
	c2 := GenFrag{F: func() string { return "" }}
	if expr.Type != nil {
		c2 = g.genExpr(expr.Type)
	}
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return GenFrag{F: func() string {
		var out string
		if expr.Type != nil {
			out += c2.F()
		}
		if out == "AglVoid{}" {
			return out
		}
		out += e("{")
		if expr.Type != nil && TryCast[types.SetType](g.env.GetType(expr.Type)) {
			out += MapJoin(e, expr.Elts, func(el ast.Expr) string { return g.genExpr(el).F() + e(": {}") }, ", ")
		} else {
			out += c1.F()
		}
		out += e("}")
		return out
	}, B: bs}
}

func (g *Generator) genTupleExpr(expr *ast.TupleExpr) GenFrag {
	e := EmitWith(g, expr)
	t := g.env.GetType(expr)
	var isType bool
	if v, ok := t.(types.TypeType); ok {
		isType = true
		t = v.W
	}
	if v, ok := t.(types.TypeType); ok { // TODO fix double wrapped
		isType = true
		t = v.W
	}
	if g.asType {
		isType = true
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
	return GenFrag{F: func() string {
		if isType {
			return e(structName)
		}
		out := e(structName) + e("{")
		for i, x := range expr.Values {
			out += e(fmt.Sprintf("Arg%d: ", i))
			out += g.genExpr(x).F()
			if i < len(expr.Values)-1 {
				out += e(", ")
			}
		}
		out += e("}")
		return out
	}}
}

func (g *Generator) genExprs(e []ast.Expr) GenFrag {
	arr1 := make([]func() string, len(e))
	arr2 := make([]ast.Expr, len(e))
	var bs []func() string
	for i, x := range e {
		tmp := g.genExpr(x)
		arr1[i] = tmp.F
		arr2[i] = x
		bs = append(bs, tmp.B...)
	}
	return GenFrag{F: func() string {
		var out string
		for i, el := range arr1 {
			out += el()
			if i < len(e)-1 {
				out += g.Emit(", ", WithNode(arr2[i]))
			}
		}
		return out
	}, B: bs}
}

func (g *Generator) genStmts(s []ast.Stmt) GenFrag {
	var bs []func() string
	var stmts []GenFrag
	for _, stmt := range s {
		tmp := g.genStmt(stmt)
		stmts = append(stmts, tmp)
		bs = append(bs, tmp.B...)
	}
	return GenFrag{F: func() string {
		var out string
		for _, stmt := range stmts {
			for _, b := range stmt.B {
				out += b()
			}
			out += stmt.F()
		}
		return out
	}, B: bs}
}

func (g *Generator) genBlockStmt(stmt *ast.BlockStmt) GenFrag {
	var bs []func() string
	c1 := g.genStmts(stmt.List)
	bs = append(bs, c1.B...)
	return GenFrag{F: func() string {
		return c1.F()
	}, B: bs}
}

func (g *Generator) genSpecs(specs []ast.Spec, tok token.Token) GenFrag {
	return GenFrag{F: func() string {
		var tmp string
		for _, spec := range specs {
			tmp += g.genSpec(spec, tok).F()
		}
		return tmp
	}}
}

func (g *Generator) genSpec(s ast.Spec, tok token.Token) GenFrag {
	return GenFrag{F: func() string {
		var out string
		switch spec := s.(type) {
		case *ast.ValueSpec:
			e := EmitWith(g, spec)
			var namesArr []string
			for _, name := range spec.Names {
				namesArr = append(namesArr, name.Name)
			}
			out += e(g.prefix + tok.String() + " ")
			out += e(strings.Join(namesArr, ", "))
			if spec.Type != nil {
				tStr := func() (out string) {
					g.withAsType(func() { out += g.genExpr(spec.Type).F() })
					return
				}
				out += e(" ") + tStr()
			}
			if spec.Values != nil {
				out += e(" = ") + g.genExprs(spec.Values).F()
			}
			out += e("\n")
			return out
		case *ast.TypeSpec:
			e := EmitWith(g, spec)
			if v, ok := spec.Type.(*ast.EnumType); ok {
				out += e(g.prefix) + e(g.genEnumType(spec.Name.Name, v)) + e("\n")
			} else {
				out += e(g.prefix + "type " + spec.Name.Name)
				if typeParams := spec.TypeParams; typeParams != nil {
					if len(typeParams.List) > 0 {
						out += e("[")
					}
					out += MapJoin(e, typeParams.List, func(field *ast.Field) (out string) {
						out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return g.genIdent(n.Ident).F() }, ", ")
						if len(field.Names) > 0 {
							out += e(" ")
						}
						out += g.genExpr(field.Type).F()
						return
					}, ", ")
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

func (g *Generator) genDecl(d ast.Decl) GenFrag {
	switch decl := d.(type) {
	case *ast.GenDecl:
		return g.genGenDecl(decl)
	case *ast.FuncDecl:
		return g.genFuncDecl(decl)
	default:
		panic(fmt.Sprintf("%v", to(d)))
	}
}

func (g *Generator) genGenDecl(decl *ast.GenDecl) GenFrag {
	return g.genSpecs(decl.Specs, decl.Tok)
}

func (g *Generator) genDeclStmt(stmt *ast.DeclStmt) GenFrag {
	return g.genDecl(stmt.Decl)
}

func (g *Generator) genIncDecStmt(stmt *ast.IncDecStmt) GenFrag {
	c1 := g.genExpr(stmt.X)
	return GenFrag{F: func() string {
		var out string
		e := EmitWith(g, stmt)
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
		out += c1.F() + e(op)
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}}
}

func (g *Generator) genForStmt(stmt *ast.ForStmt) GenFrag {
	e := EmitWith(g, stmt)
	cc1 := g.genStmt(stmt.Body)
	body := func() string { return g.incrPrefix(func() string { return cc1.F() }) }
	if stmt.Init == nil && stmt.Post == nil && stmt.Cond != nil {
		if v, ok := stmt.Cond.(*ast.BinaryExpr); ok {
			if v.Op == token.IN {
				xT := g.env.GetType(v.X)
				yT := g.env.GetType(v.Y)
				c1 := g.genExpr(v.Y)
				c2 := g.genExpr(v.X)
				return GenFrag{F: func() string {
					var out string
					if tup, ok := xT.(types.TupleType); ok && !TryCast[types.MapType](yT) {
						varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
						out += e(g.prefix+"for _, "+varName+" := range ") + c1.F() + e(" {\n")
						out += e(g.prefix + "\t")
						switch vv := v.X.(type) {
						case *ast.TupleExpr:
							for i := range tup.Elts {
								out += g.genExpr(vv.Values[i]).F()
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
						case *ast.Ident:
							out += c2.F() + e(" := "+varName+"\n")
						}
					} else {
						switch types.Unwrap(yT).(type) {
						case types.ArrayType:
							out += e(g.prefix+"for _, ") + c2.F() + e(" := range ") + c1.F() + e(" {\n")
						case types.SetType:
							out += e(g.prefix+"for ") + c2.F() + e(" := range (") + c1.F() + e(").Iter() {\n")
						case types.MapType:
							xTup := v.X.(*ast.TupleExpr)
							key := func() string { return g.genExpr(xTup.Values[0]).F() }
							val := func() string { return g.genExpr(xTup.Values[1]).F() }
							y := func() string { return c1.F() }
							out += e(g.prefix+"for ") + key() + e(", ") + val() + e(" := range ") + y() + e(" {\n")
						case types.RangeType:
							out += e(g.prefix + "for ")
							c2V := c2.F()
							op := utils.Ternary(c2V == "_", "=", ":=")
							out += c2V
							out += e(" "+op+" range ") + c1.F() + e(".Iter()") + e(" {\n")
						default:
							panic(fmt.Sprintf("%v", to(v.X)))
						}
					}
					out += body()
					out += e(g.prefix + "}\n")
					return out
				}}
			}
		}
	}
	c1 := GenFrag{F: func() string { return "" }}
	if stmt.Cond != nil {
		c1 = g.genExpr(stmt.Cond)
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
			cond = func() string { return c1.F() }
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
		out += MapJoin(e, els, func(el func() string) string { return el() }, "; ")
		if out != "" {
			out += e(" ")
		}
		return out
	}
	return GenFrag{F: func() string {
		var out string
		out += e(g.prefix+"for ") + tmp() + e("{\n")
		out += body()
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genRangeStmt(stmt *ast.RangeStmt) GenFrag {
	c1 := g.genExpr(stmt.X)
	c2 := GenFrag{F: func() string { return "" }}
	c3 := GenFrag{F: func() string { return "" }}
	if stmt.Key != nil {
		c2 = g.genExpr(stmt.Key)
	}
	if stmt.Value != nil {
		c3 = g.genExpr(stmt.Value)
	}
	c4 := g.genStmt(stmt.Body)
	return GenFrag{F: func() string {
		var out string
		e := EmitWith(g, stmt)
		content3 := func() string {
			isCompLit := TryCast[*ast.CompositeLit](stmt.X)
			var out string
			if isCompLit {
				out += e("(")
			}
			out += c1.F()
			if isCompLit {
				out += e(")")
			}
			return out
		}
		op := stmt.Tok
		if stmt.Key == nil && stmt.Value == nil {
			out += e(g.prefix+"for range ") + content3() + e(" {\n")
		} else if stmt.Value == nil {
			out += e(g.prefix+"for ") + c2.F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		} else {
			out += e(g.prefix+"for ") + c2.F() + e(", ") + c3.F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		}
		out += g.incrPrefix(func() string {
			return c4.F()
		})
		out += e(g.prefix + "}\n")
		return out
	}}
}

type GenFrag struct {
	F func() string
	B []func() string // Store "functions that generated code" that we want to generate before the current statement
}

func (s GenFrag) FNoEmit(g *Generator) (out string) {
	g.WithoutEmit(func() {
		out = s.F()
	})
	return
}

func (g *Generator) genReturnStmt(stmt *ast.ReturnStmt) GenFrag {
	e := EmitWith(g, stmt)
	var bs []func() string
	var tmp GenFrag
	if stmt.Result != nil {
		tmp = g.genExpr(stmt.Result)
		bs = append(bs, tmp.B...)
	}
	return GenFrag{F: func() (out string) {
		out += e(g.prefix + "return")
		if stmt.Result != nil {
			out += e(" ") + tmp.F()
		}
		return out + e("\n")
	}, B: bs}
}

func (g *Generator) genExprStmt(stmt *ast.ExprStmt) GenFrag {
	e := EmitWith(g, stmt)
	tmp := g.genExpr(stmt.X)
	bs := tmp.B
	return GenFrag{F: func() string {
		var out string
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += tmp.F()
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) genAssignStmt(stmt *ast.AssignStmt) GenFrag {
	e := EmitWith(g, stmt)
	lhs := func() string { return "" }
	var after string
	rhsT := g.env.GetType(stmt.Rhs[0])
	if v, ok := rhsT.(types.CustomType); ok {
		rhsT = v.W
	}
	if len(stmt.Rhs) == 1 && TryCast[types.EnumType](rhsT) {
		enumT := rhsT.(types.EnumType)
		if len(stmt.Lhs) == 1 {
			c1 := g.genExprs(stmt.Lhs)
			lhs = func() string { return c1.F() }
		} else {
			varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
			lhs = func() string { return e(varName) }
			var names []string
			var exprs []string
			for i, x := range stmt.Lhs {
				names = append(names, x.(*ast.Ident).Name)
				exprs = append(exprs, fmt.Sprintf("%s.%s_%d", varName, enumT.SubTyp, i))
			}
			after += strings.Join(names, ", ") + " := " + strings.Join(exprs, ", ")
		}
	} else if len(stmt.Rhs) == 1 && TryCast[types.TupleType](rhsT) {
		if len(stmt.Lhs) == 1 {
			c1 := g.genExprs(stmt.Lhs)
			lhs = func() string { return c1.F() }
		} else {
			if v, ok := rhsT.(types.TupleType); ok && v.KeepRaw {
				c1 := g.genExprs(stmt.Lhs)
				lhs = func() string { return c1.F() }
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
				after += fmt.Sprintf("%s := %s", strings.Join(names, ", "), strings.Join(exprs, ", "))
			}
		}
	} else if len(stmt.Lhs) == 1 && TryCast[*ast.IndexExpr](stmt.Lhs[0]) && TryCast[*ast.MapType](stmt.Lhs[0].(*ast.IndexExpr).X) {
		c1 := g.genExprs(stmt.Lhs)
		lhs = func() string { return c1.F() }
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
			c1 := g.genExpr(v.X)
			c2 := g.genExpr(v.Index)
			lhs = func() string { return e("(*") + c1.F() + e(")[") + c2.F() + e("]") }
		} else {
			c1 := g.genExprs(stmt.Lhs)
			lhs = func() string { return c1.F() }
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
						c1 := g.genExprs(stmt.Rhs)
						content2 = GenFrag{F: func() string { return e("AglWrapNative1(") + c1.F() + e(")") }, B: c1.B}
					} else {
						c1 := g.genExprs(stmt.Rhs)
						content2 = GenFrag{F: func() string { return e("AglWrapNative2(") + c1.F() + e(")") }, B: c1.B}
					}
				}
			}
		}
	}
	var bs []func() string
	bs = append(bs, content2.B...)
	return GenFrag{F: func() string {
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
			if !g.inlineStmt {
				after = g.prefix + after
			}
			out += e(after)
		}
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) wrapIfNative(e Emitter, x ast.Expr, v func() string) string {
	switch exprXT := g.env.GetType(x).(type) {
	case types.ResultType:
		if _, ok := exprXT.W.(types.VoidType); ok && exprXT.Native {
			return e.Emit("AglWrapNative1(") + v() + e.Emit(")")
		} else if exprXT.Native {
			return e.Emit("AglWrapNative2(") + v() + e.Emit(")")
		}
	case types.OptionType:
		if exprXT.Native {
			return e.Emit("AglWrapNativeOpt(") + v() + e.Emit(")")
		}
	}
	return v()
}

func (g *Generator) genIfLetStmt(stmt *ast.IfLetExpr) GenFrag {
	e := EmitWith(g, stmt)
	ass := stmt.Ass
	lhs0, rhs0 := ass.Lhs[0], ass.Rhs[0]
	c1 := g.genExpr(lhs0)
	c2 := g.genExpr(rhs0)
	c3 := g.genStmt(stmt.Body)
	c4 := GenFrag{F: func() string { return "" }}
	if stmt.Else != nil {
		c4 = g.genStmt(stmt.Else)
	}
	return GenFrag{F: func() string {
		var out string
		gPrefix := g.prefix
		lhs := func() string { return c1.F() }
		rhs := func() string { return g.wrapIfNative(e, rhs0, c2.F) }
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
		if _, ok := ass.Rhs[0].(*ast.TypeAssertExpr); ok {
			out += e("if ") + lhs() + e(", ok := ") + rhs() + e("; ok {\n")
			g.inlineStmt = false
		} else {
			out += e("if "+varName+" := ") + rhs() + e("; "+cond+" {\n")
			out += e(gPrefix+"\t") + lhs() + e(" := "+varName+"."+unwrapFn+"()\n")
			g.inlineStmt = false
		}
		if g.allowUnused {
			out += e(gPrefix+"\tAglNoop(") + lhs() + e(")\n")
		}
		out += g.incrPrefix(func() string { return c3.F() })
		if stmt.Else != nil {
			if v, ok := stmt.Else.(*ast.ExprStmt); ok {
				switch v.X.(type) {
				case *ast.IfExpr, *ast.IfLetExpr:
					out += e(gPrefix + "} else ")
					g.WithInlineStmt(func() {
						out += c4.F()
					})
				default:
					out += e(gPrefix + "} else {\n")
					out += g.incrPrefix(func() string {
						return c4.F()
					})
					out += e(gPrefix + "}")
				}
			} else {
				out += e(gPrefix + "} else {\n")
				out += g.incrPrefix(func() string {
					return c4.F()
				})
				out += e(gPrefix + "}")
			}
		} else {
			out += e(gPrefix + "}")
		}
		return out
	}}
}

func (g *Generator) genGuardLetStmt(stmt *ast.GuardLetStmt) GenFrag {
	e := EmitWith(g, stmt)
	ass := stmt.Ass
	lhs0, rhs0 := ass.Lhs[0], ass.Rhs[0]
	c1 := g.genExpr(lhs0)
	c2 := g.genExpr(rhs0)
	c3 := g.genStmt(stmt.Body)
	return GenFrag{F: func() string {
		var out string
		gPrefix := g.prefix
		lhs := func() string { return c1.F() }
		rhs := func() string { return g.wrapIfNative(e, rhs0, c2.F) }
		body := func() string { return g.incrPrefix(func() string { return c3.F() }) }
		varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
		var cond string
		unwrapFn := "Unwrap"
		switch stmt.Op {
		case token.SOME:
			cond = fmt.Sprintf("%s.IsNone()", varName)
		case token.OK:
			cond = fmt.Sprintf("%s.IsErr()", varName)
		case token.ERR:
			cond = fmt.Sprintf("%s.IsOk()", varName)
			unwrapFn = "Err"
		default:
			panic("")
		}
		if _, ok := rhs0.(*ast.TypeAssertExpr); ok {
			out += e(gPrefix) + lhs() + e(", "+varName+" := ") + rhs() + e("\n")
			out += e(gPrefix + "if !" + varName + " {\n")
			out += body()
			out += e(gPrefix + "}\n")
		} else {
			out += e(gPrefix+varName+" := ") + rhs() + e("\n")
			out += e(gPrefix + "if " + cond + " {\n")
			out += body()
			out += e(gPrefix + "}\n")
			out += e(gPrefix) + lhs() + e(" := "+varName+"."+unwrapFn+"()\n")
		}
		if g.allowUnused {
			out += e(gPrefix+"AglNoop(") + lhs() + e(")\n")
		}
		return out
	}}
}

func (g *Generator) genIfStmt(stmt *ast.IfExpr) GenFrag {
	e := EmitWith(g, stmt)
	var bs []func() string
	ifT := g.env.GetType(stmt)
	var varName string
	hasTyp := !TryCast[types.VoidType](ifT)
	var isFirst bool
	if hasTyp {
		if g.ifVarName == "" {
			varName = fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
			g.ifVarName = varName
			isFirst = true
		} else {
			varName = g.ifVarName
		}
	}
	cond := g.genExpr(stmt.Cond)
	c1 := GenFrag{F: func() string { return "" }}
	c3 := GenFrag{F: func() string { return "" }}
	if stmt.Init != nil {
		c3 = g.genStmt(stmt.Init)
	}
	if hasTyp {
		last := Must(Last(stmt.Body.List))
		stmt.Body.List[len(stmt.Body.List)-1] = &ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: varName}}, Rhs: []ast.Expr{last.(*ast.ExprStmt).X}, Tok: token.ASSIGN}
	}
	g.ifVarName = ""
	c2 := g.genStmt(stmt.Body)
	g.ifVarName = varName
	if stmt.Else != nil {
		if hasTyp {
			switch v := stmt.Else.(type) {
			case *ast.BlockStmt:
				last := Must(Last(v.List))
				v.List[len(v.List)-1] = &ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: varName}}, Rhs: []ast.Expr{last.(*ast.ExprStmt).X}, Tok: token.ASSIGN}
			}
		}
		g.WithIfVarName(varName, func() {
			switch stmt.Else.(type) {
			case *ast.BlockStmt:
				g.ifVarName = ""
				c1 = g.genStmt(stmt.Else)
				g.ifVarName = varName
			default:
				c1 = g.genStmt(stmt.Else)
			}
		})
	}
	bs = append(bs, cond.B...)
	bs = append(bs, c3.B...)
	tmp := func() string {
		var out string
		gPrefix := g.prefix
		if hasTyp && isFirst {
			out += e(gPrefix + "var " + varName + " " + ifT.GoStrType() + "\n")
		}
		if hasTyp && isFirst {
			out += e(gPrefix)
		}
		out += e("if ")
		if stmt.Init != nil {
			g.WithInlineStmt(func() {
				out += c3.F() + e("; ")
			})
		}
		out += cond.F() + e(" {\n")
		g.inlineStmt = false
		out += g.incrPrefix(func() string {
			return c2.F()
		})
		if stmt.Else != nil {
			switch stmt.Else.(type) {
			case *ast.ExprStmt:
				switch stmt.Else.(*ast.ExprStmt).X.(type) {
				case *ast.IfExpr, *ast.IfLetExpr:
					out += e(gPrefix + "} else ")
					g.WithInlineStmt(func() {
						out += c1.F()
					})
				default:
					out += e(gPrefix + "} else {\n")
					out += g.incrPrefix(func() string {
						return c1.F()
					})
					out += e(gPrefix + "}")
				}
			default:
				out += e(gPrefix + "} else {\n")
				out += g.incrPrefix(func() string {
					return c1.F()
				})
				out += e(gPrefix + "}")
			}
		} else {
			out += e(gPrefix + "}")
		}
		if hasTyp && isFirst {
			out += e("\n")
		}
		return out
	}
	if hasTyp && isFirst {
		bs = append(bs, tmp)
		return GenFrag{F: func() string {
			return e("AglIdentity(" + varName + ")")
		}, B: bs}
	} else {
		return GenFrag{F: tmp, B: bs}
	}
}

func (g *Generator) genGuardStmt(stmt *ast.GuardStmt) GenFrag {
	e := EmitWith(g, stmt)
	c1 := g.genExpr(stmt.Cond)
	c2 := g.genStmt(stmt.Body)
	return GenFrag{F: func() string {
		var out string
		cond := func() string { return c1.F() }
		gPrefix := g.prefix
		out += e(gPrefix+"if !(") + cond() + e(") {\n")
		out += g.incrPrefix(func() string {
			return c2.F()
		})
		out += e(gPrefix + "}\n")
		return out
	}}
}

func (g *Generator) genDecls(f *ast.File) GenFrag {
	var bs []func() string
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
	return GenFrag{F: func() string {
		var out string
		for _, decl := range decls {
			out += decl()
		}
		return out
	}, B: bs}
}

func (g *Generator) genFuncDecl(decl *ast.FuncDecl) GenFrag {
	e := EmitWith(g, decl)
	g.returnType = g.env.GetType(decl).(types.FuncType).Return
	var bs []func() string
	recv := func() string { return "" }
	typeParamsFn := func() string { return "" }
	var name, paramsStr, resultStr string
	if decl.Recv != nil {
		if len(decl.Recv.List) >= 1 {
			if tmp1, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if tmp2, ok := tmp1.X.(*ast.SelectorExpr); ok {
					if tmp2.Sel.Name == "Vec" {
						fnName := fmt.Sprintf("agl1.Vec.%s", decl.Name.Name)
						tmp := g.extensions[fnName]
						tmp.decl = decl
						g.extensions[fnName] = tmp
						return GenFrag{F: func() string { return "" }}
					}
				}
			} else if tmp2, ok := decl.Recv.List[0].Type.(*ast.SelectorExpr); ok {
				if tmp2.Sel.Name == "String" {
					fnName := fmt.Sprintf("agl1.String.%s", decl.Name.Name)
					tmp := g.extensionsString[fnName]
					tmp.decl = decl
					g.extensionsString[fnName] = tmp
					return GenFrag{F: func() string { return "" }}
				}
			}
		}
		recv = func() string {
			var out string
			if decl.Recv != nil {
				out += e(" (")
				out += g.joinList(decl.Recv)
				out += e(")")
			}
			return out
		}
	}
	fnT := g.env.GetType(decl)
	if g.genMap == nil && fnT.(types.FuncType).IsGeneric() {
		g.genFuncDecls[fnT.String()] = decl
		return GenFrag{F: func() string { return "" }}
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
		typeParamsFn = func() string {
			out := g.joinList(decl.Type.TypeParams)
			out = utils.WrapIf(out, "[", "]")
			return out
		}
	}
	if params := decl.Type.Params; params != nil {
		var fieldsItems []string
		for _, field := range params.List {
			var content string
			if v, ok := g.env.GetType(field.Type).(types.TypeType); ok {
				content = types.ReplGenM(v.W, g.genMap).GoStrType()
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
		typeParamsFn = func() string { return "" }
		for _, k := range slices.Sorted(maps.Keys(g.genMap)) {
			v := g.genMap[k]
			name += fmt.Sprintf("_%v_%v", k, v.GoStr())
		}
	}
	c1 := g.genStmt(decl.Body)
	bs = append(bs, c1.B...)
	return GenFrag{F: func() string {
		var out string
		out += e("func")
		out += recv()
		out += e(fmt.Sprintf("%s%s(%s)%s {\n", name, typeParamsFn(), paramsStr, resultStr))
		if decl.Body != nil {
			out += g.incrPrefix(func() string {
				return c1.F()
			})
		}
		out += e("}\n")
		return out
	}, B: bs}
}

func (g *Generator) joinList(l *ast.FieldList) (out string) {
	if l == nil {
		return ""
	}
	e := EmitWith(g, l)
	out += MapJoin(e, l.List, func(field *ast.Field) (out string) {
		out += MapJoin(e, field.Names, func(n *ast.LabelledIdent) string { return g.genIdent(n.Ident).F() }, ", ")
		if out != "" {
			out += e(" ")
		}
		out += g.genExpr(field.Type).F()
		return
	}, ", ")
	return
}

func GenCore(packageName string) string {
	by := []byte(GeneratedFilePrefix)
	by = append(by, '\n')
	by = append(by, Must(ContentFs.ReadFile(filepath.Join("core", "core.go")))...)
	by = bytes.Replace(by, []byte("package main"), []byte(fmt.Sprintf("package %s", packageName)), 1)
	return string(by)
}
