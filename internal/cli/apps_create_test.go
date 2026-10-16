package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// createServer answers GET /repositories with reposFixture and POST /apps
// with createStatus and createFixture, and keeps what was posted.
type createServer struct {
	url   string
	mu    sync.Mutex
	posts []map[string]any
}

func newCreateServer(t *testing.T, reposFixture string, createStatus int, createFixture string) *createServer {
	t.Helper()
	s := &createServer{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/repositories":
			_, _ = w.Write(absFixture(t, reposFixture))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps":
			body, _ := io.ReadAll(r.Body)
			var parsed map[string]any
			_ = json.Unmarshal(body, &parsed)
			s.mu.Lock()
			s.posts = append(s.posts, parsed["data"].(map[string]any)["attributes"].(map[string]any))
			s.mu.Unlock()

			w.WriteHeader(createStatus)
			_, _ = w.Write(absFixture(t, createFixture))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s
}

// absFixture reads a fixture by its absolute path, since TestAppsCreateFromGitRemote
// changes the working directory.
func absFixture(t *testing.T, name string) []byte {
	data, err := os.ReadFile(filepath.Join(fixtures, name))
	if err != nil {
		t.Error(err)
	}
	return data
}

func (s *createServer) sent(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.posts
}

func TestAppsCreateWithRepo(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	code, stdout, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "Example/bakery", "--yes")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	sent := s.sent(t)
	if len(sent) != 1 {
		t.Fatalf("posts = %v", sent)
	}
	attrs := sent[0]
	if attrs["repo_id"] != 731.0 || attrs["integration_id"] != "4b9f4d2e-2b8c-4c3e-9a51-0a7d1f6c2e11" ||
		attrs["name"] != "bakery" || attrs["subdomain"] != "bakery" {
		t.Errorf("attributes = %v", attrs)
	}
	if _, ok := attrs["auto_deploy_enabled"]; ok {
		t.Errorf("auto deploy was turned on: %v", attrs)
	}
	for _, want := range []string{"Created bakery from example/Bakery", "lillamolnet check --build", "lillamolnet deploy bakery --wait"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q does not say %q", stdout, want)
		}
	}
}

func TestAppsCreateJSON(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	code, stdout, _ := runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery", "--yes", "--json")

	var app appJSON
	if code != exitOK || json.Unmarshal([]byte(stdout), &app) != nil {
		t.Fatalf("exit code = %d, stdout %q", code, stdout)
	}
	if app.Subdomain != "bakery" || app.RepoURL != "https://github.com/example/Bakery.git" {
		t.Errorf("app = %+v", app)
	}
}

func TestAppsCreateFlags(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/site",
		"--name", "My site", "--subdomain", "my-site", "--description", "The site", "--auto-deploy", "--yes")

	if code != exitOK {
		t.Fatalf("exit code = %d, stderr %q", code, stderr)
	}
	attrs := s.sent(t)[0]
	if attrs["repo_id"] != 732.0 || attrs["name"] != "My site" || attrs["subdomain"] != "my-site" ||
		attrs["description"] != "The site" || attrs["auto_deploy_enabled"] != true || attrs["auto_deploy_branch"] != "trunk" {
		t.Errorf("attributes = %v", attrs)
	}

	code, _, _ = runWithAPI(t, s.url, "apps", "create", "--repo", "example/site", "--auto-deploy=release", "--yes")
	if code != exitOK || s.sent(t)[1]["auto_deploy_branch"] != "release" {
		t.Errorf("exit code = %d, attributes = %v", code, s.sent(t)[1])
	}
}

func TestAppsCreateFromGitRemote(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:example/Bakery.git"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	t.Chdir(dir)

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--yes")

	if code != exitOK || len(s.sent(t)) != 1 || s.sent(t)[0]["repo_id"] != 731.0 {
		t.Errorf("exit code = %d, posts %v, stderr %q", code, s.sent(t), stderr)
	}
}

func TestAppsCreateNeedsYesWithoutTerminal(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery")

	if code != exitUsage || !strings.Contains(stderr, "--yes") || len(s.sent(t)) != 0 {
		t.Errorf("exit code = %d, stderr %q, posts %v", code, stderr, s.sent(t))
	}
}

func TestAppsCreateRepoNotReached(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/other", "--yes")

	if code != exitNotFound || !strings.Contains(stderr, "https://lillamolnet.se/apps/new") || len(s.sent(t)) != 0 {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}
}

func TestAppsCreateNotReachedByTheAPI(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 422, "app_create_unreachable.json")

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery", "--yes")

	if code != exitNotFound || !strings.Contains(stderr, "https://lillamolnet.se/apps/new") {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}
}

func TestAppsCreateTwoAccounts(t *testing.T) {
	s := newCreateServer(t, "repositories_two_accounts.json", 201, "app_created.json")

	code, _, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery", "--yes")
	if code != exitUsage || !strings.Contains(stderr, "--integration") ||
		!strings.Contains(stderr, "9d0c1a77-5e3f-4f0b-8c2d-6b1e2f3a4c55") || len(s.sent(t)) != 0 {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}

	code, _, _ = runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery",
		"--integration", "9d0c1a77-5e3f-4f0b-8c2d-6b1e2f3a4c55", "--yes")
	if code != exitOK || s.sent(t)[0]["integration_id"] != "9d0c1a77-5e3f-4f0b-8c2d-6b1e2f3a4c55" {
		t.Errorf("exit code = %d, posts %v", code, s.sent(t))
	}
}

func TestAppsCreateSubdomainTaken(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 409, "app_create_conflict.json")

	code, stdout, stderr := runWithAPI(t, s.url, "apps", "create", "--repo", "example/Bakery", "--yes", "--json")

	if code != exitConflict || !strings.Contains(stderr, "--subdomain bakery-2") {
		t.Errorf("exit code = %d, stderr %q", code, stderr)
	}
	var out errorJSON
	if err := json.Unmarshal([]byte(stdout), &out); err != nil || out.Error.Code != "conflict" || out.Error.ExitCode != 12 {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestAppsCreateUsage(t *testing.T) {
	s := newCreateServer(t, "repositories.json", 201, "app_created.json")

	for _, args := range [][]string{
		{"apps", "create", "--repo", "not-a-repo", "--yes"},
		{"apps", "create", "--repo", "a/b/c", "--yes"},
		{"apps", "create", "--repo", "example/ab", "--yes"},
		{"apps", "create", "extra", "--yes"},
	} {
		code, _, stderr := runWithAPI(t, s.url, args...)
		if code != exitUsage {
			t.Errorf("%v: exit code = %d, stderr %q", args, code, stderr)
		}
	}
	if len(s.sent(t)) != 0 {
		t.Errorf("posts = %v", s.sent(t))
	}
}

func TestAppsCreateAPIErrors(t *testing.T) {
	testAPIErrors(t, "apps", "create", "--repo", "example/Bakery", "--yes")
}

func TestGithubFullName(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/ada/blog.git":            "ada/blog",
		"https://github.com/ada/blog":                "ada/blog",
		"https://user:token@github.com/ada/blog.git": "ada/blog",
		"git@github.com:ada/blog.git":                "ada/blog",
		"ssh://git@github.com/ada/my.site.git":       "ada/my.site",
		"https://GitHub.com/Ada/Blog/":               "Ada/Blog",
		"https://gitlab.com/ada/blog.git":            "",
		"git@gitlab.com:ada/blog.git":                "",
		"https://github.com/ada":                     "",
		"/tmp/blog":                                  "",
	} {
		got, ok := githubFullName(remote)
		if got != want || ok != (want != "") {
			t.Errorf("githubFullName(%q) = %q, %v, want %q", remote, got, ok, want)
		}
	}
}

func TestSubdomainFrom(t *testing.T) {
	for name, want := range map[string]string{
		"blog":                         "blog",
		"My_Blog.site":                 "my-blog-site",
		"--weird--name--":              "weird-name",
		"ab":                           "",
		"åäö":                          "",
		strings.Repeat("a", 70):        strings.Repeat("a", 63),
		strings.Repeat("a", 62) + "-b": strings.Repeat("a", 62),
	} {
		if got := subdomainFrom(name); got != want {
			t.Errorf("subdomainFrom(%q) = %q, want %q", name, got, want)
		}
	}
}
