package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// checkServer has Bakery, with its env variables, as the API's apps.
func checkServer(t *testing.T) string {
	t.Helper()
	return apiServer(t, map[string]string{
		"/api/v1/apps": "apps_for_status.json",
		"/api/v1/apps/fdf37127-5906-4c4c-9005-4da8cf355adb/env": "env_list.json",
	}, 404, `{"errors":[{"status":"404","code":"not_found","detail":"not found"}]}`)
}

// gitRepo makes a repository with the files and origin as its remote, and
// makes it the current directory.
func gitRepo(t *testing.T, origin string, files map[string]string) {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	if origin != "" {
		git("remote", "add", "origin", origin)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "init")
	t.Chdir(dir)
}

func runCheckJSON(t *testing.T, args ...string) (int, checkJSON, string) {
	t.Helper()
	code, stdout, stderr := runWithAPI(t, checkServer(t), append([]string{"check", "--json"}, args...)...)

	var out checkJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("not JSON: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return code, out, stderr
}

func statuses(out checkJSON) map[string]string {
	got := map[string]string{}
	for _, c := range out.Checks {
		got[c.Name] = c.Status
	}
	return got
}

func TestCheckFindsTheAppFromGit(t *testing.T) {
	gitRepo(t, "git@github.com:Example/bakery.git", map[string]string{
		"Dockerfile": "FROM node:24-alpine\nEXPOSE 4000\nCMD [\"node\", \"server.js\"]\n",
	})

	code, out, _ := runCheckJSON(t)
	if code != exitOK || !out.OK {
		t.Errorf("exit code = %d, out = %+v", code, out)
	}
	if out.App == nil || *out.App != "bakery" {
		t.Errorf("app = %v", out.App)
	}
	if out.Settings.ContainerPort != 4000 || *out.Settings.HealthCheckPath != "/health" {
		t.Errorf("settings = %+v", out.Settings)
	}

	want := map[string]string{
		"app": "ok", "branch": "ok", "pushed": "warning", "dockerfile": "ok", "port": "ok",
		"command": "ok", "labels": "ok", "build_args": "ok",
	}
	if got := statuses(out); !jsonEqual(got, want) {
		t.Errorf("checks = %v\nwant %v", got, want)
	}
}

func TestCheckFails(t *testing.T) {
	cases := map[string]struct {
		files map[string]string
		check string
	}{
		"port 80": {map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 80\n"}, "port"},
		"traefik label": {map[string]string{
			"Dockerfile": "FROM node\nLABEL traefik.http.routers.x.rule=\"Host(`a.se`)\"\nCMD node a.js --port $PORT\n",
		}, "labels"},
		"no Dockerfile": {map[string]string{"docker/Dockerfile": "FROM node\n"}, "dockerfile"},
		"lower case":    {map[string]string{"dockerfile": "FROM node\n"}, "dockerfile"},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gitRepo(t, "https://github.com/example/bakery", tc.files)

			code, out, stderr := runCheckJSON(t)
			if code != exitCheckFailed || out.OK {
				t.Errorf("exit code = %d, ok = %v", code, out.OK)
			}
			if got := statuses(out)[tc.check]; got != checkError {
				t.Errorf("%s = %q: %+v", tc.check, got, out.Checks)
			}
			if !strings.Contains(stderr, "1 check failed") {
				t.Errorf("stderr = %q", stderr)
			}
		})
	}
}

func TestCheckNoDockerfileFindsOthers(t *testing.T) {
	gitRepo(t, "", map[string]string{"docker/Dockerfile": "FROM node\n"})

	_, out, _ := runCheckJSON(t, "bakery")
	for _, c := range out.Checks {
		if c.Name == "dockerfile" && !strings.Contains(c.Message, "Found docker/Dockerfile") {
			t.Errorf("message = %q", c.Message)
		}
	}
}

func TestCheckWithoutAnApp(t *testing.T) {
	gitRepo(t, "https://github.com/someone/else.git", map[string]string{
		"Dockerfile": "FROM node\nCMD node a.js\n",
	})

	code, out, _ := runCheckJSON(t)
	if code != exitOK || out.App != nil {
		t.Errorf("exit code = %d, app = %v", code, out.App)
	}
	if out.Settings.ContainerPort != 4000 || out.Settings.MemoryLimitMB != 512 {
		t.Errorf("settings = %+v", out.Settings)
	}
	if statuses(out)["app"] != checkWarning {
		t.Errorf("checks = %+v", out.Checks)
	}
}

func TestCheckBranch(t *testing.T) {
	gitRepo(t, "https://github.com/example/bakery", map[string]string{"Dockerfile": "FROM node\nEXPOSE 4000\n"})
	if out, err := exec.Command("git", "checkout", "-q", "-b", "feature").CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}

	_, out, _ := runCheckJSON(t)
	for _, c := range out.Checks {
		if c.Name == "branch" && (c.Status != checkWarning || !strings.Contains(c.Message, "on feature, but the platform builds main")) {
			t.Errorf("branch = %+v", c)
		}
	}
}

func TestCheckText(t *testing.T) {
	gitRepo(t, "https://github.com/example/bakery", map[string]string{"Dockerfile": "FROM nginx\nEXPOSE 80\n"})

	code, stdout, _ := runWithAPI(t, checkServer(t), "check")
	if code != exitCheckFailed {
		t.Errorf("exit code = %d", code)
	}
	for _, want := range []string{"error    port", "1 error, 2 warnings."} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout has no %q:\n%s", want, stdout)
		}
	}
}

func TestCheckUsage(t *testing.T) {
	for _, args := range [][]string{{"check", "a", "b"}, {"check", "--env-file", "x"}} {
		code, _, _ := runWithAPI(t, checkServer(t), args...)
		if code != exitUsage {
			t.Errorf("%v: exit code = %d", args, code)
		}
	}
}

func TestSameRepo(t *testing.T) {
	same := [][2]string{
		{"https://github.com/a/b", "git@github.com:a/b.git"},
		{"https://github.com/A/B/", "https://github.com/a/b.git"},
		{"https://github.com/a/b", "ssh://git@github.com/a/b.git"},
		{"https://github.com/a/b", "https://user:token@github.com/a/b"},
	}
	for _, pair := range same {
		if !sameRepo(pair[0], pair[1]) {
			t.Errorf("%s and %s should be the same", pair[0], pair[1])
		}
	}
	if sameRepo("https://github.com/a/b", "https://github.com/a/bc") || sameRepo("", "") {
		t.Error("different repositories are the same")
	}
}

func TestRunArgs(t *testing.T) {
	got := strings.Join(runArgs("c", "img", defaultSettings, ".env"), " ")
	want := "run -d --name c -p 127.0.0.1::4000 --memory=512m --memory-swap=512m " +
		"--cpus=1 --pids-limit=256 --security-opt no-new-privileges --cap-drop=ALL --cap-add=CHOWN " +
		"--cap-add=SETUID --cap-add=SETGID --cap-add=NET_BIND_SERVICE --env-file .env -e PORT=4000 img"
	if got != want {
		t.Errorf("runArgs =\n%s\nwant\n%s", got, want)
	}
}

func TestCheckWithoutRemote(t *testing.T) {
	gitRepo(t, "", map[string]string{"Dockerfile": "FROM node\nEXPOSE 4000\n"})

	_, out, _ := runCheckJSON(t, "bakery")
	for _, c := range out.Checks {
		if c.Name == "pushed" && (c.Status != checkWarning || !strings.HasPrefix(c.Message, "no git remote origin")) {
			t.Errorf("pushed = %+v", c)
		}
	}
}

func TestConnectPortAndHealth(t *testing.T) {
	exposed := checkItem{"port", checkWarning, "EXPOSE 3000, but the platform sends traffic to port 4000"}
	healthy := checkItem{"health_check", checkOK, "answered 200 on /"}
	silent := checkItem{"health_check", checkError, "no answer that passes within 60 s"}

	got := connectPortAndHealth([]checkItem{exposed, healthy}, defaultSettings)
	if got[0].Status != checkOK || !strings.Contains(got[0].Message, "answers on $PORT (4000)") {
		t.Errorf("an answer should settle the port: %+v", got[0])
	}

	got = connectPortAndHealth([]checkItem{exposed, silent}, defaultSettings)
	if got[0].Status != checkWarning || !strings.Contains(got[1].Message, "likely cause is what port found: EXPOSE 3000") {
		t.Errorf("no answer should point to the port: %+v", got)
	}

	// A tcp check on another port says nothing about the app's port.
	tcp := defaultSettings
	tcp.HealthCheckType, tcp.HealthCheckPort = "tcp", 5027
	got = connectPortAndHealth([]checkItem{exposed, healthy}, tcp)
	if got[0].Status != checkWarning {
		t.Errorf("port = %+v", got[0])
	}

	// Without --build there is nothing to connect.
	got = connectPortAndHealth([]checkItem{exposed}, defaultSettings)
	if got[0] != exposed {
		t.Errorf("port = %+v", got[0])
	}
}
