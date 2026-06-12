package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gotd/td/tg"
)

func TestUsersToPeerList(t *testing.T) {
	res := usersToPeerList([]tg.UserClass{
		&tg.User{ID: 1, Username: "alice", FirstName: "Alice"},
		&tg.UserEmpty{ID: 2},
		&tg.User{ID: 3, FirstName: "Bob"},
	})
	if len(res.Peers) != 2 {
		t.Fatalf("got %d peers, want 2 (empty skipped)", len(res.Peers))
	}
	if res.Peers[0].Username != "alice" {
		t.Errorf("peer0 = %+v", res.Peers[0])
	}
}

func TestReadContactsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contacts.csv")
	content := "# header\n+100,Alice,Smith\n+200,Bob\n\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	contacts, err := readContactsFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}
	if contacts[0].Phone != "+100" || contacts[0].FirstName != "Alice" || contacts[0].LastName != "Smith" {
		t.Errorf("contact0 = %+v", contacts[0])
	}
	if contacts[1].Phone != "+200" || contacts[1].FirstName != "Bob" {
		t.Errorf("contact1 = %+v", contacts[1])
	}
}

func TestReadContactsFileEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.csv")
	if err := os.WriteFile(path, []byte("# only a comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readContactsFile(path); err == nil {
		t.Error("expected error for file with no contacts")
	}
}
