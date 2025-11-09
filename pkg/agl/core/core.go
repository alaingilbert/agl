package main

import (
	"bytes"
	"cmp"
	"encoding/binary"
	"fmt"
	"hash/fnv"
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
	return AglBuildArray(AglIteratorMap(AglVec[T](a).Iter(), f))
}

func AglVecFilterMap[T, R any](a []T, f func(T) Option[R]) []R {
	return AglBuildArray(AglIteratorFilterMap(AglVec[T](a).Iter(), f))
}

func AglVec__ADD[T any](a, b []T) []T {
	out := make([]T, 0, len(a)+len(b))
	out = append(out, a...)
	out = append(out, b...)
	return out
}

func AglVecFilter[T any](a []T, f func(T) bool) []T {
	return AglBuildArray(AglIteratorFilter(AglVec[T](a).Iter(), f))
}

func AglVecDropFirst[T any](a []T, k int) []T {
	if k <= 0 {
		return a
	}
	if k >= len(a) {
		return []T{}
	}
	return a[k:]
}

func AglVecDropLast[T any](a []T, k int) []T {
	if k <= 0 {
		return a
	}
	if k >= len(a) {
		return []T{}
	}
	return a[:len(a)-k]
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
	return AglIteratorAllSatisfy(AglVec[T](a).Iter(), f)
}

func AglVecContains[T comparable](a []T, e T) bool {
	return AglIteratorContains(AglVec[T](a).Iter(), e)
}

func AglVecContainsWhere[T comparable](a []T, p func(T) bool) bool {
	return AglIteratorContainsWhere(AglVec[T](a).Iter(), p)
}

func AglVecAny[T any](a []T, f func(T) bool) bool {
	return AglIteratorContainsWhere(AglVec[T](a).Iter(), f)
}

func AglVecReduce[T, R any](a []T, acc R, f func(R, T) R) R {
	return AglIteratorReduce(AglVec[T](a).Iter(), acc, f)
}

func AglVecReduceInto[T, R any](a []T, acc R, f func(*R, T) AglVoid) R {
	return AglIteratorReduceInto(AglVec[T](a).Iter(), acc, f)
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

func AglPrint(a ...any) {
	fmt.Println(a...)
}

func AglPrintln(a ...any) {
	fmt.Println(a...)
}

func AglPrintf(format string, a ...any) {
	fmt.Printf(format+"\n", a...)
}

func AglPrecondition(pred bool, msg ...string) {
	if !pred {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		panic(m)
	}
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

func AglAssertEq[T comparable](left, right T, msg ...string) {
	if left != right {
		m := ""
		if len(msg) > 0 {
			m = msg[0]
		}
		m = fmt.Sprintf("%s, left: %v, right: %v", m, left, right)
		panic(m)
	}
}

func AglVecIn[T comparable](a []T, v T) bool {
	for _, el := range a {
		if el == v {
			return true
		}
	}
	return false
}

func AglNoop(_ ...any) {}

func AglTypeAssert[T any](v any) Option[T] {
	if v, ok := v.(T); ok {
		return MakeOptionSome(v)
	}
	return MakeOptionNone[T]()
}

func AglIdentity[T any](v T) T { return v }

func AglVecFind[T any](a []T, f func(T) bool) Option[T] {
	return AglIteratorFind(AglVec[T](a).Iter(), f)
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

func (v *VecIter[T]) Clone() CloneIterator[T] {
	return &VecIter[T]{v: v.v, i: v.i}
}

type SeqIter[T any] struct {
	next func() (T, bool)
	stop func()
}

func (s *SeqIter[T]) Next() Option[T] {
	e, ok := s.next()
	if !ok {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome(e)
}

// Sequence anything that can be turned into an Iterator
type Sequence[T any] iter.Seq[T]

func (s Sequence[T]) Iter() Iterator[T] {
	next, stop := iter.Pull(iter.Seq[T](s))
	return &SeqIter[T]{next: next, stop: stop}
}

func AglSequenceSum[T, R Number](s Sequence[T]) (out R) {
	for e := range s {
		out += R(e)
	}
	return out
}

func AglIteratorSum[T, R Number](it Iterator[T]) (out R) {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		out += R(v.Unwrap())
	}
	return out
}

func AglSequenceLen[T any](s Sequence[T]) (out int) {
	for range s {
		out++
	}
	return
}

func AglSequenceMax[T cmp.Ordered](s Sequence[T]) Option[T] {
	var out Option[T]
	for v := range s {
		if !out.IsNone() {
			v = max(out.Unwrap(), v)
		}
		out = MakeOptionSome(v)
	}
	return out
}

func AglSequenceMin[T cmp.Ordered](s Sequence[T]) Option[T] {
	var out Option[T]
	for v := range s {
		if !out.IsNone() {
			v = min(out.Unwrap(), v)
		}
		out = MakeOptionSome(v)
	}
	return out
}

// AglSequenceInspect does something with each element of an iterator, passing the value on.
func AglSequenceInspect[T any](s Sequence[T], f func(T) AglVoid) Sequence[T] {
	return func(yield func(T) bool) {
		for v := range s {
			f(v)
			if !yield(v) {
				return
			}
		}
	}
}

// AglSequenceLast consumes the iterator, returning the last element.
func AglSequenceLast[T any](s Sequence[T]) Option[T] {
	out := MakeOptionNone[T]()
	for v := range s {
		out = MakeOptionSome(v)
	}
	return out
}

// AglSequenceNth returns the nth element of the iterator.
func AglSequenceNth[T any](s Sequence[T], n int) Option[T] {
	pos := 0
	for v := range s {
		if pos == n {
			return MakeOptionSome(v)
		}
		pos++
	}
	return MakeOptionNone[T]()
}

type Chain[T any] struct {
	a Iterator[T]
	b Iterator[T]
}

func (i *Chain[T]) Next() Option[T] {
	v := i.a.Next()
	if v.IsNone() {
		return i.b.Next()
	}
	return v
}

// AglIteratorChain takes two iterators and creates a new iterator over both in sequence.
func AglIteratorChain[T any](a, b Iterator[T]) *Chain[T] {
	return &Chain[T]{a: a, b: b}
}

// AglSequenceChain takes two iterators and creates a new iterator over both in sequence.
func AglSequenceChain[T any](s, other Sequence[T]) Sequence[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if !yield(v) {
				return
			}
		}
		for v := range other {
			if !yield(v) {
				return
			}
		}
	}
}

// AglSequenceStepBy creates an iterator starting at the same point, but stepping by the given amount at each iteration.
func AglSequenceStepBy[T any](s Sequence[T], step int) Sequence[T] {
	return func(yield func(T) bool) {
		pos := 0
		for v := range s {
			if pos%step == 0 {
				if !yield(v) {
					return
				}
			}
			pos++
		}
	}
}

// AglSequenceIntersperse creates a new iterator which places a copy of separator between adjacent items of the original iterator.
func AglSequenceIntersperse[T any](s Sequence[T], separator T) Sequence[T] {
	return func(yield func(T) bool) {
		pos := 0
		for v := range s {
			if pos > 0 {
				if !yield(separator) {
					return
				}
			}
			if !yield(v) {
				return
			}
			pos++
		}
	}
}

// AglSequenceTakeWhile creates an iterator that yields elements based on a predicate.
func AglSequenceTakeWhile[T any](s Sequence[T], f func(T) bool) Sequence[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if f(v) {
				if !yield(v) {
					return
				}
			} else {
				return
			}
		}
	}
}

type FilterMap[T, R any] struct {
	iter Iterator[T]
	f    func(T) Option[R]
}

func (i *FilterMap[T, R]) Next() Option[R] {
	for {
		v := i.iter.Next()
		if v.IsNone() {
			break
		}
		res := i.f(v.Unwrap())
		if res.IsSome() {
			return res
		}
	}
	return MakeOptionNone[R]()
}

func AglIteratorFilterMap[T, R any](it Iterator[T], f func(T) Option[R]) *FilterMap[T, R] {
	return &FilterMap[T, R]{iter: it, f: f}
}

func AglIteratorCompactMap[T, R any](it Iterator[T], f func(T) Option[R]) Iterator[R] {
	return AglIteratorFilterMap(it, f)
}

func AglSequenceFlatMap[T, R any](s Sequence[T], f func(T) []R) Sequence[R] {
	return func(yield func(R) bool) {
		for el := range s {
			subArr := f(el)
			for _, el1 := range subArr {
				if !yield(el1) {
					return
				}
			}
		}
	}
}

func AglSequenceFilter[T any](s Sequence[T], f func(T) bool) Sequence[T] {
	return func(yield func(T) bool) {
		for v := range s {
			if f(v) {
				if !yield(v) {
					return
				}
			}
		}
	}
}

// AglIteratorFind searches for an element of an iterator that satisfies a predicate.
func AglIteratorFind[T any](it Iterator[T], f func(T) bool) Option[T] {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		if f(v.Unwrap()) {
			return MakeOptionSome(v.Unwrap())
		}
	}
	return MakeOptionNone[T]()
}

// AglSequenceFindMap applies function to the elements of iterator and returns the first non-none result.
func AglSequenceFindMap[T, R any](s Sequence[T], f func(T) Option[R]) Option[R] {
	for v := range s {
		res := f(v)
		if res.IsSome() {
			return MakeOptionSome(res.Unwrap())
		}
	}
	return MakeOptionNone[R]()
}

// AglSequencePosition searches for an element in an iterator, returning its index.
func AglSequencePosition[T any](s Sequence[T], f func(T) bool) Option[int] {
	pos := 0
	for v := range s {
		if f(v) {
			return MakeOptionSome(pos)
		}
		pos++
	}
	return MakeOptionNone[int]()
}

func AglIteratorPosition[T any, I Iterator[T]](it I, f func(T) bool) Option[int] {
	pos := 0
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		if f(v.Unwrap()) {
			return MakeOptionSome(pos)
		}
		pos++
	}
	return MakeOptionNone[int]()
}

func AglIteratorRPosition[T any, I DoubleEndedExactSizeIterator[T]](it I, f func(T) bool) Option[int] {
	pos := 0
	for {
		v := it.NextBack()
		if v.IsNone() {
			break
		}
		if f(v.Unwrap()) {
			return MakeOptionSome(it.Len() - pos)
		}
		pos++
	}
	return MakeOptionNone[int]()
}

type Cycle[T any] struct {
	orig CloneIterator[T]
	iter Iterator[T]
	pos  int
}

func (i *Cycle[T]) Next() Option[T] {
	v := i.iter.Next()
	if v.IsNone() {
		i.iter = i.orig.Clone()
		v = i.iter.Next()
	}
	return MakeOptionSome(v.Unwrap())
}

func AglIteratorCycle[T any](it CloneIterator[T]) *Cycle[T] {
	return &Cycle[T]{orig: it.Clone(), iter: it}
}

func AglSequenceContains[T comparable](s Sequence[T], e T) bool {
	return AglSequenceContainsWhere(s, func(t T) bool { return t == e })
}

func AglSequenceContainsWhere[T any](s Sequence[T], pred func(T) bool) bool {
	for v := range s {
		if pred(v) {
			return true
		}
	}
	return false
}

func AglSequenceAllSatisfy[T any](s Sequence[T], pred func(T) bool) bool {
	for v := range s {
		if !pred(v) {
			return false
		}
	}
	return true
}

func AglSequenceAny[T any](s Sequence[T], f func(T) bool) bool {
	return AglSequenceContainsWhere(s, f)
}

func AglSequenceMap[T, R any](s Sequence[T], f func(T) R) []R {
	var out []R
	for v := range s {
		out = append(out, f(v))
	}
	return out
}

func AglIteratorReduce[T, R any](it Iterator[T], acc R, f func(R, T) R) R {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		acc = f(acc, v.Unwrap())
	}
	return acc
}

func AglIteratorReduceInto[T, R any](it Iterator[T], acc R, f func(*R, T) AglVoid) R {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		f(&acc, v.Unwrap())
	}
	return acc
}

func AglSequenceForEach[T any](s Sequence[T], f func(T) AglVoid) {
	for v := range s {
		f(v)
	}
}

func AglSequenceSorted[T cmp.Ordered](s Sequence[T]) []T {
	return slices.Sorted(iter.Seq[T](s))
}

func AglIteratorSorted[T cmp.Ordered](it Iterator[T]) []T {
	arr := AglBuildArray(it)
	slices.Sort(arr)
	return arr
}

func AglIteratorJoined(s Iterator[string], sep string) string {
	return strings.Join(AglBuildArray(s), sep)
}

type AglVecEq[T Equatable[T]] []T

func (v AglVecEq[T]) __EQ(rhs AglVecEq[T]) bool {
	if len(v) != len(rhs) {
		return false
	}
	for i := range v {
		if !v[i].__EQ(rhs[i]) {
			return false
		}
	}
	return true
}

type IterVec[T any] struct {
	v     []T
	start int
	end   int
}

func (i *IterVec[T]) Clone() CloneIterator[T] {
	return &IterVec[T]{v: i.v, start: i.start, end: i.end}
}

func (i *IterVec[T]) Len() int {
	return max(i.end-i.start, 0)
}

func (i *IterVec[T]) IsEmpty() bool {
	return i.Len() == 0
}

func (i *IterVec[T]) Next() Option[T] {
	if i.start >= i.end {
		return MakeOptionNone[T]()
	}
	e := i.v[i.start]
	i.start++
	return MakeOptionSome(e)
}

func (i *IterVec[T]) NextBack() Option[T] {
	i.end--
	if i.end < i.start {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome(i.v[i.end])
}

type AglVec[T any] []T

func (v AglVec[T]) Len() int { return len(v) }

func (v AglVec[T]) Iter() *IterVec[T] {
	return &IterVec[T]{v: v, start: 0, end: len(v)}
}

func (v AglVec[T]) __EQ(rhs AglVec[T]) bool {
	if len(v) != len(rhs) {
		return false
	}
	for i := range v {
		if any(v[i]) != any(rhs[i]) {
			return false
		}
	}
	return true
}

func AglVecIter[T any](v AglVec[T]) *IterVec[T] {
	return v.Iter()
}

type Peekable[T any] struct {
	iter Iterator[T]
	el   Option[T]
}

func (p *Peekable[T]) Peek() Option[T] {
	if p.el.IsSome() {
		return p.el
	}
	p.el = p.iter.Next()
	return p.el
}

func (p *Peekable[T]) Next() Option[T] {
	if p.el.IsSome() {
		cur := p.el
		p.el = MakeOptionNone[T]()
		return cur
	}
	return p.iter.Next()
}

func AglIteratorPeekable[T any](it Iterator[T]) *Peekable[T] {
	return &Peekable[T]{iter: it}
}

type DictEntry[K comparable, V any] struct {
	Key   K
	Value V
}

type IterMap[K comparable, V any] struct {
	m    map[K]V
	next func() (DictEntry[K, V], bool)
	stop func()
}

func (i *IterMap[K, V]) Next() Option[DictEntry[K, V]] {
	e, ok := i.next()
	if !ok {
		i.stop()
		return MakeOptionNone[DictEntry[K, V]]()
	}
	return MakeOptionSome(e)
}

type IterMapDrain[K comparable, V any] struct {
	next func() (DictEntry[K, V], bool)
	stop func()
}

func (i *IterMapDrain[K, V]) Next() Option[DictEntry[K, V]] {
	e, ok := i.next()
	if !ok {
		i.stop()
		return MakeOptionNone[DictEntry[K, V]]()
	}
	return MakeOptionSome(e)
}

type AglMap[K comparable, V any] map[K]V

func (m AglMap[K, V]) Iter() Iterator[DictEntry[K, V]] {
	seq := func(yield func(DictEntry[K, V]) bool) {
		for k := range m {
			if !yield(DictEntry[K, V]{Key: k, Value: m[k]}) {
				return
			}
		}
	}
	next, stop := iter.Pull(seq)
	return &IterMap[K, V]{m: m, next: next, stop: stop}
}

func (m AglMap[K, V]) Len() int { return len(m) }

type Hashable[T any] interface {
	Equatable[T]
	Hash() uint64
}

type Equatable[T any] interface {
	__EQ(rhs T) bool
}

type ExpressibleByStringLiteral[T any] interface {
	FromStringLit(string) T
}

type AglInt struct{ int }

func (i AglInt) __EQ(rhs AglInt) bool { return i == rhs }

func (i AglInt) Hash() uint64 {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i.int))
	return fnvHash(buf)
}

type AglI8 struct{ int8 }

func (i AglI8) __EQ(rhs AglI8) bool { return i == rhs }

func (i AglI8) Hash() uint64 {
	return fnvHash([]byte{byte(i.int8)})
}

type AglI16 struct{ int16 }

func (i AglI16) __EQ(rhs AglI16) bool { return i == rhs }

func (i AglI16) Hash() uint64 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(i.int16))
	return fnvHash(buf)
}

type AglI32 struct{ int32 }

func (i AglI32) __EQ(rhs AglI32) bool { return i == rhs }

func (i AglI32) Hash() uint64 {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(i.int32))
	return fnvHash(buf)
}

type AglI64 struct{ int64 }

func (i AglI64) __EQ(rhs AglI64) bool { return i == rhs }

func (i AglI64) Hash() uint64 {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i.int64))
	return fnvHash(buf)
}

type AglUint struct{ uint }

func (i AglUint) __EQ(rhs AglUint) bool { return i == rhs }

func (i AglUint) Hash() uint64 {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(i.uint))
	return fnvHash(buf)
}

type AglU8 struct{ uint8 }

func (i AglU8) __EQ(rhs AglU8) bool { return i == rhs }

func (i AglU8) Hash() uint64 {
	return fnvHash([]byte{i.uint8})
}

type AglU16 struct{ uint16 }

func (i AglU16) __EQ(rhs AglU16) bool { return i == rhs }

func (i AglU16) Hash() uint64 {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, i.uint16)
	return fnvHash(buf)
}

type AglU32 struct{ uint32 }

func (i AglU32) __EQ(rhs AglU32) bool { return i == rhs }

func (i AglU32) Hash() uint64 {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, i.uint32)
	return fnvHash(buf)
}

type AglU64 struct{ uint64 }

func (i AglU64) __EQ(rhs AglU64) bool { return i == rhs }

func (i AglU64) Hash() uint64 {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, i.uint64)
	return fnvHash(buf)
}

type AglF32 struct{ float32 }

func (f AglF32) __EQ(rhs AglF32) bool { return f == rhs }

func (f AglF32) Hash() uint64 {
	bits := math.Float32bits(f.float32)
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, bits)
	return fnvHash(buf)
}

type AglF64 struct{ float64 }

func (f AglF64) __EQ(rhs AglF64) bool { return f == rhs }

func (f AglF64) Hash() uint64 {
	bits := math.Float64bits(f.float64)
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, bits)
	return fnvHash(buf)
}

type AglString struct{ string }

func (s AglString) __EQ(rhs AglString) bool { return s == rhs }

func (s AglString) Hash() uint64 {
	return fnvHash([]byte(s.string))
}

func fnvHash(by []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(by)
	return h.Sum64()
}

type DictEntry1[K Hashable[K], V any] struct {
	Key   K
	Value V
}

type AglMap1[K Hashable[K], V any] struct {
	data map[uint64]DictEntry1[K, V]
}

type AglSet1[H Hashable[H]] struct {
	data map[uint64]H
}

func (s *AglSet1[H]) Insert(e H) bool {
	if s.data == nil {
		s.data = make(map[uint64]H)
	}
	eH := e.Hash()
	if _, ok := s.data[eH]; ok {
		return false
	}
	s.data[eH] = e
	return true
}

func (s AglSet1[H]) __EQ(rhs AglSet1[H]) bool {
	if len(s.data) != len(rhs.data) {
		return false
	}
	for _, v := range s.data {
		if !rhs.Contains(v) {
			return false
		}
	}
	return true
}

func (s AglSet1[H]) Hash() uint64 {
	hashes := make([]uint64, len(s.data))
	i := 0
	for k := range s.data {
		hashes[i] = k
		i++
	}
	sort.Slice(hashes, func(i, j int) bool { return hashes[i] < hashes[j] })
	var final uint64
	for _, h := range hashes {
		final ^= h
	}
	return final
}

func (s *AglSet1[H]) Contains(e H) bool {
	_, ok := s.data[e.Hash()]
	return ok
}

type AglSet[T comparable] map[T]struct{}

func (s AglSet[T]) Len() int { return len(s) }

type AglRangeFrom[T Integer] struct {
	Start T
}

func AglNewRangeFrom[T Integer](start T) *AglRangeFrom[T] {
	return &AglRangeFrom[T]{Start: start}
}

func (r *AglRangeFrom[T]) Next() Option[T] {
	res := MakeOptionSome(r.Start)
	r.Start++
	return res
}

func (r *AglRangeFrom[T]) Iter() Sequence[T] {
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

type AglRangeInclusive[T Integer] struct {
	Start, End T
}

func AglNewRangeInclusive[T Integer](start, end T) *AglRangeInclusive[T] {
	return &AglRangeInclusive[T]{Start: start, End: end}
}

func (r *AglRangeInclusive[T]) Next() Option[T] {
	if r.Start > r.End {
		return MakeOptionNone[T]()
	}
	res := MakeOptionSome(r.Start)
	r.Start++
	return res
}

func (r *AglRangeInclusive[T]) NextBack() Option[T] {
	if r.Start > r.End {
		return MakeOptionNone[T]()
	}
	res := MakeOptionSome(r.End)
	r.End--
	return res
}

func (r *AglRangeInclusive[T]) Seq() Sequence[T] {
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

func (r *AglRangeInclusive[T]) Rev() *Rev[T] {
	return AglDoubleEndedIteratorRev[T](r)
}

type AglRange[T Integer] struct {
	Start, End T
}

func AglNewRange[T Integer](start, end T) *AglRange[T] {
	return &AglRange[T]{Start: start, End: end}
}

func (r *AglRange[T]) AllSatisfy(pred func(T) bool) bool {
	for {
		if el := r.Next(); el.IsSome() {
			if !pred(el.Unwrap()) {
				return false
			}
		} else {
			return true
		}
	}
}

func (r *AglRange[T]) Next() Option[T] {
	if r.Start >= r.End {
		return MakeOptionNone[T]()
	}
	res := MakeOptionSome(r.Start)
	r.Start++
	return res
}

func (r *AglRange[T]) NextBack() Option[T] {
	if r.Start >= r.End {
		return MakeOptionNone[T]()
	}
	r.End--
	return MakeOptionSome(r.End)
}

func (r *AglRange[T]) Seq() Sequence[T] {
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

func (r *AglRange[T]) Rev() *Rev[T] {
	return AglDoubleEndedIteratorRev[T](r)
}

type IntoIterator[T any] interface {
	Iter() Iterator[T]
}

type Iterator[T any] interface {
	Next() Option[T]
}

type IteratorIntoIterator[T any] struct {
	iter Iterator[T]
}

func (w *IteratorIntoIterator[T]) Iter() Iterator[T] {
	return w.iter
}

func AglIteratorIntoIterator[T any](it Iterator[T]) *IteratorIntoIterator[T] {
	return &IteratorIntoIterator[T]{iter: it}
}

type CloneIterator[T any] interface {
	Iterator[T]
	Clone() CloneIterator[T]
}

type DoubleEndedIterator[T any] interface {
	Iterator[T]
	NextBack() Option[T]
}

type ExactSizeIterator[T any] interface {
	Iterator[T]
	Len() int      // Returns the exact remaining length of the iterator.
	IsEmpty() bool // Returns true if the iterator is empty.
}

type DoubleEndedExactSizeIterator[T any] interface {
	DoubleEndedIterator[T]
	ExactSizeIterator[T]
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

func AglIteratorAllSatisfy[T any, I Iterator[T]](it I, pred func(T) bool) bool {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		if !pred(v.Unwrap()) {
			return false
		}
	}
	return true
}

func AglIteratorContains[T comparable, I Iterator[T]](it I, el T) bool {
	return AglIteratorContainsWhere(it, func(v T) bool { return el == v })
}

func AglIteratorContainsWhere[T any, I Iterator[T]](it I, f func(T) bool) bool {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		if f(v.Unwrap()) {
			return true
		}
	}
	return false
}

func AglIteratorForEach[T any, I Iterator[T]](it I, f func(T) AglVoid) {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		f(v.Unwrap())
	}
}

type Filter[T any] struct {
	iter      Iterator[T]
	predicate func(T) bool
}

func (i *Filter[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		if i.predicate(n.Unwrap()) {
			return n
		}
	}
	return MakeOptionNone[T]()
}

func AglIteratorFilter[T any, I Iterator[T]](it I, f func(T) bool) *Filter[T] {
	return &Filter[T]{iter: it, predicate: f}
}

type Take[T any] struct {
	iter Iterator[T]
	n    int
	i    int
}

func (i *Take[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() || i.i >= i.n {
			break
		}
		i.i++
		return n
	}
	return MakeOptionNone[T]()
}

// AglIteratorTake creates an iterator that yields the first n elements, or fewer if the underlying iterator ends sooner.
func AglIteratorTake[T any](it Iterator[T], n int) *Take[T] {
	return &Take[T]{iter: it, n: n}
}

type TakeWhile[T any] struct {
	iter      Iterator[T]
	predicate func(T) bool
}

func (i *TakeWhile[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() || !i.predicate(n.Unwrap()) {
			break
		}
		return n
	}
	return MakeOptionNone[T]()
}

// AglIteratorTakeWhile creates an iterator that yields elements based on a predicate.
func AglIteratorTakeWhile[T any](it Iterator[T], f func(T) bool) *TakeWhile[T] {
	return &TakeWhile[T]{iter: it, predicate: f}
}

type Skip[T any] struct {
	iter Iterator[T]
	n    int
	i    int
}

func (i *Skip[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		if i.i < i.n {
			i.i++
			continue
		}
		return n
	}
	return MakeOptionNone[T]()
}

// AglIteratorSkip creates an iterator that skips the first n elements.
func AglIteratorSkip[T any](it Iterator[T], n int) *Skip[T] {
	return &Skip[T]{iter: it, n: n}
}

type SkipWhile[T any] struct {
	iter      Iterator[T]
	predicate func(T) bool
	found     bool
}

func (i *SkipWhile[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		if i.predicate(n.Unwrap()) && !i.found {
			continue
		}
		i.found = true
		return n
	}
	return MakeOptionNone[T]()
}

// AglIteratorSkipWhile creates an iterator that skips elements based on a predicate.
func AglIteratorSkipWhile[T any](it Iterator[T], f func(T) bool) *SkipWhile[T] {
	return &SkipWhile[T]{iter: it, predicate: f}
}

type StepBy[T any] struct {
	iter Iterator[T]
	step int
	i    int
}

func (i *StepBy[T]) Next() Option[T] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		if i.i%i.step == 0 {
			i.i++
			return n
		}
		i.i++
	}
	return MakeOptionNone[T]()
}

// AglIteratorStepBy creates an iterator starting at the same point, but stepping by the given amount at each iteration.
func AglIteratorStepBy[T any](it Iterator[T], n int) *StepBy[T] {
	return &StepBy[T]{iter: it, step: n}
}

type Map[T, R any] struct {
	iter Iterator[T]
	f    func(T) R
}

func (i *Map[T, R]) Next() Option[R] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		return MakeOptionSome(i.f(n.Unwrap()))
	}
	return MakeOptionNone[R]()
}

func AglIteratorMap[T, R any, I Iterator[T]](it I, f func(T) R) *Map[T, R] {
	return &Map[T, R]{iter: it, f: f}
}

type MapWhile[T, R any] struct {
	iter Iterator[T]
	f    func(T) Option[R]
}

func (i *MapWhile[T, R]) Next() Option[R] {
	for {
		n := i.iter.Next()
		if n.IsNone() {
			break
		}
		res := i.f(n.Unwrap())
		if res.IsNone() {
			break
		}
		return res
	}
	return MakeOptionNone[R]()
}

func AglIteratorMapWhile[T, R any](it Iterator[T], f func(T) Option[R]) *MapWhile[T, R] {
	return &MapWhile[T, R]{iter: it, f: f}
}

// AglIteratorCount consumes the iterator, counting the number of iterations and returning it.
func AglIteratorCount[T any](it Iterator[T]) int {
	count := 0
	for {
		n := it.Next()
		if n.IsNone() {
			break
		}
		count++
	}
	return count
}

type IterSet[T comparable] struct {
	s    AglSet[T]
	next func() (T, bool)
	stop func()
}

func (i *IterSet[T]) Next() Option[T] {
	n, ok := i.next()
	if !ok {
		return MakeOptionNone[T]()
	}
	return MakeOptionSome[T](n)
}

func (s AglSet[T]) Iter() Iterator[T] {
	seq := func(yield func(T) bool) {
		for k := range s {
			if !yield(k) {
				return
			}
		}
	}
	next, stop := iter.Pull(seq)
	return &IterSet[T]{s: s, next: next, stop: stop}
}

func AglSetIter[T comparable](s AglSet[T]) Iterator[T] {
	return s.Iter()
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

func AglIteratorMin[T cmp.Ordered](it Iterator[T]) Option[T] {
	v := it.Next()
	if v.IsNone() {
		return MakeOptionNone[T]()
	}
	out := v.Unwrap()
	for {
		v = it.Next()
		if v.IsNone() {
			break
		}
		out = min(out, v.Unwrap())
	}
	return MakeOptionSome(out)
}

func AglIteratorMax[T cmp.Ordered](it Iterator[T]) Option[T] {
	v := it.Next()
	if v.IsNone() {
		return MakeOptionNone[T]()
	}
	out := v.Unwrap()
	for {
		v = it.Next()
		if v.IsNone() {
			break
		}
		out = max(out, v.Unwrap())
	}
	return MakeOptionSome(out)
}

func AglSetMin[T cmp.Ordered](s AglSet[T]) Option[T] {
	return AglIteratorMin(s.Iter())
}

func AglSetMax[T cmp.Ordered](s AglSet[T]) Option[T] {
	return AglIteratorMax(s.Iter())
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

func AglSetContainsWhere[T comparable](s AglSet[T], p func(T) bool) bool {
	for k := range s {
		if p(k) {
			return true
		}
	}
	return false
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
	return AglBuildArray(AglIteratorMap(s.Iter(), f))
}

func AglSetForEach[T comparable](s AglSet[T], f func(T) AglVoid) {
	AglIteratorForEach(s.Iter(), f)
}

func AglSetFilter[T comparable](s AglSet[T], pred func(T) bool) AglSet[T] {
	return AglBuildSet(AglIteratorFilter(s.Iter(), pred))
}

// AglSetUnion returns a new set with the elements of both this set and the given sequence.
func AglSetUnion[T comparable](s AglSet[T], other Iterator[T]) AglSet[T] {
	newSet := make(AglSet[T])
	for k := range s {
		newSet[k] = struct{}{}
	}
	AglIteratorForEach(other, func(k T) AglVoid {
		newSet[k] = struct{}{}
		return AglVoid{}
	})
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
	AglIteratorForEach(other, func(k T) AglVoid {
		delete(newSet, k)
		return AglVoid{}
	})
	return newSet
}

func AglSetSubtract[T comparable](s AglSet[T], other Iterator[T]) {
	AglIteratorForEach(other, func(k T) AglVoid {
		delete(s, k)
		return AglVoid{}
	})
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
	AglIteratorForEach(other, func(k T) AglVoid {
		if _, ok := s[k]; !ok {
			s[k] = struct{}{}
		} else {
			delete(s, k)
		}
		return AglVoid{}
	})
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
	for {
		v := other.Next()
		if v.IsNone() {
			break
		}
		if _, ok := s[v.Unwrap()]; !ok {
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
	otherSet = make(AglSet[T])
	AglIteratorForEach(other, func(e T) AglVoid {
		otherSet[e] = struct{}{}
		return AglVoid{}
	})
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

type AglTupleStruct_int_int32 struct {
	Arg0 int
	Arg1 int32
}

func AglStringEnumerated(s string) []AglTupleStruct_int_int32 {
	out := make([]AglTupleStruct_int_int32, 0, len(s))
	for i, c := range s {
		out = append(out, AglTupleStruct_int_int32{Arg0: i, Arg1: c})
	}
	return out
}

func AglStringLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.Split(s, "\n")
}

func AglStringZFill(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
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

func AglStringIsEmpty(s string) bool {
	return s == ""
}

func AglStringTrimPrefix(s string, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

func AglStringTrimSuffix(s string, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

type AglTupleStruct_string_string struct {
	Arg0 string
	Arg1 string
}

func AglStringCut(s string, sep string) Option[AglTupleStruct_string_string] {
	before, after, found := strings.Cut(s, sep)
	if !found {
		return MakeOptionNone[AglTupleStruct_string_string]()
	}
	return MakeOptionSome(AglTupleStruct_string_string{Arg0: before, Arg1: after})
}

func AglStringCutPrefix(s string, prefix string) Option[string] {
	after, found := strings.CutPrefix(s, prefix)
	if !found {
		return MakeOptionNone[string]()
	}
	return MakeOptionSome(after)
}

func AglStringCutSuffix(s string, suffix string) Option[string] {
	before, found := strings.CutSuffix(s, suffix)
	if !found {
		return MakeOptionNone[string]()
	}
	return MakeOptionSome(before)
}

func AglStringHasPrefix(s string, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}

func AglStringContains(s string, substr string) bool {
	return strings.Contains(s, substr)
}

func AglStringContainsAny(s, chars string) bool {
	return strings.ContainsAny(s, chars)
}

func AglStringIndex(s, substr string) Option[int] {
	idx := strings.Index(s, substr)
	if idx == -1 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(idx)
}

func AglStringLastIndex(s, substr string) Option[int] {
	idx := strings.LastIndex(s, substr)
	if idx == -1 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(idx)
}

func AglStringCount(s, substr string) int {
	return strings.Count(s, substr)
}

func AglStringRepeat(s string, count int) string {
	return strings.Repeat(s, count)
}

func AglStringTrim(s, cutset string) string {
	return strings.Trim(s, cutset)
}

func AglStringTrimLeft(s, cutset string) string {
	return strings.TrimLeft(s, cutset)
}

func AglStringTrimRight(s, cutset string) string {
	return strings.TrimRight(s, cutset)
}

func AglStringHasSuffix(s string, suffix string) bool {
	return strings.HasSuffix(s, suffix)
}

func AglStringSplit(s string, sep string) []string {
	return strings.Split(s, sep)
}

func AglStringSplitAfter(s string, sep string) []string {
	return strings.SplitAfter(s, sep)
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

func AglPow[T, E Number](v T, e E) (out float64) {
	return math.Pow(float64(v), float64(e))
}

func AglVecWith[T any](a *[]T, i int, clb func(*T) AglVoid) {
	el := (*a)[i]
	clb(&el)
	(*a)[i] = el
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

func AglMapAllSatisfy[K comparable, V any](m map[K]V, pred func(DictEntry[K, V]) bool) bool {
	for k, v := range m {
		if !pred(DictEntry[K, V]{Key: k, Value: v}) {
			return false
		}
	}
	return true
}

func AglMapKeys[K comparable, V any](m map[K]V) Iterator[K] {
	return Sequence[K](maps.Keys(m)).Iter()
}

func AglMapValues[K comparable, V any](m map[K]V) Iterator[V] {
	return Sequence[V](maps.Values(m)).Iter()
}

func AglMapRemove[K comparable, V any](m map[K]V, k K) Option[V] {
	if v, ok := m[k]; ok {
		delete(m, k)
		return MakeOptionSome(v)
	}
	return MakeOptionNone[V]()
}

func AglMapInsert[K comparable, V any](m map[K]V, k K, v V) Option[V] {
	prev, ok := m[k]
	m[k] = v
	if ok {
		return MakeOptionSome(prev)
	}
	return MakeOptionNone[V]()
}

func AglMapDrain[K comparable, V any](m map[K]V) *IterMapDrain[K, V] {
	seq := func(yield func(DictEntry[K, V]) bool) {
		for k := range m {
			entry := DictEntry[K, V]{Key: k, Value: m[k]}
			delete(m, k)
			if !yield(entry) {
				return
			}
		}
	}
	next, stop := iter.Pull(seq)
	return &IterMapDrain[K, V]{next: next, stop: stop}
}

func AglMapIsEmpty[K comparable, V any](m map[K]V) bool {
	return AglMapLen(m) == 0
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

func AglIntSqrt(v int) int         { return int(math.Sqrt(float64(v))) }
func AglI8Sqrt(v int8) int8        { return int8(math.Sqrt(float64(v))) }
func AglI16Sqrt(v int16) int16     { return int16(math.Sqrt(float64(v))) }
func AglI32Sqrt(v int32) int32     { return int32(math.Sqrt(float64(v))) }
func AglI64Sqrt(v int64) int64     { return int64(math.Sqrt(float64(v))) }
func AglUintSqrt(v uint) uint      { return uint(math.Sqrt(float64(v))) }
func AglU8Sqrt(v uint8) uint8      { return uint8(math.Sqrt(float64(v))) }
func AglU16Sqrt(v uint16) uint16   { return uint16(math.Sqrt(float64(v))) }
func AglU32Sqrt(v uint32) uint32   { return uint32(math.Sqrt(float64(v))) }
func AglU64Sqrt(v uint64) uint64   { return uint64(math.Sqrt(float64(v))) }
func AglF32Sqrt(v float32) float32 { return float32(math.Sqrt(float64(v))) }
func AglF64Sqrt(v float64) float64 { return float64(math.Sqrt(float64(v))) }

func AglIntAbs(v int) int         { return int(math.Abs(float64(v))) }
func AglI8Abs(v int8) int8        { return int8(math.Abs(float64(v))) }
func AglI16Abs(v int16) int16     { return int16(math.Abs(float64(v))) }
func AglI32Abs(v int32) int32     { return int32(math.Abs(float64(v))) }
func AglI64Abs(v int64) int64     { return int64(math.Abs(float64(v))) }
func AglF32Abs(v float32) float32 { return float32(math.Abs(float64(v))) }
func AglF64Abs(v float64) float64 { return math.Abs(v) }

func AglIntString(v int) string    { return strconv.FormatInt(int64(v), 10) }
func AglI8String(v int8) string    { return strconv.FormatInt(int64(v), 10) }
func AglI16String(v int16) string  { return strconv.FormatInt(int64(v), 10) }
func AglI32String(v int32) string  { return strconv.FormatInt(int64(v), 10) }
func AglI64String(v int64) string  { return strconv.FormatInt(int64(v), 10) }
func AglUintString(v uint) string  { return strconv.FormatUint(uint64(v), 10) }
func AglU8String(v uint8) string   { return strconv.FormatUint(uint64(v), 10) }
func AglU16String(v uint16) string { return strconv.FormatUint(uint64(v), 10) }
func AglU32String(v uint32) string { return strconv.FormatUint(uint64(v), 10) }
func AglU64String(v uint64) string { return strconv.FormatUint(uint64(v), 10) }

func AglIntCheckedDiv(v, d int) Option[int] {
	if d == 0 {
		return MakeOptionNone[int]()
	}
	return MakeOptionSome(v / d)
}

func AglIn[T comparable](e T, it IntoIterator[T]) bool {
	return AglIteratorContains(it.Iter(), e)
}

func AglBuildSet[T comparable](it Iterator[T]) (out AglSet[T]) {
	out = make(AglSet[T])
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		AglSetInsert(out, v.Unwrap())
	}
	return
}

func AglBuildArray[T any](it Iterator[T]) (out []T) {
	for {
		v := it.Next()
		if v.IsNone() {
			break
		}
		out = append(out, v.Unwrap())
	}
	return
}
