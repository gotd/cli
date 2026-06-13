---
name: tg
description: Drive a Telegram account from the shell with the `tg` CLI — send/read/search messages, manage chats, contacts, groups and channels, upload/download media, and stream messages in real time. Use whenever the user asks to do something on Telegram (message someone, check chats, download a file, watch for messages) from this machine.
---

# tg — Telegram from the command line

`tg` is a single static Go binary (this repo: `gotd/cli`, built on `gotd/td`) for
driving a Telegram **personal account** or a **bot** non-interactively. It is
designed for agents: structured JSON output, stable peer syntax, explicit safety
gates.

## Before anything: check it's set up

`tg` needs a one-time config + login. Verify first, and stop to ask the user if
not authenticated — login is interactive and must be done by a human.

```bash
tg whoami -o json   # authenticated → {"schema":1,"data":{...}}; else errors
```

If it errors with "no config … run `tg init` first" or a not-authorized error:

```bash
tg init             # writes ~/.config/gotd/gotd.cli.yaml (release binaries embed app creds)
tg login            # QR by default: the USER scans from Settings → Devices
```

Release binaries embed app credentials; source builds need them supplied
(`tg init --app-id … --app-hash …`, from https://my.telegram.org). If `tg init`
errors with "no app credentials", that's the cause — ask the user.

Do not run `tg login` autonomously expecting it to succeed headless — QR/phone
login needs the user. `tg init` is safe to run yourself.

## Golden rules for agent use

- **Always pass `-o json`** (`--output json`) for anything you need to parse. It
  emits `{"schema":1,"data":…}` on **stdout**; logs/prompts/progress go to
  **stderr**. Pipe stdout to `jq`.
- **Peers** (`--peer`/`<peer>`) accept: `me` or `self` (Saved Messages),
  `@username`, a phone number, or a `t.me/…` link. Resolved access-hashes are
  cached locally, so reuse the same form.
- **Destructive commands require `--yes`**: `delete`, `delete-history`,
  `unpin-all`, and similar. Never add `--yes` without the user asking for the
  destructive action — confirm intent first.
- **Read before write.** Prefer `history`/`search`/`chats list` to understand
  state before sending, deleting, or editing.

## Common recipes

Note the shapes: `send`/`upload` take the peer via the `--peer` flag (default
self); most others take `<peer>` (and a `<message-id>`) as **positional** args.

```bash
# Message yourself (Saved Messages) and a peer
tg send "note to self"
tg send --peer @durov "hello"

# List chats, extract usernames
tg chats list -o json | jq -r '.data.chats[].peer.username // empty'

# Read recent history / search (peer is positional)
tg history @gotd_test --limit 20 -o json
tg search @gotd_test "invoice" -o json     # within a chat
tg search --global "invoice" -o json       # across all chats

# Reply / edit / react: <peer> <message-id> [text|emoji], ids come from history
tg reply @x 12345 "on it"
tg edit  @x 12345 "fixed typo"
tg react @x 12345 👍

# Media (upload uses --peer; download is positional + --out)
tg upload --peer me ./report.pdf
tg upload --peer me ./clip.mp4 --type video
tg download @x 12345 --out ./downloads/

# Realtime
tg watch @x -o json                 # stream new messages as JSON lines (long-running)
tg wait  @x --timeout 30s -o json   # block for the next message, then exit

# Triage
tg read @x                          # mark read
tg mute @x
tg archive @x
```

Destructive (only when the user explicitly asks):

```bash
tg delete @x 12345 --yes            # <peer> <message-id>...
tg delete-history @x --yes
```

## Multiple accounts

```bash
tg accounts                          # list configured accounts + auth status
tg accounts add work --app-id … --app-hash …
tg login --account work              # or: tg login --account <new-label> (auto-created)
tg chats list --account work -o json
tg watch --account all -o json       # fan out across all accounts, labeled
```

`tg login --account <label>` creates the account entry on the fly (reusing the
build-time app credentials), so `tg accounts add` is only needed for custom app
credentials, a bot token, or a per-account proxy.

## Bots

```bash
tg init --token <bot-token>
tg whoami --bot
# pass --bot to commands that support a bot session
```

## Global flags

| Flag | Meaning |
| --- | --- |
| `-o, --output text\|json` | output format (use `json` for parsing) |
| `-a, --account <label>` | pick an account, or `all` to fan out |
| `-c, --config <path>` | config file to use |
| `--proxy <url>` | `socks5://…` or `tg://proxy?…` (MTProxy) |

## Discovery

The command surface is large and self-documenting. When unsure, ask `tg` rather
than guessing flags:

```bash
tg --help              # grouped command list
tg <command> --help    # flags + examples for one command
```

Notes:
- `--test` is set at config time (`tg init --test`), not per command.
- Errors print to stderr prefixed with `tg:`; a non-zero exit means failure.
