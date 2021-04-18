# cli

The command line interface for telegram.

```console
$ go install github.com/gotd/cli/cmd/tg@latest
```

## Usage

First, initialize configuration (currently only for bots)

```console
$ tg init --app-id APP_ID --app-hash APP_HASH --token BOT_TOKEN
```

This will create config in `gotd` subdirectory of default config directory, for example `~/.config/gotd/gotd.cli.yaml`.

Now you can issue commands to control your bot.
For example, you can send `Hello world` to `@gotd_test`:
```bash
$ tg send --peer gotd_test "Hello world"
```
