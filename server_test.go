package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/boltdb/bolt"
)

func init() {
	log.SetOutput(ioutil.Discard)
}

func getBoltDB(t *testing.T) *bolt.DB {
	td, err := ioutil.TempDir("", "")
	if err != nil {
		t.Fatal(err)
	}
	fp := filepath.Join(td, "bolt.db")
	db, err := bolt.Open(fp, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := createHeaderBucketIfNotExists(db); err != nil {
		t.Fatal(err)
	}
	if err := createRootBucketIfNotExists(db); err != nil {
		t.Fatal(err)
	}
	return db
}

type server struct {
	*httptest.Server
	db *bolt.DB
}

func newServer(t *testing.T) server {
	t.Parallel()
	db := getBoltDB(t)
	ctx := context{db}
	return server{
		Server: httptest.NewServer(router{ctx: ctx}),
		db:     db,
	}
}

func TestRange(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	req, err := http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobarbaz"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	req, err = http.NewRequest("GET", s.URL+"/foo", nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		Range         string
		Expected      string
		ExpectedError string
		ExpectedCode  int
	}{
		{
			Range:        "bytes=0-2",
			Expected:     "foo",
			ExpectedCode: http.StatusPartialContent,
		},
		{
			Range:        "bytes=1-2,4-5",
			Expected:     "ooar",
			ExpectedCode: http.StatusPartialContent,
		},
		{
			Range:        "bytes=2-1000",
			Expected:     "Requested range not satisfiable.\n",
			ExpectedCode: http.StatusRequestedRangeNotSatisfiable,
		},
		{
			Range:        "runes=2-3",
			Expected:     "Bad request.\n",
			ExpectedCode: http.StatusBadRequest,
		},
	}

	for i, test := range tests {
		req.Header.Set("Range", test.Range)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := resp.StatusCode, test.ExpectedCode; got != want {
			t.Errorf("range test %d: bad status: got %d, want %d", i, got, want)
		}
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(b), test.Expected; got != want {
			t.Errorf("range test %d: bad response body: got %q, want %q", i, got, want)
		}
	}
}

func TestIfMatch(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	req, err := http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobar"))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	eTag := resp.Header.Get("ETag")
	req, err = http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobarbaz"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("If-Match", "foo")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusPreconditionFailed; got != want {
		t.Errorf("bad status: got %d, want %d", got, want)
	}

	req, err = http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobarbaz"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", eTag)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Errorf("bad status: got %d, want %d", got, want)
	}

	eTag = resp.Header.Get("ETag")

	req, err = http.NewRequest("DELETE", s.URL+"/foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", "foo")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusPreconditionFailed; got != want {
		t.Errorf("bad status: got %d, want %d", got, want)
	}

	req.Header.Set("If-Match", eTag)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusNoContent; got != want {
		t.Errorf("bad status: got %d, want %d", got, want)
	}

	req, err = http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobarbaz"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-Match", "*")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resp.StatusCode, http.StatusPreconditionFailed; got != want {
		t.Errorf("bad status: got %d, want %d", got, want)
	}
}

func TestIfNoneMatch(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	req, err := http.NewRequest("PUT", s.URL+"/foo", strings.NewReader("foobar"))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	eTag := resp.Header.Get("ETag")

	req, err = http.NewRequest("GET", s.URL+"/foo", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("If-None-Match", eTag)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusNotModified; got != want {
		t.Errorf("Bad status: got %d, want %d", got, want)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}

	if len(b) > 0 {
		t.Errorf("Response body not empty: %q", string(b))
	}
}

func TestDisallowedMethods(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	for _, method := range []string{"POST", "PATCH", "TRACE", "CONNECT"} {
		req, err := http.NewRequest(method, s.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := resp.StatusCode, http.StatusMethodNotAllowed; got != want {
			t.Errorf("Bad status code: got %d, want %d", got, want)
		}
	}
}

func TestEscapedPath(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	req, err := http.NewRequest("PUT", s.URL+"/foo/bar/baz", strings.NewReader("Hello"))
	if err != nil {
		t.Fatal(err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusCreated; got != want {
		t.Errorf("Bad status: got %d, want %d", got, want)
	}

	resp, err = http.Get(s.URL + "/foo/bar%2fbaz") // Fails if escaping is broken
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusNotFound; got != want {
		t.Errorf("Bad status: got %d, want %d", got, want)
	}

	resp, err = http.Head(s.URL + "/foo/bar%2fbaz") // Fails if escaping is broken
	if err != nil {
		t.Fatal(err)
	}

	if got, want := resp.StatusCode, http.StatusNotFound; got != want {
		t.Errorf("Bad status: got %d, want %d", got, want)
	}
}

// End-to-end test.
func TestCRUD(t *testing.T) {
	s := newServer(t)
	defer s.Close()
	client := &http.Client{}

	// Get the root bucket
	{
		resp, err := http.Get(s.URL)
		if err != nil {
			t.Fatal(err)
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if got, want := body, []byte("[]\n"); !bytes.Equal(got, want) {
			t.Errorf("bad body: got %q, want %q", string(got), string(want))
		}
	}

	// Create a bucket, /foo/bar
	{
		req, err := http.NewRequest("PUT", s.URL+"/foo/bar", nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("bad status code: got %d, want %d", resp.StatusCode, http.StatusOK)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if len(b) != 0 {
			t.Fatalf("server wrote bytes when it shouldn't: %q", string(b))
		}
	}

	// Get the root bucket
	{
		resp, err := http.Get(s.URL)
		if err != nil {
			t.Fatal(err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("bad status code: got %d, want %d", resp.StatusCode, http.StatusOK)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		// Expect to see a bucket "foo"
		got, want := []string{}, []string{"foo"}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("bad bucket list: got %v, want %v", got, want)
		}
	}

	// Get bucket "foo"
	{
		resp, err := http.Get(s.URL + "/foo")
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		got, want := []string{}, []string{"bar"}
		if err := json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("bad bucket list: got %v, want %v", got, want)
		}
	}

	var eTag string

	// Create a JSON document, /foo/xfiles
	{
		b, err := json.Marshal(map[string]interface{}{
			"The X-Files": map[string]interface{}{
				"Fox Mulder":  "FBI",
				"Dana Scully": "FBI",
			},
		})

		req, err := http.NewRequest("PUT", s.URL+"/foo/xfiles", bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		if eTag = resp.Header.Get("ETag"); len(eTag) == 0 {
			t.Error("zero-length ETag")
		}

		if got, want := eTag, etag(b); got != want {
			t.Errorf("bad etag: got %q, want %q", got, want)
		}

		respBody, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if len(respBody) != 0 {
			t.Errorf("unexpected bytes in response body: %q", string(respBody))
		}
	}

	// Get the header for the JSON document.
	// Expect Content-Type, Content-Length, ETag, Last-Modified to be present.
	{
		resp, err := http.Head(s.URL + "/foo/xfiles")
		if err != nil {
			t.Fatal(err)
		}

		if got := resp.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Bad Content-Type: got %q, want %q", got, "application/json")
		}

		if got := resp.Header.Get("Content-Length"); got != "56" {
			t.Errorf("Bad Content-Length: got %q, want %q", got, "56")
		}

		if got := resp.Header.Get("ETag"); got != eTag {
			t.Errorf("Bad ETag: got %q, want %q", got, eTag)
		} else if len(got) != 12 {
			t.Errorf("ETag wrong length: got %q, want %q", len(got), 12)
		}

		lastModified := resp.Header.Get("Last-Modified")
		if _, err := time.Parse(time.RFC1123Z, lastModified); err != nil {
			t.Errorf("bad Last-Modified: got %q", lastModified)
		}
	}

	// Get the JSON document. Expect Content-Type, Content-Length and ETag to be present.
	{
		want := map[string]interface{}{
			"The X-Files": map[string]interface{}{
				"Fox Mulder":  "FBI",
				"Dana Scully": "FBI",
			},
		}

		resp, err := http.Get(s.URL + "/foo/xfiles")
		if err != nil {
			t.Fatal(err)
		}

		if got := resp.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Bad Content-Type: got %q, want %q", got, "application/json")
		}

		if got := resp.Header.Get("Content-Length"); got != "56" {
			t.Errorf("Bad Content-Length: got %q, want %q", got, "56")
		}

		if got := resp.Header.Get("ETag"); got != eTag {
			t.Errorf("Bad ETag: got %q, want %q", got, eTag)
		}

		var got map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("bad content: got %#v, want %#v\n", got, want)
		}
	}

	// Delete the document /foo/xfiles
	{
		req, err := http.NewRequest("DELETE", s.URL+"/foo/xfiles", nil)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}

		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		if len(b) > 0 {
			t.Errorf("response body not empty: %q", string(b))
		}
	}

	// Try and fail to get /foo/xfiles
	{
		resp, err := http.Get(s.URL + "/foo/xfiles")
		if err != nil {
			t.Fatal(err)
		}

		if got, want := resp.StatusCode, http.StatusNotFound; got != want {
			t.Errorf("bad status: got %d, want %d", got, want)
		}
	}
}
