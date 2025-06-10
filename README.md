# AGL (AnotherGoLang)

## features

- Tuple
- Enum
- Error propagation operators (`?` for Option / `!` for Result)
- Anon function with type inferred arguments (`other := someArr.filter({ $0 % 2 == 0 })`)
- Array built-in map/reduce/filter methods
- Compile down to Go code

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