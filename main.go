package main

import (
	"agl/pkg/agl"
	"bytes"
	"context"
	"errors"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"
)

var version = "0.0.1"

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
		Name:    "AGL",
		Usage:   "AnotherGoLang",
		Version: version,
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
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "force", Aliases: []string{"f"}},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}},
				},
				Usage:  "build command",
				Action: buildAction,
			},
			{
				Name:   "clean",
				Usage:  "cleanup agl generated files",
				Action: cleanupAction,
			},
			{
				Name:    "execute",
				Aliases: []string{"e"},
				Usage:   "execute command",
				Action:  executeAction,
			},
			{
				Name:   "version",
				Usage:  "print AGL version",
				Action: versionAction,
			},
			{
				Name: "mod",
				Commands: []*cli.Command{
					{
						Name:   "init",
						Usage:  "initialize a new module",
						Action: modInitAction,
					},
					{
						Name:   "tidy",
						Action: modTidyAction,
					},
					{
						Name:   "vendor",
						Action: modVendorAction,
					},
				},
			},
		},
		Action: startAction,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func spawnGoRunFromBytes(source []byte, programArgs []string) error {
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
	err = os.WriteFile(coreFile, []byte(agl.GenCore("main")), 0644)
	if err != nil {
		return err
	}

	// Run `go run` on the file with additional arguments
	cmdArgs := append([]string{"run", tmpFile, coreFile}, programArgs...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func spawnGoBuild(fileName, outputFlag string) error {
	isAgl := strings.HasSuffix(fileName, ".agl")
	fileName = strings.Replace(fileName, ".agl", ".go", 1)
	var cmdArgs []string
	cmdArgs = append(cmdArgs, "build")
	if outputFlag != "" {
		cmdArgs = append(cmdArgs, "-o", outputFlag)
	}
	dir := filepath.Dir(fileName)
	cmdArgs = append(cmdArgs, fileName)
	if isAgl {
		cmdArgs = append(cmdArgs, filepath.Join(dir, "aglCore.go"))
	}
	cmd := exec.Command("go", cmdArgs...)
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
			var inferErr *agl.InferError
			if err, ok := r.(error); ok && errors.As(err, &inferErr) {
				msg := inferErr.Error()
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
	by := agl.Must(os.ReadFile(fileName))
	fset, f := agl.ParseSrc(string(by))
	env := agl.NewEnv()
	i := agl.NewInferrer(env)
	errs := i.InferFile(fileName, f, fset, false)
	if len(errs) > 0 {
		panic(errs[0])
	}
	src := agl.NewGenerator(i.Env, f, fset).Generate()

	// Get any additional arguments to pass to the program
	var programArgs []string
	if cmd.NArg() > 1 {
		programArgs = cmd.Args().Slice()[1:] // Skip the .agl filename
	}

	_ = spawnGoRunFromBytes([]byte(src), programArgs)
	return nil
}

func executeAction(ctx context.Context, cmd *cli.Command) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("EXIT_CODE:1")
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
	var input string
	if cmd.NArg() > 0 {
		input = cmd.Args().Get(0)
	} else {
		input = ""
	}
	fset, f := agl.ParseSrc(input)
	env := agl.NewEnv()
	i := agl.NewInferrer(env)
	i.InferFile("", f, fset, true)
	src := agl.NewGenerator(i.Env, f, fset).Generate()
	coreHeaders := agl.GenHeaders()

	out := insertHeadersAfterFirstLine(src, coreHeaders) + agl.GenContent()
	fmt.Println(out)
	return nil
}

// Walk the current directory and subdirectories and delete all `*.go` files that start with `// agl:generated`
func cleanupAction(ctx context.Context, cmd *cli.Command) error {
	generatedFilePrefix := agl.GeneratedFilePrefix
	_ = fs.WalkDir(os.DirFS("."), ".", func(path string, d fs.DirEntry, err error) error {
		base := filepath.Base(path)
		if d.IsDir() && (base == "vendor" || base == "node_modules") {
			return fs.SkipDir
		}
		if strings.HasSuffix(path, ".go") {
			if file, err := os.Open(path); err == nil {
				defer file.Close()
				buf := make([]byte, len(generatedFilePrefix))
				if _, err := file.Read(buf); err == nil {
					if bytes.Equal(buf, []byte(generatedFilePrefix)) {
						fmt.Println("removing", path)
						if err := os.Remove(path); err != nil {
							panic(err)
						}
					}
				}
			}
		}
		return nil
	})
	return nil
}

func buildAction(ctx context.Context, cmd *cli.Command) error {
	if cmd.NArg() == 0 {
		return buildProject()
	}
	outputFlag := cmd.String("output")
	forceFlag := cmd.Bool("force")
	fileName := cmd.Args().Get(0)
	m := agl.NewPkgVisited()
	err := buildFile(fileName, forceFlag, m)
	if err != nil {
		panic(err)
	}
	return spawnGoBuild(fileName, outputFlag)
}

func buildFile(fileName string, forceFlag bool, m *agl.PkgVisited) error {
	if m.ContainsAdd(fileName) {
		return nil
	}
	fmt.Println("build", fileName)
	isGo := strings.HasSuffix(fileName, ".go")
	isAGL := strings.HasSuffix(fileName, ".agl")
	if !isAGL && !isGo {
		return nil
	}

	generatedFilePrefix := agl.GeneratedFilePrefix

	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	if bytes.Equal(by[:len(generatedFilePrefix)], []byte(generatedFilePrefix)) {
		return nil
	}

	if isGo {
		f, err := goparser.ParseFile(gotoken.NewFileSet(), fileName, by, 0)
		if err != nil {
			panic(err)
		}
		for _, i := range f.Imports {
			importPath := strings.ReplaceAll(i.Path.Value, `"`, ``)
			if strings.HasPrefix(importPath, modPrefix) {
				importPath = strings.TrimPrefix(importPath, modPrefix)
				entries, err := os.ReadDir(importPath)
				if err != nil {
					panic(err)
				}
				for _, entry := range entries {
					if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
						continue
					}
					if err := buildFile(filepath.Join(importPath, entry.Name()), forceFlag, m); err != nil {
						panic(err)
					}
				}
			}
		}
		return nil
	}

	fset, f := agl.ParseSrc(string(by))
	for _, i := range f.Imports {
		importPath := strings.ReplaceAll(i.Path.Value, `"`, ``)
		if strings.HasPrefix(importPath, modPrefix) {
			importPath = strings.TrimPrefix(importPath, modPrefix)
			entries, err := os.ReadDir(importPath)
			if err != nil {
				panic(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				if err := buildFile(filepath.Join(importPath, entry.Name()), forceFlag, m); err != nil {
					panic(err)
				}
			}
		}
	}
	env := agl.NewEnv()
	i := agl.NewInferrer(env)
	errs := i.InferFile(fileName, f, fset, false)
	if len(errs) > 0 {
		panic(errs[0])
	}
	g := agl.NewGenerator(i.Env, f, fset)
	src := g.Generate()
	path := strings.Replace(fileName, ".agl", ".go", 1)
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		buf := make([]byte, len(generatedFilePrefix))
		if _, err := file.Read(buf); err != nil {
			panic(err)
		}
		if !bytes.Equal(buf, []byte(generatedFilePrefix)) && !forceFlag {
			panic(fmt.Sprintf("%s would be overwritten, use -f to force", path))
		}
	}
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		return err
	}

	packageName := g.PkgName()
	coreFile := filepath.Join(filepath.Dir(path), "aglCore.go")
	err = os.WriteFile(coreFile, []byte(agl.GenCore(packageName)), 0644)
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
			node, err := goparser.ParseFile(fset, fileName, src, goparser.AllErrors)
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
	fset, f := agl.ParseSrc(string(by))
	env := agl.NewEnv()
	i := agl.NewInferrer(env)
	i.InferFile(fileName, f, fset, true)
	src := agl.NewGenerator(i.Env, f, fset).Generate()
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
	fset, f := agl.ParseSrc(string(by))
	env := agl.NewEnv()
	i := agl.NewInferrer(env)
	i.InferFile(fileName, f, fset, true)
	g := agl.NewGenerator(i.Env, f, fset)
	fmt.Println(g.Generate())
	return nil
}

func insertHeadersAfterFirstLine(src, headers string) string {
	lines := strings.Split(src, "\n")
	if len(lines) == 0 {
		return src
	}

	// Find the package declaration line
	packageLineIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			packageLineIndex = i
			break
		}
	}

	if packageLineIndex == -1 {
		// If no package declaration found, just prepend headers
		return headers + "\n" + src
	}

	// Insert headers after the package declaration
	var result []string
	result = append(result, lines[:packageLineIndex+1]...)
	result = append(result, headers)
	result = append(result, lines[packageLineIndex+1:]...)

	return strings.Join(result, "\n")
}

func modInitAction(_ context.Context, c *cli.Command) error {
	cmd := exec.Command("go", "mod", "init", c.Args().First())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func modTidyAction(_ context.Context, c *cli.Command) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func modVendorAction(_ context.Context, c *cli.Command) error {
	entries, _ := os.ReadDir(".")
	var imports []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".agl") {
			by, _ := os.ReadFile(entry.Name())
			_, f := agl.ParseSrc(string(by))
			for _, i := range f.Imports {
				imports = append(imports, fmt.Sprintf("import _ %s", i.Path.Value))
			}
		}
	}
	importsStr := strings.Join(imports, "\n")
	const tmpGoFile = "aglTmp.go"
	_ = os.WriteFile(tmpGoFile, []byte("package main\n"+importsStr), 0644)
	defer os.Remove(tmpGoFile)
	cmd := exec.Command("go", "mod", "vendor")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func versionAction(ctx context.Context, cmd *cli.Command) error {
	fmt.Printf("AGL v%s\n", version)
	return nil
}
