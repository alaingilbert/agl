package utils

import (
	"reflect"
	"strings"
)

func Noop(...any) {}

func Test() string {
	return "test"
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

func Map[T, R any](a []T, clb func(T) R) (out []R) {
	for _, el := range a {
		out = append(out, clb(el))
	}
	return
}

func MapJoin[T any](a []T, clb func(T) string, sep string) string {
	return strings.Join(Map(a, clb), sep)
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

func ApplyIf(s string, clb func() string) string {
	if s != "" {
		return clb()
	}
	return s
}

func PrefixIf(s, prefix string) string {
	return ApplyIf(s, func() string { return prefix + s })
}

func SuffixIf(s, suffix string) string {
	return ApplyIf(s, func() string { return s + suffix })
}

func WrapIf(s, prefix, suffix string) string {
	return ApplyIf(s, func() string { return prefix + s + suffix })
}
