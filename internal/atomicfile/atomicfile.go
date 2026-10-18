// Package atomicfile replaces files so that concurrent readers never observe a
// partially written file.
//
// Several tg processes commonly share one session directory (an agent running
// commands in parallel, or two agents driving the same account). A plain
// os.WriteFile truncates the file before writing it, so a reader that opens it
// mid-write sees an empty or torn file and fails to parse it. Writing to a
// temporary file in the same directory and renaming it over the target makes
// the replacement atomic: readers see either the old or the new contents.
package atomicfile

import (
	"os"
	"path/filepath"

	"github.com/go-faster/errors"
)

// WriteFile atomically replaces path with data and syncs it to disk before the
// rename, so a crash leaves either the old or the new file, never a truncated
// one. Use it for state that must survive a crash, such as sessions and config.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, true)
}

// Replace is WriteFile without the fsync: readers still never observe a
// partial file, but a crash may lose the most recent write. Use it for caches
// that are rewritten often and can be rebuilt.
func Replace(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, false)
}

func write(path string, data []byte, perm os.FileMode, durable bool) (rerr error) {
	// Replace the file a symlink points at rather than the symlink itself, so
	// a config managed as a dotfile symlink stays a symlink.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return errors.Wrap(err, "create temp file")
	}
	tmp := f.Name()
	defer func() {
		if rerr != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(perm); err != nil {
		return errors.Wrap(err, "chmod temp file")
	}
	if _, err := f.Write(data); err != nil {
		return errors.Wrap(err, "write temp file")
	}
	if durable {
		if err := f.Sync(); err != nil {
			return errors.Wrap(err, "sync temp file")
		}
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close temp file")
	}
	if err := os.Rename(tmp, path); err != nil {
		return errors.Wrap(err, "rename temp file")
	}
	return nil
}
