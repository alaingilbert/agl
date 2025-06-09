package main

import (
	"errors"
	"fmt"
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
	fileName := os.Args[1]
	if !strings.HasSuffix(fileName, ".agl") {
		panic("must have agl extension")
	}
	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	out := codegen(infer(parser(NewTokenStream(string(by)))))
	fmt.Println(out)
}
