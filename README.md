# AGL (AnotherGoLang)

## features

- Tuple
- Enum
- Error propagation operators (`?` for Option[T] / `!` for Result[T])
- Concise anonymous function with type inferred arguments (`other := someArr.Filter({ $0 % 2 == 0 })`)
- Array built-in Map/Reduce/Filter/Find/Sum methods
- Operator overloading
- Compile down to Go code

## How to use

```go
go build               // Build the "agl" executable
./agl main.agl         // Output Go code in stdout
./agl run main.agl     // Run the code directly and output the result in stdout
./agl build main.agl   // Create a main.go file
```

## Error propagation

### Result propagation

```go
package main

import "fmt"

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
package main

import "fmt"

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

### Propagation chaining

```go
package main

type Person struct { Name string }

func (p Person) MaybeSelf() Person? {
    return Some(p)
}

func main() {
    bob := Person{Name: "bob"}
    bob.MaybeSelf()?.MaybeSelf()?.MaybeSelf()?
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
package main

type Person struct {
    Name string
    Age int
}

func (p Person) == (other Person) bool {
    return p.Age == other.Age
}

func main() {
    p1 := Person{Name: "foo", Age: 42}
    p2 := Person{Name: "bar", Age: 42}
    assert(p1 == p2)
}
```

## Extend built-in types

```go
package main

import "fmt"

func (v agl.Vec[T]) Even() []T {
    out := make([]T, 0)
    for _, el := range v {
        if el % 2 == 0 {
            out = append(out, el)
        }
    }
    return out
}

func main() {
    arr := []int{1, 2, 3, 4, 5, 6, 7, 8}
    res := arr.Even().Filter({ $0 <= 6 }).Map({ $0 + 1 }) 
    //         ^^^^^^ new method available
    fmt.Println(res) // [3 5 7]
}
```

## Using Go libraries

```go
package main

import (
    "fmt"
    "os"
)

func main() {
    os.WriteFile("test.txt", []byte("test"), 0755)!
    by := os.ReadFile("test.txt")!
    fmt.Println(string(by))
}
```

## Making http request

```go
package main

import (
    "fmt"
    "net/http"
    "io"
)

func main() {
    req := http.NewRequest(http.MethodGet, "https://google.com", nil)!
    c := http.Client{}
    resp := c.Do(req)!
    defer resp.Body.Close()
    by := io.ReadAll(resp.Body)!
    fmt.Println(string(by))
}
```