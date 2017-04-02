package auth

import "fmt"

type CSRFConfig struct {
	Key string
}

func (c CSRFConfig) Validate() error {
	if len(c.Key) > 0 && len(c.Key) != 32 {
		return fmt.Errorf("bad CSRF key: want 32 bytes, got %d", len(c.Key))
	}
	return nil
}
