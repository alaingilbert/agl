package main

import (
	"agl/pkg/agl"
	"agl/pkg/ast"
	parser1 "agl/pkg/parser"
	"agl/pkg/token"
	"context"
	"errors"
	"fmt"
	goast "go/ast"
	"go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			var aglErr *agl.AglError
			if err, ok := r.(error); ok && errors.As(err, &aglErr) {
				msg := aglErr.Error()
				if msg == "" {
					msg += string(debug.Stack())
				}
				_, _ = fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			panic(r)
		}
	}()
	cmd := &cli.Command{
		Name:  "AGL",
		Usage: "AnotherGoLang",
		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "run command",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "debug"},
				},
				Action: runAction,
			},
			{
				Name:    "build",
				Aliases: []string{"b"},
				Usage:   "build command",
				Action:  buildAction,
			},
			{
				Name:    "execute",
				Aliases: []string{"e"},
				Usage:   "execute command",
				Action:  executeAction,
			},
		},
		Action: startAction,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func spawnGoRunFromBytes(source []byte) error {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "gorun")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir) // clean up

	// Create a .go file inside it
	tmpFile := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(tmpFile, source, 0644)
	if err != nil {
		return err
	}

	coreFile := filepath.Join(tmpDir, "aglCore.go")
	err = os.WriteFile(coreFile, []byte(agl.GenCore()), 0644)
	if err != nil {
		return err
	}

	// Run `go run` on the file
	cmd := exec.Command("go", "run", tmpFile, coreFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	debugFlag := cmd.Bool("debug")

	defer func() {
		if r := recover(); r != nil {
			var aglErr *agl.AglError
			if err, ok := r.(error); ok && errors.As(err, &aglErr) {
				msg := aglErr.Error()
				if msg == "" || debugFlag {
					msg += string(debug.Stack())
				}
				_, _ = fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			panic(r)
		}
	}()

	if cmd.NArg() == 0 {
		fmt.Println("You must specify a file to compile")
		return nil
	}
	fileName := cmd.Args().Get(0)
	if !strings.HasSuffix(fileName, ".agl") {
		fmt.Println("file must have '.agl' extension")
		return nil
	}
	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	fset, f := parser2(string(by))
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(fset, env)
	i.InferFile(f)
	src := agl.NewGenerator(i.Env, f).Generate()
	_ = spawnGoRunFromBytes([]byte(src))
	return nil
}

func executeAction(ctx context.Context, cmd *cli.Command) error {
	var input string
	if cmd.NArg() > 0 {
		input = cmd.Args().Get(0)
	} else {
		input = ""
	}
	fset, f := parser2(input)
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(fset, env)
	i.InferFile(f)
	src := agl.NewGenerator(i.Env, f).Generate()
	fmt.Println(src)
	fmt.Println(agl.GenCore())
	return nil
}

func buildAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		return buildProject()
	}
	fileName := cmd.Args().Get(0)
	if !strings.HasSuffix(fileName, ".agl") {
		fmt.Println("file must have '.agl' extension")
		return nil
	}
	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	fset, f := parser2(string(by))
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(fset, env)
	i.InferFile(f)
	src := agl.NewGenerator(i.Env, f).Generate()
	path := strings.Replace(fileName, ".agl", ".go", 1)
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		return err
	}

	coreFile := filepath.Join(filepath.Dir(path), "aglCore.go")
	err = os.WriteFile(coreFile, []byte(agl.GenCore()), 0644)
	if err != nil {
		return err
	}

	return nil
}

func buildProject() error {
	visited := make(map[string]struct{})
	if err := buildFolder(".", visited); err != nil {
		return err
	}
	cmd := exec.Command("go", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const modPrefix = "agl/" // TODO get from go.mod

func buildFolder(folderPath string, visited map[string]struct{}) error {
	if _, ok := visited[folderPath]; ok {
		return nil
	}
	visited[folderPath] = struct{}{}
	entries, _ := os.ReadDir(folderPath)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := filepath.Join(folderPath, entry.Name())
		if strings.HasSuffix(fileName, ".agl") {
			if err := buildAglFile(fileName); err != nil {
				panic(fmt.Sprintf("failed to build %s", fileName))
			}
		} else if strings.HasSuffix(fileName, ".go") {
			if strings.HasSuffix(fileName, "_test.go") {
				continue
			}
			src, err := os.ReadFile(fileName)
			if err != nil {
				return err
			}
			fset := gotoken.NewFileSet()
			node, err := parser.ParseFile(fset, fileName, src, parser.AllErrors)
			if err != nil {
				log.Printf("failed to parse %s\n", fileName)
				continue
			}
			conf := gotypes.Config{Importer: nil}
			info := &gotypes.Info{Defs: make(map[*goast.Ident]gotypes.Object)}
			_, _ = conf.Check("", fset, []*goast.File{node}, info)
			for _, decl := range node.Decls {
				switch d := decl.(type) {
				case *goast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *goast.ImportSpec:
							pathValue := strings.ReplaceAll(s.Path.Value, `"`, ``)
							if strings.HasPrefix(pathValue, modPrefix) {
								newPath := strings.TrimPrefix(pathValue, modPrefix)
								if err := buildFolder(newPath, visited); err != nil {
									return err
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func buildAglFile(fileName string) error {
	by, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	fset, f := parser2(string(by))
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(fset, env)
	i.InferFile(f)
	src := agl.NewGenerator(i.Env, f).Generate()
	path := strings.Replace(fileName, ".agl", "_agl.go", 1)
	return os.WriteFile(path, []byte(src), 0644)
}

func startAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		fmt.Println("You must specify a file to compile")
		return nil
	}
	fileName := cmd.Args().Get(0)
	if fileName == "run" {
		return runAction(ctx, cmd)
	}
	if !strings.HasSuffix(fileName, ".agl") {
		fmt.Println("file must have '.agl' extension")
		return nil
	}
	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	fset, f := parser2(string(by))
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(fset, env)
	i.InferFile(f)
	g := agl.NewGenerator(i.Env, f)
	fmt.Println(g.Generate())
	return nil
}

func parser2(src string) (*token.FileSet, *ast.File) {
	var fset = token.NewFileSet()
	f, err := parser1.ParseFile(fset, "", src, 0)
	if err != nil {
		panic(err)
	}
	return fset, f
}
