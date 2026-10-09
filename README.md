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
| `lillamolnet whoami` | Shows the user and the API key in use, with its scopes and expiry |
| `lillamolnet apps` | Lists your apps, newest first |

Every command takes `--json`.

## Configuration

| Variable | Meaning |
|---|---|
| `LILLAMOLNET_TOKEN` | Your personal API key. Create one at [lillamolnet.se/users/settings/api_keys](https://lillamolnet.se/users/settings/api_keys). |
| `LILLAMOLNET_API_URL` | The API to use. Defaults to `https://lillamolnet.se`. |

A key has scopes. `read` is enough for `whoami` and `apps`.

## Output

Data goes to stdout, errors go to stderr. With `--json`, stdout is a single JSON
value. The JSON format is a stable contract: fields may be added, but are not
removed or changed. The tables in normal output may change at any time.

The CLI never asks interactive questions when stdin is not a terminal.

### JSON formats

`whoami --json`:

```json
{
  "id": "2e3dd1de-6ec3-4aec-8c17-21a3d96ad63b",
  "email": "ada@example.com",
  "api_key": {
    "name": "laptop",
    "scopes": ["read", "deploy"],
    "expires_at": "2027-01-07T20:10:44Z"
  }
}
```

`apps --json` is an array, newest first. `stopped` is the state you asked for,
not whether the container runs right now. `stopped_by` is `user` or `platform`.

```json
[
  {
    "id": "c4c0ea51-0000-4000-8000-000000000002",
    "name": "Old shop",
    "subdomain": "old-shop",
    "description": null,
    "repo_url": "https://github.com/example/shop",
    "default_branch": null,
    "auto_deploy_enabled": false,
    "auto_deploy_branch": null,
    "stopped": true,
    "stopped_at": "2026-10-05T12:30:00Z",
    "stopped_by": "user",
    "created_at": "2026-09-01T10:00:00Z",
    "updated_at": "2026-10-05T12:30:00Z"
  }
]
```

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
