package agl

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestCodeGenGuard_1(t *testing.T) {
	src := `package main
import "fmt"
func test() int? { Some(42) }
func main() {
	a := 42
	guard a < 100 else { return }
	guard Some(b) := test() else { return }
	fmt.Println(b)
}`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	a := 42
	if !(a < 100) {
		return
	}
	aglTmp1 := test()
	if aglTmp1.IsNone() {
		return
	}
	b := aglTmp1.Unwrap()
	fmt.Println(b)
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGenGuard_2(t *testing.T) {
	src := `package main
import "fmt"
func test() int? { Some(42) }
func main() {
	a := 42
	guard a < 100 else {
		fmt.Println("something")
	}
}`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	a := 42
	if !(a < 100) {
		fmt.Println("something")
	}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Contains(t, test.errs[0].Error(), "guard must return/break/continue")
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGenGuard_3(t *testing.T) {
	src := `package main
import "fmt"
func test() int? { Some(42) }
func main() {
	a := 42
	for i := 0; i < 10; i++ {
		guard a < 2 else { return }
		guard a < 10 else { break }
		guard a < 100 else { continue }
		fmt.Println("something")
	}
}`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	a := 42
	for i := 0; i < 10; i++ {
		if !(a < 2) {
			return
		}
		if !(a < 10) {
			break
		}
		if !(a < 100) {
			continue
		}
		fmt.Println("something")
	}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}
