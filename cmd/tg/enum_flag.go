package main

import (
	"strings"

	"github.com/go-faster/errors"
)

// enumValue is a pflag.Value that only accepts one of a fixed set of strings.
// It powers both validation and shell completion for enum-style flags.
type enumValue struct {
	value   string
	allowed []string
}

func newEnumValue(def string, allowed ...string) *enumValue {
	return &enumValue{value: def, allowed: allowed}
}

// String implements pflag.Value.
func (e *enumValue) String() string { return e.value }

// Set implements pflag.Value.
func (e *enumValue) Set(v string) error {
	for _, a := range e.allowed {
		if v == a {
			e.value = v
			return nil
		}
	}
	return errors.Errorf("must be one of: %s", strings.Join(e.allowed, ", "))
}

// Type implements pflag.Value.
func (e *enumValue) Type() string { return "string" }
