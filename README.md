# lillamolnet

The command line interface for [Lilla Molnet](https://lillamolnet.se). It works
for people and for coding agents: every command has `--json` output and exit
codes that mean something.

> Early development. The commands below are what exists today.

## Install

On macOS, with [Homebrew](https://brew.sh):

```sh
brew install --cask lillamolnet/tap/lillamolnet
```

This installs `lillamolnet` and the short alias `lm`.

On Linux and Windows, download the archive for your system from
[the releases](https://github.com/lillamolnet/cli/releases), unpack it and put
`lillamolnet` (or `lillamolnet.exe`) on your `PATH`. `checksums.txt` has the
SHA-256 of every archive.

With Go 1.25 or newer:

```sh
go install github.com/lillamolnet/cli/cmd/lillamolnet@latest
```

The binaries are not signed yet. The Homebrew cask removes the macOS quarantine
flag, so that macOS runs the binary. If you download the macOS archive by hand,
run `xattr -d com.apple.quarantine lillamolnet` once.

## Commands

`<app>` is the app's subdomain, as the first column of `lillamolnet apps` shows
it: `my-blog` for `https://my-blog.lillamolnet.se`. Its id or name works too.
`lillamolnet <command> --help` shows examples.

| Command | What it does |
|---|---|
| `lillamolnet version` | Prints the version. `lillamolnet --version` does the same |
| `lillamolnet login` | Logs in through the browser and saves an API key |
| `lillamolnet logout` | Revokes the saved key and removes it |
| `lillamolnet whoami` | Shows the user and the API key in use, with its scopes and expiry |
| `lillamolnet apps` | Lists your apps, newest first |
| `lillamolnet status <app>` | Shows an app with its port, health check and limits, and its latest build and latest deploy. `<app>` is the subdomain, id or name |
| `lillamolnet check [<app>]` | Checks that the repository in the current directory can be deployed, without deploying it. Without `<app>`, the app is found from the git remote. `--build` also builds and runs the image with Docker, and `--env-file` gives it env variables |
| `lillamolnet builds <app>` | Lists an app's builds, newest first. `--limit` sets how many, at most 100 |
| `lillamolnet build-logs <build>` | Prints a build's log. `<build>` is a build id, from `builds` or `status` |
| `lillamolnet deploy <app> [--wait]` | Builds and deploys an app, like the Deploy button. `--wait` follows it to the end, for at most `--timeout` (default 30m) |
| `lillamolnet logs <app>` | Prints an app's logs, oldest first. `--since` (default 1h, at most 168h), `--limit` (default 200, at most 1000), `--grep` and `--follow`. `--limit` keeps the newest lines, and `--grep` is case-sensitive and applied before it |
| `lillamolnet env <app>` | Lists an app's environment variables by key. The values are never shown |
| `lillamolnet env set <app> KEY=VALUE` | Sets a variable, or rotates a secret one. The value can also come on stdin. Takes effect at the next deploy |
| `lillamolnet env unset <app> KEY` | Removes a variable. Takes effect at the next deploy |
| `lillamolnet stop <app>` | Stops an app without removing it |
| `lillamolnet start <app>` | Starts a stopped app |

Every command takes `--json`.

## Logging in

```sh
lillamolnet login
```

shows a code and opens the browser. Approve the code on lillamolnet.se, and
choose what the key may do and how long it is valid. The key is saved in
`~/.config/lillamolnet/credentials` (or `$XDG_CONFIG_HOME`), readable only by
you. `lillamolnet logout` revokes it and removes it.

A coding agent on the same computer uses the saved key, and never needs to see
it. On another computer, run `lillamolnet login` there and approve the code from
any device. The key is never read from the environment, so that it stays out of
shells, logs and transcripts.

## Configuration

| Variable | Meaning |
|---|---|
| `LILLAMOLNET_API_URL` | The API to use. Defaults to `https://lillamolnet.se`. A saved key is only sent to the API it was saved for. |

A key has scopes. `deploy`, `stop` and `start` need `read` and `deploy`. `logs` needs `logs:read`,
since logs may contain personal data, and also `read` unless you give the
app's id. `env set` and `env unset` need `read` and `env:write`. `read` is enough for
the other commands.

## Output

Data goes to stdout, errors go to stderr. With `--json`, stdout is a single JSON
value. The JSON format is a stable contract: fields may be added, but are not
removed or changed. The tables in normal output may change at any time.

When a command fails with `--json`, stdout holds an error object, and the
error is also printed as text on stderr:

```json
{"error": {"code": "not_found", "message": "no app \"my-blg\". Run 'lillamolnet apps' to list your apps", "exit_code": 5}}
```

`code` names the exit code: `error`, `usage`, `auth`, `forbidden`, `not_found`,
`unreachable`, `build_failed`, `deploy_failed`, `timeout`, `already_building` or
`check_failed`. `http_status` is there when the API refused the call, and
`required_scopes` when the key lacks a scope. `deploy --wait` prints its result
instead when the build or the deploy fails, or the wait times out, and `check`
when a check fails, since the result already says how it went.

`logs` prints notes on stderr, never on stdout: when `--limit` cut older lines,
and when there were no lines at all. New lines can take up to a minute to show
up, for example right after a deploy.

The CLI never asks interactive questions when stdin is not a terminal.
`env set`, `env unset` and `stop` ask before changing anything when stdin is a
terminal, and need `--yes` when it is not. Give a secret on stdin rather than as an
argument, so that it does not end up in the shell's history:

```sh
printf %s "$DATABASE_URL" | lillamolnet env set myapp DATABASE_URL --yes
```

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

The last seven fields are how the platform runs the app. `container_port` is
the port the app must listen on, and the value of `PORT`. `health_check_type`
is `http` or `tcp`. An `http` check asks for `health_check_path`, or `/` when
it is `null`, and a `tcp` check connects to `health_check_port`, or to
`container_port` when it is `null`. The limits are set by the platform. The
seven are `null` from a version of the API that does not have them.

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
    "updated_at": "2026-10-05T12:30:00Z",
    "container_port": 4000,
    "health_check_type": "tcp",
    "health_check_path": null,
    "health_check_port": 5000,
    "memory_limit_mb": 1024,
    "cpu_limit": 0.5,
    "pids_limit": 256
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

`check --json` has the app's subdomain, or `null` when no app deploys the
repository, the settings it was checked against, in the same format as in
`apps --json`, and the checks. `status` is `ok`, `warning` or `error`, and `ok`
is `false` when a check is an error. Then check exits with 11. The names of
the checks are `app`, `git`, `branch`, `pushed`, `dockerfile`, `port`,
`command`, `labels` and `build_args`, and with `--build` also `docker`,
`build`, `image_labels`, `run` and `health_check`. A check that does not apply
is left out.

```json
{
  "app": "bakery",
  "settings": { "container_port": 4000, "health_check_type": "http", "health_check_path": "/health", "...": "..." },
  "ok": false,
  "checks": [
    { "name": "dockerfile", "status": "ok", "message": "Dockerfile in the repository root" },
    { "name": "port", "status": "error", "message": "EXPOSE 80, but the platform sends traffic to port 4000. ..." }
  ]
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

`env --json` is an array of `{"key", "scope", "secret", "last_rotated_at",
"created_at", "updated_at"}`, never with a value. `env set --json` and
`env unset --json` give `{"app", "key", "result", "scope", "secret",
"takes_effect"}`, where `result` is `created`, `updated`, `rotated` or `removed`
and `takes_effect` is `next_deploy`.

`stop --json` and `start --json` give `{"app": ..., "result": ...}`, with the app
in the same format as `apps --json`. `result` is `stopped`, `started`,
`redeploying` (the container was gone, so the same build is deployed again) or
`not_deployed`.

## Exit codes

`lillamolnet help exit-codes` prints the same list.

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unknown error |
| 2 | Wrong usage, such as an unknown command or flag, or a malformed id |
| 3 | No API key, or the key is invalid, expired or revoked |
| 4 | The key lacks the scope the command needs |
| 5 | Not found, such as an unknown app or build |
| 6 | The API could not be reached |
| 7 | `deploy --wait`: the build failed |
| 8 | `deploy --wait`: the deploy failed |
| 9 | `deploy --wait`: gave up waiting after `--timeout`. The build goes on |
| 10 | `deploy`: a build of the app is already queued or running, so no new one was started |
| 11 | `check`: a check failed |

## Repository requirements

`lillamolnet help requirements` prints this. `lillamolnet check` tests the
repository you are in against it, and `lillamolnet check --build` also builds and
runs the image with Docker, as the platform does.

```text
What the platform expects of a repository. lillamolnet check tests the
repository you are in against it, without deploying.

Source:
  - The repository is on GitHub, and the app is connected to it. The
    platform builds from GitHub, never from your computer, so push before
    you deploy.
  - lillamolnet deploy and the Deploy button build the repository's default
    branch. With auto deploy, a push to the app's auto deploy branch builds
    that branch. lillamolnet apps --json shows both.

Build:
  - A Dockerfile in the root of the repository. The root is the build
    context. No other path or directory is supported.
  - The platform runs docker build . without build args. Env variables are
    only set when the container starts, so a build that needs one, such as
    DATABASE_URL, fails.
  - No labels that start with traefik. or spwnr. The image is rejected.

Run:
  - Listen on $PORT, on all interfaces (0.0.0.0). PORT is the app's port,
    4000 unless another one is chosen. An image that listens on port 80 does
    not work without a change.
  - The container runs with --cap-drop=ALL and only CHOWN, SETUID, SETGID
    and NET_BIND_SERVICE added back, and with no-new-privileges.
  - Limits: 512 MB memory without swap, 1 CPU and 256 processes, unless the
    platform has set others for the app.

Health check:
  - Within 60 s, an answer below 500 on /, or 2xx on the app's health check
    path when it has one. The request has X-Forwarded-Proto: https, so a
    redirect to https does not pass on a path of its own.
  - With a tcp health check, it is enough that the port accepts a
    connection.
  - Then the container must keep running for 10 s without a restart, and
    answer again.

lillamolnet status <app> shows the app's port, health check and limits.
```

## For coding agents

`lillamolnet help agents` prints this, so an agent finds it without the README.

```text
Workflow:
  lillamolnet whoami --json            who is logged in, and the key's scopes
  lillamolnet apps --json              the apps. <app> is the subdomain
  lillamolnet status <app> --json      latest build and deploy, with commit
  lillamolnet check --json             whether the repo here can be deployed
  lillamolnet deploy <app> --wait      build and deploy, and wait for the result
  lillamolnet build-logs <build>       the log of a build, for a failed one
  lillamolnet logs <app> --since 15m   the app's own logs

Logging in:
  If a command exits with 3, run lillamolnet login and ask the user to approve
  the code in a browser. The key is saved on this computer. It is never read
  from the environment, and you never need to see it.

Output:
  Use --json to parse output. stdout is then one JSON value, or one per line
  for logs. When a command fails, stdout holds
  {"error": {"code", "message", "exit_code"}} and stderr the same as text.
  The exit code says what happened: lillamolnet help exit-codes.

Scopes:
  read        apps, status, check, builds, build-logs, env (names only)
  deploy      deploy, stop and start, together with read
  logs:read   logs. Also read, unless <app> is the app's id
  env:write   env set and env unset, together with read
  Exit code 4 means the key lacks one. The user creates a new key with
  lillamolnet login.

Repository:
  lillamolnet help requirements lists what the platform expects of a
  repository: on GitHub, a Dockerfile in the root, an app that listens on
  $PORT, and a health check that passes. check tests the repo you are in
  against it and changes nothing. check --build also builds and runs the
  image with the platform's flags and health check, which needs Docker running
  on this computer. check exits with 11 when a check is an error.

Good to know:
  - deploy --wait exits 0 when the new version is live, 7 when the build
    failed and 8 when the deploy failed. build-logs shows why.
  - New log lines can take up to a minute to show up, for example right after
    a deploy. No lines is not an error.
  - logs --limit keeps the newest lines, and says on stderr when it cut any.
    --grep is case-sensitive.
  - Without a terminal, stop, env set and env unset need --yes. Give a secret
    on stdin: printf %s "$VALUE" | lillamolnet env set <app> KEY --yes
  - Env changes take effect at the next deploy.
  - Deploy, stop and env changes affect a live app. Ask the user first unless
    they asked for it.
```

## Development

```sh
go test ./...
go run ./cmd/lillamolnet --help
```

The tests check the test responses against `api/openapi.json`, a copy of the
API's OpenAPI spec. Update it with `scripts/update-openapi.sh`.

## License

[MIT](LICENSE)
