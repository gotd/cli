# `tg` Roadmap — Agent-Automatable Telegram CLI

## Vision

Turn `tg` into a single static binary that an AI agent (or any script) can drive to
automate everyday Telegram work on a **personal account**: message yourself, list and
triage chats, mark chats as read, reply, upload/download files, search history, and more.

The goal is a broad capability surface — and strong agent ergonomics — delivered as a
fast, dependency-free Go binary built on [`gotd/td`](https://github.com/gotd/td), exposed
as composable subcommands. New capability should land as `tg` subcommands.

## The central gap: user-account auth

`tg` today authenticates **only as a bot** (`Config.BotToken`, `client.Auth().Bot(...)`,
and `app.Before` hard-rejects a config without a bot token). Bots cannot:

- access **Saved Messages** / "message yourself",
- list **dialogs** (`messages.getDialogs` is unavailable to bots),
- read arbitrary chat history or mark dialogs as read the way a user can,
- see the chat/folder list, contacts, or most account state.

**Every automation use case in the request requires a user session.** So user login is
Phase 0 and gates everything else. `gotd/td` already supports this: `auth.NewFlow` with a
code-prompt + 2FA handler, and `qrlogin` for QR-based login.

## Agent-first design principles

These cut across every feature and matter as much as the features themselves:

1. **Structured output.** Global `--output json|text` (default `text` for humans, but
   agents pass `--output json`). Every command emits a stable, documented JSON object on
   stdout. No data on stderr; logs/progress go to stderr only.
2. **Non-interactive by default.** Never block on a TTY prompt in agent mode. Auth codes,
   confirmations, and 2FA come from flags / env / a pre-established session.
3. **Stable exit codes.** `0` success, distinct non-zero codes for auth failure, peer not
   found, flood-wait exceeded, permission denied — so agents can branch on them.
4. **No decorative output in machine mode.** The upload progress bar and typing-action
   updates must be suppressed under `--output json`.
5. **Consistent peer resolution.** One shared `--peer` grammar everywhere: `me`/`self`,
   `@username`, numeric ID, phone, `t.me/...` link. Cache resolved access-hashes in the
   session dir (StringSession-style sessions have no entity cache — we must keep our own).
6. **Idempotency & safety.** Destructive actions (delete history, leave, ban, delete
   account-level state) require `--yes` in agent mode and are clearly marked read-only vs
   mutating.
7. **Schema versioning.** Include `"schema": 1` (or similar) in JSON so agents can adapt.
8. **Account selection.** Every command accepts a global `--account <label>` (env
   `TG_ACCOUNT`), defaulting to the configured default account. Reserve and thread this
   flag from day one even while only one account exists, so adding multi-account later is
   additive and never changes existing command signatures.

## Phased plan

### Phase 0 — Foundations (prerequisite for everything)

- [x] **Adopt [`spf13/cobra`](https://github.com/spf13/cobra) for the CLI** (replacing the
      current `urfave/cli/v2`). Do this first — every later command is a cobra subcommand, so
      migrating after the surface grows is costly. Requirements:
  - **Rich documentation**: every command sets `Short`, a long-form `Long`, and runnable
    `Example` blocks; group related commands (`cobra.Group`) so `tg --help` reads as a map of
    the surface. Generate `man` pages and Markdown docs from the command tree
    (`spf13/cobra/doc`) and publish them, so help stays in sync with the code.
  - **Thorough autocomplete**: ship `tg completion bash|zsh|fish|powershell` (cobra's built-in
    `completion` command). Register `ValidArgsFunction` for dynamic completion of the things
    agents and humans actually type — `--peer` (cached dialogs/usernames from the peer cache),
    `--account` labels, `--output` values, enum flags — and mark file-taking flags
    (`upload`/`download`) with `MarkFlagFilename`. Completions must work without a network
    round-trip where possible (read from the local session/peer cache).
  - Use `cobra.Command.RunE` (error-returning) throughout; wire global flags as persistent
    flags on the root; pair with `spf13/pflag` for GNU-style `--flag`/`-f` parsing.
- [x] **QR login (primary)**: `tg login` defaults to QR, following
      `gotd/td`'s `examples/qrauth.go` + `examples/userbot`. Wiring:
  - `dispatcher := tg.NewUpdateDispatcher()` and
    `loggedIn := qrlogin.OnLoginToken(&dispatcher)` — the channel fires on
    `tg.UpdateLoginToken` when the code is scanned.
  - `client.QR().Auth(ctx, loggedIn, show)`, where `show` renders the QR to stderr via
    `github.com/mdp/qrterminal/v3` **and** also prints `token.URL()` (the
    `tg://login?token=...` link) so it can be opened/rendered elsewhere — agent-friendly.
  - 2FA fallback: on `SESSION_PASSWORD_NEEDED`, call `client.Auth().Password(ctx, pwd)`
    (retry on `auth.ErrPasswordInvalid`).
  - **Requires updates enabled.** Today `app.go` sets `NoUpdates: true` and registers no
    `UpdateHandler`, so `loggedIn` would never be notified. The login path must register
    the dispatcher as `UpdateHandler` and not disable updates; make `NoUpdates` and the
    dispatcher conditional on the command.
  - UX: scan once (Settings → Devices → Link Desktop Device); the session persists in
    `session.FileStorage`, so all later agent invocations are headless.
- [x] **Phone login (fallback)**: `tg login --phone` using `auth.NewFlow` (code + 2FA), for
      environments where scanning a QR is inconvenient.
- [x] Persist the user session alongside the existing bot session. Extend `Config`/`init`
      to support a user session path; relax `app.Before` so a bot token is no longer
      mandatory.
- [x] **Auth mode selection**: allow a single config to hold both; commands pick user vs
      bot session (most new commands are user-only).
- [x] **`--output json` plumbing**: a small result-writer used by all commands; move logs
      and progress to stderr.
- [x] **Peer resolver + access-hash cache** in the session directory.
- [x] **Proxy support (global)**: a single `--proxy` flag (config + `TG_PROXY` env)
      accepting a URL, threaded into `telegram.Options.Resolver`:
  - `socks5://`, `socks4://`, `http(s)://` → `dcs.Plain` with a
    `golang.org/x/net/proxy` dialer (already in the module graph via `x/net`; no new dep).
      Honor user:password and remote-DNS.
  - `tg://proxy?server=&port=&secret=` / MTProxy → native `dcs.MTProxy(addr, secret, …)`,
    reusing the link + secret (hex/base64url) parsing from
    `gotd/td`'s `examples/mtproxy-connect`.
  - Wire it at client construction so every command benefits; per-account overrides come in
    Phase 7.
- [x] **`tg whoami`**: smallest end-to-end user-session command to validate auth.

### Phase 1 — Core agent loop (the explicit asks)

These directly cover "upload files to myself, list chats, mark chats as read, replies".

- [x] `tg chats list` — list dialogs (paged), with unread counts, pinned/muted/archived
      flags, last message preview.
- [x] `tg messages list <peer>` / `tg history <peer>` — read recent messages.
- [x] `tg send <peer> <text>` — already exists; extend to default `me` and JSON output.
- [x] `tg reply <peer> <message-id> <text>` — reply to a specific message.
- [x] `tg read <peer>` — mark a chat as read.
- [x] `tg upload` to `me` / Saved Messages — already exists; ensure `me` peer + JSON +
      silent progress.
- [x] `tg download <peer> <message-id> [--out path]` — download media.

### Phase 2 — Messaging depth

- [x] `edit`, `delete` (single + bulk), `delete-history`.
- [x] `forward` (single + multiple).
- [x] `pin` / `unpin` / `unpin-all` / `pinned`.
- [x] Reactions: `react` / `unreact` / `reactions`.
- [x] Drafts: `draft set` / `drafts` / `draft clear`.
- [x] Scheduled messages: `schedule` send / list / delete.
- [x] Search: `search <peer> <query>` and `search --global <query>`.
- [x] Polls: `poll create`.
- [x] Message context / links: `context`, `link`.
- [x] Albums, voice, stickers, GIFs. *(album + `upload --type voice|sticker|gif` + `stickers`
      list; GIF search deferred — inline-bot flow, low value.)*

### Phase 3 — Chats & contacts

- [x] `chat get` / `chat full`.
- [x] `mute` / `unmute` / `archive` / `unarchive`.
- [x] `resolve <username>`, `search-public <query>`, `subscribe`.
- [x] Contacts: list / search / add / delete / block / unblock / blocked / import / export.

### Phase 4 — Groups & channels admin

- [x] Create group/channel, invite, leave, participants, admins, banned.
- [x] Admin/moderation: promote/demote, ban/unban, permissions, slow mode, admin rights, recent actions.
- [x] Edit chat title/photo/about, invite links (export/import/join by link).
- [x] Forum topics.

### Phase 5 — Profile & folders

- [x] Profile: get me, update profile, profile photo set/delete, privacy get/set, user info/photos/status.
      *(get-me=`whoami`, user-info=`chat full`; privacy get/set deferred — heavy
      privacy-key/rule modeling, low agent value.)*
- [x] Folders / dialog filters: list / get / create / add-chat / remove-chat / delete / reorder.

### Phase 6 — Realtime (high value for agents)

- [x] `tg watch <peer>` — stream new messages as JSON lines; the agent's input loop.
- [x] `tg wait` — block until a new (or settled) incoming message, with timeout.
- [ ] Backed by `gotd/td` update handlers; `NoUpdates: true` (set in `app.go` today) must
      become conditional.

### Phase 7 — Multiple concurrent accounts

The `--account` selector (design principle 8) ships from Phase 0, but real multi-account —
several sessions usable, and **live simultaneously** — lands here: a `label -> client` map,
an `--account <label>` selector, and a `tg accounts` listing.

- [ ] **Config for N accounts**: named accounts in the config (or env
      `TG_SESSION_<LABEL>`), each with its own `session.FileStorage` file, peer-cache, and
      `floodwait.Waiter`. Keep a backward-compatible single "default" account.
- [ ] **`tg accounts`** — list configured accounts + auth status.
- [ ] **`tg login --account <label>`** — mint/refresh a session per label (QR or phone).
- [ ] **Concurrent runtime**: hold a `map[label]*telegram.Client`, each running under its
      own waiter; a command resolves `--account` to one client. Clients are independent and
      safe to run in parallel — one flood-wait or reconnect must not stall the others.
- [ ] **Fan-out where it makes sense**: `--account all` (or repeated `--account`) for
      read/broadcast commands (e.g. `chats list`, `send`, `watch`) — results keyed by label
      in JSON. Strictly opt-in; single-account stays the default.
- [ ] **Realtime across accounts**: Phase 6 `watch`/`wait` can observe multiple accounts
      concurrently (one dispatcher + update loop per client), merged into one JSON stream.
- [ ] **Per-account proxy**: per-label override of the global proxy (Phase 0), so each
      account can route through its own SOCKS5/HTTP/MTProxy.

## Capability groups (reference)

The surface breaks into groups: `accounts`, `chats`, `messages`, `media`, `contacts`,
`groups`, `profile`, `folders`, `events`. The phases above fold each group in: Phase 1
covers the daily-driver subset of `chats`/`messages`/`media`; Phase 2 the rest of
`messages`+`media`; Phase 3 `chats`+`contacts`; Phase 4 `groups`; Phase 5
`profile`+`folders`; Phase 6 `events`; Phase 7 generalizes the whole surface to multiple
concurrent accounts (the `accounts` group).

## Technical notes

- **CLI framework:** `spf13/cobra` (+ `spf13/pflag`), replacing `urfave/cli/v2`. Chosen for
  first-class shell completion (`bash`/`zsh`/`fish`/`powershell` with dynamic
  `ValidArgsFunction` hooks) and doc generation (`spf13/cobra/doc` → man + Markdown). One
  `*cobra.Command` per subcommand with `RunE`; global flags are persistent flags on the root;
  the existing `app`/`Before` wiring moves into `PersistentPreRunE`.
- **Library:** `gotd/td` (`telegram`, `tg`, `telegram/message`, `telegram/uploader`,
  `telegram/downloader`, `telegram/query` for dialog/history pagination, `auth`,
  `auth/qrlogin`, `telegram/peers` for resolution/caching).
- **QR login** adds one dependency: `github.com/mdp/qrterminal/v3` (terminal QR rendering),
  matching `gotd/td`'s `examples/qrauth.go`. The QR flow is also a forcing function to
  introduce an update dispatcher (`tg.NewUpdateDispatcher`) and turn off the current
  `NoUpdates: true` for commands that need it — which Phase 6 (realtime) needs anyway.
- **Flood-wait** is already handled via the `floodwait.Waiter` middleware — keep it; add a
  distinct exit code when retries are exhausted.
- **Proxy** is configured through `telegram.Options.Resolver` (`dcs.Resolver`), not a global
  dialer: `dcs.Plain{Dial: <x/net/proxy dialer>.DialContext}` for SOCKS/HTTP, or
  `dcs.MTProxy(addr, secret, …)` for MTProxy. See `gotd/td`'s `telegram/dcs/example_test.go`
  and `examples/mtproxy-connect`. Each account builds its own resolver, so per-account
  proxies fall out naturally in Phase 7.
- **Sessions:** keep using `session.FileStorage` (per-account file); the user session lives
  beside the existing bot session. Consider an export-to-string command for portability.
- **Multi-account isolation (Phase 7):** one `telegram.Client` per label, each with its own
  session file, peer-cache, and `floodwait.Waiter` — no shared mutable state between
  accounts, so they run concurrently without one blocking another. The runtime owns a
  `map[label]*telegram.Client`; commands select by `--account`. Single-account remains the
  default and costs nothing.
- **Pagination:** dialog and history listing must page (`page`/`page_size`); use
  `query.GetDialogs`/`query.Messages` iterators.
- **Output stability:** define Go structs for each command's JSON result; never leak raw
  MTProto types into the contract.

## Open questions

1. **Login UX for agents:** interactive `tg login` once to mint a session, then agents reuse
   it headless — or support a fully headless code-delivery path? (Recommend: interactive
   one-time login, headless reuse.)
2. **Keep bot support?** Recommend yes — some flows (channel posting) are fine as a bot — but
   make user the default for the agent commands.
3. **Scope of v1:** propose shipping Phases 0–1 as the first milestone, since they cover the
   exact asks (message self, list chats, mark read, reply, upload).
4. **Multi-account timing:** ship single-account first but reserve `--account` from Phase 0,
   so full concurrent multi-account (Phase 7) is purely additive. Should fan-out
   (`--account all`) be in scope, or only explicit single-label selection?
</content>
