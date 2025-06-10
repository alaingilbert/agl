package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/urfave/cli/v3"
	"log"
	"os"
	"strings"
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
				_, _ = fmt.Fprintln(os.Stderr, aglErr.Error())
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
	out := codegen(infer(parser(NewTokenStream(string(by)))))
	fmt.Println(out)
	return nil
}
