---
name: tg
description: Drive a Telegram account from the shell with the `tg` CLI — send/read/search messages, manage chats, contacts, groups and channels, upload/download media, stream messages in real time, and test Telegram bots end to end (commands, inline buttons, deep links, multi-step conversations, media). Use whenever the user asks to do something on Telegram (message someone, check chats, download a file, watch for messages) or to test, exercise or verify a Telegram bot from this machine.
---

# tg — Telegram from the command line

`tg` is a single static Go binary (this repo: `gotd/cli`, built on `gotd/td`) for
driving a Telegram **personal account** or a **bot** non-interactively. It is
designed for agents: structured JSON output, stable peer syntax, explicit safety
gates.

## Before anything: check it's set up

`tg` needs a one-time config + login. Verify first, and stop to ask the user if
not authenticated — login is interactive and must be done by a human.

If the user's instructions (CLAUDE.md, memory) name a config path or account
label, pass them on **every** call (`-c <path> -a <label>`) and never run
`tg init` or `tg login` yourself in that setup: a missing `-c` only means the
default config is absent, not that tg is unconfigured.

```bash
tg whoami -o json   # authenticated → {"schema":1,"data":{...}}; else errors
tg doctor -o json   # when anything fails: checks config, credentials, session,
                    # peer cache, connection and login, each with a fix hint
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
- **Bound every call** with `--timeout 60s` (exit code 6 when it fires) so a
  network stall never hangs the session.
- **Branch on exit codes**, not error text: 0 ok, 2 usage, 3 auth (the user must
  `tg login`), 4 not found (peer/message/button), 5 flood wait, 6 timeout,
  7 other Telegram error, 130/143 interrupted (SIGINT/SIGTERM). With `-o json` a failure also prints
  `{"schema":1,"error":{"code":…,"exit":…,"message":…,"retry_after":…}}` on stdout.

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

## Testing bots

`tg` acts as a real user, so it drives any bot end to end. The loop is: act,
collect the response, assert, repeat.

```bash
# 1. Discover what the bot offers: descriptions, command menu, menu button
tg bot-info @mybot -o json

# 2. Act and collect the response in one call. tg listens before sending, so a
#    fast reply is never missed. It stops when the bot has been quiet for
#    --settle (default 2s; longer while it shows "typing…"), after --replies N
#    messages, or when --wait runs out.
tg send --peer @mybot /start --wait 30s -o json
tg start-bot @mybot ref_42 --wait 20s -o json      # deep link t.me/mybot?start=ref_42

# 3. Inspect the keyboard of the last reply
tg send --peer @mybot /menu --wait 20s -o json | jq '.data.replies[-1].reply_markup'

# 4. Press buttons (the newest message with buttons unless a message id is given)
tg click @mybot --text "Settings" --wait 15s -o json   # callback: .data.answer + edits
tg click @mybot 4242 --data "page:2" --wait 15s -o json
tg click @mybot --row 2 --col 1 --wait 15s             # reply keyboard: sends its label

# 5. Multi-step flows: chain calls; answer a prompt by replying to it if needed
tg reply @mybot 4250 "Berlin" --wait 20s -o json

# 6. Media handling
tg upload --peer @mybot ./sample.pdf --wait 30s -o json
tg download @mybot 4260 --out ./evidence/

# Secrets (codes, keys) go through stdin, never argv
printf '%s' "$CODE" | tg send --peer @mybot --stdin --wait 20s -o json

# Replies to a message sent earlier, without waiting again
tg history @mybot --after 4242 -o json
tg webapp @mybot --out session.json -o json            # open its Mini App as this account: Telegram signs real launch data
tg webapp @mybot --text "Open store" --out session.json  # or a web app button; --url, --main and --start-param too
```

Result shapes (`.data`):

- `send`/`reply`/`start-bot`: `message_id`; `upload`: `files[]`. With `--wait`
  also `replies` (bot messages ordered after your action, in their latest
  version), `edits` (other messages the bot edited, e.g. the one whose button
  you pressed), `linked` (ids Telegram ties to your action: a `reply_to` your
  message or an edit of the pressed message), `earlier` (bot messages sent
  before your action, e.g. a late answer to the previous step: not responses)
  and `timed_out` (true when `--wait` ran out before the bot settled).
  `replies` is always a list; `edits`, `linked`, `earlier` and `timed_out`
  are omitted when empty or false, so read them with a default
  (`jq '.data.linked // []'`, Python `.get("linked") or []`).
- `webapp`: launch data is a credential for that Mini App, so keep it in the owner-only `--out` file. Use `init_data` as `Authorization: tma <init_data>` for the app's API, or load `url` from the file in a browser. Fields: `url` (the page), `source`, `signed`, `user_id`, `auth_date`, `query_id`, `out_file`. `signed_url` and `init_data` appear only with `--reveal`.
- `click`: `button` (`type`, `text`, `data`, `url`), `row`, `col`, `answer`
  (`answered`, `message`, `alert`, `url`: the toast/alert; `answered: false`
  means the bot did not answer the callback in time), plus `replies`/`edits`.
- Every message: `id`, `out`, `text`, `entities[]` (`type`, `text`, `url`),
  `reply_markup` (`type`: inline/keyboard/remove/force_reply, `rows[][]`),
  `file` (`kind`: photo/video/voice/audio/sticker/animation/file, `name`,
  `mime`, `size`), `edit_date`, `reply_to`.

What to assert: reply text and formatting, the buttons offered, callback
answers, which message was edited, media kind/size — and that nothing arrives
when nothing should (`timed_out: true` with empty `replies`). Treat `linked` as
proof a message answers your action; anything else in `replies` is what the bot
did right after it, which is the response unless the bot also sends unprompted
messages. Drive one conversation per bot at a time: two parallel flows with the
same bot from one account cannot be told apart. A non-empty `earlier` means the
bot is still answering a previous step: slow it down. Set `--wait` a
little above the bot's worst-case latency; use `--replies N` when the count is
known. Bots and Telegram rate-limit: pace bursts, don't loop the same command,
and prefer one `--wait` call over polling `history`. On exit code 5, wait
`.error.retry_after` seconds before retrying.

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
| `-c, --config <path>` | config file to use (env `TG_CONFIG`) |
| `--proxy <url>` | `socks5://…` or `tg://proxy?…` (MTProxy) |
| `--timeout <dur>` | abort after this long, exit code 6 (env `TG_TIMEOUT`) |

## Discovery

The command surface is large and self-documenting. When unsure, ask `tg` rather
than guessing flags:

```bash
tg --help              # grouped command list
tg <command> --help    # flags + examples for one command
```

Notes:
- `--test` is set at config time (`tg init --test`), not per command.
- Errors print to stderr prefixed with `tg:`; a non-zero exit means failure
  (see the exit codes above).
- `tg version -o json` shows which build is running.
