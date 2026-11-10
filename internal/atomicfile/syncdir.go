//go:build !windows

package atomicfile

import (
	"os"

	"github.com/go-faster/errors"
)

// syncDir makes a rename or link in dir durable.
func syncDir(dir string) error {
	d, err := os.Open(dir) // #nosec G304 // directory of a caller-provided path
	if err != nil {
		return errors.Wrap(err, "open directory")
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return errors.Wrap(err, "sync directory")
	}
	return nil
}
