package cli

// requirementsHelp is the help topic lillamolnet help requirements: what the
// platform expects of a repository. The README has the same text, which a
// test checks. lillamolnet check tests a repository against it.
const requirementsHelp = `Repository requirements

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

lillamolnet status <app> shows the app's port, health check and limits.`
