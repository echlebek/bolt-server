// Copyright 2017 Eric Chlebek. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package server

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
