package agl

import (
	"agl/pkg/ast"
	"fmt"
	"iter"
	"os"
	"path"
	"reflect"
	"runtime"
)

func noop(_ ...any) {}

func printCallers(n int) {
	fmt.Println("--- callers ---")
	for i := 0; i < n; i++ {
		pc, _, _, ok := runtime.Caller(i + 2)
		if !ok {
			break
		}
		f := runtime.FuncForPC(pc)
		file, line := f.FileLine(pc)
		fmt.Printf("%s:%d %s\n", path.Base(file), line, f.Name())
	}
}

func p(a ...any) {
	var tmp []any
	for _, e := range a {
		var tmp2 any
		switch v := e.(type) {
		case *ast.Ident:
			name := "<nil>"
			if v != nil {
				name = v.Name
			}
			tmp2 = fmt.Sprintf("Ident(%v)", name)
		case *ast.FuncType:
			tmp2 = fmt.Sprintf("FuncType(...)")
		case *ast.CallExpr:
			tmp2 = fmt.Sprintf("CallExpr(%v %v)", v.Fun, v.Args)
		default:
			tmp2 = e
		}
		tmp = append(tmp, tmp2)
	}
	_, _ = fmt.Fprintln(os.Stderr, tmp...)
}

func assert(pred bool, msg ...string) {
	if !pred {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		panic(NewAglError(m))
	}
}

func assertf(pred bool, format string, a ...any) {
	if !pred {
		m := ""
		if len(format) > 0 {
			m = fmt.Sprintf(format, a...)
		}
		panic(NewAglError(m))
	}
}

type AglError struct {
	msg string
}

func (e *AglError) Error() string {
	return e.msg
}

func NewAglError(msg string) *AglError {
	return &AglError{msg: msg}
}

func to(v any) reflect.Type {
	return reflect.TypeOf(v)
}

// Cast ...
func Cast[T any](origin any) (T, bool) {
	if val, ok := origin.(reflect.Value); ok {
		origin = val.Interface()
	}
	val, ok := origin.(T)
	return val, ok
}

// NotNil panics if the input is nil.
func NotNil[T any](v T) T {
	if vi, ok := any(v).(interface{ IsNil() bool }); ok && vi.IsNil() {
		panic("value is nil")
	}
	if any(v) == nil {
		panic("value is nil")
	}
	return v
}

// Must ...
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// MustCast ...
func MustCast[T any](origin any) T {
	v, ok := Cast[T](origin)
	if !ok {
		panic(fmt.Sprintf("%v", reflect.TypeOf(origin)))
	}
	return v
}

// TryCast ...
func TryCast[T any](origin any) bool {
	_, ok := Cast[T](origin)
	return ok
}

// CastInto ...
func CastInto[T any](origin any, into *T) bool {
	originVal, ok := origin.(reflect.Value)
	if !ok {
		originVal = reflect.ValueOf(origin)
	}
	if originVal.IsValid() {
		if _, ok := originVal.Interface().(T); ok {
			rv := reflect.ValueOf(into)
			if rv.Kind() == reflect.Pointer && !rv.IsNil() {
				rv.Elem().Set(originVal)
				return true
			}
		}
	}
	return false
}

// Deref generic deref return the zero value if v is nil
func Deref[T any](v *T) T {
	var zero T
	if v == nil {
		return zero
	}
	return *v
}

// Ptr ...
func Ptr[T any](v T) *T { return &v }

// Override ...
func Override[T any](v *T, w *T) *T {
	if w != nil {
		if v == nil {
			v = new(T)
		}
		*v = *w
	}
	return v
}

// Default ...
func Default[T any](v *T, d T) T {
	if v == nil {
		return d
	}
	return *v
}

// Ternary ...
func Ternary[T any](predicate bool, a, b T) T {
	if predicate {
		return a
	}
	return b
}

// TernaryOrZero ...
func TernaryOrZero[T any](predicate bool, a T) (zero T) {
	return Ternary(predicate, a, zero)
}

// Or return "a" if it is non-zero otherwise "b"
func Or[T comparable](a, b T) (zero T) {
	return Ternary(a != zero, a, b)
}

// InArray returns either or not a string is in an array
func InArray[T comparable](needle T, haystack []T) bool {
	return IndexOf(haystack, needle) > -1
}

// IndexOf ...
func IndexOf[T comparable](arr []T, needle T) (idx int) {
	predicate := func(el T) bool { return el == needle }
	return Second(FindIdx(arr, predicate))
}

// Find looks through each value in the list, returning the first one that passes a truth test (predicate),
// or nil if no value passes the test.
// The function returns as soon as it finds an acceptable element, and doesn't traverse the entire list
func Find[T any](arr []T, predicate func(T) bool) (out *T) {
	return FindIter(SliceSeq(arr), predicate)
}

// FindIter ...
func FindIter[T any](it iter.Seq[T], predicate func(T) bool) (out *T) {
	return First(FindIdxIter(it, predicate))
}

// FindIdx ...
func FindIdx[T any](arr []T, predicate func(T) bool) (*T, int) {
	return FindIdxIter(SliceSeq(arr), predicate)
}

// FindIdxIter ...
func FindIdxIter[T any](it iter.Seq[T], predicate func(T) bool) (*T, int) {
	var i int
	for el := range it {
		if predicate(el) {
			return &el, i
		}
		i++
	}
	return nil, -1
}

func First[T any](a T, _ ...any) T { return a }

func Second[T any](_ any, a T, _ ...any) T { return a }

func SliceSeq[T any](s []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range s {
			if !yield(v) {
				return
			}
		}
	}
}
