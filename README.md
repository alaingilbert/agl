# AGL (AnotherGoLang)

## features

- Tuple
- Enum
- Error propagation operators (`?` for Option[T] / `!` for Result[T])
- Concise anonymous function with type inferred arguments (`other := someArr.Filter({ $0 % 2 == 0 })`)
- Array built-in Map/Reduce/Filter/Find/Sum methods
- Operator overloading
- Compile down to Go code

## Error propagation

### Result propagation

```go
func getInt() int! {
	return Ok(42)
}
func intermediate() int! {
	num := getInt()! // Propagate 'Err' value to the caller
	return Ok(num + 1)
}
func main() {
	num := intermediate()! // crash on 'Err' value
	fmt.Println(num)
}
```

### Option propagation

```go
func maybeInt() int? {
	return Some(42)
}
func intermediate() int? {
	num := maybeInt()? // Propagate 'None' value to the caller
	return Some(num + 1)
}
func main() {
	num := intermediate()? // crash on 'None' value
	fmt.Println(num)
}
```

### `If let` to use a Option[T]/Result[T] value safely

```go
func maybeInt() int? {
    return Some(42)
}
func main() {
    if let Some(num) := maybeInt() {
        fmt.Println(num)
    }
}
```

## Short anonymous function (type inferred)

```go
package main

type Person struct {
	Name string
	Age int
}

func main() {
	arr := []int{1, 2, 3, 4, 5}
	sum := arr.Filter({ $0 % 2 == 0 }).Map({ $0 + 1 }).Sum()
	assert(sum == 8)

	p1 := Person{Name: "foo", Age: 18}
	p2 := Person{Name: "bar", Age: 19}
	people := []Person{p1, p2}
	names := people.Map({ $0.Name }).Joined(", ")
	sumAge := people.Map({ $0.Age }).Sum()
	assert(names == "foo, bar")
	assert(sumAge == 37)
}
```

## Destructuring

```go
package main

import "fmt"

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
```

## Operator overloading

```go
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
```