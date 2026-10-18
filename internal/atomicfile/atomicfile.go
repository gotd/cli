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
	"runtime"

	"github.com/go-faster/errors"
)

// WriteFile atomically replaces path with data. The data is synced before the
// rename and the directory after it, so after a crash the file holds either the
// old or the new contents, never a truncated mix, and a completed write
// survives power loss. Use it for state such as sessions and config.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, true)
}

// Create atomically creates path with data, failing with an error satisfying
// errors.Is(err, os.ErrExist) if path already exists. Like WriteFile, the file
// appears complete or not at all, even after a crash.
func Create(path string, data []byte, perm os.FileMode) error {
	tmp, err := writeTemp(path, data, perm, true)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp) }()

	// A hard link is the portable "rename unless the target exists".
	if err := os.Link(tmp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.Wrapf(os.ErrExist, "create %s", path)
		}
		return createExclusive(path, data, perm) // no hard links on this filesystem
	}
	return syncDir(filepath.Dir(path))
}

// createExclusive is Create without atomicity, for filesystems that do not
// support hard links: the file is created exclusively and synced.
func createExclusive(path string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm) // #nosec G304 // caller-provided path
	if err != nil {
		return errors.Wrap(err, "create")
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return errors.Wrap(err, "write")
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return errors.Wrap(err, "sync")
	}
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close")
	}
	return syncDir(filepath.Dir(path))
}

// Replace is WriteFile without the fsync: readers still never observe a
// partial file, but a crash may lose the most recent write. Use it for caches
// that are rewritten often and can be rebuilt.
func Replace(path string, data []byte, perm os.FileMode) error {
	return write(path, data, perm, false)
}

func write(path string, data []byte, perm os.FileMode, durable bool) error {
	// Replace the file a symlink points at rather than the symlink itself, so
	// a config managed as a dotfile symlink stays a symlink.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	tmp, err := writeTemp(path, data, perm, durable)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return errors.Wrap(err, "rename temp file")
	}
	if durable {
		return syncDir(filepath.Dir(path))
	}
	return nil
}

// writeTemp writes data to a new temporary file next to path and returns its
// name; with durable set the data is synced to disk.
func writeTemp(path string, data []byte, perm os.FileMode, durable bool) (_ string, rerr error) {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return "", errors.Wrap(err, "create temp file")
	}
	tmp := f.Name()
	defer func() {
		if rerr != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}()

	if err := f.Chmod(perm); err != nil {
		return "", errors.Wrap(err, "chmod temp file")
	}
	if _, err := f.Write(data); err != nil {
		return "", errors.Wrap(err, "write temp file")
	}
	if durable {
		if err := f.Sync(); err != nil {
			return "", errors.Wrap(err, "sync temp file")
		}
	}
	if err := f.Close(); err != nil {
		return "", errors.Wrap(err, "close temp file")
	}
	return tmp, nil
}

// syncDir makes a rename or link in dir durable. Windows cannot sync a
// directory handle (NTFS journals metadata itself), so it is skipped there.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
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
