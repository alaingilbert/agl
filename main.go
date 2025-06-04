package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
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
