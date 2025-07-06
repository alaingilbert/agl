package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"runtime/debug"

	"agl/pkg/agl"
	"agl/pkg/ast"
	"agl/pkg/parser"
	"agl/pkg/scanner"
	"agl/pkg/token"
	"agl/pkg/types"

	"github.com/sourcegraph/go-lsp"
	"github.com/sourcegraph/jsonrpc2"
)

type Server struct {
	documents map[string]*Document
	fset      *token.FileSet
	conn      *jsonrpc2.Conn
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
			CompletionProvider: &lsp.CompletionOptions{
				TriggerCharacters: []string{"."},
			},
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
			log.Printf("Error: %v", r)
			debug.PrintStack()
		}
	}()
	// Parse the file
	content += agl.CoreFns()
	file, err := parser.ParseFile(s.fset, uri, content, 0)
	if err != nil {
		// Convert parser errors to LSP diagnostics
		if parserErr, ok := err.(scanner.ErrorList); ok {
			diagnostics := make([]lsp.Diagnostic, 0, len(parserErr))
			for _, e := range parserErr {
				diagnostics = append(diagnostics, lsp.Diagnostic{
					Range: lsp.Range{
						Start: lsp.Position{
							Line:      e.Pos.Line - 1,
							Character: e.Pos.Column - 1,
						},
						End: lsp.Position{
							Line:      e.Pos.Line - 1,
							Character: e.Pos.Column,
						},
					},
					Severity: lsp.Error,
					Message:  e.Msg,
				})
			}
			// Send diagnostic notification
			if s.conn != nil {
				_ = s.conn.Notify(context.Background(), "textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
					URI:         lsp.DocumentURI(uri),
					Diagnostics: diagnostics,
				})
			}
		}
		return fmt.Errorf("failed to parse file: %v", err)
	}

	// Create an environment and infer types
	env := agl.NewEnv()
	inferrer := agl.NewInferrer(env)
	errs := inferrer.InferFile(uri, file, s.fset)
	if len(errs) > 0 {
		diagnostics := make([]lsp.Diagnostic, 0, len(errs))
		for _, e := range errs {
			var ee *agl.InferError
			if errors.As(e, &ee) {
				pos, endPos := s.fset.Position(ee.N.Pos()), s.fset.Position(ee.N.End())
				diagnostics = append(diagnostics, lsp.Diagnostic{
					Range: lsp.Range{
						Start: lsp.Position{Line: pos.Line - 1, Character: pos.Column - 1},
						End:   lsp.Position{Line: endPos.Line - 1, Character: endPos.Column - 1},
					},
					Severity: lsp.Error,
					Message:  e.Error(),
				})
			}
		}
		// Send diagnostic notification
		if s.conn != nil {
			_ = s.conn.Notify(context.Background(), "textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
				URI:         lsp.DocumentURI(uri),
				Diagnostics: diagnostics,
			})
		}
		return fmt.Errorf("failed to infer file: %v", err)
	}

	// Store the document
	s.documents[uri] = &Document{
		content: content,
		ast:     file,
		env:     env,
	}

	// Clear any existing diagnostics since the file is now valid
	if s.conn != nil {
		_ = s.conn.Notify(context.Background(), "textDocument/publishDiagnostics", lsp.PublishDiagnosticsParams{
			URI:         lsp.DocumentURI(uri),
			Diagnostics: []lsp.Diagnostic{}, // Empty diagnostics list to clear errors
		})
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
		var contents []lsp.MarkedString
		contents = append(contents, lsp.MarkedString{Language: "agl", Value: typ.String()})
		if info.Message != "" {
			contents = append(contents, lsp.MarkedString{Language: "markdown", Value: info.Message})
		}
		return &lsp.Hover{
			Contents: contents,
			Range: &lsp.Range{
				Start: startPos,
				End:   endPos,
			},
		}, nil
	}
	return nil, nil
}

func (s *Server) Completion(ctx context.Context, params lsp.TextDocumentPositionParams) (*lsp.CompletionList, error) {
	uri := string(params.TextDocument.URI)
	doc, ok := s.documents[uri]
	if !ok {
		return nil, fmt.Errorf("document not found: %s", uri)
	}

	// Convert LSP position to Go token position
	line := params.Position.Line + 1
	column := params.Position.Character + 1

	// Find the file in the file set
	file := s.fset.File(doc.ast.Pos())
	if file == nil {
		return nil, nil
	}

	// Convert line/column to offset
	offset := file.LineStart(line) + token.Pos(column-1)

	// Find the node at the current position
	node := s.findNodeAtPosition(doc.ast, s.fset.Position(offset-1))

	// Get completions based on the current context
	completions := s.getCompletions(doc, node, offset)

	return &lsp.CompletionList{
		IsIncomplete: false,
		Items:        completions,
	}, nil
}

func (s *Server) getCompletions(doc *Document, node ast.Node, offset token.Pos) []lsp.CompletionItem {
	var completions []lsp.CompletionItem
	if info := doc.env.GetInfo(node); info != nil {
		if typ := info.Type; typ != nil {
			typ = types.Unwrap(typ)
			switch typ.(type) {
			case types.ArrayType:
				completions = append(completions,
					lsp.CompletionItem{Label: "AllSatisfy", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Any", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Clone", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Clone", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Contains", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Enumerated", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Filter", Kind: lsp.CIKMethod, Detail: "Filter elements"},
					lsp.CompletionItem{Label: "Find", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "First", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Indices", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Insert", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsEmpty", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Joined", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Last", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Len", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Map", Kind: lsp.CIKMethod, Detail: "Transform elements"},
					lsp.CompletionItem{Label: "Max", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Min", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Pop", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "PopFront", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "PopIf", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Push", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "PushFront", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Reduce", Kind: lsp.CIKMethod, Detail: "Reduce elements"},
					lsp.CompletionItem{Label: "Remove", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Sum", Kind: lsp.CIKMethod, Detail: ""},
				)
			case types.MapType:
				completions = append(completions,
					lsp.CompletionItem{Label: "Get", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Keys", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Len", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Values", Kind: lsp.CIKMethod, Detail: ""},
				)
			case types.SetType:
				completions = append(completions,
					lsp.CompletionItem{Label: "Contains", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "FormIntersection", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "FormSymmetricDifference", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "FormUnion", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Insert", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Intersection", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Intersects", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsDisjoint", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsStrictSubset", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsStrictSuperset", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsSubset", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsSuperset", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Len", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Max", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Min", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Remove", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Subtract", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Subtracting", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "SymmetricDifference", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Union", Kind: lsp.CIKMethod, Detail: ""},
				)
			case types.OptionType:
				completions = append(completions,
					lsp.CompletionItem{Label: "IsNone", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsSome", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Unwrap", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "UnwrapOr", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "UnwrapOrDefault", Kind: lsp.CIKMethod, Detail: ""},
				)
			case types.ResultType:
				completions = append(completions,
					lsp.CompletionItem{Label: "IsErr", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "IsOk", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Unwrap", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "UnwrapOr", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "UnwrapOrDefault", Kind: lsp.CIKMethod, Detail: ""},
				)
			case types.UntypedNumType, types.IntType:
				completions = append(completions,
					lsp.CompletionItem{Label: "String", Kind: lsp.CIKMethod, Detail: "String"},
				)
			case types.UntypedStringType, types.StringType:
				completions = append(completions,
					lsp.CompletionItem{Label: "HasPrefix", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "HasSuffix", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "I16", Kind: lsp.CIKMethod, Detail: "", InsertText: "I16()"},
					lsp.CompletionItem{Label: "I32", Kind: lsp.CIKMethod, Detail: "", InsertText: "I32()"},
					lsp.CompletionItem{Label: "I64", Kind: lsp.CIKMethod, Detail: "", InsertText: "I64()"},
					lsp.CompletionItem{Label: "I8", Kind: lsp.CIKMethod, Detail: "", InsertText: "I8()"},
					lsp.CompletionItem{Label: "Int", Kind: lsp.CIKMethod, Detail: "", InsertText: "Int()"},
					lsp.CompletionItem{Label: "Lowercased", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "Split", Kind: lsp.CIKMethod, Detail: ""},
					lsp.CompletionItem{Label: "U16", Kind: lsp.CIKMethod, Detail: "", InsertText: "U16()"},
					lsp.CompletionItem{Label: "U32", Kind: lsp.CIKMethod, Detail: "", InsertText: "U32()"},
					lsp.CompletionItem{Label: "U64", Kind: lsp.CIKMethod, Detail: "", InsertText: "U64()"},
					lsp.CompletionItem{Label: "U8", Kind: lsp.CIKMethod, Detail: "", InsertText: "U8()"},
					lsp.CompletionItem{Label: "Uint", Kind: lsp.CIKMethod, Detail: "", InsertText: "Uint()"},
					lsp.CompletionItem{Label: "Uppercased", Kind: lsp.CIKMethod, Detail: ""},
				)
			default:
				log.Printf("Unknown type: %s %v", typ, reflect.TypeOf(typ))
			}
		}
	}
	return completions
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
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
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
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidOpen(ctx, params)
		result = nil

	case "textDocument/didChange":
		var params lsp.DidChangeTextDocumentParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidChange(ctx, params)
		result = nil

	case "textDocument/didClose":
		var params lsp.DidCloseTextDocumentParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		err = h.server.DidClose(ctx, params)
		result = nil

	case "textDocument/definition":
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Definition(ctx, params)

	case "textDocument/hover":
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Hover(ctx, params)

	case "textDocument/completion":
		var params lsp.TextDocumentPositionParams
		if err := json.Unmarshal(*req.Params, &params); err != nil {
			conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeParseError, Message: err.Error()})
			return
		}
		result, err = h.server.Completion(ctx, params)

	default:
		_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeMethodNotFound, Message: fmt.Sprintf("method not supported: %s", req.Method)})
		return
	}

	if err != nil {
		_ = conn.ReplyWithError(ctx, req.ID, &jsonrpc2.Error{Code: jsonrpc2.CodeInternalError, Message: err.Error()})
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
	server.conn = conn

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
