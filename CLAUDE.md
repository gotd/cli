# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`tg` is a single static Go binary for driving a Telegram personal account (or bot) from the
command line or an AI agent. It is built on [`gotd/td`](https://github.com/gotd/td) and is
designed to be agent-friendly: stable JSON output, consistent peer syntax, and explicit
safety gates.

## gotd/td reference

This CLI is a thin layer over `gotd/td`. When you need the exact shape of a Telegram API
method, request, or response type, **read the latest `gotd/td` source directly** rather than
guessing — it is the source of truth for generated `tg.*` types and `*tg.Client` methods.

It is checked out locally at **`../td`** or **`/src/gotd/td`**. Prefer the local checkout
(latest). Also confirm the version the CLI actually compiles against — it may differ from the
local checkout — with `go list -m -f '{{.Dir}}' github.com/gotd/td` (currently `v0.157.1`), and
read that module directory when the two disagree.

Useful entry points in `gotd/td`:
- `tg/tl_*_gen.go` — generated request/response structs and `*tg.Client` RPC methods (e.g.
  `tg/tl_messages_export_chat_invite_gen.go`). Optional fields use `SetX`/`GetX` helpers.
- `telegram/peers` — the peer/access-hash `Manager` (`Resolve`, `ResolveUserID`, `Apply`, …).
- `telegram/message` — the `Sender` fluent builder used for sending.
- `telegram/query` — paginated iterators (e.g. `query.GetDialogs`).

## Common commands

```bash
go build ./...                       # build everything
go run ./cmd/tg <command> --help     # run the CLI locally (binary is package cmd/tg, named "tg")
go test ./...                        # all tests
go test -race ./...                  # race detector (what CI runs)
go test ./cmd/tg/ -run TestName      # a single test
golangci-lint run ./...              # lint (config in .golangci.yml; CI pins v2.12.1)
gofmt -l .                           # list misformatted files; gofmt -w to fix
go mod tidy                          # CI fails if this produces a diff
```

Go 1.25. Releases are cut by pushing a `v*` tag, which triggers GoReleaser
(`.github/workflows/release.yml`); the release build embeds app credentials via `-ldflags` and
fails if the `APP_ID`/`APP_HASH` secrets are missing.

Commits must follow Conventional Commits (`feat(...)`, `ci:`, `fix:`, …) — enforced by
`.github/workflows/commitlint.yml`.

## Architecture

The whole CLI lives in `cmd/tg/` (package `main`). Supporting packages are under `internal/`.

**Command pattern.** Each command is a `func (a *app) newXxxCmd() *cobra.Command` method
(spf13/cobra) registered in `root.go`'s `newRootCmd`. Commands carry a `GroupID`
(`groupAuth`/`groupMessaging`/`groupChats`/`groupProfile`). Parent-with-subcommands is done via
`cmd.AddCommand(...)` (see `devices`, `chats`, `invite-link`).

**The `app.run` envelope.** Almost every command body is:

```go
return a.run(ctx, runParams{auth: authUser}, func(ctx context.Context, api *tg.Client) error {
    // ... use api (*tg.Client) ...
})
```

`app.run` (`app.go`) resolves the selected account(s) — `--account all` fans out across
configured accounts sequentially, labeling each result — connects, ensures the session is
authorized (or authorizes a bot via its token), and invokes the callback with a ready
`*tg.Client`. This is the single seam where connection, multi-account, and auth live; don't
reimplement it per command.

**Output.** Never print results directly. Build a result struct and call `a.printer.Emit(v)`
(`internal/output`). `Emit` writes either the JSON envelope `{"schema":1,"data":…}` (when
`-o json`) or, for text mode, calls the value's `MarshalText(w io.Writer)` if it implements
`output.TextMarshaler`. So every result type gets a JSON shape for free and an optional text
renderer. Generic acknowledgements use `okResult{OK: true}`. Logs/prompts go to stderr.

**Peer resolution** (`peers.go`) is central and shared. `a.manager(api)` returns a
`*peerManager` that embeds `gotd/td`'s `peers.Manager` and bundles the persistent access-hash
cache (`internal/peercache`, a JSON file in the session dir — `gotd/td`'s string sessions keep
no entity cache, so this persists hashes across invocations). Resolve peers with:
- `resolvePeer(ctx, m, arg)` → `tg.InputPeerClass`, or
- `m.Resolve(ctx, arg)` / `resolvePeerArg(ctx, m, arg)` → typed `peers.Peer` when you must
  branch on user/chat/channel (e.g. to pick `channels.*` vs `messages.*` methods).

A `<peer>` argument accepts `me`/`self`, `@username`, a phone number, a `t.me/…` link, or
`id:<n>` (a raw numeric id from `tg chats list --output json`, resolved from the cache — works
only after the peer has been cached). For sending, `builderFor(sender, peer)` turns a peer
string into a `message.RequestBuilder` (the sender's resolver also understands `id:`).

**Testing convention.** Keep network code thin and factor the logic into pure functions that
are unit-tested directly (e.g. `inviteLinkFromFull`, `newDevicesResult`). For code that must
issue RPCs, use the mock invoker helpers in tests: `newTestAPI(t)` (tgmock, `Expect()`-based)
and `newFuncAPI(t, fn)` (callback invoker). Tests run with `-race` in CI.

## Conventions to match

- Destructive actions require a `--yes` flag and return an error if it's absent (see
  `delete.go`, `pin.go`); follow that pattern for anything irreversible.
- Wrap errors with `github.com/go-faster/errors` (`errors.Wrap`/`Wrapf`), tagging the RPC
  name, e.g. `errors.Wrap(err, "messages.exportChatInvite")`. Note `errors.Wrap(nil)` is
  **non-nil** — guard with `if err != nil` before wrapping.
- Group/channel commands that differ by peer type switch on `peers.Channel` vs `peers.Chat`
  and call `channels.*` vs `messages.*` accordingly.
