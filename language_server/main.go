package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"agl/pkg/agl"
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/token"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
)

type Server struct {
	documents map[string]*Document
	fset      *token.FileSet
}

type Document struct {
	content string
	ast     *ast.File
	env     *agl.Env
}

func NewServer() *Server {
	return &Server{
		documents: make(map[string]*Document),
		fset:      token.NewFileSet(),
	}
}

func (s *Server) Initialize(ctx context.Context, params lsp.InitializeParams) (lsp.InitializeResult, error) {
	kind := lsp.TDSKFull
	return lsp.InitializeResult{
		Capabilities: lsp.ServerCapabilities{
			TextDocumentSync: &lsp.TextDocumentSyncOptionsOrKind{
				Kind: &kind,
			},
			DefinitionProvider: true,
			HoverProvider:      true,
		},
	}, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

func (s *Server) Exit(ctx context.Context) error {
	os.Exit(0)
	return nil
}

func (s *Server) DidOpen(ctx context.Context, params lsp.DidOpenTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	content := params.TextDocument.Text
	return s.updateDocument(uri, content)
}

func (s *Server) DidChange(ctx context.Context, params lsp.DidChangeTextDocumentParams) error {
	if len(params.ContentChanges) > 0 {
		uri := string(params.TextDocument.URI)
		content := params.ContentChanges[0].Text
		return s.updateDocument(uri, content)
	}
	return nil
}

func (s *Server) DidClose(ctx context.Context, params lsp.DidCloseTextDocumentParams) error {
	uri := string(params.TextDocument.URI)
	delete(s.documents, uri)
	return nil
}

func (s *Server) updateDocument(uri string, content string) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("%v", r)
		}
	}()
	// Parse the file
	file, err := parser.ParseFile(s.fset, uri, content, 0)
	if err != nil {
		return fmt.Errorf("failed to parse file: %v", err)
	}

	// Create environment and infer types
	env := agl.NewEnv(s.fset)
	inferrer := agl.NewInferrer(s.fset, env)
	inferrer.InferFile(file)

	// Store the document
	s.documents[uri] = &Document{
		content: content,
		ast:     file,
		env:     env,
	}

	return nil
}

func (s *Server) Definition(ctx context.Context, params lsp.TextDocumentPositionParams) ([]lsp.Location, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := s.documents[uri]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", uri)
	}

	// Convert LSP position to Go token position
	// LSP uses 0-based line numbers, Go uses 1-based
	line := params.Position.Line + 1
	column := params.Position.Character + 1

	// Find the file in the file set
	file := s.fset.File(doc.ast.Pos())
	if file == nil {
		log.Printf("File not found in file set")
		return nil, nil
	}

	// Convert line/column to offset
	offset := file.LineStart(line) + token.Pos(column-1)

	node := s.findNodeAtPosition(doc.ast, s.fset.Position(offset))

	// If it's an identifier, look up its definition
	if ident, ok := node.(*ast.Ident); ok {
		// Look up the symbol in the environment
		if tmp := doc.env.GetInfo(ident); tmp != nil {
			pos := s.fset.Position(tmp.Definition)
			return []lsp.Location{
				{
					URI: lsp.DocumentURI(uri),
					Range: lsp.Range{
						Start: lsp.Position{
							Line:      pos.Line - 1,
							Character: pos.Column - 1,
						},
						End: lsp.Position{
							Line:      pos.Line - 1,
							Character: pos.Column - 1,
						},
					},
				},
			}, nil
		}
		if typ := doc.env.Get(ident.Name); typ != nil {
			// For now, return the current position as the definition
			// TODO: Implement proper definition lookup
			return []lsp.Location{
				{
					URI: lsp.DocumentURI(uri),
					Range: lsp.Range{
						Start: params.Position,
						End:   params.Position,
					},
				},
			}, nil
		}
	}

	return nil, nil
}

func (s *Server) findNodeAtPosition(file *ast.File, pos token.Position) ast.Node {
	var result ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		nodePos := s.fset.Position(n.Pos())
		nodeEnd := s.fset.Position(n.End())

		// Check if the position is within this node's range
		if nodePos.Offset <= pos.Offset && pos.Offset <= nodeEnd.Offset {
			// If this is a more specific node (smaller range), use it
			if result == nil ||
				(s.fset.Position(result.Pos()).Offset <= nodePos.Offset &&
					nodeEnd.Offset <= s.fset.Position(result.End()).Offset) {
				result = n
			}
			return true // Continue searching for more specific nodes
		}
		return false
	})
	return result
}

func (s *Server) Hover(ctx context.Context, params lsp.TextDocumentPositionParams) (*lsp.Hover, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := s.documents[uri]
	if !ok {
		log.Printf("Document not found: %s", uri)
		return nil, fmt.Errorf("document not found: %s", uri)
	}

	// Convert LSP position to Go token position
	// LSP uses 0-based line numbers, Go uses 1-based
	line := params.Position.Line + 1
	column := params.Position.Character + 1

	// Find the file in the file set
	file := s.fset.File(doc.ast.Pos())
	if file == nil {
		log.Printf("File not found in file set")
		return nil, nil
	}

	// Convert line/column to offset
	offset := file.LineStart(line) + token.Pos(column-1)

	// Find the node at the calculated position
	node := s.findNodeAtPosition(doc.ast, s.fset.Position(offset))

	// Look up the symbol in the environment
	if info := doc.env.GetInfo(node); info != nil {
		typ := info.Type
		if typ == nil {
			return nil, nil
		}
		pos := s.fset.Position(node.Pos())
		l := int(node.End() - node.Pos())
		startPos := lsp.Position{Line: pos.Line - 1, Character: pos.Column - 1}
		endPos := lsp.Position{Line: startPos.Line, Character: startPos.Character + l}
		return &lsp.Hover{
			Contents: []lsp.MarkedString{{Language: "agl", Value: typ.String()}},
			Range: &lsp.Range{
				Start: startPos,
				End:   endPos,
			},
		}, nil
	}
	return nil, nil
}

type handler struct {
	server *Server
	conn   *jsonrpc2.Conn
}

func (h *handler) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	var result interface{}
	var err error

	switch req.Method {
	case "initialize":
		var params lsp.InitializeParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Initialize(ctx, params)

	case "shutdown":
		err = h.server.Shutdown(ctx)
		result = nil

	case "exit":
		err = h.server.Exit(ctx)
		result = nil

	case "textDocument/didOpen":
		var params lsp.DidOpenTextDocumentParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidOpen(ctx, params)
		result = nil

	case "textDocument/didChange":
		var params lsp.DidChangeTextDocumentParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidChange(ctx, params)
		result = nil

	case "textDocument/didClose":
		var params lsp.DidCloseTextDocumentParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidClose(ctx, params)
		result = nil

	case "textDocument/definition":
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Definition(ctx, params)

	case "textDocument/hover":
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Hover(ctx, params)

	default:
		conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: fmt.Sprintf("method not supported: %s", req.Method)})
		return
	}

	if err != nil {
		conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: err.Error()})
		return
	}

	if err := conn.Reply(ctx, req.ID, result); err != nil {
		log.Printf("failed to reply: %v", err)
	}
}

func main() {
	server := NewServer()

	// Create a handler that implements the LSP protocol
	h := &handler{server: server}

	// Create a new connection
	conn := jsonrpc2.NewConn(
		context.Background(),
		jsonrpc2.NewBufferedStream(stdrwc{}, jsonrpc2.VSCodeObjectCodec{}),
		h,
	)

	// Wait for the connection to close
	<-conn.DisconnectNotify()
}

// stdrwc implements io.ReadWriteCloser for stdin/stdout
type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}
