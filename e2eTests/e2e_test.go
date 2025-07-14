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
	fset, f, f2 := agl.ParseSrc(src)
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(env)
	i.InferFile("", f2, fset, true)
	i.InferFile("", f, fset, true)
	outSrc := agl.NewGenerator(env, f, f2, fset).Generate()
	out := agl.Must(spawnGoRunFromBytes([]byte(outSrc), nil))
	return string(out)
}

func Test1(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	a := 1
	fmt.Println(a)
}`
	tassert.Equal(t, "1\n", testGenOutput(src))
}

func Test2(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	src := `package main
func main() {
	a := []byte("test")
	b := []byte("test")
	assert(a == b)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test4(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	a := []byte("hello")
	b := []byte("world")
	assert(a != b)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test5(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	m := map[string]int{"a": 42}
	assert(m.ContainsKey("a"))
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test6(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	assert(m.Len() == 3)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

//func Test7(t *testing.T) {
//	src := `package main
//type HasLen interface {
//	Len() int
//}
//func main() {
//	a := []int{1, 2, 3, 4}
//	m := map[string]int{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}
//	s := set[int]{1, 2, 3, 4, 5, 6}
//	res := []HasLen{a, m, s}
//	assert(res.Len() == 3)
//	assert(res[0].Len() == 4)
//	assert(res[1].Len() == 5)
//	assert(res[2].Len() == 6)
//}`
//	tassert.NotPanics(t, func() { testGenOutput(src) })
//}

func Test8(t *testing.T) {
	t.Parallel()
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

func Test9(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	a := []u8{3, 1, 2}
	b := a.Sorted()
	assert(b[0] == 1)
	assert(b[1] == 2)
	assert(b[2] == 3)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test10(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	a := []u8{1, 2, 3}
	assert(a.First().Unwrap() == 1)
	assert(a.First({ $0 == 3 }).Unwrap() == 3)
	assert(a.First({ $0 == 4 }).IsNone())
	assert(a.Min().Unwrap() == 1)
	assert(a.Max().Unwrap() == 3)
}`
	tassert.NotPanics(t, func() { testGenOutput(src) })
}

func Test11(t *testing.T) {
	t.Parallel()
	src := `package main
import "regexp"
import "fmt"
func main() {
	data := "mul(42,43)"
	rgxMul := regexp.MustCompile("mul\\((\\d+),(\\d+)\\)")
	matches := rgxMul.FindAllStringSubmatch(data, -1)
	fmt.Println(matches.Map({ $0[1].Int()? }).Sum())
}`
	tassert.Equal(t, "42\n", testGenOutput(src))
}

func Test12(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	a := []int{1, 2, 3}
	b := []int{4, 5, 6}
	c := a + b
	fmt.Println(c)
}`
	tassert.Equal(t, "[1 2 3 4 5 6]\n", testGenOutput(src))
}

func Test13(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	a := set[int]{1, 2, 3}
	b := a.Union([]int{3, 4, 5})
	var mut out []int
	for el := range b {
		out.Push(el)
	}
	fmt.Println(out.Sorted())
}`
	tassert.Equal(t, "[1 2 3 4 5]\n", testGenOutput(src))
}
