package main

import "fmt"

type Tmp struct {
}

func main() {
	a := AglVecEq[AglInt]{AglInt(1), AglInt(2), AglInt(3)}
	b := AglVecEq[AglInt]{AglInt(1), AglInt(2), AglInt(3)}
	AglPrint(AglVecEq__Eq(a, b))
	//aa := []int{1, 2, 3}
	//bb := []int{1, 2, 3}
	//AglPrint(AglVecEq(aa, bb))
	aa := AglVecEq[AglVecEq[AglInt]]{{AglInt(1)}}
	bb := AglVecEq[AglVecEq[AglInt]]{{AglInt(1)}}
	AglPrint(AglVecEq__Eq(aa, bb))
	cc := AglVec[Tmp]{Tmp{}}
	d := AglVecEq[AglInt](AglVecFilter(AglVec[AglInt](a)))
	e := AglVec[Tmp](AglVecFilter(AglVec[Tmp](cc)))
	AglPrint(d, e)
	AglPrint(cc)
	AglPrint(AglInt(1).__EQ(AglInt(1)))
}

func AglPrint(a ...any) {
	fmt.Println(a...)
}

type Equatable[T any] interface {
	__EQ(rhs T) bool
}

type AglInt int

func (i AglInt) __EQ(rhs AglInt) bool { return i == rhs }

type AglVec[T any] []T

func (v AglVec[T]) Filter() {
}

func AglVecFilter[T any](v AglVec[T]) AglVec[T] {
	return v
}

func AglVecEqFilter[T Equatable[T]](v AglVecEq[T]) AglVecEq[T] {
	return AglVecEq[T](AglVecFilter(AglVec[T](v)))
}

type AglVecEq[T Equatable[T]] AglVec[T]

func (v AglVec[T]) Len() int { return len(v) }

func (v AglVecEq[T]) __EQ(rhs AglVecEq[T]) bool {
	return AglVecEq__Eq(v, rhs)
}

func AglVecEq__Eq[T Equatable[T]](lhs, rhs AglVecEq[T]) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for i := range lhs {
		if !lhs[i].__EQ(rhs[i]) {
			return false
		}
	}
	return true
}
