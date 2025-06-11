package main

import (
	"fmt"
	"unicode"
	"unicode/utf8"
)

const (
	AND = iota + 1
	ASSERT
	ASSIGN // =
	BANG
	BREAK
	CASE
	CHAN
	COLON
	COMMA
	CONTINUE
	DEC
	DEFAULT
	DEFER
	DOT
	ELSE
	ENUM
	EQL // ==
	ERR
	FATARROWRIGHT
	FN
	FOR
	GO
	GT  // >
	GTE // >=
	IDENT
	IF
	IMPORT
	IN
	INC
	INLINECOMMENT
	LAND     // &&
	LBRACE   // {
	LBRACKET // [
	LEFTARROW
	LOR // ||
	LPAREN
	LT  // <
	LTE // <=
	MAKE
	MAP
	MATCH
	MINUS // -
	REM   // %
	MUL   // *
	MUT
	NEQ // !=
	NEW
	NONE
	NUMBER
	OK
	OPTION
	PACKAGE
	PANIC
	PIPE
	ADD // +
	PUB
	QUESTION
	QUO // /
	RANGE
	RBRACE   // }
	RBRACKET // ]
	RESULT
	RETURN
	RPAREN
	SELECT
	SHIFTLEFT
	SHIFTRIGHT
	SOME
	STRING
	STRUCT
	TYPE
	TRUE
	FALSE
	WALRUS // :=
	XOR
	ELLIPSIS // ...
	EOF
	INTERFACE
	VAR
	SEMICOLON

	ADD_ASSIGN // +=
	SUB_ASSIGN // -=
	MUL_ASSIGN // *=
	QUO_ASSIGN // /=
	REM_ASSIGN // %=

	SHL_ASSIGN     // <<=
	SHR_ASSIGN     // >>=
	AND_ASSIGN     // &=
	XOR_ASSIGN     // ^=
	OR_ASSIGN      // |=
	AND_NOT_ASSIGN // &^=
)

type Pos struct {
	Row, Col int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Row, p.Col) }

type Tok struct {
	Pos
	typ int
	lit string
}

func (t Tok) String() string {
	return fmt.Sprintf("Tok('%s' %d:%d)", t.lit, t.Row, t.Col)
}

func lower(ch rune) rune { return ('a' - 'A') | ch } // returns lower-case ch iff ch is ASCII letter

func isLetter(ch rune) bool {
	return 'a' <= lower(ch) && lower(ch) <= 'z' || ch == '_' || ch >= utf8.RuneSelf && unicode.IsLetter(ch)
}

func isDecimal(ch rune) bool { return '0' <= ch && ch <= '9' }

func isDigit(ch rune) bool {
	return isDecimal(ch) || ch >= utf8.RuneSelf && unicode.IsDigit(ch)
}

type TokenStream struct {
	pos    int
	tokens []Tok
}

func NewTokenStream(src string) *TokenStream {
	return &TokenStream{tokens: lexer(src)}
}

func (s *TokenStream) Back() {
	s.pos--
}

func (s *TokenStream) Next() Tok {
	if s.pos >= len(s.tokens) {
		return Tok{typ: EOF}
	}
	tok := s.tokens[s.pos]
	s.pos++
	return tok
}

func (s *TokenStream) Peek() Tok {
	if s.pos >= len(s.tokens) {
		return Tok{typ: EOF}
	}
	return s.tokens[s.pos]
}

func lexer(src string) (out []Tok) {
	row, col := 1, 0
	for i := 0; i < len(src); i++ {
		pos := Pos{Row: row, Col: col}
		col++
		c := src[i]
		switch c {
		case '\n':
			row++
			col = 1
		case ' ', '	':
		case '|':
			if src[i+1] == '|' {
				i++
				col++
				out = append(out, Tok{pos, LOR, "||"})
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, OR_ASSIGN, "|="})
			} else {
				out = append(out, Tok{pos, PIPE, "|"})
			}
		case '*':
			if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, MUL_ASSIGN, "*="})
			} else {
				out = append(out, Tok{pos, MUL, "*"})
			}
		case '^':
			if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, XOR_ASSIGN, "^="})
			} else {
				out = append(out, Tok{pos, XOR, "^"})
			}
		case '%':
			if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, REM_ASSIGN, "%="})
			} else {
				out = append(out, Tok{pos, REM, "%"})
			}
		case '.':
			if src[i+1] == '.' && src[i+2] == '.' {
				i += 2
				col += 2
				out = append(out, Tok{pos, ELLIPSIS, "..."})
			} else {
				out = append(out, Tok{pos, DOT, "."})
			}
		case '(':
			out = append(out, Tok{pos, LPAREN, "("})
		case ')':
			out = append(out, Tok{pos, RPAREN, ")"})
		case '[':
			out = append(out, Tok{pos, LBRACKET, "["})
		case ']':
			out = append(out, Tok{pos, RBRACKET, "]"})
		case '{':
			out = append(out, Tok{pos, LBRACE, "{"})
		case '}':
			out = append(out, Tok{pos, RBRACE, "}"})
		case '?':
			out = append(out, Tok{pos, QUESTION, "?"})
		case ';':
			out = append(out, Tok{pos, SEMICOLON, ";"})
		case ',':
			out = append(out, Tok{pos, COMMA, ","})
		case '/':
			if src[i+1] == '/' {
				tok := scanInlineComment(pos, src, i)
				i += len(tok.lit) - 1
				row++
				col = 0
				out = append(out, tok)
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, QUO_ASSIGN, "/="})
			} else {
				out = append(out, Tok{pos, QUO, "/"})
			}
		case '!':
			if i+1 < len(src) && src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, NEQ, "!="})
			} else {
				out = append(out, Tok{pos, BANG, "!"})
			}
		case '<':
			if src[i+1] == '-' {
				i++
				col++
				out = append(out, Tok{pos, LEFTARROW, "<-"})
			} else if src[i+1] == '<' {
				if src[i+2] == '=' {
					i += 2
					col += 2
					out = append(out, Tok{pos, SHL_ASSIGN, "<<="})
				} else {
					i++
					col++
					out = append(out, Tok{pos, SHIFTLEFT, "<<"})
				}
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, LTE, "<="})
			} else {
				out = append(out, Tok{pos, LT, "<"})
			}
		case '>':
			if src[i+1] == '>' {
				if src[i+2] == '=' {
					i += 2
					col += 2
					out = append(out, Tok{pos, SHR_ASSIGN, ">>="})
				} else {
					i++
					col++
					out = append(out, Tok{pos, SHIFTRIGHT, ">>"})
				}
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, GTE, ">="})
			} else {
				out = append(out, Tok{pos, GT, ">"})
			}
		case ':':
			if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, WALRUS, ":="})
			} else {
				out = append(out, Tok{pos, COLON, ":"})
			}
		case '+':
			if src[i+1] == '+' {
				i++
				col++
				out = append(out, Tok{pos, INC, "++"})
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, ADD_ASSIGN, "+="})
			} else {
				out = append(out, Tok{pos, ADD, "+"})
			}
		case '-':
			if src[i+1] == '-' {
				i++
				col++
				out = append(out, Tok{pos, DEC, "--"})
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, SUB_ASSIGN, "-="})
			} else {
				out = append(out, Tok{pos, MINUS, "-"})
			}
		case '&':
			if src[i+1] == '&' {
				i++
				col++
				out = append(out, Tok{pos, LAND, "&&"})
			} else if src[i+1] == '^' && src[i+2] == '=' {
				i += 2
				col += 2
				out = append(out, Tok{pos, AND_NOT_ASSIGN, "&^="})
			} else if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, AND_ASSIGN, "&="})
			} else {
				out = append(out, Tok{pos, AND, "&"})
			}
		case '=':
			if src[i+1] == '=' {
				i++
				col++
				out = append(out, Tok{pos, EQL, "=="})
			} else if src[i+1] == '>' {
				i++
				col++
				out = append(out, Tok{pos, FATARROWRIGHT, "=>"})
			} else {
				out = append(out, Tok{pos, ASSIGN, "="})
			}
		case '"':
			tok := scanString(pos, src, i)
			i += len(tok.lit) - 1
			col += len(tok.lit) - 1
			out = append(out, tok)
		default:
			if isLetter(rune(c)) {
				tok := scanIdentifier(pos, src, i)
				i += len(tok.lit) - 1
				col += len(tok.lit) - 1
				out = append(out, tok)
			} else if isDecimal(rune(c)) {
				tok := scanNumber(pos, src, i)
				i += len(tok.lit) - 1
				col += len(tok.lit) - 1
				out = append(out, tok)
			} else if c == '$' && isDecimal(rune(src[i+1])) {
				p := i
				i++
				for {
					c := src[i]
					if isDecimal(rune(c)) {
						i++
						continue
					}
					break
				}
				lit := src[p:i]
				tok := Tok{pos, IDENT, lit}
				col += len(tok.lit) - 1
				out = append(out, tok)
			} else {
				panic(fmt.Sprintf("invalid character '%c' at %d:%d", c, row, col))
			}
		}
	}
	return
}

func scanString(p Pos, src string, pos int) Tok {
	i := pos + 1
	for {
		ch := src[i]
		if ch == '\n' || ch < 0 {
			panic("string literal not terminated")
		}
		i++
		if ch == '"' {
			break
		}
	}
	lit := src[pos:i]
	return Tok{p, STRING, lit}
}

func scanInlineComment(p Pos, src string, pos int) Tok {
	i := pos + 1
	for {
		ch := src[i]
		if ch == '\n' {
			break
		}
		i++
	}
	lit := src[pos:i]
	return Tok{p, INLINECOMMENT, lit}
}

func scanIdentifier(p Pos, src string, pos int) Tok {
	i := pos
	for {
		if i > len(src)-1 {
			break
		}
		c := src[i]
		if isLetter(rune(c)) || isDecimal(rune(c)) {
			i++
			continue
		}
		break
	}
	lit := src[pos:i]
	m := map[string]int{
		"Err":       ERR,
		"None":      NONE,
		"Ok":        OK,
		"Option":    OPTION,
		"Result":    RESULT,
		"Some":      SOME,
		"assert":    ASSERT,
		"break":     BREAK,
		"case":      CASE,
		"default":   DEFAULT,
		"chan":      CHAN,
		"continue":  CONTINUE,
		"defer":     DEFER,
		"else":      ELSE,
		"enum":      ENUM,
		"fn":        FN,
		"for":       FOR,
		"go":        GO,
		"if":        IF,
		"import":    IMPORT,
		"in":        IN,
		"make":      MAKE,
		"map":       MAP,
		"match":     MATCH,
		"mut":       MUT,
		"new":       NEW,
		"package":   PACKAGE,
		"panic":     PANIC,
		"pub":       PUB,
		"range":     RANGE,
		"return":    RETURN,
		"select":    SELECT,
		"struct":    STRUCT,
		"type":      TYPE,
		"true":      TRUE,
		"false":     FALSE,
		"interface": INTERFACE,
		"var":       VAR,
	}

	v := IDENT
	if val, ok := m[lit]; ok {
		v = val
	}
	return Tok{p, v, lit}
}

func scanNumber(p Pos, src string, pos int) Tok {
	i := pos
	for {
		if i > len(src)-1 {
			break
		}
		c := src[i]
		if isDecimal(rune(c)) {
			i++
			continue
		}
		break
	}
	lit := src[pos:i]
	return Tok{p, NUMBER, lit}
}
