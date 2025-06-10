package main

import (
	"fmt"
	"iter"
	"os"
	"reflect"
)

func noop[T any](_ ...T) {}

func p(a ...any) {
	_, _ = fmt.Fprintln(os.Stderr, a...)
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

// MustCast ...
func MustCast[T any](origin any) T {
	v, ok := Cast[T](origin)
	if !ok {
		panic("")
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
