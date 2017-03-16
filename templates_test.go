package main

import (
	"io/ioutil"
	"testing"
)

func TestKeysTemplate(t *testing.T) {
	t.Parallel()
	data := &KeyPkg{
		Path: "foo",
		Keys: []string{"foo", "bar"},
	}
	if err := keysTmpl.Execute(ioutil.Discard, data); err != nil {
		t.Error(err)
	}
}
