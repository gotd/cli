package main

import (
	"flag"
	"strings"

	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

// EnumFlag is a simple wrapper for StringFlag to make a string enum flag.
type EnumFlag struct {
	cli.StringFlag
	Allowed []string
}

// GetUsage returns the usage string for the flag
func (e *EnumFlag) GetUsage() string {
	return e.Usage + "(" + "allowed: " + strings.Join(e.Allowed, ", ") + ")"
}

// Apply implements cli.Flag.
func (e *EnumFlag) Apply(set *flag.FlagSet) error {
	if err := e.StringFlag.Apply(set); err != nil {
		return err
	}

	if !e.HasBeenSet {
		return nil
	}

	for i := range e.Allowed {
		if e.Value == e.Allowed[i] {
			return nil
		}
	}
	return xerrors.Errorf("allowed values are %s", strings.Join(e.Allowed, ", "))
}
