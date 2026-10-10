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
| `lillamolnet status <app>` | Shows an app with its latest build and latest deploy. `<app>` is the subdomain, id or name |
| `lillamolnet builds <app>` | Lists an app's builds, newest first. `--limit` sets how many, at most 100 |
| `lillamolnet build-logs <build>` | Prints a build's log. `<build>` is a build id, from `builds` or `status` |
| `lillamolnet deploy <app> [--wait]` | Builds and deploys an app, like the Deploy button. `--wait` follows it to the end, for at most `--timeout` (default 30m) |
| `lillamolnet logs <app>` | Prints an app's logs, oldest first. `--since` (default 1h, at most 168h), `--limit` (default 200, at most 1000), `--grep` and `--follow` |

Every command takes `--json`.

## Configuration

| Variable | Meaning |
|---|---|
| `LILLAMOLNET_TOKEN` | Your personal API key. Create one at [lillamolnet.se/users/settings/api_keys](https://lillamolnet.se/users/settings/api_keys). |
| `LILLAMOLNET_API_URL` | The API to use. Defaults to `https://lillamolnet.se`. |

A key has scopes. `deploy` needs `read` and `deploy`. `logs` needs `logs:read`,
since logs may contain personal data, and also `read` unless you give the
app's id. `read` is enough for the other commands.

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
    "url": "https://old-shop.lillamolnet.se",
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

`status --json` has the app, in the same format as `apps --json`, its latest
build and its latest deploy. The latest deploy can belong to an older build
than the latest, for example when the latest build failed. Both are `null` for
an app that has never been built.

```json
{
  "app": { "id": "fdf37127-5906-4c4c-9005-4da8cf355adb", "name": "Bakery", "subdomain": "bakery", "...": "..." },
  "latest_build": {
    "id": "f712304f-d4c6-4c1d-be59-40a3cb90920d",
    "status": "failed",
    "branch": "main",
    "commit": "def5678abc",
    "trigger": "manual",
    "error_reason": "mix compile failed",
    "started_at": "2026-10-02T10:00:00Z",
    "finished_at": "2026-10-02T10:01:00Z"
  },
  "latest_deploy": {
    "id": "c1135c59-4ac0-4879-8343-849bdc008eac",
    "status": "deployed",
    "error_reason": null,
    "started_at": "2026-10-01T10:03:10Z",
    "finished_at": "2026-10-01T10:03:40Z",
    "build": { "id": "556cad24-fb5f-4ece-903c-3c534f7126d5", "status": "success", "commit": "abc1234def", "...": "..." }
  }
}
```

`builds --json` is an array of builds, in the same format as `latest_build` in
`status --json`, newest first.

`build-logs --json` is `{"id": "<build id>", "log": "<the log>"}`. Without
`--json`, the log is printed as it is.

The platform keeps at most 50,000 lines of a build's log. A longer log ends with
the line `… log truncated after 50000 lines`. A response over 100 MB is an error
rather than cut off.

`deploy --json` has the app, its URL, the build, its latest deploy, and how it
went. `result` is `queued`, `building` or `deploying` while it goes on, and
`deployed`, `build_failed` or `deploy_failed` when `done` is true. Without
`--wait`, `result` is `queued`.

```json
{
  "app": "bakery",
  "url": "https://bakery.lillamolnet.se",
  "build": { "id": "560b8ba0-8027-449f-b695-d5a4c13077bc", "status": "success", "commit": "def5678abc", "...": "..." },
  "deploy": { "id": "...", "status": "deployed", "...": "..." },
  "done": true,
  "result": "deployed"
}
```

With `--wait`, progress goes to stderr. A short outage of the API, such as while
it is being deployed, does not stop the wait.

`logs --json` prints one JSON object per log line, also with `--follow`:

```json
{"ts":"1791626222878119000","time":"2026-10-10T09:57:02.878119Z","line":"GET / 200","container":"app-...-blue"}
```

`ts` is nanoseconds since the Unix epoch, as text, and orders the lines exactly.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unknown error |
| 2 | Wrong usage, such as an unknown command or flag |
| 3 | No API key, or the key is invalid, expired or revoked |
| 4 | The key lacks the scope the command needs |
| 5 | Not found, such as an unknown app |
| 6 | The API could not be reached |
| 7 | `deploy --wait`: the build failed |
| 8 | `deploy --wait`: the deploy failed |
| 9 | `deploy --wait`: gave up waiting after `--timeout`. The build goes on |
| 10 | `deploy`: a build of the app is already queued or running, so no new one was started |

## Development

```sh
go test ./...
go run ./cmd/lillamolnet --help
```

The tests check the test responses against `api/openapi.json`, a copy of the
API's OpenAPI spec. Update it with `scripts/update-openapi.sh`.

## License

[MIT](LICENSE)
