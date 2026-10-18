//go:build !unix && !windows

package filelock

import "os"

// Platforms without file locking (js, wasip1, plan9) run tg as a single
// process; locking is a no-op there.
func lockFile(*os.File) error { return nil }

func unlockFile(*os.File) error { return nil }
