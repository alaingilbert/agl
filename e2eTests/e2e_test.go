package e2eTests

import (
	"agl/pkg/agl"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func spawnGoRunFromBytes(source []byte, programArgs []string) ([]byte, error) {
	// Create a temporary directory
	tmpDir, err := os.MkdirTemp("", "gorun")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir) // clean up

	// Create a .go file inside it
	tmpFile := filepath.Join(tmpDir, "main.go")
	err = os.WriteFile(tmpFile, source, 0644)
	if err != nil {
		return nil, err
	}

	coreFile := filepath.Join(tmpDir, "aglCore.go")
	err = os.WriteFile(coreFile, []byte(agl.GenCore("main")), 0644)
	if err != nil {
		return nil, err
	}

	// Run `go run` on the file with additional arguments
	cmdArgs := append([]string{"run", tmpFile, coreFile}, programArgs...)
	cmd := exec.Command("go", cmdArgs...)
	return cmd.Output()
}

func testGenOutput(src string) string {
	fset, f := agl.ParseSrc(src)
	env := agl.NewEnv()
	agl.NewInferrer(env).InferFile("", f, fset, true)
	outSrc := agl.NewGenerator(env, f, fset).Generate()
	out := agl.Must(spawnGoRunFromBytes([]byte(outSrc), nil))
	return string(out)
}

func Test1(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	a := 1
	fmt.Println(a)
}`
	tassert.Equal(t, "1\n", testGenOutput(src))
}

func Test2(t *testing.T) {
	src := `package main
import "fmt"
func test() int? { Some(42) }
func main() {
	guard Some(a) := test() else { return }
	fmt.Println(a)
}`
	tassert.Equal(t, "42\n", testGenOutput(src))
}

func Test3(t *testing.T) {
	src := `package main
func main() {
	a := []byte("test")
	b := []byte("test")
	assert(a == b)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test4(t *testing.T) {
	src := `package main
func main() {
	a := []byte("hello")
	b := []byte("world")
	assert(a != b)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test5(t *testing.T) {
	src := `package main
func main() {
	m := map[string]int{"a": 42}
	assert(m.ContainsKey("a"))
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test6(t *testing.T) {
	src := `package main
func main() {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	assert(m.Len() == 3)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test7(t *testing.T) {
	src := `package main
type HasLen interface {
	Len() int
}
func main() {
	a := []int{1, 2, 3, 4}
	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
	s := set[int]{1, 2, 3, 4, 5, 6}
	res := []HasLen{a, m, s}
	assert(res.Len() == 3)
	assert(res[0].Len() == 4)
	assert(res[1].Len() == 5)
	assert(res[2].Len() == 6)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test8(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3}
	assert(a.Get(-1).IsNone())
	assert(a.Get(0).Unwrap() == 1)
	assert(a.Get(2).Unwrap() == 3)
	assert(a.Get(3).IsNone())
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}
