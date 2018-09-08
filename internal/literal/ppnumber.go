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

package literal

import (
	"fmt"
	"io"

	"github.com/hajimehoshi/goc/internal/ioutil"
)

func isNondigit(c byte) bool {
	if 'A' <= c && c <= 'Z' {
		return true
	}
	if 'a' <= c && c <= 'z' {
		return true
	}
	if c == '_' {
		return true
	}
	return false
}

func ReadPPNumber(src Source) (string, error) {
	b, err := ioutil.ShouldReadByte(src)
	if err != nil {
		return "", err
	}

	if !isDigit(b) && b != '.' {
		return "", fmt.Errorf("literal: expected digit or . but %q", string(rune(b)))
	}

	r := []byte{b}

	for {
		bs, err := src.Peek(1)
		if err != nil && err != io.EOF {
			return "", err
		}
		if len(bs) == 0 {
			if err != io.EOF {
				panic("not reached")
			}
			break
		}

		b := bs[0]
		if !isDigit(b) && b != '.' && !isNondigit(b) {
			break
		}
		src.Discard(1)
		r = append(r, b)

		if b != 'e' && b != 'E' && b != 'p' && b != 'P' {
			continue
		}

		bs, err = src.Peek(1)
		if err != nil && err != io.EOF {
			return "", err
		}
		if len(bs) == 0 {
			if err != io.EOF {
				panic("not reached")
			}
			break
		}
		b = bs[0]

		if b == '+' || b == '-' {
			src.Discard(1)
			r = append(r, b)
			continue
		}
	}

	return string(r), nil
}