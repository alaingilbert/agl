# AGL (AnotherGoLang)

## features

- Tuple
- Enum
- Error propagation operators (`?` for Option / `!` for Result)
- Anon function with type inferred arguments (`other := someArr.filter({ $0 % 2 == 0 })`)
- Array built-in map/reduce/filter methods
- Compile down to Go code

## Destructuring

```go
type IpAddr enum {
    v4(u8, u8, u8, u8),
    v6(string),
}
fn main() {
    // enum values can be destructured
    addr1 := IpAddr.v4(127, 0, 0, 1)
    (a, b, c, d) := addr1
	
    // tuple can be destructured
    tuple := (1, "hello", true)
    (f, g, h) := tuple
}
```

## Operator overloading

```go
type Person struct {
    name string
    age int
}
fn (p Person) == (other Person) bool {
    return p.age == other.age
}
fn main() {
    p1 := Person{name: "foo", age: 42}
    p2 := Person{name: "bar", age: 42}
    assert(p1 == p2)
}
```