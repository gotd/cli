package main

import (
	"testing"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgmock"
)

// newFuncAPI returns a tg.Client whose invoker dispatches each request through
// fn. Unlike the Expect-based mock, fn may be called any number of times, which
// suits iterator-based commands that issue a trailing pagination request.
func newFuncAPI(_ *testing.T, fn func(req bin.Encoder) (bin.Encoder, error)) *tg.Client {
	return tg.NewClient(tgmock.Invoker(fn))
}
