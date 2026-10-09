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

## Configuration

| Variable | Meaning |
|---|---|
| `LILLAMOLNET_TOKEN` | Your personal API key. Create one at [lillamolnet.se/users/settings/api_keys](https://lillamolnet.se/users/settings/api_keys). |
| `LILLAMOLNET_API_URL` | The API to use. Defaults to `https://lillamolnet.se`. |

A key has scopes. `read` is enough for the commands that exist today.

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
| 3 | No API key, or the key is invalid, expired or revoked |
| 4 | The key lacks the scope the command needs |
| 5 | Not found |
| 6 | The API could not be reached |

## Development

```sh
go test ./...
go run ./cmd/lillamolnet --help
```

The tests check the test responses against `api/openapi.json`, a copy of the
API's OpenAPI spec. Update it with `scripts/update-openapi.sh`.

## License

[MIT](LICENSE)
