package lsp

import (
	"path/filepath"
	"strings"
	"sync"

	"github.com/sourcegraph/go-lsp"
)

// Definition represents a definition in the code
type Definition struct {
	Name     string
	Location lsp.Location
	Kind     string // "function", "variable", etc.
}

// DefinitionManager manages definitions across files
type DefinitionManager struct {
	mu          sync.RWMutex
	definitions map[string]map[string]Definition // file -> name -> definition
}

// NewDefinitionManager creates a new definition manager
func NewDefinitionManager() *DefinitionManager {
	return &DefinitionManager{
		definitions: make(map[string]map[string]Definition),
	}
}

// AddDefinition adds a definition to the manager
func (dm *DefinitionManager) AddDefinition(file string, def Definition) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, ok := dm.definitions[file]; !ok {
		dm.definitions[file] = make(map[string]Definition)
	}
	dm.definitions[file][def.Name] = def
}

// GetDefinition looks up a definition by name
func (dm *DefinitionManager) GetDefinition(name string) (Definition, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// Search in all files
	for _, fileDefs := range dm.definitions {
		if def, ok := fileDefs[name]; ok {
			return def, true
		}
	}
	return Definition{}, false
}

// ParseFile parses a file and extracts definitions
func (dm *DefinitionManager) ParseFile(uri string, content string) error {
	// Convert URI to file path
	filePath := strings.TrimPrefix(uri, "file://")
	filePath = filepath.Clean(filePath)

	// TODO: Implement actual parsing of AGL code
	// For now, we'll use a simple heuristic to find definitions
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		// Look for function definitions (simple heuristic)
		if strings.HasPrefix(strings.TrimSpace(line), "func ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				name := strings.TrimSuffix(parts[1], "(")
				def := Definition{
					Name: name,
					Location: lsp.Location{
						URI: uri,
						Range: lsp.Range{
							Start: lsp.Position{
								Line:      i,
								Character: 0,
							},
							End: lsp.Position{
								Line:      i,
								Character: len(line),
							},
						},
					},
					Kind: "function",
				}
				dm.AddDefinition(filePath, def)
			}
		}
		// Look for variable definitions (simple heuristic)
		if strings.Contains(line, ":=") {
			parts := strings.Split(line, ":=")
			if len(parts) >= 1 {
				name := strings.TrimSpace(parts[0])
				def := Definition{
					Name: name,
					Location: lsp.Location{
						URI: uri,
						Range: lsp.Range{
							Start: lsp.Position{
								Line:      i,
								Character: 0,
							},
							End: lsp.Position{
								Line:      i,
								Character: len(line),
							},
						},
					},
					Kind: "variable",
				}
				dm.AddDefinition(filePath, def)
			}
		}
	}

	return nil
}

// FindDefinitionAtPosition finds a definition at the given position
func (dm *DefinitionManager) FindDefinitionAtPosition(uri string, position lsp.Position) (Definition, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	// Get the line content
	filePath := strings.TrimPrefix(uri, "file://")
	filePath = filepath.Clean(filePath)

	// TODO: Implement actual parsing of the line to find the identifier
	// For now, we'll use a simple heuristic
	// In a real implementation, you would:
	// 1. Parse the line to find the identifier at the cursor position
	// 2. Look up that identifier in the definitions map
	// 3. Return the definition if found

	return Definition{}, false
}
