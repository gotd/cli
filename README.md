# tg — Telegram CLI for humans and agents

`tg` is a single static Go binary for driving a Telegram **personal account** (or a
bot) from the command line or an AI agent: message yourself, list and triage chats,
read history, send/forward/edit/delete messages, upload and download files, manage
groups and channels, and stream new messages in real time.

Built on [`gotd/td`](https://github.com/gotd/td).

```console
$ go install github.com/gotd/cli/cmd/tg@latest
```

## Quick start

Get an `app_id` / `app_hash` from <https://my.telegram.org>, then:

```console
$ tg init --app-id APP_ID --app-hash APP_HASH      # write config (bot token optional)
$ tg login                                         # QR login — scan once from a logged-in device
$ tg whoami                                         # confirm you're authenticated
$ tg send "Hello world"                             # message yourself (Saved Messages)
$ tg send --peer @gotd_test "Hello world"           # message a peer
```

The config is written to the `gotd` subdirectory of your config dir, e.g.
`~/.config/gotd/gotd.cli.yaml`. The session persists there too, so subsequent
commands run headless.

### Login options

```console
$ tg login                     # QR (default): scan from Settings → Devices → Link Desktop Device
$ tg login --phone +1234567890  # phone-code login (use --phone= to be prompted for the number)
$ TG_PASSWORD=secret tg login   # supply the 2FA cloud password non-interactively
```

Bot login still works too: provide a token via `tg init --token <bot-token>` and pass
`--bot` to commands that support it (e.g. `tg whoami --bot`).

## Agent-friendly by design

- **Structured output:** `--output json` (`-o json`) on any command emits a stable
  envelope `{"schema":1,"data":…}` on stdout. Logs, prompts and progress go to stderr.
- **Consistent peers:** every `<peer>` accepts `me`/`self`, `@username`, a phone number,
  or a `t.me/…` link. Resolved access-hashes are cached locally.
- **Safety:** destructive actions (`delete`, `delete-history`, `unpin-all`) require `--yes`.
- **Shell completion:** `tg completion bash|zsh|fish|powershell`, with dynamic completion
  for peers, accounts, output format and enum flags.

```console
$ tg chats list --output json | jq '.data.chats[].peer.username'
$ tg history @durov --limit 20 -o json
```

## What you can do

Run `tg --help` for the full, grouped command list, or `tg <command> --help` for details.

- **Messaging:** `send`, `reply`, `edit`, `delete`, `delete-history`, `forward`,
  `history`, `search` (`--global`), `read`, `pin`/`unpin`/`unpin-all`/`pinned`,
  `react`/`unreact`/`reactions`, `draft`/`drafts`, `schedule`, `poll`, `link`, `context`.
- **Media:** `upload` (with `--type` video/audio/voice/gif/sticker), `album`, `download`,
  `stickers`.
- **Chats & contacts:** `chats list`, `chat get`/`full`, `mute`/`unmute`,
  `archive`/`unarchive`, `resolve`, `search-public`, `subscribe`, `contacts` (list,
  search, add, delete, block/unblock, blocked, import).
- **Groups & channels:** `create-group`, `create-channel`, `invite`, `leave`,
  `participants`/`admins`/`banned`, `promote`/`demote`, `ban`/`unban`, `slow-mode`,
  `set-title`/`set-about`/`set-photo`, `invite-link`/`join-link`, `topics`,
  `recent-actions`.
- **Profile & folders:** `profile` (update, set/delete photo, status), `folders`
  (list, create, add-chat/remove-chat, delete, reorder).
- **Realtime:** `watch [peer]` streams new messages as JSON lines; `wait [peer]` blocks
  until the next message (with `--timeout`).

## Global flags

| Flag | Description |
| --- | --- |
| `-o, --output text\|json` | output format (default `text`) |
| `-a, --account <label>` | select an account, or `all` to fan out across accounts |
| `-c, --config <path>` | config file to use |
| `--proxy <url>` | `socks5://…` or `tg://proxy?…` (MTProxy) |
| `--test` | connect to the Telegram test server (uses built-in test credentials if no config) |

## Multiple accounts

Add named accounts and select them per command:

```console
$ tg accounts add work --app-id … --app-hash …
$ tg login --account work
$ tg accounts                       # list accounts + auth status
$ tg chats list --account work
$ tg watch --account all            # stream every account concurrently, labeled
```

## Trying it without credentials

The Telegram **test server** works out of the box (no `my.telegram.org` needed):

```console
$ tg login --test --phone 9996621234   # test number for DC 2; the code is 22222
$ tg whoami --test
```
