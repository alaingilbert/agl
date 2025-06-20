package main

import (
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	goroot := runtime.GOROOT()
	fileName := "request.go"
	fnName := "NewRequest"
	filePath := filepath.Join(goroot, "src", "net", "http", fileName)
	src, err := os.ReadFile(filePath)
	if err != nil {
		panic(err)
	}
	fset := gotoken.NewFileSet()
	node, err := goparser.ParseFile(fset, fileName, src, goparser.AllErrors)
	if err != nil {
		panic(fmt.Sprintf("failed to parse %s\n", fileName))
	}
	conf := gotypes.Config{Importer: nil}
	info := &gotypes.Info{Defs: make(map[*goast.Ident]gotypes.Object)}
	_, _ = conf.Check("", fset, []*goast.File{node}, info)
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *goast.FuncDecl:
			if d.Name.Name == fnName && d.Recv == nil {
				wrapFn(fset, d)
			}
		}
	}
}

func wrapFn(fset *gotoken.FileSet, decl *goast.FuncDecl) {
	env := map[string]string{
		"io.Reader": "interface",
	}
	out := "func " + decl.Name.Name + "("
	var paramsArr []string
	for _, param := range decl.Type.Params.List {
		var name string
		var typ string
		switch param := param.Type.(type) {
		case *goast.SelectorExpr:
			name = param.X.(*goast.Ident).Name + "." + param.Sel.Name
		case *goast.Ident:
			name = param.Name
		}
		var namesArr []string
		for _, pname := range param.Names {
			namesArr = append(namesArr, pname.Name)
		}
		names := strings.Join(namesArr, ", ")
		if names != "" {
			names += " "
		}
		paramsStr := names
		typ = env[name]
		paramsStr += name
		if typ == "interface" {
			paramsStr += "?"
		}
		paramsArr = append(paramsArr, paramsStr)
	}
	out += strings.Join(paramsArr, ", ") + ")"
	if decl.Type.Results != nil {
		//for _, result := range decl.Type.Results.List {
		//}
		if decl.Type.Results.List[len(decl.Type.Results.List)-1].Type.(*goast.Ident).Name == "error" {
			out += "!"
		}
	}
	out += "{\n"
	out += "\t" + decl.Name.Name + "()\n"
	out += "}\n"
	fmt.Println(out)
}
