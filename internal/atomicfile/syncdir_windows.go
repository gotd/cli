//go:build windows

package atomicfile

// syncDir is a no-op: Windows cannot sync a directory handle, and NTFS
// journals metadata changes such as renames itself.
func syncDir(string) error { return nil }
