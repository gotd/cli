// Package peercache provides a file-backed implementation of peers.Storage.
//
// StringSession/FileStorage sessions keep no entity cache, so resolved
// access-hashes would be lost between CLI invocations. This storage persists
// them as JSON in the session directory, keyed per account, so peer resolution
// is cheap and survives restarts.
//
// Several tg processes may share one cache file. Writes replace the file
// atomically and merge in entries other processes added since it was loaded,
// so concurrent commands neither corrupt the file nor drop each other's
// access hashes (re-resolving a username costs a rate-limited RPC).
package peercache

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"sync"

	"github.com/go-faster/errors"

	"github.com/gotd/td/telegram/peers"

	"github.com/gotd/cli/internal/atomicfile"
)

// Storage is a JSON-file-backed peers.Storage.
type Storage struct {
	path string

	mu   sync.Mutex
	data data
}

type data struct {
	// Peers maps "<prefix>:<id>" to the access hash.
	Peers map[string]int64 `json:"peers"`
	// Phones maps a phone number to its peer key "<prefix>:<id>".
	Phones map[string]string `json:"phones"`
	// ContactsHash is the cached contacts hash.
	ContactsHash int64 `json:"contacts_hash"`
}

var _ peers.Storage = (*Storage)(nil)

// Open loads the cache at path, creating an empty one if it does not exist. An
// unreadable (corrupt) cache is treated as empty and rewritten on the next
// save: it only holds data that can be fetched again.
func Open(path string) (*Storage, error) {
	d, err := load(path)
	if err != nil {
		return nil, err
	}
	return &Storage{path: path, data: d}, nil
}

// load reads the cache file at path. A missing or corrupt file yields an empty
// cache; only I/O errors are returned.
func load(path string) (data, error) {
	d := data{
		Peers:  map[string]int64{},
		Phones: map[string]string{},
	}
	raw, err := os.ReadFile(path) // #nosec G304 // path derived from config dir
	if err != nil {
		if os.IsNotExist(err) {
			return d, nil
		}
		return d, errors.Wrap(err, "read peer cache")
	}
	var onDisk data
	if json.Unmarshal(raw, &onDisk) != nil {
		return d, nil
	}
	for k, v := range onDisk.Peers {
		d.Peers[k] = v
	}
	for k, v := range onDisk.Phones {
		d.Phones[k] = v
	}
	d.ContactsHash = onDisk.ContactsHash
	return d, nil
}

func keyString(k peers.Key) string {
	return k.Prefix + ":" + strconv.FormatInt(k.ID, 10)
}

// flush persists the cache; caller must hold mu. Entries another process saved
// since this cache was loaded are merged in first (this process's values win
// for keys both know), so concurrent writers do not drop each other's entries.
func (s *Storage) flush() error {
	onDisk, err := load(s.path)
	if err != nil {
		return err
	}
	for k, v := range onDisk.Peers {
		if _, ok := s.data.Peers[k]; !ok {
			s.data.Peers[k] = v
		}
	}
	for k, v := range onDisk.Phones {
		if _, ok := s.data.Phones[k]; !ok {
			s.data.Phones[k] = v
		}
	}

	raw, err := json.Marshal(s.data)
	if err != nil {
		return errors.Wrap(err, "marshal peer cache")
	}
	if err := atomicfile.Replace(s.path, raw, 0o600); err != nil {
		return errors.Wrap(err, "write peer cache")
	}
	return nil
}

// Save implements peers.Storage.
func (s *Storage) Save(_ context.Context, key peers.Key, value peers.Value) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Peers[keyString(key)] = value.AccessHash
	return s.flush()
}

// Find implements peers.Storage.
func (s *Storage) Find(_ context.Context, key peers.Key) (peers.Value, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hash, ok := s.data.Peers[keyString(key)]
	return peers.Value{AccessHash: hash}, ok, nil
}

// Peer kinds returned by Kind, matching the type names used in CLI output.
const (
	KindUser    = "user"
	KindChat    = "chat"
	KindChannel = "channel"
)

// Kind reports the cached peer kind ("user", "chat" or "channel") for a numeric
// id, if the peer has been seen before (e.g. via `tg chats list`). Users and
// channels are checked before chats so a populated access hash wins.
//
// gotd's peers.Storage keys entries as "<prefix>:<id>"; the prefixes are
// unexported there (telegram/peers/storage.go), so we mirror them here.
func (s *Storage) Kind(id int64) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idStr := strconv.FormatInt(id, 10)
	for _, kp := range []struct{ kind, prefix string }{
		{KindUser, "users_"},
		{KindChannel, "channel_"},
		{KindChat, "chats_"},
	} {
		if _, ok := s.data.Peers[kp.prefix+":"+idStr]; ok {
			return kp.kind, true
		}
	}
	return "", false
}

// SavePhone implements peers.Storage.
func (s *Storage) SavePhone(_ context.Context, phone string, key peers.Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.Phones[phone] = keyString(key)
	return s.flush()
}

// FindPhone implements peers.Storage.
func (s *Storage) FindPhone(_ context.Context, phone string) (peers.Key, peers.Value, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, ok := s.data.Phones[phone]
	if !ok {
		return peers.Key{}, peers.Value{}, false, nil
	}
	key, err := parseKey(raw)
	if err != nil {
		return peers.Key{}, peers.Value{}, false, err
	}
	return key, peers.Value{AccessHash: s.data.Peers[raw]}, true, nil
}

func parseKey(raw string) (peers.Key, error) {
	for i := len(raw) - 1; i >= 0; i-- {
		if raw[i] == ':' {
			id, err := strconv.ParseInt(raw[i+1:], 10, 64)
			if err != nil {
				return peers.Key{}, errors.Wrap(err, "parse peer key id")
			}
			return peers.Key{Prefix: raw[:i], ID: id}, nil
		}
	}
	return peers.Key{}, errors.Errorf("malformed peer key %q", raw)
}

// GetContactsHash implements peers.Storage.
func (s *Storage) GetContactsHash(_ context.Context) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.ContactsHash, nil
}

// SaveContactsHash implements peers.Storage.
func (s *Storage) SaveContactsHash(_ context.Context, hash int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ContactsHash = hash
	return s.flush()
}
