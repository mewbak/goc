// Copyright 2018 Hajime Hoshi
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tokenize

import (
	"bufio"
	"fmt"
	"io"

	"github.com/hajimehoshi/goc/internal/ctype"
	"github.com/hajimehoshi/goc/internal/ioutil"
	"github.com/hajimehoshi/goc/internal/lex"
	"github.com/hajimehoshi/goc/internal/token"
)

type tokenizer struct {
	src *bufio.Reader

	// ppstate represents the current context is in the preprocessor or not.
	// -1 means header-name is no longer expected in the current line.
	// 0 means the start of the new line (just after '\n' or the initial state).
	// 1 means the start of the line of preprocessing (just after '#').
	// 2 means header-name is expected (just after '#include').
	ppstate int

	isSpace  bool
	wasSpace bool

	// TODO: Consider #error directive
}

func (t *tokenizer) headerNameExpected() bool {
	return t.ppstate == 2
}

func (t *tokenizer) next(src *bufio.Reader) (*token.Token, error) {
	var tk *token.Token
	for {
		var err error
		tk, err = t.nextImpl(src)
		if tk == nil && err == nil {
			continue
		}
		if err != nil {
			if err == io.EOF && tk != nil {
				panic("not reached")
			}
			return nil, err
		}
		break
	}

	tk.Adjacent = !t.wasSpace

	switch tk.Type {
	case '\n':
		t.ppstate = 0
	case '#':
		if t.ppstate == 0 {
			t.ppstate = 1
		} else {
			t.ppstate = -1
		}
	case token.Ident:
		if t.ppstate == 1 && tk.Name == "include" {
			t.ppstate = 2
		} else {
			t.ppstate = -1
		}
	default:
		t.ppstate = -1
	}

	return tk, nil
}

func (t *tokenizer) nextImpl(src *bufio.Reader) (*token.Token, error) {
	// TODO: Can this read runes intead of bytes?
	bs, err := src.Peek(3)
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(bs) == 0 {
		if err != io.EOF {
			panic("not reached")
		}
		return nil, err
	}

	t.wasSpace = t.isSpace
	switch b := bs[0]; b {
	case ' ', '\t', '\v', '\f', '\r', '\n':
		t.isSpace = true
	default:
		t.isSpace = false
	}

	switch b := bs[0]; b {
	case '\n':
		// New line; preprocessor uses this.
		src.Discard(1)
		return &token.Token{
			Type: token.Type(b),
		}, nil
	case ' ', '\t', '\v', '\f', '\r':
		// Space
		src.Discard(1)
		return nil, nil
	case '+':
		if len(bs) >= 2 {
			switch bs[1] {
			case '+':
				src.Discard(2)
				return &token.Token{
					Type: token.Inc,
				}, nil
			case '=':
				src.Discard(2)
				return &token.Token{
					Type: token.AddEq,
				}, nil
			}
		}
	case '-':
		if len(bs) >= 2 {
			switch bs[1] {
			case '-':
				src.Discard(2)
				return &token.Token{
					Type: token.Dec,
				}, nil
			case '=':
				src.Discard(2)
				return &token.Token{
					Type: token.SubEq,
				}, nil
			case '>':
				src.Discard(2)
				return &token.Token{
					Type: token.Arrow,
				}, nil
			}
		}
	case '*':
		if len(bs) >= 2 && bs[1] == '=' {
			src.Discard(2)
			return &token.Token{
				Type: token.MulEq,
			}, nil
		}
	case '/':
		if len(bs) >= 2 {
			switch bs[1] {
			case '/':
				// Line comment
				src.Discard(2)
				for {
					b, err := src.ReadByte()
					if err != nil && err != io.EOF {
						return nil, err
					}
					if b == '\n' {
						break
					}
					if err == io.EOF {
						break
					}
				}
				return nil, nil
			case '*':
				// Block comment
				src.Discard(2)
				for {
					bs, err := src.Peek(2)
					if err != nil && err != io.EOF {
						return nil, err
					}
					if len(bs) <= 1 {
						return nil, fmt.Errorf("tokenizer: unclosed block comment")
					}
					if bs[0] == '*' && bs[1] == '/' {
						src.Discard(2)
						break
					}
					src.Discard(1)
				}
				return nil, nil
			case '=':
				src.Discard(2)
				return &token.Token{
					Type: token.DivEq,
				}, nil
			}
		}
	case '%':
		if len(bs) >= 2 && bs[1] == '=' {
			src.Discard(2)
			return &token.Token{
				Type: token.ModEq,
			}, nil
		}
	case '=':
		if len(bs) >= 2 && bs[1] == '=' {
			src.Discard(2)
			return &token.Token{
				Type: token.Eq,
			}, nil
		}
	case '<':
		if t.headerNameExpected() {
			s, err := lex.ReadHeaderName(src)
			if err != nil {
				return nil, err
			}
			return &token.Token{
				Type:        token.HeaderName,
				StringValue: s,
			}, nil
		}
		if len(bs) >= 2 && bs[1] == '<' {
			if len(bs) >= 3 && bs[2] == '=' {
				src.Discard(3)
				return &token.Token{
					Type: token.ShlEq,
				}, nil
			}
			src.Discard(2)
			return &token.Token{
				Type: token.Shl,
			}, nil
		}
	case '>':
		if len(bs) >= 2 && bs[1] == '>' {
			if len(bs) >= 3 && bs[2] == '=' {
				src.Discard(3)
				return &token.Token{
					Type: token.ShrEq,
				}, nil
			}
			src.Discard(2)
			return &token.Token{
				Type: token.Shr,
			}, nil
		}
	case '&':
		if len(bs) >= 2 {
			switch bs[1] {
			case '&':
				src.Discard(2)
				return &token.Token{
					Type: token.AndAnd,
				}, nil
			case '=':
				src.Discard(2)
				return &token.Token{
					Type: token.AndEq,
				}, nil
			}
		}
	case '|':
		if len(bs) >= 2 {
			switch bs[1] {
			case '|':
				src.Discard(2)
				return &token.Token{
					Type: token.OrOr,
				}, nil
			case '=':
				src.Discard(2)
				return &token.Token{
					Type: token.OrEq,
				}, nil
			}
		}
	case '!':
		if len(bs) >= 2 && bs[1] == '=' {
			src.Discard(2)
			return &token.Token{
				Type: token.Ne,
			}, nil
		}
	case '^':
		if len(bs) >= 2 && bs[1] == '=' {
			src.Discard(2)
			return &token.Token{
				Type: token.XorEq,
			}, nil
		}
	case '\'':
		// Char literal
		n, err := lex.ReadChar(src)
		if err != nil {
			return nil, err
		}
		return &token.Token{
			Type:        token.NumberLiteral,
			NumberValue: ctype.Int(n),
		}, nil
	case '"':
		if t.headerNameExpected() {
			s, err := lex.ReadHeaderName(src)
			if err != nil {
				return nil, err
			}
			return &token.Token{
				Type:        token.HeaderName,
				StringValue: s,
			}, nil
		}
		// String literal
		s, err := lex.ReadString(src)
		if err != nil {
			return nil, err
		}
		return &token.Token{
			Type:        token.StringLiteral,
			StringValue: s,
		}, nil
	case '.':
		if len(bs) >= 2 {
			if bs[1] == '.' && len(bs) >= 3 && bs[2] == '.' {
				src.Discard(3)
				return &token.Token{
					Type: token.DotDotDot,
				}, nil
			}
			if '0' <= bs[1] && bs[1] <= '9' {
				n, err := lex.ReadNumber(src)
				if err != nil {
					return nil, err
				}
				return &token.Token{
					Type:        token.NumberLiteral,
					NumberValue: n,
				}, nil
			}
		}
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		n, err := lex.ReadNumber(src)
		if err != nil {
			return nil, err
		}
		return &token.Token{
			Type:        token.NumberLiteral,
			NumberValue: n,
		}, nil
	case '#':
		if len(bs) >= 2 && bs[1] == '#' {
			src.Discard(2)
			return &token.Token{
				Type: token.HashHash,
			}, nil
		}
	case ';', '(', ')', ',', '{', '}', '[', ']', '?', ':', '~':
		// Single char token
	default:
		if lex.IsNondigit(b) {
			name, err := lex.ReadIdentifier(src)
			if err != nil {
				return nil, err
			}
			if t, ok := token.KeywordToType(string(name)); ok {
				return &token.Token{
					Type: t,
				}, nil
			}
			return &token.Token{
				Type: token.Ident,
				Name: string(name),
			}, nil
		}
		// Invalid
		return nil, fmt.Errorf("tokenizer: invalid token: %s", string(b))
	}

	src.Discard(1)
	return &token.Token{
		Type: token.Type(bs[0]),
	}, nil
}

func (t *tokenizer) scan(src *bufio.Reader) ([]*token.Token, error) {
	ts := []*token.Token{}
	for {
		tk, err := t.next(src)
		if tk != nil {
			ts = append(ts, tk)
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	// Add '\n' if necessary in order to make preprocessing simple.
	if len(ts) == 0 || ts[len(ts)-1].Type != '\n' {
		ts = append(ts, &token.Token{Type: '\n'})
	}
	return ts, nil
}

func stripNewLineTokens(tokens []*token.Token) []*token.Token {
	r := []*token.Token{}
	for _, t := range tokens {
		if t.Type == '\n' {
			continue
		}
		r = append(r, t)
	}
	return r
}

func joinStringLiterals(tokens []*token.Token) []*token.Token {
	r := []*token.Token{}
	for _, t := range tokens {
		var last *token.Token
		if len(r) > 0 {
			last = r[len(r)-1]
		}
		if last != nil && last.Type == token.StringLiteral && t.Type == token.StringLiteral {
			last.StringValue += t.StringValue
			continue
		}
		r = append(r, t)
	}
	return r
}

func (t *tokenizer) tokenize(src io.Reader) ([]*token.Token, error) {
	// TODO: Add TokenReader instead of using slices
	// TODO: Count line numbers
	return t.scan(bufio.NewReader(ioutil.NewBackslashNewLineStripper(src)))
}

func Tokenize(src io.Reader) ([]*token.Token, error) {
	t := &tokenizer{
		src: bufio.NewReader(src),
	}
	return t.tokenize(src)
}

// TODO: Rename?
func FinishTokenize(tokens []*token.Token) []*token.Token {
	tokens = stripNewLineTokens(tokens)
	tokens = joinStringLiterals(tokens)
	return tokens
}
