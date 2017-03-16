package ranger

import (
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

type parseTest struct {
	Ranges         []string
	Prefix         string
	ContentLength  int
	ExpectedRanges []Range
	ExpectedError  string
}

func TestParse(t *testing.T) {
	tests := []parseTest{
		{ // valid ranges are represented and merged
			Ranges: []string{
				"bytes=0-99",
				"bytes=50-99,200-300",
				"bytes=250-,-50",
			},
			Prefix:        "bytes=",
			ContentLength: 350,
			ExpectedRanges: []Range{
				{Start: 0, Stop: 99},
				{Start: 200, Stop: 349},
			},
			ExpectedError: "<nil>",
		},
		{ // ranges falling outside of maxLen return an error
			Ranges: []string{
				"bytes=0-99",
				"bytes=50-99",
				"bytes=200-300",
				"bytes=250-",
			},
			Prefix:         "bytes=",
			ContentLength:  200,
			ExpectedRanges: nil,
			ExpectedError:  "invalid range",
		},
		{ // Wrong prefix
			Ranges: []string{
				"foo=0-100",
			},
			Prefix:         "bytes=",
			ContentLength:  200,
			ExpectedRanges: nil,
			ExpectedError:  `strconv.ParseInt: parsing "foo=0": invalid syntax`,
		},
		{ // Empty
			ExpectedError:  "<nil>",
			ExpectedRanges: []Range{},
		},
	}

	for i, test := range tests {
		ranges, err := Parse(test.Ranges, test.Prefix, test.ContentLength)
		if got, want := fmt.Sprintf("%v", err), test.ExpectedError; got != want {
			t.Errorf("test %d: bad error: got %q, want %q", i, got, want)
		}
		if got, want := ranges, test.ExpectedRanges; !reflect.DeepEqual(got, want) {
			t.Errorf("test %d: bad ranges: got %+v, want %+v", i, got, want)
		}
	}
}

type headerTest struct {
	Header         http.Header
	Length         int
	ExpectedRanges []Range
	ExpectedError  string
}

func TestParseHeader(t *testing.T) {
	tests := []headerTest{
		{ // Happy path
			Header: http.Header{
				"Range": {"100-200"},
			},
			Length: 300,
			ExpectedRanges: []Range{
				{Start: 100, Stop: 200},
			},
			ExpectedError: "<nil>",
		},
	}
	for i, test := range tests {
		ranges, err := ParseHeader(test.Header, test.Length)
		if got, want := fmt.Sprintf("%v", err), test.ExpectedError; got != want {
			t.Errorf("test %d: bad error: got %q, want %q", i, got, want)
		}
		if got, want := ranges, test.ExpectedRanges; !reflect.DeepEqual(got, want) {
			t.Errorf("test %d: bad ranges: got %+v, want %+v", i, got, want)
		}
	}
}
