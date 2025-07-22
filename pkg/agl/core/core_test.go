package main

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestAglIterMin(t *testing.T) {
	tassert.True(t, AglIterMin(AglVec[int]([]int{}).Iter()).IsNone())
	tassert.True(t, AglIterMin(AglSet[int]{}.Iter()).IsNone())
	tassert.Equal(t, 1, AglIterMin(AglSet[int]{4: {}, 1: {}, 2: {}}.Iter()).Unwrap())
	tassert.Equal(t, 1, AglIterMin(AglVec[int]([]int{2, 3, 1, 4, 5}).Iter()).Unwrap())
	tassert.Equal(t, 2, AglIterMin(AglVec[int]([]int{2}).Iter()).Unwrap())
}

func TestAglIterMax(t *testing.T) {
	tassert.True(t, AglIterMax(AglVec[int]([]int{}).Iter()).IsNone())
	tassert.True(t, AglIterMax(AglSet[int]{}.Iter()).IsNone())
	tassert.Equal(t, 4, AglIterMax(AglSet[int]{4: {}, 1: {}, 2: {}}.Iter()).Unwrap())
	tassert.Equal(t, 5, AglIterMax(AglVec[int]([]int{2, 3, 1, 4, 5}).Iter()).Unwrap())
	tassert.Equal(t, 2, AglIterMax(AglVec[int]([]int{2}).Iter()).Unwrap())
}
