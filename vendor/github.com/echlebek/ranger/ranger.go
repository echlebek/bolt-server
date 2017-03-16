/*
Copyright 2017 Eric Chlebek

Permission is hereby granted, free of charge, to any person obtaining a copy of
this software and associated documentation files (the "Software"), to deal in
the Software without restriction, including without limitation the rights to
use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
the Software, and to permit persons to whom the Software is furnished to do so,
subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
package ranger

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

var Error = errors.New("invalid range")

// Range is simply a contiguous range.
type Range struct {
	Start int
	Stop  int
}

func (b Range) overlaps(c Range) bool {
	return b.Start <= c.Stop && c.Start <= b.Stop
}

// valid iff b <= c
func (b Range) merge(c Range) Range {
	return Range{Start: b.Start, Stop: c.Stop}
}

type rangeSlice []Range

func (b rangeSlice) Len() int {
	return len(b)
}

func (b rangeSlice) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b rangeSlice) Less(i, j int) bool {
	if b[i].Start < b[j].Start {
		return true
	}
	if b[i].Start == b[j].Start {
		return b[i].Stop < b[j].Stop
	}
	return false
}

// ParseHeader parses an http.Header. It assumes that the range starts with
// 'bytes='. For other types of ranges, use Parse.
//
// The header must contain a valid Range field. Otherwise, Error will be
// returned.
func ParseHeader(h http.Header, contentLength int) ([]Range, error) {
	return Parse(h["Range"], "bytes=", contentLength)
}

// Parse parses an RFC2616 HTTP range. It accepts a slice of strings, each
// beginning with prefix and delimited with ','. contentLen is the size of the
// content being ranged over.
//
// Parse merges overlapping ranges together. The returned []Range will be
// sorted such that a.Start =< b.Start.
//
// If contentLen is < 0, then Error is returned. If any of the the ranges fall
// outside of 0 or contentLen, Error is returned.
func Parse(ranges []string, prefix string, contentLen int) ([]Range, error) {
	result := make([]Range, 0, len(ranges))
	for _, r := range ranges {
		r = strings.TrimPrefix(r, prefix)
		ranges := strings.Split(r, ",")
		for _, r := range ranges {
			parts := strings.Split(r, "-")
			if len(parts) != 2 {
				return nil, Error
			}
			if parts[0] == "" {
				y, err := strconv.Atoi(parts[1])
				if err != nil {
					return nil, err
				}
				if y < 0 || y > contentLen {
					return nil, Error
				}
				result = append(result, Range{Start: contentLen - y, Stop: contentLen - 1})
			} else if parts[1] == "" {
				x, err := strconv.Atoi(parts[0])
				if err != nil {
					return nil, err
				}
				if x < 0 || x >= contentLen {
					return nil, Error
				}
				result = append(result, Range{Start: x, Stop: contentLen - 1})
			} else {
				x, err := strconv.Atoi(parts[0])
				if err != nil {
					return nil, err
				}
				y, err := strconv.Atoi(parts[1])
				if err != nil {
					return nil, err
				}
				if x < 0 || y < 0 || x >= contentLen || y >= contentLen || x > y {
					return nil, Error
				}
				result = append(result, Range{Start: x, Stop: y})
			}
		}
	}
	result = mergeRanges(result)
	return result, nil
}

func mergeRanges(br []Range) []Range {
	if len(br) < 2 {
		return br
	}
	sort.Sort(rangeSlice(br))
	result := make([]Range, 0, len(br))
	cur := br[0]
	for i := 1; i < len(br); i++ {
		b := br[i]
		if cur.overlaps(b) {
			cur = cur.merge(b)
		} else {
			result = append(result, cur)
			cur = b
		}
	}
	result = append(result, cur)
	return result
}
