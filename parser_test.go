package main

import (
	"testing"

	tassert "github.com/stretchr/testify/assert"
)

func TestParser(t *testing.T) {
	src := `
package main

type test1 enum {
	A
	B
	C
	E(a,b,c u8, s u8)
}

type test struct {
}

func main() {
    test({ $0 + 1 })
}
`
	parser2(src)
}

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

func TestParseChanSendRecv(t *testing.T) {
	// Test send operation
	sendExpr := parseExpr(NewTokenStream("ch <- 42"), 0)
	tassert.IsType(t, &SendExpr{}, sendExpr)
	send := sendExpr.(*SendExpr)
	tassert.Equal(t, "ch", send.Chan.(*IdentExpr).lit)
	tassert.Equal(t, "42", send.Value.(*NumberExpr).lit)

	// Test receive operation
	recvExpr := parseExpr(NewTokenStream("<-ch"), 0)
	tassert.IsType(t, &RecvExpr{}, recvExpr)
	recv := recvExpr.(*RecvExpr)
	tassert.Equal(t, "ch", recv.Chan.(*IdentExpr).lit)
}

func TestParser1(t *testing.T) {
	x := parseStmt(NewTokenStream("a, b := 0, 1"))
	tassert.Equal(t, 2, len(x.(*AssignStmt).rhs))
}
