package main

import (
	"agl/pkg/agl"
	"agl/pkg/parser"
	"agl/pkg/token"
	"bytes"
	"context"
	"errors"
	"fmt"
	goast "go/ast"
	goparser "go/parser"
	gotoken "go/token"
	gotypes "go/types"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"

	"github.com/urfave/cli/v3"
)

var (
	version = "0.0.1"
	commit  = "unknown"
)

func main() {
	defer func() {
		if r := recover(); r != nil {
			var aglErr *agl.AglError
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
	// If commit wasn't set via linker flags, try to get it from git
	if commit == "unknown" {
		gitCmd := exec.Command("git", "rev-parse", "--short=7", "HEAD")
		if output, err := gitCmd.Output(); err == nil {
			commit = strings.TrimSpace(string(output))
		}
	}
	versionStr := version
	if commit != "unknown" {
		versionStr = fmt.Sprintf("%s (%s)", version, commit)
	}
	cmd := &cli.Command{
		Name:    "AGL",
		Usage:   "AnotherGoLang",
		Version: versionStr,
		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "run command",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "debug"},
					&cli.BoolFlag{Name: "no-mut-check", Usage: "disable mutation checking"},
					&cli.BoolFlag{Name: "allow-unused", Usage: ""},
					&cli.BoolFlag{Name: "release", Usage: "disable assert and assertEq code generation"},
				},
				Action: runAction,
			},
			{
				Name:   "execute",
				Usage:  "execute command",
				Action: executeAction,
			},
			{
				Name:    "build",
				Aliases: []string{"b"},
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "force", Aliases: []string{"f"}},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}},
					&cli.BoolFlag{Name: "sourceMap"},
					&cli.BoolFlag{Name: "no-mut-check", Usage: "disable mutation checking"},
					&cli.BoolFlag{Name: "allow-unused", Usage: ""},
					&cli.BoolFlag{Name: "standalone", Usage: "embed core library in generated file"},
					&cli.BoolFlag{Name: "release", Usage: "disable assert and assertEq code generation"},
				},
				Usage:  "build command",
				Action: buildAction,
			},
			{
				Name:   "clean",
				Usage:  "cleanup agl generated files",
				Action: cleanupAction,
			},
			{
				Name:   "version",
				Usage:  "print AGL version",
				Action: versionAction,
			},
			{
				Name: "mod",
				Commands: []*cli.Command{
					{
						Name:   "init",
						Usage:  "initialize a new module",
						Action: modInitAction,
					},
					{
						Name:   "tidy",
						Action: modTidyAction,
					},
					{
						Name:   "vendor",
						Action: modVendorAction,
					},
				},
			},
		},
		Action: startAction,
	}
	if err := cmd.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

type RuntimeError struct {
	err string
}

func (i *RuntimeError) Error() string {
	return i.err
}

func spawnGoRunFromBytes(g *agl.Generator, fset *token.FileSet, source []byte, programArgs []string) error {
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

	coreFile := filepath.Join(tmpDir, "aglCore.go")
	err = os.WriteFile(coreFile, []byte(agl.GenCore("main")), 0644)
	if err != nil {
		return err
	}

	// Run `go run` on the file with additional arguments
	cmdArgs := append([]string{"run", tmpFile, coreFile}, programArgs...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdout = os.Stdout
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	err = cmd.Run()
	stdErrStr := buf.String()
	if stdErrStr != "" {
		fileName := "main.go"
		aglFileName := "main.agl"
		rgx := regexp.MustCompile(`/main\.go:(\d+)`)
		matches := rgx.FindAllStringSubmatch(stdErrStr, -1)
		for _, match := range matches {
			origLine, _ := strconv.Atoi(match[1])
			n := g.GenerateFrags(origLine)
			if n != nil {
				nPos := fset.Position(n.Pos())
				from := fmt.Sprintf("%s:%d", fileName, origLine)
				to := fmt.Sprintf("%s:%d (%s:%d)", fileName, origLine, aglFileName, nPos.Line)
				stdErrStr = strings.Replace(stdErrStr, from, to, 1)
			}
		}
	}
	if err != nil {
		panic(&RuntimeError{err: stdErrStr})
	} else {
		print(stdErrStr)
	}
	return err
}

// hasGoMod checks if a go.mod file exists in the same directory as the given file
func hasGoMod(fileName string) bool {
	dir := filepath.Dir(fileName)
	goModPath := filepath.Join(dir, "go.mod")
	_, err := os.Stat(goModPath)
	return err == nil
}

func spawnGoBuild(fileName, outputFlag string, standalone bool) error {
	isAgl := strings.HasSuffix(fileName, ".agl")
	fileName = strings.Replace(fileName, ".agl", ".go", 1)
	var cmdArgs []string
	cmdArgs = append(cmdArgs, "build")
	if outputFlag != "" {
		cmdArgs = append(cmdArgs, "-o", outputFlag)
	}
	dir := filepath.Dir(fileName)
	cmdArgs = append(cmdArgs, fileName)
	// Only include aglCore.go if not in standalone mode
	if isAgl && !standalone {
		cmdArgs = append(cmdArgs, filepath.Join(dir, "aglCore.go"))
	}
	cmd := exec.Command("go", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func executeAction(ctx context.Context, cmd *cli.Command) error {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("EXIT_CODE:1")
			var aglErr *agl.AglError
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
	var input string
	if cmd.NArg() > 0 {
		input = cmd.Args().Get(0)
	}
	core, _ := agl.ContentFs.ReadFile(filepath.Join("core", "core.go"))
	coreLines := strings.Split(string(core), "\n")
	coreImports := []byte(strings.Join(coreLines[:18], "\n"))
	_, _, out := genCode1("", []byte(input), coreImports, true, false, false)
	lines := strings.Split(out, "\n")
	out = strings.Join(lines, "\n")
	out += strings.Join(coreLines[18:], "\n")
	fmt.Println(out)
	return nil
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	debugFlag := cmd.Bool("debug")
	noMutCheck := cmd.Bool("no-mut-check")
	allowUnused := cmd.Bool("allow-unused")
	releaseFlag := cmd.Bool("release")

	defer func() {
		if r := recover(); r != nil {
			var aglErr *agl.AglError
			if err, ok := r.(error); ok && errors.As(err, &aglErr) {
				msg := aglErr.Error()
				if msg == "" || debugFlag {
					msg += string(debug.Stack())
				}
				_, _ = fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			var inferErr *agl.InferError
			if err, ok := r.(error); ok && errors.As(err, &inferErr) {
				msg := inferErr.Error()
				if msg == "" || debugFlag {
					msg += string(debug.Stack())
				}
				_, _ = fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			var rtErr *RuntimeError
			if err, ok := r.(error); ok && errors.As(err, &rtErr) {
				msg := rtErr.Error()
				if msg == "" || debugFlag {
					msg += string(debug.Stack())
				}
				_, _ = fmt.Fprintln(os.Stderr, msg)
				os.Exit(1)
			}
			panic(r)
		}
	}()

	if cmd.NArg() == 0 {
		fmt.Println("You must specify a file to compile")
		return nil
	}
	fileName := cmd.Args().Get(0)
	if !strings.HasSuffix(fileName, ".agl") {
		fmt.Println("file must have '.agl' extension")
		return nil
	}
	by := agl.Must(os.ReadFile(fileName))
	mutEnforced := !noMutCheck
	g, fset, src := genCode1(fileName, by, nil, mutEnforced, allowUnused, releaseFlag)

	// Get any additional arguments to pass to the program
	var programArgs []string
	if cmd.NArg() > 1 {
		programArgs = cmd.Args().Slice()[1:] // Skip the .agl filename
	}

	_ = spawnGoRunFromBytes(g, fset, []byte(src), programArgs)
	return nil
}

// Walk the current directory and subdirectories and delete all `*.go` files that start with `// agl:generated`
func cleanupAction(ctx context.Context, cmd *cli.Command) error {
	generatedFilePrefix := agl.GeneratedFilePrefix
	_ = fs.WalkDir(os.DirFS("."), ".", func(path string, d fs.DirEntry, err error) error {
		base := filepath.Base(path)
		if d.IsDir() && (base == "vendor" || base == "node_modules") {
			return fs.SkipDir
		}
		if strings.HasSuffix(path, ".go") {
			if file, err := os.Open(path); err == nil {
				defer file.Close()
				buf := make([]byte, len(generatedFilePrefix))
				if _, err := file.Read(buf); err == nil {
					if bytes.Equal(buf, []byte(generatedFilePrefix)) {
						fmt.Println("removing", path)
						if err := os.Remove(path); err != nil {
							panic(err)
						}
					}
				}
			}
		}
		return nil
	})
	return nil
}

func buildAction(ctx context.Context, cmd *cli.Command) error {
	noMutCheck := cmd.Bool("no-mut-check")
	allowUnused := cmd.Bool("allow-unused")
	standalone := cmd.Bool("standalone")
	releaseFlag := cmd.Bool("release")
	mutEnforced := !noMutCheck

	if cmd.NArg() == 0 {
		return buildProject(mutEnforced, releaseFlag)
	}
	outputFlag := cmd.String("output")
	forceFlag := cmd.Bool("force")
	sourceMapFlag := cmd.Bool("sourceMap")
	fileName := cmd.Args().Get(0)

	// Auto-enable standalone mode if no go.mod exists in the same directory
	if !standalone && !hasGoMod(fileName) {
		standalone = true
	}

	m := agl.NewPkgVisited()
	err := buildFile(fileName, forceFlag, sourceMapFlag, standalone, mutEnforced, allowUnused, releaseFlag, m)
	if err != nil {
		panic(err)
	}
	return spawnGoBuild(fileName, outputFlag, standalone)
}

// extractIdentifiers extracts all capitalized identifiers from a line of code
func extractIdentifiers(line string) []string {
	var result []string
	words := strings.FieldsFunc(line, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_')
	})
	for _, word := range words {
		if len(word) > 0 && word[0] >= 'A' && word[0] <= 'Z' {
			// Remove generic brackets
			word = strings.Split(word, "[")[0]
			result = append(result, word)
		}
	}
	return result
}

// filterUnusedCode removes functions and types from coreBody that aren't referenced in src
func filterUnusedCode(src string, coreBody []string) []string {
	// Find all identifiers used in src
	usedIdents := make(map[string]bool)

	lines := strings.Split(src, "\n")
	for _, line := range lines {
		for _, word := range extractIdentifiers(line) {
			usedIdents[word] = true
		}
	}

	// Parse coreBody to identify function/type definitions and their dependencies
	type Definition struct {
		name         string
		receiverType string // For methods - the type they belong to
		lines        []string
		deps         map[string]bool
	}

	definitions := []Definition{}
	var currentDef *Definition

	for i := 0; i < len(coreBody); i++ {
		line := coreBody[i]
		trimmed := strings.TrimSpace(line)

		// Check for function or type definition
		if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") {
			// Save previous definition
			if currentDef != nil {
				definitions = append(definitions, *currentDef)
			}

			// Start new definition
			currentDef = &Definition{
				lines: []string{line},
				deps:  make(map[string]bool),
			}

			// Extract name
			if strings.HasPrefix(trimmed, "func ") {
				// Handle: func Name(...) or func (r Receiver) Name(...)
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					if parts[1] == "(" || strings.HasPrefix(parts[1], "(") {
						// Method: func (r Receiver) Name or func (r Receiver[T]) Name
						// Find the receiver type - it's between ( and )
						fullLine := strings.TrimPrefix(trimmed, "func ")
						if idx := strings.Index(fullLine, ")"); idx != -1 {
							receiverPart := fullLine[1:idx] // Skip the opening (
							methodNamePart := strings.TrimSpace(fullLine[idx+1:])

							// Extract receiver type (last word in receiver part)
							receiverFields := strings.Fields(receiverPart)
							receiverType := ""
							if len(receiverFields) > 0 {
								receiverType = receiverFields[len(receiverFields)-1]
								// Remove pointer, closing ), and generics
								receiverType = strings.TrimPrefix(receiverType, "*")
								receiverType = strings.TrimSuffix(receiverType, ")")
								receiverType = strings.Split(receiverType, "[")[0]
								currentDef.receiverType = receiverType
								currentDef.deps[receiverType] = true
							}

							// Extract method name
							if len(methodNamePart) > 0 {
								methodName := strings.Split(methodNamePart, "(")[0]
								methodName = strings.Split(methodName, "[")[0]
								// Make method names unique by combining with receiver type
								if receiverType != "" {
									currentDef.name = receiverType + "." + methodName
								} else {
									currentDef.name = methodName
								}
							}
						}
					} else {
						// Function: func Name
						currentDef.name = strings.Split(parts[1], "(")[0]
						currentDef.name = strings.Split(currentDef.name, "[")[0] // Remove generics
					}
				}
			} else if strings.HasPrefix(trimmed, "type ") {
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					currentDef.name = parts[1]
					// Handle generic types like Iterator[T]
					currentDef.name = strings.Split(currentDef.name, "[")[0]
				}
			}

			// Find dependencies in this line (look for all identifiers)
			for _, word := range extractIdentifiers(line) {
				if word != currentDef.name {
					currentDef.deps[word] = true
				}
			}
		} else if currentDef != nil {
			// Continue current definition
			currentDef.lines = append(currentDef.lines, line)

			// Find dependencies
			for _, word := range extractIdentifiers(line) {
				if word != currentDef.name {
					currentDef.deps[word] = true
				}
			}

			// Check if definition ends (blank line or next definition)
			if trimmed == "" || i == len(coreBody)-1 {
				definitions = append(definitions, *currentDef)
				currentDef = nil
			}
		}
	}

	// Add last definition if exists
	if currentDef != nil {
		definitions = append(definitions, *currentDef)
	}

	// Find all needed definitions recursively
	needed := make(map[string]bool)
	var addDeps func(name string)
	addDeps = func(name string) {
		if needed[name] {
			return
		}
		needed[name] = true

		// Find definition and add its dependencies
		for _, def := range definitions {
			if def.name == name {
				for dep := range def.deps {
					addDeps(dep)
				}
				break
			}
		}

		// Also add all methods for this type
		for _, def := range definitions {
			if def.receiverType == name {
				addDeps(def.name)
			}
		}
	}

	// Start with identifiers used in src
	for ident := range usedIdents {
		addDeps(ident)
	}

	// Build filtered coreBody
	var result []string
	for _, def := range definitions {
		if needed[def.name] {
			result = append(result, def.lines...)
		}
	}

	return result
}

// filterUnusedImports removes import statements that aren't used in the code
func filterUnusedImports(code string, imports []string) []string {
	var result []string

	for _, imp := range imports {
		// Extract package name from import line
		// Format: "\t\"package/path\"" or "\t\"package/path\" // comment"
		trimmed := strings.TrimSpace(imp)
		if trimmed == "" {
			continue
		}

		// Remove leading tab and quotes
		importPath := strings.Trim(trimmed, "\t \"")
		importPath = strings.Split(importPath, "//")[0] // Remove comments
		importPath = strings.TrimSpace(importPath)

		// Get package name (last part of path)
		pkgName := importPath
		if idx := strings.LastIndex(importPath, "/"); idx != -1 {
			pkgName = importPath[idx+1:]
		}

		// Check if package is used in code
		// Look for: pkgName.Something or direct uses for common packages
		if strings.Contains(code, pkgName+".") ||
			(pkgName == "fmt" && (strings.Contains(code, "fmt.") || strings.Contains(code, "Sprintf") || strings.Contains(code, "Printf"))) ||
			(pkgName == "strings" && strings.Contains(code, "strings.")) ||
			(pkgName == "bytes" && strings.Contains(code, "bytes.")) ||
			(pkgName == "slices" && strings.Contains(code, "slices.")) ||
			(pkgName == "maps" && strings.Contains(code, "maps.")) ||
			(pkgName == "sort" && strings.Contains(code, "sort.")) ||
			(pkgName == "strconv" && strings.Contains(code, "strconv.")) ||
			(pkgName == "math" && strings.Contains(code, "math.")) ||
			(pkgName == "iter" && strings.Contains(code, "iter.")) {
			result = append(result, imp)
		}
	}

	return result
}

func buildFile(fileName string, forceFlag, sourceMapFlag, standalone, mutEnforced, allowUnused, releaseFlag bool, m *agl.PkgVisited) error {
	if m.ContainsAdd(fileName) {
		return nil
	}
	fmt.Println("build", fileName)
	isGo := strings.HasSuffix(fileName, ".go")
	isAGL := strings.HasSuffix(fileName, ".agl")
	if !isAGL && !isGo {
		return nil
	}

	generatedFilePrefix := agl.GeneratedFilePrefix

	by, err := os.ReadFile(fileName)
	if err != nil {
		panic(err)
	}
	if bytes.Equal(by[:len(generatedFilePrefix)], []byte(generatedFilePrefix)) {
		return nil
	}

	if isGo {
		f, err := goparser.ParseFile(gotoken.NewFileSet(), fileName, by, 0)
		if err != nil {
			panic(err)
		}
		for _, i := range f.Imports {
			importPath := strings.ReplaceAll(i.Path.Value, `"`, ``)
			if strings.HasPrefix(importPath, modPrefix) {
				importPath = strings.TrimPrefix(importPath, modPrefix)
				entries, err := os.ReadDir(importPath)
				if err != nil {
					panic(err)
				}
				for _, entry := range entries {
					if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
						continue
					}
					if err := buildFile(filepath.Join(importPath, entry.Name()), forceFlag, sourceMapFlag, standalone, mutEnforced, allowUnused, releaseFlag, m); err != nil {
						panic(err)
					}
				}
			}
		}
		return nil
	}

	fset, f, f2 := agl.ParseSrc(string(by))
	for _, i := range f.Imports {
		importPath := strings.ReplaceAll(i.Path.Value, `"`, ``)
		if strings.HasPrefix(importPath, modPrefix) {
			importPath = strings.TrimPrefix(importPath, modPrefix)
			entries, err := os.ReadDir(importPath)
			if err != nil {
				panic(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				if err := buildFile(filepath.Join(importPath, entry.Name()), forceFlag, sourceMapFlag, standalone, mutEnforced, allowUnused, releaseFlag, m); err != nil {
					panic(err)
				}
			}
		}
	}
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(env)
	_, _ = i.InferFile(fileName, f2, fset, true)
	imports, errs := i.InferFile(fileName, f, fset, mutEnforced)
	if len(errs) > 0 {
		panic(errs[0])
	}
	opts := make([]agl.GeneratorOption, 0)
	if allowUnused {
		opts = append(opts, agl.AllowUnused())
	}
	if releaseFlag {
		opts = append(opts, agl.ReleaseMode())
	}
	g := agl.NewGenerator(i.Env, f, f2, imports, fset, opts...)
	src := g.Generate()
	path := strings.Replace(fileName, ".agl", ".go", 1)
	if file, err := os.Open(path); err == nil {
		defer file.Close()
		buf := make([]byte, len(generatedFilePrefix))
		if _, err := file.Read(buf); err != nil {
			panic(err)
		}
		if !bytes.Equal(buf, []byte(generatedFilePrefix)) && !forceFlag {
			panic(fmt.Sprintf("%s would be overwritten, use -f to force", path))
		}
	}

	// If standalone mode is enabled, embed the core library in the generated file
	if standalone {
		packageName := g.PkgName()
		coreContent := agl.GenCore(packageName)
		coreLines := strings.Split(coreContent, "\n")

		// Find where core imports and body start
		coreImportStart := -1
		coreImportEnd := -1
		for i, line := range coreLines {
			trimmed := strings.TrimSpace(line)
			if coreImportStart == -1 && strings.HasPrefix(trimmed, "import") {
				coreImportStart = i
			} else if coreImportStart != -1 && coreImportEnd == -1 && trimmed == ")" {
				coreImportEnd = i + 1
				break
			}
		}

		// Extract core imports and body
		var coreImports []string
		var coreBody []string
		if coreImportStart != -1 && coreImportEnd != -1 {
			coreImports = coreLines[coreImportStart+1 : coreImportEnd-1]
			coreBody = coreLines[coreImportEnd:]
		} else {
			for i := 3; i < len(coreLines); i++ {
				if strings.TrimSpace(coreLines[i]) != "" {
					coreBody = coreLines[i:]
					break
				}
			}
		}

		// Filter core library to only include used functions/types
		coreBody = filterUnusedCode(src, coreBody)

		// Filter imports to only include those used in src + coreBody
		allCode := src + "\n" + strings.Join(coreBody, "\n")
		coreImports = filterUnusedImports(allCode, coreImports)

		// Merge core library into generated code
		srcLines := strings.Split(src, "\n")

		// Find import block
		importLineIdx := -1
		importEndIdx := -1
		for i, line := range srcLines {
			trim := strings.TrimSpace(line)
			if importLineIdx == -1 && strings.HasPrefix(trim, "import") {
				importLineIdx = i
				// Check format: "import (" vs "import \"...\""
				if strings.Contains(trim, "(") {
					// Multi-line import - find closing )
					for j := i + 1; j < len(srcLines); j++ {
						if strings.TrimSpace(srcLines[j]) == ")" {
							importEndIdx = j
							break
						}
					}
				} else {
					// Single-line import like: import "pkg"
					// We'll convert to multi-line
					importPath := strings.TrimSpace(strings.TrimPrefix(trim, "import"))
					srcLines[i] = "import ("
					// Insert the import path and closing paren
					newLines := make([]string, 0, len(srcLines)+2)
					newLines = append(newLines, srcLines[:i+1]...)
					if importPath != "" {
						newLines = append(newLines, "\t"+importPath)
					}
					newLines = append(newLines, ")")
					newLines = append(newLines, srcLines[i+1:]...)
					srcLines = newLines
					importEndIdx = i + 2
					if importPath == "" {
						importEndIdx = i + 1
					}
				}
				break
			}
		}

		// Deduplicate imports - collect all imports (existing + core)
		allImportsMap := make(map[string]string) // trimmed -> original with formatting

		// Collect existing imports from srcLines
		if importLineIdx != -1 && importEndIdx != -1 {
			for i := importLineIdx + 1; i < importEndIdx; i++ {
				line := srcLines[i]
				trimmed := strings.TrimSpace(line)
				if trimmed != "" && trimmed != ")" {
					allImportsMap[trimmed] = line
				}
			}
		}

		// Add core imports (deduplicated)
		for _, imp := range coreImports {
			trimmed := strings.TrimSpace(imp)
			if trimmed != "" {
				// Keep existing formatting if already present, otherwise add new
				if _, exists := allImportsMap[trimmed]; !exists {
					allImportsMap[trimmed] = imp
				}
			}
		}

		// Sort imports for deterministic output
		var sortedKeys []string
		for key := range allImportsMap {
			sortedKeys = append(sortedKeys, key)
		}
		sort.Strings(sortedKeys)

		var allImports []string
		for _, key := range sortedKeys {
			allImports = append(allImports, allImportsMap[key])
		}

		// Replace existing imports with sorted, deduplicated imports
		var result []string
		if importLineIdx != -1 && importEndIdx != -1 {
			// Replace the import block with sorted imports
			result = append(result, srcLines[:importLineIdx+1]...)
			result = append(result, allImports...)
			result = append(result, srcLines[importEndIdx:]...)
		} else {
			// No imports - add after package line
			pkgIdx := -1
			for i, line := range srcLines {
				if strings.HasPrefix(strings.TrimSpace(line), "package ") {
					pkgIdx = i
					break
				}
			}
			if pkgIdx != -1 {
				result = append(result, srcLines[:pkgIdx+1]...)
				result = append(result, "import (")
				result = append(result, allImports...)
				result = append(result, ")")
				result = append(result, srcLines[pkgIdx+1:]...)
			} else {
				result = srcLines
			}
		}

		// Append core body
		result = append(result, coreBody...)
		src = strings.Join(result, "\n")
	}

	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		return err
	}

	if sourceMapFlag {
		if sourceMap, err := g.GenerateStandardSourceMap(fileName); err == nil {
			path := strings.Replace(fileName, ".agl", ".go.map", 1)
			_ = os.WriteFile(path, []byte(sourceMap), 0644)
		}
	}

	// Only write separate aglCore.go if not in standalone mode
	if !standalone {
		packageName := g.PkgName()
		coreFile := filepath.Join(filepath.Dir(path), "aglCore.go")
		err = os.WriteFile(coreFile, []byte(agl.GenCore(packageName)), 0644)
		if err != nil {
			return err
		}
	}
	return nil
}

func buildProject(mutEnforced, releaseFlag bool) error {
	visited := make(map[string]struct{})
	if err := buildFolder(".", mutEnforced, releaseFlag, visited); err != nil {
		return err
	}
	cmd := exec.Command("go", "build")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

const modPrefix = "agl/" // TODO get from go.mod

func buildFolder(folderPath string, mutEnforced, releaseFlag bool, visited map[string]struct{}) error {
	if _, ok := visited[folderPath]; ok {
		return nil
	}
	coreFile := filepath.Join(folderPath, "aglCore.go")
	base := filepath.Base(folderPath)
	if base == "." {
		base = "main"
	}
	_ = os.WriteFile(coreFile, []byte(agl.GenCore(base)), 0644)
	visited[folderPath] = struct{}{}
	entries, _ := os.ReadDir(folderPath)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fileName := filepath.Join(folderPath, entry.Name())
		if strings.HasSuffix(fileName, ".agl") {
			if err := buildAglFile(fileName, mutEnforced, releaseFlag); err != nil {
				panic(fmt.Sprintf("failed to build %s", fileName))
			}
		} else if strings.HasSuffix(fileName, ".go") {
			if strings.HasSuffix(fileName, "_test.go") {
				continue
			}
			src, err := os.ReadFile(fileName)
			if err != nil {
				return err
			}
			fset := gotoken.NewFileSet()
			node, err := goparser.ParseFile(fset, fileName, src, goparser.AllErrors)
			if err != nil {
				log.Printf("failed to parse %s\n", fileName)
				continue
			}
			conf := gotypes.Config{Importer: nil}
			info := &gotypes.Info{Defs: make(map[*goast.Ident]gotypes.Object)}
			_, _ = conf.Check("", fset, []*goast.File{node}, info)
			for _, decl := range node.Decls {
				switch d := decl.(type) {
				case *goast.GenDecl:
					for _, spec := range d.Specs {
						switch s := spec.(type) {
						case *goast.ImportSpec:
							pathValue := strings.ReplaceAll(s.Path.Value, `"`, ``)
							if strings.HasPrefix(pathValue, modPrefix) {
								newPath := strings.TrimPrefix(pathValue, modPrefix)
								if err := buildFolder(newPath, mutEnforced, releaseFlag, visited); err != nil {
									return err
								}
							}
						}
					}
				}
			}
		}
	}
	return nil
}

func genCode1(fileName string, src, coreGoImports []byte, mutEnforced, allowUnused, releaseFlag bool) (*agl.Generator, *token.FileSet, string) {
	fset, f, f2 := agl.ParseSrc(string(src))
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(env)
	i.InferFile("core.agl", f2, fset, true)
	imports, errs := i.InferFile(fileName, f, fset, mutEnforced)
	if len(errs) > 0 {
		panic(errs[0])
	}
	coreGo, _ := parser.ParseFile(fset, "", coreGoImports, parser.AllErrors|parser.ParseComments)
	if coreGo != nil {
		for _, i := range coreGo.Imports {
			key := i.Path.Value
			if i.Name != nil {
				key = i.Name.Name + "_" + key
			}
			imports[key] = i
		}
	}
	opts := make([]agl.GeneratorOption, 0)
	if allowUnused {
		opts = append(opts, agl.AllowUnused())
	}
	if releaseFlag {
		opts = append(opts, agl.ReleaseMode())
	}
	g := agl.NewGenerator(i.Env, f, f2, imports, fset, opts...)
	return g, fset, g.Generate()
}

func genCode(fileName string, src []byte, mutEnforced bool) string {
	_, _, out := genCode1(fileName, src, nil, mutEnforced, false, false)
	return out
}

func buildAglFile(fileName string, mutEnforced, releaseFlag bool) error {
	by, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	src := genCode(fileName, by, mutEnforced)
	path := strings.Replace(fileName, ".agl", "_agl.go", 1)
	return os.WriteFile(path, []byte(src), 0644)
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
	src := genCode(fileName, by, true)
	fmt.Println(src)
	return nil
}

func insertHeadersAfterFirstLine(src, headers string) string {
	lines := strings.Split(src, "\n")
	if len(lines) == 0 {
		return src
	}

	// Find the package declaration line
	packageLineIndex := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			packageLineIndex = i
			break
		}
	}

	if packageLineIndex == -1 {
		// If no package declaration found, just prepend headers
		return headers + "\n" + src
	}

	// Insert headers after the package declaration
	var result []string
	result = append(result, lines[:packageLineIndex+1]...)
	result = append(result, headers)
	result = append(result, lines[packageLineIndex+1:]...)

	return strings.Join(result, "\n")
}

func modInitAction(_ context.Context, c *cli.Command) error {
	cmd := exec.Command("go", "mod", "init", c.Args().First())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func modTidyAction(_ context.Context, c *cli.Command) error {
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func modVendorAction(_ context.Context, c *cli.Command) error {
	entries, _ := os.ReadDir(".")
	var imports []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".agl") {
			by, _ := os.ReadFile(entry.Name())
			_, f, _ := agl.ParseSrc(string(by))
			for _, i := range f.Imports {
				imports = append(imports, fmt.Sprintf("import _ %s", i.Path.Value))
			}
		}
	}
	importsStr := strings.Join(imports, "\n")
	const tmpGoFile = "aglTmp.go"
	_ = os.WriteFile(tmpGoFile, []byte("package main\n"+importsStr), 0644)
	defer os.Remove(tmpGoFile)
	cmd := exec.Command("go", "mod", "vendor")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func versionAction(ctx context.Context, cmd *cli.Command) error {
	fmt.Printf("AGL v%s (%s)\n", version, commit)
	return nil
}
