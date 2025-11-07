package main

import (
	"encoding/binary"
	"hash/fnv"
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

//func TestAglIterMin(t *testing.T) {
//	tassert.True(t, AglIterMin(AglVec[int]([]int{}).Iter()).IsNone())
//	tassert.True(t, AglIterMin(AglSet[int]{}.Iter()).IsNone())
//	tassert.Equal(t, 1, AglIterMin(AglSet[int]{4: {}, 1: {}, 2: {}}.Iter()).Unwrap())
//	tassert.Equal(t, 1, AglIterMin(AglVec[int]([]int{2, 3, 1, 4, 5}).Iter()).Unwrap())
//	tassert.Equal(t, 2, AglIterMin(AglVec[int]([]int{2}).Iter()).Unwrap())
//}
//
//func TestAglIterMax(t *testing.T) {
//	tassert.True(t, AglIterMax(AglVec[int]([]int{}).Iter()).IsNone())
//	tassert.True(t, AglIterMax(AglSet[int]{}.Iter()).IsNone())
//	tassert.Equal(t, 4, AglIterMax(AglSet[int]{4: {}, 1: {}, 2: {}}.Iter()).Unwrap())
//	tassert.Equal(t, 5, AglIterMax(AglVec[int]([]int{2, 3, 1, 4, 5}).Iter()).Unwrap())
//	tassert.Equal(t, 2, AglIterMax(AglVec[int]([]int{2}).Iter()).Unwrap())
//}

type CustomStruct struct {
	A int
	B string
}

func (c CustomStruct) Hash() uint64 {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(c.A))
	h := fnv.New64a()
	_, _ = h.Write(buf)
	_, _ = h.Write([]byte(c.B))
	return h.Sum64()
}

func (c CustomStruct) __EQ(rhs CustomStruct) bool {
	return true
}

func TestAglVecDropFirst(t *testing.T) {
	// Test normal case
	arr := []int{1, 2, 3, 4, 5}
	result := AglVecDropFirst(arr, 2)
	tassert.Equal(t, []int{3, 4, 5}, result)

	// Test drop 0
	result = AglVecDropFirst(arr, 0)
	tassert.Equal(t, arr, result)

	// Test drop more than length
	result = AglVecDropFirst(arr, 10)
	tassert.Equal(t, []int{}, result)

	// Test negative value
	result = AglVecDropFirst(arr, -1)
	tassert.Equal(t, arr, result)

	// Test empty array
	empty := []int{}
	result = AglVecDropFirst(empty, 5)
	tassert.Equal(t, []int{}, result)
}

func TestAglVecDropLast(t *testing.T) {
	// Test normal case
	arr := []int{1, 2, 3, 4, 5}
	result := AglVecDropLast(arr, 2)
	tassert.Equal(t, []int{1, 2, 3}, result)

	// Test drop 0
	result = AglVecDropLast(arr, 0)
	tassert.Equal(t, arr, result)

	// Test drop more than length
	result = AglVecDropLast(arr, 10)
	tassert.Equal(t, []int{}, result)

	// Test negative value
	result = AglVecDropLast(arr, -1)
	tassert.Equal(t, arr, result)

	// Test empty array
	empty := []int{}
	result = AglVecDropLast(empty, 5)
	tassert.Equal(t, []int{}, result)
}

func TestAglSet1(t *testing.T) {
	s1 := AglSet1[AglInt]{}
	s1.Insert(AglInt{1})
	tassert.True(t, s1.Contains(AglInt{1}))
	tassert.False(t, s1.Contains(AglInt{2}))

	s2 := AglSet1[AglString]{}
	s2.Insert(AglString{"a"})
	tassert.True(t, s2.Contains(AglString{"a"}))
	tassert.False(t, s2.Contains(AglString{"b"}))

	s3 := AglSet1[CustomStruct]{}
	s3.Insert(CustomStruct{A: 1, B: "a"})
	tassert.True(t, s3.Contains(CustomStruct{A: 1, B: "a"}))
	tassert.False(t, s3.Contains(CustomStruct{A: 2, B: "a"}))
	tassert.False(t, s3.Contains(CustomStruct{A: 1, B: "b"}))

	s4 := AglSet1[AglSet1[AglInt]]{}
	innerSet := AglSet1[AglInt]{}
	innerSet.Insert(AglInt{1})
	innerSet.Insert(AglInt{2})
	tassert.True(t, s4.Insert(innerSet))
	tassert.False(t, s4.Insert(innerSet))
	tassert.True(t, s4.Contains(innerSet))
	tassert.False(t, s4.Contains(s1))
}

//func TestAglSequenceStepBy(t *testing.T) {
//	arr := []int{0, 1, 2, 3, 4, 5}
//	it := AglVec[int](arr).Iter()
//	it = AglSequenceStepBy(it, 2)
//	next, _ := iter.Pull(iter.Seq[int](it))
//	tassert.Equal(t, agl.First(next()), 0)
//	tassert.Equal(t, agl.First(next()), 2)
//	tassert.Equal(t, agl.First(next()), 4)
//	tassert.False(t, agl.Second(next()))
//}
//
//func TestAglSequenceIntersperse(t *testing.T) {
//	arr := []int{0, 1, 2}
//	it := AglVec[int](arr).Iter()
//	it = AglSequenceIntersperse(it, 100)
//	next, _ := iter.Pull(iter.Seq[int](it))
//	tassert.Equal(t, agl.First(next()), 0)
//	tassert.Equal(t, agl.First(next()), 100)
//	tassert.Equal(t, agl.First(next()), 1)
//	tassert.Equal(t, agl.First(next()), 100)
//	tassert.Equal(t, agl.First(next()), 2)
//	tassert.False(t, agl.Second(next()))
//}

func TestAglIterator(t *testing.T) {
	arr := []int{0, 1, 2}
	it := AglVec[int](arr).Iter()
	tassert.Equal(t, 0, it.Next().Unwrap())
	tassert.Equal(t, 1, it.Next().Unwrap())
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorFilter(t *testing.T) {
	arr := []int{0, 1, 2}
	it := AglIteratorFilter(AglVec[int](arr).Iter(), func(i int) bool { return i%2 == 0 })
	tassert.Equal(t, 0, it.Next().Unwrap())
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorTakeWhile(t *testing.T) {
	arr := []int{1, 2, 3, 4}
	it := AglIteratorTakeWhile(AglVec[int](arr).Iter(), func(i int) bool { return i != 3 })
	tassert.Equal(t, 1, it.Next().Unwrap())
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
	tassert.Equal(t, 4, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorMap(t *testing.T) {
	arr := []int{1, 2, 3}
	it := AglIteratorMap(AglVec[int](arr).Iter(), func(e int) int { return e + 1 })
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.Equal(t, 3, it.Next().Unwrap())
	tassert.Equal(t, 4, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())

	s := AglSet[int]{1: struct{}{}}
	it = AglIteratorMap(s.Iter(), func(e int) int { return e + 1 })
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorMin(t *testing.T) {
	arr := []int{1, 2, 3, 4}
	tassert.Equal(t, 1, AglIteratorMin[int](AglVec[int](arr).Iter()).Unwrap())
	s := AglSet[int]{4: struct{}{}, 2: struct{}{}, 3: struct{}{}}
	tassert.Equal(t, 2, AglIteratorMin(s.Iter()).Unwrap())
}

func TestAglIteratorAllSatisfy(t *testing.T) {
	arr := []int{1, 2, 3}
	tassert.True(t, AglIteratorAllSatisfy(AglVec[int](arr).Iter(), func(e int) bool { return e > 0 }))
	tassert.False(t, AglIteratorAllSatisfy(AglVec[int](arr).Iter(), func(e int) bool { return e > 2 }))
}

func TestAglIteratorTake(t *testing.T) {
	arr := []int{1, 2, 3}
	it := AglIteratorTake[int](AglVec[int](arr).Iter(), 2)
	tassert.Equal(t, 1, it.Next().Unwrap())
	tassert.Equal(t, 2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorSkip(t *testing.T) {
	arr := []int{1, 2, 3}
	it := AglIteratorSkip[int](AglVec[int](arr).Iter(), 2)
	tassert.Equal(t, 3, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorSkipWhile(t *testing.T) {
	arr := []int{-1, 0, 1, -2}
	it := AglIteratorSkipWhile(AglVec[int](arr).Iter(), func(e int) bool { return e < 0 })
	tassert.Equal(t, 0, it.Next().Unwrap())
	tassert.Equal(t, 1, it.Next().Unwrap())
	tassert.Equal(t, -2, it.Next().Unwrap())
	tassert.True(t, it.Next().IsNone())
}

func TestAglIteratorRPosition(t *testing.T) {
	arr := []int{1, 2, 3}
	it := AglVec[int](arr).Iter()
	tassert.Equal(t, 2, AglIteratorRPosition(it, func(e int) bool { return e == 3 }).Unwrap())
	tassert.True(t, AglIteratorRPosition(it, func(e int) bool { return e == 5 }).IsNone())
}
