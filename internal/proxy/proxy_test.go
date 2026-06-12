package proxy

import (
	"encoding/hex"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name     string
		raw      string
		wantKind kind
		wantErr  bool
	}{
		{"socks5", "socks5://user:pass@127.0.0.1:1080", kindSOCKS, false},
		{"socks5h", "socks5h://127.0.0.1:1080", kindSOCKS, false},
		{"mtproxy", "tg://proxy?server=1.2.3.4&port=443&secret=deadbeef", kindMTProxy, false},
		{"mtproxy missing secret", "tg://proxy?server=1.2.3.4&port=443", 0, true},
		{"http unsupported", "http://127.0.0.1:8080", 0, true},
		{"unknown scheme", "ftp://x", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parse(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.kind != tc.wantKind {
				t.Errorf("kind = %d, want %d", got.kind, tc.wantKind)
			}
		})
	}
}

func TestDecodeSecret(t *testing.T) {
	hexSecret := "dd000102030405060708090a0b0c0d0e0f"
	b, err := decodeSecret(hexSecret)
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(b) != hexSecret {
		t.Errorf("hex roundtrip failed: %x", b)
	}

	// base64url secret.
	if _, err := decodeSecret("3q2-7w"); err != nil {
		t.Errorf("base64url decode failed: %v", err)
	}
}

func TestResolverEmpty(t *testing.T) {
	r, err := Resolver("")
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		t.Error("expected nil resolver for empty url")
	}
}

func TestResolverMTProxy(t *testing.T) {
	r, err := Resolver("tg://proxy?server=1.2.3.4&port=443&secret=dd00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r == nil {
		t.Error("expected non-nil resolver")
	}
}
