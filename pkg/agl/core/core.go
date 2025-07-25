package main

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"iter"
	"maps"
	"math"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
)

type Signed interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64
}

type Unsigned interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type Integer interface {
	Signed | Unsigned
}

type Float interface {
	~float32 | ~float64
}

type Complex interface {
	~complex64 | ~complex128
}

type Number interface {
	Integer | Float
}

func AglBytesEqual(a, b []byte) bool {
	return bytes.Equal(a, b)
}

func AglWrapNative1(err error) Result[AglVoid] {
	if err != nil {
		return MakeResultErr[AglVoid](err)
	}
	return MakeResultOk(AglVoid{})
}

func AglWrapNative2[T any](v1 T, err error) Result[T] {
	if err != nil {
		return MakeResultErr[T](err)
	}
	return MakeResultOk(v1)
}

func AglWrapNativeOpt[T any](v1 T, ok bool) Option[T] {
	if !ok {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome(v1)
}

type AglVoid struct{}

type Option[T any] struct {
	t *T
}

func (o Option[T]) String() string {
	if o.IsNone() {
		return "None"
	}
	return fmt.Sprintf("Some(%v)", *o.t)
}

func (o Option[T]) IsSome() bool {
	return o.t != nil
}

func (o Option[T]) IsNone() bool {
	return o.t == nil
}

func (o Option[T]) Unwrap() T {
	if o.IsNone() {
		panic("unwrap on a None value")
	}
	return *o.t
}

func (o Option[T]) UnwrapOr(d T) T {
	if o.IsNone() {
		return d
	}
	return *o.t
}

func (o Option[T]) UnwrapOrDefault() T {
	var zero T
	if o.IsNone() {
		return zero
	}
	return *o.t
}

func AglOptionMap[T, R any](o Option[T], clb func(T) R) Option[R] {
	if o.IsSome() {
		return MakeOptionSome(clb(o.Unwrap()))
	}
	return MakeOptionNone[R]()
}

func MakeOptionSome[T any](t T) Option[T] {
	return Option[T]{t: &t}
}

func MakeOptionNone[T any]() Option[T] {
	return Option[T]{t: nil}
}

type Result[T any] struct {
	t *T
	e error
}

func (r Result[T]) String() string {
	if r.IsErr() {
		return fmt.Sprintf("Err(%v)", r.e)
	}
	return fmt.Sprintf("Ok(%v)", *r.t)
}

func (r Result[T]) IsErr() bool {
	return r.e != nil
}

func (r Result[T]) IsOk() bool {
	return r.e == nil
}

func (r Result[T]) Unwrap() T {
	if r.IsErr() {
		panic(fmt.Sprintf("unwrap on an Err value: %s", r.e))
	}
	return *r.t
}

func (r Result[T]) NativeUnwrap() (*T, error) {
	return r.t, r.e
}

func (r Result[T]) UnwrapOr(d T) T {
	if r.IsErr() {
		return d
	}
	return *r.t
}

func (r Result[T]) UnwrapOrDefault() T {
	var zero T
	if r.IsErr() {
		return zero
	}
	return *r.t
}

func (r Result[T]) Err() error {
	return r.e
}

func MakeResultOk[T any](t T) Result[T] {
	return Result[T]{t: &t, e: nil}
}

func MakeResultErr[T any](err error) Result[T] {
	return Result[T]{t: nil, e: err}
}

func AglVecMap[T, R any](a []T, f func(T) R) []R {
	var out []R
	for _, v := range a {
		out = append(out, f(v))
	}
	return out
}

func AglVecFilterMap[T, R any](a []T, f func(T) Option[R]) []R {
	var out []R
	for _, v := range a {
		res := f(v)
		if res.IsSome() {
			out = append(out, res.Unwrap())
		}
	}
	return out
}

func AglVec__ADD[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func AglVecFilter[T any](a []T, f func(T) bool) []T {
	var out []T
	for _, v := range a {
		if f(v) {
			out = append(out, v)
		}
	}
	return out
}

func AglVecFirstIndex[T comparable](a []T, e T) Option[int] {
	for i, v := range a {
		if v == e {
			return MakeOptionSome(i)
		}
	}
	return MakeOptionNone[int]()
}

func AglVecFirstIndexWhere[T any](a []T, p func(T) bool) Option[int] {
	for i, v := range a {
		if p(v) {
			return MakeOptionSome(i)
		}
	}
	return MakeOptionNone[int]()
}

func AglVecAllSatisfy[T any](a []T, f func(T) bool) bool {
	for _, v := range a {
		if !f(v) {
			return false
		}
	}
	return true
}

func AglVecContains[T comparable](a []T, e T) bool {
	for _, v := range a {
		if v == e {
			return true
		}
	}
	return false
}

func AglVecContainsWhere[T comparable](a []T, p func(T) bool) bool {
	for _, v := range a {
		if p(v) {
			return true
		}
	}
	return false
}

func AglVecAny[T any](a []T, f func(T) bool) bool {
	for _, v := range a {
		if f(v) {
			return true
		}
	}
	return false
}

func AglVecReduce[T, R any](a []T, acc R, f func(R, T) R) R {
	for _, v := range a {
		acc = f(acc, v)
	}
	return acc
}

func AglVecReduceInto[T, R any](a []T, acc R, f func(*R, T) AglVoid) R {
	for _, v := range a {
		f(&acc, v)
	}
	return acc
}

func AglMapReduce[K comparable, V, R any](m map[K]V, acc R, f func(R, DictEntry[K, V]) R) R {
	for k, v := range m {
		acc = f(acc, DictEntry[K, V]{Key: k, Value: v})
	}
	return acc
}

func AglMapReduceInto[K comparable, V, R any](m map[K]V, acc R, f func(*R, DictEntry[K, V]) AglVoid) R {
	for k, v := range m {
		f(&acc, DictEntry[K, V]{Key: k, Value: v})
	}
	return acc
}

func AglAssert(pred bool, msg ...string) {
	if !pred {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		panic(m)
	}
}

func AglVecIn[T cmp.Ordered](a []T, v T) bool {
	for _, el := range a {
		if el == v {
			return true
		}
	}
	return false
}

func AglNoop(_ ...any) Result[AglVoid] { return MakeResultOk(AglVoid{}) }

func AglTypeAssert[T any](v any) Option[T] {
	if v, ok := v.(T); ok {
		return MakeOptionSome(v)
	}
	return MakeOptionNone[T]()
}

func AglIdentity[T any](v T) T { return v }

func AglVecFind[T any](a []T, f func(T) bool) Option[T] {
	for _, v := range a {
		if f(v) {
			return MakeOptionSome(v)
		}
	}
	return MakeOptionNone[T]()
}

type VecIter[T any] struct {
	v AglVec[T]
	i int
}

func (v *VecIter[T]) Next() Option[T] {
	if v.i >= len(v.v) {
		return MakeOptionNone[T]()
	}
	res := v.v[v.i]
	v.i++
	return MakeOptionSome(res)
}

type IntoIterator[T any] interface {
	Iter() Iterator[T]
}

// Sequence anything that can be turned into an Iterator
type Sequence[T any] iter.Seq[T]

func (s Sequence[T]) Iter() Sequence[T] {
	return s
}

func AglSequenceSum[T, R Number](s Sequence[T]) (out R) {
	for e := range s {
		out += R(e)
	}
	return out
}

type AglVec[T any] []T

func (v AglVec[T]) Len() int { return len(v) }

func (v AglVec[T]) Iter() Sequence[T] {
	return func(yield func(T) bool) {
		for _, e := range v {
			if !yield(e) {
				return
			}
		}
	}
}

func AglVecIter[T any](v AglVec[T]) Sequence[T] {
	return v.Iter()
}

type DictEntry[K comparable, V any] struct {
	Key   K
	Value V
}

type AglMap[K comparable, V any] map[K]V

func (m AglMap[K, V]) Iter() Sequence[K] { return AglMapKeys(m) }

func (m AglMap[K, V]) Len() int { return len(m) }

type AglSet[T comparable] map[T]struct{}

func (s AglSet[T]) Len() int { return len(s) }

type AglRange[T Integer] struct {
	Start, End T
	IsEq       bool
	Val        T
}

func AglNewRange[T Integer](start, end T, isEq bool) *AglRange[T] {
	r := &AglRange[T]{Start: start, End: end, IsEq: isEq}
	return r
}

func (r *AglRange[T]) Next() Option[T] {
	if (r.IsEq && r.Start > r.End) || (!r.IsEq && r.Start >= r.End) {
		return MakeOptionNone[T]()
	}
	res := MakeOptionSome(r.Start)
	r.Start++
	return res
}

func (r *AglRange[T]) NextBack() Option[T] {
	if (r.IsEq && r.Start > r.End) || (!r.IsEq && r.Start >= r.End) {
		return MakeOptionNone[T]()
	}
	if r.IsEq {
		res := MakeOptionSome(r.End)
		r.End--
		return res
	} else {
		r.End--
		return MakeOptionSome(r.End)
	}
}

func (r *AglRange[T]) Iter() Sequence[T] {
	return func(yield func(T) bool) {
		for {
			if el := r.Next(); el.IsSome() {
				if !yield(el.Unwrap()) {
					return
				}
			} else {
				return
			}
		}
	}
}

type Iterator[T any] interface {
	Iter() Sequence[T]
	//Next() Option[T]
}

type DoubleEndedIterator[T any] interface {
	Iterator[T]
	Next() Option[T]
	NextBack() Option[T]
}

type Rev[T any] struct {
	it DoubleEndedIterator[T]
}

func (r Rev[T]) Iter() Sequence[T] {
	return func(yield func(T) bool) {
		for {
			if el := r.Next(); el.IsSome() {
				if !yield(el.Unwrap()) {
					return
				}
			} else {
				return
			}
		}
	}
}

func (r *Rev[T]) Next() Option[T] {
	return r.it.NextBack()
}

func (r *Rev[T]) NextBack() Option[T] {
	return r.it.Next()
}

func AglDoubleEndedIteratorRev[T any, I DoubleEndedIterator[T]](it I) *Rev[T] {
	return &Rev[T]{it: it}
}

func (s AglSet[T]) Iter() Sequence[T] {
	return func(yield func(T) bool) {
		for k := range s {
			if !yield(k) {
				return
			}
		}
	}
}

func AglSetIter[T comparable](s AglSet[T]) Sequence[T] {
	return Sequence[T](s.Iter())
}

func (s AglSet[T]) String() string {
	var tmp []string
	for k := range s {
		tmp = append(tmp, fmt.Sprintf("%v", k))
	}
	return fmt.Sprintf("set(%s)", strings.Join(tmp, " "))
}

func (s AglSet[T]) Intersects(other Iterator[T]) bool {
	return AglSetIntersects(s, other)
}

func (s AglSet[T]) Contains(el T) bool {
	return AglSetContains(s, el)
}

func AglSetLen[T comparable](s AglSet[T]) int {
	return len(s)
}

func AglIterMin[T cmp.Ordered](it Sequence[T]) Option[T] {
	var out T
	first := true
	for e := range it {
		if first {
			out = e
			first = false
		} else {
			out = min(out, e)
		}
	}
	if first {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome(out)
}

func AglIterMax[T cmp.Ordered](it Sequence[T]) Option[T] {
	var out T
	first := true
	for e := range it {
		if first {
			out = e
			first = false
		} else {
			out = max(out, e)
		}
	}
	if first {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome(out)
}

func AglSetMin[T cmp.Ordered](s AglSet[T]) Option[T] {
	return AglIterMin(s.Iter())
}

func AglSetMax[T cmp.Ordered](s AglSet[T]) Option[T] {
	return AglIterMax(s.Iter())
}

// AglSetEquals returns a Boolean value indicating whether two sets have equal elements.
func AglSetEquals[T comparable](s, other AglSet[T]) bool {
	if len(s) != len(other) {
		return false
	}
	for k := range s {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	return true
}

func AglSetInsert[T comparable](s AglSet[T], el T) bool {
	if _, ok := s[el]; ok {
		return false
	}
	s[el] = struct{}{}
	return true
}

// AglSetContains returns a Boolean value that indicates whether the given element exists in the set.
func AglSetContains[T comparable](s AglSet[T], el T) bool {
	_, ok := s[el]
	return ok
}

// AglSetRemove removes the specified element from the set.
// Return: The value of the member parameter if it was a member of the set; otherwise, nil.
func AglSetRemove[T comparable](s AglSet[T], el T) Option[T] {
	if _, ok := s[el]; ok {
		delete(s, el)
		return MakeOptionSome(el)
	}
	return MakeOptionNone[T]()
}

func AglSetIsEmpty[T comparable](s AglSet[T]) bool {
	return len(s) == 0
}

func AglSetRemoveFirst[T comparable](s AglSet[T]) T {
	for k := range s {
		delete(s, k)
		return k
	}
	panic("set is empty")
}

func AglSetFirst[T comparable](s AglSet[T]) (out Option[T]) {
	if len(s) > 0 {
		for k := range s {
			return MakeOptionSome(k)
		}
	}
	return MakeOptionNone[T]()
}

func AglSetFirstWhere[T comparable](s AglSet[T], predicate func(T) bool) (out Option[T]) {
	if len(s) == 0 {
		return MakeOptionNone[T]()
	}
	for k := range s {
		if predicate(k) {
			return MakeOptionSome(k)
		}
	}
	return MakeOptionNone[T]()
}

//func AglIteratorEach[T any](it Iterator[T]) aglImportIter.Seq[T] {
//	return func(yield func(T) bool) {
//		for {
//			if el := it.Next(); el.IsSome() {
//				if !yield(el.Unwrap()) {
//					return
//				}
//			} else {
//				return
//			}
//		}
//	}
//}
//
//func AglSequenceEach[T any](seq Sequence[T]) aglImportIter.Seq[T] {
//	return AglIteratorEach(seq.Iter())
//}

func AglSetMap[T comparable, R any](s AglSet[T], f func(T) R) []R {
	var out []R
	for k := range s {
		out = append(out, f(k))
	}
	return out
}

func AglSetFilter[T comparable](s AglSet[T], pred func(T) bool) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		if pred(k) {
			newSet[k] = struct{}{}
		}
	}
	return newSet
}

// AglSetUnion returns a new set with the elements of both this set and the given sequence.
func AglSetUnion[T comparable](s AglSet[T], other Iterator[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		newSet[k] = struct{}{}
	}
	for k := range other.Iter() {
		newSet[k] = struct{}{}
	}
	return newSet
}

// AglSetFormUnion inserts the elements of the given sequence into the set.
func AglSetFormUnion[T comparable](s AglSet[T], other Sequence[T]) {
	for k := range other {
		s[k] = struct{}{}
	}
}

func AglSetSubtracting[T comparable](s AglSet[T], other Iterator[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		newSet[k] = struct{}{}
	}
	for k := range other.Iter() {
		delete(newSet, k)
	}
	return newSet
}

func AglSetSubtract[T comparable](s AglSet[T], other Iterator[T]) {
	for k := range other.Iter() {
		delete(s, k)
	}
}

// AglSetIntersection returns a new set with the elements that are common to both this set and the given sequence.
func AglSetIntersection[T comparable](s, other AglSet[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		if _, ok := other[k]; ok {
			newSet[k] = struct{}{}
		}
	}
	return newSet
}

// AglSetFormIntersection removes the elements of the set that aren't also in the given sequence.
func AglSetFormIntersection[T comparable](s, other AglSet[T]) {
	for k := range s {
		if _, ok := other[k]; !ok {
			delete(s, k)
		}
	}
}

// AglSetSymmetricDifference returns a new set with the elements that are either in this set or in the given sequence, but not in both.
func AglSetSymmetricDifference[T comparable](s, other AglSet[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		if _, ok := other[k]; !ok {
			newSet[k] = struct{}{}
		}
	}
	for k := range other {
		if _, ok := s[k]; !ok {
			newSet[k] = struct{}{}
		}
	}
	return newSet
}

// AglSetFormSymmetricDifference removes the elements of the set that are also in the given sequence and adds the members of the sequence that are not already in the set.
func AglSetFormSymmetricDifference[T comparable](s AglSet[T], other Iterator[T]) {
	for k := range other.Iter() {
		if _, ok := s[k]; !ok {
			s[k] = struct{}{}
		} else {
			delete(s, k)
		}
	}
}

// AglSetIsSubset returns a Boolean value that indicates whether the set is a subset of the given sequence.
// Return: true if the set is a subset of possibleSuperset; otherwise, false.
// Set A is a subset of another set B if every member of A is also a member of B.
func AglSetIsSubset[T comparable](s, other AglSet[T]) bool {
	for k := range s {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	return true
}

// AglSetIsStrictSubset returns a Boolean value that indicates whether the set is a strict subset of the given sequence.
// Set A is a strict subset of another set B if every member of A is also a member of B and B contains at least one element that is not a member of A.
func AglSetIsStrictSubset[T comparable](s, other AglSet[T]) bool {
	for k := range s {
		if _, ok := other[k]; !ok {
			return false
		}
	}
	for k := range other {
		if _, ok := s[k]; !ok {
			return true
		}
	}
	return false
}

// AglSetIsSuperset returns a Boolean value that indicates whether this set is a superset of the given set.
// Return: true if the set is a superset of other; otherwise, false.
// Set A is a superset of another set B if every member of B is also a member of A.
func AglSetIsSuperset[T comparable](s AglSet[T], other Iterator[T]) bool {
	for k := range other.Iter() {
		if _, ok := s[k]; !ok {
			return false
		}
	}
	return true
}

// AglSetIsStrictSuperset returns a Boolean value that indicates whether the set is a strict superset of the given sequence.
// Set A is a strict superset of another set B if every member of B is also a member of A and A contains at least one element that is not a member of B.
func AglSetIsStrictSuperset[T comparable](s, other AglSet[T]) bool {
	for k := range other {
		if _, ok := s[k]; !ok {
			return false
		}
	}
	for k := range s {
		if _, ok := other[k]; !ok {
			return true
		}
	}
	return false
}

// AglSetIsDisjoint returns a Boolean value that indicates whether the set has no members in common with the given sequence.
// Return: true if the set has no elements in common with other; otherwise, false.
func AglSetIsDisjoint[T comparable](s AglSet[T], other Iterator[T]) bool {
	var otherSet AglSet[T]
	if v, ok := other.(AglSet[T]); ok {
		otherSet = v
	} else {
		otherSet = make(AglSet[T])
		for e := range other.Iter() {
			otherSet[e] = struct{}{}
		}
	}
	for k := range s {
		if _, ok := otherSet[k]; ok {
			return false
		}
	}
	return true
}

// AglSetIntersects ...
func AglSetIntersects[T comparable](s AglSet[T], other Iterator[T]) bool {
	return !AglSetIsDisjoint(s, other)
}

func AglStringLen(s string) int {
	return len(s)
}

func AglStringLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func AglStringReplace(s string, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}

func AglStringReplaceAll(s string, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

func AglStringTrimSpace(s string) string {
	return strings.TrimSpace(s)
}

func AglStringTrimPrefix(s string, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

func AglStringHasPrefix(s string, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func AglStringContains(s string, substr string) bool {
	return strings.Contains(s, substr)
}

func AglStringHasSuffix(s string, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

func AglStringSplit(s string, sep string) []string {
	return strings.Split(s, sep)
}

func AglStringLowercased(s string) string {
	return strings.ToLower(s)
}

func AglStringUppercased(s string) string {
	return strings.ToUpper(s)
}

func AglStringAsBytes(s string) []byte {
	return []byte(s)
}

func AglCleanupIntString(s string) (string, int) {
	s = strings.ReplaceAll(s, "_", "")
	var base int
	switch {
	case strings.HasPrefix(s, "0b"):
		s, base = s[2:], 2
	case strings.HasPrefix(s, "0o"):
		s, base = s[2:], 8
	case strings.HasPrefix(s, "0x"):
		s, base = s[2:], 16
	default:
		base = 10
	}
	return s, base
}

func AglStringInt(s string) Option[int] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseInt(s, base, 0)
	if err != nil {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(int(v))
}

func AglStringI8(s string) Option[int8] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseInt(s, base, 8)
	if err != nil {
		return MakeOptionNone[int8]()
	}
	return MakeOptionSome(int8(v))
}

func AglStringI16(s string) Option[int16] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseInt(s, base, 16)
	if err != nil {
		return MakeOptionNone[int16]()
	}
	return MakeOptionSome(int16(v))
}

func AglStringI32(s string) Option[int32] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseInt(s, base, 32)
	if err != nil {
		return MakeOptionNone[int32]()
	}
	return MakeOptionSome(int32(v))
}

func AglStringI64(s string) Option[int64] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseInt(s, base, 64)
	if err != nil {
		return MakeOptionNone[int64]()
	}
	return MakeOptionSome(v)
}

func AglStringUint(s string) Option[uint] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseUint(s, base, 0)
	if err != nil {
		return MakeOptionNone[uint]()
	}
	return MakeOptionSome(uint(v))
}

func AglStringU8(s string) Option[uint8] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseUint(s, base, 8)
	if err != nil {
		return MakeOptionNone[uint8]()
	}
	return MakeOptionSome(uint8(v))
}

func AglStringU16(s string) Option[uint16] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseUint(s, base, 16)
	if err != nil {
		return MakeOptionNone[uint16]()
	}
	return MakeOptionSome(uint16(v))
}

func AglStringU32(s string) Option[uint32] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return MakeOptionNone[uint32]()
	}
	return MakeOptionSome(uint32(v))
}

func AglStringU64(s string) Option[uint64] {
	s, base := AglCleanupIntString(s)
	v, err := strconv.ParseUint(s, base, 64)
	if err != nil {
		return MakeOptionNone[uint64]()
	}
	return MakeOptionSome(uint64(v))
}

func AglStringF32(s string) Option[float32] {
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return MakeOptionNone[float32]()
	}
	return MakeOptionSome(float32(v))
}

func AglStringF64(s string) Option[float64] {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return MakeOptionNone[float64]()
	}
	return MakeOptionSome(v)
}

func AglVecSorted[E cmp.Ordered](a []E) []E {
	return slices.Sorted(slices.Values(a))
}

func AglVecSortedBy[E any](a []E, f func(a, b E) bool) []E {
	sort.Slice(a, func(i, j int) bool {
		return f(a[i], a[j])
	})
	return a
}

func AglVecJoined(a []string, s string) string {
	return strings.Join(a, s)
}

func AglVecSum[T cmp.Ordered](a []T) (out T) {
	var zero T
	return AglVecReduce(a, zero, func(acc, el T) T { return acc + el })
}

func AglAbs[T Number](e T) (out T) {
	return T(math.Abs(float64(e)))
}

func AglVecLast[T any](a []T) (out Option[T]) {
	if len(a) > 0 {
		return MakeOptionSome(a[len(a)-1])
	}
	return MakeOptionNone[T]()
}

func AglVecFirst[T any](a []T) (out Option[T]) {
	if len(a) > 0 {
		return MakeOptionSome(a[0])
	}
	return MakeOptionNone[T]()
}

func AglVecFirstWhere[T any](a []T, predicate func(T) bool) (out Option[T]) {
	if len(a) == 0 {
		return MakeOptionNone[T]()
	}
	for _, el := range a {
		if predicate(el) {
			return MakeOptionSome(el)
		}
	}
	return MakeOptionNone[T]()
}

func AglVecGet[T any](a []T, i int) (out Option[T]) {
	if i >= 0 && i <= len(a)-1 {
		return MakeOptionSome(a[i])
	}
	return MakeOptionNone[T]()
}

func AglVecWith[T any](a *[]T, i int, clb func(*T) AglVoid) {
	el := (*a)[i]
	clb(&el)
	(*a)[i] = el
}

func AglVecLen[T any](a []T) int {
	return len(a)
}

func AglVecIsEmpty[T any](a []T) bool {
	return len(a) == 0
}

func AglVecPush[T any](a *[]T, els ...T) {
	*a = append(*a, els...)
}

// AglVecPushFront ...
func AglVecPushFront[T any](a *[]T, el T) {
	*a = append([]T{el}, *a...)
}

// AglVecPopFront ...
func AglVecPopFront[T any](a *[]T) Option[T] {
	if len(*a) == 0 {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[0], (*a)[1:]
	return MakeOptionSome(el)
}

func AglVecRemoveFirst[T any](a *[]T) T {
	res := AglVecPopFront(a)
	if res.IsNone() {
		panic("Vec is empty")
	}
	return res.Unwrap()
}

// AglVecSwap ...
func AglVecSwap[T any, I, J Integer](a *[]T, b I, c J) {
	(*a)[b], (*a)[c] = (*a)[c], (*a)[b]
}

// AglVecInsert ...
func AglVecInsert[T any](a *[]T, idx int, el T) {
	*a = append((*a)[:idx], append([]T{el}, (*a)[idx:]...)...)
}

// AglVecRemove ...
func AglVecRemove[T any](a *[]T, idx int) {
	*a = append((*a)[:idx], (*a)[idx+1:]...)
}

// AglVecClone ...
func AglVecClone[S ~[]E, E any](a S) S {
	return slices.Clone(a)
}

// AglVecClear ...
func AglVecClear[T any](a *[]T) {
	*a = (*a)[:0]
}

// AglVecIndices ...
func AglVecIndices[T any](a []T) []int {
	out := make([]int, len(a))
	for i := range a {
		out[i] = i
	}
	return out
}

// AglVecPop removes the last element from a vector and returns it, or None if it is empty.
func AglVecPop[T any](a *[]T) Option[T] {
	if len(*a) == 0 {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[len(*a)-1], (*a)[:len(*a)-1]
	return MakeOptionSome(el)
}

// AglVecPopIf Removes and returns the last element from a vector if the predicate returns true,
// or None if the predicate returns false or the vector is empty (the predicate will not be called in that case).
func AglVecPopIf[T any](a *[]T, pred func() bool) Option[T] {
	if len(*a) == 0 || !pred() {
		return MakeOptionNone[T]()
	}
	var el T
	el, *a = (*a)[len(*a)-1], (*a)[:len(*a)-1]
	return MakeOptionSome(el)
}

func AglMapLen[K comparable, V any](m map[K]V) int {
	return len(m)
}

func AglMapIndex[K comparable, V any](m map[K]V, index K) Option[V] {
	if el, ok := m[index]; ok {
		return MakeOptionSome(el)
	}
	return MakeOptionNone[V]()
}

func AglMapContainsKey[K comparable, V any](m map[K]V, k K) bool {
	_, ok := m[k]
	return ok
}

func AglMapFilter[K comparable, V any](m map[K]V, f func(DictEntry[K, V]) bool) map[K]V {
	out := make(map[K]V)
	for k, v := range m {
		if f(DictEntry[K, V]{Key: k, Value: v}) {
			out[k] = v
		}
	}
	return out
}

func AglMapMap[K comparable, V, R any](m map[K]V, f func(DictEntry[K, V]) R) []R {
	var out []R
	for k, v := range m {
		out = append(out, f(DictEntry[K, V]{Key: k, Value: v}))
	}
	return out
}

func AglMapKeys[K comparable, V any](m map[K]V) Sequence[K] {
	return Sequence[K](maps.Keys(m))
}

func AglMapValues[K comparable, V any](m map[K]V) Sequence[V] {
	return Sequence[V](maps.Values(m))
}

func AglHttpNewRequest(method, url string, b Option[io.Reader]) Result[*http.Request] {
	var body io.Reader
	if b.IsSome() {
		body = b.Unwrap()
	}
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return MakeResultErr[*http.Request](err)
	}
	return MakeResultOk(req)
}

type Set[T comparable] struct {
	values map[T]struct{}
}

func (s *Set[T]) String() string {
	var vals []string
	for k := range s.values {
		vals = append(vals, fmt.Sprintf("%v", k))
	}
	return "{" + strings.Join(vals, " ") + "}"
}

func (s *Set[T]) Len() int {
	return len(s.values)
}

// Insert Adds a value to the set.
//
// Returns whether the value was newly inserted. That is:
//
// - If the set did not previously contain this value, true is returned.
// - If the set already contained this value, false is returned, and the set is not modified: original value is not replaced, and the value passed as argument is dropped.
func (s *Set[T]) Insert(value T) bool {
	if _, ok := s.values[value]; ok {
		return false
	}
	s.values[value] = struct{}{}
	return true
}

func AglNewSet[T comparable](els ...T) *Set[T] {
	s := &Set[T]{values: make(map[T]struct{})}
	for _, el := range els {
		s.values[el] = struct{}{}
	}
	return s
}

func AglIntString(v int) string   { return strconv.FormatInt(int64(v), 10) }
func AglI8String(v int8) string   { return strconv.FormatInt(int64(v), 10) }
func AglI16String(v int16) string { return strconv.FormatInt(int64(v), 10) }
func AglI32String(v int32) string { return strconv.FormatInt(int64(v), 10) }
func AglI64String(v int64) string { return strconv.FormatInt(int64(v), 10) }
func AglUintString(v uint) string { return strconv.FormatInt(int64(v), 10) }

func AglIn[T comparable](e T, it Iterator[T]) bool {
	for el := range it.Iter() {
		if el == e {
			return true
		}
	}
	return false
}

func AglBuildSet[T comparable](it Iterator[T]) (out AglSet[T]) {
	out = make(AglSet[T])
	for el := range it.Iter() {
		AglSetInsert(out, el)
	}
	return
}
