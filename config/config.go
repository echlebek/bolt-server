package config

import (
	"fmt"
	"io/ioutil"

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
	return data, err
}

type Data struct {
}