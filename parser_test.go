package main

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestParseFnSignature1(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn add(a, b i64) i64"))
	tassert.Equal(t, 1, len(stmt.args.list))
	tassert.Equal(t, 2, len(stmt.args.list[0].names))
	tassert.Equal(t, "add", stmt.name)
	tassert.Equal(t, "a", stmt.args.list[0].names[0].lit)
	tassert.Equal(t, "b", stmt.args.list[0].names[1].lit)
	tassert.Equal(t, "i64", stmt.out.expr.(*IdentExpr).lit)
}

func TestParseFnSignature2(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn(a, b fn(i8) u8) i64"))
	tassert.Equal(t, 1, len(stmt.args.list))
	tassert.Equal(t, 2, len(stmt.args.list[0].names))
	tassert.Equal(t, "", stmt.name)
	tassert.Equal(t, "a", stmt.args.list[0].names[0].lit)
	tassert.Equal(t, "b", stmt.args.list[0].names[1].lit)
	tassert.Equal(t, 1, len(stmt.args.list[0].typeExpr.(*FuncExpr).args.list))
	tassert.Equal(t, 0, len(stmt.args.list[0].typeExpr.(*FuncExpr).args.list[0].names))
	tassert.Equal(t, "i8", stmt.args.list[0].typeExpr.(*FuncExpr).args.list[0].typeExpr.(*IdentExpr).lit)
	tassert.Equal(t, "u8", stmt.args.list[0].typeExpr.(*FuncExpr).out.expr.(*IdentExpr).lit)
	tassert.Equal(t, "i64", stmt.out.expr.(*IdentExpr).lit)
}

func TestParseFnSignature3(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn find[T any](a []T, e T) T?"))
	tassert.Equal(t, "find", stmt.name)
	tassert.Equal(t, 1, len(stmt.typeParams.list))
	tassert.Equal(t, 1, len(stmt.typeParams.list[0].names))
	tassert.Equal(t, "T", stmt.typeParams.list[0].names[0].lit)
	tassert.Equal(t, "any", stmt.typeParams.list[0].typeExpr.(*IdentExpr).lit)
	tassert.Equal(t, 2, len(stmt.args.list))
	tassert.Equal(t, 1, len(stmt.args.list[0].names))
	tassert.Equal(t, 1, len(stmt.args.list[1].names))
	tassert.Equal(t, "a", stmt.args.list[0].names[0].lit)
	tassert.Equal(t, "e", stmt.args.list[1].names[0].lit)
	tassert.Equal(t, "T", stmt.args.list[0].typeExpr.(*ArrayTypeExpr).elt.(*IdentExpr).lit)
	tassert.Equal(t, "T", stmt.args.list[1].typeExpr.(*IdentExpr).lit)
	tassert.Equal(t, "T", stmt.out.expr.(*OptionExpr).x.(*IdentExpr).lit)
}

func TestParseFnSignature4(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn parseInt(s string) int!"))
	tassert.Equal(t, "parseInt", stmt.name)
	tassert.Nil(t, stmt.typeParams)
	tassert.Equal(t, 1, len(stmt.args.list))
	tassert.Equal(t, 1, len(stmt.args.list[0].names))
	tassert.Equal(t, "s", stmt.args.list[0].names[0].lit)
	tassert.Equal(t, "string", stmt.args.list[0].typeExpr.(*IdentExpr).lit)
	tassert.Equal(t, "int", stmt.out.expr.(*ResultExpr).x.(*IdentExpr).lit)
}

func TestParseFnSignature5(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn main() {}"))
	tassert.Equal(t, "main", stmt.name)
	tassert.Nil(t, stmt.typeParams)
	tassert.Equal(t, 0, len(stmt.args.list))
	tassert.Nil(t, stmt.out.expr)
}

func TestParseFnSignature6(t *testing.T) {
	stmt := parseFnSignature(NewTokenStream("fn parseInt(string)"))
	tassert.Equal(t, "parseInt", stmt.name)
	tassert.Nil(t, stmt.typeParams)
	tassert.Equal(t, 1, len(stmt.args.list))
	tassert.Equal(t, 0, len(stmt.args.list[0].names))
	tassert.Equal(t, "string", stmt.args.list[0].typeExpr.(*IdentExpr).lit)
}
