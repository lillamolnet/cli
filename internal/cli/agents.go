package cli

// agentsHelp is the help topic lillamolnet help agents: what a coding agent
// needs to deploy and debug with the CLI, without the README. The README has
// the same text, which a test checks.
const agentsHelp = `For coding agents

Workflow:
  lillamolnet whoami --json            who is logged in, and the key's scopes
  lillamolnet apps --json              the apps. <app> is the subdomain
  lillamolnet status <app> --json      latest build and deploy, with commit
  lillamolnet apps create --yes        an app from the GitHub repo here
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
  apps:write  apps create, together with read
  Exit code 4 means the key lacks one. The user creates a new key with
  lillamolnet login.

Repository:
  lillamolnet help requirements lists what the platform expects of a
  repository: on GitHub, a Dockerfile in the root, an app that listens on
  $PORT, and a health check that passes. check tests the repo you are in
  against it and changes nothing. check --build also builds and runs the
  image with the platform's flags and health check, which needs Docker running
  on this computer. check exits with 11 when a check is an error.

New app:
  An app is made from a GitHub repository. The user first installs the Lilla
  Molnet GitHub app on the repository, in a browser, from
  https://lillamolnet.se/apps/new. Then, in the repository:
    lillamolnet apps create --yes      the platform chooses the servers
    lillamolnet check --build          does it build, start and pass the check
    lillamolnet deploy <app> --wait    the first deploy
  apps create exits with 5 when no GitHub account of the user reaches the
  repository, and with 12 when the subdomain is taken. The error then names a
  free one for --subdomain. It never picks another subdomain by itself.

Good to know:
  - deploy --wait exits 0 when the new version is live, 7 when the build
    failed and 8 when the deploy failed. build-logs shows why.
  - New log lines can take up to a minute to show up, for example right after
    a deploy. No lines is not an error.
  - logs --limit keeps the newest lines, and says on stderr when it cut any.
    --grep is case-sensitive.
  - Without a terminal, apps create, stop, env set and env unset need --yes.
    Give a secret on stdin: printf %s "$VALUE" | lillamolnet env set <app> KEY --yes
  - Env changes take effect at the next deploy.
  - Deploy, stop and env changes affect a live app, and apps create takes a
    subdomain. Ask the user first unless they asked for it.`
