package agl

import (
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"
	"agl/pkg/types"
	"agl/pkg/utils"
	"errors"
	"fmt"
	goast "go/ast"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

func ParseSrc(src string) (*token.FileSet, *ast.File, *ast.File) {
	// support "#!/usr/bin/env agl run" as the first line of agl "script"
	if strings.HasPrefix(src, "#!") {
		src = "//" + src
	}
	var fset = token.NewFileSet()
	f := Must(parser.ParseFile(fset, "", src, parser.AllErrors|parser.ParseComments))
	f2 := Must(parser.ParseFile(fset, "core.agl", CoreFns(), parser.AllErrors|parser.ParseComments))
	return fset, f, f2
}

type Inferrer struct {
	Env *Env
}

func NewInferrer(env *Env) *Inferrer {
	return &Inferrer{Env: env}
}

func (infer *Inferrer) InferFile(fileName string, f *ast.File, fset *token.FileSet, mutEnforced bool) (map[string]*ast.ImportSpec, []error) {
	if f.Doc != nil {
		for _, r := range f.Doc.List {
			if r.Text == "// agl:disable(mut_check)" {
				mutEnforced = false
			}
		}
	}
	fileInferrer := &FileInferrer{fileName: fileName, env: infer.Env, f: f, fset: fset, mutEnforced: mutEnforced, imports: make(map[string]*ast.ImportSpec)}
	fileInferrer.Infer()
	return fileInferrer.imports, fileInferrer.Errors
}

type FileInferrer struct {
	fileName        string
	env             *Env
	f               *ast.File
	fset            *token.FileSet
	PackageName     string
	returnType      types.Type
	optType         *OptTypeTmp
	optType1        *OptTypeTmp
	forceReturnType types.Type
	mapKT, mapVT    types.Type
	Errors          []error
	mutEnforced     bool
	destructure     bool
	indexValue      types.Type
	imports         map[string]*ast.ImportSpec
}

type InferError struct {
	err error
	N   ast.Node
}

func (i *InferError) Error() string {
	return i.err.Error()
}

func (infer *FileInferrer) errorf(n ast.Node, f string, args ...any) {
	_, file, line, _ := runtime.Caller(1)
	file = strings.TrimSuffix(filepath.Base(file), ".go")
	pos := infer.Pos(n)
	var msg string
	msg += fmt.Sprintf("%s:%d ", file, line)
	msg += fmt.Sprintf("%d:%d: ", pos.Line, pos.Column)
	msg += fmt.Sprintf(f, args...)
	err := errors.New(msg)
	infer.Errors = append(infer.Errors, &InferError{N: n, err: err})
}

type OptTypeTmp struct {
	Pos  token.Pos
	Type types.Type
}

func (o *OptTypeTmp) IsDefinedFor(n ast.Node) bool {
	return o != nil && o.Type != nil && o.Pos == n.Pos()
}

func (infer *FileInferrer) withIndexValue(v types.Type, clb func()) {
	prev := infer.indexValue
	infer.indexValue = v
	clb()
	infer.indexValue = prev
}

func (infer *FileInferrer) withDestructure(clb func()) {
	prev := infer.destructure
	infer.destructure = true
	clb()
	infer.destructure = prev
}

func (infer *FileInferrer) sandboxed(clb func()) {
	old := infer.env
	nenv := old.SubEnv()
	infer.env = nenv
	clb()
	infer.env = old
}

func (infer *FileInferrer) withMapKV(k, v types.Type, clb func()) {
	oldMapK, oldMapV := infer.mapKT, infer.mapVT
	infer.mapKT, infer.mapVT = k, v
	clb()
	infer.mapKT, infer.mapVT = oldMapK, oldMapV
}

func (infer *FileInferrer) withOptType(n ast.Node, t types.Type, clb func()) {
	prev := infer.optType
	infer.optType = &OptTypeTmp{Type: t, Pos: n.Pos()}
	clb()
	infer.optType = prev
}

func (infer *FileInferrer) withOptType1(n ast.Node, t types.Type, clb func()) {
	prev := infer.optType1
	infer.optType1 = &OptTypeTmp{Type: t, Pos: n.Pos()}
	clb()
	infer.optType1 = prev
}

func (infer *FileInferrer) withForceReturn(t types.Type, clb func()) {
	prev := infer.forceReturnType
	infer.forceReturnType = t
	clb()
	infer.forceReturnType = prev
}

func (infer *FileInferrer) withReturnType(t types.Type, clb func()) {
	prev := infer.returnType
	infer.returnType = t
	clb()
	infer.returnType = prev
}

func (infer *FileInferrer) withEnv(clb func()) {
	old := infer.env
	nenv := old.SubEnv()
	infer.env = nenv
	clb()
	infer.env = old
}

func (infer *FileInferrer) GetTypeFn(n ast.Node) types.FuncType {
	return infer.GetType(n).(types.FuncType)
}

func (infer *FileInferrer) GetType(n ast.Node) types.Type {
	t := infer.env.GetType(n)
	if t == nil {
		infer.errorf(n, "type not found for %v %v", n, to(n))
		return nil
	}
	return t
}

func (infer *FileInferrer) GetType2(n ast.Node) types.Type {
	return infer.env.GetType2(n, infer.fset)
}

func (infer *FileInferrer) SetTypeForce(a ast.Node, t types.Type) {
	infer.env.SetType(nil, nil, a, t, infer.fset)
}

type SetTypeConf struct {
	definition  *Info
	definition1 *DefinitionProvider
	description string
}

type SetTypeOption func(*SetTypeConf)

func WithDefinition(i *Info) SetTypeOption {
	return func(o *SetTypeConf) {
		o.definition = i
	}
}

func WithDefinition1(i DefinitionProvider) SetTypeOption {
	return func(o *SetTypeConf) {
		o.definition1 = &i
	}
}

func WithDesc(desc string) SetTypeOption {
	return func(o *SetTypeConf) {
		o.description = desc
	}
}

func (infer *FileInferrer) SetType(a ast.Node, t types.Type, opts ...SetTypeOption) {
	if t == nil {
		return
	}
	conf := &SetTypeConf{}
	for _, opt := range opts {
		opt(conf)
	}
	if conf.description != "" {
		if conf.definition == nil {
			conf.definition = &Info{}
		}
		conf.definition.Message = conf.description
	}
	if tt := infer.env.GetType(a); tt != nil {
		if !cmpTypesLoose(tt, t) {
			if !TryCast[types.UntypedNumType](tt) && !TryCast[types.UntypedStringType](t) {
				panic(fmt.Sprintf("type already declared for pos:%s key:%s a:%v toa:%v aT:%v t:%v", infer.Pos(a), infer.env.makeKey(a), a, to(a), infer.env.GetType(a), t))
			}
		}
	}
	infer.env.SetType(conf.definition, conf.definition1, a, t, infer.fset)
}

func trimPrefixPath(s string) string {
	sep := string(os.PathSeparator)
	parts := strings.Split(s, sep)
	if len(parts) <= 1 {
		return s
	}
	return strings.Join(parts[1:], sep)
}

// formatFieldList formats a parameter or result list into Go-style signature string
func formatFieldList(fl *goast.FieldList) string {
	if fl == nil {
		return "()"
	}
	out := "("
	var tmp1 []string
	for _, field := range fl.List {
		var tmp []string
		for range field.Names {
			tmp = append(tmp, exprToString(field.Type))
		}
		if len(field.Names) == 0 {
			tmp = append(tmp, exprToString(field.Type))
		}
		tmp1 = append(tmp1, strings.Join(tmp, ", "))
	}
	out += strings.Join(tmp1, ", ")
	out += ")"
	return out
}

// exprToString returns a basic string representation of an expression (e.g., type)
func exprToString(expr goast.Expr) string {
	switch t := expr.(type) {
	case *goast.Ident:
		return t.Name
	case *goast.StarExpr:
		return "*" + exprToString(t.X)
	case *goast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *goast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *goast.Ellipsis:
		return "..." + exprToString(t.Elt)
	case *goast.FuncType:
		return "func" + formatFieldList(t.Params)
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func (infer *FileInferrer) Infer() {
	infer.env.withEnv(func(nenv *Env) {
		infer.PackageName = infer.f.Name.Name
		infer.SetType(infer.f.Name, types.PackageType{Name: infer.f.Name.Name})
		t := &TreeDrawer{}
		loadAglImports("main", 0, t, infer.env, nenv, infer.f, NewPkgVisited(), infer.fset)
		if utils.False() {
			t.Draw()
		}
		// TODO do a second pass for types that used before their declaration
		for _, d := range infer.f.Decls {
			switch decl := d.(type) {
			case *ast.GenDecl:
				infer.genDecl(decl)
			}
		}
		for _, d := range infer.f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				infer.funcDecl(decl)
			}
		}
		for _, d := range infer.f.Decls {
			switch decl := d.(type) {
			case *ast.FuncDecl:
				infer.funcDecl2(decl)
			}
		}
	})
}

type PkgVisited struct {
	m map[string]struct{}
}

func NewPkgVisited() *PkgVisited {
	return &PkgVisited{m: make(map[string]struct{})}
}

func (p *PkgVisited) Keys() []string {
	return slices.Collect(maps.Keys(p.m))
}

func (p *PkgVisited) Add(pkg string) {
	p.m[pkg] = struct{}{}
}

func (p *PkgVisited) Contains(pkg string) bool {
	_, ok := p.m[pkg]
	return ok
}

func (p *PkgVisited) ContainsAdd(pkg string) bool {
	res := p.Contains(pkg)
	if !res {
		p.Add(pkg)
	}
	return res
}

func (infer *FileInferrer) genDecl(decl *ast.GenDecl) {
	for _, s := range decl.Specs {
		switch spec := s.(type) {
		case *ast.TypeSpec:
			infer.typeSpec(spec)
		case *ast.ImportSpec:
		case *ast.ValueSpec:
			infer.valueSpec(spec)
		default:
			infer.errorf(spec, "%s: unsupported spec type", to(spec))
			return
		}
	}
}

func (infer *FileInferrer) valueSpec(spec *ast.ValueSpec) {
	var t types.Type
	if spec.Values != nil {
		infer.expr(spec.Values[0])
		t = types.Unwrap(infer.GetType2(spec.Values[0]))
	}
	if spec.Type != nil {
		infer.expr(spec.Type)
		t = infer.GetType2(spec.Type)
		infer.SetType(spec.Type, t)
	}
	for _, name := range spec.Names {
		tt := t
		if name.Mutable.IsValid() {
			tt = types.MutType{W: tt}
		}
		infer.env.Define(name, name.Name, tt)
		infer.SetType(name, tt)
	}
}

func (infer *FileInferrer) typeSpec(spec *ast.TypeSpec) {
	var toDef types.Type
	switch t := spec.Type.(type) {
	case *ast.Ident:
		typ := infer.GetType2(t)
		if typ == nil {
			infer.errorf(spec.Name, "type not found '%s'", t)
			return
		}
		toDef = types.TypeType{W: types.CustomType{Name: spec.Name.Name, W: typ}}
	case *ast.StructType:
		structT := types.StructType{Name: spec.Name.Name}
		infer.env.Define(spec.Name, spec.Name.Name, structT)
		infer.env.withEnv(func(nenv *Env) {
			if spec.TypeParams != nil {
				var tpFields []types.FieldType
				for _, typeParam := range spec.TypeParams.List {
					typ := infer.GetType2(typeParam.Type)
					if len(typeParam.Names) > 0 {
						for _, n := range typeParam.Names {
							tpFields = append(tpFields, types.FieldType{Name: n.Name, Typ: typ})
							structT.TypeParams = append(structT.TypeParams, types.GenericType{Name: n.Name, W: typ, IsType: true})
							nenv.Define(spec.Name, n.Name, typ)
						}
					} else {
						structT.TypeParams = append(structT.TypeParams, types.GenericType{Name: "", W: typ, IsType: true})
					}
				}
				//if len(tpFields) > 1 {
				//	toDef1 = types.IndexListType{X: structT, Indices: tpFields}
				//} else {
				//	toDef1 = types.IndexType{X: structT, Index: tpFields}
				//}
			}
			var fields []types.FieldType
			if t.Fields != nil {
				for _, f := range t.Fields.List {
					typ := nenv.GetType2(f.Type, infer.fset)
					infer.SetType(f.Type, typ)
					if len(f.Names) == 0 {
						fields = append(fields, types.FieldType{Name: "", Typ: typ})
					}
					for _, n := range f.Names {
						tt := typ
						if n.Mutable.IsValid() {
							tt = types.MutType{W: tt}
						}
						fields = append(fields, types.FieldType{Name: n.Name, Typ: tt})
						infer.env.Define(spec.Name, spec.Name.Name+"."+n.Name, tt)
					}
				}
			}
			structT.Fields = fields
		})
		toDef = structT
	case *ast.EnumType:
		var fields []types.EnumFieldType
		if t.Values != nil {
			for _, f := range t.Values.List {
				var elts []types.Type
				if f.Params != nil {
					for _, param := range f.Params.List {
						elts = append(elts, infer.GetType2(param.Type))
					}
				}
				fields = append(fields, types.EnumFieldType{Name: f.Name.Name, Elts: elts})
			}
		}
		toDef = types.EnumType{Name: spec.Name.Name, Fields: fields}
	case *ast.InterfaceType:
		var methodsT []types.InterfaceMethod
		if t.Methods.List != nil {
			for _, f := range t.Methods.List {
				if f.Type != nil {
					infer.expr(f.Type)
				}
				for _, n := range f.Names {
					fnT := funcTypeToFuncType("", f.Type.(*ast.FuncType), infer.env, infer.fset, false)
					infer.env.Define(spec.Name, spec.Name.Name+"."+n.Name, fnT)
					methodsT = append(methodsT, types.InterfaceMethod{Name: n.Name, Typ: fnT})
				}
			}
		}
		toDef = types.InterfaceType{Name: spec.Name.Name, Methods: methodsT}
	case *ast.ArrayType:
		toDef = types.CustomType{Name: spec.Name.Name, W: types.ArrayType{Elt: infer.GetType2(t.Elt)}}
	case *ast.MapType:
		kT := infer.GetType2(t.Key)
		vT := infer.GetType2(t.Value)
		mT := types.MapType{K: kT, V: vT}
		toDef = types.CustomType{Name: spec.Name.Name, W: mT}
	case *ast.FuncType:
		t.TypeParams = spec.TypeParams
		toDef = funcTypeToFuncType("", t, infer.env, infer.fset, false)
	default:
		infer.errorf(spec.Name, "%v", to(spec.Type))
		return
	}
	infer.env.Define(spec.Name, spec.Name.Name, toDef)
}

func (infer *FileInferrer) structType(name *ast.Ident, s *ast.StructType) {
	var fields []types.FieldType
	if s.Fields != nil {
		for _, f := range s.Fields.List {
			t := infer.GetType2(f.Type)
			for _, n := range f.Names {
				fields = append(fields, types.FieldType{Name: n.Name, Typ: t})
			}
		}
	}
	infer.env.Define(name, name.Name, types.StructType{Name: name.Name, Fields: fields})
}

func (infer *FileInferrer) funcDecl(decl *ast.FuncDecl) {
	var t types.FuncType
	outEnv := infer.env
	infer.sandboxed(func() {
		t = infer.getFuncDeclType(decl, outEnv)
	})
	infer.SetType(decl, t)
	fnName := decl.Name.Name
	if newName, ok := overloadMapping[fnName]; ok {
		fnName = newName
	}
	if decl.Recv != nil {
		t1 := decl.Recv.List[0].Type
		if v, ok := t1.(*ast.StarExpr); ok {
			t1 = v.X
		}
		recvTStr := infer.GetType2(t1).GoStr()
		fnName = recvTStr + "." + fnName
	}
	infer.env.Define(decl.Name, fnName, t)
	infer.SetType(decl.Name, t)
	infer.SetType(decl, t)
}

// mapping of "agl function name" to "go compiled function name"
var overloadMapping = map[string]string{
	"==":     "__EQL",
	"!=":     "__EQL",
	"+":      "__ADD",
	"-":      "__SUB",
	"*":      "__MUL",
	"/":      "__QUO",
	"%":      "__REM",
	"**":     "__POW",
	"+=":     "__ADD_ASSIGN",
	"__RADD": "__RADD",
	"__RQUO": "__RQUO",
	"__RMUL": "__RMUL",
}

func (infer *FileInferrer) funcDecl2(decl *ast.FuncDecl) {
	infer.withEnv(func() {
		if decl.Recv != nil {
			for _, recv := range decl.Recv.List {
				infer.env.NoIdxUnwrap = true
				t := infer.GetType2(recv.Type)
				infer.env.NoIdxUnwrap = false
				for _, name := range recv.Names {
					if name.Mutable.IsValid() {
						t = types.MutType{W: t}
					}
					infer.env.SetType(nil, nil, name, t, infer.fset)
					infer.env.Define(name, name.Name, t)
				}
			}
		}
		if decl.Type.TypeParams != nil {
			for _, param := range decl.Type.TypeParams.List {
				infer.expr(param.Type)
				t := infer.GetType2(param.Type)
				for _, name := range param.Names {
					infer.env.SetType(nil, nil, name, t, infer.fset)
					infer.env.Define(name, name.Name, types.GenericType{Name: name.Name, W: t, IsType: true})
				}
			}
		}
		if decl.Type.Params != nil {
			for _, param := range decl.Type.Params.List {
				infer.expr(param.Type)
				t := infer.GetType2(param.Type)
				if !TryCast[types.TypeType](t) {
					infer.SetType(param.Type, types.TypeType{W: t})
				}
				for _, name := range param.Names {
					tt := t
					infer.SetType(name.Ident, tt)
					if name.Mutable.IsValid() {
						tt = types.MutType{W: tt}
					}
					if name.Label != nil && name.Label.Name != "" {
						tt = types.LabelledType{Label: name.Label.Name, W: tt}
					}
					infer.env.Define(name, name.Name, tt)
					infer.env.SetType(nil, nil, name, tt, infer.fset)
				}
			}
		}

		var returnTyp types.Type = types.VoidType{}
		if decl.Type.Result != nil {
			infer.expr(decl.Type.Result)
			returnTyp = infer.GetType2(decl.Type.Result)
			if v, ok := decl.Type.Result.(*ast.IndexExpr); ok {
				iT := infer.GetType2(v.Index)
				switch vv := returnTyp.(type) {
				case types.FuncType:
					returnTyp = vv.Concrete([]types.Type{iT})
				case types.InterfaceType:
					returnTyp = vv.Concrete([]types.Type{iT})
				case types.StructType:
					returnTyp = vv.Concrete([]types.Type{iT})
				default:
					panic(fmt.Sprintf("%v", to(returnTyp)))
				}
			}
			infer.SetType(decl.Type.Result, returnTyp)
		}
		infer.withReturnType(returnTyp, func() {
			if decl.Body != nil {
				// implicit return
				cond1 := len(decl.Body.List) == 1 ||
					(len(decl.Body.List) == 2 && TryCast[*ast.EmptyStmt](decl.Body.List[1]))
				if cond1 && decl.Type.Result != nil {
					if v, ok := decl.Body.List[0].(*ast.ExprStmt); ok && !TryCast[*ast.MatchExpr](v.X) {
						decl.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: v.X}}
					}
				}
				infer.stmt(decl.Body)
			}
		})
	})
}

func (infer *FileInferrer) getFuncDeclType(decl *ast.FuncDecl, outEnv *Env) types.FuncType {
	var returnT types.Type
	var recvT, paramsT, typeParamsT []types.Type
	var vecExt, strExt bool
	if decl.Recv != nil {
		if len(decl.Recv.List) == 1 {
			switch v := decl.Recv.List[0].Type.(type) {
			case *ast.IndexExpr:
				if sel, ok := v.X.(*ast.SelectorExpr); ok {
					xName := sel.X.(*ast.Ident).Name
					if xName == "agl1" && sel.Sel.Name == "Vec" {
						defaultName := "T" // Should not hardcode "T"
						vecExt = true
						id := v.Index.(*ast.Ident)
						typeName := utils.Ternary(id.Name == defaultName, "any", id.Name)
						t := &ast.Field{Names: []*ast.LabelledIdent{{&ast.Ident{Name: defaultName}, nil}}, Type: &ast.Ident{Name: typeName}}
						if decl.Type.TypeParams == nil {
							decl.Type.TypeParams = &ast.FieldList{List: []*ast.Field{t}}
						} else {
							decl.Type.TypeParams.List = append([]*ast.Field{t}, decl.Type.TypeParams.List...)
						}
					}
				}
			case *ast.SelectorExpr:
				xName := v.X.(*ast.Ident).Name
				if xName == "agl1" && v.Sel.Name == "String" {
					strExt = true
				}
			}
		}
		for _, recv := range decl.Recv.List {
			for _, name := range recv.Names {
				t := infer.GetType2(recv.Type)
				if name.Mutable.IsValid() {
					t = types.MutType{W: t}
				}
				recvT = append(recvT, t)
				infer.env.Define(name, name.Name, t)
			}
		}
	}
	if decl.Type.TypeParams != nil {
		for _, typeParam := range decl.Type.TypeParams.List {
			infer.expr(typeParam.Type)
			t := infer.env.GetType(typeParam.Type)
			for _, name := range typeParam.Names {
				tt := types.GenericType{Name: name.Name, W: t, IsType: true}
				typeParamsT = append(typeParamsT, tt)
				infer.env.Define(name, name.Name, tt)
			}
		}
	}
	if decl.Type.Params != nil {
		for _, param := range decl.Type.Params.List {
			infer.expr(param.Type)
			t := infer.GetType2(param.Type)
			for i := range param.Names {
				name := param.Names[i]
				tt := t
				if name.Mutable.IsValid() {
					tt = types.MutType{W: tt}
				}
				if name.Label != nil && name.Label.Name != "" {
					tt = types.LabelledType{Label: name.Label.Name, W: tt}
				}
				paramsT = append(paramsT, tt)
			}
		}
	}
	if decl.Type.Result != nil {
		infer.expr(decl.Type.Result)
		returnT = infer.GetType2(decl.Type.Result)
		switch v := returnT.(type) {
		case types.FuncType:
			returnT = v.RenameGenericParameter("V", "T")
		}
		switch r := returnT.(type) {
		case types.ResultType:
			r.Bubble = true
			returnT = r
		case types.OptionType:
			r.Bubble = true
			returnT = r
		}
	}
	if returnT == nil {
		returnT = types.VoidType{}
	}
	fnName := decl.Name.Name
	if newName, ok := overloadMapping[fnName]; ok {
		fnName = newName
	}
	ft := types.FuncType{
		Pub:        decl.Pub.IsValid(),
		Recv:       recvT,
		Name:       fnName,
		TypeParams: typeParamsT,
		Params:     paramsT,
		Return:     returnT,
	}
	if decl.Recv != nil {
		if vecExt {
			envName := fmt.Sprintf("agl1.Vec.%s", fnName)
			for _, pp := range paramsT {
				if v, ok := pp.(types.LabelledType); ok {
					envName += fmt.Sprintf("_%s", v.Label)
				}
			}
			outEnv.Define(decl.Name, envName, ft)
		} else if strExt {
			outEnv.Define(decl.Name, fmt.Sprintf("agl1.String.%s", fnName), ft)
		}
	}
	return ft
}

func (infer *FileInferrer) stmts(s []ast.Stmt) {
	for _, stmt := range s {
		infer.stmt(stmt)
	}
}

func (infer *FileInferrer) exprs(s []ast.Expr) {
	for _, expr := range s {
		infer.expr(expr)
	}
}

func (infer *FileInferrer) exprType(e ast.Expr) {

}

func (infer *FileInferrer) expr(e ast.Expr) {
	//p("infer.expr", to(e))
	switch expr := e.(type) {
	case *ast.IfExpr:
		infer.ifExpr(expr)
	case *ast.IfLetExpr:
		infer.ifLetExpr(expr)
	case *ast.MatchExpr:
		infer.matchExpr(expr)
	case *ast.Ident:
		infer.identExpr(expr)
	case *ast.CallExpr:
		infer.callExpr(expr)
	case *ast.BinaryExpr:
		infer.binaryExpr(expr)
	case *ast.OptionExpr:
		infer.optionExpr(expr)
	case *ast.ResultExpr:
		infer.resultExpr(expr)
	case *ast.IndexExpr:
		infer.indexExpr(expr)
	case *ast.ArrayType:
		infer.arrayType(expr)
	case *ast.FuncType:
		infer.funcType(expr)
	case *ast.BasicLit:
		infer.basicLit(expr)
	case *ast.ShortFuncLit:
		infer.shortFuncLit(expr)
	case *ast.CompositeLit:
		infer.compositeLit(expr)
	case *ast.BubbleOptionExpr:
		infer.bubbleOptionExpr(expr)
	case *ast.BubbleResultExpr:
		infer.bubbleResultExpr(expr)
	case *ast.SelectorExpr:
		infer.selectorExpr(expr)
	case *ast.FuncLit:
		infer.funcLit(expr)
	case *ast.TupleExpr:
		infer.tupleExpr(expr)
	case *ast.Ellipsis:
		infer.ellipsis(expr)
	case *ast.VoidExpr:
		infer.voidExpr(expr)
	case *ast.StarExpr:
		infer.starExpr(expr)
	case *ast.SomeExpr:
		infer.someExpr(expr)
	case *ast.OkExpr:
		infer.okExpr(expr)
	case *ast.ErrExpr:
		infer.errExpr(expr)
	case *ast.NoneExpr:
		infer.noneExpr(expr)
	case *ast.ChanType:
		infer.chanType(expr)
	case *ast.UnaryExpr:
		infer.unaryExpr(expr)
	case *ast.TypeAssertExpr:
		infer.typeAssertExpr(expr)
	case *ast.MapType:
		infer.mapType(expr)
	case *ast.OrBreakExpr:
		infer.orBreak(expr)
	case *ast.OrContinueExpr:
		infer.orContinue(expr)
	case *ast.OrReturnExpr:
		infer.orReturn(expr)
	case *ast.IndexListExpr:
		infer.indexListExpr(expr)
	case *ast.KeyValueExpr:
		infer.keyValueExpr(expr)
	case *ast.InterfaceType:
		infer.interfaceType(expr)
	case *ast.SliceExpr:
		infer.sliceExpr(expr)
	case *ast.DumpExpr:
		infer.dumpExpr(expr)
	case *ast.ParenExpr:
		infer.parenExpr(expr)
	case *ast.StructType:
		infer.structTypeExpr(expr)
	case *ast.SetType:
		infer.setTypeExpr(expr)
	case *ast.LabelledArg:
		infer.labelledArg(expr)
	case *ast.RangeExpr:
		infer.rangeExpr(expr)
	default:
		panic(fmt.Sprintf("unknown expression %v", to(e)))
	}
	if infer.optType.IsDefinedFor(e) {
		infer.tryConvertType(e, infer.optType.Type)
	}
}

func isIntType(t types.Type) bool {
	return TryCast[types.I64Type](t) ||
		TryCast[types.I32Type](t) ||
		TryCast[types.I16Type](t) ||
		TryCast[types.I8Type](t) ||
		TryCast[types.IntType](t) ||
		TryCast[types.U64Type](t) ||
		TryCast[types.U32Type](t) ||
		TryCast[types.U16Type](t) ||
		TryCast[types.U8Type](t) ||
		TryCast[types.IntType](t) ||
		TryCast[types.UintType](t)
}

func isNumericType(t types.Type) bool {
	return isIntType(t) ||
		TryCast[types.F64Type](t) ||
		TryCast[types.F32Type](t) ||
		TryCast[types.UintptrType](t) ||
		TryCast[types.Complex64Type](t) ||
		TryCast[types.Complex128Type](t)
}

func (infer *FileInferrer) tryConvertType(e ast.Expr, optType types.Type) {
	if infer.env.GetType(e) == nil {
		infer.SetType(e, optType)
	} else if _, ok := infer.GetType(e).(types.UntypedNumType); ok {
		if isIntType(optType) {
			infer.SetType(e, optType)
		}
	}
}

func (infer *FileInferrer) stmt(s ast.Stmt) {
	//p("infer.stmt", to(s))
	switch stmt := s.(type) {
	case *ast.BlockStmt:
		infer.blockStmt(stmt)
	case *ast.GuardStmt:
		infer.guardStmt(stmt)
	case *ast.GuardLetStmt:
		infer.guardLetStmt(stmt)
	case *ast.ReturnStmt:
		infer.returnStmt(stmt)
	case *ast.ExprStmt:
		infer.exprStmt(stmt)
	case *ast.AssignStmt:
		infer.assignStmt(stmt)
	case *ast.RangeStmt:
		infer.rangeStmt(stmt)
	case *ast.IncDecStmt:
		infer.incDecStmt(stmt)
	case *ast.DeclStmt:
		infer.declStmt(stmt)
	case *ast.ForStmt:
		infer.forStmt(stmt)
	case *ast.SendStmt:
		infer.sendStmt(stmt)
	case *ast.SelectStmt:
		infer.selectStmt(stmt)
	case *ast.CommClause:
		infer.commClause(stmt)
	case *ast.SwitchStmt:
		infer.switchStmt(stmt)
	case *ast.CaseClause:
		infer.caseClause(stmt)
	case *ast.LabeledStmt:
		infer.labeledStmt(stmt)
	case *ast.BranchStmt:
		infer.branchStmt(stmt)
	case *ast.DeferStmt:
		infer.deferStmt(stmt)
	case *ast.GoStmt:
		infer.goStmt(stmt)
	case *ast.TypeSwitchStmt:
		infer.typeSwitchStmt(stmt)
	case *ast.EmptyStmt:
		infer.emptyStmt(stmt)
	default:
		panic(fmt.Sprintf("unknown statement %v", to(stmt)))
	}
}

func (infer *FileInferrer) basicLit(expr *ast.BasicLit) {
	if infer.env.GetType(expr) != nil {
		return
	}
	switch expr.Kind {
	case token.STRING:
		infer.SetType(expr, types.StringType{})
	case token.FLOAT:
		infer.SetType(expr, types.UntypedNumType{})
		if infer.optType.IsDefinedFor(expr) && !TryCast[types.GenericType](infer.optType.Type) {
			infer.SetType(expr, infer.optType.Type)
		} else {
			infer.SetType(expr, types.UntypedNumType{})
		}
	case token.INT:
		infer.SetType(expr, types.UntypedNumType{})
		if infer.optType.IsDefinedFor(expr) && !TryCast[types.GenericType](infer.optType.Type) {
			infer.SetType(expr, infer.optType.Type)
		} else {
			infer.SetType(expr, types.UntypedNumType{})
		}
	case token.CHAR:
		infer.SetType(expr, types.CharType{})
	default:
		infer.errorf(expr, "unknown basic literal %v %v", to(expr), expr.Kind)
		return
	}
}

func (infer *FileInferrer) getSelectorType(e ast.Expr, id *ast.Ident) types.Type {
	eTRaw := infer.GetType2(e)
	eTRaw = types.Unwrap(eTRaw)
	switch eT := eTRaw.(type) {
	case types.StructType:
		name := eT.GetFieldName(id.Name)
		return infer.env.Get(name)
	default:
		infer.errorf(id, "%v", to(eTRaw))
		return nil
	}
}

func (infer *FileInferrer) inferStructType(sT types.StructType, expr *ast.SelectorExpr) types.Type {
	fieldName := expr.Sel.Name
	name := sT.GetFieldName(fieldName)
	t := infer.env.Get(name)

	m := make(map[string]types.Type)
	for _, pp := range sT.TypeParams {
		if v, ok := pp.(types.GenericType); ok {
			m[v.Name] = v.W
		}
	}
	t = types.ReplGenM(t, m)

	if t != nil {
		infer.SetType(expr.X, sT)
		infer.SetType(expr.Sel, t)
		infer.SetType(expr, t)
	} else {
		infer.SetType(expr.X, sT)
	}
	return t
}

func (infer *FileInferrer) callExprArrayType(expr *ast.CallExpr, call *ast.ArrayType) {
	callT := infer.GetType2(call)
	infer.SetType(call, callT)
	infer.SetType(expr, callT)
}

func (infer *FileInferrer) callExprFuncLit(expr *ast.CallExpr, call *ast.FuncLit) {
	callT := funcTypeToFuncType("", call.Type, infer.env, infer.fset, false)
	infer.withReturnType(callT.Return, func() {
		infer.stmt(call.Body)
	})
	infer.SetType(call, callT)
	infer.SetType(expr, callT.Return)
}

func (infer *FileInferrer) callExprIdent(expr *ast.CallExpr, call *ast.Ident) {
	infer.langFns(expr, call)
	if call.Name == "panic" && len(expr.Args) > 0 {
		call.Name = "panicWith"
	}
	callT := infer.env.Get(call.Name)
	if callT == nil {
		infer.errorf(call, "Unresolved reference '%s'", call.Name)
		return
	}
	switch callTT := callT.(type) {
	case types.LabelledType:
	case types.TypeType:
		infer.expr(expr.Args[0])
		infer.SetType(expr, callTT.W)
	case types.FuncType:
		oParams := callTT.Params
		for i := range expr.Args {
			arg := expr.Args[i]
			if len(oParams) == 0 {
				infer.errorf(call, "missing parameter")
				return
			}
			oArg := oParams[min(i, len(oParams)-1)]
			infer.withOptType(arg, oArg, func() {
				infer.expr(arg)
			})
			if v, ok := oArg.(types.EllipsisType); ok {
				oArg = v.Elt
			}
			got := infer.GetType(arg)
			if oArgT, ok := oArg.(types.IndexType); ok {
				oArg = oArgT.X
			}
			if v, ok := arg.(*ast.LabelledArg); ok {
				ooArg := oArg
				if vv, ok := oArg.(types.MutType); ok {
					ooArg = vv.W
				}
				if vv, ok := ooArg.(types.LabelledType); ok {
					if v.Label.Name != "" && v.Label.Name != vv.Label {
						infer.errorf(arg, "label name does not match %s vs %s", v.Label.Name, vv.Label)
						return
					}
				} else {
					infer.errorf(arg, "label does not exists")
					return
				}
				arg = v.X
			}
			if !cmpTypesLoose(oArg, got) {
				infer.errorf(arg, "types not equal, %v %v", oArg, got)
				return
			}
			callT = types.ReplGen2(callT, oArg, got)
		}
	default:
		infer.errorf(call, "%v", to(callT))
		return
	}
	parentInfo := infer.env.GetNameInfo(call.Name)
	if infer.env.GetType(call) == nil {
		infer.SetType(call, callT, WithDefinition(parentInfo))
	}
}

func (infer *FileInferrer) callExprIndexExpr(expr *ast.CallExpr, call *ast.IndexExpr) {
	sel := call.X.(*ast.SelectorExpr)
	infer.withIndexValue(infer.GetType2(call.Index), func() {
		infer.callExprSelectorExpr(expr, sel)
	})
}

func (infer *FileInferrer) callExprSelectorExpr(expr *ast.CallExpr, call *ast.SelectorExpr) {
	var exprFunT types.Type
	var callXParent *Info
	var exprFunTIsParen bool // TODO not a good fix, allows a non-mut ident to be wrapped in a paren

	infer.expr(call.X)
	switch callXT := call.X.(type) {
	case *ast.Ident:
		exprFunT = infer.env.Get(callXT.Name)
		callXParent = infer.env.GetNameInfo(callXT.Name)
		if exprFunT == nil {
			infer.errorf(call.X, "Unresolved reference '%s'", callXT.Name)
			return
		}
	case *ast.CompositeLit:
		exprFunT = infer.GetType2(callXT)
	case *ast.CallExpr, *ast.BubbleResultExpr, *ast.BubbleOptionExpr:
		exprFunT = infer.GetType2(callXT)
	case *ast.SelectorExpr:
		if callXTXT := infer.env.GetType(callXT.X); callXTXT != nil {
			switch v := callXTXT.(type) {
			case types.StructType:
				exprFunT = infer.inferStructType(v, callXT)
			case types.TupleType:
				idx, err := strconv.Atoi(callXT.Sel.Name)
				if err != nil {
					infer.errorf(callXT.Sel, "tuple selector must be a number")
					return
				}
				exprFunT = v.Elts[idx]
				infer.SetType(callXT.Sel, exprFunT)
			default:
				panic("")
			}
		} else {
			//infer.SetType(callXT.X, )
			exprFunT = infer.getSelectorType(callXT.X, callXT.Sel)
		}
	case *ast.IndexExpr:
		exprFunT = infer.GetType2(callXT)
	case *ast.TypeAssertExpr:
		exprFunT = types.OptionType{W: infer.GetType2(callXT)}
	case *ast.BasicLit:
		exprFunT = infer.GetType2(callXT)
	case *ast.ParenExpr:
		exprFunT = infer.GetType2(callXT)
		exprFunTIsParen = true
	case *ast.RangeExpr:
		exprFunT = infer.GetType2(callXT)
	case *ast.SliceExpr:
		exprFunT = infer.GetType2(callXT)
	default:
		infer.errorf(call.X, "%v %v", call.X, to(call.X))
		return
	}

	fnName := call.Sel.Name
	oexprFunT := exprFunT
	exprFunT = types.Unwrap(exprFunT)
	switch idTT := exprFunT.(type) {
	case types.TypeType:
	case types.UntypedNumType:
	case types.UntypedStringType:
	case types.StringType:
	case types.IntType:
	case types.I8Type:
	case types.I16Type:
	case types.I32Type:
	case types.I64Type:
	case types.UintType:
	case types.U8Type:
	case types.U16Type:
	case types.U32Type:
	case types.U64Type:
	case types.F32Type:
	case types.F64Type:
	case types.SetType:
	case types.ArrayType:
	case types.MapType:
	case types.CustomType:
		name := fmt.Sprintf("%s.%s", idTT, fnName)
		t := infer.env.Get(name)
		tr := t.(types.FuncType).Return
		infer.SetType(call.Sel, t)
		infer.SetType(call, tr)
		infer.SetType(expr, tr)
	case types.StructType:
		if call.Sel.Name != "Sum" {
			name := idTT.GetFieldName(call.Sel.Name)
			nameT := infer.env.Get(name)

			// Handle struct composition. If we did not find the method for the struct,
			// we need to check other structs that are "inherited" (composition).
			if nameT == nil {
				for _, field := range idTT.Fields {
					if field.Name == "" {
						fieldType := types.Unwrap(field.Typ)
						if v, ok := fieldType.(types.StructType); ok {
							name = v.GetFieldName(call.Sel.Name)
							nameT = infer.env.Get(name)
							nameT = types.Unwrap(nameT)
						}
					}
				}
			}

			if nameT == nil {
				infer.errorf(call.Sel, "method not found '%s' in struct of type '%v'", call.Sel.Name, idTT.Name)
				return
			}
			fnT := infer.env.GetFn(name)
			if len(fnT.Recv) > 0 && TryCast[types.MutType](fnT.Recv[0]) {
				if infer.mutEnforced && !exprFunTIsParen && !TryCast[types.MutType](oexprFunT) {
					infer.errorf(call.Sel, "method '%s' cannot be called on immutable type '%s'", call.Sel.Name, idTT.Name)
					return
				}
			}
			toReturn := fnT.Return
			toReturn = alterResultBubble(infer.returnType, toReturn)
			infer.SetType(call.Sel, fnT)
			infer.SetType(expr, toReturn)
		}
	case types.InterfaceType:
		t := idTT.GetMethodByName(call.Sel.Name)
		//name := fmt.Sprintf("%s.%s", idTT, fnName)
		//t := infer.env.Get(name)
		tr := t.(types.FuncType).Return
		infer.SetType(call.Sel, t)
		infer.SetType(call, tr)
		infer.SetType(expr, tr)
	case types.EnumType:
		sub := call.Sel.Name
		if sub == "RawValue" {
			eT := infer.env.GetFn("agl1.Enum.RawValue").T("T", idTT).IntoRecv(idTT)
			infer.SetType(call.Sel, eT)
		}
		infer.SetType(expr, types.EnumType{Name: idTT.Name, SubTyp: sub, Fields: idTT.Fields})
	case types.PackageType:
		pkgT := infer.env.Get(idTT.Name)
		if pkgT == nil {
			infer.errorf(call.X, "package not found '%s'", idTT.Name)
			return
		}
		name := fmt.Sprintf("%s.%s", idTT.Name, call.Sel.Name)
		nameT := infer.env.Get(name)
		nameTInfo := infer.env.GetNameInfo(name)
		if nameT == nil {
			infer.errorf(call.Sel, "not found '%s' in package '%v'", call.Sel.Name, idTT.Name)
			return
		}
		fnT := nameT.(types.FuncType)
		toReturn := fnT.Return
		if toReturn != nil {
			toReturn = alterResultBubble(infer.returnType, toReturn)
		}
		infer.SetType(call.Sel, fnT, WithDefinition1(nameTInfo.Definition1))
		infer.SetType(expr.Fun, fnT)
		if toReturn != nil {
			infer.SetType(expr, toReturn)
		} else {
			infer.SetType(expr, types.VoidType{})
		}
	case types.OptionType:
		if fnName == "Map" {
			break
		}
		if !InArray(fnName, []string{"IsNone", "IsSome", "Unwrap", "UnwrapOr", "UnwrapOrDefault", "Map"}) {
			infer.errorf(call.X, "Unresolved reference '%s'", fnName)
			return
		}
		info := infer.env.GetNameInfo("agl1.Option." + fnName)
		fnT := infer.env.GetFn("agl1.Option." + fnName)
		if InArray(fnName, []string{"Unwrap", "UnwrapOr", "UnwrapOrDefault"}) {
			fnT = fnT.T("T", idTT.W)
		}
		fnT.Recv = []types.Type{oexprFunT}
		infer.SetType(call.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.ResultType:
		if !InArray(fnName, []string{"IsOk", "IsErr", "Unwrap", "UnwrapOr", "UnwrapOrDefault", "Err"}) {
			infer.errorf(call.X, "Unresolved reference '%s'", fnName)
			return
		}
		info := infer.env.GetNameInfo("agl1.Result." + fnName)
		fnT := infer.env.GetFn("agl1.Result." + fnName)
		if InArray(fnName, []string{"Unwrap", "UnwrapOr", "UnwrapOrDefault"}) {
			fnT = fnT.T("T", idTT.W)
		} else if fnName == "Err" {
			infer.errorf(call.X, "cannot call Err on Result")
			return
		}
		fnT.Recv = []types.Type{oexprFunT}
		infer.SetType(call.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.RangeType:
		if !InArray(fnName, []string{"Rev", "AllSatisfy"}) {
			infer.errorf(call.X, "Unresolved reference '%s'", fnName)
			return
		}
		switch fnName {
		case "Rev":
			info := infer.env.GetNameInfo("agl1.DoubleEndedIterator.Rev")
			fnT := infer.env.GetFn("agl1.DoubleEndedIterator.Rev")
			fnT = fnT.T("T", idTT.Typ)
			infer.SetType(call.Sel, fnT, WithDesc(info.Message))
		case "AllSatisfy":
			info := infer.env.GetNameInfo("agl1.Iterator.AllSatisfy")
			fnT := infer.env.GetFn("agl1.Iterator.AllSatisfy")
			fnT = fnT.T("T", idTT.Typ).IntoRecv(idTT)
			infer.SetType(expr.Args[0], fnT.Params[0])
			infer.SetType(call.Sel, fnT, WithDesc(info.Message))
		}
		infer.SetType(expr, types.RangeType{Typ: idTT.Typ})
	default:
		infer.errorf(call.X, "Unresolved reference '%s'", fnName)
		return
	}
	infer.SetType(call.X, oexprFunT, WithDefinition(callXParent))
	infer.inferGoExtensions(expr, exprFunT, oexprFunT, call)
	if len(infer.Errors) > 0 {
		return
	}
	infer.exprs(expr.Args)
}

func (infer *FileInferrer) callExpr(expr *ast.CallExpr) {
	switch call := expr.Fun.(type) {
	case *ast.IndexExpr:
		infer.callExprIndexExpr(expr, call)
	case *ast.SelectorExpr:
		infer.callExprSelectorExpr(expr, call)
	case *ast.Ident:
		infer.callExprIdent(expr, call)
	case *ast.FuncLit:
		infer.callExprFuncLit(expr, call)
	case *ast.ArrayType:
		infer.callExprArrayType(expr, call)
	default:
		infer.errorf(expr.Fun, "%v", to(expr.Fun))
		return
	}
	if len(infer.Errors) > 0 {
		return
	}
	if exprFunT := infer.env.GetType(expr.Fun); exprFunT != nil {
		if v, ok := exprFunT.(types.FuncType); ok {
			switch v.Name {
			case "zip2":
				infer.imports["iter"] = &ast.ImportSpec{Path: &ast.BasicLit{Value: `"iter"`}}
			}
			if infer.mutEnforced && len(expr.Args) > 0 {
				for i, pp := range v.Params {
					if i > len(expr.Args)-1 {
						break
					}
					arg := expr.Args[i]
					if TryCast[types.MutType](pp) {
						var id *ast.Ident
						switch vv := arg.(type) {
						case *ast.Ident:
							id = vv
						case *ast.SelectorExpr:
							id = vv.X.(*ast.Ident)
						case *ast.UnaryExpr:
							id = vv.X.(*ast.Ident)
						default:
							panic(fmt.Sprintf("unsupported type %v", to(arg)))
						}
						if !id.Mutable.IsValid() {
							infer.errorf(arg, "%s: missing mut keyword", infer.Pos(arg))
							return
						}
						if !TryCast[types.MutType](infer.env.GetType(id)) {
							infer.errorf(id, "%s: cannot use immutable '%s'", infer.Pos(id), id.Name)
							return
						}
					}
				}
			}
			for i, arg := range expr.Args {
				if vv, ok := infer.env.GetType(arg).(types.OptionType); ok && vv.W == nil {
					infer.SetTypeForce(arg, types.OptionType{W: v.Params[i].(types.OptionType).W})
				}
			}
			if infer.env.GetType(expr) == nil {
				if v.Return != nil {
					toReturn := v.Return
					toReturn = alterResultBubble(infer.returnType, toReturn)
					infer.SetType(expr, toReturn)
				}
			}
		}
	}
	if infer.env.GetType(expr) == nil {
		infer.SetType(expr, types.VoidType{})
	}
}

func (infer *FileInferrer) langFns(expr *ast.CallExpr, call *ast.Ident) {
	fnName := call.Name
	switch fnName {
	case "make":
		fnT := infer.env.GetFn("make")
		if len(expr.Args) == 0 {
			infer.errorf(expr, "'make' must have at least 1 argument")
			return
		}
		arg0 := expr.Args[0]
		switch v := arg0.(type) {
		case *ast.ArrayType, *ast.ChanType, *ast.MapType, *ast.SetType:
			fnT = fnT.T("T", infer.GetType2(v))
			infer.SetType(expr, fnT.Return)
			infer.SetType(expr.Args[0], types.TypeType{W: fnT.GetParam(0)})
		default:
			infer.errorf(arg0, "%v", to(arg0))
			return
		}
	case "min", "max":
		arg0T := infer.GetType2(expr.Args[0])
		arg0T = types.Unwrap(arg0T)
		fnT := infer.env.GetFn(fnName).T("T", arg0T)
		for i := 0; i < len(expr.Args)-2; i++ {
			fnT.Params = append(fnT.Params, arg0T)
		}
		infer.SetType(expr.Fun, fnT)
	case "append":
		fnT := infer.env.GetFn("append")
		arg0 := expr.Args[0]
		arg0T := infer.GetType2(arg0)
		arg0T = types.Unwrap(arg0T)
		switch v := arg0T.(type) {
		case types.ArrayType:
			fnT = fnT.T("T", v.Elt)
			infer.SetType(expr, fnT.Return)
		default:
			infer.errorf(arg0, "%v", to(arg0T))
			return
		}
	case "abs":
		info := infer.env.GetNameInfo("abs")
		fnT := infer.env.GetFn("abs")
		arg0 := expr.Args[0]
		infer.expr(arg0)
		arg0T := infer.env.GetType(arg0)
		fnT = fnT.T("T", arg0T)
		infer.SetType(expr.Fun, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case "pow":
		info := infer.env.GetNameInfo("pow")
		fnT := infer.env.GetFn("pow")
		arg0 := expr.Args[0]
		arg1 := expr.Args[1]
		infer.expr(arg0)
		infer.expr(arg1)
		arg0T := infer.env.GetType(arg0)
		arg1T := infer.env.GetType(arg1)
		fnT = fnT.T("T", arg0T).T("E", arg1T)
		infer.SetType(expr.Fun, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	}
}

func (infer *FileInferrer) inferGoExtensions(expr *ast.CallExpr, idT, oidT types.Type, exprT *ast.SelectorExpr) {
	fnName := exprT.Sel.Name
	var fnT types.FuncType
	info := &Info{}
	switch idTT := idT.(type) {
	case types.IntType:
		switch fnName {
		case "String":
			info = infer.env.GetNameInfo("agl1.Int.String")
			fnT = infer.env.GetFn("agl1.Int.String").IntoRecv(idTT)
		}
		infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.I64Type:
		switch fnName {
		case "String":
			info = infer.env.GetNameInfo("agl1.I64.String")
			fnT = infer.env.GetFn("agl1.I64.String").IntoRecv(idTT)
		}
		infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.UintType:
		switch fnName {
		case "String":
			info = infer.env.GetNameInfo("agl1.Uint.String")
			fnT = infer.env.GetFn("agl1.Uint.String").IntoRecv(idTT)
		}
		infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.StringType, types.UntypedStringType:
		switch fnName {
		case "Replace":
			info = infer.env.GetNameInfo("agl1.String." + fnName)
			fnT = infer.env.GetFn("agl1.String." + fnName).IntoRecv(idTT)
			if len(expr.Args) < 3 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[0])
			infer.SetType(expr.Args[1], fnT.Params[1])
			infer.SetType(expr.Args[2], fnT.Params[2])
		case "ReplaceAll":
			info = infer.env.GetNameInfo("agl1.String." + fnName)
			fnT = infer.env.GetFn("agl1.String." + fnName).IntoRecv(idTT)
			if len(expr.Args) < 2 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[0])
			infer.SetType(expr.Args[1], fnT.Params[1])
		case "Split", "HasPrefix", "HasSuffix", "TrimPrefix", "Contains":
			info = infer.env.GetNameInfo("agl1.String." + fnName)
			fnT = infer.env.GetFn("agl1.String." + fnName).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[0])
		case "Len", "Int", "I8", "I16", "I32", "I64", "Uint", "U8", "U16", "U32", "U64", "F32", "F64", "Lines",
			"Uppercased", "Lowercased", "TrimSpace", "IsEmpty", "AsBytes":
			info = infer.env.GetNameInfo("agl1.String." + fnName)
			fnT = infer.env.GetFn("agl1.String." + fnName).IntoRecv(idTT)
		default:
			fnFullName := fmt.Sprintf("agl1.String.%s", fnName)
			fnTRaw := infer.env.Get(fnFullName)
			if fnTRaw == nil {
				infer.errorf(exprT.Sel, "method '%s' of type String does not exists", fnName)
				return
			}
			return
		}
		infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.SetType:
		exprPos := infer.Pos(expr)
		switch fnName {
		case "Contains", "ContainsWhere":
			info = infer.env.GetNameInfo("agl1.Set." + fnName)
			fnT = infer.env.GetFn("agl1.Set."+fnName).T("T", idTT.K)
			if len(expr.Args) < 1 {
				return
			}
			//switch expr.Args[0].(type) {
			//case *ast.FuncLit, *ast.ShortFuncLit:
			//	exprT.Sel.Name = "ContainsWhere"
			//	envFnName := "agl1.Set.ContainsWhere"
			//}
			infer.SetType(expr.Args[0], fnT.Params[1])
		case "Insert", "Remove", "Union", "FormUnion", "Subtracting", "Subtract", "Intersection", "FormIntersection",
			"SymmetricDifference", "FormSymmetricDifference", "IsSubset", "IsStrictSubset", "IsSuperset", "IsStrictSuperset",
			"IsDisjoint", "Intersects", "Equals":
			info = infer.env.GetNameInfo("agl1.Set." + fnName)
			fnT = infer.env.GetFn("agl1.Set."+fnName).T("T", idTT.K)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[1])
		case "RemoveFirst":
			info = infer.env.GetNameInfo("agl1.Set." + fnName)
			fnT = infer.env.GetFn("agl1.Set."+fnName).T("T", idTT.K)

		case "First":
			if len(expr.Args) > 0 {
				exprArg0 := expr.Args[0]
				if v, ok := exprArg0.(*ast.LabelledArg); ok {
					exprArg0 = v.X
				}
				switch exprArg0.(type) {
				case *ast.FuncLit, *ast.ShortFuncLit:
					exprT.Sel.Name = "FirstWhere"
					envFnName := "agl1.Set.FirstWhere"
					info = infer.env.GetNameInfo(envFnName)
					fnT = infer.env.GetFn(envFnName).T("T", idTT.K).IntoRecv(idTT)
					fnT.Name = "First"
					infer.SetType(exprArg0, fnT.Params[0])
					infer.SetType(expr, fnT.Return)
					ft := fnT.GetParam(0).(types.FuncType)
					if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
						infer.SetType(exprArg0, ft)
					} else if _, ok := exprArg0.(*ast.FuncType); ok {
						ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
						if !compareFunctionSignatures(ftReal, ft) {
							infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
							return
						}
					} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
						if !compareFunctionSignatures(ftReal, ft) {
							infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
							return
						}
					}
					infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
				}
			}
		case "Map":
			info = infer.env.GetNameInfo("agl1.Set.Map")
			fnT = infer.env.GetFn("agl1.Set.Map").T("T", idTT.K).IntoRecv(idTT)
			clbFnT := fnT.GetParam(0).(types.FuncType)
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, fnT.Return)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				fnT = fnT.T("R", rT)
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				infer.expr(exprArg0)
				aT := infer.env.GetType(exprArg0)
				if tmp, ok := aT.(types.FuncType); ok {
					rT := tmp.Return
					fnT = fnT.T("R", rT)
					infer.SetType(expr, types.ArrayType{Elt: rT})
					infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
				}
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			}
		case "Filter":
			info = infer.env.GetNameInfo("agl1.Set.Filter")
			fnT = infer.env.GetFn("agl1.Set.Filter").T("T", idTT.K).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[0])
			infer.SetType(expr, fnT.Return)
			ft := fnT.GetParam(0).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			}
			infer.SetType(expr, types.SetType{K: ft.Params[0]})
			infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		case "Len", "Min", "Max", "Iter", "IsEmpty":
			fnT = infer.env.GetFn("agl1.Set." + fnName)
		}
		if len(fnT.Params) > 0 {
			if TryCast[types.MutType](fnT.Params[0]) {
				if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
					infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'set'", infer.Pos(exprT.Sel), fnName)
					return
				}
				fnT.Recv = []types.Type{types.MutType{W: idT}}
			} else {
				fnT.Recv = []types.Type{idT}
			}
			fnT.Params = fnT.Params[1:]
		}
		infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
		infer.SetType(expr, fnT.Return)
	case types.StructType:
		if fnName == "Sum" {
			rT := idTT.TypeParams[0].(types.GenericType).W
			if infer.indexValue != nil {
				rT = infer.indexValue
			}
			fnT := infer.env.GetFn("Sequence."+fnName).T("T", idTT.TypeParams[0].(types.GenericType).W).T("R", rT)
			fnT.Recv = []types.Type{oidT}
			if len(fnT.Params) > 0 {
				if TryCast[types.MutType](fnT.Params[0]) {
					if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
						infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
						return
					}
				}
				fnT.Params = fnT.Params[1:]
			}
			infer.SetType(exprT.Sel, fnT)
			infer.SetType(expr, fnT.Return)
		} else if fnName == "Filter" || fnName == "Joined" {
			fnT := infer.env.GetFn("Sequence."+fnName).T("T", idTT.TypeParams[0].(types.GenericType).W).IntoRecv(oidT)
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, fnT.GetParam(0))
			infer.SetTypeForce(exprT.Sel, fnT)
			infer.SetType(expr, fnT.Return)
		} else if fnName == "Sorted" {
			fnT := infer.env.GetFn("Sequence."+fnName).T("T", idTT.TypeParams[0].(types.GenericType).W).IntoRecv(oidT)
			infer.SetTypeForce(exprT.Sel, fnT)
			infer.SetType(expr, fnT.Return)
		} else if fnName == "Len" {
			fnT := infer.env.GetFn("Sequence."+fnName).T("T", idTT.TypeParams[0].(types.GenericType).W).IntoRecv(oidT)
			infer.SetTypeForce(exprT.Sel, fnT)
			infer.SetType(expr, fnT.Return)
		}
	case types.ArrayType:
		exprPos := infer.Pos(expr)
		if fnName == "Filter" {
			info := infer.env.GetNameInfo("agl1.Vec.Filter")
			filterFnT := infer.env.GetFn("agl1.Vec.Filter").T("T", idTT.Elt).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], filterFnT.Params[0])
			infer.SetType(expr, filterFnT.Return)
			ft := filterFnT.GetParam(0).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			}
			infer.SetType(expr, types.ArrayType{Elt: ft.Params[0]})
			infer.SetType(exprT.Sel, filterFnT, WithDesc(info.Message))
		} else if fnName == "FirstIndex" {
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			if v, ok := exprArg0.(*ast.LabelledArg); ok {
				exprArg0 = v.X
			}
			switch exprArg0.(type) {
			case *ast.FuncLit, *ast.ShortFuncLit:
				exprT.Sel.Name = "FirstIndexWhere"
				envFnName := "agl1.Vec.FirstIndexWhere"
				info := infer.env.GetNameInfo(envFnName)
				fnT := infer.env.GetFn(envFnName).T("T", idTT.Elt).IntoRecv(idTT)
				fnT.Name = "FirstIndex"
				infer.SetType(exprArg0, fnT.Params[0])
				infer.SetType(expr, fnT.Return)
				ft := fnT.GetParam(0).(types.FuncType)
				if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
					infer.SetType(exprArg0, ft)
				} else if _, ok := exprArg0.(*ast.FuncType); ok {
					ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
					if !compareFunctionSignatures(ftReal, ft) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
						return
					}
				} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
					if !compareFunctionSignatures(ftReal, ft) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
						return
					}
				}
				infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
			default:
				envFnName := "agl1.Vec.FirstIndex"
				sumFnT := infer.env.GetFn(envFnName).T("T", idTT.Elt)
				sumFnT.Recv = []types.Type{oidT}
				if TryCast[types.MutType](sumFnT.Params[0]) {
					if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
						infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
						return
					}
				}
				sumFnT.Params = sumFnT.Params[1:]
				infer.SetType(expr, sumFnT.Return)
				infer.SetType(exprT.Sel, sumFnT)
			}
		} else if fnName == "AllSatisfy" {
			filterFnT := infer.env.GetFn("agl1.Vec.AllSatisfy").T("T", idTT.Elt).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], filterFnT.Params[0])
			infer.SetType(expr, filterFnT.Return)
			ft := filterFnT.GetParam(0).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			}
			infer.SetType(exprT.Sel, filterFnT)
		} else if fnName == "Contains" {
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			if v, ok := exprArg0.(*ast.LabelledArg); ok {
				exprArg0 = v.X
			}
			switch exprArg0.(type) {
			case *ast.FuncLit, *ast.ShortFuncLit:
				exprT.Sel.Name = "ContainsWhere"
				envFnName := "agl1.Vec.ContainsWhere"
				info := infer.env.GetNameInfo(envFnName)
				fnT := infer.env.GetFn(envFnName).T("T", idTT.Elt).IntoRecv(idTT)
				fnT.Name = "Contains"
				infer.SetType(exprArg0, fnT.Params[0])
				infer.SetType(expr, fnT.Return)
				ft := fnT.GetParam(0).(types.FuncType)
				if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
					infer.SetType(exprArg0, ft)
				} else if _, ok := exprArg0.(*ast.FuncType); ok {
					ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
					if !compareFunctionSignatures(ftReal, ft) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
						return
					}
				} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
					if !compareFunctionSignatures(ftReal, ft) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
						return
					}
				}
				infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
			default:
				filterFnT := infer.env.GetFn("agl1.Vec.Contains").T("T", idTT.Elt).IntoRecv(idTT)
				if len(expr.Args) < 1 {
					return
				}
				infer.SetType(exprArg0, filterFnT.Params[0])
				infer.SetType(expr, filterFnT.Return)
				infer.SetType(exprT.Sel, filterFnT)
			}
		} else if fnName == "Any" {
			filterFnT := infer.env.GetFn("agl1.Vec.Any").T("T", idTT.Elt).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], filterFnT.Params[0])
			infer.SetType(expr, filterFnT.Return)
			ft := filterFnT.GetParam(0).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			}
			infer.SetType(exprT.Sel, filterFnT)
		} else if fnName == "Map" {
			info := infer.env.GetNameInfo("agl1.Vec.Map")
			mapFnT := infer.env.GetFn("agl1.Vec.Map").T("T", idTT.Elt).IntoRecv(idTT)
			clbFnT := mapFnT.GetParam(0).(types.FuncType)
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, mapFnT.Return)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				infer.expr(exprArg0)
				aT := infer.env.GetType(exprArg0)
				if tmp, ok := aT.(types.FuncType); ok {
					rT := tmp.Return
					infer.SetType(expr, types.ArrayType{Elt: rT})
					infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
				}
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			}
		} else if fnName == "FilterMap" {
			info := infer.env.GetNameInfo("agl1.Vec.FilterMap")
			mapFnT := infer.env.GetFn("agl1.Vec.FilterMap").T("T", idTT.Elt).IntoRecv(idTT)
			clbFnT := mapFnT.GetParam(0).(types.FuncType)
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, mapFnT.Return)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				var rT types.Type
				switch v := infer.GetTypeFn(arg0).Return.(type) {
				case types.OptionType:
					rT = v.W
				}
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				infer.expr(exprArg0)
				aT := infer.env.GetType(exprArg0)
				if tmp, ok := aT.(types.FuncType); ok {
					var rT types.Type
					switch v := tmp.Return.(type) {
					case types.OptionType:
						rT = v.W
					}
					infer.SetType(expr, types.ArrayType{Elt: rT})
					infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
				}
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			}
		} else if fnName == "Reduce" {
			infer.inferVecReduce(expr, exprT, idTT)
		} else if fnName == "Find" {
			findFnT := infer.env.GetFn("agl1.Vec.Find").T("T", idTT.Elt).IntoRecv(idTT)
			infer.SetType(expr, findFnT.Return)
			if len(expr.Args) < 1 {
				return
			}
			ft := findFnT.GetParam(0).(types.FuncType)
			exprArg0 := expr.Args[0]
			if _, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.SetType(exprArg0, ft)
			} else if _, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				if !compareFunctionSignatures(ftReal, ft) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, ft)
					return
				}
			}
			infer.SetType(expr, types.OptionType{W: ft.Params[0]})
			infer.SetType(exprT.Sel, findFnT)
		} else if InArray(fnName, []string{"Sum", "Push", "Remove", "Clone", "Clear", "Indices", "PushFront",
			"Insert", "Pop", "PopFront", "__ADD", "RemoveFirst"}) {
			fnT := infer.env.GetFn("agl1.Vec."+fnName).T("T", idTT.Elt)
			fnT.Recv = []types.Type{oidT}
			if len(fnT.Params) > 0 {
				if TryCast[types.MutType](fnT.Params[0]) {
					if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
						infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
						return
					}
				}
				fnT.Params = fnT.Params[1:]
			}
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		} else if InArray(fnName, []string{"PopIf"}) {
			fnT := infer.env.GetFn("agl1.Vec.PopIf").T("T", idTT.Elt).IntoRecv(idTT)
			clbT := fnT.GetParam(0).(types.FuncType)
			if TryCast[types.MutType](fnT.Params[0]) {
				if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
					infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
					return
				}
			}
			if _, ok := expr.Args[0].(*ast.ShortFuncLit); ok {
				infer.SetType(expr.Args[0], clbT)
			}
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		} else if InArray(fnName, []string{"With"}) {
			fnT := infer.env.GetFn("agl1.Vec.With").T("T", idTT.Elt)
			clbT := fnT.GetParam(2).(types.FuncType)
			fnT.Recv = []types.Type{oidT}
			if TryCast[types.MutType](fnT.Params[0]) {
				if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
					infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
					return
				}
			}
			fnT.Params = fnT.Params[1:]
			if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
				infer.SetType(expr.Args[1], clbT)
			}
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		} else if fnName == "Swap" {
			fnT := infer.env.GetFn("agl1.Vec.Swap").T("T", idTT.Elt)
			param0 := fnT.Params[0]
			if TryCast[types.MutType](fnT.Params[0]) {
				if infer.mutEnforced && !TryCast[types.MutType](infer.env.GetType(exprT.X)) {
					infer.errorf(exprT.Sel, "%s: method '%s' cannot be called on immutable type 'Vec'", infer.Pos(exprT.Sel), fnName)
					return
				}
			}
			if !cmpTypes(idT, param0) {
				infer.errorf(exprT.Sel, "type mismatch, wants: %s, got: %s", param0, idT)
				return
			}
			infer.SetType(expr, fnT.Return)
			fnT.Recv = []types.Type{param0}
			fnT.Params = fnT.Params[1:]
			infer.SetType(exprT.Sel, fnT)
		} else if InArray(fnName, []string{"Joined"}) {
			fnT := infer.env.GetFn("agl1.Vec." + fnName)
			param0 := fnT.Params[0]
			if !cmpTypes(idT, param0) {
				infer.errorf(exprT.Sel, "type mismatch, wants: %s, got: %s", param0, idT)
				return
			}
			infer.SetType(expr, fnT.Return)
			fnT.Recv = []types.Type{param0}
			fnT.Params = fnT.Params[1:]
			infer.SetType(exprT.Sel, fnT)
		} else if fnName == "Sorted" {
			if len(expr.Args) > 0 {
				if v, ok := expr.Args[0].(*ast.LabelledArg); ok {
					if v.Label != nil && v.Label.Name == "by" {
						exprT.Sel.Name = "SortedBy"
						fnName = "SortedBy"
					}
				}
			}
			info := infer.env.GetNameInfo("agl1.Vec." + fnName)
			fnT := infer.env.GetFn("agl1.Vec."+fnName).T("E", idTT.Elt)
			param0 := fnT.Params[0]
			if !cmpTypes(idT, param0) {
				infer.errorf(exprT.Sel, "type mismatch, wants: %s, got: %s", param0, idT)
				return
			}
			infer.SetType(expr, fnT.Return)
			fnT.Recv = []types.Type{param0}
			fnT.Params = fnT.Params[1:]

			if len(expr.Args) > 0 {
				clbFnT := fnT.GetParam(0).(types.FuncType)
				exprArg0 := expr.Args[0]
				if v, ok := exprArg0.(*ast.LabelledArg); ok {
					exprArg0 = v.X
				}
				if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
					infer.SetType(arg0, clbFnT)
					infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
				} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
					ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
					if !compareFunctionSignatures(ftReal, clbFnT) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
						return
					}
				} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
					infer.expr(exprArg0)
					aT := infer.env.GetType(exprArg0)
					if tmp, ok := aT.(types.FuncType); ok {
						rT := tmp.Return
						infer.SetType(expr, types.ArrayType{Elt: rT})
						infer.SetType(exprT.Sel, fnT, WithDesc(info.Message))
					}
					if !compareFunctionSignatures(ftReal, clbFnT) {
						infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
						return
					}
				}
			}

			infer.SetType(exprT.Sel, fnT)
		} else {
			fnFullName := fmt.Sprintf("agl1.Vec.%s", fnName)
			for _, a := range expr.Args {
				if v, ok := a.(*ast.LabelledArg); ok {
					fnFullName += fmt.Sprintf("_%s", v.Label.Name)
				}
			}

			// Manually add imports for functions in core.agl that are not going to be generated unless they're used
			switch fnFullName {
			case "agl1.Vec.Iter":
				infer.imports["iter"] = &ast.ImportSpec{Path: &ast.BasicLit{Value: `"iter"`}}
			case "agl1.Vec.Shuffled":
				infer.imports["math/rand"] = &ast.ImportSpec{Path: &ast.BasicLit{Value: `"math/rand"`}}
			}

			fnTRaw := infer.env.Get(fnFullName)
			if fnTRaw == nil {
				infer.errorf(exprT.Sel, "%s: method '%s' of type Vec does not exists", infer.Pos(exprT.Sel), fnName)
				return
			}
			fnT := fnTRaw.(types.FuncType)
			assert(len(fnT.TypeParams) >= 1, "agl1.Vec should have at least one generic parameter")
			gen0 := fnT.TypeParams[0].(types.GenericType).W
			want := types.ArrayType{Elt: gen0}
			if !cmpTypes(gen0, idTT.Elt) {
				infer.errorf(exprT.Sel, "%s: cannot use %s as %s for %s", infer.Pos(exprT.Sel), idTT, want, fnName)
				return
			}
			fnT = fnT.T("T", idTT.Elt)
			retT := Or[types.Type](fnT.Return, types.VoidType{})
			infer.SetType(exprT.Sel, fnT)
			infer.SetType(expr.Fun, fnT)
			infer.SetType(expr, retT)
			ft := infer.GetTypeFn(expr.Fun)
			// Go through the arguments and get a mapping of "generic name" to "concrete type" (eg: {"T":int})
			genericMapping := make(map[string]types.Type)
			for i, arg := range expr.Args {
				if v, ok := arg.(*ast.LabelledArg); ok {
					arg = v.X
				}
				if TryCast[*ast.ShortFuncLit](arg) || TryCast[*ast.FuncLit](arg) {
					genFn := ft.GetParam(i)
					infer.SetType(arg, genFn)
					infer.expr(arg)
					concreteFn := infer.env.GetType(arg)
					m := types.FindGen(genFn, concreteFn)
					for _, k := range slices.Sorted(maps.Keys(m)) {
						v := m[k]
						if el, ok := genericMapping[k]; ok {
							if el != v {
								infer.errorf(exprT.Sel, "generic type parameter type mismatch. want: %v, got: %v", el, v)
								return
							}
						}
						genericMapping[k] = v
					}
				}
			}
			for k, v := range genericMapping {
				ft = ft.ReplaceGenericParameter(k, v)
			}
			ft.Recv = []types.Type{idTT}
			infer.SetType(exprT.Sel, ft)
			infer.SetType(expr.Fun, ft)
			infer.SetType(expr, ft.Return)
		}
	case types.MapType:
		if InArray(fnName, []string{"Len"}) {
			getFnT := infer.env.GetFn("agl1.Map."+fnName).T("K", idTT.K).T("V", idTT.V).IntoRecv(idTT)
			infer.SetType(expr, getFnT.Return)
			infer.SetType(exprT.Sel, getFnT)
		} else if InArray(fnName, []string{"Get", "Keys", "Values", "ContainsKey"}) {
			getFnT := infer.env.GetFn("agl1.Map."+fnName).T("K", idTT.K).T("V", idTT.V).IntoRecv(idTT)
			infer.SetType(expr, getFnT.Return)
			infer.SetType(exprT.Sel, getFnT)
		} else if fnName == "Reduce" {
			infer.inferMapReduce(expr, exprT, idTT)
		} else if fnName == "Filter" {
			fnT := infer.env.GetFn("agl1.Map."+fnName).T("K", idTT.K).T("V", idTT.V).IntoRecv(idTT)
			if len(expr.Args) < 1 {
				return
			}
			infer.SetType(expr.Args[0], fnT.Params[0])
			infer.SetType(expr, fnT.Return)
			infer.SetType(exprT.Sel, fnT)
		} else if fnName == "Map" {
			info := infer.env.GetNameInfo("agl1.Map.Map")
			mapFnT := infer.env.GetFn("agl1.Map.Map").T("K", idTT.K).T("V", idTT.V).IntoRecv(idTT)
			clbFnT := mapFnT.GetParam(0).(types.FuncType)
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, mapFnT.Return)
			exprPos := infer.Pos(expr)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				infer.SetType(expr, types.ArrayType{Elt: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				infer.expr(exprArg0)
				aT := infer.env.GetType(exprArg0)
				if tmp, ok := aT.(types.FuncType); ok {
					rT := tmp.Return
					infer.SetType(expr, types.ArrayType{Elt: rT})
					infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
				}
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			}
		}
	case types.OptionType:
		if fnName == "Map" {
			exprPos := infer.Pos(expr)
			info := infer.env.GetNameInfo("agl1.Option.Map")
			mapFnT := infer.env.GetFn("agl1.Option.Map").T("T", idTT.W).IntoRecv(idTT)
			clbFnT := mapFnT.GetParam(0).(types.FuncType)
			if len(expr.Args) < 1 {
				return
			}
			exprArg0 := expr.Args[0]
			infer.SetType(exprArg0, clbFnT)
			infer.SetType(expr, mapFnT.Return)
			if arg0, ok := exprArg0.(*ast.ShortFuncLit); ok {
				infer.expr(arg0)
				rT := infer.GetTypeFn(arg0).Return
				infer.SetTypeForce(expr, types.OptionType{W: rT})
				infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
			} else if arg0, ok := exprArg0.(*ast.FuncType); ok {
				ftReal := funcTypeToFuncType("", arg0, infer.env, infer.fset, false)
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
				infer.expr(exprArg0)
				aT := infer.env.GetType(exprArg0)
				if tmp, ok := aT.(types.FuncType); ok {
					rT := tmp.Return
					infer.SetType(expr, rT)
					infer.SetType(exprT.Sel, mapFnT.T("R", rT), WithDesc(info.Message))
				}
				if !compareFunctionSignatures(ftReal, clbFnT) {
					infer.errorf(exprArg0, "%s: function type %s does not match inferred type %s", exprPos, ftReal, clbFnT)
					return
				}
			}
		}
	}
}

func (infer *FileInferrer) inferVecReduce(expr *ast.CallExpr, exprFun *ast.SelectorExpr, idTArr types.ArrayType) {
	eltT := idTArr.Elt
	forceReturnType := infer.forceReturnType
	forceReturnType = types.Unwrap(forceReturnType)
	fnName := "Reduce"
	if v, ok := expr.Args[0].(*ast.LabelledArg); ok {
		if v.Label != nil && v.Label.Name == "into" {
			fnName = "ReduceInto"
			exprFun.Sel.Name = fnName
		}
	}
	fnT := infer.env.GetFn("agl1.Vec."+fnName).T("T", eltT)
	if fnName == "ReduceInto" {
		fnT.Name = "Reduce"
	}
	if len(expr.Args) == 0 {
		return
	}
	if forceReturnType != nil {
		fnT = fnT.T("R", forceReturnType)
	} else {
		arg0T := infer.GetType2(expr.Args[0])
		if r, ok := arg0T.(types.UntypedNumType); !ok {
			noop(r) // TODO should add restriction on type R (cmp.Comparable?)
			//fnT = fnT.T("R", r)
		}
	}
	if infer.env.GetType(exprFun.Sel) == nil {
		infer.SetType(exprFun.Sel, fnT)
	}
	infer.SetType(expr.Args[1], fnT.Params[2])
	infer.SetType(expr, fnT.Return)
	exprArg0 := expr.Args[0]
	infer.withOptType(exprArg0, forceReturnType, func() {
		infer.expr(exprArg0)
	})
	arg0T := infer.GetType(exprArg0)
	reduceFnT := fnT.IntoRecv(idTArr)
	ft := reduceFnT.GetParam(1).(types.FuncType)
	if forceReturnType != nil {
		arg0T = types.Unwrap(arg0T)
		ft = ft.T("R", forceReturnType)
		reduceFnT = reduceFnT.T("R", forceReturnType)
		if !cmpTypes(arg0T, forceReturnType) {
			infer.errorf(expr, "type mismatch, want: %s, got: %s", forceReturnType, arg0T)
			return
		}
	} else if _, ok := infer.GetType(exprArg0).(types.UntypedNumType); ok {
		eltT = types.Unwrap(eltT)
		if !isNumericType(eltT) {
			eltT = types.IntType{}
		}
		ft = ft.T("R", eltT)
		reduceFnT = reduceFnT.T("R", eltT)
	} else {
		arg0T = types.Unwrap(arg0T)
		ft = ft.T("R", arg0T)
		reduceFnT = reduceFnT.T("R", arg0T)
	}
	if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
		infer.SetType(expr.Args[1], ft)
	} else if v, ok := exprArg0.(*ast.FuncType); ok {
		ftReal := funcTypeToFuncType("", v, infer.env, infer.fset, false)
		if !compareFunctionSignatures(ftReal, ft) {
			infer.errorf(expr, "function type %s does not match inferred type %s", ftReal, ft)
			return
		}
	} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
		if !compareFunctionSignatures(ftReal, ft) {
			infer.errorf(expr, "function type %s does not match inferred type %s", ftReal, ft)
			return
		}
	}
	infer.SetTypeForce(exprFun.Sel, reduceFnT)
	infer.SetType(expr.Fun, reduceFnT)
	infer.SetType(expr, reduceFnT.Return)
}

func (infer *FileInferrer) inferMapReduce(expr *ast.CallExpr, exprFun *ast.SelectorExpr, idTMap types.MapType) {
	forceReturnType := infer.forceReturnType
	forceReturnType = types.Unwrap(forceReturnType)
	fnName := "Reduce"
	if v, ok := expr.Args[0].(*ast.LabelledArg); ok {
		if v.Label != nil && v.Label.Name == "into" {
			exprFun.Sel.Name = "ReduceInto"
			fnName = "ReduceInto"
		}
	}
	fnT := infer.env.GetFn("agl1.Map."+fnName).T("K", idTMap.K).T("V", idTMap.V)
	if fnName == "ReduceInto" {
		fnT.Name = "Reduce"
	}
	if len(expr.Args) == 0 {
		return
	}
	if forceReturnType != nil {
		fnT = fnT.T("R", forceReturnType)
	} else {
		arg0T := infer.GetType2(expr.Args[0])
		if r, ok := arg0T.(types.UntypedNumType); !ok {
			noop(r) // TODO should add restriction on type R (cmp.Comparable?)
			//fnT = fnT.T("R", r)
		}
	}
	infer.SetType(exprFun.Sel, fnT)
	infer.SetType(expr.Args[1], fnT.Params[2])
	infer.SetType(expr, fnT.Return)
	exprArg0 := expr.Args[0]
	infer.withOptType(exprArg0, forceReturnType, func() {
		infer.expr(exprArg0)
	})
	arg0T := infer.GetType(exprArg0)
	reduceFnT := infer.env.GetType(exprFun.Sel).(types.FuncType)
	ft := reduceFnT.GetParam(2).(types.FuncType)
	if forceReturnType != nil {
		arg0T = types.Unwrap(arg0T)
		ft = ft.T("R", forceReturnType)
		reduceFnT = reduceFnT.T("R", forceReturnType)
		if !cmpTypes(arg0T, forceReturnType) {
			infer.errorf(expr, "type mismatch, want: %s, got: %s", forceReturnType, arg0T)
			return
		}
		//} else if _, ok := infer.GetType(exprArg0).(types.UntypedNumType); ok {
		//	eltT = types.Unwrap(eltT)
		//	ft = ft.T("R", eltT)
		//	reduceFnT = reduceFnT.T("R", eltT)
	} else {
		arg0T = types.Unwrap(arg0T)
		ft = ft.T("R", arg0T)
		reduceFnT = reduceFnT.T("R", arg0T)
	}
	if _, ok := expr.Args[1].(*ast.ShortFuncLit); ok {
		infer.SetType(expr.Args[1], ft)
	} else if _, ok := exprArg0.(*ast.FuncType); ok {
		ftReal := funcTypeToFuncType("", exprArg0.(*ast.FuncType), infer.env, infer.fset, false)
		if !compareFunctionSignatures(ftReal, ft) {
			infer.errorf(expr, "function type %s does not match inferred type %s", ftReal, ft)
			return
		}
	} else if ftReal, ok := infer.env.GetType(exprArg0).(types.FuncType); ok {
		if !compareFunctionSignatures(ftReal, ft) {
			infer.errorf(expr, "function type %s does not match inferred type %s", ftReal, ft)
			return
		}
	}
	reduceFnT.Recv = []types.Type{idTMap}
	reduceFnT.Params = reduceFnT.Params[1:]
	infer.SetTypeForce(exprFun.Sel, reduceFnT)
	infer.SetType(expr.Fun, reduceFnT)
	infer.SetType(expr, reduceFnT.Return)
}

func alterResultBubble(fnReturn, curr types.Type) (out types.Type) {
	if fnReturn == nil {
		return
	}
	fnReturnIsResult := TryCast[types.ResultType](fnReturn)
	fnReturnIsOption := TryCast[types.OptionType](fnReturn)
	currIsResult := TryCast[types.ResultType](curr)
	currIsOption := TryCast[types.OptionType](curr)
	out = curr
	if fnReturnIsResult && currIsResult {
		tmp := MustCast[types.ResultType](curr)
		tmp.Bubble = true
		out = tmp
	} else if fnReturnIsOption && currIsOption {
		tmp := MustCast[types.OptionType](curr)
		tmp.Bubble = true
		out = tmp
	} else if currIsResult {
		tmp := MustCast[types.ResultType](curr)
		if fnReturnIsOption {
			fnReturnOpt := MustCast[types.OptionType](fnReturn)
			tmp.Bubble = true
			tmp.ConvertToNone = true
			tmp.ToNoneType = fnReturnOpt.W
			out = tmp
		} else {
			tmp.Bubble = false
			out = tmp
		}
	}
	if !fnReturnIsOption {
		if tmp, ok := curr.(types.OptionType); ok {
			tmp.Bubble = false
			out = tmp
		}
	}
	return
}

func (infer *FileInferrer) funcLit(expr *ast.FuncLit) {
	if infer.optType.IsDefinedFor(expr) {
		infer.SetType(expr, infer.optType.Type)
	}
	ft := funcTypeToFuncType("", expr.Type, infer.env, infer.fset, false)
	// implicit return
	if len(expr.Body.List) == 1 && TryCast[*ast.ExprStmt](expr.Body.List[0]) && !TryCast[types.VoidType](ft.Return) {
		returnStmt := expr.Body.List[0].(*ast.ExprStmt)
		expr.Body.List = []ast.Stmt{&ast.ReturnStmt{Result: returnStmt.X}}
	}
	infer.SetType(expr, ft)
	infer.withEnv(func() {
		if expr.Type.Params != nil {
			for _, field := range expr.Type.Params.List {
				infer.expr(field.Type)
				t := infer.GetType2(field.Type)
				for _, name := range field.Names {
					if name.Mutable.IsValid() {
						t = types.MutType{W: t}
					}
					infer.env.Define(name, name.Name, t)
					infer.SetType(name, t)
				}
			}
		}
		if expr.Type.Result != nil {
			infer.expr(expr.Type.Result)
		}
		infer.withReturnType(ft.Return, func() {
			infer.stmt(expr.Body)
		})
	})
}

func (infer *FileInferrer) defineDestructuredTuple(vals []ast.Expr, t types.TupleType) {
	for i, e := range vals {
		switch v := e.(type) {
		case *ast.Ident:
			infer.env.Define(nil, v.Name, t.Elts[i])
			infer.SetType(v, t.Elts[i])
		case *ast.TupleExpr:
			infer.defineDestructuredTuple(v.Values, t.Elts[i].(types.TupleType))
		}
	}
}

func (infer *FileInferrer) shortFuncLit(expr *ast.ShortFuncLit) {
	infer.withEnv(func() {
		if infer.optType.IsDefinedFor(expr) {
			infer.SetType(expr, infer.optType.Type)
		}
		// Define args shortcuts in environment ($0, $1...)
		if t := infer.env.GetType(expr); t != nil {
			var params []types.Type
			switch v := t.(type) {
			case types.FuncType:
				params = v.Params
			case types.LabelledType:
				params = v.W.(types.FuncType).Params
			default:
				panic("")
			}
			for i, arg := range expr.Args {
				switch v := arg.(type) {
				case *ast.Ident:
					infer.env.Define(nil, v.Name, params[i])
					infer.SetType(arg, params[i])
				case *ast.TupleExpr:
					infer.defineDestructuredTuple(v.Values, params[i].(types.TupleType))
				default:
					panic("")
				}
			}
			for i, param := range params {
				infer.env.Define(nil, fmt.Sprintf("$%d", i), param)
			}
		}
		inferExpr := func(returnStmt ast.Expr, ft types.FuncType) {
			if infer.env.GetType(returnStmt) != nil && infer.env.GetType(expr) != nil {
				if t, ok := ft.Return.(types.ArrayType); ok {
					ft.Return = t.Elt
				}
				if t, ok := ft.Return.(types.OptionType); ok {
					ft.Return = t.W
				}
				if t, ok := ft.Return.(types.ResultType); ok {
					ft.Return = t.W
				}
				if t, ok := ft.Return.(types.GenericType); ok {
					ft = ft.T(t.Name, infer.env.GetType(returnStmt))
					infer.SetType(expr, ft)
				}
			}
		}
		// implicit return
		ftTmp := infer.env.GetType(expr)
		if v, ok := ftTmp.(types.LabelledType); ok {
			ftTmp = v.W
		}
		ft := ftTmp.(types.FuncType)
		infer.withReturnType(ft.Return, func() {
			infer.stmt(expr.Body)
		})
		switch v := expr.Body.(type) {
		case *ast.BlockStmt:
		default:
			expr.Body = &ast.BlockStmt{List: []ast.Stmt{v}}
		}
		bodyBlock := expr.Body.(*ast.BlockStmt)
		lastStmt := func() ast.Stmt { return Must(Last(bodyBlock.List)) }
		voidReturnStmt := &ast.ReturnStmt{Result: &ast.CompositeLit{Type: &ast.Ident{Name: "void"}}}
		multStmt := len(bodyBlock.List) > 0
		singleExprStmt := len(bodyBlock.List) == 1 && TryCast[*ast.ExprStmt](bodyBlock.List[0])
		retIsVoid := (TryCast[types.TypeType](ft.Return) && TryCast[types.VoidType](ft.Return.(types.TypeType).W)) || TryCast[types.VoidType](ft.Return)
		lastIsRetStmt := func() bool { return TryCast[*ast.ReturnStmt](lastStmt()) }
		if (singleExprStmt || (multStmt && !lastIsRetStmt())) && retIsVoid {
			bodyBlock.List = append(bodyBlock.List, voidReturnStmt)
		} else if multStmt && lastIsRetStmt() {
			returnStmt := lastStmt().(*ast.ReturnStmt).Result
			inferExpr(returnStmt, ft)
		} else if singleExprStmt {
			returnStmt := lastStmt().(*ast.ExprStmt).X
			inferExpr(returnStmt, ft)
			bodyBlock.List = []ast.Stmt{&ast.ReturnStmt{Result: returnStmt}}
		}
		// expr type is set in CallExpr
	})
}

func (infer *FileInferrer) funcType(expr *ast.FuncType) {
	//infer.expr(expr.TypeParams)
	//infer.expr(expr.Params)
	var paramsT []types.Type
	if expr.Params != nil {
		for _, param := range expr.Params.List {
			infer.expr(param.Type)
			t := infer.env.GetType(param.Type)
			n := max(len(param.Names), 1)
			for i := 0; i < n; i++ {
				paramsT = append(paramsT, t)
			}
		}
	}
	var returnT types.Type = types.VoidType{}
	if expr.Result != nil {
		infer.expr(expr.Result)
		returnT = infer.env.GetType(expr.Result)
		if returnT == nil {
			returnT = types.VoidType{}
		}
	}
	ft := types.FuncType{
		Params: paramsT,
		Return: returnT,
	}
	infer.SetType(expr, ft)
}

func (infer *FileInferrer) voidExpr(expr *ast.VoidExpr) {
	infer.SetType(expr, types.VoidType{})
}

func (infer *FileInferrer) someExpr(expr *ast.SomeExpr) {
	var t types.Type
	if infer.optType.IsDefinedFor(expr) && TryCast[types.OptionType](infer.optType.Type) {
		t = infer.optType.Type
		infer.withOptType(expr.X, t.(types.OptionType).W, func() {
			infer.expr(expr.X)
		})
	} else {
		infer.expr(expr.X)
		t = types.OptionType{W: infer.env.GetType(expr.X)}
	}
	infer.SetType(expr, t)
}

func (infer *FileInferrer) noneExpr(expr *ast.NoneExpr) {
	currT := infer.env.GetType(expr)
	if v, ok := currT.(types.OptionType); ok && v.W != nil {
		return
	}
	var t types.Type
	if infer.optType1 != nil {
		t = infer.optType1.Type
	}
	infer.SetType(expr, types.OptionType{W: t})
}

func (infer *FileInferrer) okExpr(expr *ast.OkExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.ResultType{W: infer.env.GetType(expr.X)})
}

func (infer *FileInferrer) errExpr(expr *ast.ErrExpr) {
	infer.expr(expr.X)
	// turn `Err("error")` into `Err(Errors.New("error"))`
	if v, ok := expr.X.(*ast.BasicLit); ok && v.Kind == token.STRING {
		expr.X = &ast.CallExpr{Fun: &ast.SelectorExpr{X: &ast.Ident{Name: "Errors"}, Sel: &ast.Ident{Name: "New"}}, Args: []ast.Expr{v}}
	}
	var t types.Type
	if infer.optType != nil {
		t = infer.optType.Type
	} else {
		t = infer.returnType
	}
	infer.SetType(expr, types.ResultType{W: t.(types.ResultType).W})
}

func (infer *FileInferrer) chanType(expr *ast.ChanType) {
	infer.expr(expr.Value)
}

func (infer *FileInferrer) unaryExpr(expr *ast.UnaryExpr) {
	infer.expr(expr.X)
	if expr.Op == token.AND {
		infer.SetType(expr, types.StarType{X: infer.GetType(expr.X)})
	} else {
		infer.SetType(expr, infer.GetType2(expr.X))
	}
}

func (infer *FileInferrer) typeAssertExpr(expr *ast.TypeAssertExpr) {
	infer.expr(expr.X)
	if expr.Type != nil {
		infer.expr(expr.Type)
	}
	if expr.Type != nil {
		//_, bubble := infer.returnType.(types.OptionType)
		//t := types.OptionType{W: infer.GetType2(expr.Type), Bubble: bubble}
		infer.SetType(expr, types.TypeAssertType{X: infer.GetType2(expr.X), Type: infer.GetType2(expr.Type)})
	} else if infer.env.GetType(expr) == nil {
		infer.SetType(expr, types.VoidType{})
	}
}

func (infer *FileInferrer) orBreak(expr *ast.OrBreakExpr) {
	infer.expr(expr.X)
	var t types.Type
	switch v := infer.GetType(expr.X).(type) {
	case types.OptionType:
		t = v.W
	case types.ResultType:
		t = v.W
	default:
		infer.errorf(expr, "expected Option or Result type, got %v", v)
		return
	}
	infer.SetType(expr, t)
}

func (infer *FileInferrer) orContinue(expr *ast.OrContinueExpr) {
	infer.expr(expr.X)
	var t types.Type
	switch v := infer.GetType(expr.X).(type) {
	case types.OptionType:
		t = v.W
	case types.ResultType:
		t = v.W
	default:
		infer.errorf(expr, "expected Option or Result type, got %v", v)
		return
	}
	infer.SetType(expr, t)
}

func (infer *FileInferrer) orReturn(expr *ast.OrReturnExpr) {
	infer.expr(expr.X)
	xT := infer.GetType(expr.X)
	switch v := xT.(type) {
	case types.ResultType:
		infer.SetType(expr, v.W)
	case types.OptionType:
		infer.SetType(expr, v.W)
	}
}

func (infer *FileInferrer) mapType(expr *ast.MapType) {
	infer.expr(expr.Key)
	infer.expr(expr.Value)
	kT := infer.GetType(expr.Key)
	vT := infer.GetType(expr.Value)
	infer.SetType(expr.Key, kT)
	infer.SetType(expr.Value, vT)
	infer.SetType(expr, types.MapType{K: kT, V: vT})
}

func (infer *FileInferrer) starExpr(expr *ast.StarExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.StarType{X: infer.GetType(expr.X)})
}

func (infer *FileInferrer) ellipsis(expr *ast.Ellipsis) {
	infer.expr(expr.Elt)
	infer.SetType(expr, types.EllipsisType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) tupleExpr(expr *ast.TupleExpr) {
	infer.exprs(expr.Values)
	var elts []types.Type
	if infer.optType.IsDefinedFor(expr) {
		expected := infer.optType.Type.(types.TupleType).Elts
		for i, x := range expr.Values {
			expectedI := expected[i]
			xT := infer.GetType(x)
			if _, ok := xT.(types.UntypedNumType); ok {
				infer.SetType(x, expectedI)
				xT = expectedI
			} else if !cmpTypesLoose(xT, expectedI) {
				infer.errorf(expr.Values[i], "%s: type mismatch, want: %s, got: %s", infer.Pos(expr.Values[i]), expectedI, xT)
				return
			}
			elts = append(elts, xT)
		}
	} else {
		for _, v := range expr.Values {
			elT := infer.GetType(v)
			elts = append(elts, elT)
		}
	}
	infer.SetType(expr, types.TupleType{Elts: elts})
}

func compareFunctionSignatures(sig1, sig2 types.FuncType) bool {
	// Compare return types
	if !cmpTypes(sig1.Return, sig2.Return) {
		return false
	}
	// Compare number of parameters
	if len(sig1.Params) != len(sig2.Params) {
		return false
	}
	//// Compare variadic status
	//if sig1.variadic != sig2.variadic {
	//	return false
	//}
	// Compare each parameter type
	for i := range sig1.Params {
		if !cmpTypes(sig1.Params[i], sig2.Params[i]) {
			return false
		}
	}
	// Compare type parameters if they exist
	if len(sig1.TypeParams) != len(sig2.TypeParams) { // TODO
		return false
	}
	for i := range sig1.TypeParams {
		if !cmpTypes(sig1.TypeParams[i], sig2.TypeParams[i]) {
			return false
		}
	}
	return true
}

func cmpTypesLoose(a, b types.Type) bool {
	a = types.Unwrap(a)
	b = types.Unwrap(b)
	if isNumericType(a) && TryCast[types.UntypedNumType](b) {
		return true
	}
	if isNumericType(b) && TryCast[types.UntypedNumType](a) {
		return true
	}
	if TryCast[types.UntypedStringType](a) && TryCast[types.StringType](b) {
		return true
	}
	if TryCast[types.UntypedStringType](b) && TryCast[types.StringType](a) {
		return true
	}
	return cmpTypes(a, b)
}

func cmpTypes(a, b types.Type) bool {
	a = types.Unwrap(a)
	b = types.Unwrap(b)
	if TryCast[types.GenericType](a) || TryCast[types.GenericType](b) {
		return true
	}
	if aa, ok := a.(types.FuncType); ok {
		if bb, ok := b.(types.FuncType); ok {
			if aa.GoStr() == bb.GoStr() {
				return true
			}
			if !cmpTypes(aa.Return, bb.Return) {
				return false
			}
			if len(aa.Params) != len(bb.Params) {
				return false
			}
			for i := range aa.Params {
				if !cmpTypes(aa.Params[i], bb.Params[i]) {
					return false
				}
			}
			return true
		}
		return false
	}
	if aa, ok := a.(types.TupleType); ok {
		if bb, ok := b.(types.TupleType); ok {
			if len(aa.Elts) != len(bb.Elts) {
				return false
			}
			for i := range aa.Elts {
				if !cmpTypesLoose(aa.Elts[i], bb.Elts[i]) {
					return false
				}
			}
			return true
		}
		return false
	}
	if TryCast[types.AnyType](a) || TryCast[types.AnyType](b) {
		return true
	}
	if TryCast[types.BoolType](a) && TryCast[types.BoolType](b) {
		return a == b
	}
	if TryCast[types.MapType](a) && TryCast[types.MapType](b) {
		aa := MustCast[types.MapType](a)
		bb := MustCast[types.MapType](b)
		return cmpTypesLoose(aa.K, bb.K) && cmpTypesLoose(aa.V, bb.V)
	}
	if TryCast[types.SetType](a) && TryCast[types.SetType](b) {
		aa := MustCast[types.SetType](a)
		bb := MustCast[types.SetType](b)
		return cmpTypesLoose(aa.K, bb.K)
	}
	if TryCast[types.EllipsisType](a) && TryCast[types.EllipsisType](b) {
		aa := MustCast[types.EllipsisType](a)
		bb := MustCast[types.EllipsisType](b)
		return cmpTypesLoose(aa.Elt, bb.Elt)
	}
	if TryCast[types.StructType](a) || TryCast[types.StructType](b) {
		return true // TODO
	}
	if TryCast[types.InterfaceType](a) || TryCast[types.InterfaceType](b) {
		return true // TODO
	}
	if TryCast[types.ArrayType](a) && TryCast[types.ArrayType](b) {
		aa := MustCast[types.ArrayType](a)
		bb := MustCast[types.ArrayType](b)
		return cmpTypesLoose(aa.Elt, bb.Elt)
	}
	if TryCast[types.EnumType](a) || TryCast[types.EnumType](b) {
		return true // TODO
	}
	if TryCast[types.BinaryType](a) || TryCast[types.BinaryType](b) {
		return true // TODO
	}
	if TryCast[types.StarType](a) && TryCast[types.StarType](b) {
		return cmpTypesLoose(a.(types.StarType).X, b.(types.StarType).X)
	}
	if TryCast[types.OptionType](a) && TryCast[types.OptionType](b) {
		if a.(types.OptionType).W == nil || b.(types.OptionType).W == nil {
			return true
		}
		return cmpTypesLoose(a.(types.OptionType).W, b.(types.OptionType).W)
	}
	if TryCast[types.ResultType](a) && TryCast[types.ResultType](b) {
		return cmpTypesLoose(a.(types.ResultType).W, b.(types.ResultType).W)
	}
	if TryCast[types.IndexListType](a) && TryCast[types.IndexListType](b) {
		return cmpTypesLoose(a.(types.IndexListType).X, b.(types.IndexListType).X)
	}
	if TryCast[types.IndexType](a) && TryCast[types.IndexType](b) {
		return cmpTypesLoose(a.(types.IndexType).X, b.(types.IndexType).X)
	}
	return a == b
}

func (infer *FileInferrer) selectorExpr(expr *ast.SelectorExpr) {
	infer.expr(expr.X)
	exprXT := types.Unwrap(infer.GetType2(expr.X))
	if exprXT == nil {
		infer.errorf(expr.X, "%s: type not found for '%s' %v", infer.Pos(expr.X), expr.X, to(expr.X))
		return
	}
	switch exprXTT := exprXT.(type) {
	case types.StructType:
		infer.inferStructType(exprXTT, expr)
		return
	case types.InterfaceType:
		return
	case types.EnumType:
		info := infer.env.GetInfo(expr.X)
		infer.SetType(expr.X, exprXTT)
		infer.SetType(expr.Sel, exprXTT, WithDefinition(info))
		enumName := expr.X.(*ast.Ident).Name
		fieldName := expr.Sel.Name
		validFields := make([]string, 0, len(exprXTT.Fields))
		for _, f := range exprXTT.Fields {
			validFields = append(validFields, f.Name)
		}
		if !InArray(fieldName, validFields) {
			infer.errorf(expr.Sel, "%d: enum %s has no field %s", expr.Sel.Pos(), enumName, fieldName)
			return
		}
		infer.SetType(expr, exprXTT)
	case types.TupleType:
		infer.SetType(expr.X, exprXTT)
		argIdx, err := strconv.Atoi(expr.Sel.Name)
		if err != nil {
			infer.errorf(expr.Sel, "tuple arg index must be int")
			return
		}
		infer.SetType(expr.Sel, exprXTT.Elts[argIdx])
		infer.SetType(expr, exprXTT.Elts[argIdx])
	case types.PackageType:
		pkg := expr.X.(*ast.Ident).Name
		sel := expr.Sel.Name
		selT := infer.env.Get(pkg + "." + sel)
		selTInfo := infer.env.GetNameInfo(pkg + "." + sel)
		if selT == nil {
			infer.errorf(expr.Sel, "'%s' not found in package '%s'", sel, pkg)
			return
		}
		infer.SetType(expr.Sel, selT, WithDefinition1(selTInfo.Definition1))
		infer.SetType(expr, selT)
	case types.TypeAssertType:
		exprXTT.Type = types.Unwrap(exprXTT.Type)
		if v, ok := exprXTT.Type.(types.StructType); ok {
			fieldName := expr.Sel.Name
			name := v.GetFieldName(fieldName)
			if f := infer.env.Get(name); f != nil {
				infer.SetType(expr.X, v)
				infer.SetType(expr.Sel, f)
				infer.SetType(expr, f)
				return
			}
		}
		infer.SetType(expr.X, exprXTT.X)
		infer.SetType(expr, exprXTT.Type)
	case types.OptionType:
		infer.SetType(expr.X, exprXTT)
		infer.SetType(expr.Sel, exprXTT.W)
		infer.SetType(expr, exprXTT.W)
	case types.TypeType:
		infer.SetType(expr.X, exprXTT)
		infer.SetType(expr.Sel, exprXTT.W)
		infer.SetType(expr, exprXTT.W)
	default:
		infer.errorf(expr.X, "%v", to(exprXT))
		return
	}
}

func (infer *FileInferrer) bubbleResultExpr(expr *ast.BubbleResultExpr) {
	infer.expr(expr.X)
	exprXT := infer.GetType(expr.X)
	if v, ok := exprXT.(types.ResultType); ok {
		infer.SetType(expr, v.W)
	} else {
		infer.errorf(expr, "expected Result type, got %v", exprXT)
		return
	}
}

func (infer *FileInferrer) bubbleOptionExpr(expr *ast.BubbleOptionExpr) {
	infer.expr(expr.X)
	exprXT := infer.GetType(expr.X)
	if exprXT == nil {
		return
	}
	switch v := exprXT.(type) {
	case types.OptionType:
		infer.SetType(expr, v.W)
	case types.TypeAssertType:
		infer.SetType(expr, v.X)
	default:
		infer.errorf(expr, "expected Option type, got %v", exprXT)
		return
	}
}

func (infer *FileInferrer) compositeLit(expr *ast.CompositeLit) {
	if expr.Type == nil {
		if infer.optType != nil {
			switch v := infer.optType.Type.(type) {
			case types.StructType:
				for _, elExpr := range expr.Elts {
					infer.SetType(elExpr, infer.optType.Type)
					switch vv := elExpr.(type) {
					case *ast.KeyValueExpr:
						k := fmt.Sprintf("%s.%s", v.Name, vv.Key.(*ast.Ident).Name)
						kvT := infer.env.Get(k)
						infer.SetType(vv.Value, kvT)
					}
				}
			}
			infer.SetType(expr, infer.optType.Type)
		}
		return
	}
	switch v := expr.Type.(type) {
	case *ast.IndexExpr:
		t := infer.env.Get(v.X.(*ast.Ident).Name)
		infer.SetType(v.X, t)
		infer.SetType(expr, t)
		return
	case *ast.IndexListExpr:
		t := infer.env.Get(v.X.(*ast.Ident).Name)
		infer.SetType(v.X, t)
		infer.SetType(expr, t)
		return
	case *ast.ArrayType:
		t := infer.GetType2(v.Elt)
		for _, elExpr := range expr.Elts {
			infer.withOptType(elExpr, t, func() {
				infer.expr(elExpr)
			})
		}
		infer.expr(expr.Type)
		infer.SetType(v.Elt, t)
		infer.SetType(expr, types.ArrayType{Elt: t})
		return
	case *ast.Ident:
		infer.SetType(expr, infer.env.Get(v.Name))
		for _, elExpr := range expr.Elts {
			switch v1 := elExpr.(type) {
			case *ast.KeyValueExpr:
				infer.expr(v1.Value)
			case *ast.BasicLit:
				infer.expr(v1)
			case *ast.CallExpr:
				infer.expr(v1)
			case *ast.CompositeLit:
				infer.expr(v1)
			default:
				infer.errorf(elExpr, "%v", to(elExpr))
				return
			}
		}
		return
	case *ast.MapType:
		keyT := infer.GetType2(v.Key)
		valT := infer.GetType2(v.Value)
		infer.withMapKV(keyT, valT, func() {
			infer.exprs(expr.Elts)
		})
		infer.SetType(expr, types.MapType{K: keyT, V: valT})
		return
	case *ast.SetType:
		keyT := infer.GetType2(v.Key)
		infer.exprs(expr.Elts)
		t := types.SetType{K: keyT}
		if !TryCast[types.TypeType](keyT) {
			keyT = types.TypeType{W: keyT}
		}
		infer.SetType(v.Key, keyT)
		infer.SetType(expr.Type, t)
		infer.SetType(expr, t)
		return
	case *ast.SelectorExpr:
		idName := v.X.(*ast.Ident).Name
		xT := infer.env.Get(idName)
		selT := infer.env.Get(fmt.Sprintf("%s.%s", idName, v.Sel.Name))
		if expr.Elts != nil {
			for _, el := range expr.Elts {
				switch vv := el.(type) {
				case *ast.KeyValueExpr:
					infer.expr(vv.Value)
					name := fmt.Sprintf("%s.%s.%s", idName, v.Sel.Name, vv.Key.(*ast.Ident).Name)
					infer.SetType(vv.Key, infer.env.Get(name))
				default:
					infer.errorf(el, "%v", to(el))
					return
				}
			}
		}
		infer.SetType(v.X, xT)
		infer.SetType(v.Sel, selT)
		infer.SetType(expr, selT)
		return
	case *ast.StructType:
		if v.Fields != nil {
			for _, f := range v.Fields.List {
				infer.expr(f.Type)
				for _, n := range f.Names {
					infer.SetType(n, infer.GetType(f.Type))
				}
			}
		}
		return
	default:
		infer.errorf(expr, "%v", to(expr.Type))
		return
	}
}

func (infer *FileInferrer) arrayType(expr *ast.ArrayType) {
	if expr.Len != nil {
		infer.expr(expr.Len)
	}
	infer.expr(expr.Elt)
	infer.SetType(expr, types.ArrayType{Elt: infer.GetType(expr.Elt)})
}

func (infer *FileInferrer) indexListExpr(expr *ast.IndexListExpr) {
	infer.expr(expr.X)
	infer.exprs(expr.Indices)
	var indices []types.FieldType
	//for _, e := range expr.Indices {
	//	indices = append(indices)
	//}
	//fmt.Println("???", e)
	//Indices: infer.GetType(expr.Indices)
	infer.SetType(expr, types.IndexListType{X: infer.GetType(expr.X), Indices: indices})
}

func (infer *FileInferrer) parenExpr(expr *ast.ParenExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, infer.GetType(expr.X))
}

func (infer *FileInferrer) structTypeExpr(expr *ast.StructType) {
	//infer.expr(expr.Fields)
	if expr.Fields != nil {
		for _, f := range expr.Fields.List {
			infer.expr(f.Type)
		}
	}
	infer.SetType(expr, types.StructType{})
}

func (infer *FileInferrer) setTypeExpr(expr *ast.SetType) {
	infer.expr(expr.Key)
	kT := infer.GetType2(expr.Key)
	infer.SetType(expr, types.SetType{K: kT})
}

func (infer *FileInferrer) labelledArg(expr *ast.LabelledArg) {
	infer.expr(expr.X)
	t := infer.GetType2(expr.X)
	infer.SetType(expr, t)
}

func (infer *FileInferrer) rangeExpr(expr *ast.RangeExpr) {
	infer.expr(expr.Start)
	infer.expr(expr.End_)
	sT := infer.GetType2(expr.Start)
	eT := infer.GetType2(expr.End_)
	t := sT
	if TryCast[types.UntypedNumType](t) {
		t = eT
	}
	infer.SetType(expr.Start, t)
	infer.SetType(expr.End_, t)
	infer.SetType(expr, types.RangeType{Typ: t})
}

func (infer *FileInferrer) dumpExpr(expr *ast.DumpExpr) {
	infer.expr(expr.X)
}

func (infer *FileInferrer) sliceExpr(expr *ast.SliceExpr) {
	infer.expr(expr.X)
	if expr.Low != nil {
		infer.expr(expr.Low)
		infer.SetType(expr.Low, infer.GetType(expr.Low))
	}
	if expr.High != nil {
		infer.expr(expr.High)
	}
	if expr.Max != nil {
		infer.expr(expr.Max)
		infer.SetType(expr.Max, infer.GetType(expr.Max))
	}
	infer.SetType(expr, infer.GetType(expr.X))
}

func (infer *FileInferrer) interfaceType(expr *ast.InterfaceType) {
	// TODO
}

func (infer *FileInferrer) keyValueExpr(expr *ast.KeyValueExpr) {
	infer.expr(expr.Key)
	switch v := expr.Value.(type) {
	case *ast.CompositeLit:
		if v.Type == nil && infer.mapVT == nil {
			infer.errorf(expr.Value, "map key type not specified")
			return
		}
	default:
		infer.expr(expr.Value)
	}
	//infer.SetType(expr,) // TODO
}

func (infer *FileInferrer) indexExpr(expr *ast.IndexExpr) {
	infer.expr(expr.X)
	infer.expr(expr.Index)
	if TryCast[types.UntypedNumType](infer.GetType(expr.Index)) {
		infer.SetType(expr.Index, types.IntType{})
	}
	exprXT := infer.GetType2(expr.X)
	isMut := TryCast[types.MutType](exprXT)
	switch v := types.Unwrap(exprXT).(type) {
	case types.MapType:
		t := v.V
		if isMut {
			t = types.MutType{W: t}
		}
		infer.SetType(expr, t) // TODO should return an Option[T] ?
	case types.ArrayType:
		t := v.Elt
		if isMut {
			t = types.MutType{W: t}
		}
		infer.SetType(expr, t)
	default:
		infer.SetType(expr, infer.GetType(expr.X))
	}
}

func (infer *FileInferrer) resultExpr(expr *ast.ResultExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.ResultType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) optionExpr(expr *ast.OptionExpr) {
	infer.expr(expr.X)
	infer.SetType(expr, types.OptionType{W: infer.GetType(expr.X)})
}

func (infer *FileInferrer) declStmt(stmt *ast.DeclStmt) {
	switch d := stmt.Decl.(type) {
	case *ast.GenDecl:
		infer.specs(d.Specs)
	}
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) specs(s []ast.Spec) {
	for _, spec := range s {
		infer.spec(spec)
	}
}

func (infer *FileInferrer) spec(s ast.Spec) {
	switch spec := s.(type) {
	case *ast.ValueSpec:
		var t types.Type
		if spec.Type != nil {
			infer.expr(spec.Type)
			t = infer.GetType2(spec.Type)
		}
		for i, name := range spec.Names {
			tt := t
			if tt == nil && spec.Values != nil {
				infer.expr(spec.Values[i])
				tt = infer.GetType(spec.Values[i])
			}
			if name.Mutable.IsValid() {
				tt = types.MutType{W: tt}
			}
			if len(spec.Values) > 0 {
				infer.exprs(spec.Values)
				value := spec.Values[i]
				valueT := infer.env.GetType(value)
				if !cmpTypesLoose(tt, valueT) {
					infer.errorf(name, "type mismatch, want: %s, got: %s", tt, valueT)
					return
				}
			}
			infer.SetType(name, tt)
			infer.env.Define(name, name.Name, tt)
		}
	default:
		infer.errorf(s, "%v", to(s))
		return
	}
}

func (infer *FileInferrer) incDecStmt(stmt *ast.IncDecStmt) {
	infer.expr(stmt.X)
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) forStmt(stmt *ast.ForStmt) {
	infer.withEnv(func() {
		if stmt.Init == nil && stmt.Cond != nil && stmt.Post == nil &&
			TryCast[*ast.BinaryExpr](stmt.Cond) && stmt.Cond.(*ast.BinaryExpr).Op == token.IN {
			cond := stmt.Cond.(*ast.BinaryExpr)
			infer.expr(cond.Y)
			yT := infer.GetType(cond.Y)
			var t types.Type
			switch v := types.Unwrap(yT).(type) {
			case types.ArrayType:
				if xTup, ok := cond.X.(*ast.TupleExpr); ok {
					yTupT := v.Elt.(types.TupleType)
					t = yTupT
					for i := range xTup.Values {
						infer.SetType(xTup.Values[i], yTupT.Elts[i])
					}
					infer.SetType(cond.X, t)
				} else {
					t = v.Elt
					infer.SetType(cond.X, t)
				}
			case types.SetType:
				t = v.K
				infer.SetType(cond.X, t)
			case types.MapType:
				if xTup, ok := cond.X.(*ast.TupleExpr); ok {
					t = types.TupleType{Elts: []types.Type{v.K, v.V}}
					infer.SetType(xTup.Values[0], v.K)
					infer.SetType(xTup.Values[1], v.V)
					infer.SetType(cond.X, t)
				} else {
					infer.errorf(cond.X, "expected tuple, got %v", to(cond.X))
					return
				}
			case types.RangeType:
				t = v.Typ
				infer.SetType(cond.X, t)
			case types.StructType:
				if v.Name == "Sequence" {
					t = v.TypeParams[0].(types.GenericType).W
					infer.SetType(cond.X, t)
				} else {
					infer.errorf(cond.Y, "unsupported type %v", to(yT))
					return
				}
			default:
				infer.errorf(cond.Y, "unsupported type %v", to(yT))
				return
			}
			switch v := cond.X.(type) {
			case *ast.Ident:
				infer.env.Define(cond.X, v.Name, t)
			case *ast.TupleExpr:
				for i, e := range v.Values {
					infer.env.Define(e, e.(*ast.Ident).Name, t.(types.TupleType).Elts[i])
				}
			default:
				infer.errorf(cond.X, "unsupported type %v", to(cond.X))
				return
			}
		} else {
			if stmt.Init != nil {
				infer.stmt(stmt.Init)
				if len(infer.Errors) > 0 {
					return
				}
			}
			if stmt.Cond != nil {
				infer.expr(stmt.Cond)
				if len(infer.Errors) > 0 {
					return
				}
			}
			if stmt.Post != nil {
				infer.stmt(stmt.Post)
				if len(infer.Errors) > 0 {
					return
				}
			}
		}
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) rangeStmt(stmt *ast.RangeStmt) {
	infer.withEnv(func() {
		infer.expr(stmt.X)
		xT := infer.GetType2(stmt.X)
		if xT == nil {
			infer.errorf(stmt.Value, "Type not found for: %v", stmt.X)
			return
		}
		if stmt.Key != nil {
			var t types.Type = types.IntType{}
			xT = types.Unwrap(xT)
			if v, ok := xT.(types.MapType); ok {
				t = v.K
			}
			name := stmt.Key.(*ast.Ident).Name
			infer.env.Define(stmt.Key, name, t)
			infer.SetType(stmt.Key, t)
		}
		if stmt.Value != nil {
			name := stmt.Value.(*ast.Ident).Name
			xT = types.Unwrap(xT)
			switch v := xT.(type) {
			case types.StringType:
				t := types.I32Type{}
				infer.env.Define(stmt.Value, name, t)
				infer.SetType(stmt.Value, t)
			case types.ArrayType:
				infer.env.Define(stmt.Value, name, v.Elt)
				infer.SetType(stmt.Value, v.Elt)
			case types.MapType:
				infer.env.Define(stmt.Value, name, v.V)
				infer.SetType(stmt.Value, v.V)
			default:
				infer.errorf(stmt.Value, "%v %v", name, to(xT))
				return
			}
		}
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) Pos(n ast.Node) token.Position {
	return infer.fset.Position(n.Pos())
}

func (infer *FileInferrer) assignStmt(stmt *ast.AssignStmt) {
	infer.SetType(stmt, types.VoidType{})
	type AssignStruct struct {
		n       ast.Node
		name    string
		mutable bool
		typ     types.Type
	}
	myDefine := func(parentInfo *Info, n ast.Node, name string, typ types.Type) {
		infer.env.DefineForce(n, name, typ)
	}
	myAssign := func(parentInfo *Info, n ast.Node, name string, _ types.Type) {
		if err := infer.env.Assign(parentInfo, n, name, infer.fset, infer.mutEnforced); err != nil {
			infer.errorf(n, "%s: %s", infer.Pos(n), err)
			return
		}
	}
	var assigns []AssignStruct
	assignFn := func(n ast.Node, name string, mutable bool, typ types.Type) {
		op := stmt.Tok
		f := utils.Ternary(op == token.DEFINE, myDefine, myAssign)
		var parentInfo *Info
		if op != token.DEFINE {
			parentInfo = infer.env.GetNameInfo(name)
		}
		if mutable {
			if !TryCast[types.MutType](typ) {
				typ = types.MutType{W: typ}
			}
		}
		f(parentInfo, n, name, typ)
	}
	assignsFn := func() {
		op := stmt.Tok
		if op == token.DEFINE {
			var hasNewVar bool
			for _, ass := range assigns {
				if ass.name != "_" && infer.env.GetDirect(ass.name) == nil {
					hasNewVar = true
				}
			}
			if !hasNewVar && len(assigns) > 0 {
				infer.errorf(stmt, "No new variables on the left side of ':='")
				return
			}
		}
		for _, ass := range assigns {
			assignFn(ass.n, ass.name, ass.mutable, ass.typ)
		}
	}
	if len(stmt.Rhs) == 1 && len(stmt.Lhs) > 1 { // eg: `e, ok := m[0]`
		rhs := stmt.Rhs[0]
		infer.expr(rhs)
		switch rhs1 := rhs.(type) {
		case *ast.TupleExpr:
			for i, x := range rhs1.Values {
				lhs := stmt.Lhs[i]
				lhsID := MustCast[*ast.Ident](lhs)
				infer.SetType(lhs, infer.GetType(x))
				assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType(lhsID)})
			}
		case *ast.Ident:
			rhsIdT := infer.env.Get(rhs1.Name)
			switch v := rhsIdT.(type) {
			case types.TupleType:
				if len(v.Elts) != len(stmt.Lhs) {
					infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(v.Elts))
					return
				}
				for i, x := range v.Elts {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*ast.Ident](lhs)
					infer.SetType(lhs, x)
					assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType(lhsID)})
				}
			case types.EnumType:
				f := Find(v.Fields, func(f types.EnumFieldType) bool { return f.Name == v.SubTyp })
				if f == nil {
					panic(fmt.Sprintf("Field not found: %s", v.SubTyp))
				}
				if len(f.Elts) != len(stmt.Lhs) {
					infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(f.Elts))
					return
				}
				for i, x := range f.Elts {
					lhs := stmt.Lhs[i]
					lhsID := MustCast[*ast.Ident](lhs)
					infer.SetType(lhs, x)
					assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType(lhsID)})
				}
			default:
				panic("")
			}
		case *ast.IndexExpr:
			rhsId1XT := infer.GetType(rhs1.X)
			if v, ok := rhsId1XT.(types.CustomType); ok {
				rhsId1XT = v.W
			}
			switch rhsId1XTT := rhsId1XT.(type) {
			case types.MapType:
				switch len(stmt.Lhs) {
				case 2:
					lhs1 := stmt.Lhs[1].(*ast.Ident)
					lhs1T := types.BoolType{}
					infer.SetType(lhs1, lhs1T)
					assigns = append(assigns, AssignStruct{lhs1, lhs1.Name, lhs1.Mutable.IsValid(), lhs1T})
					fallthrough
				case 1:
					lhs0 := stmt.Lhs[0].(*ast.Ident)
					lhs0T := rhsId1XTT.V
					infer.SetType(lhs0, rhsId1XTT.V)
					assigns = append(assigns, AssignStruct{lhs0, lhs0.Name, lhs0.Mutable.IsValid(), lhs0T})
				default:
					infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(stmt.Rhs))
					return
				}
			default:
				infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(stmt.Rhs))
				return
			}
		default:
			if _, ok := rhs.(*ast.TupleExpr); ok {
				rhs2 := infer.env.GetType(rhs).(types.EnumType)
				for i, e := range stmt.Lhs {
					lit := rhs2.SubTyp
					fields := rhs2.Fields
					// AGL: fields.Find({ $0.name == lit })
					f := Find(fields, func(f types.EnumFieldType) bool { return f.Name == lit })
					assert(f != nil)
					assigns = append(assigns, AssignStruct{e, e.(*ast.Ident).Name, e.(*ast.Ident).Mutable.IsValid(), f.Elts[i]})
				}
			} else {
				var keepRaw bool
				if vv, ok := rhs.(*ast.CallExpr); ok {
					if vvv, ok := vv.Fun.(*ast.SelectorExpr); ok {
						if vvvv, ok := infer.env.GetType(vvv.Sel).(types.FuncType); ok {
							if vvvv.IsNative {
								keepRaw = true
							}
						}
					}
				}
				switch v := infer.env.GetType(rhs).(type) {
				case types.ResultType:
					if v.Native {
						v.KeepRaw = true
						infer.SetType(rhs, v)
						infer.SetType(stmt.Lhs[0], infer.env.GetType(rhs).(types.ResultType).W)
						infer.SetType(stmt.Lhs[1], types.TypeType{W: types.InterfaceType{Name: "error"}})
						lhsID0 := MustCast[*ast.Ident](stmt.Lhs[0])
						lhsID1 := MustCast[*ast.Ident](stmt.Lhs[1])
						assigns = append(assigns, AssignStruct{lhsID0, lhsID0.Name, lhsID0.Mutable.IsValid(), infer.GetType(lhsID0)})
						assigns = append(assigns, AssignStruct{lhsID1, lhsID1.Name, lhsID1.Mutable.IsValid(), infer.GetType(lhsID1)})
					} else {
						infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(stmt.Rhs))
						return
					}
				case types.EnumType:
					if len(v.Fields) != len(stmt.Lhs) {
						infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(v.Fields))
						return
					}
					for i, f := range v.Fields {
						lhs := stmt.Lhs[i]
						lhsID := MustCast[*ast.Ident](lhs)
						infer.SetType(lhs, f)
						assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType(lhsID)})
					}
				case types.TupleType:
					if keepRaw {
						v.KeepRaw = keepRaw
						infer.SetTypeForce(rhs, v)
					}
					if len(v.Elts) != len(stmt.Lhs) {
						infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(v.Elts))
						return
					}
					for i, x := range v.Elts {
						lhs := stmt.Lhs[i]
						lhsID := MustCast[*ast.Ident](lhs)
						infer.SetType(lhs, x)
						assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType(lhsID)})
					}
				case types.TypeAssertType:
					if len(stmt.Lhs) > 0 {
						lhsID0 := MustCast[*ast.Ident](stmt.Lhs[0])
						assigns = append(assigns, AssignStruct{lhsID0, lhsID0.Name, lhsID0.Mutable.IsValid(), v.Type})
						infer.SetType(stmt.Lhs[0], v.Type)
						if len(stmt.Lhs) == 2 {
							lhsID1 := MustCast[*ast.Ident](stmt.Lhs[1])
							assigns = append(assigns, AssignStruct{lhsID1, lhsID1.Name, lhsID1.Mutable.IsValid(), types.BoolType{}})
							infer.SetType(stmt.Lhs[1], types.BoolType{})
						}
					}
				default:
					infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(stmt.Rhs))
					return
				}
			}
		}
	} else {
		if len(stmt.Lhs) != len(stmt.Rhs) {
			infer.errorf(stmt, "Assignment count mismatch: %d = %d", len(stmt.Lhs), len(stmt.Rhs))
			return
		}
		for i := range stmt.Lhs {
			lhs := stmt.Lhs[i]
			rhs := stmt.Rhs[i]

			var lhsWantedT types.Type
			switch v := lhs.(type) {
			case *ast.Ident:
				lhsIdName := v.Name
				lhsWantedT = infer.env.Get(lhsIdName)
			case *ast.IndexExpr:
				var lhsIdName string
				infer.expr(v.X)
				infer.expr(v.Index)
				if TryCast[types.UntypedNumType](infer.GetType(v.Index)) {
					infer.SetType(v.Index, types.IntType{})
				}
				switch vv := v.X.(type) {
				case *ast.Ident:
					lhsIdName = vv.Name
				}
				lhsIdNameT := infer.env.GetType(v.X)
				if v, ok := lhsIdNameT.(types.StarType); ok {
					lhsIdNameT = v.X
				}
				if infer.mutEnforced && !TryCast[types.MutType](lhsIdNameT) {
					infer.errorf(v.X, "cannot assign to immutable variable '%s'", lhsIdName)
					return
				}
				lhsIdNameT = types.Unwrap(lhsIdNameT)
				switch vv := lhsIdNameT.(type) {
				case types.MapType:
					lhsWantedT = vv.V
				case types.ArrayType:
					lhsWantedT = vv.Elt
				case types.SetType:
				default:
					infer.errorf(lhs, "%v %v", lhsIdName, to(lhsIdNameT))
					return
				}
			case *ast.SelectorExpr:
				var lhsIdName string
				switch vv := v.X.(type) {
				case *ast.Ident:
					lhsIdName = vv.Name
				case *ast.IndexExpr:
					switch vvv := vv.X.(type) {
					case *ast.Ident:
						t := infer.env.Get(vvv.Name)
						t = types.Unwrap(t)
						tmp := t.(types.ArrayType).Elt.GoStrType()
						lhsIdName = tmp + "." + v.Sel.Name
					default:
						infer.errorf(lhs, "%v", to(vv.X))
						return
					}
				case *ast.SelectorExpr:
					switch vvv := vv.X.(type) {
					case *ast.Ident:
						tmp := infer.env.Get(vvv.Name).GoStrType()
						lhsIdName = tmp + "." + vv.Sel.Name
					case *ast.IndexExpr:
						switch vvvv := vvv.X.(type) {
						case *ast.Ident:
							t := infer.env.Get(vvvv.Name)
							t = types.Unwrap(t)
							tmp := t.(types.ArrayType).Elt.GoStrType()
							lhsIdName = tmp + "." + vv.Sel.Name
						default:
							infer.errorf(lhs, "%v", to(vvv.X))
							return
						}
					default:
						infer.errorf(lhs, "%v", to(vv.X))
						return
					}
				default:
					infer.errorf(lhs, "%v", to(v.X))
					return
				}
				xT := infer.env.Get(lhsIdName)
				xT = types.Unwrap(xT)
				switch vv := xT.(type) {
				case types.TupleType:
					if argIdx, err := strconv.Atoi(v.Sel.Name); err == nil {
						lhsWantedT = vv.Elts[argIdx]
					} else {
						lhsWantedT = xT
					}
				case types.StructType:
					selT := infer.env.Get(vv.Name + "." + v.Sel.Name)
					if infer.mutEnforced && !TryCast[types.MutType](selT) {
						infer.errorf(v.Sel, "assign to immutable prop '%s'", v.Sel.Name)
						return
					}
				}
			case *ast.StarExpr:
				lhsIdName := v.X.(*ast.Ident).Name
				lhsWantedT = infer.env.Get(lhsIdName)
			default:
				infer.errorf(lhs, "%v", to(lhs))
				return
			}
			if lhsT := lhsWantedT; lhsT != nil && stmt.Tok != token.DEFINE {
				infer.SetType(rhs, lhsWantedT)
				infer.withForceReturn(lhsT, func() {
					infer.withOptType(rhs, types.Unwrap(lhsT), func() {
						infer.expr(rhs)
					})
					if len(infer.Errors) > 0 {
						return
					}
					rhsT := infer.GetType2(rhs)
					lhsWantedTT := types.Unwrap(lhsWantedT)
					if TryCast[types.OptionType](rhsT) {
						rhsT = lhsWantedTT
						infer.SetType(rhs, rhsT)
					}
					if !cmpTypesLoose(rhsT, lhsT) {
						infer.errorf(lhs, "return type %s does not match expected type %s", rhsT, lhsT)
					}
				})
				if len(infer.Errors) > 0 {
					return
				}
			} else {
				infer.expr(rhs)
				if len(infer.Errors) > 0 {
					return
				}
			}
			var mutable bool
			var lhsID *ast.Ident
			switch v := lhs.(type) {
			case *ast.Ident:
				lhsID = v
				mutable = v.Mutable.IsValid()
			case *ast.StarExpr:
				lhsID = v.X.(*ast.Ident)
				mutable = v.X.(*ast.Ident).Mutable.IsValid()
			case *ast.IndexExpr:
				if v, ok := v.X.(*ast.Ident); ok {
					lhsID = v
					mutable = v.Mutable.IsValid()
				} else {
					return
				}
				lhsIDT := infer.env.Get(lhsID.Name)
				infer.SetType(lhsID, lhsIDT)
				lhsIDT = types.Unwrap(lhsIDT)
				switch vv := lhsIDT.(type) {
				case types.MapType:
					infer.SetType(v.Index, vv.K)
					infer.SetType(v, vv.V)
				case types.ArrayType:
					if vvv, ok := v.Index.(*ast.Ident); ok {
						infer.SetType(v.Index, infer.env.Get(vvv.Name))
					}
					infer.SetType(v, vv.Elt)
				}
				return
			case *ast.SelectorExpr:
				var lhsIdName string
				switch vv := v.X.(type) {
				case *ast.Ident:
					lhsID = vv
					lhsIdName = lhsID.Name
				case *ast.SelectorExpr:
					lhsID = vv.Sel
					var tmp string
					switch vvv := vv.X.(type) {
					case *ast.Ident:
						tmp = infer.env.Get(vvv.Name).GoStrType()
					case *ast.IndexExpr:
						switch vvvv := vvv.X.(type) {
						case *ast.Ident:
							t := infer.env.Get(vvvv.Name)
							t = types.Unwrap(t)
							tmp = t.(types.ArrayType).Elt.GoStrType()
						default:
							infer.errorf(lhs, "%v", to(vvv.X))
							return
						}
					}
					lhsIdName = tmp + "." + lhsID.Name
				case *ast.IndexExpr:
					if v, ok := v.X.(*ast.Ident); ok {
						lhsID = v
						mutable = v.Mutable.IsValid()
					} else {
						return
					}
					lhsIDT := infer.env.Get(lhsID.Name)
					infer.SetType(lhsID, lhsIDT)
					lhsIDT = types.Unwrap(lhsIDT)
					return
				default:
					infer.errorf(v.Sel, "...")
					return
				}
				mutable = lhsID.Mutable.IsValid()
				xT := infer.env.Get(lhsIdName)
				oxT := xT
				xT = types.Unwrap(xT)
				switch xTv := xT.(type) {
				case types.TupleType:
					argIdx, _ := strconv.Atoi(v.Sel.Name)
					infer.SetType(v.X, oxT)
					want, got := xTv.Elts[argIdx], infer.GetType(rhs)
					if !cmpTypesLoose(want, got) {
						infer.errorf(v.Sel, "type mismatch, wants: %s, got: %s", want, got)
						return
					}
					infer.SetType(v.Sel, got)
					return
				case types.StructType:
					selT := infer.env.Get(xT.String() + "." + v.Sel.Name)
					infer.SetType(v.X, oxT)
					infer.SetType(v.Sel, selT)
				case types.ArrayType:
					infer.SetType(v.X, xT)
				default:
					infer.errorf(lhs, "%v %v", lhsIdName, to(xT))
					return
				}
			default:
				infer.errorf(lhs, "%v", to(lhs))
				return
			}
			var rhsT types.Type
			switch ta := rhs.(type) {
			case *ast.TypeAssertExpr:
				tmp := utils.Ternary(ta.Type == nil, ta.X, ta.Type)
				rhsT = infer.GetType2(tmp)
				rhsT = types.OptionType{W: rhsT}
			case *ast.IndexExpr:
				rhsT = infer.GetType2(ta.X)
				rhsT = types.Unwrap(rhsT)
				switch v := rhsT.(type) {
				case types.MapType:
					rhsT = v.V
				case types.ArrayType:
					rhsT = v.Elt
				}
			default:
				rhsT = infer.GetType2(rhs)
			}
			if TryCast[types.VoidType](rhsT) {
				infer.errorf(lhs, "cannot assign void type to a variable")
				return
			}
			lhsT := infer.env.GetType(lhs)
			switch lhsT.(type) {
			case types.OptionType:
				rhsT = types.Unwrap(rhsT)
				if !TryCast[types.OptionType](rhsT) {
					infer.errorf(lhs, "try to destructure a non-Option type into an OptionType")
					return
				}
				switch rhsT.(type) {
				case types.OptionType:
					if infer.destructure {
						infer.SetTypeForce(lhs, rhsT.(types.OptionType).W)
					}
				default:
					panic("")
				}
			case types.ResultType:
				if !TryCast[types.ResultType](rhsT) {
					infer.errorf(lhs, "try to destructure a non-Result type into an ResultType")
					return
				}
				infer.SetTypeForce(lhs, rhsT.(types.ResultType).W)
			default:
				if v, ok := rhsT.(types.ResultType); ok {
					v.Native = false
					rhsT = v
				}
				if mutable && !TryCast[types.MutType](rhsT) {
					rhsT = types.MutType{W: rhsT}
				}
				infer.SetType(lhs, rhsT)
			}
			assigns = append(assigns, AssignStruct{lhsID, lhsID.Name, lhsID.Mutable.IsValid(), infer.GetType2(lhsID)})
		}
	}
	assignsFn()
}

func (infer *FileInferrer) exprStmt(stmt *ast.ExprStmt) {
	infer.expr(stmt.X)
	infer.SetType(stmt, infer.GetType(stmt.X))
}

func (infer *FileInferrer) returnStmt(stmt *ast.ReturnStmt) {
	if stmt.Result != nil {
		infer.withOptType(stmt.Result, infer.returnType, func() {
			// Allow Enum to be used without the name if the type is known eg: ".Red"
			if v, ok := infer.returnType.(types.EnumType); ok {
				if vv, ok := stmt.Result.(*ast.SelectorExpr); ok {
					if vvv, ok := vv.X.(*ast.Ident); ok {
						if vvv.Name == "" {
							vvv.Name = v.Name
							vv.X = vvv
							stmt.Result = vv
						}
					}
				}
			}
			rT := infer.GetType2(stmt.Result)
			if v, ok := rT.(types.UnaryType); ok {
				rT = v.X
			}
			if !cmpTypesLoose(rT, infer.returnType) {
				infer.errorf(stmt.Result, "type mismatch")
				return
			}
			infer.expr(stmt.Result)
			if _, ok := infer.GetType(stmt.Result).(types.OptionType); ok {
				if v, ok := infer.returnType.(types.OptionType); ok {
					infer.SetTypeForce(stmt.Result, types.OptionType{W: v.W})
				}
			}
		})
	}
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) blockStmt(stmt *ast.BlockStmt) {
	infer.stmts(stmt.List)
	if len(infer.Errors) > 0 {
		return
	}
	if stmt.List == nil || len(stmt.List) == 0 {
		infer.SetType(stmt, types.VoidType{})
	} else if infer.env.GetType(stmt) == nil {
		infer.SetType(stmt, infer.GetType(stmt.List[len(stmt.List)-1]))
	}
}

func (infer *FileInferrer) binaryExpr(expr *ast.BinaryExpr) {
	infer.expr(expr.X)
	if len(infer.Errors) > 0 {
		return
	}
	if TryCast[types.OptionType](infer.GetType2(expr.X)) &&
		TryCast[*ast.Ident](expr.Y) && expr.Y.(*ast.Ident).Name == "None" &&
		infer.env.GetType(expr.Y) == nil {
		infer.SetType(expr.Y, infer.GetType2(expr.X))
	}
	if t := infer.env.GetType(expr.Y); t == nil || TryCast[types.UntypedNumType](t) {
		infer.expr(expr.Y)
		if len(infer.Errors) > 0 {
			return
		}
	}
	if infer.GetType2(expr.X) != nil && infer.GetType2(expr.Y) != nil {
		tmpFn := func(x, y ast.Expr) bool {
			return isNumericType(infer.GetType2(x)) && TryCast[types.UntypedNumType](infer.GetType2(y))
		}
		if tmpFn(expr.X, expr.Y) {
			infer.SetType(expr.Y, infer.GetType2(expr.X))
		} else if tmpFn(expr.Y, expr.X) {
			infer.SetType(expr.X, infer.GetType2(expr.Y))
		}
	}
	switch expr.Op {
	case token.EQL, token.NEQ, token.LOR, token.LAND, token.LEQ, token.LSS, token.GEQ, token.GTR, token.IN:
		infer.SetType(expr, types.BoolType{})
	case token.ADD, token.SUB, token.QUO, token.MUL, token.REM, token.POW:
		oxT := infer.GetType(expr.X)
		oyT := infer.GetType(expr.Y)
		yT := types.Unwrap(oyT)
		if TryCast[types.StructType](yT) {
			infer.SetType(expr, oyT)
		} else {
			infer.SetType(expr, oxT)
		}
	default:
		infer.SetType(expr, types.Unwrap(infer.GetType(expr.X)))
	}
}

func (infer *FileInferrer) identExpr(expr *ast.Ident) {
	if expr.Name == "_" {
		return
	}
	if infer.optType != nil && expr.Name == "" {
		if v, ok := infer.optType.Type.(types.EnumType); ok {
			expr.Name = v.Name
		}
	}
	v := infer.env.Get(expr.Name)
	info := infer.env.GetNameInfo(expr.Name)
	if v == nil {
		infer.errorf(expr, "undefined identifier %s", expr.Name)
		return
	}
	if InArray(expr.Name, []string{"true", "false"}) {
		v = types.BoolType{}
	}
	infer.SetType(expr, v, WithDefinition(info))
}

func (infer *FileInferrer) sendStmt(stmt *ast.SendStmt) {
	infer.expr(stmt.Chan)
	infer.expr(stmt.Value)
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) selectStmt(stmt *ast.SelectStmt) {
	infer.stmt(stmt.Body)
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) commClause(stmt *ast.CommClause) {
	if stmt.Comm != nil {
		infer.stmt(stmt.Comm)
	}
	if stmt.Body != nil {
		infer.stmts(stmt.Body)
	}
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) typeSwitchStmt(stmt *ast.TypeSwitchStmt) {
	infer.withEnv(func() {
		if stmt.Init != nil {
			infer.stmt(stmt.Init)
		}
		if ass, ok := stmt.Assign.(*ast.AssignStmt); ok && len(ass.Lhs) == 1 {
			rhs := ass.Rhs[0]
			t := infer.GetType2(rhs)
			infer.SetType(ass.Lhs[0], t.(types.TypeAssertType).X)
		}
		infer.stmt(stmt.Assign)
		for _, el := range stmt.Body.List {
			c := el.(*ast.CaseClause)
			if c.List != nil {
				infer.exprs(c.List)
			}
			if c.Body != nil {
				infer.withEnv(func() {
					switch ass := stmt.Assign.(type) {
					case *ast.AssignStmt:
						if len(ass.Lhs) == 1 && len(c.List) == 1 {
							id := ass.Lhs[0].(*ast.Ident).Name
							idT := infer.GetType2(c.List[0])
							infer.env.Define(ass.Lhs[0], id, idT)
						}
					case *ast.ExprStmt:
					}
					infer.stmts(c.Body)
				})
			}
		}
	})
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) switchStmt(stmt *ast.SwitchStmt) {
	infer.withEnv(func() {
		var tagT types.Type
		if stmt.Tag != nil {
			infer.expr(stmt.Tag)
			tagT = infer.env.GetType(stmt.Tag)
		}
		for _, el := range stmt.Body.List {
			c := el.(*ast.CaseClause)
			if c.List != nil {
				for _, expr := range c.List {
					infer.withOptType(expr, tagT, func() {
						infer.expr(expr)
					})
				}
			}
			if c.Body != nil {
				infer.stmts(c.Body)
			}
		}
	})
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) caseClause(stmt *ast.CaseClause) {
	if stmt.List != nil {
		infer.exprs(stmt.List)
	}
	if stmt.Body != nil {
		infer.stmts(stmt.Body)
	}
}

func (infer *FileInferrer) branchStmt(stmt *ast.BranchStmt) {
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) deferStmt(stmt *ast.DeferStmt) {
	infer.expr(stmt.Call)
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) goStmt(stmt *ast.GoStmt) {
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) emptyStmt(stmt *ast.EmptyStmt) {
	infer.SetType(stmt, types.VoidType{})
}

func IsExhaustive(enumT types.EnumType, clauses []*ast.MatchClause) bool {
	names := make(map[string]struct{})
	for _, f := range enumT.Fields {
		names[f.Name] = struct{}{}
	}
	for _, clause := range clauses {
		if clause.Expr == nil {
			return true
		}
		switch v := clause.Expr.(type) {
		case *ast.CallExpr:
			name := v.Fun.(*ast.SelectorExpr).Sel.Name
			delete(names, name)
		case *ast.SelectorExpr:
			delete(names, v.Sel.Name)
		default:
			panic(fmt.Sprintf("%v", to(clause.Expr)))
		}
	}
	return len(names) == 0
}

func (infer *FileInferrer) matchExpr(expr *ast.MatchExpr) {
	infer.expr(expr.Init)
	initT := infer.GetType2(expr.Init)
	var enumT types.EnumType
	var isEnum bool
	enumT, isEnum = initT.(types.EnumType)
	var prevBranchT types.Type
	var matchClauses []*ast.MatchClause
	for _, stmtEl := range expr.Body.List {
		matchClauses = append(matchClauses, stmtEl.(*ast.MatchClause))
	}
	if isEnum && !IsExhaustive(enumT, matchClauses) {
		infer.errorf(expr, "match expression is not exhaustive")
		return
	}
	infer.withOptType(expr.Init, initT, func() {
		for _, clause := range matchClauses {
			switch v := clause.Expr.(type) {
			case *ast.OkExpr:
				t := infer.optType.Type.(types.ResultType).W
				infer.env.Define(v.X, v.X.(*ast.Ident).Name, t)
				infer.SetType(v.X, t)
			case *ast.ErrExpr:
				t := infer.env.Get("error")
				infer.env.Define(v.X, v.X.(*ast.Ident).Name, t)
				infer.SetType(v.X, t)
			case *ast.CallExpr:
				if vv, ok := v.Fun.(*ast.SelectorExpr); ok {
					if vvv, ok := infer.optType.Type.(types.EnumType); ok {
						f := Find(vvv.Fields, func(f types.EnumFieldType) bool { return f.Name == vv.Sel.Name })
						for i, el := range f.Elts {
							arg := v.Args[i]
							infer.env.Define(arg, arg.(*ast.Ident).Name, el)
							infer.SetType(arg, el)
						}
					}
				}
			}
			if clause.Expr != nil {
				infer.expr(clause.Expr)
			}
			infer.stmts(clause.Body)
			var branchT types.Type
			if len(clause.Body) == 0 {
				branchT = types.VoidType{}
			} else {
				branchT = infer.GetType(Must(Last(clause.Body)))
			}
			infer.SetType(clause, branchT)
			if prevBranchT != nil {
				if !cmpTypesLoose(prevBranchT, branchT) {
					infer.errorf(expr, "%s: match branches must have the same type `%s` VS `%s`", infer.Pos(expr), prevBranchT, branchT)
					return
				}
			}
			prevBranchT = branchT
		}
	})
	infer.SetType(expr.Body, infer.GetType(Must(Last(expr.Body.List))))
	infer.SetType(expr, infer.GetType(expr.Body))
}

func (infer *FileInferrer) labeledStmt(stmt *ast.LabeledStmt) {
	infer.env.Define(stmt.Label, stmt.Label.Name, types.LabelType{})
	infer.stmt(stmt.Stmt)
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) ifLetExpr(stmt *ast.IfLetExpr) {
	infer.withEnv(func() {
		lhs := stmt.Ass.Lhs[0]
		var lhsT types.Type
		switch stmt.Op {
		case token.NONE:
			lhsT = types.OptionType{}
		case token.OK:
			lhsT = types.ResultType{}
		case token.ERR:
			lhsT = types.ResultType{}
		case token.SOME:
			lhsT = types.OptionType{}
		default:
			panic("unreachable")
		}
		infer.SetType(lhs, lhsT)
		infer.withDestructure(func() {
			infer.stmt(stmt.Ass)
		})
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	if stmt.Else != nil {
		infer.withEnv(func() {
			infer.stmt(stmt.Else)
		})
	}
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) guardLetStmt(stmt *ast.GuardLetStmt) {
	lhs := stmt.Ass.Lhs[0]
	var lhsT types.Type
	switch stmt.Op {
	case token.NONE:
		lhsT = types.OptionType{}
	case token.OK:
		lhsT = types.ResultType{}
	case token.ERR:
		lhsT = types.ResultType{}
	case token.SOME:
		lhsT = types.OptionType{}
	default:
		panic("unreachable")
	}
	infer.SetType(lhs, lhsT)
	infer.withDestructure(func() {
		infer.stmt(stmt.Ass)
	})
	if stmt.Body == nil || len(stmt.Body.List) == 0 {
		infer.errorf(stmt.Body, "guard body msut have at least 1 statement")
		return
	}
	infer.stmt(stmt.Body)
	lastStmt := stmt.Body.List[len(stmt.Body.List)-1]
	switch v := lastStmt.(type) {
	case *ast.ReturnStmt:
	case *ast.BranchStmt:
		if v.Tok != token.BREAK && v.Tok != token.CONTINUE {
			infer.errorf(v, "guard must return/break/continue")
			return
		}
	default:
		infer.errorf(v, "guard must return/break/continue")
		return
	}
	infer.SetType(stmt, types.VoidType{})
}

func (infer *FileInferrer) ifExpr(stmt *ast.IfExpr) {
	infer.withEnv(func() {
		if stmt.Init != nil {
			infer.stmt(stmt.Init)
		}
		infer.expr(stmt.Cond)
		if stmt.Body != nil {
			infer.stmt(stmt.Body)
		}
	})
	if stmt.Else != nil {
		infer.withEnv(func() {
			infer.stmt(stmt.Else)
		})
		a := infer.GetType(stmt.Body)
		b := infer.GetType(stmt.Else)
		if v, ok := a.(types.OptionType); ok && v.W == nil {
			v.W = b.(types.OptionType).W
			a = v
			infer.SetType(stmt.Body, a)
			infer.withEnv(func() {
				infer.withOptType1(stmt.Body, v.W, func() {
					infer.stmt(stmt.Body)
				})
			})
		}
		if v, ok := b.(types.OptionType); ok && v.W == nil {
			v.W = a.(types.OptionType).W
			b = v
			infer.SetType(stmt.Else, b)
			infer.withEnv(func() {
				infer.withOptType1(stmt.Else, v.W, func() {
					infer.stmt(stmt.Else)
				})
			})
		}
		if !cmpTypesLoose(a, b) {
			infer.errorf(stmt, "%s: if branches must have the same type `%s` VS `%s`", infer.Pos(stmt), a, b)
			return
		}
	}
	if stmt.Body != nil {
		infer.SetType(stmt, infer.GetType(stmt.Body))
	} else {
		infer.SetType(stmt, types.VoidType{})
	}
}

func (infer *FileInferrer) guardStmt(stmt *ast.GuardStmt) {
	infer.withEnv(func() {
		infer.expr(stmt.Cond)
		if stmt.Body == nil || len(stmt.Body.List) == 0 {
			infer.errorf(stmt.Body, "guard body msut have at least 1 statement")
			return
		}
		infer.stmt(stmt.Body)
		lastStmt := stmt.Body.List[len(stmt.Body.List)-1]
		switch v := lastStmt.(type) {
		case *ast.ReturnStmt:
		case *ast.BranchStmt:
			if v.Tok != token.BREAK && v.Tok != token.CONTINUE {
				infer.errorf(v, "guard must return/break/continue")
				return
			}
		default:
			infer.errorf(v, "guard must return/break/continue")
			return
		}
	})
	infer.SetType(stmt, types.VoidType{})
}
