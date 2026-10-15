package cli

import (
	"slices"
	"testing"

	"github.com/lillamolnet/cli/internal/api"
)

const multiStage = `# syntax=docker/dockerfile:1
ARG NODE_VERSION=24
ARG TOKEN

FROM node:${NODE_VERSION}-alpine AS deps
ARG NODE_VERSION
RUN npm ci \
    --omit=dev
LABEL traefik.enable=true

FROM deps AS builder
RUN <<EOF
EXPOSE 9999
LABEL spwnr.fake=1
EOF
ARG DATABASE_URL

from --platform=linux/amd64 builder as runner
expose 3000/tcp
LABEL "org.opencontainers.image.source"="https://github.com/a/b" version=1
CMD ["node", "server.js"]
`

func TestParseDockerfile(t *testing.T) {
	df := parseDockerfile(multiStage)

	if len(df.args) != 2 || len(df.stages) != 3 {
		t.Fatalf("args = %v, stages = %v", df.args, df.stages)
	}
	if df.stages[2].base != "builder" || df.stages[2].name != "runner" {
		t.Errorf("last stage = %+v", df.stages[2])
	}
	if df.stages[0].instrs[1].args != "npm ci      --omit=dev" {
		t.Errorf("continued line = %q", df.stages[0].instrs[1].args)
	}

	final, image := df.final()
	if image != "node:${NODE_VERSION}-alpine" {
		t.Errorf("image = %q", image)
	}
	// The heredoc's body is not instructions.
	if got := exposedPorts(final); !slices.Equal(got, []int{3000}) {
		t.Errorf("exposedPorts = %v", got)
	}
	if got := command(final); got != "node server.js" {
		t.Errorf("command = %q", got)
	}
	// traefik.enable comes from deps, which the final stage builds on.
	wantLabels := []string{"traefik.enable", "org.opencontainers.image.source", "version"}
	if got := labelKeys(final); !slices.Equal(got, wantLabels) {
		t.Errorf("labelKeys = %v, want %v", got, wantLabels)
	}
	// NODE_VERSION has a global default, and TOKEN has none.
	if got := argsWithoutDefault(df); !slices.Equal(got, []string{"TOKEN", "DATABASE_URL"}) {
		t.Errorf("argsWithoutDefault = %v", got)
	}
}

func TestCommand(t *testing.T) {
	cases := map[string]string{
		"CMD node a.js": "node a.js",
		"ENTRYPOINT [\"/bin/app\"]\nCMD [\"serve\"]": "/bin/app serve",
		"CMD [\"old\"]\nENTRYPOINT [\"/bin/app\"]":   "/bin/app",
		"RUN make": "",
		"CMD [\"sh\", \"-c\", \"exec app --port $PORT\"]": "sh -c exec app --port $PORT",
	}
	for body, want := range cases {
		final, _ := parseDockerfile("FROM scratch\n" + body).final()
		if got := command(final); got != want {
			t.Errorf("%q: command = %q, want %q", body, got, want)
		}
	}
}

func TestCommandPorts(t *testing.T) {
	cases := map[string][]int{
		"gunicorn -b 0.0.0.0:8000 app:app":       {8000},
		"uvicorn main:app --port 80":             {80},
		"uvicorn main:app --port=$PORT":          nil,
		"http-server -p 8080":                    {8080},
		"node server.js":                         nil,
		"rails s -b 0.0.0.0 -p $PORT":            nil,
		"php -S localhost:5000":                  {5000},
		"java -jar app.jar --server.port=$PORT":  nil,
		"python -m http.server --bind [::]:8000": {8000},
	}
	for cmd, want := range cases {
		if got := commandPorts(cmd); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", cmd, got, want)
		}
	}
}

func TestImageName(t *testing.T) {
	cases := map[string]string{
		"nginx":                          "nginx",
		"nginx:1.27-alpine":              "nginx",
		"docker.io/library/nginx:latest": "nginx",
		"nginx@sha256:abc":               "nginx",
		"ghcr.io/a/nginx:1":              "ghcr.io/a/nginx",
		"localhost:5000/app":             "localhost:5000/app",
	}
	for image, want := range cases {
		if got := imageName(image); got != want {
			t.Errorf("imageName(%q) = %q, want %q", image, got, want)
		}
	}
}

func TestPortCheck(t *testing.T) {
	s := defaultSettings
	cases := []struct {
		dockerfile, status string
	}{
		{"FROM node\nEXPOSE 4000\nCMD node a.js", checkOK},
		{"FROM node\nCMD node a.js --port $PORT", checkOK},
		{"FROM node\nENV PORT=3000\nEXPOSE 3000", checkWarning},
		{"FROM node\nEXPOSE 80", checkError},
		{"FROM python\nCMD uvicorn main:app --port 80", checkError},
		{"FROM python\nCMD uvicorn main:app --port 4000", checkOK},
		{"FROM nginx:alpine\nCOPY . /usr/share/nginx/html", checkError},
		{"FROM nginx:alpine\nCOPY default.conf.template /etc/nginx/templates/\nENV PORT=4000", checkOK},
		{"FROM elixir\nCMD [\"/app/bin/server\"]", checkWarning},
	}
	for _, tc := range cases {
		final, image := parseDockerfile(tc.dockerfile).final()
		if got := portCheck(final, image, s); got.Status != tc.status {
			t.Errorf("%q: %s, want %s: %s", tc.dockerfile, got.Status, tc.status, got.Message)
		}
	}

	// Another port for the app.
	final, image := parseDockerfile("FROM node\nEXPOSE 3000").final()
	if got := portCheck(final, image, api.Settings{ContainerPort: 3000}); got.Status != checkOK {
		t.Errorf("port 3000: %+v", got)
	}
}

func TestBuildArgCheck(t *testing.T) {
	df := parseDockerfile("FROM node\nARG DATABASE_URL\nARG TARGETARCH\nARG MODE=prod")
	got := buildArgCheck(df, []string{"DATABASE_URL", "SECRET_KEY_BASE"})
	if got.Status != checkWarning ||
		got.Message != "ARG DATABASE_URL without a default. The platform passes no build args, so they are empty "+
			"in the build. DATABASE_URL is an env variable of the app, but env variables are only set when "+
			"the container starts" {
		t.Errorf("%+v", got)
	}
}

func TestForbiddenLabels(t *testing.T) {
	got := forbiddenLabels([]string{"Traefik.http.routers.x.rule", "spwnr.app_id", "traefikish", "maintainer"})
	if !slices.Equal(got, []string{"Traefik.http.routers.x.rule", "spwnr.app_id"}) {
		t.Errorf("forbiddenLabels = %v", got)
	}
}
