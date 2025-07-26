package e2eTests

import (
	"agl/pkg/agl"
	"agl/pkg/token"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func spawnGoRunFromBytes(g *agl.Generator, fset *token.FileSet, source []byte, programArgs []string) ([]byte, error) {
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
	buf := &bytes.Buffer{}
	cmd.Stderr = buf
	by, err := cmd.Output()
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
		by = []byte(stdErrStr)
	}
	return by, err
}

func testGenOutput(src string) string {
	fset, f, f2 := agl.ParseSrc(src)
	env := agl.NewEnv(fset)
	i := agl.NewInferrer(env)
	i.InferFile("", f2, fset, true)
	imports, _ := i.InferFile("", f, fset, true)
	g := agl.NewGenerator(env, f, f2, imports, fset)
	outSrc := g.Generate()
	out, err := spawnGoRunFromBytes(g, fset, []byte(outSrc), nil)
	if err != nil {
		panic(string(out))
	}
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

//func Test14(t *testing.T) {
//	t.Parallel()
//	src := `package main
//import "fmt"
//import "iter"
//type MyType struct {
//	a, b, c int
//}
//func (m MyType) Iter() iter.Seq[int] {
//	return func(yield func(int) bool) {
//		vals := []int{m.a, m.b, m.c}
//		for _, el := range vals {
//			if !yield(el) {
//				return
//			}
//		}
//	}
//}
//func main() {
//	s := set[int]{1, 2, 3}
//	t := MyType{a: 3, b: 4, c: 5}
//	u := s.Union(t)
//	var mut out []int
//	for el := range u {
//		out.Push(el)
//	}
//	fmt.Println(out.Sorted())
//}`
//	tassert.Equal(t, "[1 2 3 4 5]\n", testGenOutput(src))
//}

func Test15(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	for el in []int{1, 2, 3} {
		fmt.Print(el)
	}
	var mut tmp []int
	for el in set[int]{1, 2, 3} {
		tmp.Push(el)
	}
	fmt.Print(tmp.Sorted())
    for (a, b, c) in [](int, string, bool){(1, "foo", true), (2, "bar", false)} {
        fmt.Print(a, b, c)
    }
}`
	tassert.Equal(t, "123[1 2 3]1footrue2barfalse", testGenOutput(src))
}

func Test16(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	var a any = int(42)
	b := a.(int)?
	panic("")
	fmt.Println(b)
}`
	expected := "/main.go:11 (main.agl:6)"
	PanicsContains(t, expected, func() { testGenOutput(src) })
}

func PanicsContains(t *testing.T, errString string, f func()) bool {
	defer func() {
		if r := recover(); r != nil {
			s := r.(string)
			if strings.Contains(s, errString) {
				return
			}
			t.Errorf("panic: %s", r)
		}
	}()
	f()
	return true
}

func Test17(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	var mut t (uint, uint)?
	assert(t.IsNone())
}`
	tassert.Equal(t, "", testGenOutput(src))
}

func Test18(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
    mut a := []int{1, 2, 3}
	a.Swap(1, 2)
	assert(a[0] == 1)
	assert(a[1] == 3)
	assert(a[2] == 2)
    mut b := []int{1, 2, 3}
	b.Swap(u8(1), u16(2))
	assert(b[0] == 1)
	assert(b[1] == 3)
	assert(b[2] == 2)
}`
	tassert.Equal(t, "", testGenOutput(src))
}

func Test19(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    for e in (0..3) {
		fmt.Print(e)
	}
    for e in (0..=3) {
		fmt.Print(e)
	}
    for e in (0..3).Rev() {
		fmt.Print(e)
	}
    for e in (0..=3).Rev() {
		fmt.Print(e)
	}
}`
	tassert.Equal(t, "01201232103210", testGenOutput(src))
}

func Test20(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    m := map[int]u8{1: 200, 2: 55, 3: 10}
	fmt.Println(m.Values().Sum())
	fmt.Println(m.Values().Sum[int]())
}`
	tassert.Equal(t, "9\n265\n", testGenOutput(src))
}

func Test21(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    s := set[int]{1, 2, 3, 4}
	fmt.Println(Array(s.Filter({ $0 % 2 == 0 })).Sorted())
}`
	tassert.Equal(t, "[2 4]\n", testGenOutput(src))
}

func Test22(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    s := set[int]{1, 2, 3, 4}
	fmt.Println(s.Filter({ $0 % 2 == 0 }).Map({ $0 + 1 }).Sorted())
}`
	tassert.Equal(t, "[3 5]\n", testGenOutput(src))
}

func Test23(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    a := []int{1, 2, 3, 4}
	fmt.Println(a.Sorted())
	fmt.Println(a.Sorted(by: func(a, b int) bool { return a > b }))
	fmt.Println(a.Sorted(by: { $0 > $1 }))
}`
	tassert.Equal(t, "[1 2 3 4]\n[4 3 2 1]\n[4 3 2 1]\n", testGenOutput(src))
}

func Test24(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
    m := map[int]u8{1: 1, 2: 2, 3: 3}
	vals := m.Values()
	for _ in vals {
		fmt.Print(0)
	}
}`
	tassert.Equal(t, "000", testGenOutput(src))
}

func Test25(t *testing.T) {
	t.Parallel()
	src := `package main
func main() {
	s1 := set[int]{1, 2, 3}
	s2 := set[int]{3, 4, 5}
	_ = s1.Union(s2)
	a1 := []int{3, 4, 5}
	_ = s1.Union(a1)
	m1 := map[int]int{1: 1, 2: 2, 3: 3}
	_ = s1.Union(m1.Keys())
}`
	tassert.Equal(t, "", testGenOutput(src))
}

func Test26(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	s1 := set[int]{1, 2, 3}
	a1 := Array(s1).Sorted()
	m1 := map[int]int{1: 1, 2: 2, 3: 3}
	a2 := Array(m1.Keys()).Sorted()
	fmt.Println(a1)
	fmt.Println(a2)
}`
	tassert.Equal(t, "[1 2 3]\n[1 2 3]\n", testGenOutput(src))
}

func Test27(t *testing.T) {
	t.Parallel()
	src := `package main
import "fmt"
func main() {
	m := map[int]int{1: 1, 2: 2, 3: 3, 4: 4}
	a := m.Filter({ $0.Key % 2 == 0 }).Keys().Sorted()
	fmt.Println(a)
}`
	tassert.Equal(t, "[2 4]\n", testGenOutput(src))
}
