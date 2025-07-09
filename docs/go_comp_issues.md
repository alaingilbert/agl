Here is a list of issues encountered while trying to keep Go code as valid AGL code.

## Functions that return multiple values

Conflict between parsing the return value as a native function vs a tuple.

```go
func test() (int, int) {
	return 1, 2
}
```

## Type assert
```go
// Both valid in Go
v, ok := a.(int)
v := a.(int)
```

In AGL we'd like to have "safe" type assert which return an Option[T].  

```go
res := a.(int)?
```
But the syntax does not allow it. We wouldn't know if `v := a.(int)` is of type Option or int.  

We could introduce a separate operator/function to do safe type assert. eg: `v := a.As(int)?`  

## Native functions that return an error

```go
if err := os.WriteFile("test.txt", []byte("test"), 0644); err != nil {
}
```

Ideally in AGL we'd want to be able to call `os.WriteFile(file, content, perm)!`  

But if we keep Go syntax as valid AGL, we wouldn't know if the result value is of type `Result` or `error`.  

### Solution #1

Make a `Go2Agl` script that would automatically convert these functions into the proper AGL syntax, using Go AST.

### Solution #2

Duplicating std/vendor libs signatures.  
`os.WriteFile(string, []byte, FileMode) error` and  
`agl.os.WriteFile(string, []byte, FileMode) !`  