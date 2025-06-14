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
		},
		Action: startAction,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	fmt.Println("is running")
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
	fmt.Println(codegen(i.env, f))
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
