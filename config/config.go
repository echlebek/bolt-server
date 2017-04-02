// Copyright 2017 Eric Chlebek. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package config

import (
	"fmt"
	"io/ioutil"

	"github.com/echlebek/bolt-server/auth"

	yaml "gopkg.in/yaml.v2"
)

func New(path string) (Data, error) {
	data := Data{}
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return data, fmt.Errorf("couldn't read config file: %s", err)
	}
	if err = yaml.Unmarshal(b, &data); err != nil {
		err = fmt.Errorf("couldn't unmarshal config data: %s", err)
	}
	if err = data.CSRF.Validate(); err != nil {
		return data, fmt.Errorf("validation error: %s", err)
	}
	return data, err
}

type Data struct {
	TLS  auth.TLSConfig
	CSRF auth.CSRFConfig
}
