package agl

import (
	"fmt"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func getGenOutput(src string) string {
	fset, f := ParseSrc(src)
	env := NewEnv()
	NewInferrer(env).InferFile("", f, fset, true)
	g := NewGenerator(env, f, fset)
	return g.Generate()
}

func testCodeGen(t *testing.T, src, expected string) {
	got := getGenOutput(src)
	testCodeGen1(t, got, expected)
}

func testCodeGen1(t *testing.T, got, expected string) {
	if got != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func testCodeGenFn(src string) func() {
	return func() {
		_ = getGenOutput(src)
	}
}

func TestCodeGen1(t *testing.T) {
	src := `
package main
func add(a, b int) int {
	return a + b
}`
	expected := `// agl:generated
package main
func add(a, b int) int {
	return a + b
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen1_1(t *testing.T) {
	src := `
package main
func add1(a, b int) int {
	return a + b
}
func add2(a, b int) int {
	return a + b
}`
	expected := `// agl:generated
package main
func add1(a, b int) int {
	return a + b
}
func add2(a, b int) int {
	return a + b
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen2(t *testing.T) {
	src := `package main
func add(a, b i64) i64? {
	if a == 0 {
		return None
	}
	return Some(a + b)
}`
	expected := `// agl:generated
package main
func add(a, b int64) Option[int64] {
	if a == 0 {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(a + b)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen2_1(t *testing.T) {
//	src := `package main
//func add(a, b i64) Option[i64] {
//	if a == 0 {
//		return None
//	}
//	return Some(a + b)
//}`
//	expected := `// agl:generated
//package main
//func add(a, b int64) Option[int64] {
//	if a == 0 {
//		return MakeOptionNone[int64]()
//	}
//	return MakeOptionSome(a + b)
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen6(t *testing.T) {
	src := `package main
import "agl1/errors"
func add(a, b i64) i64! {
	if a == 0 {
		return Err(errors.New("a cannot be zero"))
	}
	return Ok(a + b)
}`
	expected := `// agl:generated
package main
import "errors"
func add(a, b int64) Result[int64] {
	if a == 0 {
		return MakeResultErr[int64](errors.New("a cannot be zero"))
	}
	return MakeResultOk(a + b)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen3(t *testing.T) {
//	src := `
//import "fmt"
//fn add(a, b int) int? {
//	if a == 0 {
//		return None
//	}
//	return Some(a + b)
//}
//fn main() {
//	opt := add(1, 2)
//	// match opt {
//	// 	Some(val) => println(val),
//	// 	None      => println("none"),
//	// }
//	// if let Some(val) = opt {
//	// 	println(val)
//	// }
//	fmt.Println(opt.unwrap())
//}`
//	expected := `
//import "fmt"
//type addOpt struct {
//	a int
//	b int
//}
//func add(a, b int) (addOpt, bool) {
//	if a == 0 {
//		return *new(addOpt), false
//	}
//	return addOpt{a: a, b: b}, true
//}
//func main() {
//	addOpt := add(1, 2)
//	if addOpt.IsSome() {
//		println(addOpt.Unwrap())
//	} else {
//		println("none")
//	}
//	if res.IsSome() {
//		println(addOpt.Unwrap())
//	}
//	println(addOpt.Unwrap())
//}`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen8(t *testing.T) {
	src := `package main
func mapFn[T any](a []T, f func(T) T) []T {
	return make([]T, 0)
}
func main() {
	a := []int{1, 2, 3}
	mapFn(a, { $0 })
}
`
	expected := `// agl:generated
package main
func mapFn_T_int(a AglVec[int], f func(int) int) AglVec[int] {
	return make(AglVec[int], 0)
}
func main() {
	a := AglVec[int]{1, 2, 3}
	mapFn_T_int(a, func(aglArg0 int) int {
		return aglArg0
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen9_optionalReturnKeyword(t *testing.T) {
	src := `package main
func add(a, b int) int { a + b }
func add1(a, b int) int { return a + b }`
	expected := `// agl:generated
package main
func add(a, b int) int {
	return a + b
}
func add1(a, b int) int {
	return a + b
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen10(t *testing.T) {
	src := `package main
func f1(f func() int) int { f() }
func f2() int { 42 }
func main() {
	f1(f2)
}`
	expected := `// agl:generated
package main
func f1(f func() int) int {
	return f()
}
func f2() int {
	return 42
}
func main() {
	f1(f2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen11(t *testing.T) {
	src := `package main
func f1(f func() i64) i64 { f() }
func main() {
	f1({ 42 })
}`
	expected := `// agl:generated
package main
func f1(f func() int64) int64 {
	return f()
}
func main() {
	f1(func() int64 {
		return 42
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen13(t *testing.T) {
	src := `package main
func f1(f func(i64) i64) i64 { f(1) }
func main() {
	f1({ $0 + 1 })
}`
	expected := `// agl:generated
package main
func f1(f func(int64) int64) int64 {
	return f(1)
}
func main() {
	f1(func(aglArg0 int64) int64 {
		return aglArg0 + 1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen14(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := make([]int, 0)
	fmt.Println(a)
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := make(AglVec[int], 0)
	fmt.Println(a)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen15(t *testing.T) {
//	src := `package main
//func main() {
//	mut a := 42
//	a = 43
//	fmt.Println(a)
//}`
//	expected := `// agl:generated
//package main
//func main() {
//	a := 42
//	a = 43
//	fmt.Println(a)
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen16(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	for _, c := range "test" {
		fmt.Println(c)
	}
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	for _, c := range "test" {
		fmt.Println(c)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen17(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	if 2 % 2 == 0 {
		fmt.Println("test")
	}
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	if 2 % 2 == 0 {
		fmt.Println("test")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen18(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3}
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen19(t *testing.T) {
	src := `package main
func findEvenNumber(arr []int) int? {
  for _, num := range arr {
      if num % 2 == 0 {
          return Some(num)
      }
  }
  return None
}`
	expected := `// agl:generated
package main
func findEvenNumber(arr AglVec[int]) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen20(t *testing.T) {
	src := `package main
import "agl1/fmt"
func findEvenNumber(arr []int) int? {
  for _, num := range arr {
      if num % 2 == 0 {
          return Some(num)
      }
  }
  return None
}
func main() {
	tmp := findEvenNumber([]int{1, 2, 3, 4})
	fmt.Println(tmp.Unwrap())
}`
	expected := `// agl:generated
package main
import "fmt"
func findEvenNumber(arr AglVec[int]) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	tmp := findEvenNumber(AglVec[int]{1, 2, 3, 4})
	fmt.Println(tmp.Unwrap())
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen21(t *testing.T) {
	src := `package main
import "agl1/fmt"
func findEvenNumber(arr []int) int? {
  for _, num := range arr {
      if num % 2 == 0 {
          return Some(num)
      }
  }
  return None
}
func main() {
	fmt.Println(findEvenNumber([]int{1, 2, 3, 4}).Unwrap())
}`
	expected := `// agl:generated
package main
import "fmt"
func findEvenNumber(arr AglVec[int]) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	fmt.Println(findEvenNumber(AglVec[int]{1, 2, 3, 4}).Unwrap())
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen22(t *testing.T) {
	src := `package main
func main() {
	a := 1
	a++
}`
	expected := `// agl:generated
package main
func main() {
	a := 1
	a++
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen23(t *testing.T) {
	src := `package main
func findEvenNumber(arr []int) int? {
  for _, num := range arr {
      if num % 2 == 0 {
          return Some(num)
      }
  }
  return None
}
func main() {
	foundInt := findEvenNumber([]int{1, 2, 3, 4})?
}`
	expected := `// agl:generated
package main
func findEvenNumber(arr AglVec[int]) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	foundInt := findEvenNumber(AglVec[int]{1, 2, 3, 4}).Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen24(t *testing.T) {
	src := `package main
func parseInt(s string) int! {
	return Ok(42)
}
func main() {
	parseInt("42")!
}`
	expected := `// agl:generated
package main
func parseInt(s string) Result[int] {
	return MakeResultOk(42)
}
func main() {
	parseInt("42").Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen25(t *testing.T) {
	src := `package main
import "agl1/errors"
func parseInt(s1 string) int! {
	return Err(Errors.New("some error"))
}
func inter(s2 string) int! {
	a := parseInt(s2)!
	return Ok(a + 1)
}
func main() {
	inter("hello")!
}`
	expected := `// agl:generated
package main
import "errors"
func parseInt(s1 string) Result[int] {
	return MakeResultErr[int](Errors.New("some error"))
}
func inter(s2 string) Result[int] {
	aglTmpVar1 := parseInt(s2)
	if aglTmpVar1.IsErr() {
		return aglTmpVar1
	}
	a := aglTmpVar1.Unwrap()
	return MakeResultOk(a + 1)
}
func main() {
	inter("hello").Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen26(t *testing.T) {
	src := `package main
import "agl1/fmt"
func add(a, b int) int {
	return a + b
}
func main() {
	fmt.Println(add(1, 2))
}`
	expected := `// agl:generated
package main
import "fmt"
func add(a, b int) int {
	return a + b
}
func main() {
	fmt.Println(add(1, 2))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen27(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen28(t *testing.T) {
	src := `package main
import "agl1/strconv"
func parseInt(s string) int! {
	num := strconv.Atoi(s)!
	return Ok(num)
}`
	expected := `// agl:generated
package main
import "strconv"
func parseInt(s string) Result[int] {
	aglTmpVar1, aglTmpErr1 := strconv.Atoi(s)
	if aglTmpErr1 != nil {
		return MakeResultErr[int](aglTmpErr1)
	}
	num := AglIdentity(aglTmpVar1)
	return MakeResultOk(num)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen29(t *testing.T) {
//	src := `
//type Person struct {
//	name string
//	age int
//}
//pub type Animal struct {
//	name string
//	age int
//}
//`
//	expected := `type aglPrivPerson struct {
//	name string
//	age int
//}
//type Animal struct {
//	name string
//	age int
//}`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen30(t *testing.T) {
	src := `package main
func main() {
	mut a := 0
	a += 1
	a -= 1
	a *= 1
	a /= 1
	a %= 1
	a &= 1
	a |= 1
	a ^= 1
	a <<= 1
	a >>= 1
	a &^= 1
	a++
	a--
}
`
	expected := `// agl:generated
package main
func main() {
	a := 0
	a += 1
	a -= 1
	a *= 1
	a /= 1
	a %= 1
	a &= 1
	a |= 1
	a ^= 1
	a <<= 1
	a >>= 1
	a &^= 1
	a++
	a--
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen31_VecBuiltInFilter(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1, 2, 3, 4}
	b := a.Filter({ $0 % 2 == 0 })
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1, 2, 3, 4}
	b := AglVecFilter(a, func(aglArg0 int64) bool {
		return aglArg0 % 2 == 0
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInFilter2(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1, 2, 3, 4}
	b := a.Filter({ $0 % 2 == 0 })
	c := b.Map({ $0 + 1 })
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1, 2, 3, 4}
	b := AglVecFilter(a, func(aglArg0 int64) bool {
		return aglArg0 % 2 == 0
	})
	c := AglVecMap(b, func(aglArg0 int64) int64 {
		return aglArg0 + 1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInFilter3(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1}
	b := a.Filter({ $0 == 1 }).Map({ $0 }).Reduce(0, { $0 + $1 })
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1}
	b := AglVecReduce(AglVecMap(AglVecFilter(a, func(aglArg0 int64) bool {
		return aglArg0 == 1
	}), func(aglArg0 int64) int64 {
		return aglArg0
	}), 0, func(aglArg0 int64, aglArg1 int64) int64 {
		return aglArg0 + aglArg1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInFilter4(t *testing.T) {
	src := `package main
func main() {
	a1 := []i64{1}
	a2 := []u8{1}
	b := a1.Filter({ $0 == 1 }).Map({ $0 }).Reduce(0, { $0 + $1 })
	c := a2.Filter({ $0 == 1 }).Map({ $0 }).Reduce(0, { $0 + $1 })
}
`
	expected := `// agl:generated
package main
func main() {
	a1 := AglVec[int64]{1}
	a2 := AglVec[uint8]{1}
	b := AglVecReduce(AglVecMap(AglVecFilter(a1, func(aglArg0 int64) bool {
		return aglArg0 == 1
	}), func(aglArg0 int64) int64 {
		return aglArg0
	}), 0, func(aglArg0 int64, aglArg1 int64) int64 {
		return aglArg0 + aglArg1
	})
	c := AglVecReduce(AglVecMap(AglVecFilter(a2, func(aglArg0 uint8) bool {
		return aglArg0 == 1
	}), func(aglArg0 uint8) uint8 {
		return aglArg0
	}), 0, func(aglArg0 uint8, aglArg1 uint8) uint8 {
		return aglArg0 + aglArg1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap1(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1, 2, 3, 4}
	b := a.Map({ $0 + 1 })
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1, 2, 3, 4}
	b := AglVecMap(a, func(aglArg0 int64) int64 {
		return aglArg0 + 1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap3(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1, 2, 3, 4}
	b := a.Map({ "a" })
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1, 2, 3, 4}
	b := AglVecMap(a, func(aglArg0 int64) string {
		return "a"
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap2(t *testing.T) {
	src := `package main
import "agl1/strconv"
func main() {
	a := []int{1, 2, 3, 4}
	b := a.Map(strconv.Itoa)
}
`
	expected := `// agl:generated
package main
import "strconv"
func main() {
	a := AglVec[int]{1, 2, 3, 4}
	b := AglVecMap(a, strconv.Itoa)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen33_VecBuiltInReduce(t *testing.T) {
	src := `package main
func main() {
	a := []i64{1, 2, 3, 4}
	b := a.Reduce(0, { $0 + $1 })
	assert(b == 10, "b should be 10")
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int64]{1, 2, 3, 4}
	b := AglVecReduce(a, 0, func(aglArg0 int64, aglArg1 int64) int64 {
		return aglArg0 + aglArg1
	})
	AglAssert(b == 10, "assert failed line 5" + " " + "b should be 10")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen34_Assert(t *testing.T) {
	src := `package main
func main() {
	assert(1 != 2)
	assert(1 != 2, "1 should not be 2")
}
`
	expected := `// agl:generated
package main
func main() {
	AglAssert(1 != 2, "assert failed line 3")
	AglAssert(1 != 2, "assert failed line 4" + " " + "1 should not be 2")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Reduce1(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3, 4}
	b := a.Filter({ $0 % 2 == 0 })
	c := b.Map({ $0 + 1 })
	d := c.Reduce(0, { $0 + $1 })
	assert(d == 8)
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3, 4}
	b := AglVecFilter(a, func(aglArg0 int) bool {
		return aglArg0 % 2 == 0
	})
	c := AglVecMap(b, func(aglArg0 int) int {
		return aglArg0 + 1
	})
	d := AglVecReduce(c, 0, func(aglArg0 int, aglArg1 int) int {
		return aglArg0 + aglArg1
	})
	AglAssert(d == 8, "assert failed line 7")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen35(t *testing.T) {
	src := `package main
import "agl1/os"
import "agl1/fmt"
func main() {
	by := os.ReadFile("test.txt")!
	fmt.Println(by)
}
`
	expected := `// agl:generated
package main
import (
	"os"
	"fmt"
)
func main() {
	aglTmp1, aglTmpErr1 := os.ReadFile("test.txt")
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	by := AglIdentity(aglTmp1)
	fmt.Println(by)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen36(t *testing.T) {
	src := `package main
func testOption() int? {
	return None
}
func main() {
	res := testOption()
	assert(res.IsNone())
	assert(testOption().IsNone())
}
`
	expected := `// agl:generated
package main
func testOption() Option[int] {
	return MakeOptionNone[int]()
}
func main() {
	res := testOption()
	AglAssert(res.IsNone(), "assert failed line 7")
	AglAssert(testOption().IsNone(), "assert failed line 8")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Tuple1(t *testing.T) {
	src := `package main
func main() {
	res := (1, "hello", true)
	assert(res.0 == 1)
	assert(res.1 == "hello")
	assert(res.2 == true)
}
`
	expected := `// agl:generated
package main
type AglTupleStruct_int_string_bool struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	res := AglTupleStruct_int_string_bool{Arg0: 1, Arg1: "hello", Arg2: true}
	AglAssert(res.Arg0 == 1, "assert failed line 4")
	AglAssert(res.Arg1 == "hello", "assert failed line 5")
	AglAssert(res.Arg2 == true, "assert failed line 6")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_TupleDestructuring1(t *testing.T) {
	src := `package main
func main() {
	a, b, c := (1, "hello", true)
	assert(a == 1)
	assert(b == "hello")
	assert(c == true)
}
`
	expected := `// agl:generated
package main
type AglTupleStruct_int_string_bool struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	aglVar1 := AglTupleStruct_int_string_bool{Arg0: 1, Arg1: "hello", Arg2: true}
	a, b, c := aglVar1.Arg0, aglVar1.Arg1, aglVar1.Arg2
	AglAssert(a == 1, "assert failed line 4")
	AglAssert(b == "hello", "assert failed line 5")
	AglAssert(c == true, "assert failed line 6")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Tuple2(t *testing.T) {
	src := `package main
func testTuple() (u8, string, bool) {
	return (1, "hello", true)
}
func main() {
	res := testTuple()
	assert(res.0 == 1)
	assert(res.1 == "hello")
	assert(res.2 == true)
}
`
	expected := `// agl:generated
package main
type AglTupleStruct_uint8_string_bool struct {
	Arg0 uint8
	Arg1 string
	Arg2 bool
}
func testTuple() AglTupleStruct_uint8_string_bool {
	return AglTupleStruct_uint8_string_bool{Arg0: 1, Arg1: "hello", Arg2: true}
}
func main() {
	res := testTuple()
	AglAssert(res.Arg0 == 1, "assert failed line 7")
	AglAssert(res.Arg1 == "hello", "assert failed line 8")
	AglAssert(res.Arg2 == true, "assert failed line 9")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen37(t *testing.T) {
//	src := `package main
//type Dog struct {
//	name string?
//}
//type Person struct {
//	dog Dog?
//}
//func getPersonDogName(p Person) string? {
//	return p.getDog()?.getName()?
//}
//func main() {
//	person1 := Person{dog: Dog{name: Some("foo")}}
//	person2 := Person{dog: Some(Dog{name: None})}
//	person3 := Person{dog: None}
//	assert(person1.getDog()?.getName()? == "foo")
//	assert(person2.getDog()?.getName()?.IsNone())
//	assert(person3.getDog()?.getName()?.IsNone())
//}
//`
//	expected := `...`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen38(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
	ssn string?
	nicknames ([]string)?
	testArray []string
	testArrayOfOpt []string?
}
`
	expected := `// agl:generated
package main
type Person struct {
	name string
	age int
	ssn Option[string]
	nicknames Option[(AglVec[string])]
	testArray AglVec[string]
	testArrayOfOpt AglVec[Option[string]]
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen39(t *testing.T) {
//	src := `
//type Person struct {
//	name string
//}
//
//fn (p Person) getName() string {
//	return p.name
//}
//`
//	expected := `type Person struct {
//	name string
//}
//func (p Person) getName() string {
//	return p.name
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen40(t *testing.T) {
//	src := `
//fn addOne(i int) int { return i + 1 }
//fn main() {
//	a := true
//	addOne(a)
//}
//`
//	tassert.PanicsWithError(t, "5:9 wrong type of argument 0 in call to addOne, wants: int, got: bool", testCodeGenFn(src))
//}
//
//func TestCodeGen41(t *testing.T) {
//	src := `
//fn addOne(i int) int { return i + 1 }
//fn main() {
//	addOne(true)
//}
//`
//	tassert.PanicsWithError(t, "4:9 wrong type of argument 0 in call to addOne, wants: int, got: bool", testCodeGenFn(src))
//}

func TestCodeGen_Variadic1(t *testing.T) {
	src := `package main
func variadic(a, b u8, c ...string) int {
	return 1
}
func main() {
	variadic(1, 2, "a", "b", "c")
}
`
	expected := `// agl:generated
package main
func variadic(a, b uint8, c ...string) int {
	return 1
}
func main() {
	variadic(1, 2, "a", "b", "c")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen_Variadic2(t *testing.T) {
//	src := `
//fn variadic(a, b u8, c ...string) int {
//	return 1
//}
//fn main() {
//	variadic(1)
//}
//`
//	tassert.PanicsWithError(t, "6:2 not enough arguments in call to variadic", testCodeGenFn(src))
//}
//
//func TestCodeGen_Variadic3(t *testing.T) {
//	src := `
//fn variadic(a, b u8, c ...string) int {
//	return 1
//}
//fn main() {
//	variadic(1, 2, "a", 3, "c")
//}
//`
//	tassert.PanicsWithError(t, "6:22 wrong type of argument 3 in call to variadic, wants: string, got: UntypedNumType", testCodeGenFn(src))
//}

func TestCodeGen42(t *testing.T) {
	src := `package main
func someFn() {
}
func main() {
}
`
	expected := `// agl:generated
package main
func someFn() {
}
func main() {
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen43(t *testing.T) {
//	src := `
//fn main() {
//	a := 1u8
//	b := 2u16
//	c := 3u32
//	d := 4u64
//	e := 5i8
//	f := 6i16
//	g := 7i32
//	h := 8i64
//	i := 9f32
//	j := 10f64
//}
//`
//	expected := `func main() {
//	a := uint(42)
//	a := uint8(42)
//	a := uint16(42)
//	a := uint32(42)
//	a := uint64(42)
//	a := int8(42)
//	a := int16(42)
//	a := int32(42)
//	a := int64(42)
//	a := float32(42)
//	a := float64(42)
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen44(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := 1
	if a == 1 {
		fmt.Println("a == 1")
	} else if a == 2 {
		fmt.Println("a == 2")
	} else {
		fmt.Println("else")
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := 1
	if a == 1 {
		fmt.Println("a == 1")
	} else if a == 2 {
		fmt.Println("a == 2")
	} else {
		fmt.Println("else")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen45(t *testing.T) {
	src := `package main
func main() {
	a := 1
	if a == 1 {
	} else if a == 2 {
	} else {
	}
}
`
	expected := `// agl:generated
package main
func main() {
	a := 1
	if a == 1 {
	} else if a == 2 {
	} else {
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen46(t *testing.T) {
	src := `package main
func main() {
	a := 1 == 1 || 2 == 2 && 3 == 3
}
`
	expected := `// agl:generated
package main
func main() {
	a := 1 == 1 || 2 == 2 && 3 == 3
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen47(t *testing.T) {
	src := `package main
func test(v bool) {}
func main() {
	test(true)
	test(1 == 1)
	test(1 == 1 && 2 == 2)
	test("a" == "b")
}
`
	expected := `// agl:generated
package main
func test(v bool) {
}
func main() {
	test(true)
	test(1 == 1)
	test(1 == 1 && 2 == 2)
	test("a" == "b")
}
`
	testCodeGen(t, src, expected)
}

//	func TestCodeGen48(t *testing.T) {
//		src := `
//
// fn test(v bool) {}
//
//	fn main() {
//		test("a" == 42)
//	}
//
// `
//
//		tassert.PanicsWithError(t, "4:7 mismatched types string and UntypedNumType", testCodeGenFn(src))
//	}
func TestCodeGen49(t *testing.T) {
	src := `package main
func main() {
	a := []u8{1, 2, 3}
	s := a.Sum()
	assert(s == 6)
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[uint8]{1, 2, 3}
	s := AglVecSum(a)
	AglAssert(s == 6, "assert failed line 5")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen50(t *testing.T) {
//	src := `
//fn main() {
//	a := []string{"a", "b", "c"}
//}
//`
//	tassert.PanicsWithValue(t, "should fail for non numbers", func() { codegen(infer(parser(NewTokenStream(src)))) })
//}

func TestCodeGen51(t *testing.T) {
	src := `package main
type Person struct {
}
func (p Person) speak() string {
}
func main() {
}
`
	expected := `// agl:generated
package main
type Person struct{}
func (p Person) speak() string {
}
func main() {
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen51_1(t *testing.T) {
	src := `package main
type Person struct {
	age int
}
func main() {
	p := Person{}
}
`
	expected := `// agl:generated
package main
type Person struct {
	age int
}
func main() {
	p := Person{}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen52(t *testing.T) {
	src := `package main
type Person struct {
}
func (p Person) method1() Person! {
	return Ok(p)
}
func main() {
	p := Person{}
	a := p.method1()!.method1()
}
`
	expected := `// agl:generated
package main
type Person struct{}
func (p Person) method1() Result[Person] {
	return MakeResultOk(p)
}
func main() {
	p := Person{}
	a := p.method1().Unwrap().method1()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen52_1(t *testing.T) {
	src := `package main
type Person struct {
}
func (p Person) method1() Person? {
	return Some(p)
}
func main() {
	p := Person{}
	a := p.method1()?.method1()
}
`
	expected := `// agl:generated
package main
type Person struct{}
func (p Person) method1() Option[Person] {
	return MakeOptionSome(p)
}
func main() {
	p := Person{}
	a := p.method1().Unwrap().method1()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen53(t *testing.T) {
	src := `package main
type Person struct {
}
func (p Person) method1() Person! {
	return Ok(p)
}
func main() {
	p := Person{}
	a := p.method1()!.method1()!
}
`
	expected := `// agl:generated
package main
type Person struct{}
func (p Person) method1() Result[Person] {
	return MakeResultOk(p)
}
func main() {
	p := Person{}
	a := p.method1().Unwrap().method1().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen54(t *testing.T) {
	src := `package main
type Color enum {
	red
	green
	blue
}
func takeColor(c Color) {}
func main() {
	color := Color.red
	takeColor(color)
}
`
	expected := `// agl:generated
package main
type ColorTag int
const (
	Color_red ColorTag = iota
	Color_green
	Color_blue
)
type Color struct {
	tag ColorTag
}
func (v Color) String() string {
	switch v.tag {
	case Color_red:
		return "red"
	case Color_green:
		return "green"
	case Color_blue:
		return "blue"
	default:
		panic("")
	}
}
func (v Color) RawValue() int {
	return int(v.tag)
}
func Make_Color_red() Color {
	return Color{tag: Color_red}
}
func Make_Color_green() Color {
	return Color{tag: Color_green}
}
func Make_Color_blue() Color {
	return Color{tag: Color_blue}
}

func takeColor(c Color) {
}
func main() {
	color := Make_Color_red()
	takeColor(color)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Enum2(t *testing.T) {
	src := `package main
type Color enum {
	red
	other(u8, string)
}
func takeColor(c Color) {}
func main() {
	color1 := Color.red
	color2 := Color.other(1, "yellow")
}
`
	expected := `// agl:generated
package main
type ColorTag int
const (
	Color_red ColorTag = iota
	Color_other
)
type Color struct {
	tag ColorTag
	other_0 uint8
	other_1 string
}
func (v Color) String() string {
	switch v.tag {
	case Color_red:
		return "red"
	case Color_other:
		return fmt.Sprintf("other(%v, %v)", v.other_0, v.other_1)
	default:
		panic("")
	}
}
func (v Color) RawValue() int {
	return int(v.tag)
}
func Make_Color_red() Color {
	return Color{tag: Color_red}
}
func Make_Color_other(arg0 uint8, arg1 string) Color {
	return Color{tag: Color_other, other_0: arg0, other_1: arg1}
}

func takeColor(c Color) {
}
func main() {
	color1 := Make_Color_red()
	color2 := Make_Color_other(1, "yellow")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Enum3(t *testing.T) {
	src := `package main
type Color enum {
	red
	other(u8, string)
}
func main() {
	a, b := Color.other(1, "yellow")
}
`
	expected := `// agl:generated
package main
type ColorTag int
const (
	Color_red ColorTag = iota
	Color_other
)
type Color struct {
	tag ColorTag
	other_0 uint8
	other_1 string
}
func (v Color) String() string {
	switch v.tag {
	case Color_red:
		return "red"
	case Color_other:
		return fmt.Sprintf("other(%v, %v)", v.other_0, v.other_1)
	default:
		panic("")
	}
}
func (v Color) RawValue() int {
	return int(v.tag)
}
func Make_Color_red() Color {
	return Color{tag: Color_red}
}
func Make_Color_other(arg0 uint8, arg1 string) Color {
	return Color{tag: Color_other, other_0: arg0, other_1: arg1}
}

func main() {
	aglVar1 := Make_Color_other(1, "yellow")
	a, b := aglVar1.other_0, aglVar1.other_1
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Enum4(t *testing.T) {
	src := `package main
type Color enum {
	red
	other(u8, string)
}
func main() {
	other := Color.other(1, "yellow")
	a, b := other
}
`
	expected := `// agl:generated
package main
type ColorTag int
const (
	Color_red ColorTag = iota
	Color_other
)
type Color struct {
	tag ColorTag
	other_0 uint8
	other_1 string
}
func (v Color) String() string {
	switch v.tag {
	case Color_red:
		return "red"
	case Color_other:
		return fmt.Sprintf("other(%v, %v)", v.other_0, v.other_1)
	default:
		panic("")
	}
}
func (v Color) RawValue() int {
	return int(v.tag)
}
func Make_Color_red() Color {
	return Color{tag: Color_red}
}
func Make_Color_other(arg0 uint8, arg1 string) Color {
	return Color{tag: Color_other, other_0: arg0, other_1: arg1}
}

func main() {
	other := Make_Color_other(1, "yellow")
	aglVar1 := other
	a, b := aglVar1.other_0, aglVar1.other_1
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen_GenericMethod(t *testing.T) {
//	src := `
//type Person struct {
//}
//fn (p Person) speak[T any, U any](a T, b U) string {
//	fmt.Println(a)
//}
//fn main() {
//	p1 := Person{}
//	p1.speak("hello", 123)
//	p1.speak(123, "hello")
//}
//`
//	expected := `type Person struct {
//}
//func (p Person) speak__string_int(a string) string {
//	fmt.Println(a)
//}
//func (p Person) speak__int_string(a int) string {
//	fmt.Println(a)
//}
//func main() {
//	p1 := Person{}
//	p1.speak__string_int("hello", 123)
//	p1.speak__int_string(123, "hello")
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen_OperatorOverloading(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
}
func (p Person) == (other Person) bool {
	return p.age == other.age
}
func main() {
	p1 := Person{name: "foo", age: 42}
	p2 := Person{name: "bar", age: 42}
	assert(p1 == p2)
}
`
	expected := `// agl:generated
package main
type Person struct {
	name string
	age int
}
func (p Person) __EQL(other Person) bool {
	return p.age == other.age
}
func main() {
	p1 := Person{name: "foo", age: 42}
	p2 := Person{name: "bar", age: 42}
	AglAssert(p1.__EQL(p2), "assert failed line 12")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_55(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := 2
	fmt.Println("first")
	if a == 2 {
		fmt.Println("second")
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := 2
	fmt.Println("first")
	if a == 2 {
		fmt.Println("second")
	}
}
`
	testCodeGen(t, src, expected)
}

//	func TestCodeGen_56(t *testing.T) {
//		src := `
//
//	type Color enum {
//		blue,
//		red,
//	}
//
//	fn main() {
//		color := Color.red1
//	}
//
// `
//
//		tassert.PanicsWithError(t, "7:17: enum Color has no field red1", testCodeGenFn(src))
//	}
//func TestCodeGen57(t *testing.T) {
//	src := `package main
//func main() {
//	a := []int{1, 2, 3}
//	if 2 in a {
//		fmt.Println("found")
//	}
//}
//`
//	expected := `// agl:generated
//package main
//func main() {
//	a := []int{1, 2, 3}
//	if AglVecIn(a, 2) {
//		fmt.Println("found")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen58(t *testing.T) {
	src := `package main
func test() int! {
   return Err("test")
}
func main() {
   test()!
}
`
	expected := `// agl:generated
package main
func test() Result[int] {
	return MakeResultErr[int](Errors.New("test"))
}
func main() {
	test().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen59(t *testing.T) {
	src := `package main
func test() ! {
   return Err("test")
}
func main() {
   test()!
}
`
	expected := `// agl:generated
package main
func test() Result[AglVoid] {
	return MakeResultErr[AglVoid](Errors.New("test"))
}
func main() {
	test().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_ErrPropagationInOption(t *testing.T) {
	src := `package main
func errFn() ! {
   return Err("some error")
}
func maybeInt() int? {
	errFn()!
	return Some(42)
}
func main() {
   maybeInt()?
}
`
	expected := `// agl:generated
package main
func errFn() Result[AglVoid] {
	return MakeResultErr[AglVoid](Errors.New("some error"))
}
func maybeInt() Option[int] {
	aglTmpVar1 := errFn()
	if aglTmpVar1.IsErr() {
		return MakeOptionNone[int]()
	}
	aglTmpVar1.Unwrap()
	return MakeOptionSome(42)
}
func main() {
	maybeInt().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen60(t *testing.T) {
	src := `package main
type Writer interface {}
func main() {
}
`
	expected := `// agl:generated
package main
type Writer interface{}
func main() {
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_TypeAssertion(t *testing.T) {
	src := `
package main

import "agl1/fmt"

type Writer interface {}

type WriterA struct {}

type WriterB struct {}

func test(w Writer) {
   if _, ok := w.(WriterA); ok {
       fmt.Println("A")
   }
}

func main() {
   w := WriterA{}
   test(w)
   fmt.Println("done")
}

`
	expected := `// agl:generated
package main
import "fmt"
type Writer interface{}
type WriterA struct{}
type WriterB struct{}
func test(w Writer) {
	if _, ok := w.(WriterA); ok {
		fmt.Println("A")
	}
}
func main() {
	w := WriterA{}
	test(w)
	fmt.Println("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen61(t *testing.T) {
	src := `
package main

import "agl1/os"
import "agl1/fmt"

func main() {
	os.WriteFile("test.txt", []byte("test"), 0755)!
}
`
	expected := `// agl:generated
package main
import (
	"os"
	"fmt"
)
func main() {
	aglTmpErr1 := os.WriteFile("test.txt", AglVec[byte]("test"), 0755)
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	AglNoop()
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen62(t *testing.T) {
//	src := `package main
//func maybeInt() int? { return Some(42) }
//func getInt() int! { return Ok(42) }
//func main() {
//	if Some(a) := maybeInt() {
//	}
//	if Ok(a) := getInt() {
//	}
//	if Err(e) := getInt() {
//	}
//}
//`
//	expected := `// agl:generated
//package main
//func maybeInt() Option[int] {
//	return MakeOptionSome(42)
//}
//func getInt() Result[int] {
//	return MakeResultOk(42)
//}
//func main() {
//	if res := maybeInt(); res.IsSome() {
//		a := res.Unwrap()
//	}
//	if res := getInt(); res.IsOk() {
//		a := res.Unwrap()
//	}
//	if res := getInt(); res.IsErr() {
//		e := res.Err()
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

//	func TestCodeGen63(t *testing.T) {
//		src1 := `
//
// fn maybeInt() int? { return Some(42) }
//
//	fn main() {
//		if Some(a) := maybeInt() {
//		}
//		fmt.Println(a)
//	}
//
// `
//
//	tassert.PanicsWithError(t, "6:14: undefined identifier a", testCodeGenFn(src1))
//	src2 := `
//
// fn maybeInt() int? { return Some(42) }
//
//	fn main() {
//		if Some(a) := maybeInt() {
//			fmt.Println(a)
//		}
//	}
//
// `
//
//		tassert.NotPanics(t, testCodeGenFn(src2))
//	}

func TestCodeGen_FunctionImplicitReturn(t *testing.T) {
	src := `package main
func maybeInt() int? { Some(42) }
func main() {
	maybeInt()
}
`
	expected := `// agl:generated
package main
func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	maybeInt()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen64(t *testing.T) {
	src := `package main
type Writer interface {
	write([]byte) int!
}
`
	expected := `// agl:generated
package main
type Writer interface {
	write(AglVec[byte]) Result[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen65(t *testing.T) {
	src := `package main
type Writer interface {
	write([]byte) int!
	another() bool
}
`
	expected := `// agl:generated
package main
type Writer interface {
	write(AglVec[byte]) Result[int]
	another() bool
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen_ValueSpec1(t *testing.T) {
//	src := `package main
//func main() {
//	var a int? = None
//}
//`
//	expected := `// agl:generated
//package main
//func main() {
//	var a Option[int] = MakeOptionNone[int]()
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen_ValueSpec2(t *testing.T) {
	src := `package main
func main() {
	var a int?
}
`
	expected := `// agl:generated
package main
func main() {
	var a Option[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen66(t *testing.T) {
	src := `package main
import "agl1/fmt"
type Color enum {
	red
	other(u8, string)
}
func main() {
	a, b := Color.other(1, "yellow")
	fmt.Println(a, b)
}
`
	expected := `// agl:generated
package main
import "fmt"
type ColorTag int
const (
	Color_red ColorTag = iota
	Color_other
)
type Color struct {
	tag ColorTag
	other_0 uint8
	other_1 string
}
func (v Color) String() string {
	switch v.tag {
	case Color_red:
		return "red"
	case Color_other:
		return fmt.Sprintf("other(%v, %v)", v.other_0, v.other_1)
	default:
		panic("")
	}
}
func (v Color) RawValue() int {
	return int(v.tag)
}
func Make_Color_red() Color {
	return Color{tag: Color_red}
}
func Make_Color_other(arg0 uint8, arg1 string) Color {
	return Color{tag: Color_other, other_0: arg0, other_1: arg1}
}

func main() {
	aglVar1 := Make_Color_other(1, "yellow")
	a, b := aglVar1.other_0, aglVar1.other_1
	fmt.Println(a, b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen67(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := []u8{1, 2, 3, 4, 5}
	var b u8 = a.Find({ $0 == 2 })?
	fmt.Println(b)
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := AglVec[uint8]{1, 2, 3, 4, 5}
	var b uint8 = AglVecFind(a, func(aglArg0 uint8) bool {
		return aglArg0 == 2
	}).Unwrap()
	fmt.Println(b)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen68(t *testing.T) {
//	src := `
//fn main() {
//	a := []u8{1, 2, 3, 4, 5}
//	var b u8 = a.find({ $0 == 2 })
//}
//`
//	tassert.PanicsWithError(t, "4:2: cannot use Option[u8] as u8 value in variable declaration", testCodeGenFn(src))
//}

func TestCodeGen69(t *testing.T) {
	src := `package main
func test() []u8 {
	[]u8{1, 2, 3}
}
func main() {
	test().Filter({ $0 == 2 })
}
`
	expected := `// agl:generated
package main
func test() AglVec[uint8] {
	return AglVec[uint8]{1, 2, 3}
}
func main() {
	AglVecFilter(test(), func(aglArg0 uint8) bool {
		return aglArg0 == 2
	})
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen70(t *testing.T) {
//	src := `
//fn test() []u8 { []u8{1, 2, 3} }
//fn main() {
//	test().filter({ %0 == 2 })
//}
//`
//	tassert.PanicsWithError(t, "4:18: syntax error", testCodeGenFn(src))
//}
//
//func TestCodeGen71(t *testing.T) {
//	src := `
//fn maybeInt() int? { Some(42) }
//fn main() {
//	match maybeInt() {
//		Some(a) => { fmt.Println("some ", a) },
//		None    => { fmt.Println("none") },
//	}
//}
//`
//	expected := `func maybeInt() Option[int] {
//	return MakeOptionSome(42)
//}
//func main() {
//	res := maybeInt()
//	if res.IsSome() {
//		a := res.Unwrap()
//		fmt.Println("some ", a)
//	} else if res.IsNone() {
//		fmt.Println("none")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen72(t *testing.T) {
//	src := `
//fn maybeInt() int? { Some(42) }
//fn main() {
//	match maybeInt() {
//		_ => { fmt.Println("Some or None") },
//	}
//}
//`
//	expected := `func maybeInt() Option[int] {
//	return MakeOptionSome(42)
//}
//func main() {
//	res := maybeInt()
//	if res.IsSome() || res.IsNone() {
//		fmt.Println("Some or None")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen75(t *testing.T) {
//	src := `
//fn maybeInt() int? { Some(42) }
//fn main() {
//	match maybeInt() {
//		_ => { fmt.Println("Some or None") },
//		None => { fmt.Println("none") },
//	}
//}
//`
//	expected := `func maybeInt() Option[int] {
//	return MakeOptionSome(42)
//}
//func main() {
//	res := maybeInt()
//	if res.IsNone() {
//		fmt.Println("none")
//	} else if res.IsSome() || res.IsNone() {
//		fmt.Println("Some or None")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen73(t *testing.T) {
//	src := `
//fn maybeInt() int? { Some(42) }
//fn main() {
//	match maybeInt() {
//		Some(a) => { fmt.Println("some ", a) },
//	}
//}
//`
//	tassert.PanicsWithError(t, "4:8: match statement must be exhaustive", testCodeGenFn(src))
//}
//
//func TestCodeGen74(t *testing.T) {
//	src := `
//fn maybeInt() int? { Some(42) }
//fn main() {
//	match maybeInt() {
//		None => { fmt.Println("none") },
//	}
//}
//`
//	tassert.PanicsWithError(t, "4:8: match statement must be exhaustive", testCodeGenFn(src))
//}
//
//func TestCodeGen76(t *testing.T) {
//	src := `
//fn getInt() int! { Ok(42) }
//fn main() {
//	match getInt() {
//		_ => { fmt.Println("Ok or Err") },
//	}
//}
//`
//	expected := `func getInt() Result[int] {
//	return MakeResultOk(42)
//}
//func main() {
//	res := getInt()
//	if res.IsOk() || res.IsErr() {
//		fmt.Println("Ok or Err")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen77(t *testing.T) {
//	src := `
//fn getInt() int! { Ok(42) }
//fn main() {
//	match getInt() {
//		_ => { fmt.Println("Ok or Err") },
//		Ok(n) => { fmt.Println("Ok ", n) },
//	}
//}
//`
//	expected := `func getInt() Result[int] {
//	return MakeResultOk(42)
//}
//func main() {
//	res := getInt()
//	if res.IsOk() {
//		n := res.Unwrap()
//		fmt.Println("Ok ", n)
//	} else if res.IsOk() || res.IsErr() {
//		fmt.Println("Ok or Err")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen78(t *testing.T) {
//	src := `
//fn getInt() int! { Ok(42) }
//fn main() {
//	match getInt() {
//		Ok(n) => { fmt.Println("Ok ", n) },
//		Err(e) => { fmt.Println("Err ", e) },
//	}
//}
//`
//	expected := `func getInt() Result[int] {
//	return MakeResultOk(42)
//}
//func main() {
//	res := getInt()
//	if res.IsOk() {
//		n := res.Unwrap()
//		fmt.Println("Ok ", n)
//	} else if res.IsErr() {
//		fmt.Println("Err ", e)
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen79(t *testing.T) {
//	src := `
//fn getInt() int! { Ok(42) }
//fn main() {
//	match getInt() {
//		Ok(n) => fmt.Println("Ok ", n),
//		Err(e) => fmt.Println("Err ", e),
//	}
//}
//`
//	expected := `func getInt() Result[int] {
//	return MakeResultOk(42)
//}
//func main() {
//	res := getInt()
//	if res.IsOk() {
//		n := res.Unwrap()
//		fmt.Println("Ok ", n)
//	} else if res.IsErr() {
//		fmt.Println("Err ", e)
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen80(t *testing.T) {
//	src := `
//fn main() {
//	select {
//	case <-time.After(100):
//		fmt.Println("timeout")
//	default:
//		fmt.Println("default")
//	}
//}
//`
//	expected := `func main() {
//	select {
//		case <-time.After(100):
//			fmt.Println("timeout")
//		default:
//			fmt.Println("default")
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen81(t *testing.T) {
	src := `package main
func main() {
	_ = 42
}
`
	expected := `// agl:generated
package main
func main() {
	_ = 42
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen82(t *testing.T) {
//	src := `
//fn main() {
//	_ := 42
//}
//`
//	tassert.PanicsWithError(t, "3:4: No new variables on the left side of ':='", testCodeGenFn(src))
//}

//func TestCodeGen83(t *testing.T) {
//	src := `package main
//func main() {
//	a := []u8{1, 2, 3, 4, 5}
//	a.find(fn(e u8) bool { e == 2 })?
//}
//`
//	expected := `// agl:generated
//package main
//func main() {
//	a := []uint8{1, 2, 3, 4, 5}
//	AglVecFind(a, func(e uint8) bool {
//		return e == 2
//	}).Unwrap()
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen84(t *testing.T) {
//	src := `
//fn main() {
//	a := []u8{1, 2, 3, 4, 5}
//	a.find(fn(e i64) bool { e == 2 })?
//}
//`
//	tassert.PanicsWithError(t, "4:2: function type fn(i64) bool does not match inferred type fn(u8) bool", testCodeGenFn(src))
//}
//
//func TestCodeGen84_1(t *testing.T) {
//	src := `
//fn main() {
//	a := []u8{1, 2, 3, 4, 5}
//	a.find(fn(e u8) { e == 2 })?
//}
//`
//	tassert.PanicsWithError(t, "4:2: function type fn(u8) does not match inferred type fn(u8) bool", testCodeGenFn(src))
//}
//
//func TestCodeGen84_2(t *testing.T) {
//	src := `
//fn main() {
//	a := []u8{1, 2, 3, 4, 5}
//	f := fn(e u8) { e == 2 }
//	a.find(f)?
//}
//`
//	tassert.PanicsWithError(t, "5:2: function type fn(u8) does not match inferred type fn(u8) bool", testCodeGenFn(src))
//}

func TestCodeGen85(t *testing.T) {
	src := `package main
func main() {
	a := []u8{1, 2, 3, 4, 5}
	f := func(e u8) bool { return e == 2 }
	a.Find(f)?
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[uint8]{1, 2, 3, 4, 5}
	f := func(e uint8) bool {
		return e == 2
	}
	AglVecFind(a, f).Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen86(t *testing.T) {
	src := `package main
func main() {
	if a := 123; a == 2 || a == 3 {
	}
}
`
	expected := `// agl:generated
package main
func main() {
	if a := 123; a == 2 || a == 3 {
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen87(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/errors"
)
`
	expected := `// agl:generated
package main
import (
	"fmt"
	"errors"
)
`
	testCodeGen(t, src, expected)
}

func TestCodeGen88(t *testing.T) {
	src := `package main
type Pos struct {
	Row, Col int
}
`
	expected := `// agl:generated
package main
type Pos struct {
	Row, Col int
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen89(t *testing.T) {
	src := `
package main
import "agl1/fmt"
type Person struct {
	name string
}
func main() {
	p1 := Person{name: "John"}
	p2 := Person{name: "Jane"}
	arr := []Person{p1, p2}
	res := arr.Map({ $0.name }).Joined(", ")
	fmt.Println(res)
}
`
	expected := `// agl:generated
package main
import "fmt"
type Person struct {
	name string
}
func main() {
	p1 := Person{name: "John"}
	p2 := Person{name: "Jane"}
	arr := AglVec[Person]{p1, p2}
	res := AglVecJoined(AglVecMap(arr, func(aglArg0 Person) string {
		return aglArg0.name
	}), ", ")
	fmt.Println(res)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen90(t *testing.T) {
//	src := `
//	package main
//	import "fmt"
//	type Person struct {
//		age int
//	}
//	fn main() {
//		p1 := Person{age: 1}
//		p2 := Person{age: 2}
//		arr := []Person{p1, p2}
//		res := arr.map({ $0.age }).joined(", ")
//		fmt.Println(res)
//	}
//`
//	tassert.PanicsWithError(t, "11:10: type mismatch, wants: []string, got: []int", testCodeGenFn(src))
//}

func TestCodeGen91(t *testing.T) {
	src := `package main
	func main() {
		var arr []int
	}
`
	expected := `// agl:generated
package main
func main() {
	var arr AglVec[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen93(t *testing.T) {
	src := `package main
	func main() {
		var arr1 []int
		var arr2 []int
	}
`
	expected := `// agl:generated
package main
func main() {
	var arr1 AglVec[int]
	var arr2 AglVec[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen94(t *testing.T) {
	src := `package main
	func main() {
		var arr1, arr3 []int
		var arr2 []int
	}
`
	expected := `// agl:generated
package main
func main() {
	var arr1, arr3 AglVec[int]
	var arr2 AglVec[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen92(t *testing.T) {
	src := `package main
	func main() {
		 row, col := 1, 0
	}
`
	expected := `// agl:generated
package main
func main() {
	row, col := 1, 0
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := []Person{p1, p2}
	names := people.Map(func(el Person) string { return el.name }).Joined(", ")
}
`
	expected := `// agl:generated
package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := AglVec[Person]{p1, p2}
	names := AglVecJoined(AglVecMap(people, func(el Person) string {
		return el.name
	}), ", ")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95_2(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
}
func clb(el Person) string { return el.name }
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := []Person{p1, p2}
	names := people.Map(clb).Joined(", ")
}
`
	expected := `// agl:generated
package main
type Person struct {
	name string
	age int
}
func clb(el Person) string {
	return el.name
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := AglVec[Person]{p1, p2}
	names := AglVecJoined(AglVecMap(people, clb), ", ")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95_3(t *testing.T) {
	src := `package main
import "agl1/strconv"
func main() {
	a := []string{"1", "2"}
	a.Map({ strconv.Atoi($0)! })
}
`
	expected := `// agl:generated
package main
import "strconv"
func main() {
	a := AglVec[string]{"1", "2"}
	AglVecMap(a, func(aglArg0 string) int {
		aglTmp1, aglTmpErr1 := strconv.Atoi(aglArg0)
		if aglTmpErr1 != nil {
			panic(aglTmpErr1)
		}
		return AglIdentity(aglTmp1)
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95_4(t *testing.T) {
	src := `package main
import "agl1/strconv"
func main() {
	a := "1 2"
	a.Split(" ").Map({ strconv.Atoi($0)! })
}
`
	expected := `// agl:generated
package main
import "strconv"
func main() {
	a := "1 2"
	AglVecMap(AglStringSplit(a, " "), func(aglArg0 string) int {
		aglTmp1, aglTmpErr1 := strconv.Atoi(aglArg0)
		if aglTmpErr1 != nil {
			panic(aglTmpErr1)
		}
		return AglIdentity(aglTmp1)
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95_5(t *testing.T) {
	src := `package main
import "agl1/strconv"
func main() {
	a := "1 2, 3 4"
	a.Split(",").Map({
		$0.Split(" ").Map({ strconv.Atoi($0)! })
	})
}
`
	expected := `// agl:generated
package main
import "strconv"
func main() {
	a := "1 2, 3 4"
	AglVecMap(AglStringSplit(a, ","), func(aglArg0 string) AglVec[int] {
		return AglVecMap(AglStringSplit(aglArg0, " "), func(aglArg0 string) int {
			aglTmp1, aglTmpErr1 := strconv.Atoi(aglArg0)
			if aglTmpErr1 != nil {
				panic(aglTmpErr1)
			}
			return AglIdentity(aglTmp1)
		})
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen95_6(t *testing.T) {
	src := `package main
import "agl1/strconv"
func main() {
	a := "1 2, 3 4"
	a.Split(",").Map({
		tmp1 := $0.Split(" ")
		return tmp1.Map({ strconv.Atoi($0)! })
	})
}
`
	expected := `// agl:generated
package main
import "strconv"
func main() {
	a := "1 2, 3 4"
	AglVecMap(AglStringSplit(a, ","), func(aglArg0 string) AglVec[int] {
		tmp1 := AglStringSplit(aglArg0, " ")
		return AglVecMap(tmp1, func(aglArg0 string) int {
			aglTmp1, aglTmpErr1 := strconv.Atoi(aglArg0)
			if aglTmpErr1 != nil {
				panic(aglTmpErr1)
			}
			return AglIdentity(aglTmp1)
		})
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen96(t *testing.T) {
	src := `package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := []Person{p1, p2}
	names := people.Map(func(el Person) string { el.name }).Joined(", ")
}
`
	expected := `// agl:generated
package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := AglVec[Person]{p1, p2}
	names := AglVecJoined(AglVecMap(people, func(el Person) string {
		return el.name
	}), ", ")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen97(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	for i := 0; i < 10; i++ {
		fmt.Println(i)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	for i := 0; i < 10; i++ {
		fmt.Println(i)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen98(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	for {
		fmt.Println("hello")
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	for {
		fmt.Println("hello")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen99(t *testing.T) {
	src := `package main
import "agl1/fmt"
func testSome() int? {
	return Some(42)
}
func main() {
	if Some(a) := testSome() {
		fmt.Println(a)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func testSome() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	if aglTmp1 := testSome(); aglTmp1.IsSome() {
		a := aglTmp1.Unwrap()
		fmt.Println(a)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen100(t *testing.T) {
	src := `package main
import "agl1/fmt"
func testOk() int! {
	return Ok(42)
}
func main() {
	if Ok(a) := testOk() {
		fmt.Println(a)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func testOk() Result[int] {
	return MakeResultOk(42)
}
func main() {
	if aglTmp1 := testOk(); aglTmp1.IsOk() {
		a := aglTmp1.Unwrap()
		fmt.Println(a)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen101(t *testing.T) {
	src := `package main
import "agl1/fmt"
func testOk() int! {
	return Err("error")
}
func main() {
	if Err(e) := testOk() {
		fmt.Println(e)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
func testOk() Result[int] {
	return MakeResultErr[int](Errors.New("error"))
}
func main() {
	if aglTmp1 := testOk(); aglTmp1.IsErr() {
		e := aglTmp1.Err()
		fmt.Println(e)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen102(t *testing.T) {
	src := `package main
import "agl1/fmt"
func testSome() int? {
   return Some(42)
}
func main() {
   if Err(a) := testSome() {
       fmt.Println("test", a)
   }
}
`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "7:11: try to destructure a non-Result type into an ResultType")
}

func TestCodeGen103(t *testing.T) {
	src := `package main
import "fmt"
func testResult() int! {
   return Ok(42)
}
func main() {
   if Some(a) := testResult() {
       fmt.Println("test", a)
   }
}
`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "7:12: try to destructure a non-Option type into an OptionType")
}

func TestCodeGen104(t *testing.T) {
	src := `package main
func main() {
	a := 1
	a++
}
`
	expected := `// agl:generated
package main
func main() {
	a := 1
	a++
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen105(t *testing.T) {
	src := `package main
func main() {
	c := make(chan int)
	c <- 1
}
`
	expected := `// agl:generated
package main
func main() {
	c := make(chan int)
	c <- 1
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen106(t *testing.T) {
	src := `package main
func main() {
	c1 := make(chan int)
	c2 := make(chan int)
	select {
	case <-c1:
	case <-c2:
	default:
	}
}
`
	expected := `// agl:generated
package main
func main() {
	c1 := make(chan int)
	c2 := make(chan int)
	select {
	case <-c1:
	case <-c2:
	default:
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen107(t *testing.T) {
	src := `package main
func main() {
	a := 1
	switch a {
	case 1:
	case 2, 3:
	default:
	}
}
`
	expected := `// agl:generated
package main
func main() {
	a := 1
	switch a {
	case 1:
	case 2, 3:
	default:
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen108(t *testing.T) {
	src := `package main
func main() {
	mut a := map[string]int{"a": 1}
	a["a"] = 2
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglMap[string, int]{"a": 1}
	a["a"] = 2
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen109(t *testing.T) {
	src := `package main
func main() {
	Loop:
	for {
		break Loop
	}
}
`
	expected := `// agl:generated
package main
func main() {
	Loop:
	for {
		break Loop
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen110(t *testing.T) {
	src := `package main
func test() {
}
func main() {
	defer test()
	go test()
}
`
	expected := `// agl:generated
package main
func test() {
}
func main() {
	defer test()
	go test()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen111(t *testing.T) {
	src := `package main
func main() {
	var v any
	switch v.(type) {
	default:
	}
}
`
	expected := `// agl:generated
package main
func main() {
	var v any
	switch v.(type) {
	default:
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen112(t *testing.T) {
	src := `package main
import "agl1/fmt"
type IpAddr enum {
    v4(u8, u8, u8, u8)
    v6(string)
}
func main() {
    // enum values can be destructured
    addr1 := IpAddr.v4(127, 0, 0, 1)
    a, b, c, d := addr1
	
    // tuple can be destructured
    tuple := (1, "hello", true)
    e, f, g := tuple
	
	fmt.Println(a, b, c, d, e, f, g)
}
`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_int_string_bool struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
type IpAddrTag int
const (
	IpAddr_v4 IpAddrTag = iota
	IpAddr_v6
)
type IpAddr struct {
	tag IpAddrTag
	v4_0 uint8
	v4_1 uint8
	v4_2 uint8
	v4_3 uint8
	v6_0 string
}
func (v IpAddr) String() string {
	switch v.tag {
	case IpAddr_v4:
		return fmt.Sprintf("v4(%v, %v, %v, %v)", v.v4_0, v.v4_1, v.v4_2, v.v4_3)
	case IpAddr_v6:
		return fmt.Sprintf("v6(%v)", v.v6_0)
	default:
		panic("")
	}
}
func (v IpAddr) RawValue() int {
	return int(v.tag)
}
func Make_IpAddr_v4(arg0 uint8, arg1 uint8, arg2 uint8, arg3 uint8) IpAddr {
	return IpAddr{tag: IpAddr_v4, v4_0: arg0, v4_1: arg1, v4_2: arg2, v4_3: arg3}
}
func Make_IpAddr_v6(arg0 string) IpAddr {
	return IpAddr{tag: IpAddr_v6, v6_0: arg0}
}

func main() {
	addr1 := Make_IpAddr_v4(127, 0, 0, 1)
	aglVar1 := addr1
	a, b, c, d := aglVar1.v4_0, aglVar1.v4_1, aglVar1.v4_2, aglVar1.v4_3
	tuple := AglTupleStruct_int_string_bool{Arg0: 1, Arg1: "hello", Arg2: true}
	aglVar2 := tuple
	e, f, g := aglVar2.Arg0, aglVar2.Arg1, aglVar2.Arg2
	fmt.Println(a, b, c, d, e, f, g)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen113(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := 'a'
	fmt.Println(string(a))
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := 'a'
	fmt.Println(string(a))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen114(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := 1
	fmt.Println(u8(a))
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := 1
	fmt.Println(uint8(a))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen115(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	a := 1
	b := &a
	c := *b
	fmt.Println(a, b, c)
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	a := 1
	b := &a
	c := *b
	fmt.Println(a, b, c)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen116(t *testing.T) {
	src := `package main

type Person struct { Name string }

func (p Person) MaybeSelf() Person? {
	return Some(p)
}

func main() {
	bob := Person{Name: "bob"}
	bob.MaybeSelf().MaybeSelf()?
}
`
	fset, f := ParseSrc(src)
	env := NewEnv()
	i := NewInferrer(env)
	errs := i.InferFile("", f, fset, true)
	fmt.Print(errs)
	tassert.Equal(t, 1, 1)
}

func TestCodeGen117(t *testing.T) {
	src := `package main
func main() {
	m := make(map[string]int)
}
`
	expected := `// agl:generated
package main
func main() {
	m := make(AglMap[string, int])
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen118(t *testing.T) {
	src := `package main
func test(m map[string]int) {
}
func main() {
	m := make(map[string]int)
	test(m)
}
`
	expected := `// agl:generated
package main
func test(m map[string]int) {
}
func main() {
	m := make(AglMap[string, int])
	test(m)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen119(t *testing.T) {
	src := `package main
func test(m map[int]int) {
}
func main() {
	m := make(map[string]int)
	test(m)
}
`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "6:7: types not equal, map[int]int map[string]int")
}

func TestCodeGen120(t *testing.T) {
	src := `package main
func test(m map[string]int) {
}
func main() {
	a := map[string]int{"a": 1}
	test(a)
}
`
	expected := `// agl:generated
package main
func test(m map[string]int) {
}
func main() {
	a := AglMap[string, int]{"a": 1}
	test(a)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen121(t *testing.T) {
	src := `package main
func test(a []string) {
}
func main() {
	a := []int{1, 2, 3}
	test(a)
}
`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "6:7: types not equal, []string []int")
}

func TestCodeGen122(t *testing.T) {
	src := `package main
func test(a []int) {
}
func main() {
	a := []int{1, 2, 3}
	test(a)
}
`
	expected := `// agl:generated
package main
func test(a AglVec[int]) {
}
func main() {
	a := AglVec[int]{1, 2, 3}
	test(a)
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen123(t *testing.T) {
//	src := `package main
//func test() int? { Some(42) }
//func main() {
//	num := test() or_return
//	fmt.Println(num)
//}
//`
//	expected := `// agl:generated
//package main
//func test() Option[int] {
//	return MakeOptionSome(42)
//}
//func main() {
//	num := test()
//	if num.IsNone() {
//		return
//	}
//	fmt.Println(num)
//}
//`
//	testCodeGen(t, src, expected)
//}
//
//func TestCodeGen124(t *testing.T) {
//	src := `package main
//func test() int? { Some(42) }
//func main() {
//	num := test() or {
//		fmt.Println("result is None")
//		return
//	}
//	fmt.Println(num)
//}
//`
//	expected := `// agl:generated
//package main
//func test() Option[int] {
//	return MakeOptionSome(42)
//}
//func main() {
//	num := test()
//	if num.IsNone() {
//		fmt.Println("result is None")
//		return
//	}
//	fmt.Println(num)
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen125(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int? { Some(42) }
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen126(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int! { Ok(42) }
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Result[int] {
	return MakeResultOk(42)
}
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen127(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int! { Ok(42) }
func main() {
	isOk := test().IsOk()
	fmt.Println(isOk)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Result[int] {
	return MakeResultOk(42)
}
func main() {
	isOk := test().IsOk()
	fmt.Println(isOk)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen128(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int! { Ok(42) }
func main() {
	isErr := test().IsErr()
	fmt.Println(isErr)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Result[int] {
	return MakeResultOk(42)
}
func main() {
	isErr := test().IsErr()
	fmt.Println(isErr)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen129(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int? { Some(42) }
func main() {
	isSome := test().IsSome()
	fmt.Println(isSome)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	isSome := test().IsSome()
	fmt.Println(isSome)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen130(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int? { Some(42) }
func main() {
	isNone := test().IsNone()
	fmt.Println(isNone)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	isNone := test().IsNone()
	fmt.Println(isNone)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen131(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int? { Some(42) }
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen132(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test() int! { Ok(42) }
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	expected := `// agl:generated
package main
import "fmt"
func test() Result[int] {
	return MakeResultOk(42)
}
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen133(t *testing.T) {
	src := `package main
import "agl1/os"
import "agl1/strconv"
func test() ! {
	os.Chdir("")!
	return Ok(void)
}`
	expected := `// agl:generated
package main
import (
	"os"
	"strconv"
)
func test() Result[AglVoid] {
	if aglTmpErr1 := os.Chdir(""); aglTmpErr1 != nil {
		return MakeResultErr[AglVoid](aglTmpErr1)
	}
	AglNoop()
	return MakeResultOk(AglVoid{})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen134(t *testing.T) {
	src := `package main
import "agl1/strconv"
import "agl1/os"
func test() string? {
	res := os.LookupEnv("")?
	return Some(res)
}`
	expected := `// agl:generated
package main
import (
	"strconv"
	"os"
)
func test() Option[string] {
	aglTmp1, ok := os.LookupEnv("")
	if !ok {
		return MakeOptionNone[string]()
	}
	res := AglIdentity(aglTmp1)
	return MakeOptionSome(res)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen135(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/net/http"
func test() string! {
	res := http.Get("https://google.com")!
	fmt.Println(res)
	return Ok("done")
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"net/http"
)
func test() Result[string] {
	aglTmpVar1, aglTmpErr1 := http.Get("https://google.com")
	if aglTmpErr1 != nil {
		return MakeResultErr[string](aglTmpErr1)
	}
	res := AglIdentity(aglTmpVar1)
	fmt.Println(res)
	return MakeResultOk("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen136(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/net/http"
func main() {
	res := http.Get("https://google.com")!
	fmt.Println(res)
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"net/http"
)
func main() {
	aglTmp1, aglTmpErr1 := http.Get("https://google.com")
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	res := AglIdentity(aglTmp1)
	fmt.Println(res)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen136_1(t *testing.T) {
	src := `package main
import "agl1/fmt"
import myHttp "agl1/net/http"
func main() {
	res := myHttp.Get("https://google.com")!
	fmt.Println(res)
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	myHttp "net/http"
)
func main() {
	aglTmp1, aglTmpErr1 := myHttp.Get("https://google.com")
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	res := AglIdentity(aglTmp1)
	fmt.Println(res)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen137(t *testing.T) {
	src := `package main
func parseInt(s1 string) int? {
	return Some(42)
}
func inter(s2 string) int? {
	a := parseInt(s2)?
	return Some(42)
}
func main() {
	inter("hello")?
}`
	expected := `// agl:generated
package main
func parseInt(s1 string) Option[int] {
	return MakeOptionSome(42)
}
func inter(s2 string) Option[int] {
	aglTmp1 := parseInt(s2)
	if aglTmp1.IsNone() {
		return MakeOptionNone[int]()
	}
	a := aglTmp1.Unwrap()
	return MakeOptionSome(42)
}
func main() {
	inter("hello").Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen138(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/time"

func test() int? {
	Some(42)
}

func main() {
    for {
        test()
        fmt.Println("test")
        time.Sleep(1000000)
    }
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	for {
		test()
		fmt.Println("test")
		time.Sleep(1000000)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen139(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/time"

func test(i int) int? {
    if i >= 2 {
        return None
    }
    return Some(i)
}

func main() {
    var i int
    for {
        res := test(i) or_break
        fmt.Println("test", res)
        time.Sleep(1000000000)
        i++
    }
    fmt.Println("done")
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test(i int) Option[int] {
	if i >= 2 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(i)
}
func main() {
	var i int
	for {
		aglTmp1 := test(i)
		if aglTmp1.IsNone() {
			break
		}
		res := AglIdentity(aglTmp1).Unwrap()
		fmt.Println("test", res)
		time.Sleep(1000000000)
		i++
	}
	fmt.Println("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen140(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/time"

func test(i int) int! {
    if i >= 2 {
        return Err("error")
    }
    return Ok(i)
}

func main() {
    var i int
    for {
        res := test(i) or_break
        fmt.Println("test", res)
        time.Sleep(1000000000)
        i++
    }
    fmt.Println("done")
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test(i int) Result[int] {
	if i >= 2 {
		return MakeResultErr[int](Errors.New("error"))
	}
	return MakeResultOk(i)
}
func main() {
	var i int
	for {
		aglTmp1 := test(i)
		if aglTmp1.IsErr() {
			break
		}
		res := AglIdentity(aglTmp1).Unwrap()
		fmt.Println("test", res)
		time.Sleep(1000000000)
		i++
	}
	fmt.Println("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen141(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/time"

func test(i int) int! {
    if i >= 2 {
        return Err("error")
    }
    return Ok(i)
}

func main() {
    var i int
    for {
        res := test(i) or_continue
        fmt.Println("test", res)
        time.Sleep(1000000000)
        i++
    }
    fmt.Println("done")
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test(i int) Result[int] {
	if i >= 2 {
		return MakeResultErr[int](Errors.New("error"))
	}
	return MakeResultOk(i)
}
func main() {
	var i int
	for {
		aglTmp1 := test(i)
		if aglTmp1.IsErr() {
			continue
		}
		res := AglIdentity(aglTmp1).Unwrap()
		fmt.Println("test", res)
		time.Sleep(1000000000)
		i++
	}
	fmt.Println("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen142(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/time"

func test(i int) int? {
    if i >= 2 {
        return None
    }
    return Some(i)
}

func main() {
    var i int
    for {
        res := test(i) or_continue
        fmt.Println("test", res)
        time.Sleep(1000000000)
        i++
    }
    fmt.Println("done")
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test(i int) Option[int] {
	if i >= 2 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(i)
}
func main() {
	var i int
	for {
		aglTmp1 := test(i)
		if aglTmp1.IsNone() {
			continue
		}
		res := AglIdentity(aglTmp1).Unwrap()
		fmt.Println("test", res)
		time.Sleep(1000000000)
		i++
	}
	fmt.Println("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen143(t *testing.T) {
	src := `package main
func test() int? { Some(42) }
func main() {
    test() or_return
}`
	expected := `// agl:generated
package main
func test() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	aglTmp1 := test()
	if aglTmp1.IsNone() {
		return
	}
	AglIdentity(aglTmp1)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen144(t *testing.T) {
	src := `package main
func test() int? { Some(42) }
func test2() int? {
    num := test() or_return
	return Some(num)
}`
	expected := `// agl:generated
package main
func test() Option[int] {
	return MakeOptionSome(42)
}
func test2() Option[int] {
	aglTmp1 := test()
	if aglTmp1.IsNone() {
		return MakeOptionNone[int]()
	}
	num := AglIdentity(aglTmp1)
	return MakeOptionSome(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen145(t *testing.T) {
	src := `package main
func test() int! { Ok(42) }
func test2() int! {
    num := test() or_return
	return Ok(num)
}`
	expected := `// agl:generated
package main
func test() Result[int] {
	return MakeResultOk(42)
}
func test2() Result[int] {
	aglTmp1 := test()
	if aglTmp1.IsErr() {
		return MakeResultErr[int](aglTmp1.Err())
	}
	num := AglIdentity(aglTmp1)
	return MakeResultOk(num)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen146(t *testing.T) {
	src := `package main
func test() int! { Ok(42) }
func test2() int {
    num := test() or_return
	return num
}`
	tassert.PanicsWithError(t, "cannot use or_return in a function that does not return void/Option/Result", testCodeGenFn(src))
}

func TestCodeGen147(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) Even() []T {
   mut out := make([]T, len(v))
   for _, el := range v {
       if el % 2 == 0 {
           out = append(out, el)
       }
   }
   return out
}
func main() {
   arr := []int{1, 2, 3}
   fmt.Println(arr.Even())
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	arr := AglVec[int]{1, 2, 3}
	fmt.Println(AglVecEven_T_int(arr))
}
func AglVecEven_T_int(v AglVec[int]) AglVec[int] {
	out := make(AglVec[int], len(v))
	for _, el := range v {
		if el % 2 == 0 {
			out = append(out, el)
		}
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen147_1(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) Even() []T {
   mut out := make([]T, len(v))
   for _, el := range v {
       if el % 2 == 0 {
           out = append(out, el)
       }
   }
   return out
}
func main() {
   arr := []int{1, 2, 3}
   fmt.Println(arr.Even())
   arr1 := []int{4, 5, 6}
   fmt.Println(arr1.Even())
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	arr := AglVec[int]{1, 2, 3}
	fmt.Println(AglVecEven_T_int(arr))
	arr1 := AglVec[int]{4, 5, 6}
	fmt.Println(AglVecEven_T_int(arr1))
}
func AglVecEven_T_int(v AglVec[int]) AglVec[int] {
	out := make(AglVec[int], len(v))
	for _, el := range v {
		if el % 2 == 0 {
			out = append(out, el)
		}
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen148(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyMap[R any](clb func(T) R) []R {
	mut out := make([]R, len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func main() {
	arr := []int{1, 2, 3}
	fmt.Println(arr.MyMap(func(int) int {
		return 1
	}))
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	arr := AglVec[int]{1, 2, 3}
	fmt.Println(AglVecMyMap_R_int_T_int(arr, func(int) int {
		return 1
	}))
}
func AglVecMyMap_R_int_T_int(v AglVec[int], clb func(int) int) AglVec[int] {
	out := make(AglVec[int], len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen148_1(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyMap[R any](clb func(T) R) []R {
	mut out := make([]R, len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func main() {
	arr := []i64{1, 2, 3}
	r := arr.MyMap({ $0 + 1 })
	fmt.Println(r)
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	arr := AglVec[int64]{1, 2, 3}
	r := AglVecMyMap_R_int64_T_int64(arr, func(aglArg0 int64) int64 {
		return aglArg0 + 1
	})
	fmt.Println(r)
}
func AglVecMyMap_R_int64_T_int64(v AglVec[int64], clb func(int64) int64) AglVec[int64] {
	out := make(AglVec[int64], len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen148_2(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyMap[R any](clb func(T) R) []R {
	mut out := make([]R, len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func main() {
	arr := []i64{1, 2, 3}
	r := arr.MyMap({ u8($0) + 1 })
	fmt.Println(r)
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	arr := AglVec[int64]{1, 2, 3}
	r := AglVecMyMap_R_uint8_T_int64(arr, func(aglArg0 int64) uint8 {
		return uint8(aglArg0) + 1
	})
	fmt.Println(r)
}
func AglVecMyMap_R_uint8_T_int64(v AglVec[int64], clb func(int64) uint8) AglVec[uint8] {
	out := make(AglVec[uint8], len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen149(t *testing.T) {
	src := `package main
func (v agl1.Vec[T]) MyMap[R any](clb func(T) R) []R {
	mut out := make([]R, len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func main() {
	arr1 := []int{1, 2, 3}
	arr2 := []u8{1, 2, 3}
	arr1.MyMap(func(int) int { return 1 })
	arr2.MyMap(func(u8) u64 { return 1 })
}`
	expected := `// agl:generated
package main
func main() {
	arr1 := AglVec[int]{1, 2, 3}
	arr2 := AglVec[uint8]{1, 2, 3}
	AglVecMyMap_R_int_T_int(arr1, func(int) int {
		return 1
	})
	AglVecMyMap_R_uint64_T_uint8(arr2, func(uint8) uint64 {
		return 1
	})
}
func AglVecMyMap_R_int_T_int(v AglVec[int], clb func(int) int) AglVec[int] {
	out := make(AglVec[int], len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func AglVecMyMap_R_uint64_T_uint8(v AglVec[uint8], clb func(uint8) uint64) AglVec[uint64] {
	out := make(AglVec[uint64], len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen150(t *testing.T) {
	src := `package main
import "agl1/strings"
func (v agl1.Vec[string]) MyJoined(sep string) string {
	return strings.Join(v, sep)
}
func (v agl1.Vec[string]) MyJoined2() string {
	return strings.Join(v, ", ")
}
func (v agl1.Vec[string]) Test() {
}
func main() {
	arr := []string{"a", "b", "c"}
	arr.MyJoined(", ")
	arr.MyJoined2()
	arr.Test()
}`
	expected := `// agl:generated
package main
import "strings"
func main() {
	arr := AglVec[string]{"a", "b", "c"}
	AglVecMyJoined_T_string(arr, ", ")
	AglVecMyJoined2_T_string(arr)
	AglVecTest_T_string(arr)
}
func AglVecMyJoined_T_string(v AglVec[string], sep string) string {
	return strings.Join(v, sep)
}
func AglVecMyJoined2_T_string(v AglVec[string]) string {
	return strings.Join(v, ", ")
}
func AglVecTest_T_string(v AglVec[string]) {
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen151(t *testing.T) {
	src := `package main
func test() int? { Some(1) }
func main() {
	Loop:
    for {
		for {
			test() or_break Loop
		}
    }
}`
	expected := `// agl:generated
package main
func test() Option[int] {
	return MakeOptionSome(1)
}
func main() {
	Loop:
	for {
		for {
			aglTmp1 := test()
			if aglTmp1.IsNone() {
				break Loop
			}
			AglIdentity(aglTmp1).Unwrap()
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen152(t *testing.T) {
	src := `package main
func test() int? { Some(1) }
func main() {
	Loop:
    for {
		for {
			test() or_continue Loop
		}
    }
}`
	expected := `// agl:generated
package main
func test() Option[int] {
	return MakeOptionSome(1)
}
func main() {
	Loop:
	for {
		for {
			aglTmp1 := test()
			if aglTmp1.IsNone() {
				continue Loop
			}
			AglIdentity(aglTmp1).Unwrap()
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen153(t *testing.T) {
	src := `package main
import "agl1/strings"
func (v agl1.Vec[string]) MyJoined(sep string) string {
   return strings.Join(v, sep)
}
func main() {
   arr := []int{1, 2, 3}
	arr.MyJoined(":")
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "8:6: cannot use []int as []string for MyJoined")
}

func TestCodeGen154(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	var mut a u8
	a = arr.Reduce(0, { $0 + u8($1) })
}`
	expected := `// agl:generated
package main
func main() {
	arr := AglVec[int]{1, 2, 3}
	var a uint8
	a = AglVecReduce(arr, 0, func(aglArg0 uint8, aglArg1 int) uint8 {
		return aglArg0 + uint8(aglArg1)
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen155(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	var a u8
	a = arr.Reduce(u16(0), { $0 + u8($1) })
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "5:6: type mismatch, want: u8, got: u16")
}

func TestCodeGen156(t *testing.T) {
	src := `package main
func main() {
	defer func() {}()
}`
	expected := `// agl:generated
package main
func main() {
	defer func() {
	}()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen156_1(t *testing.T) {
	src := `package main
func test() ! { Err("error") }
func main() {
	defer test()!
}`
	tassert.PanicsWithError(t, "4:15: expression in defer must be function call", testCodeGenFn(src))
}

func TestCodeGen157(t *testing.T) {
	src := `package main
import "agl1/net/http"
func main() {
	r := http.Get("")!
	r.Body.Close()
}`
	expected := `// agl:generated
package main
import "net/http"
func main() {
	aglTmp1, aglTmpErr1 := http.Get("")
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	r := AglIdentity(aglTmp1)
	r.Body.Close()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen158(t *testing.T) {
	src := `package main
import "agl1/net/http"
func main() {
	r := http.Get("")!
	bod := r.Body
	v := bod.Close()!
}`
	test := NewTest(src)
	tassert.Contains(t, test.errs[0].Error(), "cannot assign void type to a variable")
}

func TestCodeGen159(t *testing.T) {
	src := `package main
import "agl1/net/http"
func main() {
	r := http.Get("")!
	v := r.Body.Close()!
}`
	test := NewTest(src)
	tassert.Contains(t, test.errs[0].Error(), "cannot assign void type to a variable")
}

func TestCodeGen160(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	r := arr.Filter({ $0 == 1 }).Map({ $0 }).Reduce(u8(0), { $0 + u8($1) })
}`
	expected := `// agl:generated
package main
func main() {
	arr := AglVec[int]{1, 2, 3}
	r := AglVecReduce(AglVecMap(AglVecFilter(arr, func(aglArg0 int) bool {
		return aglArg0 == 1
	}), func(aglArg0 int) int {
		return aglArg0
	}), uint8(0), func(aglArg0 uint8, aglArg1 int) uint8 {
		return aglArg0 + uint8(aglArg1)
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen160_1(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	var r u16
	r = arr.Filter({ $0 == 1 }).Map({ $0 }).Reduce(u8(0), { $0 + u8($1) })
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "5:6: type mismatch, want: u16, got: u8")
}

func TestCodeGen160_2(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	var r u16 = arr.Filter({ $0 == 1 }).Map({ $0 }).Reduce(u8(0), { $0 + u8($1) })
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:6: type mismatch, want: u16, got: u8")
}

func TestCodeGen161(t *testing.T) {
	src := `package main
type TestStruct[T any] struct {
	a T
}
func main() {
	i := TestStruct[string]{a: "foo"}
}`
	expected := `// agl:generated
package main
type TestStruct[T any] struct {
	a T
}
func main() {
	i := TestStruct[string]{a: "foo"}
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen162(t *testing.T) {
//	src := `package main
//type TestStruct[T any] struct {
//	a T
//}
//func testFn[T any](t *TestStruct[T]) {
//}
//func main() {
//	i := &TestStruct[string]{a: "foo"}
//	testFn(i)
//}`
//	expected := `// agl:generated
//package main
//type TestStruct_T_string struct {
//	a string
//}
//func testFn_T_string(t *TestStruct_T_string) {
//}
//func main() {
//	i := &TestStruct_T_string{a: "foo"}
//	testFn_T_string(i)
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen163(t *testing.T) {
//	src := `package main
//type TestStruct[T, U any] struct {
//	a T
//	b U
//}
//func testFn[T, U any](t *TestStruct[T, U]) {
//}
//func main() {
//	i := &TestStruct[string, int]{a: "foo", b: 42}
//	testFn(i)
//}`
//	expected := `// agl:generated
//package main
//type TestStruct[T, U any] struct {
//	a T
//	b U
//}
//func testFn[T, U any](t *TestStruct[T, U]) {
//}
//func main() {
//	i := &TestStruct[string, int]{a: "foo", b: 42}
//	testFn(i)
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen164(t *testing.T) {
	src := `package main
func test(t (int, bool)) (int, bool) { return t }
func main() {
	t1 := (int(1), true)
	t2 := (int(2), false)
	test(t1)
	test(t2)
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_bool struct {
	Arg0 int
	Arg1 bool
}
func test(t AglTupleStruct_int_bool) AglTupleStruct_int_bool {
	return t
}
func main() {
	t1 := AglTupleStruct_int_bool{Arg0: int(1), Arg1: true}
	t2 := AglTupleStruct_int_bool{Arg0: int(2), Arg1: false}
	test(t1)
	test(t2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen165(t *testing.T) {
	src := `package main
func main() {
	arr := [](int, bool){ (1, true), (2, false) }
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_bool struct {
	Arg0 int
	Arg1 bool
}
func main() {
	arr := AglVec[AglTupleStruct_int_bool]{AglTupleStruct_int_bool{Arg0: 1, Arg1: true}, AglTupleStruct_int_bool{Arg0: 2, Arg1: false}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen166(t *testing.T) {
	src := `package main
func test(t (int, bool)) (int, bool) { return t }
func main() {
	t1 := (1, true)
	t2 := (2, false)
	test(t1)
	test(t2)
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_bool struct {
	Arg0 int
	Arg1 bool
}
func test(t AglTupleStruct_int_bool) AglTupleStruct_int_bool {
	return t
}
func main() {
	t1 := AglTupleStruct_int_bool{Arg0: 1, Arg1: true}
	t2 := AglTupleStruct_int_bool{Arg0: 2, Arg1: false}
	test(t1)
	test(t2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen167(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test(t (int, bool)) (int, bool) {
    t.0 += 1
    return t
}
func main() {
    t1 := (1, true)
    t2 := test(t1)
    fmt.Println(t2)
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_int_bool struct {
	Arg0 int
	Arg1 bool
}
func test(t AglTupleStruct_int_bool) AglTupleStruct_int_bool {
	t.Arg0 += 1
	return t
}
func main() {
	t1 := AglTupleStruct_int_bool{Arg0: 1, Arg1: true}
	t2 := test(t1)
	fmt.Println(t2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen168(t *testing.T) {
	src := `package main
func main() {
	mut arr := []int{1, 2, 3}
	arr[1] = 42
}`
	expected := `// agl:generated
package main
func main() {
	arr := AglVec[int]{1, 2, 3}
	arr[1] = 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen169(t *testing.T) {
	src := `package main
func main() {
	mut m := map[string]int{"a": 1, "b": 2, "c": 3}
	m["a"] = 42
}`
	expected := `// agl:generated
package main
func main() {
	m := AglMap[string, int]{"a": 1, "b": 2, "c": 3}
	m["a"] = 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen170(t *testing.T) {
	src := `package main
func main() {
	m := make(map[string](int, int))
}`
	expected := `// agl:generated
package main
func main() {
	m := make(AglMap[string, AglTupleStruct_int_int])
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen171(t *testing.T) {
	src := `package main
func main() {
	t := (1, 2)
	m := map[string](int, int){"a": t}
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	t := AglTupleStruct_int_int{Arg0: 1, Arg1: 2}
	m := AglMap[string, AglTupleStruct_int_int]{"a": t}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen172(t *testing.T) {
	src := `package main
func main() {
	m := map[string](int, int){"a": (1, 2)}
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	m := AglMap[string, AglTupleStruct_int_int]{"a": AglTupleStruct_int_int{Arg0: 1, Arg1: 2}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen173(t *testing.T) {
	src := `package main
func main() {
	arr := [](int, int){(0, 0), (0, 1)}
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	arr := AglVec[AglTupleStruct_int_int]{AglTupleStruct_int_int{Arg0: 0, Arg1: 0}, AglTupleStruct_int_int{Arg0: 0, Arg1: 1}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen174(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test(t (int, bool)) (bool, int) {
    t.0 += 1
    return (t.1, t.0)
}
func main() {
    t1 := (1, true)
    mut t2 := test(t1)
    fmt.Println(t2)
    t2 = (false, 3)
    fmt.Println(t2)
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_bool_int struct {
	Arg0 bool
	Arg1 int
}
type AglTupleStruct_int_bool struct {
	Arg0 int
	Arg1 bool
}
func test(t AglTupleStruct_int_bool) AglTupleStruct_bool_int {
	t.Arg0 += 1
	return AglTupleStruct_bool_int{Arg0: t.Arg1, Arg1: t.Arg0}
}
func main() {
	t1 := AglTupleStruct_int_bool{Arg0: 1, Arg1: true}
	t2 := test(t1)
	fmt.Println(t2)
	t2 = AglTupleStruct_bool_int{Arg0: false, Arg1: 3}
	fmt.Println(t2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen175(t *testing.T) {
	src := `package main
type MyFloat64 f64
func main() {
	a := MyFloat64(1)
}`
	expected := `// agl:generated
package main
type MyFloat64 float64
func main() {
	a := MyFloat64(1)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen176(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/math"
)
type Abser interface {
	Abs() f64
}
func main() {
	var mut a Abser
	f := MyFloat(-math.Sqrt2)
	v := Vertex{3, 4}
	a = f  // a MyFloat implements Abser
	a = &v // a *Vertex implements Abser
	fmt.Println(a.Abs())
}
type MyFloat f64
func (f MyFloat) Abs() f64 {
	if f < 0 {
		return f64(-f)
	}
	return f64(f)
}
type Vertex struct {
	X, Y f64
}
func (v *Vertex) Abs() f64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"math"
)
type Abser interface {
	Abs() float64
}
func main() {
	var a Abser
	f := MyFloat(-math.Sqrt2)
	v := Vertex{3, 4}
	a = f
	a = &v
	fmt.Println(a.Abs())
}
type MyFloat float64
func (f MyFloat) Abs() float64 {
	if f < 0 {
		return float64(-f)
	}
	return float64(f)
}
type Vertex struct {
	X, Y float64
}
func (v *Vertex) Abs() float64 {
	return math.Sqrt(v.X * v.X + v.Y * v.Y)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen177(t *testing.T) {
	src := `package main
import "agl1/fmt"
type I interface {
	M()
}
type T struct {
	S string
}
func (t T) M() {
	fmt.Println(t.S)
}
func main() {
	var i I = T{"hello"}
	i.M()
}`
	expected := `// agl:generated
package main
import "fmt"
type I interface {
	M()
}
type T struct {
	S string
}
func (t T) M() {
	fmt.Println(t.S)
}
func main() {
	var i I = T{"hello"}
	i.M()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen178(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/math"
)
type I interface {
	M()
}
type T struct {
	S string
}
func (t *T) M() {
	fmt.Println(t.S)
}
type F f64
func (f F) M() {
	fmt.Println(f)
}
func main() {
	var mut i I
	i = &T{"Hello"}
	describe(i)
	i.M()
	i = F(math.Pi)
	describe(i)
	i.M()
}
func describe(i I) {
	fmt.Printf("(%v, %T)\n", i, i)
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"math"
)
type I interface {
	M()
}
type T struct {
	S string
}
func (t *T) M() {
	fmt.Println(t.S)
}
type F float64
func (f F) M() {
	fmt.Println(f)
}
func main() {
	var i I
	i = &T{"Hello"}
	describe(i)
	i.M()
	i = F(math.Pi)
	describe(i)
	i.M()
}
func describe(i I) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen179(t *testing.T) {
	src := `package main
import "agl1/fmt"
type I interface {
	M()
}
type T struct {
	S string
}
func (t *T) M() {
	if t == nil {
		fmt.Println("<nil>")
		return
	}
	fmt.Println(t.S)
}
func main() {
	var mut i I
	var t *T
	i = t
	describe(i)
	i.M()
	i = &T{"hello"}
	describe(i)
	i.M()
}
func describe(i I) {
	fmt.Printf("(%v, %T)\n", i, i)
}`
	expected := `// agl:generated
package main
import "fmt"
type I interface {
	M()
}
type T struct {
	S string
}
func (t *T) M() {
	if t == nil {
		fmt.Println("<nil>")
		return
	}
	fmt.Println(t.S)
}
func main() {
	var i I
	var t *T
	i = t
	describe(i)
	i.M()
	i = &T{"hello"}
	describe(i)
	i.M()
}
func describe(i I) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen180(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	var mut i any
	describe(i)
	i = 42
	describe(i)
	i = "hello"
	describe(i)
}
func describe(i any) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	var i any
	describe(i)
	i = 42
	describe(i)
	i = "hello"
	describe(i)
}
func describe(i any) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen181(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	var i any = "hello"
	s := i.(string)
	fmt.Println(s)
	f := i.(f64)
	fmt.Println(f)
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	var i any = "hello"
	s := i.(string)
	fmt.Println(s)
	f := i.(float64)
	fmt.Println(f)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen182(t *testing.T) {
	src := `package main
import "agl1/fmt"
func do(i any) {
	switch v := i.(type) {
	case int:
		fmt.Printf("Twice %v is %v\n", v, v*2)
	case string:
		fmt.Printf("%q is %v bytes long\n", v, len(v))
	default:
		fmt.Printf("I don't know about type %T!\n", v)
	}
}
func main() {
	do(21)
	do("hello")
	do(true)
}`
	expected := `// agl:generated
package main
import "fmt"
func do(i any) {
	switch v := i.(type) {
	case int:
		fmt.Printf("Twice %v is %v\n", v, v * 2)
	case string:
		fmt.Printf("%q is %v bytes long\n", v, len(v))
	default:
		fmt.Printf("I don't know about type %T!\n", v)
	}
}
func main() {
	do(21)
	do("hello")
	do(true)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen183(t *testing.T) {
	src := `package main
import "agl1/fmt"
type IPAddr [4]byte
func main() {
	hosts := map[string]IPAddr{
		"loopback":  IPAddr{127, 0, 0, 1},
		"googleDNS": IPAddr{8, 8, 8, 8},
	}
	for name, ip := range hosts {
		fmt.Printf("%v: %v\n", name, ip)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
type IPAddr AglVec[byte]
func main() {
	hosts := AglMap[string, IPAddr]{"loopback": IPAddr{127, 0, 0, 1}, "googleDNS": IPAddr{8, 8, 8, 8}}
	for name, ip := range hosts {
		fmt.Printf("%v: %v\n", name, ip)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen184(t *testing.T) {
	src := `package main
import "agl1/fmt"
type IPAddr [4]byte
func main() {
	hosts := map[string]IPAddr{
		"loopback":  {127, 0, 0, 1},
		"googleDNS": {8, 8, 8, 8},
	}
	for name, ip := range hosts {
		fmt.Printf("%v: %v\n", name, ip)
	}
}
`
	expected := `// agl:generated
package main
import "fmt"
type IPAddr AglVec[byte]
func main() {
	hosts := AglMap[string, IPAddr]{"loopback": {127, 0, 0, 1}, "googleDNS": {8, 8, 8, 8}}
	for name, ip := range hosts {
		fmt.Printf("%v: %v\n", name, ip)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen185(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/time"
)
type MyError struct {
	When time.Time
	What string
}
func (e *MyError) Error() string {
	return fmt.Sprintf("at %v, %s", e.When, e.What)
}
func run() ! {
	return Err(&MyError{time.Now(), "it didn't work"})
}
func main() {
    if Err(err) := run() {
		fmt.Println(err)
	}
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
type MyError struct {
	When time.Time
	What string
}
func (e *MyError) Error() string {
	return fmt.Sprintf("at %v, %s", e.When, e.What)
}
func run() Result[AglVoid] {
	return MakeResultErr[AglVoid](&MyError{time.Now(), "it didn't work"})
}
func main() {
	if aglTmp1 := run(); aglTmp1.IsErr() {
		err := aglTmp1.Err()
		fmt.Println(err)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen186(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/time"
)
type MyError struct {
	When time.Time
	What string
}
func (e *MyError) Error() string {
	return fmt.Sprintf("at %v, %s", e.When, e.What)
}
func run() ! {
	return Err(&MyError{time.Now(), "it didn't work"})
}
func main() {
	res := run()
    if Err(err) := res {
		fmt.Println(err)
	}
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
type MyError struct {
	When time.Time
	What string
}
func (e *MyError) Error() string {
	return fmt.Sprintf("at %v, %s", e.When, e.What)
}
func run() Result[AglVoid] {
	return MakeResultErr[AglVoid](&MyError{time.Now(), "it didn't work"})
}
func main() {
	res := run()
	if aglTmp1 := res; aglTmp1.IsErr() {
		err := aglTmp1.Err()
		fmt.Println(err)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen187(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/io"
	"agl1/strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make([]byte, 8)
	for {
		n := r.Read(b) or_break
		fmt.Printf("n = %v b = %v\n", n, b)
		fmt.Printf("b[:n] = %q\n", b[:n])
	}
}
`
	expected := `// agl:generated
package main
import (
	"fmt"
	"io"
	"strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make(AglVec[byte], 8)
	for {
		aglTmp1 := r.Read(b)
		if aglTmp1.IsErr() {
			break
		}
		n := AglIdentity(aglTmp1).Unwrap()
		fmt.Printf("n = %v b = %v\n", n, b)
		fmt.Printf("b[:n] = %q\n", b[:n])
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen188(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/io"
	"agl1/strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make([]byte, 8)
	for {
		match r.Read(b) {
		case Ok(n):
			_ = fmt.Printf("n = %v b = %v\n", n, b)
			_ = fmt.Printf("b[:n] = %q\n", b[:n])
		case Err(err):
			fmt.Printf("err = %v", err)
			if err == io.EOF {
				break
			}
		}
	}
}
`
	expected := `// agl:generated
package main
import (
	"fmt"
	"io"
	"strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make(AglVec[byte], 8)
	for {
		aglTmp1, aglTmpErr1 := AglWrapNative2(r.Read(b)).NativeUnwrap()
		if aglTmpErr1 == nil {
			n := *aglTmp1
			_ = AglWrapNative2(fmt.Printf("n = %v b = %v\n", n, b))
			_ = AglWrapNative2(fmt.Printf("b[:n] = %q\n", b[:n]))
		}
		if aglTmpErr1 != nil {
			err := aglTmpErr1
			fmt.Printf("err = %v", err)
			if err == io.EOF {
				break
			}
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen188_1(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/io"
	"agl1/strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make([]byte, 8)
	for {
		res := r.Read(b)
		match res {
		case Ok(n):
			_ = fmt.Printf("n = %v b = %v\n", n, b)
			_ = fmt.Printf("b[:n] = %q\n", b[:n])
		case Err(err):
			fmt.Printf("err = %v", err)
			if err == io.EOF {
				break
			}
		}
	}
}
`
	expected := `// agl:generated
package main
import (
	"fmt"
	"io"
	"strings"
)
func main() {
	r := strings.NewReader("Hello, Reader!")
	b := make(AglVec[byte], 8)
	for {
		res := AglWrapNative2(r.Read(b))
		aglTmp1 := res
		if aglTmp1.IsOk() {
			n := aglTmp1.Unwrap()
			_ = AglWrapNative2(fmt.Printf("n = %v b = %v\n", n, b))
			_ = AglWrapNative2(fmt.Printf("b[:n] = %q\n", b[:n]))
		}
		if aglTmp1.IsErr() {
			err := aglTmp1.Err()
			fmt.Printf("err = %v", err)
			if err == io.EOF {
				break
			}
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen189(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	v := Vertex{3, 4}
	v.Scale(10)
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "11:4: method 'Scale' cannot be called on immutable type 'Vertex'")
}

//func TestCodeGen189_1(t *testing.T) {
//	src := `package main
//type Vertex struct {
//	mut X, mut Y f64
//}
//func (mut v *Vertex) Scale(f f64) {
//	v.X = v.X * f
//	v.Y = v.Y * f
//}
//func main() {
//	v := &Vertex{3, 4}
//	mut vv := v
//	vv.Scale(10)
//}`
//	tassert.Contains(t, NewTest(src).errs[0].Error(), "11:2: cannot make mutable bind of an immutable variable")
//}

func TestCodeGen189_2(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := &Vertex{3, 4}
	mut vv := v
	vv.Scale(10)
}`
	tassert.NotPanics(t, testCodeGenFn(src))
}

//func TestCodeGen189_3(t *testing.T) {
//	src := `package main
//type Vertex struct {
//	mut X, mut Y f64
//}
//func (mut v *Vertex) Scale(f f64) {
//	v.X = v.X * f
//	v.Y = v.Y * f
//}
//func main() {
//	v := &Vertex{3, 4}
//	mut vv, vvv := v, v
//	vv.Scale(10)
//}`
//	tassert.Contains(t, NewTest(src).errs[0].Error(), "11:2: cannot make mutable bind of an immutable variable")
//}

func TestCodeGen189_4(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := &Vertex{3, 4}
	v.Scale(10)
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "7:4: assign to immutable prop 'Y'")
}

func TestCodeGen189_5(t *testing.T) {
	src := `package main
type Vertex struct {
	mut X, mut Y f64
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := &Vertex{3, 4}
	v.Scale(10)
}`
	expected := `// agl:generated
package main
type Vertex struct {
	X, Y float64
}
func (v *Vertex) Scale(f float64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	v := &Vertex{3, 4}
	v.Scale(10)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen189_6(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/math"
)
type Vertex struct {
	mut X, mut Y f64
}
func (v Vertex) Abs() f64 {
	return math.Sqrt(v.X*v.X + v.Y*v.Y)
}
func (mut v *Vertex) Scale(f f64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	mut v := Vertex{3, 4}
	v.Scale(10)
	fmt.Println(v.Abs())
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"math"
)
type Vertex struct {
	X, Y float64
}
func (v Vertex) Abs() float64 {
	return math.Sqrt(v.X * v.X + v.Y * v.Y)
}
func (v *Vertex) Scale(f float64) {
	v.X = v.X * f
	v.Y = v.Y * f
}
func main() {
	v := Vertex{3, 4}
	v.Scale(10)
	fmt.Println(v.Abs())
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen190(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	var mut i interface{}
	describe(i)
	i = 42
	describe(i)
	i = "hello"
	describe(i)
}
func describe(i interface{}) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	var i interface{}
	describe(i)
	i = 42
	describe(i)
	i = "hello"
	describe(i)
}
func describe(i any) {
	fmt.Printf("(%v, %T)\n", i, i)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen191(t *testing.T) {
	src := `package main
import "agl1/fmt"
func Index[T comparable](s []T, x T) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}
func main() {
	si := []int{10, 20, 15, -10}
	fmt.Println(Index(si, 15))
	ss := []string{"foo", "bar", "baz"}
	fmt.Println(Index(ss, "hello"))
}
`
	expected := `// agl:generated
package main
import "fmt"
func Index_T_int(s AglVec[int], x int) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}
func Index_T_string(s AglVec[string], x string) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}
func main() {
	si := AglVec[int]{10, 20, 15, -10}
	fmt.Println(Index_T_int(si, 15))
	ss := AglVec[string]{"foo", "bar", "baz"}
	fmt.Println(Index_T_string(ss, "hello"))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen192(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/time"
)
func say(s string) {
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		fmt.Println(s)
	}
}
func main() {
	go say("world")
	say("hello")
}
`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func say(s string) {
	for i := 0; i < 5; i++ {
		time.Sleep(100 * time.Millisecond)
		fmt.Println(s)
	}
}
func main() {
	go say("world")
	say("hello")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen193(t *testing.T) {
	src := `package main
import "agl1/fmt"
func sum(s []int, c chan int) {
	mut sum1 := 0
	for _, v := range s {
		sum1 += v
	}
	c <- sum1 // send sum to c
}
func main() {
	s := []int{7, 2, 8, -9, 4, 0}
	c := make(chan int)
	go sum(s[:len(s)/2], c)
	go sum(s[len(s)/2:], c)
	x, y := <-c, <-c // receive from c
	fmt.Println(x, y, x+y)
}`
	expected := `// agl:generated
package main
import "fmt"
func sum(s AglVec[int], c chan int) {
	sum1 := 0
	for _, v := range s {
		sum1 += v
	}
	c <- sum1
}
func main() {
	s := AglVec[int]{7, 2, 8, -9, 4, 0}
	c := make(chan int)
	go sum(s[:len(s) / 2], c)
	go sum(s[len(s) / 2:], c)
	x, y := <-c, <-c
	fmt.Println(x, y, x + y)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen194(t *testing.T) {
	src := `package main
import "agl1/fmt"
func fibonacci(c, quit chan int) {
	mut x, mut y := 0, 1
	for {
		select {
		case c <- x:
			x, y = y, x+y
		case <-quit:
			fmt.Println("quit")
			return
		}
	}
}
func main() {
	c := make(chan int)
	quit := make(chan int)
	go func() {
		for i := 0; i < 10; i++ {
			fmt.Println(<-c)
		}
		quit <- 0
	}()
	fibonacci(c, quit)
}`
	expected := `// agl:generated
package main
import "fmt"
func fibonacci(c, quit chan int) {
	x, y := 0, 1
	for {
		select {
		case c <- x:
		x, y = y, x + y
		case <-quit:
		fmt.Println("quit")
		return
		}
	}
}
func main() {
	c := make(chan int)
	quit := make(chan int)
	go func() {
		for i := 0; i < 10; i++ {
			fmt.Println(<-c)
		}
		quit <- 0
	}()
	fibonacci(c, quit)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen195(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/time"
)
func main() {
	start := time.Now()
	tick := time.Tick(100 * time.Millisecond)
	boom := time.After(500 * time.Millisecond)
	elapsed := func() time.Duration {
		return time.Since(start).Round(time.Millisecond)
	}
	for {
		select {
		case <-tick:
			fmt.Printf("[%6s] tick.\n", elapsed())
		case <-boom:
			fmt.Printf("[%6s] BOOM!\n", elapsed())
			return
		default:
			fmt.Printf("[%6s]     .\n", elapsed())
			time.Sleep(50 * time.Millisecond)
		}
	}
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func main() {
	start := time.Now()
	tick := time.Tick(100 * time.Millisecond)
	boom := time.After(500 * time.Millisecond)
	elapsed := func() time.Duration {
		return time.Since(start).Round(time.Millisecond)
	}
	for {
		select {
		case <-tick:
		fmt.Printf("[%6s] tick.\n", elapsed())
		case <-boom:
		fmt.Printf("[%6s] BOOM!\n", elapsed())
		return
		default:
		fmt.Printf("[%6s]     .\n", elapsed())
		time.Sleep(50 * time.Millisecond)
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen195_1(t *testing.T) {
	src := `package main
import "agl1/time"
func main() {
	var a time.Duration
	a.Round(time.Millisecond)
}`
	expected := `// agl:generated
package main
import "time"
func main() {
	var a time.Duration
	a.Round(time.Millisecond)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen196(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/sync"
	"agl1/time"
)
type SafeCounter struct {
	mu sync.Mutex
	v  map[string]int
}
func (c *SafeCounter) Inc(key string) {
	c.mu.Lock()
	c.v[key]++
	c.mu.Unlock()
}
func (c *SafeCounter) Value(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v[key]
}
func main() {
	c := SafeCounter{v: make(map[string]int)}
	for i := 0; i < 1000; i++ {
		go c.Inc("somekey")
	}
	time.Sleep(time.Second)
	fmt.Println(c.Value("somekey"))
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"sync"
	"time"
)
type SafeCounter struct {
	mu sync.Mutex
	v AglMap[string, int]
}
func (c *SafeCounter) Inc(key string) {
	c.mu.Lock()
	c.v[key]++
	c.mu.Unlock()
}
func (c *SafeCounter) Value(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v[key]
}
func main() {
	c := SafeCounter{v: make(AglMap[string, int])}
	for i := 0; i < 1000; i++ {
		go c.Inc("somekey")
	}
	time.Sleep(time.Second)
	fmt.Println(c.Value("somekey"))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen197(t *testing.T) {
	src := `package main
func main() {
	m := map[string]int{"a": 1}
	if el, ok := m["a"]; ok {
	}
	v2 := m["a"]
	m["a"]++
}`
	expected := `// agl:generated
package main
func main() {
	m := AglMap[string, int]{"a": 1}
	if el, ok := m["a"]; ok {
	}
	v2 := m["a"]
	m["a"]++
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen198(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
)
type Fetcher interface {
	Fetch(url string) (string, []string)!
}
func Crawl(url string, depth int, fetcher1 Fetcher) {
	if depth <= 0 {
		return
	}
	match fetcher1.Fetch(url) {
	    case Ok(res):
            body, urls := res
            fmt.Printf("found: %s %q\n", url, body)
            for _, u := range urls {
                Crawl(u, depth-1, fetcher1)
            }
            return
	    case Err(err):
            fmt.Println(err)
            return
	}
}
func main() {
	Crawl("https://golang.org/", 4, fetcher)
}
type fakeResult struct {
	body string
	urls []string
}
type fakeFetcher map[string]*fakeResult
func (f fakeFetcher) Fetch(url string) (string, []string)! {
	if res, ok := f[url]; ok {
		return Ok((res.body, res.urls))
	}
	return Err(fmt.Errorf("not found: %s", url))
}
var fetcher = fakeFetcher{
	"https://golang.org/": &fakeResult{
		"The Go Programming Language",
		[]string{
			"https://golang.org/pkg/",
			"https://golang.org/cmd/",
		},
	},
	"https://golang.org/pkg/": &fakeResult{
		"Packages",
		[]string{
			"https://golang.org/",
			"https://golang.org/cmd/",
			"https://golang.org/pkg/fmt/",
			"https://golang.org/pkg/os/",
		},
	},
	"https://golang.org/pkg/fmt/": &fakeResult{
		"Package fmt",
		[]string{
			"https://golang.org/",
			"https://golang.org/pkg/",
		},
	},
	"https://golang.org/pkg/os/": &fakeResult{
		"Package os",
		[]string{
			"https://golang.org/",
			"https://golang.org/pkg/",
		},
	},
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_string_AglVec_string_ struct {
	Arg0 string
	Arg1 AglVec[string]
}
type Fetcher interface {
	Fetch(string) Result[AglTupleStruct_string_AglVec_string_]
}
func Crawl(url string, depth int, fetcher1 Fetcher) {
	if depth <= 0 {
		return
	}
	aglTmp1 := fetcher1.Fetch(url)
	if aglTmp1.IsOk() {
		res := aglTmp1.Unwrap()
		aglVar2 := res
		body, urls := aglVar2.Arg0, aglVar2.Arg1
		fmt.Printf("found: %s %q\n", url, body)
		for _, u := range urls {
			Crawl(u, depth - 1, fetcher1)
		}
		return
	}
	if aglTmp1.IsErr() {
		err := aglTmp1.Err()
		fmt.Println(err)
		return
	}
}
func main() {
	Crawl("https://golang.org/", 4, fetcher)
}
type fakeResult struct {
	body string
	urls AglVec[string]
}
type fakeFetcher AglMap[string, *fakeResult]
func (f fakeFetcher) Fetch(url string) Result[AglTupleStruct_string_AglVec_string_] {
	if res, ok := f[url]; ok {
		return MakeResultOk(AglTupleStruct_string_AglVec_string_{Arg0: res.body, Arg1: res.urls})
	}
	return MakeResultErr[AglTupleStruct_string_AglVec_string_](fmt.Errorf("not found: %s", url))
}
var fetcher = fakeFetcher{"https://golang.org/": &fakeResult{"The Go Programming Language", AglVec[string]{"https://golang.org/pkg/", "https://golang.org/cmd/"}}, "https://golang.org/pkg/": &fakeResult{"Packages", AglVec[string]{"https://golang.org/", "https://golang.org/cmd/", "https://golang.org/pkg/fmt/", "https://golang.org/pkg/os/"}}, "https://golang.org/pkg/fmt/": &fakeResult{"Package fmt", AglVec[string]{"https://golang.org/", "https://golang.org/pkg/"}}, "https://golang.org/pkg/os/": &fakeResult{"Package os", AglVec[string]{"https://golang.org/", "https://golang.org/pkg/"}}}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen199(t *testing.T) {
	src := `package main
func main() {
	m := map[string]int{"a": 1}
	mv := m.Get("a")
}`
	expected := `// agl:generated
package main
func main() {
	m := AglMap[string, int]{"a": 1}
	mv := AglIdentity(AglMapIndex(m, "a"))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen200(t *testing.T) {
	src := `package main
func main() {
	a := func() int { return 42 }()
}`
	expected := `// agl:generated
package main
func main() {
	a := func() int {
		return 42
	}()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen201(t *testing.T) {
	src := `package main
var a = 42
func main() {
	a := 42
}`
	expected := `// agl:generated
package main
var a = 42
func main() {
	a := 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen202(t *testing.T) {
	src := `package main
import (
   "agl1/fmt"
   "agl1/net/http"
   "agl1/io"
)
func main() {
   req := http.NewRequest(http.MethodGet, "https://jsonip.com", None)!
   c := http.Client{}
   resp := c.Do(req)!
   defer resp.Body.Close()
   by := io.ReadAll(resp.Body)!
   fmt.Println(string(by))
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"net/http"
	"io"
)
func main() {
	req := AglHttpNewRequest(http.MethodGet, "https://jsonip.com", MakeOptionNone[io.Reader]()).Unwrap()
	c := http.Client{}
	aglTmp1, aglTmpErr1 := c.Do(req)
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	resp := AglIdentity(aglTmp1)
	defer resp.Body.Close()
	aglTmp2, aglTmpErr2 := io.ReadAll(resp.Body)
	if aglTmpErr2 != nil {
		panic(aglTmpErr2)
	}
	by := AglIdentity(aglTmp2)
	fmt.Println(string(by))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen203(t *testing.T) {
	src := `package main
import (
	"agl1/go/ast"
	"agl1/go/parser"
	"agl1/go/token"
	"agl1/go/types"
	"agl1/os"
	"agl1/path/filepath"
	"agl1/runtime"
)
func main() {
	goroot := runtime.GOROOT()
	fileName := "request.go"
	fnName := "NewRequest"
	filePath := filepath.Join(goroot, "src", "net", "http", fileName)
	src := os.ReadFile(filePath)!
	fset := token.NewFileSet()
	node := parser.ParseFile(fset, fileName, src, parser.AllErrors)!
	conf := types.Config{Importer: nil}
	info := &types.Info{Defs: make(map[*ast.Ident]types.Object)}
	_ = conf.Check("", fset, []*ast.File{node}, info)!
	for _, decl := range node.Decls {
		switch d := decl.(type) {
	 	case *ast.FuncDecl:
			if d.Name.Name == fnName && d.Recv == nil {
				var mut name string
				for _, param := range d.Type.Params.List {
					switch param1 := param.Type.(type) {
					case *ast.SelectorExpr:
						name = param1.X.(*ast.Ident).Name
					case *ast.Ident:
						name = param1.Name
					}
				}
	 		}
	 	}
	}
}
`
	expected := `// agl:generated
package main
import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"runtime"
)
func main() {
	goroot := runtime.GOROOT()
	fileName := "request.go"
	fnName := "NewRequest"
	filePath := filepath.Join(goroot, "src", "net", "http", fileName)
	aglTmp1, aglTmpErr1 := os.ReadFile(filePath)
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	src := AglIdentity(aglTmp1)
	fset := token.NewFileSet()
	aglTmp2, aglTmpErr2 := parser.ParseFile(fset, fileName, src, parser.AllErrors)
	if aglTmpErr2 != nil {
		panic(aglTmpErr2)
	}
	node := AglIdentity(aglTmp2)
	conf := types.Config{Importer: nil}
	info := &types.Info{Defs: make(AglMap[*ast.Ident, types.Object])}
	aglTmp3, aglTmpErr3 := conf.Check("", fset, AglVec[*ast.File]{node}, info)
	if aglTmpErr3 != nil {
		panic(aglTmpErr3)
	}
	_ = AglIdentity(aglTmp3)
	for _, decl := range node.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name.Name == fnName && d.Recv == nil {
				var name string
				for _, param := range d.Type.Params.List {
					switch param1 := param.Type.(type) {
					case *ast.SelectorExpr:
						name = param1.X.(*ast.Ident).Name
					case *ast.Ident:
						name = param1.Name
					}
				}
			}
		}
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen204(t *testing.T) {
	src := `package main
type Test struct {}
func (t Test) Method() {}
func main() {
	a := []Test{Test{}}
	b := a[0]
	b.Method()
}
`
	expected := `// agl:generated
package main
type Test struct{}
func (t Test) Method() {
}
func main() {
	a := AglVec[Test]{Test{}}
	b := a[0]
	b.Method()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen205(t *testing.T) {
	src := `package main
type Test struct {}
func (t Test) Method() []int {
	return []int{1, 2, 3}
}
func main() {
	a := Test{}
	b := a.Method()
	c := b[0]
}
`
	expected := `// agl:generated
package main
type Test struct{}
func (t Test) Method() AglVec[int] {
	return AglVec[int]{1, 2, 3}
}
func main() {
	a := Test{}
	b := a.Method()
	c := b[0]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen206(t *testing.T) {
	src := `package main
type Test struct {}
func (t Test) Method() []int {
	return []int{1, 2, 3}
}
func main() {
	a := Test{}
	b := a.Method()[0]
}
`
	expected := `// agl:generated
package main
type Test struct{}
func (t Test) Method() AglVec[int] {
	return AglVec[int]{1, 2, 3}
}
func main() {
	a := Test{}
	b := a.Method()[0]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen207(t *testing.T) {
	src := `package main
type Test struct {}
func (t Test) Method() {}
func main() {
	a := []Test{Test{}}
	a[0].Method()
}
`
	expected := `// agl:generated
package main
type Test struct{}
func (t Test) Method() {
}
func main() {
	a := AglVec[Test]{Test{}}
	a[0].Method()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen208(t *testing.T) {
	src := `package main
type Test struct {
	Name string
}
func test() int? {
	var a any = Test{Name: "foo"}
	tmp := a.(Test).Name == "foo"
	if tmp {
	}
	return Some(42)
}
`
	expected := `// agl:generated
package main
type Test struct {
	Name string
}
func test() Option[int] {
	var a any = Test{Name: "foo"}
	tmp := a.(Test).Name == "foo"
	if tmp {
	}
	return MakeOptionSome(42)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen209(t *testing.T) {
	src := `package main
type Test struct {
	Name string
}
func main() {
	var a any = Test{Name: "foo"}
	tmp := a.(Test).Name == "foo"
	if tmp {
	}
	return Some(42)
}
`
	expected := `// agl:generated
package main
type Test struct {
	Name string
}
func main() {
	var a any = Test{Name: "foo"}
	tmp := a.(Test).Name == "foo"
	if tmp {
	}
	return MakeOptionSome(42)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen210(t *testing.T) {
	src := `package main
type Test struct {
	Name string
}
func test() int? {
	var a any = Test{Name: "foo"}
	if a.(Test).Name == "foo" {
	}
	return Some(42)
}
`
	expected := `// agl:generated
package main
type Test struct {
	Name string
}
func test() Option[int] {
	var a any = Test{Name: "foo"}
	if a.(Test).Name == "foo" {
	}
	return MakeOptionSome(42)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen211(t *testing.T) {
	src := `package main
type Test struct {
	Name string
}
func main() {
	var a any = Test{Name: "foo"}
	if a.(Test).Name == "foo" {
	}
}
`
	expected := `// agl:generated
package main
type Test struct {
	Name string
}
func main() {
	var a any = Test{Name: "foo"}
	if a.(Test).Name == "foo" {
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen212(t *testing.T) {
	src := `package main
import "agl1/go/ast"
func main() {
	a := []*ast.Ident{&ast.Ident{Name: "foo"}}
	b := a.Map({ $0.Name }).Joined(", ")
}
`
	expected := `// agl:generated
package main
import "go/ast"
func main() {
	a := AglVec[*ast.Ident]{&ast.Ident{Name: "foo"}}
	b := AglVecJoined(AglVecMap(a, func(aglArg0 *ast.Ident) string {
		return aglArg0.Name
	}), ", ")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen213(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3}
	b := a.Last()
}
`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3}
	b := AglVecLast(a)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen214(t *testing.T) {
	src := `package main
import "agl1/fmt"
type Test struct {}
func main() {
	var a any = Test{}
	switch a := a.(type) {
		case Test:
			fmt.Println(a)
	}
}`
	expected := `// agl:generated
package main
import "fmt"
type Test struct{}
func main() {
	var a any = Test{}
	switch a := a.(type) {
	case Test:
		fmt.Println(a)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen215(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	if dump(1 == 1) {
		fmt.Println("test")
	}
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	aglTmp1 := 1 == 1
	fmt.Printf("4:10: %s: %v\n", "1 == 1", aglTmp1)
	if 1 == 1 {
		fmt.Println("test")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen216(t *testing.T) {
	src := `package main
func main() {
	mut a := []int{1, 2, 3}
	a.PopIf(func() bool { true })
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3}
	AglVecPopIf((*[]int)(&a), func() bool {
		return true
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen217(t *testing.T) {
	src := `package main
func main() {
	mut a := []int{1, 2, 3}
	a.PopIf({ true })
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3}
	AglVecPopIf((*[]int)(&a), func() bool {
		return true
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen218(t *testing.T) {
	src := `package main
func main() {
	mut a := []int{1, 2, 3}
	a.Push(4)
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[int]{1, 2, 3}
	AglVecPush((*[]int)(&a), 4)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen219(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	fmt.Println(@LINE, @COLUMN)
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	fmt.Println("4", "21")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen220(t *testing.T) {
//	src := `package main
//func main() {
//	s := agl1.NewSet()
//	fmt.Println(s)
//}`
//	expected := `// agl:generated
//package main
//func main() {
//	s := AglNewSet()
//	fmt.Println(s)
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen221(t *testing.T) {
//	src := `package main
//func main() {
//	s := agl1.NewSet()
//	fmt.Println(s.Len())
//}`
//	expected := `// agl:generated
//package main
//func main() {
//	s := AglNewSet()
//	fmt.Println(s.Len())
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen222(t *testing.T) {
//	src := `package main
//func main() {
//	s := agl1.NewSet("a")
//	s.Insert("b")
//}`
//	expected := `// agl:generated
//package main
//func main() {
//	s := AglNewSet("a")
//	s.Insert("b")
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen223(t *testing.T) {
	src := `package main
type Test struct {}
func main() {
	s := new(Test)
}`
	expected := `// agl:generated
package main
type Test struct{}
func main() {
	s := new(Test)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen224(t *testing.T) {
	src := `package main
func main() {
	a, _ := 1, 2
	_, a, b := 1, 2, 3
}`
	expected := `// agl:generated
package main
func main() {
	a, _ := 1, 2
	_, a, b := 1, 2, 3
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen225(t *testing.T) {
	src := `package main
func main() {
	a, _ := 1, 2
	_, a := 1, 2
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:2: No new variables on the left side of ':='")
}

func TestCodeGen226(t *testing.T) {
	src := `package main
func main() {
	a, b := 1, 2, 3
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "3:2: Assignment count mismatch: 2 = 3")
}

func TestCodeGen227(t *testing.T) {
	src := `package main
func main() {
	a, b := 1
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "3:2: Assignment count mismatch: 2 = 1")
}

func TestCodeGen228(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3}
	b, c := a[1]
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:2: Assignment count mismatch: 2 = 1")
}

func TestCodeGen229(t *testing.T) {
	src := `package main
type ITest interface {
	Test()
}
type Test struct {
}
func (t *Test) Test() {
}
func main() {
	var t ITest = Test{}
	t.Test()
}`
	expected := `// agl:generated
package main
type ITest interface {
	Test()
}
type Test struct{}
func (t *Test) Test() {
}
func main() {
	var t ITest = Test{}
	t.Test()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen230(t *testing.T) {
	src := `package main
import (
    "agl1/fmt"
    "agl1/net/http"
    "golang.org/x/net/html"
    "agl1/io"
)
func findTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
		return n.FirstChild.Data
	}
	for mut c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findTitle(c); title != "" {
			return title
		}
	}
	return ""
}
func main () {
    resp := http.Get("https://news.ycombinator.com")!
	doc := html.Parse(resp.Body)!
    title := findTitle(doc)
    fmt.Println("Title:", title)
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"net/http"
	"golang.org/x/net/html"
	"io"
)
func findTitle(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
		return n.FirstChild.Data
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if title := findTitle(c); title != "" {
			return title
		}
	}
	return ""
}
func main() {
	aglTmp1, aglTmpErr1 := http.Get("https://news.ycombinator.com")
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	resp := AglIdentity(aglTmp1)
	aglTmp2, aglTmpErr2 := html.Parse(resp.Body)
	if aglTmpErr2 != nil {
		panic(aglTmpErr2)
	}
	doc := AglIdentity(aglTmp2)
	title := findTitle(doc)
	fmt.Println("Title:", title)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen231(t *testing.T) {
	src := `package main

import "agl1/fmt"

func getInt() int! { Ok(42) }

func maybeInt() int? { Some(42) }

func main() {
    match getInt() {
    case Ok(num):
        fmt.Println("Num:", num)
    case Err(err):
        fmt.Println("Error:", err)
    }

    match maybeInt() {
    case Some(num):
        fmt.Println("Num:", num)
    case None:
        fmt.Println("No value")
    }
}`
	expected := `// agl:generated
package main
import "fmt"
func getInt() Result[int] {
	return MakeResultOk(42)
}
func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	aglTmp1 := getInt()
	if aglTmp1.IsOk() {
		num := aglTmp1.Unwrap()
		fmt.Println("Num:", num)
	}
	if aglTmp1.IsErr() {
		err := aglTmp1.Err()
		fmt.Println("Error:", err)
	}
	aglTmp2 := maybeInt()
	if aglTmp2.IsSome() {
		num := aglTmp2.Unwrap()
		fmt.Println("Num:", num)
	}
	if aglTmp2.IsNone() {
		fmt.Println("No value")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen232(t *testing.T) {
	src := `package main

import "agl1/fmt"
import "agl1/time"

func test(i int) int? {
    if i >= 2 {
        return None
    }
    return Some(i)
}

func main() {
    for i := 0; i < 10; i++ {
        res := test(i) or_break
        fmt.Println(res)
        time.Sleep(time.Second)
    }
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"time"
)
func test(i int) Option[int] {
	if i >= 2 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(i)
}
func main() {
	for i := 0; i < 10; i++ {
		aglTmp1 := test(i)
		if aglTmp1.IsNone() {
			break
		}
		res := AglIdentity(aglTmp1).Unwrap()
		fmt.Println(res)
		time.Sleep(time.Second)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen233(t *testing.T) {
	src := `package main
func main() {
	var mut a int
	if true {
		a = 1
	} else {
		a = 2
	}
}`
	expected := `// agl:generated
package main
func main() {
	var a int
	if true {
		a = 1
	} else {
		a = 2
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen234(t *testing.T) {
	src := `package main
func main() {
	var mut a int
	if true {
		a = 1
	} else {
		2
	}
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:2: if branches must have the same type `void` VS `UntypedNumType`")
}

func TestCodeGen235(t *testing.T) {
	src := `package main
func main() {
	a := [](u8, u8){(0, 0), (0, 1)}
}`
	expected := `// agl:generated
package main
type AglTupleStruct_uint8_uint8 struct {
	Arg0 uint8
	Arg1 uint8
}
func main() {
	a := AglVec[AglTupleStruct_uint8_uint8]{AglTupleStruct_uint8_uint8{Arg0: 0, Arg1: 0}, AglTupleStruct_uint8_uint8{Arg0: 0, Arg1: 1}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen236(t *testing.T) {
	src := `package main
func test(t (u8, u8)) {}
func main() {
	test((u8(1), int(2)))
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:15: type mismatch, want: u8, got: int")
}

func TestCodeGen237(t *testing.T) {
	src := `package main
func test(t (u8, u8)) {}
func main() {
	test((u8(1), u8(2)))
}`
	expected := `// agl:generated
package main
type AglTupleStruct_uint8_uint8 struct {
	Arg0 uint8
	Arg1 uint8
}
func test(t AglTupleStruct_uint8_uint8) {
}
func main() {
	test(AglTupleStruct_uint8_uint8{Arg0: uint8(1), Arg1: uint8(2)})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen239(t *testing.T) {
	src := `#!/usr/bin/env agl run
package main
import "agl1/fmt"
func main() {
	fmt.Println("Hello world!")
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	fmt.Println("Hello world!")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen240(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	arr := [](u8, u8){(0, 0)}
    fmt.Println(arr.Map({ $0.0 }))
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_uint8_uint8 struct {
	Arg0 uint8
	Arg1 uint8
}
func main() {
	arr := AglVec[AglTupleStruct_uint8_uint8]{AglTupleStruct_uint8_uint8{Arg0: 0, Arg1: 0}}
	fmt.Println(AglVecMap(arr, func(aglArg0 AglTupleStruct_uint8_uint8) uint8 {
		return aglArg0.Arg0
	}))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen241(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	arr := [](u8, u8){(0, 0)}
    fmt.Println(arr.Map(func(t (u8, u8)) u8 { t.0 }))
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_uint8_uint8 struct {
	Arg0 uint8
	Arg1 uint8
}
func main() {
	arr := AglVec[AglTupleStruct_uint8_uint8]{AglTupleStruct_uint8_uint8{Arg0: 0, Arg1: 0}}
	fmt.Println(AglVecMap(arr, func(t AglTupleStruct_uint8_uint8) uint8 {
		return t.Arg0
	}))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen242(t *testing.T) {
	src := `package main
import "agl1/fmt"
func test[T, U any](a []T, b []U) [](T, U) {
	return [](T, U){(a[0], b[0])}
}
func main() {
	test([]int{1}, []int{2})
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func test_T_int_U_int(a AglVec[int], b AglVec[int]) AglVec[AglTupleStruct_int_int] {
	return AglVec[AglTupleStruct_int_int]{AglTupleStruct_int_int{Arg0: a[0], Arg1: b[0]}}
}
func main() {
	test_T_int_U_int(AglVec[int]{1}, AglVec[int]{2})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen243(t *testing.T) {
	src := `package main
func main() {
	"".DoNotExists()
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "3:5: method 'DoNotExists' of type String does not exists")
}

func TestCodeGen244(t *testing.T) {
	src := `package main
func zip2[T, U any](a []T, b []U) [](T, U) {
	mut out := make([](T, U), 0)
	for i := range a {
		out.Push((a[i], b[i]))
	}
	return nil
}
func main() {
	zip2([]int{1}, []int{2}).Map({ $0.0 + $0.1 })
	zip2([]int{1}, []u8{2}).Map({ $0.0 + int($0.1) })
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
type AglTupleStruct_int_uint8 struct {
	Arg0 int
	Arg1 uint8
}
func zip2_T_int_U_int(a AglVec[int], b AglVec[int]) AglVec[AglTupleStruct_int_int] {
	out := make(AglVec[AglTupleStruct_int_int], 0)
	for i := range a {
		AglVecPush((*[]AglTupleStruct_int_int)(&out), AglTupleStruct_int_int{Arg0: a[i], Arg1: b[i]})
	}
	return nil
}
func zip2_T_int_U_uint8(a AglVec[int], b AglVec[uint8]) AglVec[AglTupleStruct_int_uint8] {
	out := make(AglVec[AglTupleStruct_int_uint8], 0)
	for i := range a {
		AglVecPush((*[]AglTupleStruct_int_uint8)(&out), AglTupleStruct_int_uint8{Arg0: a[i], Arg1: b[i]})
	}
	return nil
}
func main() {
	AglVecMap(zip2_T_int_U_int(AglVec[int]{1}, AglVec[int]{2}), func(aglArg0 AglTupleStruct_int_int) int {
		return aglArg0.Arg0 + aglArg0.Arg1
	})
	AglVecMap(zip2_T_int_U_uint8(AglVec[int]{1}, AglVec[uint8]{2}), func(aglArg0 AglTupleStruct_int_uint8) int {
		return aglArg0.Arg0 + int(aglArg0.Arg1)
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen245(t *testing.T) {
	src := `package main
func main() {
	arr := []string{"a", "b"}
	arr.Contains("a")
}`
	expected := `// agl:generated
package main
func main() {
	arr := AglVec[string]{"a", "b"}
	AglVecContains(arr, "a")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen246(t *testing.T) {
	src := `package main
func main() {
	[]string{"a", "b"}.Contains("a")
}`
	expected := `// agl:generated
package main
func main() {
	AglVecContains(AglVec[string]{"a", "b"}, "a")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen247(t *testing.T) {
	src := `package main
func main() {
	arr := make([](int, int), 0)
}`
	expected := `// agl:generated
package main
func main() {
	arr := make(AglVec[AglTupleStruct_int_int], 0)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen248(t *testing.T) {
	src := `package main
func main() {
	var arr [](int, int)
}`
	expected := `// agl:generated
package main
func main() {
	var arr AglVec[AglTupleStruct_int_int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen249(t *testing.T) {
	src := `package main
func main() {
	make(map[int]struct{})
}`
	expected := `// agl:generated
package main
func main() {
	make(AglMap[int, struct{}])
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen250(t *testing.T) {
	src := `package main
func main() {
	make(map[int]map[int]struct{})
}`
	expected := `// agl:generated
package main
func main() {
	make(AglMap[int, map[int]struct{}])
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen251(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyForEach(f func(T)) {
	for i := range v {
		f(v[i])
	}
}
func main() {
	[]int{1, 2}.MyForEach({ fmt.Println($0) })
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	AglVecMyForEach_T_int(AglVec[int]{1, 2}, func(aglArg0 int) AglVoid {
		fmt.Println(aglArg0)
		return AglVoid{}
	})
}
func AglVecMyForEach_T_int(v AglVec[int], f func(int)) {
	for i := range v {
		f(v[i])
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen252(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyCompactMap[R any](f func(T) R?) []R {
	mut out := make([]R, 0)
	for _, el := range v {
		if Some(res) := f(el) {
			out.Push(res)
		}
	}
	return out
}
func main() {
	[]string{"1", "two"}.MyCompactMap({ $0.Int() })
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	AglVecMyCompactMap_R_int_T_string(AglVec[string]{"1", "two"}, func(aglArg0 string) Option[int] {
		return AglStringInt(aglArg0)
	})
}
func AglVecMyCompactMap_R_int_T_string(v AglVec[string], f func(string) Option[int]) AglVec[int] {
	out := make(AglVec[int], 0)
	for _, el := range v {
		if aglTmp1 := f(el); aglTmp1.IsSome() {
			res := aglTmp1.Unwrap()
			AglVecPush((*[]int)(&out), res)
		}
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen253(t *testing.T) {
	src := `package main
import "agl1/fmt"
func (v agl1.Vec[T]) MyFlatMap[R any](f func(T) []R) []R {
	mut out := make([]R, 0)
	for _, el := range v {
		subArr := f(el)
		for _, el1 := range subArr {
			out.Push(el1)
		}
	}
	return out
}
func main() {
	[]int{1, 2}.FlatMap({
        mut out := make([]int, 0)
        for i := 0; i < $0; i++ {
            out.Push($0)
        }
        return out
    })
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	AglVecFlatMap_R_int_T_int(AglVec[int]{1, 2}, func(aglArg0 int) AglVec[int] {
		out := make(AglVec[int], 0)
		for i := 0; i < aglArg0; i++ {
			AglVecPush((*[]int)(&out), aglArg0)
		}
		return out
	})
}
func AglVecFlatMap_R_int_T_int(v AglVec[int], f func(int) AglVec[int]) AglVec[int] {
	out := make(AglVec[int], 0)
	for _, el := range v {
		subArr := f(el)
		for _, el1 := range subArr {
			AglVecPush((*[]int)(&out), el1)
		}
	}
	return out
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen254(t *testing.T) {
	src := `package main
func (v agl1.Vec[T]) MyMin() T? {
	if len(v) == 0 {
		return None
	}
	out := v[0]
	return Some(out)
}
func main() {
	[]int{1, 2}.MyMin()
}`
	expected := `// agl:generated
package main
func main() {
	AglVecMyMin_T_int(AglVec[int]{1, 2})
}
func AglVecMyMin_T_int(v AglVec[int]) Option[int] {
	if len(v) == 0 {
		return MakeOptionNone[int]()
	}
	out := v[0]
	return MakeOptionSome(out)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen255(t *testing.T) {
	src := `package main
import "agl1/fmt"
func main() {
	_ = fmt.Printf("")
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	_ = AglWrapNative2(fmt.Printf(""))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen256(t *testing.T) {
	src := `package main
type IpAddr enum {
    V4(u8, u8, u8, u8)
    V6(string)
}
func isPrivate(ip IpAddr) bool {
    match ip {
    case IpAddr.V4(a, b, _, _):
        return (a == 10) || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168)
    case IpAddr.V6(s):
        return s.HasPrefix("fc00::")
    }
    return true
}
func main() {
	home := IpAddr.V4(127, 0, 0, 1)
	isPrivate(home)
}`
	expected := `// agl:generated
package main
type IpAddrTag int
const (
	IpAddr_V4 IpAddrTag = iota
	IpAddr_V6
)
type IpAddr struct {
	tag IpAddrTag
	V4_0 uint8
	V4_1 uint8
	V4_2 uint8
	V4_3 uint8
	V6_0 string
}
func (v IpAddr) String() string {
	switch v.tag {
	case IpAddr_V4:
		return fmt.Sprintf("V4(%v, %v, %v, %v)", v.V4_0, v.V4_1, v.V4_2, v.V4_3)
	case IpAddr_V6:
		return fmt.Sprintf("V6(%v)", v.V6_0)
	default:
		panic("")
	}
}
func (v IpAddr) RawValue() int {
	return int(v.tag)
}
func Make_IpAddr_V4(arg0 uint8, arg1 uint8, arg2 uint8, arg3 uint8) IpAddr {
	return IpAddr{tag: IpAddr_V4, V4_0: arg0, V4_1: arg1, V4_2: arg2, V4_3: arg3}
}
func Make_IpAddr_V6(arg0 string) IpAddr {
	return IpAddr{tag: IpAddr_V6, V6_0: arg0}
}

func isPrivate(ip IpAddr) bool {
	if ip.tag == IpAddr_V4 {
		a := ip.V4_0
		b := ip.V4_1
		_ = ip.V4_2
		_ = ip.V4_3
		return (a == 10) || (a == 172 && b >= 16 && b <= 31) || (a == 192 && b == 168)
	}
	if ip.tag == IpAddr_V6 {
		s := ip.V6_0
		return AglStringHasPrefix(s, "fc00::")
	}
	return true
}
func main() {
	home := Make_IpAddr_V4(127, 0, 0, 1)
	isPrivate(home)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen257(t *testing.T) {
	src := `package main
func main() {
	[]int{1, 2}.Min()
}`
	expected := `// agl:generated
package main
func main() {
	AglVecMin_T_int(AglVec[int]{1, 2})
}
func AglVecMin_T_int(v AglVec[int]) Option[int] {
	if len(v) == 0 {
		return MakeOptionNone[int]()
	}
	out := v[0]
	for _, el := range v {
		out = min(out, el)
	}
	return MakeOptionSome(out)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen258(t *testing.T) {
	src := `package main
func main() {
	s := set[int]{1, 2, 3}
}`
	expected := `// agl:generated
package main
func main() {
	s := AglSet[int]{1: {}, 2: {}, 3: {}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen259(t *testing.T) {
	src := `package main
func main() {
	s1 := set[int]{1, 2, 3}
	s2 := set[int]{3, 4, 5}
	s3 := s1.Union(s2)
}`
	expected := `// agl:generated
package main
func main() {
	s1 := AglSet[int]{1: {}, 2: {}, 3: {}}
	s2 := AglSet[int]{3: {}, 4: {}, 5: {}}
	s3 := AglSetUnion(s1, s2)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen260(t *testing.T) {
	src := `package main
func main() {
	a := [][]int{{1, 2}, {2, 3}}
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[AglVec[int]]{{1, 2}, {2, 3}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen261(t *testing.T) {
	src := `package main
func main() {
	mut a := 42
}`
	expected := `// agl:generated
package main
func main() {
	a := 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen262(t *testing.T) {
	src := `package main
func main() {
	mut a := 42
	a = 43
}`
	expected := `// agl:generated
package main
func main() {
	a := 42
	a = 43
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen263(t *testing.T) {
	src := `package main
func main() {
	a := 42
	a = 43
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:2: cannot assign to immutable variable 'a'")
}

func TestCodeGen264(t *testing.T) {
	src := `package main
func test(a int) {
	a = 43
}
func main() {
	a := 42
	test(a)
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "3:2: cannot assign to immutable variable 'a'")
}

func TestCodeGen265(t *testing.T) {
	src := `package main
func test(mut a int) {
	a = 43
}
func main() {
	mut a := 42
	test(a)
}`
	expected := `// agl:generated
package main
func test(a int) {
	a = 43
}
func main() {
	a := 42
	test(a)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen266(t *testing.T) {
	src := `package main
func test(mut a int) {
	a = 43
}
func main() {
	a := 42
	test(a)
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "7:7: cannot use immutable 'a'")
}

func TestCodeGen267(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/strings"
func main() {
	var mut sb strings.Builder
	sb.WriteString("hello world")
	fmt.Println(sb.String())
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"strings"
)
func main() {
	var sb strings.Builder
	sb.WriteString("hello world")
	fmt.Println(sb.String())
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen268(t *testing.T) {
	src := `package main
import "agl1/fmt"
import "agl1/strings"
func main() {
	var sb strings.Builder
	sb.WriteString("hello world")
	fmt.Println(sb.String())
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "6:5: method 'WriteString' cannot be called on immutable type 'Builder'")
}

func TestCodeGen269(t *testing.T) {
	src := `package main
type A struct {}
type B struct {
	*A
}
func (a *A) AMethod() {}
func main() {
	b := B{}
	b.AMethod()
}`
	expected := `// agl:generated
package main
type A struct{}
type B struct {
	 *A
}
func (a *A) AMethod() {
}
func main() {
	b := B{}
	b.AMethod()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen270(t *testing.T) {
	src := `package main
var mut a int
func main() {
	a = 42
}`
	expected := `// agl:generated
package main
var a int
func main() {
	a = 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen271(t *testing.T) {
	src := `package main
func main() {
	m := make(map[int]set[int])
	m[1] = set[int]{1, 2, 3}
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:2: cannot assign to immutable variable 'm'")
}

func TestCodeGen272(t *testing.T) {
	src := `package main
func main() {
	mut m := make(map[int]set[int])
	m[1] = set[int]{1, 2, 3}
}`
	expected := `// agl:generated
package main
func main() {
	m := make(AglMap[int, AglSet[int]])
	m[1] = AglSet[int]{1: {}, 2: {}, 3: {}}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen273(t *testing.T) {
	src := `package main
func main() {
	mut m := make(map[int]set[int])
	m[1] = set[int]{1, 2, 3}
	mut s := m[1]
	if s == nil {
		s = make(set[int])
	}
}`
	expected := `// agl:generated
package main
func main() {
	m := make(AglMap[int, AglSet[int]])
	m[1] = AglSet[int]{1: {}, 2: {}, 3: {}}
	s := m[1]
	if s == nil {
		s = make(AglSet[int])
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen274(t *testing.T) {
	src := `package main
func main() {
	s := set[int]{1, 2, 3}
	s.Insert(4)
}`
	tassert.Contains(t, NewTest(src).errs[0].Error(), "4:4: method 'Insert' cannot be called on immutable type 'set'")
}

func TestCodeGen275(t *testing.T) {
	src := `package main
func main() {
	mut s := set[int]{1, 2, 3}
	s.Insert(4)
}`
	expected := `// agl:generated
package main
func main() {
	s := AglSet[int]{1: {}, 2: {}, 3: {}}
	AglSetInsert(s, 4)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen276(t *testing.T) {
	src := `package main
pub func test() {}
func main() {
	test()
}`
	expected := `// agl:generated
package main
func AglPub_test() {
}
func main() {
	AglPub_test()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen277(t *testing.T) {
	src := "package main\n" +
		"type Test struct {\n" +
		"	SomeProp int `json:\"some_prop\"`\n" +
		"}\n"
	expected := "// agl:generated\n" +
		"package main\n" +
		"type Test struct {\n" +
		"	SomeProp int `json:\"some_prop\"`\n" +
		"}\n"
	testCodeGen(t, src, expected)
}

func TestCodeGen278(t *testing.T) {
	src := `package main
pub func test(a []int) {
	a.Iter()
}
func main() {
	test([]int{1, 2})
}`
	expected := `// agl:generated
package main
func AglPub_test(a AglVec[int]) {
	AglVecIter(a)
}
func main() {
	AglPub_test(AglVec[int]{1, 2})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen279(t *testing.T) {
	src := `package main
import "agl1/os"
func main() {
	by, err := os.ReadFile("test.agl")
	if err != nil {
		panic(err)
	}
}`
	expected := `// agl:generated
package main
import "os"
func main() {
	by, err := os.ReadFile("test.agl")
	if err != nil {
		panic(err)
	}
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "[]byte", test.TypeAt(4, 2).String())
	tassert.Equal(t, "error", test.TypeAt(4, 6).String())
	tassert.Equal(t, expected, test.GenCode())
}

func TestCodeGen280(t *testing.T) {
	src := `package main
import "agl1/math"
func main() {
	i, f := math.Modf(3.14)
}`
	expected := `// agl:generated
package main
import "math"
func main() {
	i, f := math.Modf(3.14)
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "f64", test.TypeAt(4, 2).String())
	tassert.Equal(t, "f64", test.TypeAt(4, 5).String())
	tassert.Equal(t, expected, test.GenCode())
}

func TestCodeGen281(t *testing.T) {
	src := `package main
func main() {
	a := 1
	a = 2
}`
	expected := `// agl:generated
package main
func main() {
	a := 1
	a = 2
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, expected, test.GenCode())
}

func TestCodeGen282(t *testing.T) {
	src := `package main
import "os"
func main() {
	if err := os.WriteFile("test.txt", []byte("test"), 0644); err != nil {
	}
}`
	expected := `// agl:generated
package main
import "os"
func main() {
	if err := os.WriteFile("test.txt", AglVec[byte]("test"), 0644); err != nil {
	}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen283(t *testing.T) {
	src := `package main
import "agl1/errors"
type SomeErr struct {}
func (e *SomeErr) Error() string { return "" }
func main() {
	defer func() {
		if r := recover(); r != nil {
			var someErr *SomeErr
			if err, ok := r.(error); ok && errors.As(err, &someErr) {
			}
		}
	}()
}`
	expected := `// agl:generated
package main
import "errors"
type SomeErr struct{}
func (e *SomeErr) Error() string {
	return ""
}
func main() {
	defer func() {
		if r := recover(); r != nil {
			var someErr *SomeErr
			if err, ok := r.(error); ok && errors.As(err, &someErr) {
			}
		}
	}()
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen284(t *testing.T) {
	src := `package main
func test(m map[string]struct{}) {
}`
	expected := `// agl:generated
package main
func test(m map[string]struct{}) {
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen(t, src, expected)
}

func TestCodeGen285(t *testing.T) {
	src := `package main
import (
	stdFmt "fmt"
)
func main() {
	stdFmt.Println("")
}`
	expected := `// agl:generated
package main
import stdFmt "fmt"
func main() {
	stdFmt.Println("")
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen286(t *testing.T) {
	src := `package main
func main() {
	const test = "Hello world"
}`
	expected := `// agl:generated
package main
func main() {
	const test = "Hello world"
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "string", test.TypeAt(3, 8).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen287(t *testing.T) {
	src := `package main
func main() {
	const test1, test2 = "Hello", 42
}`
	expected := `// agl:generated
package main
func main() {
	const test1, test2 = "Hello", 42
}
`
	test := NewTest(src)
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "string", test.TypeAt(3, 8).String())
	tassert.Equal(t, "UntypedNumType", test.TypeAt(3, 15).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen288(t *testing.T) {
	src := `package main
import "os"
import "os/exec"
func main() {
	cmd := exec.Command("go", "mod")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}`
	expected := `// agl:generated
package main
import (
	"os"
	"os/exec"
)
func main() {
	cmd := exec.Command("go", "mod")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "*exec.Cmd", test.TypeAt(6, 2).String())
	tassert.Equal(t, "io.Writer", test.TypeAt(6, 6).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen289(t *testing.T) {
	src := `package main
import "path"
func main() {
	path := "Hello world"
}`
	expected := `// agl:generated
package main
import "path"
func main() {
	path := "Hello world"
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "string", test.TypeAt(4, 2).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen290(t *testing.T) {
	src := `package main
func main() {
	defer func() {
		test := "Hello world"
	}()
}`
	expected := `// agl:generated
package main
func main() {
	defer func() {
		test := "Hello world"
	}()
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "string", test.TypeAt(4, 3).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen291(t *testing.T) {
	src := `package main
import "agl1/os"
func main() {
	os.WriteFile("test.txt", []byte("test"), 0644)!
	os.WriteFile("test.txt", []byte("test"), 0644)!
}`
	expected := `// agl:generated
package main
import "os"
func main() {
	aglTmpErr1 := os.WriteFile("test.txt", AglVec[byte]("test"), 0644)
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	AglNoop()
	aglTmpErr2 := os.WriteFile("test.txt", AglVec[byte]("test"), 0644)
	if aglTmpErr2 != nil {
		panic(aglTmpErr2)
	}
	AglNoop()
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen292(t *testing.T) {
	src := `package main
import "agl1/os"
func test() ! {
	by := os.ReadFile("test.txt")!
	os.WriteFile("test.txt", []byte("test"), 0644)!
	return Ok(void)
}
func main() {
	test()!
}`
	expected := `// agl:generated
package main
import "os"
func test() Result[AglVoid] {
	aglTmpVar1, aglTmpErr1 := os.ReadFile("test.txt")
	if aglTmpErr1 != nil {
		return MakeResultErr[AglVoid](aglTmpErr1)
	}
	by := AglIdentity(aglTmpVar1)
	if aglTmpErr2 := os.WriteFile("test.txt", AglVec[byte]("test"), 0644); aglTmpErr2 != nil {
		return MakeResultErr[AglVoid](aglTmpErr2)
	}
	AglNoop()
	return MakeResultOk(AglVoid{})
}
func main() {
	test().Unwrap()
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen293(t *testing.T) {
	src := `package main
import "agl1/os"
import "agl1/fmt"
func main() {
	match os.ReadFile("test.txt") {
	case Ok(_):
		fmt.Println("no error")
	case Err(_):
		fmt.Println("error")
	}
}`
	expected := `// agl:generated
package main
import (
	"os"
	"fmt"
)
func main() {
	aglTmp1, aglTmpErr1 := AglWrapNative2(os.ReadFile("test.txt")).NativeUnwrap()
	if aglTmpErr1 == nil {
		_ = *aglTmp1
		fmt.Println("no error")
	}
	if aglTmpErr1 != nil {
		_ = aglTmpErr1
		fmt.Println("error")
	}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen294(t *testing.T) {
	src := `package main
import "agl1/os"
func test() ! {
	_ = os.ReadFile("test.txt")!
	return Ok(void)
}`
	expected := `// agl:generated
package main
import "os"
func test() Result[AglVoid] {
	aglTmpVar1, aglTmpErr1 := os.ReadFile("test.txt")
	if aglTmpErr1 != nil {
		return MakeResultErr[AglVoid](aglTmpErr1)
	}
	_ = AglIdentity(aglTmpVar1)
	return MakeResultOk(AglVoid{})
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen295(t *testing.T) {
	src := `package main
import "agl1/os"
func test() ! {
	os.ReadFile("test.txt")!
	return Ok(void)
}`
	expected := `// agl:generated
package main
import "os"
func test() Result[AglVoid] {
	aglTmpVar1, aglTmpErr1 := os.ReadFile("test.txt")
	if aglTmpErr1 != nil {
		return MakeResultErr[AglVoid](aglTmpErr1)
	}
	AglIdentity(aglTmpVar1)
	return MakeResultOk(AglVoid{})
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen296(t *testing.T) {
	src := `package main
func main() {
	words := []string{"foo", "bar", "baz"}
	var a []string
	a.Push(words...)
}`
	expected := `// agl:generated
package main
func main() {
	words := AglVec[string]{"foo", "bar", "baz"}
	var a AglVec[string]
	AglVecPush((*[]string)(&a), words...)
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen297(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	tuples := [](int, int){(0, 0), (1, 1)}
	t := tuples.Pop()
	fmt.Println(t)
}`
	expected := `// agl:generated
package main
import "fmt"
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	tuples := AglVec[AglTupleStruct_int_int]{AglTupleStruct_int_int{Arg0: 0, Arg1: 0}, AglTupleStruct_int_int{Arg0: 1, Arg1: 1}}
	t := AglVecPop((*[]AglTupleStruct_int_int)(&tuples))
	fmt.Println(t)
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen298(t *testing.T) {
	src := `package main
func main() {
	tuples := [](int, int){(0, 0), (1, 1)}
	tuples.Remove(0)
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	tuples := AglVec[AglTupleStruct_int_int]{AglTupleStruct_int_int{Arg0: 0, Arg1: 0}, AglTupleStruct_int_int{Arg0: 1, Arg1: 1}}
	AglVecRemove((*[]AglTupleStruct_int_int)(&tuples), 0)
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen299(t *testing.T) {
	src := `package main
import (
	"agl1/io"
	"net/http"
)
func main() {
	f1 := io.ReadFull
	f2 := http.Get
}`
	expected := `// agl:generated
package main
import (
	"io"
	"net/http"
)
func main() {
	f1 := io.ReadFull
	f2 := http.Get
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "func ReadFull(io.Reader, []byte) int!", test.TypeAt(7, 2).String())
	tassert.Equal(t, "func Get(string) (*http.Response, error)", test.TypeAt(8, 2).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen300(t *testing.T) {
	src := `package main
import (
	"agl1/os"
)
func main() {
	os.Remove()!
	_ = os.Remove()
}`
	expected := `// agl:generated
package main
import "os"
func main() {
	aglTmpErr1 := os.Remove()
	if aglTmpErr1 != nil {
		panic(aglTmpErr1)
	}
	AglNoop()
	_ = AglWrapNative1(os.Remove())
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen301(t *testing.T) {
	src := `package main
func main() {
	a := []byte("test")
	b := []byte("test")
	assert(a == b)
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[byte]("test")
	b := AglVec[byte]("test")
	AglAssert(AglBytesEqual(a, b), "assert failed line 5")
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen302(t *testing.T) {
	src := `package main
type HasLen interface {
	Len() int
}
func main() {
	a := []int{1, 2, 3}
	m := map[string]int{"a": 1, "b": 2, "c": 3}
	s := set[int]{1, 2, 3}
	res := []HasLen{a, m, s}
	assert(res.Len() == 3)
}`
	expected := `// agl:generated
package main
type HasLen interface {
	Len() int
}
func main() {
	a := AglVec[int]{1, 2, 3}
	m := AglMap[string, int]{"a": 1, "b": 2, "c": 3}
	s := AglSet[int]{1: {}, 2: {}, 3: {}}
	res := AglVec[HasLen]{a, m, s}
	AglAssert(AglVecLen(res) == 3, "assert failed line 10")
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen303(t *testing.T) {
	src := `package main
import "fmt"
func main() {
	mut m := make(map[int]map[int]struct{})
	m[1] = make(map[int]struct{})
	m[1][1] = struct{}{}
	fmt.Println(m)
}`
	expected := `// agl:generated
package main
import "fmt"
func main() {
	m := make(AglMap[int, map[int]struct{}])
	m[1] = make(AglMap[int, struct{}])
	m[1][1] = struct{}{}
	fmt.Println(m)
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "int", test.TypeAt(5, 4).String())
	tassert.Equal(t, "int", test.TypeAt(6, 4).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen304(t *testing.T) {
	src := `package main
func main() {
	for i, c := range "test" {
	}
	for k, v := range map[string]int{"a": 1, "b": 2, "c": 3} {
	}
}`
	expected := `// agl:generated
package main
func main() {
	for i, c := range "test" {
	}
	for k, v := range AglMap[string, int]{"a": 1, "b": 2, "c": 3} {
	}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "int", test.TypeAt(3, 6).String())
	tassert.Equal(t, "i32", test.TypeAt(3, 9).String())
	tassert.Equal(t, "string", test.TypeAt(5, 6).String())
	tassert.Equal(t, "int", test.TypeAt(5, 9).String())
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen305(t *testing.T) {
	src := `package main
func main() {
	mut m := map[int]set[int]{1: set[int]{1, 2}}
	m[1].Intersects([]int{1})
}`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "func (set[int]) Intersects(agl1.Iterator[int]) bool", test.TypeAt(4, 7).String())
}

func TestCodeGen306(t *testing.T) {
	src := `package main
func main() {
	mut a := []int{1, 2, 3}
	a.Insert(4)
}`
	test := NewTest(src, WithMutEnforced(true))
	tassert.Equal(t, 0, len(test.errs))
	tassert.Equal(t, "func (mut []int) Insert(int, int)", test.TypeAt(4, 4).String())
}

func TestCodeGen307(t *testing.T) {
	src := `package main
func main() {
	a := set[(int, int)]{}
}`
	expected := `// agl:generated
package main
type AglTupleStruct_int_int struct {
	Arg0 int
	Arg1 int
}
func main() {
	a := AglSet[AglTupleStruct_int_int]{}
}
`
	test := NewTest(src, WithMutEnforced(false))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

func TestCodeGen308(t *testing.T) {
	src := `package main
func main() {
	mut a := [][]int{{1, 2}, {3, 4}}
	a[0][0] = 42
}`
	expected := `// agl:generated
package main
func main() {
	a := AglVec[AglVec[int]]{{1, 2}, {3, 4}}
	a[0][0] = 42
}
`
	test := NewTest(src, WithMutEnforced(true))
	tassert.Equal(t, 0, len(test.errs))
	testCodeGen1(t, test.GenCode(), expected)
}

//func TestCodeGen283(t *testing.T) {
//	src := `package main
//import "agl1/os"
//func main() {
//	if err := os.WriteFile("test.txt", []byte("test"), 0644); err != nil {
//	}
//}`
//	expected := `// agl:generated
//package main
//import "fmt"
//func main() {
//	aglTmp1 := AglWrapNative2(os.WriteFile("test.txt", AglVec[byte]("test"), 0644))
//	if aglTmp1.IsErr() {
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen257(t *testing.T) {
//	src := `package main
//type IpAddr enum {
//    V4(u8, u8, u8, u8)
//    V6(string)
//}
//func main() {
//	home := IpAddr.V4(127, 0, 0, 1)
//    isV4 := match home {
//    case IpAddr.V4(a, b, _, _):
//        true
//    case IpAddr.V6(s):
//        false
//    }
//}`
//	expected := `// agl:generated
//package main
//type IpAddrTag int
//const (
//	IpAddr_V4 IpAddrTag = iota + 1
//	IpAddr_V6
//)
//type IpAddr struct {
//	tag IpAddrTag
//	V4_0 uint8
//	V4_1 uint8
//	V4_2 uint8
//	V4_3 uint8
//	V6_0 string
//}
//func (v IpAddr) String() string {
//	switch v.tag {
//	case IpAddr_V4:
//		return fmt.Sprintf("V4(%v, %v, %v, %v)", v.V4_0, v.V4_1, v.V4_2, v.V4_3)
//	case IpAddr_V6:
//		return fmt.Sprintf("V6(%v)", v.V6_0)
//	default:
//		panic("")
//	}
//}
//func Make_IpAddr_V4(arg0 uint8, arg1 uint8, arg2 uint8, arg3 uint8) IpAddr {
//	return IpAddr{tag: IpAddr_V4, V4_0: arg0, V4_1: arg1, V4_2: arg2, V4_3: arg3}
//}
//func Make_IpAddr_V6(arg0 string) IpAddr {
//	return IpAddr{tag: IpAddr_V6, V6_0: arg0}
//}
//
//func main() {
//	home := Make_IpAddr_V4(127, 0, 0, 1)
//	var isV4 bool
//	if home.tag == IpAddr_V4 {
//		a := home.V4_0
//		b := home.V4_1
//		_ = home.V4_2
//		_ = home.V4_3
//		isV4 = true
//	}
//	if home.tag == IpAddr_V6 {
//		s := home.V6_0
//		isV4 = false
//	}
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen218(t *testing.T) {
//	src := `package main
//func main() {
//	a := map[int]struct{}{1: {}, 2: {}, 3: {}}
//}`
//	expected := `// agl:generated
//package main
//func main() {
//	a := map[int]struct{}{1: {}, 2: {}, 3: {}}
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen200(t *testing.T) {
//	src := `package main
//import "agl1/fmt"
//type fakeFetcher map[string]*fakeResult
//type fakeResult struct {
//	body string
//}
//func (f fakeFetcher) Fetch() string! {
//	if res, ok := f["url"]; ok {
//		return Ok(res.body)
//	}
//	return Err(fmt.Errorf("not found"))
//}
//var fetcher = fakeFetcher{}`
//	expected := `// agl:generated
//package main
//import "fmt"
//type fakeFetcher map[string]*fakeResult
//type fakeResult struct {
//	body string
//}
//func (f fakeFetcher) Fetch() Result[string] {
//	if res, ok := f["url"]; ok {
//		return MakeResultOk(res.body)
//	}
//	return MakeResultErr[string](fmt.Errorf("not found"))
//}
//var fetcher = fakeFetcher{}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen167(t *testing.T) {
//	src := `package main
//func test(t (u8, bool)) (u8, bool) { return t }
//func main() {
//	t1 := (1, true)
//	t2 := (2, false)
//	test(t1)
//	test(t2)
//}`
//	expected := `// agl:generated
//package main
//func test(t AglTupleStruct_uint8_bool) AglTupleStruct_uint8_bool {
//	return t
//}
//type AglTupleStruct_uint8_bool struct {
//	Arg0 uint8
//	Arg1 bool
//}
//func main() {
//	t1 := AglTupleStruct_uint8_bool{Arg0: 1, Arg1: true}
//	t2 := AglTupleStruct_uint8_bool{Arg0: 2, Arg1: false}
//	test(t1)
//	test(t2)
//}
//`
//	testCodeGen(t, src, expected)
//}

//func TestCodeGen154(t *testing.T) {
//	src := `package main
//import "agl1/fmt"
//func (v agl1.Vec[T]) MyMap[R any](clb func(T) R) []R {
//	mut out := make([]R, len(v))
//	for _, el := range v {
//		out = append(out, clb(el))
//	}
//	return out
//}
//func main() {
//	arr := []int{1, 2, 3}
//	fmt.Println(arr.MyMap({ $0 + 1 }))
//}`
//	expected := `// agl:generated
//package main
//import "fmt"
//func main() {
//	arr := []int{1, 2, 3}
//	fmt.Println(AglVecMyMap_R_int_T_int(arr, func(aglArg0 int) int {
//		return aglArg0 + 1
//	}))
//}
//func AglVecMyMap_R_int_T_int(v []int, clb func(int) int) []int {
//	out := make([]int, len(v))
//	for _, el := range v {
//		out = append(out, clb(el))
//	}
//	return out
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen_Tmp(t *testing.T) {
	src := `
package main
func main() {
}
`
	tassert.NotPanics(t, testCodeGenFn(src))
}

func TestCodeGen_OsArgs(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/os"
)
func main() {
	if len(os.Args) > 1 {
		fmt.Println(os.Args[1])
	}
	for i, arg := range os.Args {
		fmt.Printf("Arg %d: %s\n", i, arg)
	}
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"os"
)
func main() {
	if len(os.Args) > 1 {
		fmt.Println(os.Args[1])
	}
	for i, arg := range os.Args {
		fmt.Printf("Arg %d: %s\n", i, arg)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_OsArgsWithResult(t *testing.T) {
	src := `package main
import (
	"agl1/os"
	"agl1/fmt"
)
func getFirstArg() string! {
	if len(os.Args) < 2 {
		return Err("no arguments provided")
	}
	return Ok(os.Args[1])
}
func main() {
	arg := getFirstArg()!
	fmt.Println(arg)
}`
	expected := `// agl:generated
package main
import (
	"os"
	"fmt"
)
func getFirstArg() Result[string] {
	if len(os.Args) < 2 {
		return MakeResultErr[string](Errors.New("no arguments provided"))
	}
	return MakeResultOk(os.Args[1])
}
func main() {
	arg := getFirstArg().Unwrap()
	fmt.Println(arg)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_WcExample(t *testing.T) {
	src := `package main
import (
	"agl1/fmt"
	"agl1/os"
	"agl1/strings"
)
func countLines(filename string) int! {
	data := os.ReadFile(filename)!
	content := string(data)
	lines := strings.Split(content, "\n")
	totalCount := lines.Map({ 1 }).Sum() - 1
	return Ok(totalCount)
}
func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: wc <filename>")
		return
	}
	filename := os.Args[1]
	match countLines(filename) {
	case Ok(total):
		fmt.Printf("%8d %s\n", total, filename)
	case Err(err):
		fmt.Printf("wc: %s: %s\n", filename, err)
	}
}`
	expected := `// agl:generated
package main
import (
	"fmt"
	"os"
	"strings"
)
func countLines(filename string) Result[int] {
	aglTmpVar1, aglTmpErr1 := os.ReadFile(filename)
	if aglTmpErr1 != nil {
		return MakeResultErr[int](aglTmpErr1)
	}
	data := AglIdentity(aglTmpVar1)
	content := string(data)
	lines := strings.Split(content, "\n")
	totalCount := AglVecSum(AglVecMap(lines, func(aglArg0 string) int {
		return 1
	})) - 1
	return MakeResultOk(totalCount)
}
func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: wc <filename>")
		return
	}
	filename := os.Args[1]
	aglTmp2 := countLines(filename)
	if aglTmp2.IsOk() {
		total := aglTmp2.Unwrap()
		fmt.Printf("%8d %s\n", total, filename)
	}
	if aglTmp2.IsErr() {
		err := aglTmp2.Err()
		fmt.Printf("wc: %s: %s\n", filename, err)
	}
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen_native_multi_values(t *testing.T) {
//	src := `package agl
//func test() (int, int) {
//	return 1, 2
//}
//`
//	expected := `// agl:generated
//`
//	testCodeGen(t, src, expected)
//}
