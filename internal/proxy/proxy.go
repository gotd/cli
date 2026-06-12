// Package proxy builds a gotd/td dcs.Resolver from a proxy URL, so every
// command can route MTProto traffic through a SOCKS5 or MTProxy proxy.
//
// Supported URL schemes:
//
//	socks5://[user:pass@]host:port      SOCKS5 (also socks5h://)
//	tg://proxy?server=&port=&secret=     MTProxy (native)
//
// An empty URL yields a nil resolver (use the default).
package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/url"
	"strings"

	"github.com/go-faster/errors"
	xproxy "golang.org/x/net/proxy"

	"github.com/gotd/td/telegram/dcs"
)

// kind classifies a parsed proxy URL.
type kind int

const (
	kindSOCKS kind = iota
	kindMTProxy
)

// parsed is the normalized form of a proxy URL.
type parsed struct {
	kind kind
	url  *url.URL // for SOCKS
	addr string   // host:port for MTProxy
	// secret for MTProxy.
	secret []byte
}

// parse normalizes a proxy URL string.
func parse(raw string) (parsed, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return parsed{}, errors.Wrap(err, "parse proxy url")
	}

	switch scheme := strings.ToLower(u.Scheme); scheme {
	case "socks5", "socks5h", "socks4", "socks4a":
		return parsed{kind: kindSOCKS, url: u}, nil
	case "tg", "mtproxy":
		return parseMTProxy(u)
	default:
		return parsed{}, errors.Errorf("unsupported proxy scheme %q (use socks5:// or tg://proxy?...)", scheme)
	}
}

// parseMTProxy extracts the server/port/secret from a tg://proxy?... link.
func parseMTProxy(u *url.URL) (parsed, error) {
	q := u.Query()
	server, port, secret := q.Get("server"), q.Get("port"), q.Get("secret")
	if server == "" || port == "" || secret == "" {
		return parsed{}, errors.New("mtproxy url requires server, port and secret")
	}
	raw, err := decodeSecret(secret)
	if err != nil {
		return parsed{}, err
	}
	return parsed{
		kind:   kindMTProxy,
		addr:   net.JoinHostPort(server, port),
		secret: raw,
	}, nil
}

// decodeSecret accepts a hex or base64url-encoded MTProxy secret.
func decodeSecret(s string) ([]byte, error) {
	if b, err := hex.DecodeString(s); err == nil {
		return b, nil
	}
	if b, err := base64.RawURLEncoding.DecodeString(s); err == nil {
		return b, nil
	}
	return nil, errors.Errorf("invalid mtproxy secret %q (want hex or base64url)", s)
}

// Resolver returns a dcs.Resolver for the given proxy URL, or (nil, nil) when
// raw is empty (meaning: use gotd's default resolver).
func Resolver(raw string) (dcs.Resolver, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	p, err := parse(raw)
	if err != nil {
		return nil, err
	}

	switch p.kind {
	case kindSOCKS:
		return socksResolver(p.url)
	case kindMTProxy:
		r, err := dcs.MTProxy(p.addr, p.secret, dcs.MTProxyOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "mtproxy resolver")
		}
		return r, nil
	default:
		return nil, errors.Errorf("unsupported proxy kind %d", p.kind)
	}
}

// socksResolver builds a plain resolver dialing through a SOCKS proxy.
func socksResolver(u *url.URL) (dcs.Resolver, error) {
	d, err := xproxy.FromURL(u, xproxy.Direct)
	if err != nil {
		return nil, errors.Wrap(err, "socks dialer")
	}
	cd, ok := d.(xproxy.ContextDialer)
	if !ok {
		return nil, errors.New("socks dialer does not support contexts")
	}
	return dcs.Plain(dcs.PlainOptions{Dial: cd.DialContext}), nil
}
