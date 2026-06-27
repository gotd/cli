//go:build !darwin

package main

// keychainSessionStore reports that no Keychain backend exists off macOS, so
// newSessionStore falls back to file storage.
func keychainSessionStore(_, _, _ string) (sessionStore, bool) {
	return nil, false
}
