package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/sourcegraph/go-lsp"
)

// Server represents the language server
type Server struct {
	conn   io.ReadWriteCloser
	logger *log.Logger
}

// NewServer creates a new language server instance
func NewServer(conn io.ReadWriteCloser) *Server {
	return &Server{
		conn:   conn,
		logger: log.New(os.Stderr, "[agl-lsp] ", log.LstdFlags),
	}
}

// Initialize handles the initialize request
func (s *Server) Initialize(ctx context.Context, params lsp.InitializeParams) (*lsp.InitializeResult, error) {
	s.logger.Printf("Initializing language server for %s", params.RootURI)

	return &lsp.InitializeResult{
		Capabilities: lsp.ServerCapabilities{
			TextDocumentSync: &lsp.TextDocumentSyncOptions{
				OpenClose: true,
				Change:    lsp.TDSKFull,
			},
			CompletionProvider: &lsp.CompletionOptions{
				TriggerCharacters: []string{".", ":"},
			},
			HoverProvider:      true,
			DefinitionProvider: true,
		},
	}, nil
}

// Shutdown handles the shutdown request
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Println("Shutting down language server")
	return nil
}

// Exit handles the exit notification
func (s *Server) Exit(ctx context.Context) error {
	s.logger.Println("Exiting language server")
	return nil
}

// DidOpen handles the textDocument/didOpen notification
func (s *Server) DidOpen(ctx context.Context, params lsp.DidOpenTextDocumentParams) error {
	s.logger.Printf("Document opened: %s", params.TextDocument.URI)
	return nil
}

// DidChange handles the textDocument/didChange notification
func (s *Server) DidChange(ctx context.Context, params lsp.DidChangeTextDocumentParams) error {
	s.logger.Printf("Document changed: %s", params.TextDocument.URI)
	return nil
}

// DidClose handles the textDocument/didClose notification
func (s *Server) DidClose(ctx context.Context, params lsp.DidCloseTextDocumentParams) error {
	s.logger.Printf("Document closed: %s", params.TextDocument.URI)
	return nil
}

// Completion handles the textDocument/completion request
func (s *Server) Completion(ctx context.Context, params lsp.CompletionParams) (*lsp.CompletionList, error) {
	s.logger.Printf("Completion requested for: %s", params.TextDocument.URI)

	// TODO: Implement actual completion logic
	return &lsp.CompletionList{
		IsIncomplete: false,
		Items: []lsp.CompletionItem{
			{
				Label:  "example",
				Kind:   lsp.CIKText,
				Detail: "Example completion item",
			},
		},
	}, nil
}

// Hover handles the textDocument/hover request
func (s *Server) Hover(ctx context.Context, params lsp.TextDocumentPositionParams) (*lsp.Hover, error) {
	s.logger.Printf("Hover requested for: %s", params.TextDocument.URI)

	// TODO: Implement actual hover logic
	return &lsp.Hover{
		Contents: []lsp.MarkedString{
			{
				Language: "agl",
				Value:    "Example hover content",
			},
		},
	}, nil
}

// Definition handles the textDocument/definition request
func (s *Server) Definition(ctx context.Context, params lsp.TextDocumentPositionParams) ([]lsp.Location, error) {
	s.logger.Printf("Definition requested for: %s", params.TextDocument.URI)

	// TODO: Implement actual definition logic
	return []lsp.Location{}, nil
}

// Run starts the language server
func (s *Server) Run() error {
	decoder := json.NewDecoder(s.conn)
	encoder := json.NewEncoder(s.conn)

	for {
		var msg json.RawMessage
		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("failed to decode message: %w", err)
		}

		var request lsp.Request
		if err := json.Unmarshal(msg, &request); err != nil {
			return fmt.Errorf("failed to unmarshal request: %w", err)
		}

		// Handle the request based on its method
		switch request.Method {
		case "initialize":
			var params lsp.InitializeParams
			if err := json.Unmarshal(request.Params, &params); err != nil {
				return fmt.Errorf("failed to unmarshal initialize params: %w", err)
			}
			result, err := s.Initialize(context.Background(), params)
			if err != nil {
				return fmt.Errorf("failed to handle initialize: %w", err)
			}
			if err := encoder.Encode(lsp.ResponseMessage{
				ID:     request.ID,
				Result: result,
			}); err != nil {
				return fmt.Errorf("failed to encode initialize response: %w", err)
			}
			// Add more method handlers here
		}
	}
}
