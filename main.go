package main

import (
	goast "agl/ast"
	parser1 "agl/parser"
	"agl/token"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/urfave/cli/v3"
)

type AglError struct {
	msg string
}

func (e *AglError) Error() string {
	return e.msg
}

func NewAglError(msg string) *AglError {
	return &AglError{msg: msg}
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			var aglErr *AglError
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
				Action:  runAction,
			},
			{
				Name:    "build",
				Aliases: []string{"b"},
				Usage:   "build command",
				Action:  buildAction,
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

	coreFile := filepath.Join(tmpDir, "core.go")
	err = os.WriteFile(coreFile, []byte(genCore()), 0644)
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
	i := NewInferrer(fset)
	i.InferFile(f)
	src := NewGenerator(i.env, f).Generate()
	_ = spawnGoRunFromBytes([]byte(src))
	return nil
}

func buildAction(ctx context.Context, cmd *cli.Command) error {
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
	i := NewInferrer(fset)
	i.InferFile(f)
	src := NewGenerator(i.env, f).Generate()
	if err := os.WriteFile(strings.Replace(fileName, ".agl", ".go", 1), []byte(src), 0644); err != nil {
		return err
	}
	return nil
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
	i := NewInferrer(fset)
	i.InferFile(f)
	g := NewGenerator(i.env, f)
	fmt.Println(g.Generate())
	return nil
}

func parser2(src string) (*token.FileSet, *goast.File) {
	var fset = token.NewFileSet()
	f, err := parser1.ParseFile(fset, "", src, 0)
	if err != nil {
		panic(err)
	}
	return fset, f
}
