package main

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func testCodeGen(t *testing.T, src, expected string) {
	ts := NewTokenStream(src)
	a, e := infer(parser(ts))
	got := NewGenerator(a, e).Generate()
	if got != expected {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
	}
}

func testCodeGenFn(src string) func() {
	return func() {
		NewGenerator(infer(parser(NewTokenStream(src)))).Generate()
	}
}

func TestCodeGen1(t *testing.T) {
	src := `
fn add(a, b int) int {
	return a + b
}`
	expected := `func add(a, b int) int {
	return a + b
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen2(t *testing.T) {
	src := `
fn add(a, b i64) i64? {
	if a == 0 {
		return None
	}
	return Some(a + b)
}`
	expected := `func add(a, b int64) Option[int64] {
	if a == 0 {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(a + b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen4(t *testing.T) {
	src := `
fn add(a, b i64) Option[i64] {
	if a == 0 {
		return None
	}
	return Some(a + b)
}`
	expected := `func add(a, b int64) Option[int64] {
	if a == 0 {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(a + b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen6(t *testing.T) {
	src := `
fn add(a, b i64) i64! {
	if a == 0 {
		return Err(errors.New("a cannot be zero"))
	}
	return Ok(a + b)
}`
	expected := `func add(a, b int64) Result[int64] {
	if a == 0 {
		return MakeResultErr[int64](errors.New("a cannot be zero"))
	}
	return MakeResultOk(a + b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen5(t *testing.T) {
	src := `
fn add(a, b i64) Result[i64] {
	if a == 0 {
		return Err(errors.New("a cannot be zero"))
	}
	return Ok(a + b)
}`
	expected := `func add(a, b int64) Result[int64] {
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
	src := `
fn map[T any](a []T, f fn(T) T) []T {
	return make([]T, 0)
}
`
	expected := `func map[T any](a []T, f func(T) T) []T {
	return make([]T, 0)
}
`
	testCodeGen(t, src, expected)
}

//	func TestCodeGen7(t *testing.T) {
//		src := `
//
//	fn addOne(a i64) i64 {
//		return a + 1
//	}
//
//	fn map[T any](a []T, f fn(T) T) []T {
//		out := make([]T, len(a))
//		for e := range a {
//			out = append(out, f(e))
//		}
//		return out
//	}
//
//	fn main() {
//		someArr := []int{1, 2, 3}
//		map(someArr, addOne)
//		map(someArr, |el| { return el + 1 })
//		map(someArr, |el| { el + 1 })
//		map(someArr, { $0 + 1 })
//	}
//
// `
//
//		expected := `func addOne(a i64) int64 {
//		return a + 1
//	}
//
//	func map[T any](a []T, f func(T) T) []T {
//		out := make([]T, len(a))
//		for e := range a {
//			out = append(out, f(e))
//		}
//		return out
//	}
//
//	fn main() {
//		someArr := []int{1, 2, 3}
//		map(someArr, addOne)
//		map(someArr, func(el int) { return el + 1 })
//		map(someArr, func(el int) { return el + 1 })
//		map(someArr, func(el int) { return el + 1 })
//	}
//
// `
//
//		got := codegen(infer(parser(lexer(src))))
//		if got != expected {
//			t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//		}
//	}

func TestCodeGen9_optionalReturnKeyword(t *testing.T) {
	src := `
fn add(a, b int) int { a + b }
fn add1(a, b int) int { return a + b }`
	expected := `func add(a, b int) int {
	return a + b
}
func add1(a, b int) int {
	return a + b
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen10(t *testing.T) {
	src := `
fn f1(f fn() int) int { f() }
fn f2() int { 42 }
fn main() {
	f1(f2)
}`
	expected := `func f1(f func() int) int {
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
	src := `
fn f1(f fn() i64) i64 { f() }
fn main() {
	f1({ 42 })
}`
	expected := `func f1(f func() int64) int64 {
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

//func TestCodeGen12(t *testing.T) {
//	src := `
//fn f1(f fn(i64) i64) i64 { f(1) }
//fn main() {
//	f1(|a| { a + 1 })
//}`
//	expected := `func f1(f func(int64) int64) int64 {
//	return f(1)
//}
//func main() {
//	f1(func(a int64) int64 {
//		return a + 1
//	})
//}
//`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen13(t *testing.T) {
	src := `
fn f1(f fn(i64) i64) i64 { f(1) }
fn main() {
	f1({ $0 + 1 })
}`
	expected := `func f1(f func(int64) int64) int64 {
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
	src := `
fn main() {
	a := make([]int, 0)
	fmt.Println(a)
}`
	expected := `func main() {
	a := make([]int, 0)
	fmt.Println(a)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen15(t *testing.T) {
	src := `
fn main() {
	mut a := 42
	a = 43
	fmt.Println(a)
}`
	expected := `func main() {
	a := 42
	a = 43
	fmt.Println(a)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen16(t *testing.T) {
	src := `
fn main() {
	for _, c := range "test" {
		fmt.Println(c)
	}
}`
	expected := `func main() {
	for _, c := range "test" {
		fmt.Println(c)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen17(t *testing.T) {
	src := `
fn main() {
	if 2 % 2 == 0 {
		fmt.Println("test")
	}
}`
	expected := `func main() {
	if 2 % 2 == 0 {
		fmt.Println("test")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen18(t *testing.T) {
	src := `
fn main() {
	a := []int{1, 2, 3}
}`
	expected := `func main() {
	a := []int{1, 2, 3}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen19(t *testing.T) {
	src := `
fn findEvenNumber(arr []int) int? {
   for _, num := range arr {
       if num % 2 == 0 {
           return Some(num)
       }
   }
   return None
}`
	expected := `func findEvenNumber(arr []int) Option[int] {
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
	src := `
fn findEvenNumber(arr []int) int? {
   for _, num := range arr {
       if num % 2 == 0 {
           return Some(num)
       }
   }
   return None
}
fn main() {
	tmp := findEvenNumber([]int{1, 2, 3, 4})
	fmt.Println(tmp.unwrap())
}`
	expected := `func findEvenNumber(arr []int) Option[int] {
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

//func TestCodeGen21(t *testing.T) {
//	src := `
//fn findEvenNumber(arr []int) int? {
//   for _, num := range arr {
//       if num % 2 == 0 {
//           return Some(num)
//       }
//   }
//   return None
//}
//fn main() {
//	fmt.Println(findEvenNumber([]int{1, 2, 3, 4}).unwrap())
//}`
//	expected := `func findEvenNumber(arr []int) Option[int] {
//	for _, num := range arr {
//		if num % 2 == 0 {
//			return MakeOptionSome(num)
//		}
//	}
//	return MakeOptionNone[int]()
//}
//func main() {
//	fmt.Println(findEvenNumber([]int{1, 2, 3, 4}).Unwrap())
//}
//`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen22(t *testing.T) {
	src := `
fn main() {
	a := 1
	a++
}`
	expected := `func main() {
	a := 1
	a++
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen23(t *testing.T) {
	src := `
fn findEvenNumber(arr []int) int? {
   for _, num := range arr {
       if num % 2 == 0 {
           return Some(num)
       }
   }
   return None
}
fn main() {
	foundInt := findEvenNumber([]int{1, 2, 3, 4})?
}`
	expected := `func findEvenNumber(arr []int) Option[int] {
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
	src := `
fn parseInt(s string) int! {
	return Ok(42)
}
fn main() {
	parseInt("42")!
}`
	expected := `func parseInt(s string) Result[int] {
	return MakeResultOk(42)
}
func main() {
	parseInt("42").Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen25(t *testing.T) {
	src := `
fn parseInt(s1 string) int! {
	return Err(errors.New("some error"))
}
fn inter(s2 string) int! {
	a := parseInt(s2)!
	return Ok(a + 1)
}
fn main() {
	inter("hello")!
}`
	expected := `func parseInt(s1 string) Result[int] {
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
	src := `
fn add(a, b int) int {
	return a + b
}
fn main() {
	fmt.Println(add(1, 2))
}`
	expected := `func add(a, b int) int {
	return a + b
}
func main() {
	fmt.Println(add(1, 2))
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen27(t *testing.T) {
	src := `
fn main() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
}`
	expected := `func main() {
	fmt.Println("1")
	fmt.Println("2")
	fmt.Println("3")
	fmt.Println("4")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen28(t *testing.T) {
//	src := `package main
//import "strconv"
//fn parseInt(s string) int! {
//	num := strconv.Atoi(s)!
//	return Ok(num)
//}`
//	expected := `package main
//import "strconv"
//func parseInt(s string) Result[int] {
//	res, err := strconv.Atoi(s)
//	if err != nil {
//		return MakeResultErr[int](err)
//	}
//	num := res
//	return MakeResultOk(num)
//}
//`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

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
	src := `
fn main() {
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
	expected := `func main() {
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
	src := `
fn main() {
	a := []i64{1, 2, 3, 4}
	b := a.filter({ $0 % 2 == 0 })
}
`
	expected := `func main() {
	a := []int64{1, 2, 3, 4}
	b := AglVecFilter(a, func(aglArg0 int64) bool {
		return aglArg0 % 2 == 0
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInFilter2(t *testing.T) {
	src := `
fn main() {
	a := []i64{1, 2, 3, 4}
	b := a.filter({ $0 % 2 == 0 })
	c := b.map({ $0 + 1 })
}
`
	expected := `func main() {
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
	src := `
fn main() {
	a := []i64{1}
	b := a.filter({ $0 == 1 }).map({ $0 }).reduce(0, { $0 + $1 })
}
`
	expected := `func main() {
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
	src := `
fn main() {
	a1 := []i64{1}
	a2 := []u8{1}
	b := a1.filter({ $0 == 1 }).map({ $0 }).reduce(0, { $0 + $1 })
	c := a2.filter({ $0 == 1 }).map({ $0 }).reduce(0, { $0 + $1 })
}
`
	expected := `func main() {
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
	src := `
fn main() {
	a := []i64{1, 2, 3, 4}
	b := a.map({ $0 + 1 })
}
`
	expected := `func main() {
	a := []int64{1, 2, 3, 4}
	b := AglVecMap(a, func(aglArg0 int64) int64 {
		return aglArg0 + 1
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap3(t *testing.T) {
	src := `
fn main() {
	a := []i64{1, 2, 3, 4}
	b := a.map({ "a" })
}
`
	expected := `func main() {
	a := []int64{1, 2, 3, 4}
	b := AglVecMap(a, func(aglArg0 int64) string {
		return "a"
	})
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_VecBuiltInMap2(t *testing.T) {
	src := `
fn main() {
	a := []int{1, 2, 3, 4}
	b := a.map(strconv.Itoa)
}
`
	expected := `func main() {
	a := []int{1, 2, 3, 4}
	b := AglVecMap(a, strconv.Itoa)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen33_VecBuiltInReduce(t *testing.T) {
	src := `
fn main() {
	a := []i64{1, 2, 3, 4}
	b := a.reduce(0, { $0 + $1 })
	assert(b == 10, "b should be 10")
}
`
	expected := `func main() {
	a := []int64{1, 2, 3, 4}
	b := AglReduce(a, 0, func(aglArg0 int64, aglArg1 int64) int64 {
		return aglArg0 + aglArg1
	})
	AglAssert(b == 10, "assert failed 'b == 10' line 5" + " " + "b should be 10")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen34_Assert(t *testing.T) {
	src := `
fn main() {
	assert(1 != 2)
	assert(1 != 2, "1 should not be 2")
}
`
	expected := `func main() {
	AglAssert(1 != 2, "assert failed '1 != 2' line 3")
	AglAssert(1 != 2, "assert failed '1 != 2' line 4" + " " + "1 should not be 2")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen35(t *testing.T) {
//	src := "" +
//		"fn main() {\n" +
//		"\ta := 42\n" +
//		"\tfmt.Println(`${a}`)\n" +
//		"}\n"
//	expected := `func main() {
//	a := 42
//	fmt.Println(fmt.Sprintf("%d", a))
//}
//`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen_Reduce1(t *testing.T) {
	src := `
fn main() {
	a := []int{1, 2, 3, 4}
	b := a.filter({ $0 % 2 == 0 })
	c := b.map({ $0 + 1 })
	d := c.reduce(0, { $0 + $1 })
	assert(d == 8)
}
`
	expected := `func main() {
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
	AglAssert(d == 8, "assert failed 'd == 8' line 7")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen35(t *testing.T) {
	src := `
fn main() {
	by := os.ReadFile("test.txt")!
	fmt.Println(by)
}
`
	expected := `func main() {
	res, err := os.ReadFile("test.txt")
	if err != nil {
		panic(err)
	}
	by := AglIdentity(res)
	fmt.Println(by)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen36(t *testing.T) {
	src := `
fn testOption() int? {
	return None
}
fn main() {
	res := testOption()
	assert(res == None)
	assert(testOption() == None)
}
`
	expected := `func testOption() Option[int] {
	return MakeOptionNone[int]()
}
func main() {
	res := testOption()
	AglAssert(res == MakeOptionNone[int](), "assert failed 'res == None' line 7")
	AglAssert(testOption() == MakeOptionNone[int](), "assert failed 'testOption() == None' line 8")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Tuple1(t *testing.T) {
	src := `
fn main() {
	res := (1, "hello", true)
	assert(res.0 == 1)
	assert(res.1 == "hello")
	assert(res.2 == true)
}
`
	expected := `type AglTupleStruct1 struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	res := AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
	AglAssert(res.Arg0 == 1, "assert failed 'res.0 == 1' line 4")
	AglAssert(res.Arg1 == "hello", "assert failed 'res.1 == "hello"' line 5")
	AglAssert(res.Arg2 == true, "assert failed 'res.2 == true' line 6")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_TupleDestructuring1(t *testing.T) {
	src := `
fn main() {
	(a, b, c) := (1, "hello", true)
	assert(a == 1)
	assert(b == "hello")
	assert(c == true)
}
`
	expected := `type AglTupleStruct1 struct {
	Arg0 int
	Arg1 string
	Arg2 bool
}
func main() {
	aglVar1 := AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
	a, b, c := aglVar1.Arg0, aglVar1.Arg1, aglVar1.Arg2
	AglAssert(a == 1, "assert failed 'a == 1' line 4")
	AglAssert(b == "hello", "assert failed 'b == "hello"' line 5")
	AglAssert(c == true, "assert failed 'c == true' line 6")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Tuple2(t *testing.T) {
	src := `
fn testTuple() (u8, string, bool) {
	return (1, "hello", true)
}
fn main() {
	res := testTuple()
	assert(res.0 == 1)
	assert(res.1 == "hello")
	assert(res.2 == true)
}
`
	expected := `type AglTupleStruct1 struct {
	Arg0 uint8
	Arg1 string
	Arg2 bool
}
func testTuple() AglTupleStruct1 {
	return AglTupleStruct1{Arg0: 1, Arg1: "hello", Arg2: true}
}
func main() {
	res := testTuple()
	AglAssert(res.Arg0 == 1, "assert failed 'res.0 == 1' line 7")
	AglAssert(res.Arg1 == "hello", "assert failed 'res.1 == "hello"' line 8")
	AglAssert(res.Arg2 == true, "assert failed 'res.2 == true' line 9")
}
`
	testCodeGen(t, src, expected)
}

//func TestCodeGen37(t *testing.T) {
//	src := `
//type Dog struct {
//	name string?
//}
//type Person struct {
//	dog Dog?
//}
//fn getPersonDogName(p Person) string? {
//	return p.getDog()?.getName()?
//}
//fn main() {
//	person1 := Person{dog: Dog{name: Some("foo")}}
//	person2 := Person{dog: Some(Dog{name: None})}
//	person3 := Person{dog: None}
//	assert(person1.getDog()?.getName()? == "foo")
//	assert(person2.getDog()?.getName()? == None)
//	assert(person3.getDog()?.getName()? == None)
//}
//`
//	expected := `...`
//	got := codegen(infer(parser(NewTokenStream(src))))
//	if got != expected {
//		t.Errorf("expected:\n%s\ngot:\n%s", expected, got)
//	}
//}

func TestCodeGen38(t *testing.T) {
	src := `
type Person struct {
	name string
	age int
	ssn string?
	nicknames Option[[]string]
	testArray []string
	testArrayOfOpt []Option[string]
}
`
	expected := `type Person struct {
	name string
	age int
	ssn Option[string]
	nicknames Option[[]string]
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

func TestCodeGen40(t *testing.T) {
	src := `
fn addOne(i int) int { return i + 1 }
fn main() {
	a := true
	addOne(a)
}
`
	tassert.PanicsWithError(t, "5:9 wrong type of argument 0 in call to addOne, wants: int, got: bool", testCodeGenFn(src))
}

func TestCodeGen41(t *testing.T) {
	src := `
fn addOne(i int) int { return i + 1 }
fn main() {
	addOne(true)
}
`
	tassert.PanicsWithError(t, "4:9 wrong type of argument 0 in call to addOne, wants: int, got: bool", testCodeGenFn(src))
}

func TestCodeGen_Variadic1(t *testing.T) {
	src := `
fn variadic(a, b u8, c ...string) int {
	return 1
}
fn main() {
	variadic(1, 2, "a", "b", "c")
}
`
	expected := `func variadic(a, b uint8, c ...string) int {
	return 1
}
func main() {
	variadic(1, 2, "a", "b", "c")
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Variadic2(t *testing.T) {
	src := `
fn variadic(a, b u8, c ...string) int {
	return 1
}
fn main() {
	variadic(1)
}
`
	tassert.PanicsWithError(t, "6:2 not enough arguments in call to variadic", testCodeGenFn(src))
}

func TestCodeGen_Variadic3(t *testing.T) {
	src := `
fn variadic(a, b u8, c ...string) int {
	return 1
}
fn main() {
	variadic(1, 2, "a", 3, "c")
}
`
	tassert.PanicsWithError(t, "6:22 wrong type of argument 3 in call to variadic, wants: string, got: UntypedNumType", testCodeGenFn(src))
}

func TestCodeGen42(t *testing.T) {
	src := `
fn someFn() {
}
fn main() {
}
`
	expected := `func someFn() {
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
	src := `
fn main() {
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
	expected := `func main() {
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
	src := `
fn main() {
	a := 1
	if a == 1 {
	} else if a == 2 {
	} else {
	}
}
`
	expected := `func main() {
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
	src := `
fn main() {
	a := 1 == 1 || 2 == 2 && 3 == 3
}
`
	expected := `func main() {
	a := 1 == 1 || 2 == 2 && 3 == 3
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen47(t *testing.T) {
	src := `
fn test(v bool) {}
fn main() {
	test(true)
	test(1 == 1)
	test(1 == 1 && 2 == 2)
	test("a" == "b")
}
`
	expected := `func test(v bool) {
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

func TestCodeGen48(t *testing.T) {
	src := `
fn test(v bool) {}
fn main() {
	test("a" == 42)
}
`
	tassert.PanicsWithError(t, "4:7 mismatched types string and UntypedNumType", testCodeGenFn(src))
}

func TestCodeGen49(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3}
	s := a.sum()
	assert(s == 6)
}
`
	expected := `func main() {
	a := []uint8{1, 2, 3}
	s := AglVecSum(a)
	AglAssert(s == 6, "assert failed 's == 6' line 5")
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
	src := `
type Person struct {
}
fn (p Person) speak() string {
}
fn main() {
}
`
	expected := `type Person struct {
}
func (p Person) speak() string {
}
func main() {
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen52(t *testing.T) {
	src := `
type Person struct {
}
fn (p Person) method1() Person! {
	return Ok(p)
}
fn (p Person) method2() Person! {
	return Ok(p)
}
fn main() {
	p := Person{}
	a := p.method1()!.method1()
}
`
	expected := `type Person struct {
}
func (p Person) method1() Result[Person] {
	return MakeResultOk(p)
}
func (p Person) method2() Result[Person] {
	return MakeResultOk(p)
}
func main() {
	p := Person{}
	a := p.method1().Unwrap().method1()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen53(t *testing.T) {
	src := `
type Person struct {
}
fn (p Person) method1() Person! {
	return Ok(p)
}
fn main() {
	p := Person{}
	a := p.method1()!.method1()!
}
`
	expected := `type Person struct {
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
	src := `
type Color enum {
	red,
	green,
	blue,
}
fn takeColor(c Color) {}
fn main() {
	color := Color.red
	takeColor(color)
}
`
	expected := `type ColorTag int
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
	src := `
type Color enum {
	red,
	other(u8, string),
}
fn takeColor(c Color) {}
fn main() {
	color1 := Color.red
	color2 := Color.other(1, "yellow")
}
`
	expected := `type ColorTag int
const (
	Color_red ColorTag = iota + 1
	Color_other
)
type Color struct {
	tag ColorTag
	other0 uint8
	other1 string
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
	return Color{tag: Color_other, other0: arg0, other1: arg1}
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
	src := `
type Color enum {
	red,
	other(u8, string),
}
fn main() {
	(a, b) := Color.other(1, "yellow")
}
`
	expected := `type ColorTag int
const (
	Color_red ColorTag = iota + 1
	Color_other
)
type Color struct {
	tag ColorTag
	other0 uint8
	other1 string
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
	return Color{tag: Color_other, other0: arg0, other1: arg1}
}
func main() {
	aglVar1 := Make_Color_other(1, "yellow")
	a, b := aglVar1.other0, aglVar1.other1
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_Enum4(t *testing.T) {
	src := `
type Color enum {
	red,
	other(u8, string),
}
fn main() {
	other := Color.other(1, "yellow")
	(a, b) := other
}
`
	expected := `type ColorTag int
const (
	Color_red ColorTag = iota + 1
	Color_other
)
type Color struct {
	tag ColorTag
	other0 uint8
	other1 string
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
	return Color{tag: Color_other, other0: arg0, other1: arg1}
}
func main() {
	other := Make_Color_other(1, "yellow")
	aglVar1 := other
	a, b := aglVar1.other0, aglVar1.other1
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

//func TestCodeGen_OperatorOverloading(t *testing.T) {
//	src := `
//type Person struct {
//	name string
//	age int
//}
//fn (p Person) == (other Person) bool {
//	return p.age == other.age
//}
//fn main() {
//	p1 := Person{name: "foo", age: 42}
//	p2 := Person{name: "bar", age: 42}
//	assert(p1 == p2)
//}
//`
//	expected := `type Person struct {
//	name string
//	age int
//}
//func (p Person) __EQL(other Person) bool {
//	return p.age == other.age
//}
//func main() {
//	p1 := Person{name: "foo", age: 42}
//	p2 := Person{name: "bar", age: 42}
//	AglAssert(p1.__EQL(p2), "assert failed 'p1 == p2' line 12")
//}
//`
//	testCodeGen(t, src, expected)
//}

func TestCodeGen_55(t *testing.T) {
	src := `
fn main() {
	a := 2
	fmt.Println("first")
	if a == 2 {
		fmt.Println("second")
	}
}
`
	expected := `func main() {
	a := 2
	fmt.Println("first")
	if a == 2 {
		fmt.Println("second")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_56(t *testing.T) {
	src := `
type Color enum {
	blue,
	red,
}
fn main() {
	color := Color.red1
}
`
	tassert.PanicsWithError(t, "7:17: enum Color has no field red1", testCodeGenFn(src))
}

func TestCodeGen57(t *testing.T) {
	src := `
fn main() {
	a := []int{1, 2, 3}
	if 2 in a {
		fmt.Println("found")
	}
}
`
	expected := `func main() {
	a := []int{1, 2, 3}
	if AglVecIn(a, 2) {
		fmt.Println("found")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen58(t *testing.T) {
	src := `
fn test() int! {
    return Err("test")
}
fn main() {
    test()!
}
`
	expected := `func test() Result[int] {
	return MakeResultErr[int](errors.New("test"))
}
func main() {
	test().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen59(t *testing.T) {
	src := `
fn test() ! {
    return Err("test")
}
fn main() {
    test()!
}
`
	expected := `func test() Result[AglVoid] {
	return MakeResultErr[AglVoid](errors.New("test"))
}
func main() {
	test().Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_ErrPropagationInOption(t *testing.T) {
	src := `
fn errFn() ! {
    return Err("some error")
}
fn maybeInt() int? {
	errFn()!
	return Some(42)
}
fn main() {
    maybeInt()?
}
`
	expected := `func errFn() Result[AglVoid] {
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
	src := `
type Writer interface {}
fn main() {
}
`
	expected := `type Writer interface {
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

fn test(w Writer) {
    if w.(WriterA).is_some() {
        fmt.Println("A")
    }
}

fn main() {
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

fn main() {
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
	AglNoop[struct{}]()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen62(t *testing.T) {
	src := `
fn maybeInt() int? { return Some(42) }
fn getInt() int! { return Ok(42) }
fn main() {
	if Some(a) := maybeInt() {
	}
	if Ok(a) := getInt() {
	}
	if Err(e) := getInt() {
	}
}
`
	expected := `func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func getInt() Result[int] {
	return MakeResultOk(42)
}
func main() {
	if res := maybeInt(); res.IsSome() {
		a := res.Unwrap()
	}
	if res := getInt(); res.IsOk() {
		a := res.Unwrap()
	}
	if res := getInt(); res.IsErr() {
		e := res.Err()
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen63(t *testing.T) {
	src1 := `
fn maybeInt() int? { return Some(42) }
fn main() {
	if Some(a) := maybeInt() {
	}
	fmt.Println(a)
}
`
	tassert.PanicsWithError(t, "6:14: undefined identifier a", testCodeGenFn(src1))
	src2 := `
fn maybeInt() int? { return Some(42) }
fn main() {
	if Some(a) := maybeInt() {
		fmt.Println(a)
	}
}
`
	tassert.NotPanics(t, testCodeGenFn(src2))
}

func TestCodeGen_FunctionImplicitReturn(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	maybeInt()
}
`
	expected := `func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	maybeInt()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen64(t *testing.T) {
	src := `
type Writer interface {
	fn write(p []byte) int!
}
`
	expected := `type Writer interface {
	write([]byte) Result[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen65(t *testing.T) {
	src := `
type Writer interface {
	fn write(p []byte) int!
	fn another() bool
}
`
	expected := `type Writer interface {
	write([]byte) Result[int]
	another() bool
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_ValueSpec1(t *testing.T) {
	src := `
fn main() {
	var a int? = None
}
`
	expected := `func main() {
	var a Option[int] = MakeOptionNone[int]()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen_ValueSpec2(t *testing.T) {
	src := `
fn main() {
	var a int?
}
`
	expected := `func main() {
	var a Option[int]
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen66(t *testing.T) {
	src := `
type Color enum {
	red,
	other(u8, string),
}
fn main() {
	(a, b) := Color.other(1, "yellow")
	fmt.Println(a, b)
}
`
	expected := `type ColorTag int
const (
	Color_red ColorTag = iota + 1
	Color_other
)
type Color struct {
	tag ColorTag
	other0 uint8
	other1 string
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
	return Color{tag: Color_other, other0: arg0, other1: arg1}
}
func main() {
	aglVar1 := Make_Color_other(1, "yellow")
	a, b := aglVar1.other0, aglVar1.other1
	fmt.Println(a, b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen67(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	var b u8 = a.find({ $0 == 2 })?
	fmt.Println(b)
}
`
	expected := `func main() {
	a := []uint8{1, 2, 3, 4, 5}
	var b uint8 = AglVecFind(a, func(aglArg0 uint8) bool {
		return aglArg0 == 2
	}).Unwrap()
	fmt.Println(b)
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen68(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	var b u8 = a.find({ $0 == 2 })
}
`
	tassert.PanicsWithError(t, "4:2: cannot use Option[u8] as u8 value in variable declaration", testCodeGenFn(src))
}

func TestCodeGen69(t *testing.T) {
	src := `
fn test() []u8 { []u8{1, 2, 3} }
fn main() {
	test().filter({ $0 == 2 })
}
`
	expected := `func test() []uint8 {
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

func TestCodeGen70(t *testing.T) {
	src := `
fn test() []u8 { []u8{1, 2, 3} }
fn main() {
	test().filter({ %0 == 2 })
}
`
	tassert.PanicsWithError(t, "4:18: syntax error", testCodeGenFn(src))
}

func TestCodeGen71(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	match maybeInt() {
		Some(a) => { fmt.Println("some ", a) },
		None    => { fmt.Println("none") },
	}
}
`
	expected := `func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	res := maybeInt()
	if res.IsSome() {
		a := res.Unwrap()
		fmt.Println("some ", a)
	} else if res.IsNone() {
		fmt.Println("none")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen72(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	match maybeInt() {
		_ => { fmt.Println("Some or None") },
	}
}
`
	expected := `func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	res := maybeInt()
	if res.IsSome() || res.IsNone() {
		fmt.Println("Some or None")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen75(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	match maybeInt() {
		_ => { fmt.Println("Some or None") },
		None => { fmt.Println("none") },
	}
}
`
	expected := `func maybeInt() Option[int] {
	return MakeOptionSome(42)
}
func main() {
	res := maybeInt()
	if res.IsNone() {
		fmt.Println("none")
	} else if res.IsSome() || res.IsNone() {
		fmt.Println("Some or None")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen73(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	match maybeInt() {
		Some(a) => { fmt.Println("some ", a) },
	}
}
`
	tassert.PanicsWithError(t, "4:8: match statement must be exhaustive", testCodeGenFn(src))
}

func TestCodeGen74(t *testing.T) {
	src := `
fn maybeInt() int? { Some(42) }
fn main() {
	match maybeInt() {
		None => { fmt.Println("none") },
	}
}
`
	tassert.PanicsWithError(t, "4:8: match statement must be exhaustive", testCodeGenFn(src))
}

func TestCodeGen76(t *testing.T) {
	src := `
fn getInt() int! { Ok(42) }
fn main() {
	match getInt() {
		_ => { fmt.Println("Ok or Err") },
	}
}
`
	expected := `func getInt() Result[int] {
	return MakeResultOk(42)
}
func main() {
	res := getInt()
	if res.IsOk() || res.IsErr() {
		fmt.Println("Ok or Err")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen77(t *testing.T) {
	src := `
fn getInt() int! { Ok(42) }
fn main() {
	match getInt() {
		_ => { fmt.Println("Ok or Err") },
		Ok(n) => { fmt.Println("Ok ", n) },
	}
}
`
	expected := `func getInt() Result[int] {
	return MakeResultOk(42)
}
func main() {
	res := getInt()
	if res.IsOk() {
		n := res.Unwrap()
		fmt.Println("Ok ", n)
	} else if res.IsOk() || res.IsErr() {
		fmt.Println("Ok or Err")
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen78(t *testing.T) {
	src := `
fn getInt() int! { Ok(42) }
fn main() {
	match getInt() {
		Ok(n) => { fmt.Println("Ok ", n) },
		Err(e) => { fmt.Println("Err ", e) },
	}
}
`
	expected := `func getInt() Result[int] {
	return MakeResultOk(42)
}
func main() {
	res := getInt()
	if res.IsOk() {
		n := res.Unwrap()
		fmt.Println("Ok ", n)
	} else if res.IsErr() {
		fmt.Println("Err ", e)
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen79(t *testing.T) {
	src := `
fn getInt() int! { Ok(42) }
fn main() {
	match getInt() {
		Ok(n) => fmt.Println("Ok ", n),
		Err(e) => fmt.Println("Err ", e),
	}
}
`
	expected := `func getInt() Result[int] {
	return MakeResultOk(42)
}
func main() {
	res := getInt()
	if res.IsOk() {
		n := res.Unwrap()
		fmt.Println("Ok ", n)
	} else if res.IsErr() {
		fmt.Println("Err ", e)
	}
}
`
	testCodeGen(t, src, expected)
}

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
	src := `
fn main() {
	_ = 42
}
`
	expected := `func main() {
	_ = 42
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen82(t *testing.T) {
	src := `
fn main() {
	_ := 42
}
`
	tassert.PanicsWithError(t, "3:4: No new variables on the left side of ':='", testCodeGenFn(src))
}

func TestCodeGen83(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	a.find(fn(e u8) bool { e == 2 })?
}
`
	expected := `func main() {
	a := []uint8{1, 2, 3, 4, 5}
	AglVecFind(a, func(e uint8) bool {
		return e == 2
	}).Unwrap()
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen84(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	a.find(fn(e i64) bool { e == 2 })?
}
`
	tassert.PanicsWithError(t, "4:2: function type fn(i64) bool does not match inferred type fn(u8) bool", testCodeGenFn(src))
}

func TestCodeGen84_1(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	a.find(fn(e u8) { e == 2 })?
}
`
	tassert.PanicsWithError(t, "4:2: function type fn(u8) does not match inferred type fn(u8) bool", testCodeGenFn(src))
}

func TestCodeGen84_2(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	f := fn(e u8) { e == 2 }
	a.find(f)?
}
`
	tassert.PanicsWithError(t, "5:2: function type fn(u8) does not match inferred type fn(u8) bool", testCodeGenFn(src))
}

func TestCodeGen85(t *testing.T) {
	src := `
fn main() {
	a := []u8{1, 2, 3, 4, 5}
	f := fn(e u8) bool { return e == 2 }
	a.find(f)?
}
`
	expected := `func main() {
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
	src := `
fn main() {
	if a := 123; a == 2 || a == 3 {
	}
}
`
	expected := `func main() {
	if a := 123; a == 2 || a == 3 {
	}
}
`
	testCodeGen(t, src, expected)
}

func TestCodeGen87(t *testing.T) {
	src := `
import (
	"fmt"
	"errors"
)
`
	expected := `import (
	"fmt"
	"errors"
)
`
	testCodeGen(t, src, expected)
}

func TestCodeGen88(t *testing.T) {
	src := `
type Pos struct {
    Row, Col int
}
`
	expected := `type Pos struct {
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
fn main() {
	p1 := Person{name: "John"}
	p2 := Person{name: "Jane"}
	arr := []Person{p1, p2}
	res := arr.map({ $0.name }).joined(", ")
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

func TestCodeGen_Tmp(t *testing.T) {
	src := `
fn main() {
	if a := 123; a == 2 || a == 3 {
	}
}
`
	tassert.NotPanics(t, testCodeGenFn(src))
}
