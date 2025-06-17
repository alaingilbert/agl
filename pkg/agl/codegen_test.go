package agl

import (
	"agl/pkg/ast"
	parser1 "agl/pkg/parser"
	"agl/pkg/token"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func parser2(src string) (*token.FileSet, *ast.File) {
	var fset = token.NewFileSet()
	f, err := parser1.ParseFile(fset, "", src, 0)
	if err != nil {
		panic(err)
	}
	return fset, f
}

func testCodeGen(t *testing.T, src, expected string) {
	fset, f := parser2(src)
	env := NewEnv(fset)
	i := NewInferrer(fset, env)
	i.InferFile(f)
	got := NewGenerator(i.Env, f).Generate()
	if got != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func testCodeGenFn(src string) func() {
	return func() {
		fset, f := parser2(src)
		env := NewEnv(fset)
		i := NewInferrer(fset, env)
		i.InferFile(f)
		NewGenerator(i.Env, f).Generate()
	}
}

func TestCodeGen1(t *testing.T) {
	src := `
package main
func add(a, b int) int {
	return a + b
}`
	expected := `package main
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
	expected := `package main
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
	expected := `package main
func add(a, b int64) Option[int64] {
	if a == 0 {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(a + b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen6(t *testing.T) {
	src := `package main
func add(a, b i64) i64! {
	if a == 0 {
		return Err(errors.New("a cannot be zero"))
	}
	return Ok(a + b)
}`
	expected := `package main
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
`
	expected := `package main
func mapFn[T any](a []T, f func(T) T) []T {
	return make([]T, 0)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen9_optionalReturnKeyword(t *testing.T) {
	src := `package main
func add(a, b int) int { a + b }
func add1(a, b int) int { return a + b }`
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
func main() {
	a := make([]int, 0)
	fmt.Println(a)
}`
	expected := `package main
func main() {
	a := make([]int, 0)
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
//	expected := `package main
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
func main() {
	for _, c := range "test" {
		fmt.Println(c)
	}
}`
	expected := `package main
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
func main() {
	if 2 % 2 == 0 {
		fmt.Println("test")
	}
}`
	expected := `package main
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
	expected := `package main
func main() {
	a := []int{1, 2, 3}
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
	expected := `package main
func findEvenNumber(arr []int) Option[int] {
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
	expected := `package main
func findEvenNumber(arr []int) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	tmp := findEvenNumber([]int{1, 2, 3, 4})
	fmt.Println(tmp.Unwrap())
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen21(t *testing.T) {
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
	fmt.Println(findEvenNumber([]int{1, 2, 3, 4}).Unwrap())
}`
	expected := `package main
func findEvenNumber(arr []int) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	fmt.Println(findEvenNumber([]int{1, 2, 3, 4}).Unwrap())
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
	expected := `package main
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
	expected := `package main
func findEvenNumber(arr []int) Option[int] {
	for _, num := range arr {
		if num % 2 == 0 {
			return MakeOptionSome(num)
		}
	}
	return MakeOptionNone[int]()
}
func main() {
	foundInt := findEvenNumber([]int{1, 2, 3, 4}).Unwrap()
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
	expected := `package main
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
func parseInt(s1 string) int! {
	return Err(errors.New("some error"))
}
func inter(s2 string) int! {
	a := parseInt(s2)!
	return Ok(a + 1)
}
func main() {
	inter("hello")!
}`
	expected := `package main
func parseInt(s1 string) Result[int] {
	return MakeResultErr[int](errors.New("some error"))
}
func inter(s2 string) Result[int] {
	res := parseInt(s2)
	if res.IsErr() {
		return res
	}
	a := res.Unwrap()
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
func add(a, b int) int {
	return a + b
}
func main() {
	fmt.Println(add(1, 2))
}`
	expected := `package main
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
func main() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
}`
	expected := `package main
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
import "strconv"
func parseInt(s string) int! {
	num := strconv.Atoi(s)!
	return Ok(num)
}`
	expected := `package main
import "strconv"
func parseInt(s string) Result[int] {
	tmp, err := strconv.Atoi(s)
	if err != nil {
		return MakeResultErr[int](err)
	}
	num := AglIdentity(tmp)
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
	expected := `package main
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
	expected := `package main
func main() {
	a := []int64{1, 2, 3, 4}
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
	expected := `package main
func main() {
	a := []int64{1, 2, 3, 4}
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
	expected := `package main
func main() {
	a := []int64{1}
	b := AglReduce(AglVecMap(AglVecFilter(a, func(aglArg0 int64) bool {
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
	expected := `package main
func main() {
	a1 := []int64{1}
	a2 := []uint8{1}
	b := AglReduce(AglVecMap(AglVecFilter(a1, func(aglArg0 int64) bool {
		return aglArg0 == 1
	}), func(aglArg0 int64) int64 {
		return aglArg0
	}), 0, func(aglArg0 int64, aglArg1 int64) int64 {
		return aglArg0 + aglArg1
	})
	c := AglReduce(AglVecMap(AglVecFilter(a2, func(aglArg0 uint8) bool {
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
	expected := `package main
func main() {
	a := []int64{1, 2, 3, 4}
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
	expected := `package main
func main() {
	a := []int64{1, 2, 3, 4}
	b := AglVecMap(a, func(aglArg0 int64) string {
		return "a"
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap2(t *testing.T) {
	src := `package main
func main() {
	a := []int{1, 2, 3, 4}
	b := a.Map(strconv.Itoa)
}
`
	expected := `package main
func main() {
	a := []int{1, 2, 3, 4}
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
	expected := `package main
func main() {
	a := []int64{1, 2, 3, 4}
	b := AglReduce(a, 0, func(aglArg0 int64, aglArg1 int64) int64 {
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
	expected := `package main
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
	expected := `package main
func main() {
	a := []int{1, 2, 3, 4}
	b := AglVecFilter(a, func(aglArg0 int) bool {
		return aglArg0 % 2 == 0
	})
	c := AglVecMap(b, func(aglArg0 int) int {
		return aglArg0 + 1
	})
	d := AglReduce(c, 0, func(aglArg0 int, aglArg1 int) int {
		return aglArg0 + aglArg1
	})
	AglAssert(d == 8, "assert failed line 7")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen35(t *testing.T) {
	src := `package main
func main() {
	by := os.ReadFile("test.txt")!
	fmt.Println(by)
}
`
	expected := `package main
func main() {
	aglTmp1, err := os.ReadFile("test.txt")
	if err != nil {
		panic(err)
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
	expected := `package main
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
	expected := `package main
type AglTupleStruct1 struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	res := AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
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
	expected := `package main
type AglTupleStruct1 struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	aglVar1 := AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
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
	expected := `package main
type AglTupleStruct1 struct {
	Arg0 uint8
	Arg1 string
	Arg2 bool
}
func testTuple() AglTupleStruct1 {
	return AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
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
	expected := `package main
type Person struct {
	name string
	age int
	ssn Option[string]
	nicknames Option[([]string)]
	testArray []string
	testArrayOfOpt []Option[string]
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
func main() {
	a := []uint8{1, 2, 3}
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
	expected := `package main
type Person struct {
}
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
	expected := `package main
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
	expected := `package main
type Person struct {
}
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
	expected := `package main
type Person struct {
}
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
	expected := `package main
type Person struct {
}
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
	expected := `package main
type ColorTag int
const (
	Color_red ColorTag = iota + 1
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
	expected := `package main
type ColorTag int
const (
	Color_red ColorTag = iota + 1
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
		return "other"
	default:
		panic("")
	}
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
	expected := `package main
type ColorTag int
const (
	Color_red ColorTag = iota + 1
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
		return "other"
	default:
		panic("")
	}
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
	expected := `package main
type ColorTag int
const (
	Color_red ColorTag = iota + 1
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
		return "other"
	default:
		panic("")
	}
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
	expected := `package main
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
func main() {
	a := 2
	fmt.Println("first")
	if a == 2 {
		fmt.Println("second")
	}
}
`
	expected := `package main
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
//	expected := `package main
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
	expected := `package main
func test() Result[int] {
	return MakeResultErr[int](errors.New("test"))
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
	expected := `package main
func test() Result[AglVoid] {
	return MakeResultErr[AglVoid](errors.New("test"))
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
	expected := `package main
func errFn() Result[AglVoid] {
	return MakeResultErr[AglVoid](errors.New("some error"))
}
func maybeInt() Option[int] {
	res := errFn()
	if res.IsErr() {
		return MakeOptionNone[int]()
	}
	res.Unwrap()
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
	expected := `package main
type Writer interface {
}
func main() {
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_TypeAssertion(t *testing.T) {
	src := `
package main

import "fmt"

type Writer interface {}

type WriterA struct {}

type WriterB struct {}

func test(w Writer) {
   if w.(WriterA).IsSome() {
       fmt.Println("A")
   }
}

func main() {
   w := WriterA{}
   test(w)
   fmt.Println("done")
}

`
	expected := `package main
import "fmt"
type Writer interface {
}
type WriterA struct {
}
type WriterB struct {
}
func test(w Writer) {
	if AglTypeAssert[WriterA](w).IsSome() {
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

import "os"
import "fmt"

func main() {
	os.WriteFile("test.txt", []byte("test"), 0755)!
}
`
	expected := `package main
import "os"
import "fmt"
func main() {
	err := os.WriteFile("test.txt", []byte("test"), 0755)
	if err != nil {
		panic(err)
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
//	expected := `package main
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
	expected := `package main
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
	write(p []byte) int!
}
`
	expected := `package main
type Writer interface {
	write(p []byte) Result[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen65(t *testing.T) {
	src := `package main
type Writer interface {
	write(p []byte) int!
	another() bool
}
`
	expected := `package main
type Writer interface {
	write(p []byte) Result[int]
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
//	expected := `package main
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
	expected := `package main
func main() {
	var a Option[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen66(t *testing.T) {
	src := `package main
type Color enum {
	red
	other(u8, string)
}
func main() {
	a, b := Color.other(1, "yellow")
	fmt.Println(a, b)
}
`
	expected := `package main
type ColorTag int
const (
	Color_red ColorTag = iota + 1
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
		return "other"
	default:
		panic("")
	}
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
func main() {
	a := []u8{1, 2, 3, 4, 5}
	var b u8 = a.Find({ $0 == 2 })?
	fmt.Println(b)
}
`
	expected := `package main
func main() {
	a := []uint8{1, 2, 3, 4, 5}
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
	expected := `package main
func test() []uint8 {
	return []uint8{1, 2, 3}
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
	expected := `package main
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
//	expected := `package main
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
	expected := `package main
func main() {
	a := []uint8{1, 2, 3, 4, 5}
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
	expected := `package main
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
	"fmt"
	"errors"
)
`
	expected := `package main
import "fmt"
import "errors"
`
	testCodeGen(t, src, expected)
}

func TestCodeGen88(t *testing.T) {
	src := `package main
type Pos struct {
	Row, Col int
}
`
	expected := `package main
type Pos struct {
	Row, Col int
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen89(t *testing.T) {
	src := `
package main
import "fmt"
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
	expected := `package main
import "fmt"
type Person struct {
	name string
}
func main() {
	p1 := Person{name: "John"}
	p2 := Person{name: "Jane"}
	arr := []Person{p1, p2}
	res := AglJoined(AglVecMap(arr, func(aglArg0 Person) string {
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
	expected := `package main
func main() {
	var arr []int
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
	expected := `package main
func main() {
	var arr1 []int
	var arr2 []int
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
	expected := `package main
func main() {
	var arr1, arr3 []int
	var arr2 []int
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
	expected := `package main
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
	expected := `package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := []Person{p1, p2}
	names := AglJoined(AglVecMap(people, func(el Person) string {
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
	expected := `package main
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
	people := []Person{p1, p2}
	names := AglJoined(AglVecMap(people, clb), ", ")
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
	expected := `package main
type Person struct {
	name string
	age int
}
func main() {
	p1 := Person{name: "John", age: 10}
	p2 := Person{name: "Jane", age: 20}
	people := []Person{p1, p2}
	names := AglJoined(AglVecMap(people, func(el Person) string {
		return el.name
	}), ", ")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen97(t *testing.T) {
	src := `package main
func main() {
	for i := 0; i < 10; i++ {
		fmt.Println(i)
	}
}
`
	expected := `package main
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
func main() {
	for {
		fmt.Println("hello")
	}
}
`
	expected := `package main
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
func testSome() int? {
	return Some(42)
}
func main() {
	if let Some(a) := testSome() {
		fmt.Println(a)
	}
}
`
	expected := `package main
func testSome() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	if tmp := testSome(); tmp.IsSome() {
		a := tmp.Unwrap()
		fmt.Println(a)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen100(t *testing.T) {
	src := `package main
func testOk() int! {
	return Ok(42)
}
func main() {
	if let Ok(a) := testOk() {
		fmt.Println(a)
	}
}
`
	expected := `package main
func testOk() Result[int] {
	return MakeResultOk(42)
}
func main() {
	if tmp := testOk(); tmp.IsOk() {
		a := tmp.Unwrap()
		fmt.Println(a)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen101(t *testing.T) {
	src := `package main
func testOk() int! {
	return Err("error")
}
func main() {
	if let Err(e) := testOk() {
		fmt.Println(e)
	}
}
`
	expected := `package main
func testOk() Result[int] {
	return MakeResultErr[int](errors.New("error"))
}
func main() {
	if tmp := testOk(); tmp.IsErr() {
		e := tmp.Err()
		fmt.Println(e)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen102(t *testing.T) {
	src := `package main
func testSome() int? {
   return Some(42)
}
func main() {
   if let Err(a) := testSome() {
       fmt.Println("test", a)
   }
}
`
	tassert.PanicsWithError(t, "6:15: try to destructure a non-Result type into an ResultType", testCodeGenFn(src))
}

func TestCodeGen103(t *testing.T) {
	src := `package main
func testResult() int! {
   return Ok(42)
}
func main() {
   if let Some(a) := testResult() {
       fmt.Println("test", a)
   }
}
`
	tassert.PanicsWithError(t, "6:16: try to destructure a non-Option type into an OptionType", testCodeGenFn(src))
}

func TestCodeGen104(t *testing.T) {
	src := `package main
func main() {
	a := 1
	a++
}
`
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	a := map[string]int{"a": 1}
	a["a"] = 2
}
`
	expected := `package main
func main() {
	a := map[string]int{"a": 1}
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
type IpAddrTag int
const (
	IpAddr_v4 IpAddrTag = iota + 1
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
		return "v4"
	case IpAddr_v6:
		return "v6"
	default:
		panic("")
	}
}
func Make_IpAddr_v4(arg0 uint8, arg1 uint8, arg2 uint8, arg3 uint8) IpAddr {
	return IpAddr{tag: IpAddr_v4, v4_0: arg0, v4_1: arg1, v4_2: arg2, v4_3: arg3}
}
func Make_IpAddr_v6(arg0 string) IpAddr {
	return IpAddr{tag: IpAddr_v6, v6_0: arg0}
}

type AglTupleStruct1 struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	addr1 := Make_IpAddr_v4(127, 0, 0, 1)
	aglVar1 := addr1
	a, b, c, d := aglVar1.v4_0, aglVar1.v4_1, aglVar1.v4_2, aglVar1.v4_3
	tuple := AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
	aglVar2 := tuple
	e, f, g := aglVar2.Arg0, aglVar2.Arg1, aglVar2.Arg2
	fmt.Println(a, b, c, d, e, f, g)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen113(t *testing.T) {
	src := `package main
func main() {
	a := 'a'
	fmt.Println(string(a))
}
`
	expected := `package main
func main() {
	a := 'a'
	fmt.Println(string(a))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen114(t *testing.T) {
	src := `package main
func main() {
	a := 1
	fmt.Println(u8(a))
}
`
	expected := `package main
func main() {
	a := 1
	fmt.Println(uint8(a))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen115(t *testing.T) {
	src := `package main
func main() {
	a := 1
	b := &a
	c := *b
	fmt.Println(a, b, c)
}
`
	expected := `package main
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
	tassert.PanicsWithError(t, "Unresolved reference 'MaybeSelf'", testCodeGenFn(src))
}

func TestCodeGen117(t *testing.T) {
	src := `package main
func main() {
	m := make(map[string]int)
}
`
	expected := `package main
func main() {
	m := make(map[string]int)
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
	expected := `package main
func test(m map[string]int) {
}
func main() {
	m := make(map[string]int)
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
	tassert.PanicsWithError(t, "types not equal, map[int]int map[string]int", testCodeGenFn(src))
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
	expected := `package main
func test(m map[string]int) {
}
func main() {
	a := map[string]int{"a": 1}
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
	tassert.PanicsWithError(t, "types not equal, []string []int", testCodeGenFn(src))
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
	expected := `package main
func test(a []int) {
}
func main() {
	a := []int{1, 2, 3}
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
//	expected := `package main
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
//	expected := `package main
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
func test() int? { Some(42) }
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	expected := `package main
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
func test() int! { Ok(42) }
func main() {
	num := test().UnwrapOr(1)
	fmt.Println(num)
}
`
	expected := `package main
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
func test() int! { Ok(42) }
func main() {
	isOk := test().IsOk()
	fmt.Println(isOk)
}
`
	expected := `package main
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
func test() int! { Ok(42) }
func main() {
	isErr := test().IsErr()
	fmt.Println(isErr)
}
`
	expected := `package main
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
func test() int? { Some(42) }
func main() {
	isSome := test().IsSome()
	fmt.Println(isSome)
}
`
	expected := `package main
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
func test() int? { Some(42) }
func main() {
	isNone := test().IsNone()
	fmt.Println(isNone)
}
`
	expected := `package main
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
func test() int? { Some(42) }
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	expected := `package main
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
func test() int! { Ok(42) }
func main() {
	num := test().Unwrap()
	fmt.Println(num)
}
`
	expected := `package main
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
import "strconv"
func test() ! {
	os.Chdir("")!
	return Ok(void)
}`
	expected := `package main
import "strconv"
func test() Result[AglVoid] {
	if err := os.Chdir(""); err != nil {
		return MakeResultErr[AglVoid](err)
	}
	AglNoop()
	return MakeResultOk(AglVoid)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen134(t *testing.T) {
	src := `package main
import "strconv"
func test() string? {
	res := os.LookupEnv("")?
	return Some(res)
}`
	expected := `package main
import "strconv"
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
import "fmt"
import "net/http"
func test() string! {
	res := http.Get("https://google.com")!
	fmt.Println(res)
	return Ok("done")
}`
	expected := `package main
import "fmt"
import "net/http"
func test() Result[string] {
	tmp, err := http.Get("https://google.com")
	if err != nil {
		return MakeResultErr[string](err)
	}
	res := AglIdentity(tmp)
	fmt.Println(res)
	return MakeResultOk("done")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen136(t *testing.T) {
	src := `package main
import "fmt"
import "net/http"
func main() {
	res := http.Get("https://google.com")!
	fmt.Println(res)
}`
	expected := `package main
import "fmt"
import "net/http"
func main() {
	aglTmp1, err := http.Get("https://google.com")
	if err != nil {
		panic(err)
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
	expected := `package main
func parseInt(s1 string) Option[int] {
	return MakeOptionSome(42)
}
func inter(s2 string) Option[int] {
	aglTmp1 := parseInt(s2)
	if aglTmp1.IsNone() {
		return aglTmp1
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
import "fmt"

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
	expected := `package main
import "fmt"
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
import "fmt"
import "time"

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
	expected := `package main
import "fmt"
import "time"
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
import "fmt"
import "time"

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
	expected := `package main
import "fmt"
import "time"
func test(i int) Result[int] {
	if i >= 2 {
		return MakeResultErr[int](errors.New("error"))
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
import "fmt"
import "time"

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
	expected := `package main
import "fmt"
import "time"
func test(i int) Result[int] {
	if i >= 2 {
		return MakeResultErr[int](errors.New("error"))
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
import "fmt"
import "time"

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
	expected := `package main
import "fmt"
import "time"
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
	expected := `package main
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
	expected := `package main
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
	expected := `package main
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
import "fmt"
func (v agl.Vec[T]) Even() []T {
    out := make([]T, len(v))
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
	expected := `package main
import "fmt"
func main() {
	arr := []int{1, 2, 3}
	fmt.Println(AglVecEven_T_int(arr))
}
func AglVecEven_T_int(v []int) []int {
	out := make([]int, len(v))
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
import "fmt"
func (v agl.Vec[T]) MyMap[R any](clb func(T) R) []R {
	out := make([]R, len(v))
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
	expected := `package main
import "fmt"
func main() {
	arr := []int{1, 2, 3}
	fmt.Println(AglVecMyMap_R_int_T_int(arr, func(int) int {
		return 1
	}))
}
func AglVecMyMap_R_int_T_int(v []int, clb func(int) int) []int {
	out := make([]int, len(v))
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
func (v agl.Vec[T]) MyMap[R any](clb func(T) R) []R {
	out := make([]R, len(v))
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
	expected := `package main
func main() {
	arr1 := []int{1, 2, 3}
	arr2 := []uint8{1, 2, 3}
	AglVecMyMap_R_int_T_int(arr1, func(int) int {
		return 1
	})
	AglVecMyMap_R_uint64_T_uint8(arr2, func(uint8) uint64 {
		return 1
	})
}
func AglVecMyMap_R_int_T_int(v []int, clb func(int) int) []int {
	out := make([]int, len(v))
	for _, el := range v {
		out = append(out, clb(el))
	}
	return out
}
func AglVecMyMap_R_uint64_T_uint8(v []uint8, clb func(uint8) uint64) []uint64 {
	out := make([]uint64, len(v))
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
func (v agl.Vec[string]) MyJoined(sep string) string {
	return strings.Join(v, sep)
}
func (v agl.Vec[string]) MyJoined2() string {
	return strings.Join(v, ", ")
}
func (v agl.Vec[string]) Test() {
}
func main() {
	arr := []string{"a", "b", "c"}
	arr.MyJoined(", ")
	arr.MyJoined2()
	arr.Test()
}`
	expected := `package main
func main() {
	arr := []string{"a", "b", "c"}
	AglVecMyJoined_T_string(arr, ", ")
	AglVecMyJoined2_T_string(arr)
	AglVecTest_T_string(arr)
}
func AglVecMyJoined_T_string(v []string, sep string) string {
	return strings.Join(v, sep)
}
func AglVecMyJoined2_T_string(v []string) string {
	return strings.Join(v, ", ")
}
func AglVecTest_T_string(v []string) {
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
	expected := `package main
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
	expected := `package main
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
func (v agl.Vec[string]) MyJoined(sep string) string {
    return strings.Join(v, sep)
}
func main() {
    arr := []int{1, 2, 3}
	arr.MyJoined(":")
}`
	tassert.PanicsWithError(t, "7:6: cannot use []int as []string for MyJoined", testCodeGenFn(src))
}

func TestCodeGen154(t *testing.T) {
	src := `package main
func main() {
	arr := []int{1, 2, 3}
	var a u8
	a = arr.Reduce(0, { $0 + u8($1) })
}`
	expected := `package main
func main() {
	arr := []int{1, 2, 3}
	var a uint8
	a = AglReduce(arr, 0, func(aglArg0 uint8, aglArg1 int) uint8 {
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
	tassert.PanicsWithError(t, "5:6: type mismatch, want: u8, got u16", testCodeGenFn(src))
}

func TestCodeGen156(t *testing.T) {
	src := `package main
func main() {
	defer func() {}()
}`
	expected := `package main
func main() {
	defer func() {
	}()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen157(t *testing.T) {
	src := `package main
func main() {
	r := http.Get("")!
	r.Body.Close()
}`
	expected := `package main
func main() {
	aglTmp1, err := http.Get("")
	if err != nil {
		panic(err)
	}
	r := AglIdentity(aglTmp1)
	r.Body.Close()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen158(t *testing.T) {
	src := `package main
func main() {
	r := http.Get("")!
	bod := r.Body
	v := bod.Close()
}`
	tassert.PanicsWithError(t, "cannot assign void type to a variable", testCodeGenFn(src))
}

func TestCodeGen159(t *testing.T) {
	src := `package main
func main() {
	r := http.Get("")!
	v := r.Body.Close()
}`
	tassert.PanicsWithError(t, "cannot assign void type to a variable", testCodeGenFn(src))
}

//func TestCodeGen154(t *testing.T) {
//	src := `package main
//import "fmt"
//func (v agl.Vec[T]) MyMap[R any](clb func(T) R) []R {
//	out := make([]R, len(v))
//	for _, el := range v {
//		out = append(out, clb(el))
//	}
//	return out
//}
//func main() {
//	arr := []int{1, 2, 3}
//	fmt.Println(arr.MyMap({ $0 + 1 }))
//}`
//	expected := `package main
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
