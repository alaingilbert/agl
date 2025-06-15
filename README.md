# AGL (AnotherGoLang)

## features

- Tuple
- Enum
- Error propagation operators (`?` for Option / `!` for Result)
- Concise anonymous function with type inferred arguments (`other := someArr.filter({ $0 % 2 == 0 })`)
- Array built-in map/reduce/filter methods
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

## Destructuring

```go
type IpAddr enum {
    v4(u8, u8, u8, u8),
    v6(string),
}
func main() {
    // enum values can be destructured
    addr1 := IpAddr.v4(127, 0, 0, 1)
    a, b, c, d := addr1
	
    // tuple can be destructured
    tuple := (1, "hello", true)
    f, g, h := tuple
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