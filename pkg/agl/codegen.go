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
	fset                       *token.FileSet
	env                        *Env
	a, b                       *ast.File
	prefix                     string
	genFuncDecls2              map[string]func() string
	tupleStructs               map[string]string
	genFuncDecls               map[string]*ast.FuncDecl
	genTypeDecls               map[string]*ast.TypeSpec
	genTypeDecls2              map[string]func() string
	varCounter                 atomic.Int64
	returnType                 types.Type
	extensions                 map[string]Extension
	genMap                     map[string]types.Type
	allowUnused                bool
	releaseMode                bool
	inlineStmt                 bool
	fragments                  Frags
	emitEnabled                bool
	asType                     bool
	ifVarName                  string
	imports                    map[string]*ast.ImportSpec
	shadowedVars               map[string]string // maps original name to shadowed name (e.g., "a" -> "a_")
	shadowingExclusionNode     ast.Node          // AST node to use old shadowing level for (RHS of mut x := x)
	shadowingExclusionOldValue string            // the old shadow value to use for the exclusion node
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
	prev := g.emitEnabled
	g.emitEnabled = false
	clb()
	g.emitEnabled = prev
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

type ExtensionTest struct {
	raw      types.Type
	concrete types.Type
}

type GeneratorConf struct {
	AllowUnused bool
	ReleaseMode bool
}

type GeneratorOption func(*GeneratorConf)

func AllowUnused() GeneratorOption {
	return func(c *GeneratorConf) {
		c.AllowUnused = true
	}
}

func ReleaseMode() GeneratorOption {
	return func(c *GeneratorConf) {
		c.ReleaseMode = true
	}
}

func NewGenerator(env *Env, a, b *ast.File, imports map[string]*ast.ImportSpec, fset *token.FileSet, opts ...GeneratorOption) *Generator {
	conf := &GeneratorConf{}
	for _, opt := range opts {
		opt(conf)
	}
	genFns := make(map[string]*ast.FuncDecl)
	return &Generator{
		fset:          fset,
		env:           env,
		a:             a,
		b:             b,
		extensions:    make(map[string]Extension),
		tupleStructs:  make(map[string]string),
		genFuncDecls2: make(map[string]func() string),
		genFuncDecls:  genFns,
		genTypeDecls:  make(map[string]*ast.TypeSpec),
		genTypeDecls2: make(map[string]func() string),
		allowUnused:   conf.AllowUnused,
		releaseMode:   conf.ReleaseMode,
		emitEnabled:   true,
		imports:       imports,
		shadowedVars:  make(map[string]string),
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

func (g *Generator) genExtension(ext Extension) (out string) {
	e := EmitWith(g, ext.decl)
	for _, key := range slices.Sorted(maps.Keys(ext.gen)) {
		ge := ext.gen[key]
		decl := ext.decl
		if decl == nil {
			return ""
		}
		typeParamsStr := emptyContent
		paramsStr := emptyContent
		resultStr := emptyContent
		bodyStr := emptyContent
		var name string
		if decl.Name != nil {
			name = decl.Name.Name
		}
		for _, a := range decl.Type.Params.List {
			for _, n := range a.Names {
				if n.Label != nil {
					name += fmt.Sprintf("_%s", n.Label.Name)
				}
			}
		}
		assert(len(decl.Recv.List) == 1)
		recv := decl.Recv.List[0]
		var recvName string
		if len(recv.Names) >= 1 {
			recvName = recv.Names[0].Name
		}
		var recvT string
		var extType string
		switch v := recv.Type.(type) {
		case *ast.IndexExpr:
			recvT = v.Index.(*ast.Ident).Name
			extType = "Vec"
		case *ast.SelectorExpr:
			recvT = v.Sel.Name
			extType = "String"
		default:
			panic(fmt.Sprintf("%v %v", recv.Type, to(recv.Type)))
		}
		m := types.FindGen(ge.raw, ge.concrete)
		var recvTName string
		if el, ok := m[recvT]; ok {
			recvTName = el.GoStr()
		} else {
			switch v := ge.concrete.(types.FuncType).Recv[0].(type) {
			case types.ArrayType:
				recvTName = v.Elt.GoStrType()
			case types.StringType:
				recvTName = v.GoStrType()
			default:
				panic(fmt.Sprintf("%v", to(v)))
			}
		}

		r := strings.NewReplacer(
			"[", "_",
			"]", "_",
			"*", "_",
		)

		var elts []string
		if extType == "Vec" {
			for _, k := range slices.Sorted(maps.Keys(m)) {
				elts = append(elts, fmt.Sprintf("%s_%s", k, r.Replace(m[k].GoStr())))
			}
			if _, ok := m["T"]; !ok {
				elts = append(elts, fmt.Sprintf("%s_%s", "T", r.Replace(recvTName)))
			}
		}

		firstArg := ast.Field{Names: []*ast.LabelledIdent{{Ident: &ast.Ident{Name: recvName}, Label: nil}}}
		if extType == "Vec" {
			firstArg.Type = &ast.ArrayType{Elt: &ast.Ident{Name: recvTName}}
		} else {
			firstArg.Type = &ast.Ident{Name: recvTName}
		}
		var paramsClone []ast.Field
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				paramsClone = append(paramsClone, *param)
			}
		}
		paramsClone = append([]ast.Field{firstArg}, paramsClone...)
		g.WithGenMapping(m, func() {
			if params := paramsClone; params != nil {
				paramsStr = func() (out string) {
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
					return g.incrPrefix(g.genStmt(decl.Body).F)
				}
			}
			var eltsStr string
			if len(elts) > 0 {
				eltsStr = "_" + strings.Join(elts, "_")
			}
			out += e("func Agl"+extType+name+eltsStr) + typeParamsStr() + e("(") + paramsStr() + e(")") + resultStr() + e(" {\n")
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

func (g *Generator) scanForEnums() {
	// Scan both AST files for enum declarations
	needsFmt := false
	scanDecls := func(decls []ast.Decl) {
		for _, decl := range decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if enumType, ok := typeSpec.Type.(*ast.EnumType); ok {
							// Check if any enum field has parameters
							for _, field := range enumType.Values.List {
								if field.Params != nil {
									needsFmt = true
									return
								}
							}
						}
					}
				}
			}
		}
	}
	if g.a != nil {
		scanDecls(g.a.Decls)
	}
	if g.b != nil && !needsFmt {
		scanDecls(g.b.Decls)
	}

	// Add fmt import if needed and not already present
	if needsFmt {
		// Check if fmt is already imported in source file
		hasFmt := false
		if g.a != nil {
			for _, imp := range g.a.Imports {
				// Check for both "fmt" and "agl1/fmt" imports
				if imp.Path.Value == "\"fmt\"" || imp.Path.Value == "\"agl1/fmt\"" {
					hasFmt = true
					break
				}
			}
		}
		if !hasFmt {
			if g.imports == nil {
				g.imports = make(map[string]*ast.ImportSpec)
			}
			g.imports["\"fmt\""] = &ast.ImportSpec{
				Path: &ast.BasicLit{Value: "\"fmt\""},
			}
		}
	}
}

func (g *Generator) Generate2() (out1, out2 string) {
	out1 = g.Generate()
	for _, f := range g.fragments {
		out2 += f.s
	}
	return
}

// typeToGoStr converts a type to its Go string representation.
// For user-defined structs with concrete type parameters, it uses the monomorphized name.
func (g *Generator) typeToGoStr(t types.Type) string {
	// Get the default string representation
	defaultStr := t.GoStr()

	// Check if the base type (unwrapped) is a user-defined struct
	unwrapped := types.Unwrap(t)
	if structType, ok := unwrapped.(types.StructType); ok {
		// Check if this is a user-defined struct (in genTypeDecls)
		if structType.IsGeneric() {
			// Try to find this struct in genTypeDecls
			// The key format is "StructName[any]" for generic structs
			baseName := structType.String1()
			// Check if any key in genTypeDecls matches the base name
			for key := range g.genTypeDecls {
				if structType.String1() == "" {
					continue
				}
				// Check if the key starts with the struct base name
				if len(key) > len(baseName) && key[:len(baseName)] == baseName {
					// This is a user-defined struct - use monomorphized name
					monomorphizedName := structType.GoStrMonomorphized()
					// If the original type was a pointer, add the pointer prefix
					if len(defaultStr) > 0 && defaultStr[0] == '*' {
						return "*" + monomorphizedName
					}
					return monomorphizedName
				}
			}
		}
	}

	// Default behavior
	return defaultStr
}

func (g *Generator) scanForGenericStructs() {
	// Scan both AST files for generic struct declarations and store them in genTypeDecls
	scanDecls := func(decls []ast.Decl) {
		for _, decl := range decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				if genDecl.Tok == token.TYPE {
					for _, spec := range genDecl.Specs {
						if typeSpec, ok := spec.(*ast.TypeSpec); ok {
							hasTypeParams := typeSpec.TypeParams != nil && len(typeSpec.TypeParams.List) > 0
							_, isStructType := typeSpec.Type.(*ast.StructType)

							if hasTypeParams && isStructType {
								// Get the struct type from environment
								typeT := g.env.Get(typeSpec.Name.Name)
								if typeT != nil {
									if tt, ok := typeT.(types.TypeType); ok {
										typeT = tt.W
									}
									if structT, ok := types.Unwrap(typeT).(types.StructType); ok {
										key := structT.String()
										g.genTypeDecls[key] = typeSpec
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if g.a != nil {
		scanDecls(g.a.Decls)
	}
	if g.b != nil {
		scanDecls(g.b.Decls)
	}
}

func (g *Generator) Generate() (out string) {
	// Scan for enums first to add necessary imports
	g.scanForEnums()

	// Scan for generic structs to populate genTypeDecls before code generation
	g.scanForGenericStructs()

	// Pre-generate all declarations to populate g.imports before collecting imports
	out4 := g.genDecls(g.b)
	out5 := g.genDecls(g.a)

	// Execute the declarations WITHOUT emitting to populate g.imports (especially for tuple structs that need fmt)
	// This needs to happen before collecting imports
	g.WithoutEmit(func() {
		_ = out4.F() + out5.F()
	})

	// Now generate monomorphized types and functions WITHOUT emitting (to discover all imports first)
	// Keep generating until both maps are empty
	// Functions may add type declarations, so we need to iterate
	// Save the generation functions for later regeneration
	var savedGenFuncDecls2 []struct {
		key string
		fn  func() string
	}
	var savedGenTypeDecls2 []struct {
		key string
		fn  func() string
	}
	g.WithoutEmit(func() {
		for len(g.genFuncDecls2) > 0 || len(g.genTypeDecls2) > 0 {
			// First, generate ALL type declarations (generating functions may add more types)
			for len(g.genTypeDecls2) > 0 {
				sorted := slices.Sorted(maps.Keys(g.genTypeDecls2))
				key := sorted[0]
				fn := g.genTypeDecls2[key]
				savedGenTypeDecls2 = append(savedGenTypeDecls2, struct {
					key string
					fn  func() string
				}{key, fn})
				_ = fn()
				delete(g.genTypeDecls2, key)
			}
			// Then generate functions (they may add more type declarations, which will be handled in next iteration)
			if len(g.genFuncDecls2) > 0 {
				sorted := slices.Sorted(maps.Keys(g.genFuncDecls2))
				key := sorted[0]
				fn := g.genFuncDecls2[key]
				savedGenFuncDecls2 = append(savedGenFuncDecls2, struct {
					key string
					fn  func() string
				}{key, fn})
				_ = fn()
				delete(g.genFuncDecls2, key)
			}
		}
	})

	// Generate extensions WITHOUT emitting (they may create tuple structs that need fmt import)
	// Save them for later regeneration
	var savedExtensions []struct {
		key string
		ext Extension
	}
	g.WithoutEmit(func() {
		for len(g.extensions) > 0 {
			sorted := slices.Sorted(maps.Keys(g.extensions))
			extKey := sorted[0]
			ext := g.extensions[extKey]
			savedExtensions = append(savedExtensions, struct {
				key string
				ext Extension
			}{extKey, ext})
			_ = g.genExtension(ext)
			delete(g.extensions, extKey)
		}
	})

	// Collect tuple structs
	var tupleStr string
	for _, k := range slices.Sorted(maps.Keys(g.tupleStructs)) {
		tupleStr += g.tupleStructs[k]
	}

	// Now collect imports after ALL code generation (declarations, monomorphized functions, and extensions)
	imports := make(map[string]*ast.ImportSpec)
	addImport := func(i *ast.ImportSpec) {
		// Normalize agl1/ prefix to match what will be generated
		pathValue := i.Path.Value
		if strings.HasPrefix(pathValue, `"agl1/`) {
			pathValue = `"` + pathValue[6:]
		}
		key := pathValue
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

	// Clear fragments and regenerate everything WITH emitting in the correct order
	g.fragments = nil
	g.varCounter.Store(0) // Reset counter so variable names match the first pass

	// Emit in correct order
	out += g.Emit(GeneratedFilePrefix)
	out += g.genPackage()
	out += g.genImports(importsArr)

	// Regenerate monomorphized types first (they need to be before functions that use them)
	for _, saved := range savedGenTypeDecls2 {
		out += saved.fn()
	}

	// Regenerate everything WITH emitting in the correct order
	out4 = g.genDecls(g.b)
	out5 = g.genDecls(g.a)
	out += out4.F() + out5.F()

	// Regenerate monomorphized functions WITH emitting
	for _, saved := range savedGenFuncDecls2 {
		out += saved.fn()
	}

	// Regenerate extensions WITH emitting
	for _, saved := range savedExtensions {
		out += g.genExtension(saved.ext)
	}

	// Emit tuple structs (already generated above)
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

// matchHasExplicitReturns checks if a match expression has explicit return statements in its body
func matchHasExplicitReturns(matchExpr *ast.MatchExpr) bool {
	if matchExpr.Body == nil {
		return false
	}
	for _, clause := range matchExpr.Body.List {
		if mc, ok := clause.(*ast.MatchClause); ok {
			for _, stmt := range mc.Body {
				if _, isReturn := stmt.(*ast.ReturnStmt); isReturn {
					return true
				}
			}
		}
	}
	return false
}

func ifExprHasExplicitReturns(ifExpr *ast.IfExpr) bool {
	// Check if all branches end with return statements
	if ifExpr.Body == nil || len(ifExpr.Body.List) == 0 {
		return false
	}
	// Check if the then branch ends with a return
	lastStmt := ifExpr.Body.List[len(ifExpr.Body.List)-1]
	if _, isReturn := lastStmt.(*ast.ReturnStmt); !isReturn {
		return false
	}
	// Check the else branch (if it exists)
	if ifExpr.Else == nil {
		// No else branch means not all paths return
		return false
	}
	// Check what kind of else we have
	switch elseStmt := ifExpr.Else.(type) {
	case *ast.BlockStmt:
		// Else is a block - check if it ends with return
		if len(elseStmt.List) == 0 {
			return false
		}
		_, isReturn := elseStmt.List[len(elseStmt.List)-1].(*ast.ReturnStmt)
		return isReturn
	case *ast.ExprStmt:
		// Else is an expression - check if it's another if-expr with explicit returns
		if nestedIf, ok := elseStmt.X.(*ast.IfExpr); ok {
			return ifExprHasExplicitReturns(nestedIf)
		}
		return false
	default:
		return false
	}
}

func (g *Generator) genStmt(s ast.Stmt) GenFrag {
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

func (g *Generator) genExpr(e ast.Expr) GenFrag {
	//p("genExpr", to(e))
	switch expr := e.(type) {
	case *ast.IfExpr:
		return g.genIfExpr(expr)
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
	case *ast.TemplateLit:
		return g.genTemplateLit(expr)
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

func (g *Generator) genIdent(expr *ast.Ident) GenFrag {
	e := EmitWith(g, expr)
	return GenFrag{F: func() string {
		// Check if this is the exclusion node - if so, use the old shadow value
		if expr == g.shadowingExclusionNode && g.shadowingExclusionOldValue != "" {
			return e(g.shadowingExclusionOldValue)
		}
		// Otherwise, check if this identifier has been shadowed
		if shadowedName, ok := g.shadowedVars[expr.Name]; ok {
			return e(shadowedName)
		}

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

		// If type is nil but we have a generic mapping, try to replace
		if t == nil && g.genMap != nil {
			if replacement, ok := g.genMap[expr.Name]; ok {
				return e(replacement.GoStr())
			}
		}

		switch v := t.(type) {
		case types.GenericType:
			// Handle GenericType directly (not wrapped in TypeType)
			// This is a value with a generic type - just use the identifier name
			// Don't replace it with the concrete type
			return e(expr.Name)
		case types.TypeType:
			// Check if this identifier is a type parameter that should be replaced
			if g.genMap != nil {
				if replacement, ok := g.genMap[expr.Name]; ok {
					return e(replacement.GoStr())
				}
			}
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
		case "pow":
			return e("AglPow")
		}
		v := g.env.Get(expr.Name)
		if v != nil {
			switch vv := v.(type) {
			case types.StructType:
				// Check if this is a generic struct type that needs monomorphization
				lookupKey := vv.String()
				if vv.IsGeneric() && g.genMap != nil {
					// Generate monomorphized struct if needed
					typeDecl := g.genTypeDecls[lookupKey]
					if typeDecl != nil {
						m := make(map[string]types.Type)
						for k, v := range g.genMap {
							m[k] = v
						}
						outTypeDecl := func() (out string) {
							g.WithGenMapping(m, func() {
								out = g.decrPrefix(g.genSpec(typeDecl, token.TYPE).F)
							})
							return
						}
						name := expr.Name
						for _, k := range slices.Sorted(maps.Keys(m)) {
							name += "_" + k + "_" + types.NormalizeTypeForMonomorphization(m[k].GoStr())
						}
						g.genTypeDecls2[name] = outTypeDecl
						return e(name)
					}
				}
				// For non-generic or when genMap is nil, return the type name
				return e(vv.GoStr())
			case types.TypeType:
				return e(vv.W.GoStr())
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

func handleTupleExpr(prefix, n string, v *ast.TupleExpr, out *string, e EmitterFunc) {
	handleTupleExprWithType(prefix, n, v, out, e, nil)
}

func handleTupleExprWithType(prefix, n string, v *ast.TupleExpr, out *string, e EmitterFunc, paramType types.Type) {
	// Check if this is a DictEntry type
	isDictEntry := false
	if paramType != nil {
		if structT, ok := types.Unwrap(paramType).(types.StructType); ok {
			if structT.Name == "DictEntry" {
				isDictEntry = true
			}
		}
	}

	for i, val := range v.Values {
		switch vv := val.(type) {
		case *ast.Ident:
			if vv.Name != "_" {
				var path string
				if isDictEntry {
					// For DictEntry, use .Key and .Value
					if i == 0 {
						path = fmt.Sprintf("%s.Key", n)
					} else if i == 1 {
						path = fmt.Sprintf("%s.Value", n)
					} else {
						path = fmt.Sprintf("%s.Arg%d", n, i)
					}
				} else {
					path = fmt.Sprintf("%s.Arg%d", n, i)
				}
				*out += e(fmt.Sprintf("%s%s := %s\n", prefix, vv.Name, path))
			}
		case *ast.TupleExpr:
			nextPath := fmt.Sprintf("%s.Arg%d", n, i)
			handleTupleExprWithType(prefix, nextPath, vv, out, e, nil)
		}
	}
}

func (g *Generator) genShortFuncLit(expr *ast.ShortFuncLit) GenFrag {
	e := EmitWith(g, expr)

	// Check if we need an implicit return (same logic as genFuncDecl)
	var c1 GenFrag
	implicitReturn := false
	hasReturnType := false

	// Get the function type to check if it has a return type
	ftTmp := g.env.GetType(expr)
	if ftTmp != nil {
		if v, ok := ftTmp.(types.LabelledType); ok {
			ftTmp = v.W
		}
		if t, ok := ftTmp.(types.FuncType); ok {
			hasReturnType = t.Return != nil && !TryCast[types.VoidType](t.Return)
		}
	}

	// Check if body is a BlockStmt with a single expression
	if blockStmt, ok := expr.Body.(*ast.BlockStmt); ok && hasReturnType && len(blockStmt.List) > 0 {
		// Find the first non-empty statement
		var firstNonEmpty ast.Stmt
		nonEmptyCount := 0
		for _, stmt := range blockStmt.List {
			if _, isEmpty := stmt.(*ast.EmptyStmt); !isEmpty {
				if nonEmptyCount == 0 {
					firstNonEmpty = stmt
				}
				nonEmptyCount++
			}
		}
		// If there's exactly one non-empty statement and it's an expression, add implicit return
		if nonEmptyCount == 1 {
			if exprStmt, ok := firstNonEmpty.(*ast.ExprStmt); ok {
				// Single expression in function with return type - implicit return
				implicitReturn = true
				// Generate the expression first
				exprFrag := g.genExpr(exprStmt.X)
				// Create a GenFrag that executes B functions then returns the expression
				c1 = GenFrag{
					F: func() string {
						var out string
						// Execute B functions first (like genStmts does)
						for _, b := range exprFrag.B {
							out += b()
						}
						// Then add the return statement (like genReturnStmt does)
						out += e(g.prefix+"return ") + exprFrag.F() + e("\n")
						return out
					},
				}
			}
		}
	}

	if !implicitReturn {
		c1 = g.genStmt(expr.Body)
	}

	return GenFrag{F: func() string {
		var out string
		ftTmp := g.env.GetType(expr)
		if v, ok := ftTmp.(types.LabelledType); ok {
			ftTmp = v.W
		}
		var t types.FuncType
		if ftTmp != nil {
			var ok bool
			if t, ok = ftTmp.(types.FuncType); !ok {
				panic(fmt.Sprintf("expected FuncType, got %T", ftTmp))
			}
		}
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
			returnStr += val
		}
		out = e(fmt.Sprintf("func(%s)%s {\n", argsStr, returnStr))
		for i, _ := range t.Params {
			n := fmt.Sprintf("aglArg%d", i)
			if len(expr.Args) > 0 {
				switch v := expr.Args[i].(type) {
				case *ast.TupleExpr:
					out += g.incrPrefix(func() (out string) {
						handleTupleExprWithType(g.prefix, n, v, &out, e, t.Params[i])
						return
					})
				}
			}
		}
		out += g.incrPrefix(c1.F)
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
	var c1 GenFrag
	g.withAsType(func() {
		c1 = g.genExpr(expr.X)
	})
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
	t := g.env.GetType(expr).(types.ResultType).W.GoStrType()
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string { return e("MakeResultErr["+t+"](") + c1.F() + e(")") }}
}

func (g *Generator) genChanType(expr *ast.ChanType) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Value)
	return GenFrag{F: func() string { return e("chan ") + c1.F() }}
}

func getCheck(t types.Type) string {
	t = types.Unwrap(t)
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

func (g *Generator) genOrContinueExpr(expr *ast.OrContinueExpr) GenFrag {
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

func (g *Generator) genOrReturn(expr *ast.OrReturnExpr) GenFrag {
	e := EmitWith(g, expr)
	check := getCheck(g.env.GetType(expr.X))
	varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
	c1 := g.genExpr(expr.X)
	returnType := g.returnType
	return GenFrag{F: func() string {
		return e("AglIdentity(" + varName + ")")
	}, B: []func() string{func() (out string) {
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
		return
	}}}
}

func (g *Generator) genUnaryExpr(expr *ast.UnaryExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		xT := types.Unwrap(g.env.GetType(expr.X))
		if v, ok := xT.(types.StructType); ok {
			lhsName := v.Name
			if expr.Op.String() == "-" && g.env.Get(lhsName+".__NEG") != nil {
				return c1.F() + e(".__NEG()")
			}
		}
		return e(expr.Op.String()) + c1.F()
	}}
}

func (g *Generator) genSendStmt(expr *ast.SendStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Chan)
	c2 := g.genExpr(expr.Value)
	return GenFrag{F: func() (out string) {
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += c1.F() + e(" <- ") + c2.F()
		if !g.inlineStmt {
			out += e("\n")
		}
		return
	}}
}

func (g *Generator) genSelectStmt(expr *ast.SelectStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Body)
	return GenFrag{F: func() (out string) {
		out += e(g.prefix + "select {\n")
		out += c1.F()
		out += e(g.prefix + "}\n")
		return
	}}
}

func (g *Generator) genLabeledStmt(expr *ast.LabeledStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genStmt(expr.Stmt)
	return GenFrag{F: func() (out string) {
		out += e(g.prefix + expr.Label.Name + ":\n")
		out += c1.F()
		return
	}}
}

func (g *Generator) genBranchStmt(expr *ast.BranchStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := GenFrag{F: emptyContent}
	if expr.Label != nil {
		c1 = g.genExpr(expr.Label)
	}
	return GenFrag{F: func() (out string) {
		out += e(g.prefix + expr.Tok.String())
		if expr.Label != nil {
			out += e(" ") + c1.F()
		}
		out += e("\n")
		return
	}}
}

func (g *Generator) genDeferStmt(expr *ast.DeferStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Call)
	return GenFrag{F: func() (out string) {
		out += e(g.prefix+"defer ") + c1.F() + e("\n")
		return
	}}
}

func (g *Generator) genGoStmt(expr *ast.GoStmt) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.Call)
	return GenFrag{F: func() (out string) {
		out += e(g.prefix+"go ") + c1.F() + e("\n")
		return
	}}
}

func (g *Generator) genEmptyStmt(expr *ast.EmptyStmt) GenFrag {
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

	// For enum types, check if this is a value-returning match
	if v, ok := initT.(types.EnumType); ok {
		if expr.Body != nil {
			matchT := g.env.GetType(expr)
			if matchT != nil && !TryCast[types.VoidType](matchT) {
				// Value-returning enum match - use the B field pattern
				tmpId := g.varCounter.Add(1)
				matchReturnsTmp := fmt.Sprintf("aglTmp%d", tmpId)

				var bs []func() string
				bs = append(bs, func() string {
					var out string
					gPrefix := g.prefix
					out += e(gPrefix + "var " + matchReturnsTmp + " " + matchT.GoStrType() + "\n")

					var hasDefault bool
					for i, cc := range expr.Body.List {
						c := cc.(*ast.MatchClause)
						if i > 0 {
							out += e(gPrefix + "} else ")
						}
						if c.Expr != nil {
							switch cv := c.Expr.(type) {
							case *ast.CallExpr:
								sel := cv.Fun.(*ast.SelectorExpr)
								prefix := ""
								if i == 0 {
									prefix = gPrefix
								}
								out += e(prefix+"if ") + g.genExpr(expr.Init).F() + e(".Tag == "+v.Name+"_"+sel.Sel.Name+" {\n")
								for j, id := range cv.Args {
									rhs := func() string { return g.genExpr(expr.Init).F() + e("."+v.Fields[i].Name+"_"+strconv.Itoa(j)) }
									if id.(*ast.Ident).Name == "_" {
										out += e(gPrefix+"\t_ = ") + rhs() + e("\n")
									} else {
										out += e(gPrefix+"\t") + g.genExpr(id).F() + e(" := ") + rhs() + e("\n")
										if g.allowUnused {
											out += e(gPrefix+"\tAglNoop(") + g.genExpr(id).F() + e(")\n")
										}
									}
								}
								if len(c.Body) > 0 {
									lastStmt := c.Body[len(c.Body)-1]
									if exprStmt, ok := lastStmt.(*ast.ExprStmt); ok {
										out += e(gPrefix+"\t"+matchReturnsTmp+" = ") + g.genExpr(exprStmt.X).F() + e("\n")
									} else {
										out += g.incrPrefix(g.genStmts(c.Body).F)
									}
								}
							case *ast.SelectorExpr:
								prefix := ""
								if i == 0 {
									prefix = gPrefix
								}
								out += e(prefix+"if ") + g.genExpr(expr.Init).F() + e(".Tag == "+v.Name+"_"+cv.Sel.Name+" {\n")
								if len(c.Body) > 0 {
									lastStmt := c.Body[len(c.Body)-1]
									if exprStmt, ok := lastStmt.(*ast.ExprStmt); ok {
										out += e(gPrefix+"\t"+matchReturnsTmp+" = ") + g.genExpr(exprStmt.X).F() + e("\n")
									} else {
										out += g.incrPrefix(g.genStmts(c.Body).F)
									}
								}
							default:
								panic(fmt.Sprintf("%v", to(c.Expr)))
							}
						} else {
							hasDefault = true
							out += e("{\n")
							if len(c.Body) > 0 {
								lastStmt := c.Body[len(c.Body)-1]
								if exprStmt, ok := lastStmt.(*ast.ExprStmt); ok {
									out += e(gPrefix+"\t"+matchReturnsTmp+" = ") + g.genExpr(exprStmt.X).F() + e("\n")
								} else {
									out += g.incrPrefix(g.genStmts(c.Body).F)
								}
							}
							out += e(gPrefix + "}")
						}
					}
					if !hasDefault {
						out += e(gPrefix + "} else {\n")
						out += e(gPrefix + "\tpanic(\"match on enum should be exhaustive\")\n")
						out += e(gPrefix + "}")
					}
					out += e("\n")
					return out
				})

				return GenFrag{
					F: func() string {
						return e("AglIdentity(" + matchReturnsTmp + ")")
					},
					B: bs,
				}
			}
		}
	}

	// Non-value-returning or non-enum matches use the original pattern
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
							binding := g.genExpr(v.X).F
							out += e(gPrefix + "if " + errName + " == nil {\n" + gPrefix + "\t")
							op := binding()
							out += op + e(" "+assignOp(op)+" *"+varName+"\n")
						case *ast.ErrExpr:
							binding := g.genExpr(v.X).F
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
					content3 := g.incrPrefix(g.genStmts(c.Body).F)
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
					content3 := g.incrPrefix(g.genStmts(c.Body).F)
					out += content3
					out += e(gPrefix + "}")
					return
				}, "\n")
			}
		case types.EnumType:
			// Non-value-returning enum match
			if expr.Body != nil {
				var hasDefault bool
				for i, cc := range expr.Body.List {
					c := cc.(*ast.MatchClause)
					if i > 0 {
						out += e(gPrefix + "} else ")
					}
					if c.Expr != nil {
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
									if g.allowUnused {
										out += e(gPrefix+"\tAglNoop(") + g.genExpr(id).F() + e(")\n")
									}
								}
							}
							out += g.incrPrefix(g.genStmts(c.Body).F)
						case *ast.SelectorExpr:
							out += e("if ") + g.genExpr(expr.Init).F() + e(".Tag == "+v.Name+"_"+cv.Sel.Name+" {\n")
							out += g.incrPrefix(g.genStmts(c.Body).F)
						default:
							panic(fmt.Sprintf("%v", to(c.Expr)))
						}
					} else {
						hasDefault = true
						out += e("{\n")
						out += g.incrPrefix(g.genStmts(c.Body).F)
						out += e(gPrefix + "}")
					}
				}
				if !hasDefault {
					out += e(gPrefix + "} else {\n")
					out += e(gPrefix + "\tpanic(\"match on enum should be exhaustive\")\n")
					out += e(gPrefix + "}")
				}
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
				out += g.incrPrefix(g.genStmts(cc.Body).F)
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
		c1 := GenFrag{F: emptyContent}
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
			content3 := func() (out string) {
				if expr1.Body != nil {
					out = g.incrPrefix(g.genStmts(expr1.Body).F)
				}
				return out
			}
			out += e(g.prefix) + listStr() + content3()
		}
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genCommClause(expr *ast.CommClause) GenFrag {
	e := EmitWith(g, expr)
	c1 := GenFrag{F: emptyContent}
	c2 := GenFrag{F: emptyContent}
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
		content1 := g.incrPrefix(c1.F)
		return "..." + content1
	}}
}

func (g *Generator) genParenExpr(expr *ast.ParenExpr) GenFrag {
	e := EmitWith(g, expr)
	c1 := g.genExpr(expr.X)
	return GenFrag{F: func() string {
		var out string
		out += e("(")
		out += g.incrPrefix(c1.F)
		out += e(")")
		return out
	}, B: c1.B}
}

func (g *Generator) genFuncLit(expr *ast.FuncLit) GenFrag {
	e := EmitWith(g, expr)

	// Check if we need an implicit return (same logic as genFuncDecl)
	var c1 GenFrag
	implicitReturn := false
	if expr.Body != nil {
		hasReturnType := expr.Type.Result != nil && !TryCast[types.VoidType](g.env.GetType(expr.Type.Result))
		if hasReturnType && len(expr.Body.List) > 0 {
			// Find the first non-empty statement
			var firstNonEmpty ast.Stmt
			nonEmptyCount := 0
			for _, stmt := range expr.Body.List {
				if _, isEmpty := stmt.(*ast.EmptyStmt); !isEmpty {
					if nonEmptyCount == 0 {
						firstNonEmpty = stmt
					}
					nonEmptyCount++
				}
			}
			// If there's exactly one non-empty statement and it's an expression, add implicit return
			if nonEmptyCount == 1 {
				if exprStmt, ok := firstNonEmpty.(*ast.ExprStmt); ok {
					// Single expression in function with return type - implicit return
					implicitReturn = true
					// Generate the expression first
					exprFrag := g.genExpr(exprStmt.X)
					// Create a GenFrag that executes B functions then returns the expression
					c1 = GenFrag{
						F: func() string {
							var out string
							// Execute B functions first (like genStmts does)
							for _, b := range exprFrag.B {
								out += b()
							}
							// Then add the return statement (like genReturnStmt does)
							out += e(g.prefix+"return ") + exprFrag.F() + e("\n")
							return out
						},
					}
				}
			}
		}
		if !implicitReturn {
			c1 = g.genStmt(expr.Body)
		}
	}

	return GenFrag{F: func() string {
		var out string
		out += g.genFuncType(expr.Type).F() + e(" {\n")
		out += g.incrPrefix(c1.F)
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
			g.withAsType(func() {
				out += g.genExpr(field.Type).F()
			})
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
					out += e(types.ReplGenM(g.env.GetType(field.Type), g.genMap).GoStr())
				} else if fieldType := g.env.GetType(field.Type); fieldType != nil {
					// Check if it's a FuncType that needs generic replacement
					if ft, ok := fieldType.(types.FuncType); ok {
						if g.genMap != nil {
							replacedType := types.ReplGenM(ft, g.genMap).(types.FuncType)
							out += e(replacedType.GoStrType())
						} else {
							out += g.genExpr(field.Type).F()
						}
					} else if v, ok := fieldType.(types.TypeType); ok {
						// Handle TypeType - check if it wraps a FuncType
						if _, ok := v.W.(types.FuncType); ok {
							out += e(types.ReplGenM(v.W, g.genMap).(types.FuncType).GoStrType())
						} else {
							out += g.genExpr(field.Type).F()
						}
					} else if id, ok := field.Type.(*ast.Ident); ok && TryCast[types.GenericType](fieldType) {
						typ := fieldType.(types.GenericType)
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
				if resultType := g.env.GetType(expr.Result); resultType != nil {
					return e(" ") + e(resultType.GoStrType())
				}
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

	// Check if this is a generic struct type instantiation (e.g., TestStruct[string])
	t := g.env.GetType(expr)

	// If type is nil, this might be a type-level index expression like TestStruct[string]
	// where we're applying a type parameter to a generic struct
	if t == nil {
		if baseIdent, ok := expr.X.(*ast.Ident); ok {
			baseType := g.env.Get(baseIdent.Name)

			if structT, ok := baseType.(types.StructType); ok && structT.IsGeneric() {
				// This is a generic struct - get the concrete type parameter
				// The index should be a type identifier
				if indexIdent, ok := expr.Index.(*ast.Ident); ok {
					concreteType := g.env.Get(indexIdent.Name)

					// Build the monomorphized struct
					lookupKey := structT.String() // e.g., "TestStruct[any]"
					typeDecl := g.genTypeDecls[lookupKey]

					if typeDecl != nil {
						// Build type parameter mapping
						m := make(map[string]types.Type)
						if typeDecl.TypeParams != nil && len(typeDecl.TypeParams.List) > 0 {
							// Assume single type parameter for now
							field := typeDecl.TypeParams.List[0]
							if len(field.Names) > 0 {
								m[field.Names[0].Name] = concreteType
							}
						}

						// Generate the monomorphized struct
						outTypeDecl := func() (out string) {
							g.WithGenMapping(m, func() {
								out = g.decrPrefix(g.genSpec(typeDecl, token.TYPE).F)
							})
							return
						}

						name := baseIdent.Name
						for _, k := range slices.Sorted(maps.Keys(m)) {
							name += "_" + k + "_" + types.NormalizeTypeForMonomorphization(m[k].GoStr())
						}

						g.genTypeDecls2[name] = outTypeDecl

						return GenFrag{F: func() string {
							return e(name)
						}}
					}
				}
			}
		}
	}

	if tt, ok := t.(types.TypeType); ok {
		t = tt.W
	}

	// If this is a struct type with concrete type parameters, monomorphize it
	if structT, ok := t.(types.StructType); ok && structT.IsGeneric() {
		// Check if all type parameters are concrete (not GenericType)
		allConcrete := true
		for _, tp := range structT.TypeParams {
			if _, isGeneric := tp.(types.GenericType); isGeneric {
				allConcrete = false
				break
			}
		}

		if allConcrete {
			// This is a monomorphizable struct - generate it and return the monomorphized name
			if baseIdent, ok := expr.X.(*ast.Ident); ok {
				lookupKey := structT.String1() + "[any]" // Generic key
				typeDecl := g.genTypeDecls[lookupKey]

				if typeDecl != nil {
					// Build type parameter mapping
					m := make(map[string]types.Type)
					if typeDecl.TypeParams != nil {
						for i, field := range typeDecl.TypeParams.List {
							if i < len(structT.TypeParams) {
								for _, name := range field.Names {
									m[name.Name] = structT.TypeParams[i]
								}
							}
						}
					}

					// Generate the monomorphized struct
					outTypeDecl := func() (out string) {
						g.WithGenMapping(m, func() {
							out = g.decrPrefix(g.genSpec(typeDecl, token.TYPE).F)
						})
						return
					}

					name := baseIdent.Name
					for _, k := range slices.Sorted(maps.Keys(m)) {
						name += "_" + k + "_" + types.NormalizeTypeForMonomorphization(m[k].GoStr())
					}

					g.genTypeDecls2[name] = outTypeDecl

					return GenFrag{F: func() string {
						return e(name)
					}}
				}
			}
		}
	}

	// Default behavior for non-struct or non-monomorphizable cases
	c1 := g.genExpr(expr.X)
	c2 := g.genExpr(expr.Index)
	return GenFrag{F: func() string {
		return c1.F() + e("[") + c2.F() + e("]")
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
		opStr := utils.Ternary(expr.Op == token.RANGEOPEQ, "true", "false")
		return e(opStr)
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
	var bs []func() string
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return GenFrag{F: func() string {
		name := expr.Sel.Name
		t := types.Unwrap(g.env.GetType(expr.X))
		switch t.(type) {
		case types.TupleType:
			name = "Arg" + name
		case types.EnumType:
			var out string
			if expr.Sel.Name == "RawValue" {
				out = c1.F() + e(".") + c2.F()
			} else {
				out = e("Make_") + c1.F() + e("_") + c2.F()
				if _, ok := g.env.GetType(expr).(types.EnumType); ok { // TODO
					out += e("()")
				}
			}
			return out
		}
		return c1.F() + e("."+name)
	}, B: bs}
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
		panic(fmt.Sprintf("%v", exprXT))
	}
}

func (g *Generator) genBubbleResultExpr(expr *ast.BubbleResultExpr) GenFrag {
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

func (g *Generator) genCallExprIdent(expr *ast.CallExpr, x *ast.Ident) GenFrag {
	e := EmitWith(g, expr)
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
	} else if x.Name == "assertEq" {
		var contents []func() string
		var bs []func() string
		for _, arg := range expr.Args {
			argFrag := g.genExpr(arg)
			contents = append(contents, argFrag.F)
			bs = append(bs, argFrag.B...)
		}
		return GenFrag{F: func() string {
			var out string
			out = e("AglAssertEq(")
			line := g.fset.Position(expr.Pos()).Line
			msg := fmt.Sprintf(`"assert failed line %d"`, line)
			if len(contents) == 2 {
				contents = append(contents, func() string { return e(msg) })
			} else {
				tmp := contents[2]
				contents[2] = func() string { return e(msg+` + " " + `) + tmp() }
			}
			out += MapJoin(e, contents, func(f func() string) string { return f() }, ", ")
			out += e(")")
			return out
		}, B: bs}
	} else if x.Name == "precondition" {
		var contents []func() string
		for _, arg := range expr.Args {
			contents = append(contents, g.genExpr(arg).F)
		}
		return GenFrag{F: func() string {
			var out string
			out = e("AglPrecondition(")
			line := g.fset.Position(expr.Pos()).Line
			msg := fmt.Sprintf(`"precondition failed line %d"`, line)
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
	} else if x.Name == "printf" {
		var contents []func() string
		for _, arg := range expr.Args {
			contents = append(contents, g.genExpr(arg).F)
		}
		return GenFrag{F: func() (out string) {
			out = e("AglPrintf(")
			out += MapJoin(e, contents, func(f func() string) string { return f() }, ", ")
			out += e(")")
			return
		}}
	} else if x.Name == "print" {
		var contents []func() string
		var bs []func() string
		for _, arg := range expr.Args {
			argFrag := g.genExpr(arg)
			contents = append(contents, argFrag.F)
			bs = append(bs, argFrag.B...)
		}
		return GenFrag{F: func() (out string) {
			out = e("AglPrint(")
			out += MapJoin(e, contents, func(f func() string) string { return f() }, ", ")
			out += e(")")
			return
		}, B: bs}
	} else if x.Name == "println" {
		var contents []func() string
		var bs []func() string
		for _, arg := range expr.Args {
			argFrag := g.genExpr(arg)
			contents = append(contents, argFrag.F)
			bs = append(bs, argFrag.B...)
		}
		return GenFrag{F: func() (out string) {
			out = e("AglPrintln(")
			out += MapJoin(e, contents, func(f func() string) string { return f() }, ", ")
			out += e(")")
			return
		}, B: bs}
	} else if x.Name == "panic" {
		return GenFrag{F: func() string { return e("panic(nil)") }}
	} else if x.Name == "panicWith" {
		c1 := g.genExpr(expr.Args[0])
		return GenFrag{F: func() string { return e("panic(") + c1.F() + e(")") }}
	} else if x.Name == "Set" {
		arg0 := expr.Args[0]
		arg0T := g.env.GetType(arg0)
		c1 := g.genExpr(arg0)
		return GenFrag{F: func() (out string) {
			out += e("AglBuildSet(")
			if TryCast[types.ArrayType](arg0T) {
				out += e("AglVec["+arg0T.(types.ArrayType).Elt.GoStrType()+"](") + c1.F() + e(")")
			} else {
				out += c1.F()
			}
			out += e(")")
			return
		}}
	} else if x.Name == "Array" {
		arg0 := expr.Args[0]
		arg0T := g.env.GetType(arg0)
		c1 := g.genExpr(arg0)
		return GenFrag{F: func() (out string) {
			out += e("AglBuildArray(")
			if TryCast[types.ArrayType](arg0T) {
				out += e("AglVec["+arg0T.(types.ArrayType).Elt.GoStrType()+"](") + c1.F() + e(")")
			} else {
				out += c1.F()
			}
			out += e(")")
			return
		}}
	}
	return GenFrag{}
}

func (g *Generator) genCallExprSelectorExpr(expr *ast.CallExpr, x *ast.SelectorExpr) GenFrag {
	e := EmitWith(g, expr)
	oeXT := g.env.GetType(x.X)
	// Unwrap wrapper types but preserve StructType (including type aliases)
	eXT := oeXT
	for {
		switch v := eXT.(type) {
		case types.TypeType:
			eXT = v.W
		case types.LabelledType:
			eXT = v.W
		case types.MutType:
			eXT = v.W
		case types.StarType:
			eXT = v.X
		default:
			goto afterUnwrap4
		}
	}
afterUnwrap4:
	// Workaround: Fix incorrect type inference for map method calls
	// Sometimes the return type (Sequence) is incorrectly stored on the receiver identifier
	if eXT != nil {
		if st, ok := eXT.(types.StructType); ok && st.Name == "Sequence" {
			if x.Sel.Name == "Keys" || x.Sel.Name == "Values" {
				// This is likely a map method call that was mistyped
				if _, ok := x.X.(*ast.Ident); ok {
					// Look for the defining assignment
					// For now, just treat it as a generic map type
					// The actual K and V types will be inferred during code generation
					eXT = types.MapType{K: types.IntType{}, V: types.IntType{}}
				}
			}
		}
	}
	switch eXTT := eXT.(type) {
	case types.InterfaceType:
		c1 := g.genExpr(x.X)
		genEX := c1.F
		fnName := x.Sel.Name

		// Get the function type to extract type parameters
		fnT := g.env.GetType(x.Sel).(types.FuncType)

		// Extract type parameter from the interface
		var tType string
		var tTypeObj types.Type
		if len(eXTT.TypeParams) > 0 {
			tTypeObj = eXTT.TypeParams[0]
			tType = tTypeObj.GoStrType()
		}

		// Create a genMap for generic type parameters
		genMap := make(map[string]types.Type)
		genMap["T"] = tTypeObj

		// Generate argument with proper type mapping
		var genArgFn func(i int) string
		genArgFn = func(i int) string {
			var result string
			g.WithGenMapping(genMap, func() {
				result = g.genExpr(expr.Args[i]).F()
			})
			return result
		}

		// Generate call to standalone function
		switch fnName {
		case "Filter":
			return GenFrag{F: func() string {
				return e("AglIteratorFilter["+tType+"](") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		case "Map":
			// Extract return type parameter
			var rType string
			if retT, ok := fnT.Return.(types.StructType); ok && len(retT.TypeParams) > 0 {
				rType = retT.TypeParams[0].GoStrType()
			}
			return GenFrag{F: func() string {
				return e("AglIteratorMap["+tType+", "+rType+"](") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		case "Any", "AllSatisfy":
			return GenFrag{F: func() string {
				return e("AglIterator"+fnName+"["+tType+"](") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		case "Contains", "ForEach":
			return GenFrag{F: func() string {
				return e("AglIterator"+fnName+"["+tType+"](") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		default:
			// Fallback: call method directly (may not work for all interfaces)
			argFrags := g.genExprs(expr.Args)
			return GenFrag{F: func() string {
				return genEX() + e("."+fnName+"(") + argFrags.F() + e(")")
			}}
		}
	case types.StructType:
		c1 := g.genExpr(x.X)
		genEX := c1.F
		genArgFn := func(i int) string { return g.genExpr(expr.Args[i]).F() }
		fnName := x.Sel.Name
		switch fnName {
		case "Sum":
			fnT := g.env.GetType(x.Sel).(types.FuncType)
			structType := fnT.Recv[0].(types.StructType)
			// Handle both GenericType and concrete types
			var recvTType types.Type
			if genType, ok := structType.TypeParams[0].(types.GenericType); ok {
				recvTType = genType.W
			} else {
				recvTType = structType.TypeParams[0]
			}
			recvT := recvTType.GoStrType()
			retT := fnT.Return.GoStrType()
			return GenFrag{F: func() string { return e("AglSequence"+fnName+"["+recvT+", "+retT+"](") + genEX() + e(")") }}
		case "Filter", "Joined", "TakeWhile":
			return GenFrag{F: func() string { return e("AglSequence"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(")") }}
		case "FilterMap":
			fnT := g.env.GetType(x.Sel).(types.FuncType)
			structType := fnT.Recv[0].(types.StructType)
			var recvTType types.Type
			if genType, ok := structType.TypeParams[0].(types.GenericType); ok {
				recvTType = genType.W
			} else {
				recvTType = structType.TypeParams[0]
			}
			recvT := recvTType.GoStrType()
			// Extract the R type from the return type Sequence[R]
			retType := fnT.Return.(types.StructType)
			var retTType types.Type
			if genType, ok := retType.TypeParams[0].(types.GenericType); ok {
				retTType = genType.W
			} else {
				retTType = retType.TypeParams[0]
			}
			retT := retTType.GoStrType()
			return GenFrag{F: func() string {
				return e("AglSequenceFilterMap["+recvT+", "+retT+"](") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		case "Len", "Sorted", "Max", "Min":
			return GenFrag{F: func() string { return e("AglSequence"+fnName+"(") + genEX() + e(")") }}
		}
	case types.ArrayType:
		c1 := g.genExpr(x.X)
		genEX := c1.F
		genArgFn := func(i int) string { return g.genExpr(expr.Args[i]).F() }
		eltT := types.ReplGenM(eXTT.Elt, g.genMap)
		eltTStr := eltT.GoStr()
		fnName := x.Sel.Name
		switch fnName {
		case "Iter":
			return GenFrag{F: func() string {
				return e("AglVecIter["+eltTStr+"](") + genEX() + e(")")
			}}
		case "Sum", "Clone", "Indices", "Sorted":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"(") + genEX() + e(")")
			}}
		case "Filter", "AllSatisfy", "Contains", "ContainsWhere", "Any", "Map", "FilterMap", "Find", "Joined",
			"FirstIndex", "FirstIndexWhere", "__ADD", "SortedBy", "DropFirst", "DropLast":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(")")
			}}
		case "Reduce", "ReduceInto":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"(") + genEX() + e(", ") + genArgFn(0) + e(", ") + genArgFn(1) + e(")")
			}}
		case "Insert", "Swap", "With":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + genArgFn(0) + e(", ") + genArgFn(1) + e(")")
			}}
		case "PopIf", "PushFront", "Remove":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + genArgFn(0) + e(")")
			}}
		case "Pop", "PopFront", "Clear", "RemoveFirst":
			return GenFrag{F: func() string {
				return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e(")") + e(")")
			}}
		case "Push":
			argFrags := g.genExprs(expr.Args)
			paramsStr := argFrags.F
			var bs []func() string
			bs = append(bs, c1.B...)
			bs = append(bs, argFrags.B...)
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
						}, B: bs}
					}
				}
			}

			if _, ok := tmpoeXT.(types.StarType); ok {
				return GenFrag{F: func() string {
					return e("AglVec"+fnName+"(") + genEX() + e(", ") + paramsStr() + ellipsis() + e(")")
				}, B: bs}
			} else {
				return GenFrag{F: func() string {
					return e("AglVec"+fnName+"((*[]"+eltTStr+")(&") + genEX() + e("), ") + paramsStr() + ellipsis() + e(")")
				}, B: bs}
			}
		default:
			extName := "agl1.Vec." + fnName
			for _, a := range expr.Args {
				if v, ok := a.(*ast.LabelledArg); ok {
					extName += fmt.Sprintf("_%s", v.Label.Name)
					fnName += fmt.Sprintf("_%s", v.Label.Name)
				}
			}
			rawFnT := g.env.Get(extName)
			concreteT := g.env.GetType(expr.Fun)
			m := types.FindGen(rawFnT, concreteT)
			g.addExtension(extName, ExtensionTest{raw: rawFnT, concrete: concreteT})
			r := strings.NewReplacer(
				"[", "_",
				"]", "_",
				"*", "_",
			)
			var els []string
			for _, k := range slices.Sorted(maps.Keys(m)) {
				els = append(els, fmt.Sprintf("%s_%s", k, r.Replace(m[k].GoStr())))
			}
			if _, ok := m["T"]; !ok {
				recvTName := concreteT.(types.FuncType).Recv[0].(types.ArrayType).Elt.GoStrType()
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
		case "FormUnion":
			arg0 := expr.Args[0]
			content2 := func() string {
				switch v := types.Unwrap(g.env.GetType(arg0)).(type) {
				case types.ArrayType:
					return e("AglVec["+v.Elt.GoStrType()+"](") + g.genExpr(arg0).F() + e(")")
				default:
					return g.genExpr(arg0).F()
				}
			}
			c1 := g.genExpr(x.X)
			c1T := g.env.GetType(x.X)
			if v, ok := c1T.(types.MutType); ok {
				c1T = v.W
			}
			return GenFrag{F: func() (out string) {
				out += e("AglSet" + fnName + "(")
				if TryCast[types.StarType](c1T) {
					out += e("*")
				}
				out += c1.F() + e(", ") + content2() + e(".Iter())")
				return
			}}
		case "Union", "Intersects", "Subtracting", "Subtract", "Intersection", "FormIntersection",
			"SymmetricDifference", "FormSymmetricDifference", "IsSubset", "IsStrictSubset", "IsSuperset", "IsStrictSuperset", "IsDisjoint",
			"Filter", "ForEach", "Map":
			arg0 := expr.Args[0]
			content2 := func() string {
				switch v := types.Unwrap(g.env.GetType(arg0)).(type) {
				case types.ArrayType:
					return e("AglVec["+v.Elt.GoStrType()+"](") + g.genExpr(arg0).F() + e(")")
				default:
					return g.genExpr(arg0).F()
				}
			}
			c1 := g.genExpr(x.X)
			c1T := g.env.GetType(x.X)
			if v, ok := c1T.(types.MutType); ok {
				c1T = v.W
			}
			return GenFrag{F: func() (out string) {
				out += e("AglSet" + fnName + "(")
				if TryCast[types.StarType](c1T) {
					out += e("*")
				}
				out += c1.F() + e(", ") + content2() + e(")")
				return
			}}
		case "Insert", "Remove", "Contains", "ContainsWhere", "Equals", "FirstWhere":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglSet"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(")")
			}}
		case "Len", "Min", "Max", "Iter", "RemoveFirst", "First", "IsEmpty":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglSet"+fnName+"(") + c1.F() + e(")") }}
		}
	case types.RangeType:
		fnName := x.Sel.Name
		switch fnName {
		case "Rev":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglDoubleEndedIteratorRev(") + c1.F() + e(")") }}
		case "AllSatisfy", "Contains", "ForEach", "Filter", "Map":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string { return e("AglIterator"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(")") }}
		}
	case types.IntType, types.UntypedNumType:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglIntString(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglIntSqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglIntAbs(") + c1.F() + e(")") }}
		}
	case types.I64Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI64String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI64Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI64Abs(") + c1.F() + e(")") }}
		}
	case types.I32Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI32String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI32Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI32Abs(") + c1.F() + e(")") }}
		}
	case types.I16Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI16String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI16Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI16Abs(") + c1.F() + e(")") }}
		}
	case types.I8Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI8String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI8Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglI8Abs(") + c1.F() + e(")") }}
		}
	case types.U8Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU8String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU8Sqrt(") + c1.F() + e(")") }}
		}
	case types.U16Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU16String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU16Sqrt(") + c1.F() + e(")") }}
		}
	case types.U32Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU32String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU32Sqrt(") + c1.F() + e(")") }}
		}
	case types.U64Type:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU64String(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglU64Sqrt(") + c1.F() + e(")") }}
		}
	case types.UintType:
		fnName := x.Sel.Name
		switch fnName {
		case "String":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglUintString(") + c1.F() + e(")") }}
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglUintSqrt(") + c1.F() + e(")") }}
		}
	case types.F32Type:
		fnName := x.Sel.Name
		switch fnName {
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglF32Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglF32Abs(") + c1.F() + e(")") }}
		}
	case types.F64Type:
		fnName := x.Sel.Name
		switch fnName {
		case "Sqrt":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglF64Sqrt(") + c1.F() + e(")") }}
		case "Abs":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglF64Abs(") + c1.F() + e(")") }}
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
		case "Split", "SplitAfter", "Cut", "CutPrefix", "CutSuffix", "TrimPrefix", "TrimSuffix", "HasPrefix", "HasSuffix",
			"Contains", "ContainsAny", "Index", "LastIndex", "Count", "Repeat", "Trim", "TrimLeft", "TrimRight", "ZFill":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglString"+fnName+"(") + c1.F() + e(", ") + c2.F() + e(")")
			}}
		case "TrimSpace", "IsEmpty", "Lowercased", "Uppercased", "AsBytes", "Lines", "Int", "I8", "I16", "I32", "I64", "Uint", "U8", "U16", "U32", "U64", "F32", "F64", "Len":
			c1 := g.genExpr(x.X)
			return GenFrag{F: func() string { return e("AglString"+fnName+"(") + c1.F() + e(")") }}
		default:
			extName := "agl1.String." + fnName
			rawFnT := g.env.Get(extName)
			concreteT := rawFnT
			g.addExtension(extName, ExtensionTest{raw: rawFnT, concrete: concreteT})
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
		case "AllSatisfy":
			c1 := g.genExpr(x.X)
			c2 := g.genExpr(expr.Args[0])
			return GenFrag{F: func() string {
				return e("AglMapAllSatisfy(") + c1.F() + e(", ") + c2.F() + e(")")
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
		genEX := c1.F
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
		if res := g.genCallExprIdent(expr, x); res.F != nil {
			return res
		}
	}
	content1 := emptyContent
	content2 := emptyContent
	var isSet bool
	switch v := expr.Fun.(type) {
	case *ast.Ident:
		t1 := g.env.Get(v.Name)
		if t2, ok := t1.(types.TypeType); ok && TryCast[types.CustomType](t2.W) {
			isSet = true
			c2 := g.genExprs(expr.Args)
			content1 = func() string { return e(v.Name) }
			content2 = c2.F
		} else if fnT, ok := t1.(types.FuncType); ok {
			if !InArray(v.Name, []string{"make", "append", "len", "new", "abs", "pow", "min", "max", "zip"}) && fnT.IsGeneric() {
				isSet = true
				oFnT := g.env.Get(v.Name)
				newFnT := g.env.GetType(v)
				fnDecl := g.genFuncDecls[oFnT.String()]
				m := types.FindGen(oFnT, newFnT)
				for k, v := range g.genMap {
					m[k] = v
				}
				outFnDecl := func() (out string) {
					g.WithGenMapping(m, func() {
						out = g.decrPrefix(g.genFuncDecl(fnDecl).F)
					})
					return
				}
				name := g.genExpr(v).FNoEmit(g)
				for _, k := range slices.Sorted(maps.Keys(m)) {
					name += "_" + k + "_" + types.NormalizeTypeForMonomorphization(m[k].GoStr())
				}
				g.genFuncDecls2[name] = outFnDecl
				content1 = func() string { return e(name) }
				content2 = func() (out string) {
					g.WithGenMapping(m, func() {
						out += g.genExprs(expr.Args).F()
					})
					return out
				}
			} else if v.Name == "make" {
				isSet = true
				c1 := g.genExpr(v)
				c2 := g.genExpr(expr.Args[0])
				bs = append(bs, c1.B...)
				bs = append(bs, c2.B...)
				content1 = c1.F
				content2 = func() (out string) {
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
			} else if v.Name == "zip" {
				isSet = true
				newFnT := g.env.GetType(v).(types.FuncType)

				// Build the monomorphized function name
				name := "zip"
				for i := 0; i < len(expr.Args); i++ {
					genericName := fmt.Sprintf("T%d", i+1)
					argType := types.Unwrap(g.env.GetType(expr.Args[i]))
					if arrT, ok := argType.(types.ArrayType); ok {
						name += fmt.Sprintf("_%s_%s", genericName, types.NormalizeTypeForMonomorphization(arrT.Elt.GoStr()))
					}
				}

				// Generate the zip function implementation if not already generated
				if g.genFuncDecls2[name] == nil {
					g.genFuncDecls2[name] = g.generateZipFunc(name, newFnT, expr.Args)
				}

				content1 = func() string { return e(name) }
				content2 = func() (out string) {
					return MapJoin(e, expr.Args, func(arg ast.Expr) string { return g.genExpr(arg).F() }, ", ")
				}
			}
		}
	}
	if !isSet {
		c1 := g.genExpr(expr.Fun)
		c2 := g.genExprs(expr.Args)
		bs = append(bs, c1.B...)
		bs = append(bs, c2.B...)
		content1 = c1.F
		content2 = c2.F
	}
	return GenFrag{F: func() string {
		maybeEllipsis := func() (out string) {
			if expr.Ellipsis != 0 {
				out += e("...")
			}
			return
		}
		return content1() + e("(") + content2() + maybeEllipsis() + e(")")
	}, B: bs}
}

func (g *Generator) genSetType(expr *ast.SetType) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	var c1 GenFrag
	g.withAsType(func() {
		c1 = g.genExpr(expr.Key)
	})
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
		return c1.F() + e(": ") + c2.F()
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

func (g *Generator) genTemplateLit(expr *ast.TemplateLit) GenFrag {
	e := EmitWith(g, expr)

	// Add fmt import since we're using fmt.Sprintf
	if g.imports == nil {
		g.imports = make(map[string]*ast.ImportSpec)
	}
	g.imports[`"fmt"`] = &ast.ImportSpec{
		Path: &ast.BasicLit{Value: `"fmt"`},
	}

	// Generate code for all parts
	var partFrags []GenFrag
	var bs []func() string
	for _, part := range expr.Parts {
		frag := g.genExpr(part)
		partFrags = append(partFrags, frag)
		bs = append(bs, frag.B...)
	}

	// Build the concatenation expression
	return GenFrag{
		B: bs,
		F: func() string {
			if len(partFrags) == 0 {
				return e(`""`)
			}

			// Use fmt.Sprintf for string concatenation
			var args []string
			for _, frag := range partFrags {
				var argStr string
				g.WithoutEmit(func() {
					argStr = frag.F()
				})
				args = append(args, argStr)
			}

			formatStr := `"` + strings.Repeat("%v", len(args)) + `"`
			if len(args) > 0 {
				return e("fmt.Sprintf(" + formatStr + ", " + strings.Join(args, ", ") + ")")
			}
			return e(`""`)
		},
	}
}

func (g *Generator) genBinaryExpr(expr *ast.BinaryExpr) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExpr(expr.X)
	c2 := g.genExpr(expr.Y)
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)

	// Handle nil-coalesce operator (??)
	if expr.Op.String() == "??" {
		tmpId := g.varCounter.Add(1)
		tmpVar := fmt.Sprintf("aglTmp%d", tmpId)
		// Only include c1.B in the before slice, c2.B will be emitted inside the if block
		var beforeBs []func() string
		beforeBs = append(beforeBs, c1.B...)
		beforeBs = append(beforeBs, func() (out string) {
			gPrefix := g.prefix
			out += e(gPrefix+tmpVar+" := ") + c1.F() + e("\n")
			out += e(gPrefix + "if " + tmpVar + " == nil {\n")
			// Emit c2.B inside the if block
			for _, b := range c2.B {
				out += g.incrPrefix(b)
			}
			out += e(gPrefix+"\t"+tmpVar+" = ") + c2.F() + e("\n")
			out += e(gPrefix + "}\n")
			return out
		})
		return GenFrag{F: func() string {
			return e(tmpVar)
		}, B: beforeBs}
	}

	return GenFrag{F: func() string {
		content1 := c1.F
		content2 := c2.F
		op := expr.Op.String()
		xT := types.Unwrap(g.env.GetType(expr.X))
		yT := types.Unwrap(g.env.GetType(expr.Y))
		if xT != nil && yT != nil {
			if op == "in" {
				t := g.env.GetType(expr.Y)
				t = types.Unwrap(t)
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
				case types.StructType:
				case types.FuncType:
					// This is a Sequence[T] (iter.Seq[T]), no wrapping needed
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
						out += content1() + e(".__EQL_"+rhsName+"(") + content2() + e(")")
						return out
					}

					// Map operators to method names
					opMap := map[string]string{
						"+":  "__ADD",
						"-":  "__SUB",
						"*":  "__MUL",
						"**": "__POW",
						"/":  "__QUO",
						"%":  "__REM",
					}
					if methodName, ok := opMap[op]; ok {
						if g.env.Get(lhsName+"."+methodName) != nil {
							return content1() + e("."+methodName+"_"+rhsName+"(") + content2() + e(")")
						}
					}
				}
			} else if TryCast[types.StructType](xT) {
				lhsName := xT.(types.StructType).Name
				rhsName := yT.GoStrType()
				if (op == "==" || op == "!=") && g.env.Get(lhsName+".__EQL") != nil {
					var out string
					if op == "!=" {
						out += e("!")
					}
					out += content1() + e(".__EQL_"+rhsName+"(") + content2() + e(")")
					return out
				}

				// Map operators to method names
				opMap := map[string]string{
					"+":  "__ADD",
					"-":  "__SUB",
					"*":  "__MUL",
					"**": "__POW",
					"/":  "__QUO",
					"%":  "__REM",
				}
				if methodName, ok := opMap[op]; ok {
					if g.env.Get(lhsName+"."+methodName) != nil {
						return content1() + e("."+methodName+"_"+rhsName+"(") + content2() + e(")")
					}
				}
			} else if TryCast[types.StructType](yT) {
				lhsName := xT.GoStrType()
				rhsName := yT.(types.StructType).Name
				if (op == "==" || op == "!=") && g.env.Get(lhsName+".__EQL") != nil {
					var out string
					if op == "!=" {
						out += e("!")
					}
					out += content1() + e(".__EQL_"+rhsName+"(") + content2() + e(")")
					return out
				}

				// Map operators to reverse method names
				opMap := map[string]string{
					"+":  "__RADD",
					"-":  "__RSUB",
					"*":  "__RMUL",
					"**": "__RPOW",
					"/":  "__RQUO",
					"%":  "__RREM",
				}
				if methodName, ok := opMap[op]; ok {
					if g.env.Get(rhsName+"."+methodName) != nil {
						return content2() + e("."+methodName+"_"+lhsName+"(") + content1() + e(")")
					}
				}
			}
		}
		if op == "**" {
			return e("AglPow(") + content1() + e(", ") + content2() + e(")")
		}
		return content1() + e(" "+op+" ") + content2()
	}, B: bs}
}

var emptyContent = func() string { return "" }

func (g *Generator) genCompositeLit(expr *ast.CompositeLit) GenFrag {
	e := EmitWith(g, expr)
	var bs []func() string
	c1 := g.genExprs(expr.Elts)
	c2 := GenFrag{F: emptyContent}
	if expr.Type != nil {
		c2 = g.genExpr(expr.Type)
	}
	bs = append(bs, c1.B...)
	bs = append(bs, c2.B...)
	return GenFrag{F: func() (out string) {
		if expr.Type != nil {
			out += c2.F()
			if out == "AglVoid{}" {
				return out
			}
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
	if g.asType {
		isType = true
	}
	replaced := types.ReplGenM(t, g.genMap)
	if replaced == nil {
		panic(fmt.Sprintf("ReplGenM returned nil for type %v with genMap %v", t, g.genMap))
	}
	tup := replaced.(types.TupleType)
	structName := tup.GoStr()
	var args []string
	for i, x := range expr.Values {
		xT := g.env.GetType2(x, g.fset)
		args = append(args, fmt.Sprintf("\tArg%d %s\n", i, types.ReplGenM(xT, g.genMap).GoStr()))
	}
	structStr := fmt.Sprintf("type %s struct {\n", structName)
	structStr += strings.Join(args, "")
	structStr += "}\n"

	// Generate String() method for tuple
	structStr += fmt.Sprintf("func (t %s) String() string {\n", structName)
	structStr += "\treturn fmt.Sprintf(\"("
	for i := range expr.Values {
		if i > 0 {
			structStr += ", "
		}
		structStr += "%v"
	}
	structStr += ")\""
	for i := range expr.Values {
		structStr += fmt.Sprintf(", t.Arg%d", i)
	}
	structStr += ")\n}\n"

	// Add fmt import since we're using fmt.Sprintf in the String() method
	g.imports[`"fmt"`] = &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`},
	}

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
	return GenFrag{F: func() (out string) {
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
	return GenFrag{F: func() (out string) {
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
	c1 := g.genStmts(stmt.List)
	return GenFrag{F: func() string {
		// Save the current shadowing state
		savedShadows := make(map[string]string)
		for k, v := range g.shadowedVars {
			savedShadows[k] = v
		}

		// Generate the block
		result := c1.F()

		// Restore the shadowing state
		g.shadowedVars = savedShadows

		return result
	}, B: c1.B}
}

func (g *Generator) genSpecs(specs []ast.Spec, tok token.Token) GenFrag {
	return GenFrag{F: func() (out string) {
		for _, spec := range specs {
			out += g.genSpec(spec, tok).F()
		}
		return
	}}
}

func (g *Generator) genSpec(s ast.Spec, tok token.Token) GenFrag {
	return GenFrag{F: func() (out string) {
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
			// Add AglNoop for variables without initializers when AllowUnused is enabled
			if g.allowUnused && spec.Values == nil {
				// Filter out blank identifiers
				var nonBlankNames []string
				for _, name := range spec.Names {
					if name.Name != "_" {
						nonBlankNames = append(nonBlankNames, name.Name)
					}
				}
				if len(nonBlankNames) > 0 {
					out += e(g.prefix + "AglNoop(" + strings.Join(nonBlankNames, ", ") + ")\n")
				}
			}
			return out
		case *ast.TypeSpec:
			e := EmitWith(g, spec)
			if v, ok := spec.Type.(*ast.EnumType); ok {
				out += e(g.prefix) + e(g.genEnumType(spec.Name.Name, v)) + e("\n")
			} else {
				// Check if this is a generic type declaration
				// Check if the type has generic type parameters
				hasTypeParams := spec.TypeParams != nil && len(spec.TypeParams.List) > 0

				// Also check if it's a struct type
				_, isStructType := spec.Type.(*ast.StructType)

				if hasTypeParams && isStructType && g.genMap == nil {
					// This is a generic struct type declaration - store it for later monomorphization
					typeT := g.env.Get(spec.Name.Name)
					if typeT != nil {
						if tt, ok := typeT.(types.TypeType); ok {
							typeT = tt.W
						}
						if _, ok := types.Unwrap(typeT).(types.StructType); ok {
							// Generic struct already stored in scanForGenericStructs
							return out
						}
					}
				}

				// When we have a genMap, use monomorphized name and skip type parameters
				if g.genMap != nil {
					out += e(g.prefix + "type " + spec.Name.Name)
					for _, k := range slices.Sorted(maps.Keys(g.genMap)) {
						v := g.genMap[k]
						out += e(fmt.Sprintf("_%v_%v", k, types.NormalizeTypeForMonomorphization(v.GoStr())))
					}
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
	return GenFrag{F: func() (out string) {
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
	body := func() string { return g.incrPrefix(cc1.F) }

	// Handle optional if condition for "for x in y if cond" syntax
	var ifCondFrag GenFrag
	if stmt.WhereCond != nil {
		ifCondFrag = g.genExpr(stmt.WhereCond)
	}

	if stmt.Init == nil && stmt.Post == nil && stmt.Cond != nil {
		if v, ok := stmt.Cond.(*ast.BinaryExpr); ok {
			if v.Op == token.IN {
				xT := g.env.GetType(v.X)
				yT := g.env.GetType(v.Y)

				// Check if v.Y is a call to .Enumerated() on a string
				if callExpr, ok := v.Y.(*ast.CallExpr); ok {
					if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
						if selExpr.Sel.Name == "Enumerated" {
							receiverType := g.env.GetType(selExpr.X)
							if _, ok := types.Unwrap(receiverType).(types.StringType); ok {
								// Optimize: for (i, c) in str.Enumerated() -> for i, c := range str
								c1 := g.genExpr(selExpr.X)
								return GenFrag{F: func() (out string) {
									if tup, ok := xT.(types.TupleType); ok && len(tup.Elts) == 2 {
										xTup := v.X.(*ast.TupleExpr)
										idx := g.genExpr(xTup.Values[0]).F
										val := g.genExpr(xTup.Values[1]).F
										out += e(g.prefix+"for ") + idx() + e(", ") + val() + e(" := range ") + c1.F() + e(" {\n")
									} else {
										c2 := g.genExpr(v.X)
										out += e(g.prefix+"for _, ") + c2.F() + e(" := range ") + c1.F() + e(" {\n")
									}

									// Add if condition check if present
									if stmt.WhereCond != nil {
										out += e(g.prefix+"\t") + e("if !(") + ifCondFrag.F() + e(") {\n")
										out += e(g.prefix+"\t\t") + e("continue\n")
										out += e(g.prefix+"\t") + e("}\n")
									}

									out += body()
									out += e(g.prefix + "}\n")
									return out
								}}
							}
						}
					}
				}

				c1 := g.genExpr(v.Y)
				c2 := g.genExpr(v.X)
				return GenFrag{F: func() (out string) {
					// Check MapType first before checking TupleType
					if _, ok := types.Unwrap(yT).(types.MapType); ok {
						xTup := v.X.(*ast.TupleExpr)
						key := g.genExpr(xTup.Values[0]).F
						val := g.genExpr(xTup.Values[1]).F
						y := c1.F
						out += e(g.prefix+"for ") + key() + e(", ") + val() + e(" := range ") + y() + e(" {\n")
					} else if tup, ok := xT.(types.TupleType); ok {
						varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))
						// For sets, range only on keys (not index, key)
						if TryCast[types.SetType](types.Unwrap(yT)) {
							out += e(g.prefix+"for "+varName+" := range (") + c1.F() + e(") {\n")
						} else {
							out += e(g.prefix+"for _, "+varName+" := range ") + c1.F() + e(" {\n")
						}
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
						switch vv := types.Unwrap(yT).(type) {
						case types.ArrayType:
							out += e(g.prefix+"for _, ") + c2.F() + e(" := range ") + c1.F() + e(" {\n")
						case types.SetType:
							out += e(g.prefix+"for ") + c2.F() + e(" := range (") + c1.F() + e(").Iter() {\n")
						case types.RangeType:
							out += e(g.prefix + "for ")
							c2V := c2.F()
							op := utils.Ternary(c2V == "_", "=", ":=")
							out += c2V
							out += e(" "+op+" range ") + c1.F() + e(".Iter()") + e(" {\n")
						case types.StringType:
							out += e(g.prefix+"for _, ") + c2.F() + e(" := range ") + c1.F() + e(" {\n")
						case types.FuncType:
							// Handle iter.Seq-style iterators (unwrapped Sequence types)
							out += e(g.prefix + "for ")
							c2V := c2.F()
							op := utils.Ternary(c2V == "_", "=", ":=")
							out += c2V
							out += e(" "+op+" range ") + c1.F() + e(" {\n")
						case types.StructType:
							if vv.Name == "Sequence" {
								out += e(g.prefix + "for ")
								c2V := c2.F()
								op := utils.Ternary(c2V == "_", "=", ":=")
								out += c2V
								out += e(" "+op+" range ") + c1.F() + e(" {\n")
							} else {
								panic("")
							}
						default:
							panic(fmt.Sprintf("%v", to(v.X)))
						}
					}

					// Add if condition check if present
					if stmt.WhereCond != nil {
						out += e(g.prefix+"\t") + e("if !(") + ifCondFrag.F() + e(") {\n")
						out += e(g.prefix+"\t\t") + e("continue\n")
						out += e(g.prefix+"\t") + e("}\n")
					}

					out += body()
					out += e(g.prefix + "}\n")
					return out
				}}
			}
		}
	}
	c1 := GenFrag{F: emptyContent}
	if stmt.Cond != nil {
		c1 = g.genExpr(stmt.Cond)
	}
	tmp := func() string {
		var els []func() string
		if stmt.Init != nil {
			init := func() (out string) {
				g.WithInlineStmt(func() {
					out = g.genStmt(stmt.Init).F()
				})
				return out
			}
			els = append(els, init)
		}
		if stmt.Cond != nil {
			els = append(els, c1.F)
		}
		if stmt.Post != nil {
			post := func() (out string) {
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
	return GenFrag{F: func() (out string) {
		out += e(g.prefix+"for ") + tmp() + e("{\n")
		out += body()
		out += e(g.prefix + "}\n")
		return out
	}}
}

func (g *Generator) genRangeStmt(stmt *ast.RangeStmt) GenFrag {
	c1 := g.genExpr(stmt.X)
	c2 := GenFrag{F: emptyContent}
	c3 := GenFrag{F: emptyContent}

	// Check if ranging over a map with tuple destructuring syntax: for (k, v) in m
	xType := g.env.GetType(stmt.X)
	if v, ok := xType.(types.MutType); ok {
		xType = v.W
	}
	isMap := TryCast[types.MapType](xType)

	// Check if Key is a TupleExpr (indicating tuple destructuring syntax)
	var keyName, valueName string
	if tupleKey, ok := stmt.Key.(*ast.TupleExpr); ok && isMap && len(tupleKey.Values) == 2 {
		// Extract the names from the tuple
		if id0, ok := tupleKey.Values[0].(*ast.Ident); ok {
			keyName = id0.Name
		}
		if id1, ok := tupleKey.Values[1].(*ast.Ident); ok {
			valueName = id1.Name
		}
	} else {
		// Normal case: separate key and value
		if stmt.Key != nil {
			c2 = g.genExpr(stmt.Key)
		}
		if stmt.Value != nil {
			c3 = g.genExpr(stmt.Value)
		}
	}

	c4 := g.genStmt(stmt.Body)

	// Handle optional condition
	var condFrag GenFrag
	if stmt.Cond != nil {
		condFrag = g.genExpr(stmt.Cond)
	}

	return GenFrag{F: func() string {
		var out string
		e := EmitWith(g, stmt)
		content3 := func() string {
			isCompositeLit := TryCast[*ast.CompositeLit](stmt.X)
			var out string
			if isCompositeLit {
				out += e("(")
			}
			out += c1.F()
			if isCompositeLit {
				out += e(")")
			}
			return out
		}
		op := stmt.Tok

		// If we have keyName and valueName from tuple destructuring, use them directly
		if keyName != "" && valueName != "" {
			out += e(g.prefix+"for ") + e(keyName) + e(", ") + e(valueName) + e(" "+op.String()+" range ") + content3() + e(" {\n")
		} else if stmt.Key == nil && stmt.Value == nil {
			out += e(g.prefix+"for range ") + content3() + e(" {\n")
		} else if stmt.Value == nil {
			out += e(g.prefix+"for ") + c2.F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		} else {
			out += e(g.prefix+"for ") + c2.F() + e(", ") + c3.F() + e(" "+op.String()+" range ") + content3() + e(" {\n")
		}

		// Add condition check if present
		if stmt.Cond != nil {
			out += e(g.prefix+"\t") + e("if !(") + condFrag.F() + e(") {\n")
			out += e(g.prefix+"\t\t") + e("continue\n")
			out += e(g.prefix+"\t") + e("}\n")
		}

		out += g.incrPrefix(c4.F)
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
	var resultFrag GenFrag

	// Check if the result is an IfExpr with explicit returns in all branches
	// If so, generate the if statement directly without the outer "return" keyword
	if ifExpr, ok := stmt.Result.(*ast.IfExpr); ok && ifExprHasExplicitReturns(ifExpr) {
		// Generate the if expression as a statement (it has its own returns)
		return g.genStmt(&ast.ExprStmt{X: ifExpr})
	}

	if stmt.Result != nil {
		resultFrag = g.genExpr(stmt.Result)
		bs = append(bs, resultFrag.B...)
	}
	return GenFrag{F: func() (out string) {
		out += e(g.prefix + "return")
		if stmt.Result != nil {
			out += e(" ") + resultFrag.F()
		}
		return out + e("\n")
	}, B: bs}
}

func (g *Generator) genExprStmt(stmt *ast.ExprStmt) GenFrag {
	// Check if this is an assert/assertEq call in release mode
	if g.releaseMode {
		if callExpr, ok := stmt.X.(*ast.CallExpr); ok {
			if ident, ok := callExpr.Fun.(*ast.Ident); ok {
				if ident.Name == "assert" || ident.Name == "assertEq" {
					// Skip this statement entirely in release mode
					return GenFrag{F: func() string { return "" }}
				}
			}
		}
	}

	e := EmitWith(g, stmt)
	xFrag := g.genExpr(stmt.X)
	bs := xFrag.B
	return GenFrag{F: func() (out string) {
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		out += xFrag.F()
		if !g.inlineStmt {
			out += e("\n")
		}
		return out
	}, B: bs}
}

func (g *Generator) genAssignStmt(stmt *ast.AssignStmt) GenFrag {
	e := EmitWith(g, stmt)

	// Detect variable shadowing: mut x := x
	// When a mutable variable is being assigned from an identifier with the same name,
	// we need to generate x_ := x to avoid Go's "no new variables" error
	var shadowingVar string
	var shadowingRHSNode *ast.Ident
	if stmt.Tok == token.DEFINE && len(stmt.Lhs) == 1 && len(stmt.Rhs) == 1 {
		if lhsIdent, ok := stmt.Lhs[0].(*ast.Ident); ok {
			if rhsIdent, ok := stmt.Rhs[0].(*ast.Ident); ok {
				if lhsIdent.Name == rhsIdent.Name && lhsIdent.Mutable.IsValid() {
					// This is a shadowing case: remember to rename the variable
					// Exclude the RHS node from shadowing
					shadowingVar = lhsIdent.Name
					shadowingRHSNode = rhsIdent
				}
			}
		}
	}

	// Generate RHS first (before applying shadowing)
	rhsT := g.env.GetType(stmt.Rhs[0])
	if v, ok := rhsT.(types.CustomType); ok {
		rhsT = v.W
	}

	lhs := emptyContent
	var after string
	if len(stmt.Rhs) == 1 && TryCast[types.EnumType](rhsT) {
		enumT := rhsT.(types.EnumType)
		if len(stmt.Lhs) == 1 {
			// Check if LHS is a TupleExpr (enum destructuring with parentheses)
			if tupleExpr, ok := stmt.Lhs[0].(*ast.TupleExpr); ok {
				// Generate enum destructuring: extract identifiers from TupleExpr
				varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				lhs = func() string { return e(varName) }
				var names, exprs []string
				for i, ident := range tupleExpr.Values {
					name := ident.(*ast.Ident).Name
					names = append(names, name)
					exprs = append(exprs, fmt.Sprintf("%s.%s_%d", varName, enumT.SubTyp, i))
				}
				after += strings.Join(names, ", ") + " := " + strings.Join(exprs, ", ")
			} else {
				// Single LHS, not a TupleExpr - assign the whole enum
				c1 := g.genExprs(stmt.Lhs)
				lhs = c1.F
			}
		} else {
			varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
			lhs = func() string { return e(varName) }
			var names, exprs []string
			for i, x := range stmt.Lhs {
				names = append(names, x.(*ast.Ident).Name)
				exprs = append(exprs, fmt.Sprintf("%s.%s_%d", varName, enumT.SubTyp, i))
			}
			after += strings.Join(names, ", ") + " := " + strings.Join(exprs, ", ")
		}
	} else if len(stmt.Rhs) == 1 && TryCast[types.TupleType](rhsT) {
		// Check if this is a map index expression with value+ok pattern (e.g., tup, ok := map['key'])
		// In this case, don't destructure the tuple even though rhsT is a TupleType
		isMapIndexWithOk := false
		if len(stmt.Lhs) == 2 {
			if indexExpr, ok := stmt.Rhs[0].(*ast.IndexExpr); ok {
				mapType := types.Unwrap(g.env.GetType(indexExpr.X))
				if _, isMap := mapType.(types.MapType); isMap {
					isMapIndexWithOk = true
				}
			}
		}

		if isMapIndexWithOk {
			// Don't destructure - this is value, ok := map['key'] where value is a tuple
			c1 := g.genExprs(stmt.Lhs)
			lhs = c1.F
		} else if len(stmt.Lhs) == 1 {
			// Check if LHS is a TupleExpr (tuple destructuring with parentheses)
			if tupleExpr, ok := stmt.Lhs[0].(*ast.TupleExpr); ok {
				// Generate tuple destructuring: extract identifiers from TupleExpr
				varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				lhs = func() string { return e(varName) }
				var names, exprs []string
				for i, ident := range tupleExpr.Values {
					name := ident.(*ast.Ident).Name
					names = append(names, name)
					exprs = append(exprs, fmt.Sprintf("%s.Arg%d", varName, i))
				}
				// Use the same operator as the original statement
				op := stmt.Tok.String()
				after += fmt.Sprintf("%s %s %s", strings.Join(names, ", "), op, strings.Join(exprs, ", "))
				if g.allowUnused {
					// Add AglNoop for non-blank identifiers
					nonBlankNames := Filter(names, func(s string) bool { return s != "_" })
					if len(nonBlankNames) > 0 {
						after += "\n\tAglNoop(" + strings.Join(nonBlankNames, ", ") + ")"
					}
				}
			} else {
				// Single LHS, not a TupleExpr - assign the whole tuple
				c1 := g.genExprs(stmt.Lhs)
				lhs = c1.F
			}
		} else {
			if v, ok := rhsT.(types.TupleType); ok && v.KeepRaw {
				c1 := g.genExprs(stmt.Lhs)
				lhs = c1.F
			} else {
				varName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				lhs = func() string { return e(varName) }
				rhs := stmt.Rhs[0]
				var names, exprs []string
				for i := range g.env.GetType(rhs).(types.TupleType).Elts {
					name := stmt.Lhs[i].(*ast.Ident).Name
					names = append(names, name)
					exprs = append(exprs, fmt.Sprintf("%s.Arg%d", varName, i))
				}
				// Use the same operator as the original statement
				op := stmt.Tok.String()
				after += fmt.Sprintf("%s %s %s", strings.Join(names, ", "), op, strings.Join(exprs, ", "))
				if g.allowUnused {
					// Add AglNoop for non-blank identifiers
					nonBlankNames := Filter(names, func(s string) bool { return s != "_" })
					if len(nonBlankNames) > 0 {
						after += "\n\tAglNoop(" + strings.Join(nonBlankNames, ", ") + ")"
					}
				}
			}
		}
	} else if len(stmt.Lhs) == 1 && TryCast[*ast.IndexExpr](stmt.Lhs[0]) && TryCast[*ast.MapType](stmt.Lhs[0].(*ast.IndexExpr).X) {
		c1 := g.genExprs(stmt.Lhs)
		lhs = c1.F
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
			lhs = c1.F
		}
	}
	// Handle nil-coalesce operator specially in assignments
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) == 1 {
		if binExpr, ok := stmt.Rhs[0].(*ast.BinaryExpr); ok && binExpr.Op.String() == "??" {
			// Generate: tmpVar := leftExpr; if tmpVar == nil { tmpVar = rightExpr }; lhs := tmpVar
			c1 := g.genExpr(binExpr.X)
			c2 := g.genExpr(binExpr.Y)
			var bs []func() string
			bs = append(bs, c1.B...)
			bs = append(bs, c2.B...)
			tmpId := g.varCounter.Add(1)
			tmpVar := fmt.Sprintf("aglTmp%d", tmpId)
			return GenFrag{F: func() string {
				var out string
				if !g.inlineStmt {
					out += e(g.prefix)
				}
				out += e(tmpVar+" := ") + c1.F() + e("\n")
				out += e(g.prefix + "if " + tmpVar + " == nil {\n")
				out += e(g.prefix+"\t"+tmpVar+" = ") + c2.F() + e("\n")
				out += e(g.prefix + "}\n")
				out += e(g.prefix) + lhs() + e(" "+stmt.Tok.String()+" "+tmpVar+"\n")
				return out
			}, B: bs}
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
	op := stmt.Tok
	lhsT := types.Unwrap(g.env.GetType(stmt.Lhs[0]))
	assignOpsFrag := GenFrag{}
	if TryCast[types.StructType](lhsT) {
		m := map[token.Token]token.Token{
			token.ADD_ASSIGN:     token.ADD,
			token.SUB_ASSIGN:     token.SUB,
			token.MUL_ASSIGN:     token.MUL,
			token.QUO_ASSIGN:     token.QUO,
			token.REM_ASSIGN:     token.REM,
			token.AND_ASSIGN:     token.AND,
			token.OR_ASSIGN:      token.OR,
			token.XOR_ASSIGN:     token.XOR,
			token.SHL_ASSIGN:     token.SHL,
			token.SHR_ASSIGN:     token.SHR,
			token.AND_NOT_ASSIGN: token.AND_NOT,
		}
		if v, ok := m[op]; ok {
			assignOpsFrag = g.genBinaryExpr(&ast.BinaryExpr{X: stmt.Lhs[0], Op: v, Y: stmt.Rhs[0]})
		}
	}
	var bs []func() string
	bs = append(bs, content2.B...)
	return GenFrag{F: func() (out string) {
		// Apply shadowing, but RHS uses old shadow level
		if shadowingVar != "" {
			oldShadow, hadOldShadow := g.shadowedVars[shadowingVar]
			newShadow := shadowingVar + "_"
			if hadOldShadow {
				newShadow = oldShadow + "_"
				// RHS should use the old shadow level
				g.shadowingExclusionOldValue = oldShadow
			} else {
				// RHS should use the original name (no shadow)
				g.shadowingExclusionOldValue = shadowingVar
			}
			g.shadowingExclusionNode = shadowingRHSNode
			// Update to new shadowing for LHS
			g.shadowedVars[shadowingVar] = newShadow
		}
		if !g.inlineStmt {
			out += e(g.prefix)
		}
		if assignOpsFrag.F != nil {
			result := out + lhs() + e(" = ") + assignOpsFrag.F() + e("\n")
			g.shadowingExclusionNode = nil
			g.shadowingExclusionOldValue = ""
			return result
		}
		// For tuple unpacking (when after != ""), always use := for the temp variable
		actualOp := op.String()
		if after != "" {
			actualOp = ":="
		}

		out += lhs() + e(" "+actualOp+" ") + content2.F()
		g.shadowingExclusionNode = nil
		g.shadowingExclusionOldValue = ""
		if !g.inlineStmt && g.allowUnused {
			// Check if all lhs are blank identifiers
			allBlank := true
			for _, l := range stmt.Lhs {
				if ident, ok := l.(*ast.Ident); !ok || ident.Name != "_" {
					allBlank = false
					break
				}
			}
			if !allBlank {
				out += e("\n"+g.prefix+"AglNoop(") + lhs() + e(")")
			}
		}
		if after != "" {
			out += e("\n")
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
	c4 := GenFrag{F: emptyContent}
	if stmt.Else != nil {
		c4 = g.genStmt(stmt.Else)
	}
	return GenFrag{F: func() (out string) {
		gPrefix := g.prefix
		lhs := c1.F
		rhs := func() string { return g.wrapIfNative(e, rhs0, c2.F) }
		varName := fmt.Sprintf("aglTmp%d", g.varCounter.Add(1))

		// Helper function to generate else clause (shared by all branches)
		genElse := func() {
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
						out += g.incrPrefix(c4.F)
						out += e(gPrefix + "}")
					}
				} else {
					out += e(gPrefix + "} else {\n")
					out += g.incrPrefix(c4.F)
					out += e(gPrefix + "}")
				}
			} else {
				out += e(gPrefix + "}")
			}
		}

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
			// Handle enum pattern matching: if let MyEnum.SomeProp(x, y) := someEnumValue
			if callExpr, ok := lhs0.(*ast.CallExpr); ok {
				if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					rhsType := g.env.GetType(rhs0)
					// Unwrap MutType if present
					if mutType, ok := rhsType.(types.MutType); ok {
						rhsType = mutType.W
					}
					if enumType, ok := rhsType.(types.EnumType); ok {
						fieldName := selExpr.Sel.Name

						out += e(varName+" := ") + rhs() + e("\n")
						out += e(gPrefix + "if " + varName + ".Tag == " + enumType.Name + "_" + fieldName + " {\n")

						// Extract the values from the enum
						for j, arg := range callExpr.Args {
							if argIdent, ok := arg.(*ast.Ident); ok {
								if argIdent.Name == "_" {
									out += e(gPrefix + "\t_ = " + varName + "." + fieldName + "_" + strconv.Itoa(j) + "\n")
								} else {
									out += e(gPrefix+"\t") + g.genExpr(arg).F() + e(" := "+varName+"."+fieldName+"_"+strconv.Itoa(j)+"\n")
									if g.allowUnused {
										out += e(gPrefix+"\tAglNoop(") + g.genExpr(arg).F() + e(")\n")
									}
								}
							}
						}

						g.inlineStmt = false
						out += g.incrPrefix(c3.F)
						genElse()
						return out
					}
				}
			}
			panic("")
		}
		if _, ok := ass.Rhs[0].(*ast.TypeAssertExpr); ok {
			out += e("if ") + lhs() + e(", ok := ") + rhs() + e("; ok {\n")
			if g.allowUnused {
				out += e(gPrefix+"\tAglNoop(") + lhs() + e(")\n")
			}
			g.inlineStmt = false
		} else {
			out += e("if "+varName+" := ") + rhs() + e("; "+cond+" {\n")

			// Handle tuple destructuring for multiple LHS values
			if len(ass.Lhs) > 1 {
				// Generate tuple destructuring: aglVar1 := tmp.Unwrap()
				tupleVarName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				out += e(gPrefix + "\t" + tupleVarName + " := " + varName + "." + unwrapFn + "()\n")

				// Generate LHS names - directly get identifier names
				var lhsNames []string
				for _, lhsExpr := range ass.Lhs {
					if ident, ok := lhsExpr.(*ast.Ident); ok {
						lhsNames = append(lhsNames, ident.Name)
					} else {
						// Fallback to generating the expression
						lhsNames = append(lhsNames, g.genExpr(lhsExpr).F())
					}
				}

				// Generate individual assignments: a, b := aglVar1.Arg0, aglVar1.Arg1
				var rhsParts []string
				for i := range ass.Lhs {
					rhsParts = append(rhsParts, tupleVarName+".Arg"+strconv.Itoa(i))
				}
				out += e(gPrefix + "\t" + strings.Join(lhsNames, ", ") + " := " + strings.Join(rhsParts, ", ") + "\n")

				if g.allowUnused {
					for _, name := range lhsNames {
						out += e(gPrefix + "\tAglNoop(" + name + ")\n")
					}
				}
			} else {
				out += e(gPrefix+"\t") + lhs() + e(" := "+varName+"."+unwrapFn+"()\n")
				if g.allowUnused {
					out += e(gPrefix+"\tAglNoop(") + lhs() + e(")\n")
				}
			}
			g.inlineStmt = false
		}
		out += g.incrPrefix(c3.F)
		genElse()
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
		lhs := c1.F
		rhs := func() string { return g.wrapIfNative(e, rhs0, c2.F) }
		body := func() string { return g.incrPrefix(c3.F) }
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
			// Handle enum pattern matching: guard let MyEnum.SomeProp(x, y) := someEnumValue else { ... }
			if callExpr, ok := lhs0.(*ast.CallExpr); ok {
				if selExpr, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
					rhsType := g.env.GetType(rhs0)
					// Unwrap MutType if present
					if mutType, ok := rhsType.(types.MutType); ok {
						rhsType = mutType.W
					}
					if enumType, ok := rhsType.(types.EnumType); ok {
						fieldName := selExpr.Sel.Name
						// Find field index
						var fieldIdx int
						for i, f := range enumType.Fields {
							if f.Name == fieldName {
								fieldIdx = i
								break
							}
						}

						out += e(gPrefix+varName+" := ") + rhs() + e("\n")
						out += e(gPrefix + "if " + varName + ".Tag != " + enumType.Name + "_" + fieldName + " {\n")
						out += body()
						out += e(gPrefix + "}\n")

						// Extract the values from the enum
						for j, arg := range callExpr.Args {
							if argIdent, ok := arg.(*ast.Ident); ok {
								if argIdent.Name == "_" {
									out += e(gPrefix + "_ = " + varName + "." + enumType.Fields[fieldIdx].Name + "_" + strconv.Itoa(j) + "\n")
								} else {
									out += e(gPrefix) + g.genExpr(arg).F() + e(" := "+varName+"."+enumType.Fields[fieldIdx].Name+"_"+strconv.Itoa(j)+"\n")
								}
							}
						}

						if g.allowUnused {
							for _, arg := range callExpr.Args {
								if argIdent, ok := arg.(*ast.Ident); ok && argIdent.Name != "_" {
									out += e(gPrefix+"AglNoop(") + g.genExpr(arg).F() + e(")\n")
								}
							}
						}
						return out
					}
				}
			}
			panic("")
		}
		if _, ok := rhs0.(*ast.TypeAssertExpr); ok {
			out += e(gPrefix) + lhs() + e(", "+varName+" := ") + rhs() + e("\n")
			out += e(gPrefix + "if !" + varName + " {\n")
			out += body()
			out += e(gPrefix + "}\n")
			if g.allowUnused {
				out += e(gPrefix+"AglNoop(") + lhs() + e(")\n")
			}
		} else {
			out += e(gPrefix+varName+" := ") + rhs() + e("\n")
			out += e(gPrefix + "if " + cond + " {\n")
			out += body()
			out += e(gPrefix + "}\n")

			// Handle tuple destructuring for multiple LHS values
			if len(ass.Lhs) > 1 {
				// Generate tuple destructuring: aglVar1 := tmp.Unwrap()
				tupleVarName := fmt.Sprintf("aglVar%d", g.varCounter.Add(1))
				out += e(gPrefix + tupleVarName + " := " + varName + "." + unwrapFn + "()\n")

				// Generate LHS names - directly get identifier names
				var lhsNames []string
				for _, lhsExpr := range ass.Lhs {
					if ident, ok := lhsExpr.(*ast.Ident); ok {
						lhsNames = append(lhsNames, ident.Name)
					} else {
						// Fallback to generating the expression
						lhsNames = append(lhsNames, g.genExpr(lhsExpr).F())
					}
				}

				// Generate individual assignments: a, b := aglVar1.Arg0, aglVar1.Arg1
				var rhsParts []string
				for i := range ass.Lhs {
					rhsParts = append(rhsParts, tupleVarName+".Arg"+strconv.Itoa(i))
				}
				out += e(gPrefix + strings.Join(lhsNames, ", ") + " := " + strings.Join(rhsParts, ", ") + "\n")

				if g.allowUnused {
					for _, name := range lhsNames {
						out += e(gPrefix + "AglNoop(" + name + ")\n")
					}
				}
			} else {
				out += e(gPrefix) + lhs() + e(" := "+varName+"."+unwrapFn+"()\n")
				if g.allowUnused {
					out += e(gPrefix+"AglNoop(") + lhs() + e(")\n")
				}
			}
		}
		return out
	}}
}

func (g *Generator) genIfExpr(stmt *ast.IfExpr) GenFrag {
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
	c1 := GenFrag{F: emptyContent}
	c3 := GenFrag{F: emptyContent}
	if stmt.Init != nil {
		c3 = g.genStmt(stmt.Init)
	}
	genAssignStmt := func(last ast.Stmt) *ast.AssignStmt {
		return &ast.AssignStmt{Lhs: []ast.Expr{&ast.Ident{Name: varName}}, Rhs: []ast.Expr{last.(*ast.ExprStmt).X}, Tok: token.ASSIGN}
	}
	// Check if this if-expr has explicit returns - if so, don't convert to assignments
	hasExplicitReturns := ifExprHasExplicitReturns(stmt)
	// Also check if the last statement is actually a return
	if hasTyp && len(stmt.Body.List) > 0 {
		if _, isReturn := stmt.Body.List[len(stmt.Body.List)-1].(*ast.ReturnStmt); isReturn {
			hasExplicitReturns = true
		}
	}
	if hasTyp && !hasExplicitReturns && len(stmt.Body.List) > 0 {
		last := Must(Last(stmt.Body.List))
		// Only convert if the last statement is actually an ExprStmt
		if _, ok := last.(*ast.ExprStmt); ok {
			stmt.Body.List[len(stmt.Body.List)-1] = genAssignStmt(last)
		}
	}
	g.ifVarName = ""
	c2 := g.genStmt(stmt.Body)
	g.ifVarName = varName
	if stmt.Else != nil {
		if hasTyp && !hasExplicitReturns {
			switch v := stmt.Else.(type) {
			case *ast.BlockStmt:
				if len(v.List) > 0 {
					last := Must(Last(v.List))
					// Only convert if the last statement is actually an ExprStmt
					if _, ok := last.(*ast.ExprStmt); ok {
						v.List[len(v.List)-1] = genAssignStmt(last)
					}
				}
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
	g.ifVarName = ""
	bs = append(bs, cond.B...)
	bs = append(bs, c3.B...)
	tmp := func() (out string) {
		gPrefix := g.prefix
		if hasTyp && isFirst && !hasExplicitReturns {
			out += e(gPrefix + "var " + varName + " " + ifT.GoStrType() + "\n")
			out += e(gPrefix)
		}
		// For explicit returns, don't add prefix - it's already in g.prefix from parent context
		// For non-first if-expressions (else if), the prefix will be added by the parent
		out += e("if ")
		if stmt.Init != nil {
			g.WithInlineStmt(func() {
				out += c3.F() + e("; ")
			})
		}
		out += cond.F() + e(" {\n")
		g.inlineStmt = false
		out += g.incrPrefix(c2.F)
		if stmt.Else != nil {
			var isIf bool
			out += e(gPrefix + "} else ")
			switch v := stmt.Else.(type) {
			case *ast.ExprStmt:
				switch v.X.(type) {
				case *ast.IfExpr, *ast.IfLetExpr:
					isIf = true
				}
			}
			if isIf {
				g.WithInlineStmt(func() {
					out += c1.F()
				})
			} else {
				out += e("{\n")
				out += g.incrPrefix(c1.F)
				out += e(gPrefix + "}")
			}
		} else {
			out += e(gPrefix + "}")
		}
		if hasTyp && isFirst && !hasExplicitReturns {
			out += e("\n")
		}
		return out
	}
	if hasTyp && isFirst && !hasExplicitReturns {
		bs = append(bs, tmp)
		tmp = func() string {
			return e("AglIdentity(" + varName + ")")
		}
	}
	return GenFrag{F: tmp, B: bs}
}

func (g *Generator) genGuardStmt(stmt *ast.GuardStmt) GenFrag {
	e := EmitWith(g, stmt)
	c1 := g.genExpr(stmt.Cond)
	c2 := g.genStmt(stmt.Body)
	return GenFrag{F: func() (out string) {
		cond := c1.F
		gPrefix := g.prefix
		out += e(gPrefix+"if !(") + cond() + e(") {\n")
		out += g.incrPrefix(c2.F)
		out += e(gPrefix + "}\n")
		return out
	}}
}

func (g *Generator) genDecls(f *ast.File) GenFrag {
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
	return GenFrag{F: func() (out string) {
		for _, decl := range decls {
			out += decl()
		}
		return out
	}}
}

func (g *Generator) genFuncDecl(decl *ast.FuncDecl) GenFrag {
	e := EmitWith(g, decl)
	g.returnType = g.env.GetType(decl).(types.FuncType).Return
	var bs []func() string
	recv := emptyContent
	typeParamsFn := emptyContent
	var name, paramsStr, resultStr string
	if decl.Recv != nil {
		if len(decl.Recv.List) >= 1 {
			if tmp1, ok := decl.Recv.List[0].Type.(*ast.IndexExpr); ok {
				if tmp2, ok := tmp1.X.(*ast.SelectorExpr); ok {
					if tmp2.Sel.Name == "Vec" {
						fnName := fmt.Sprintf("agl1.Vec.%s", decl.Name.Name)
						for _, pp := range decl.Type.Params.List {
							for _, n := range pp.Names {
								if n.Label != nil {
									fnName += fmt.Sprintf("_%s", n.Label.Name)
								}
							}
						}
						g.setExtensionDecl(fnName, decl)
						return GenFrag{F: emptyContent}
					}
				}
			} else if tmp2, ok := decl.Recv.List[0].Type.(*ast.SelectorExpr); ok {
				if tmp2.Sel.Name == "String" {
					fnName := fmt.Sprintf("agl1.String.%s", decl.Name.Name)
					g.setExtensionDecl(fnName, decl)
					return GenFrag{F: emptyContent}
				}
			}
		}
		recv = func() (out string) {
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
		return GenFrag{F: emptyContent}
	}
	if decl.Name != nil {
		fnName := decl.Name.Name
		if newName, ok := overloadMapping[fnName]; ok {
			fnName = newName + "_" + types.Unwrap(fnT.(types.FuncType).Params[0]).GoStrType()
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
		resultType := types.ReplGenM(g.env.GetType(result), g.genMap)
		// Check if this is a user-defined struct that should use monomorphized name
		resultStr = g.typeToGoStr(resultType)
		resultStr = utils.PrefixIf(resultStr, " ")
	}
	if g.genMap != nil {
		typeParamsFn = emptyContent
		for _, k := range slices.Sorted(maps.Keys(g.genMap)) {
			v := g.genMap[k]
			name += fmt.Sprintf("_%v_%v", k, types.NormalizeTypeForMonomorphization(v.GoStr()))
		}
	}
	c1 := GenFrag{F: emptyContent}
	implicitReturn := false
	if decl.Body != nil {
		// Check if we need an implicit return: function has return type, body has single non-empty expression
		hasReturnType := decl.Type.Result != nil && !TryCast[types.VoidType](g.env.GetType(decl.Type.Result))
		if hasReturnType && len(decl.Body.List) > 0 {
			// Find the first non-empty statement
			var firstNonEmpty ast.Stmt
			nonEmptyCount := 0
			for _, stmt := range decl.Body.List {
				if _, isEmpty := stmt.(*ast.EmptyStmt); !isEmpty {
					if nonEmptyCount == 0 {
						firstNonEmpty = stmt
					}
					nonEmptyCount++
				}
			}
			// If there's exactly one non-empty statement and it's an expression, add implicit return
			if nonEmptyCount == 1 {
				// Check if it's already a return statement - if so, don't add implicit return
				if _, ok := firstNonEmpty.(*ast.ReturnStmt); ok {
					// Already has explicit return, don't add implicit return
					implicitReturn = false
				} else if exprStmt, ok := firstNonEmpty.(*ast.ExprStmt); ok {
					// Check if this is a match expression with explicit returns
					if matchExpr, ok := exprStmt.X.(*ast.MatchExpr); ok && matchHasExplicitReturns(matchExpr) {
						// Don't add implicit return - match has explicit returns
						implicitReturn = false
					} else if ifExpr, ok := exprStmt.X.(*ast.IfExpr); ok && ifExprHasExplicitReturns(ifExpr) {
						// Don't add implicit return - if has explicit returns in all branches
						implicitReturn = false
					} else {
						// Single expression in function with return type - implicit return
						implicitReturn = true
						// Generate the expression first
						exprFrag := g.genExpr(exprStmt.X)
						// Create a GenFrag that executes B functions then returns the expression
						c1 = GenFrag{
							F: func() string {
								var out string
								// Execute B functions first (like genStmts does)
								for _, b := range exprFrag.B {
									out += b()
								}
								// Then add the return statement (like genReturnStmt does)
								out += e(g.prefix+"return ") + exprFrag.F() + e("\n")
								return out
							},
						}
					}
				}
			}
		}
		if !implicitReturn {
			c1 = g.genStmt(decl.Body)
		}
	}
	// Note: Don't append c1.B to bs here - B functions are executed inside the function body below
	return GenFrag{F: func() (out string) {
		out += e("func")
		out += recv()
		out += e(fmt.Sprintf("%s%s(%s)%s {\n", name, typeParamsFn(), paramsStr, resultStr))
		if decl.Body != nil {
			out += g.incrPrefix(c1.F)
		}
		out += e("}\n")
		return out
	}, B: bs}
}

func (g *Generator) addExtension(extName string, ext ExtensionTest) {
	tmp := g.extensions[extName]
	if tmp.gen == nil {
		tmp.gen = make(map[string]ExtensionTest)
	}
	tmp.gen[ext.raw.String()+"_"+ext.concrete.String()] = ext
	g.extensions[extName] = tmp
}

func (g *Generator) setExtensionDecl(fnName string, decl *ast.FuncDecl) {
	ext := g.extensions[fnName]
	ext.decl = decl
	g.extensions[fnName] = ext
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

func (g *Generator) generateZipFunc(name string, fnT types.FuncType, args []ast.Expr) func() string {
	return func() string {
		// Extract element types from the arguments
		var paramNames []string
		var paramTypes []string
		var elemTypes []types.Type

		for i, arg := range args {
			paramName := string(rune('a' + i))
			if i >= 26 {
				// Handle more than 26 parameters by using a1, a2, etc.
				paramName = fmt.Sprintf("arg%d", i)
			}
			paramNames = append(paramNames, paramName)

			argType := types.Unwrap(g.env.GetType(arg))
			if arrT, ok := argType.(types.ArrayType); ok {
				elemTypes = append(elemTypes, arrT.Elt)
				paramTypes = append(paramTypes, arrT.GoStr())
			}
		}

		// Build the tuple return type
		tupleType := types.TupleType{Elts: elemTypes}
		returnType := types.ArrayType{Elt: tupleType}

		// Generate tuple struct definition if not already generated
		structName := tupleType.GoStr()
		if g.tupleStructs[structName] == "" {
			structStr := fmt.Sprintf("type %s struct {\n", structName)
			for i, elemType := range elemTypes {
				structStr += fmt.Sprintf("\tArg%d %s\n", i, elemType.GoStr())
			}
			structStr += "}\n"

			// Generate String() method for tuple
			structStr += fmt.Sprintf("func (t %s) String() string {\n", structName)
			structStr += "\treturn fmt.Sprintf(\"("
			for i := range elemTypes {
				if i > 0 {
					structStr += ", "
				}
				structStr += "%v"
			}
			structStr += ")\""
			for i := range elemTypes {
				structStr += fmt.Sprintf(", t.Arg%d", i)
			}
			structStr += ")\n}\n"

			g.tupleStructs[structName] = structStr

			// Add fmt import since we're using fmt.Sprintf in the String() method
			if g.imports == nil {
				g.imports = make(map[string]*ast.ImportSpec)
			}
			g.imports[`"fmt"`] = &ast.ImportSpec{
				Path: &ast.BasicLit{Kind: token.STRING, Value: `"fmt"`},
			}
		}

		// Generate the function signature
		out := g.Emit(fmt.Sprintf("func %s(", name))
		for i, paramName := range paramNames {
			if i > 0 {
				out += g.Emit(", ")
			}
			out += g.Emit(fmt.Sprintf("%s %s", paramName, paramTypes[i]))
		}
		out += g.Emit(fmt.Sprintf(") %s {\n", returnType.GoStr()))

		// Generate the function body
		out += g.Emit(fmt.Sprintf("\tout := make(%s, 0)\n", returnType.GoStr()))
		out += g.Emit("\tfor i := range a {\n")

		// Generate length checks for all arrays
		out += g.Emit("\t\tif ")
		for i, paramName := range paramNames {
			if i > 0 {
				out += g.Emit(" || ")
			}
			out += g.Emit(fmt.Sprintf("len(%s) <= i", paramName))
		}
		out += g.Emit(" {\n")
		out += g.Emit("\t\t\tbreak\n")
		out += g.Emit("\t\t}\n")

		// Generate the push statement
		out += g.Emit(fmt.Sprintf("\t\tAglVecPush((*%s)(&out), %s{", returnType.GoStr(), tupleType.GoStr()))
		for i, paramName := range paramNames {
			if i > 0 {
				out += g.Emit(", ")
			}
			out += g.Emit(fmt.Sprintf("Arg%d: %s[i]", i, paramName))
		}
		out += g.Emit("})\n")
		out += g.Emit("\t}\n")
		out += g.Emit("\treturn out\n")
		out += g.Emit("}\n")

		return out
	}
}
