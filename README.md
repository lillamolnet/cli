# lillamolnet

The command line interface for [Lilla Molnet](https://lillamolnet.se). It works
for people and for coding agents: every command has `--json` output and exit
codes that mean something.

> Early development. The commands below are what exists today.

## Install

```sh
go install github.com/lillamolnet/cli/cmd/lillamolnet@latest
```

Binaries for macOS, Linux and Windows, and Homebrew, come later.

## Commands

| Command | What it does |
|---|---|
| `lillamolnet version` | Prints the version |

Every command takes `--json`.

## Output

Data goes to stdout, errors go to stderr. With `--json`, stdout is a single JSON
value. The JSON format is a stable contract: fields may be added, but are not
removed or changed. The tables in normal output may change at any time.

The CLI never asks interactive questions when stdin is not a terminal.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unknown error |
| 2 | Wrong usage, such as an unknown command or flag |

## Development

```sh
go test ./...
go run ./cmd/lillamolnet --help
```

## License

[MIT](LICENSE)
