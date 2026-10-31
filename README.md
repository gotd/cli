# tg — Telegram CLI for humans and agents

`tg` is a single static Go binary for driving a Telegram **personal account** (or a
bot) from the command line or an AI agent: message yourself, list and triage chats,
read history, send/forward/edit/delete messages, upload and download files, manage
groups and channels, and stream new messages in real time.

Built on [`gotd/td`](https://github.com/gotd/td).

> **This is the ApexAiOfficial fork of [`gotd/cli`](https://github.com/gotd/cli).**
> It stays command- and output-compatible with upstream and adds what an agent
> needs to test Telegram bots end to end: pressing inline and keyboard buttons,
> collecting a bot's replies and edits in one call, deep-link starts, bot
> introspection, richer message JSON (buttons, entities, file details), stable
> exit codes with JSON errors, a global `--timeout`, and session and cache files
> that concurrent `tg` processes share safely. See [Testing bots](#testing-bots).

## Installation

Download a prebuilt binary or package (`.deb`/`.rpm`/`.apk`) for your platform from
the [latest upstream release](https://github.com/gotd/cli/releases/latest).

To build this fork from source and install it as `~/.local/bin/tg`:

```console
$ scripts/install-local.sh                       # reuses the app credentials of the installed tg
$ APP_ID=… APP_HASH=… scripts/install-local.sh   # or embed your own
$ scripts/install-local.sh --creds-from /path/to/old/tg
```

Sessions are tied to the app credentials they were created with, so replacing a
release binary with a source build should embed the same credentials:
`--creds-from` reads them from an existing binary's build info.

## Quick start

```console
$ tg init                                          # write config (release binaries embed app credentials)
$ tg login                                         # QR login — scan once from a logged-in device
$ tg whoami                                         # confirm you're authenticated
$ tg send "Hello world"                             # message yourself (Saved Messages)
$ tg send --peer @gotd_test "Hello world"           # message a peer
```

App credentials are required. The prebuilt [release](https://github.com/gotd/cli/releases/latest)
binaries embed them at build time and present the session as a desktop client, so
`tg init` just works. If you build from source, provide your own from
<https://my.telegram.org>: `tg init --app-id APP_ID --app-hash APP_HASH`.

The config is written to the `gotd` subdirectory of your config dir, e.g.
`~/.config/gotd/gotd.cli.yaml`. The session persists too, so subsequent commands
run headless. On macOS the session is stored in the login Keychain by default; an
existing file session is migrated into the Keychain automatically on first use.
Set `keychain: false` in the config to keep it in a file alongside the config
instead (useful for headless macOS). Other platforms always use a file.

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
- **Stable exit codes and JSON errors** (see [Exit codes](#exit-codes)), and a global
  `--timeout` so no call can hang an agent.
- **Secrets off the command line:** `send`, `reply` and `edit` take `--stdin`.
- **Shell completion:** `tg completion bash|zsh|fish|powershell`, with dynamic completion
  for peers, accounts, output format and enum flags.

```console
$ tg chats list --output json | jq '.data.chats[].peer.username'
$ tg history @durov --limit 20 -o json
```

### Agent skill

This repo ships an installable [Claude Code](https://claude.com/claude-code) skill
(`skills/tg`) that teaches an agent how to drive Telegram with `tg` — setup checks,
the JSON/peers conventions, the `--yes` safety gate, exit codes, common recipes,
and a bot-testing playbook. Install it
with [`skills`](https://www.npmjs.com/package/skills):

```console
$ npx skills add https://github.com/ApexAiOfficial/tg-cli --all
# or a single skill:
$ npx skills add https://github.com/ApexAiOfficial/tg-cli --skill tg
```

## What you can do

Run `tg --help` for the full, grouped command list, or `tg <command> --help` for details.

- **Messaging:** `send`, `reply`, `edit`, `delete`, `delete-history`, `forward`,
  `history`, `search` (`--global`), `read`, `pin`/`unpin`/`unpin-all`/`pinned`,
  `react`/`unreact`/`reactions`, `draft`/`drafts`, `schedule`, `poll`, `link`, `context`.
- **Diagnostics:** `doctor` checks config, credentials, session, peer cache, connection
  and login, with a fix hint for each failure (`--offline` skips the network).
- **Bots:** `click` (press inline/keyboard buttons), `start-bot` (deep-link start),
  `bot-info`, `webapp` (open a Mini App and get its signed launch data), and `--wait` reply collection on
  `send`/`reply`/`click`/`start-bot`/`upload`.
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

## Testing bots

`tg` acts as a real user, so an agent (or a test script) can drive any bot the way
a person would and check what actually happens:

```console
$ tg bot-info @mybot                                  # descriptions, command menu, menu button
$ tg webapp @mybot --out session.json                 # open its Mini App: real signed launch data, owner-only file
$ tg send --peer @mybot /start --wait 30s -o json     # send, then collect replies until the bot settles
$ tg start-bot @mybot ref_42 --wait 20s               # deep link: t.me/mybot?start=ref_42
$ tg click @mybot --text "Settings" --wait 15s -o json  # press an inline button, get the answer + edits
$ tg click @mybot 4242 --data "page:2"                # by callback data, on a given message
$ tg reply @mybot 4250 "Berlin" --wait 20s            # answer a prompt
$ tg upload --peer @mybot ./sample.pdf --wait 30s     # media handling
$ tg history @mybot --after 4242 -o json              # everything newer than a message
```

`--wait <dur>` (on `send`, `reply`, `click`, `start-bot`, `upload`) subscribes to
updates *before* acting, so a fast reply is never missed. It stops when the bot has
been quiet for `--settle` (default 2s, extended while the bot shows "typing…"),
when `--replies N` messages arrived, or when `--wait` elapses (`timed_out: true`).

Telegram does not record which message a bot is answering, so the result separates
what is proven from what is observed:

- `replies`: the bot's messages ordered after your action, in their latest version
  (bots that stream by editing are handled). Very likely the response, but a bot
  that also sends unprompted messages, or a second conversation with the same bot
  from this account, can add messages here.
- `edits`: other messages the bot edited, e.g. the one whose button was pressed.
- `linked`: ids Telegram itself ties to your action (a `reply_to` your message, an
  edit of the pressed message). Deterministic.
- `earlier`: messages the bot sent before your action (ids are ordered per chat),
  such as a late answer to a previous command. Never a response to this action.

`replies` is always present (possibly `[]`); `edits`, `linked`, `earlier` and `timed_out`
are omitted when empty or false, like every optional field in tg's JSON, so read them
with a default (`jq '.data.linked // []'`).

Message JSON includes `reply_markup` (inline buttons or reply keyboard, with each
button's `type`, `text`, callback `data`, `url`), `entities` (formatting and links,
with the covered `text`), `file` (`kind`, `name`, `mime`, `size`, `duration`,
dimensions), `edit_date` and `via_bot_id`. New fields are omitted when empty, so
existing consumers see the same shape.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | other failure |
| 2 | usage: unknown command, bad flags or arguments |
| 3 | auth: not logged in or session revoked (`tg login`) |
| 4 | not found: peer, message or button |
| 5 | flood wait: rate limited beyond the retry budget |
| 6 | timeout: `--timeout`, or `wait --timeout` |
| 7 | other Telegram API error |
| 130, 143 | interrupted by SIGINT / SIGTERM (128 + signal) |

Errors are printed to stderr as `tg: <message>`. With `--output json` a failure
also writes an envelope to stdout, so a parser always gets JSON:

```json
{"schema": 1, "error": {"code": "flood_wait", "exit": 5, "message": "…", "retry_after": 42,
  "rpc": {"code": 420, "type": "FLOOD_WAIT", "argument": 42}}}
```

## Global flags

| Flag | Description |
| --- | --- |
| `-o, --output text\|json` | output format (default `text`) |
| `-a, --account <label>` | select an account, or `all` to fan out across accounts |
| `-c, --config <path>` | config file to use (env `TG_CONFIG`) |
| `--proxy <url>` | `socks5://…` or `tg://proxy?…` (MTProxy) |
| `--timeout <dur>` | abort the command after this long, exit code 6 (env `TG_TIMEOUT`) |
| `--test` | connect to the Telegram test server (persisted by `tg init --test`) |

## Multiple accounts

Add named accounts and select them per command:

```console
$ tg accounts add work --app-id … --app-hash …
$ tg login --account work
$ tg accounts                       # list accounts + auth status
$ tg chats list --account work
$ tg watch --account all            # stream every account concurrently, labeled
```

## Using the test server

Initialize a config against the Telegram **test server**, then log in with a test
number. Test mode only switches the DC list — it uses the same app credentials as
production (so the same app-id/app-hash requirement applies):

```console
$ tg init --test                # persists test: true
$ tg login --phone 9996621234   # test number for DC 2; the code is 22222
$ tg whoami
```
