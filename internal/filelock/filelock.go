// Package filelock serializes read-modify-write cycles on files shared by
// several tg processes.
//
// Replacing a file atomically (see internal/atomicfile) keeps readers from
// seeing a torn file, but two processes that both read, modify and replace the
// same file still lose one another's changes. Holding an exclusive lock on a
// sidecar lock file for the whole cycle makes the cycles run one at a time.
//
// The lock is advisory (only cooperating tg processes honor it) and is released
// by the operating system if the holder dies, so a crash never leaves it stuck.
package filelock

import (
	"os"

	"github.com/go-faster/errors"
)

// Lock blocks until it holds an exclusive lock on path, creating the file if
// needed, and returns the function that releases it. Lock files are left in
// place: removing one while another process waits on it would break mutual
// exclusion.
func Lock(path string) (unlock func() error, err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) // #nosec G304 // path derived from the config dir
	if err != nil {
		return nil, errors.Wrap(err, "open lock file")
	}
	if err := lockFile(f); err != nil {
		_ = f.Close()
		return nil, errors.Wrap(err, "lock")
	}
	return func() error {
		uerr := unlockFile(f)
		if cerr := f.Close(); uerr == nil {
			uerr = cerr
		}
		return uerr
	}, nil
}
