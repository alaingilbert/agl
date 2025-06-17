package main

import (
	"log"
	"os"

	"agl/pkg/lsp"
)

func main() {
	logger := log.New(os.Stderr, "[agl-lsp] ", log.LstdFlags)
	logger.Println("Starting AGL language server...")

	server := lsp.NewServer(os.Stdin)
	if err := server.Run(); err != nil {
		logger.Fatalf("Error running language server: %v", err)
	}
}
