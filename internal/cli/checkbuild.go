package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lillamolnet/cli/internal/api"
)

// The platform's health check: the time a new version has to answer, and how
// long it must then keep running before it answers again.
const (
	healthTimeout = 60 * time.Second
	healthSettle  = 10 * time.Second
)

// capAdd are the capabilities the platform gives a container back after
// --cap-drop=ALL.
var capAdd = []string{"CHOWN", "SETUID", "SETGID", "NET_BIND_SERVICE"}

type buildOptions struct {
	root     string
	settings api.Settings
	envKeys  []string
	envFile  string
	// progress gets what check is doing, since a build takes a while.
	progress io.Writer
}

// buildChecks builds the image and runs it as the platform does, and removes
// both afterwards, also when interrupted.
func buildChecks(parent context.Context, o buildOptions) []checkItem {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Removing the container and the image must work after an interrupt.
	cleanup := context.WithoutCancel(parent)

	if out, err := docker(ctx, "", "version", "--format", "{{.Server.Version}}"); err != nil {
		return []checkItem{{"docker", checkError, "Docker is needed for --build, and does not answer: " + lastLines(out, 3)}}
	}

	tag := "lillamolnet-check-" + randomHex()
	fmt.Fprintln(o.progress, "Building the image with docker build . ...")
	out, err := docker(ctx, o.root, "build", "-t", tag, ".")
	defer docker(cleanup, "", "image", "rm", "-f", tag)
	if err != nil {
		return []checkItem{{"build", checkError, "docker build . failed:\n" + lastLines(out, 20)}}
	}
	checks := []checkItem{{"build", checkOK, "docker build . succeeded"}}

	checks = append(checks, imageLabelCheck(ctx, tag))
	checks = append(checks, runCheck(ctx, cleanup, tag, o)...)
	return checks
}

func imageLabelCheck(ctx context.Context, tag string) checkItem {
	out, err := docker(ctx, "", "image", "inspect", "-f", "{{json .Config.Labels}}", tag)
	if err != nil {
		return checkItem{"image_labels", checkError, "docker image inspect failed: " + lastLines(out, 3)}
	}

	var labels map[string]string
	if err := json.Unmarshal([]byte(out), &labels); err != nil {
		return checkItem{"image_labels", checkError, "docker image inspect: " + err.Error()}
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	if forbidden := forbiddenLabels(keys); len(forbidden) > 0 {
		return checkItem{"image_labels", checkError, fmt.Sprintf("the image has the labels %s, from the "+
			"Dockerfile or its base image. The platform rejects an image with labels that start with traefik. or spwnr.",
			strings.Join(forbidden, ", "))}
	}
	return checkItem{"image_labels", checkOK, "the image has no labels that the platform controls"}
}

// runArgs are the platform's flags for docker run: PORT, the limits without
// swap, the capabilities and the restart policy, kept the same as the
// platform's. Locally, the port is published on 127.0.0.1 instead of on the
// platform's network.
func runArgs(name, tag string, s api.Settings, envFile string) []string {
	memory := strconv.Itoa(s.MemoryLimitMB) + "m"
	args := []string{"run", "-d", "--restart", "unless-stopped", "--name", name,
		"-p", "127.0.0.1::" + strconv.Itoa(probePort(s)),
		"--memory=" + memory, "--memory-swap=" + memory,
		"--cpus=" + strconv.FormatFloat(s.CPULimit, 'f', -1, 64),
		"--pids-limit=" + strconv.Itoa(s.PidsLimit),
		"--security-opt", "no-new-privileges", "--cap-drop=ALL"}
	for _, c := range capAdd {
		args = append(args, "--cap-add="+c)
	}
	if envFile != "" {
		args = append(args, "--env-file", envFile)
	}
	return append(args, "-e", "PORT="+strconv.Itoa(s.ContainerPort), tag)
}

// probePort is the port the health check connects to, as on the platform.
func probePort(s api.Settings) int {
	if s.HealthCheckType == "tcp" && s.HealthCheckPort != 0 {
		return s.HealthCheckPort
	}
	return s.ContainerPort
}

func runCheck(ctx, cleanup context.Context, tag string, o buildOptions) []checkItem {
	s := o.settings
	name := tag

	fmt.Fprintln(o.progress, "Starting the container with the platform's PORT, capabilities and limits ...")
	if out, err := docker(ctx, "", runArgs(name, tag, s, o.envFile)...); err != nil {
		return []checkItem{{"run", checkError, "docker run failed: " + lastLines(out, 5)}}
	}
	defer docker(cleanup, "", "rm", "-f", name)

	out, err := docker(ctx, "", "port", name, strconv.Itoa(probePort(s))+"/tcp")
	if err != nil {
		return []checkItem{{"run", checkError, "docker port failed: " + lastLines(out, 3)}}
	}
	// One line per address, such as 127.0.0.1:55012.
	address, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	runItem := checkItem{"run", checkOK, "the container started with PORT=" + strconv.Itoa(s.ContainerPort)}

	target := "TCP port " + strconv.Itoa(probePort(s))
	if s.HealthCheckType != "tcp" {
		target = "/"
		if s.HealthCheckPath != "" {
			target = s.HealthCheckPath
		}
	}
	fmt.Fprintf(o.progress, "Waiting for an answer on %s (up to %d s), then checking that it keeps running for %d s ...\n",
		target, int(healthTimeout.Seconds()), int(healthSettle.Seconds()))

	health := healthCheck(ctx, name, address, s)
	if health.Status != checkOK {
		health.Message += failureHints(ctx, name, o)
	}
	return []checkItem{runItem, health}
}

// healthCheck does what the platform's does: wait for the first answer while
// the container runs without restarts, wait healthSettle more, and ask again.
func healthCheck(ctx context.Context, name, address string, s api.Settings) checkItem {
	fail := func(msg string) checkItem { return checkItem{"health_check", checkError, msg} }
	probe := prober(address, s)

	deadline := time.Now().Add(healthTimeout)
	var last string
	for {
		if st := containerState(ctx, name); !st.running() {
			return fail("the container stopped before it answered: " + st.String())
		}
		ok, answer := probe()
		last = answer
		if ok {
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Sprintf("no answer that passes within %d s. The last was: %s",
				int(healthTimeout.Seconds()), last))
		}
		if !pause(ctx, time.Second) {
			return fail("interrupted")
		}
	}
	first := last

	for i := 1; i <= int(healthSettle.Seconds()); i++ {
		if !pause(ctx, time.Second) {
			return fail("interrupted")
		}
		if st := containerState(ctx, name); !st.running() {
			return fail(fmt.Sprintf("the container answered, but stopped %d s later: %s", i, st))
		}
	}

	for range 3 {
		if ok, _ := probe(); ok {
			return checkItem{"health_check", checkOK, fmt.Sprintf("answered %s, and kept running for %d s",
				first, int(healthSettle.Seconds()))}
		}
		pause(ctx, time.Second)
	}
	return fail(fmt.Sprintf("the container answered at first, but not after %d s", int(healthSettle.Seconds())))
}

// prober returns a function that asks the app once, and says whether the
// answer passes and what it was.
func prober(address string, s api.Settings) func() (bool, string) {
	if s.HealthCheckType == "tcp" {
		return func() (bool, string) { return probeTCP(address) }
	}

	path, strict := "/", false
	if s.HealthCheckPath != "" {
		path, strict = s.HealthCheckPath, true
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		// A redirect is an answer of its own, as with curl on the platform.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}

	return func() (bool, string) {
		req, err := http.NewRequest(http.MethodGet, "http://"+address+path, nil)
		if err != nil {
			return false, err.Error()
		}
		req.Header.Set("X-Forwarded-Proto", "https")
		resp, err := client.Do(req)
		if err != nil {
			return false, "no answer"
		}
		resp.Body.Close()

		answer := fmt.Sprintf("%d on %s", resp.StatusCode, path)
		switch {
		case resp.StatusCode < 300:
			return true, answer
		case resp.StatusCode < 500 && !strict:
			return true, answer
		case resp.StatusCode < 400:
			return false, answer + ", a redirect, which does not pass on a health check path of its own"
		default:
			return false, answer
		}
	}
}

// probeTCP connects to the port. Docker's port forwarding accepts a
// connection also when nothing listens in the container, and closes it right
// away, so a connection that is closed at once does not count.
func probeTCP(address string) (bool, string) {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return false, "connection refused"
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, err = conn.Read(make([]byte, 1))
	var netErr net.Error
	if err == nil || (errors.As(err, &netErr) && netErr.Timeout()) {
		return true, "a TCP connection"
	}
	return false, "the connection was closed at once"
}

type containerStatus struct {
	known     bool
	isRunning bool
	restarts  int
	oomKilled bool
	exitCode  int
}

func (s containerStatus) running() bool { return s.known && s.isRunning && s.restarts == 0 }

func (s containerStatus) String() string {
	switch {
	case !s.known:
		return "the container is gone"
	case s.oomKilled:
		return "killed for using more memory than the limit"
	case s.restarts > 0:
		return "it stopped and was restarted"
	default:
		return fmt.Sprintf("it exited with code %d", s.exitCode)
	}
}

func containerState(ctx context.Context, name string) containerStatus {
	out, err := docker(ctx, "", "inspect", "-f",
		"{{.State.Running}} {{.RestartCount}} {{.State.OOMKilled}} {{.State.ExitCode}}", name)
	fields := strings.Fields(out)
	if err != nil || len(fields) != 4 {
		return containerStatus{}
	}
	restarts, _ := strconv.Atoi(fields[1])
	exitCode, _ := strconv.Atoi(fields[3])
	return containerStatus{known: true, isRunning: fields[0] == "true", restarts: restarts,
		oomKilled: fields[2] == "true", exitCode: exitCode}
}

// failureHints are the container's last log lines, and the env variables the
// app has on the platform but not here.
func failureHints(ctx context.Context, name string, o buildOptions) string {
	var hints string
	if o.envFile == "" && len(o.envKeys) > 0 {
		hints += fmt.Sprintf("\nThe app has env variables on the platform that are not set here: %s. "+
			"If it needs them to start, give them with --env-file.", strings.Join(o.envKeys, ", "))
	}
	if logs, err := docker(ctx, "", "logs", "--tail", "20", name); err == nil && strings.TrimSpace(logs) != "" {
		hints += "\nThe container's last log lines:\n" + strings.TrimRight(logs, "\n")
	}
	return hints
}

// docker runs docker in dir and returns stdout and stderr together.
func docker(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if err != nil && out.Len() == 0 {
		out.WriteString(err.Error())
	}
	return out.String(), err
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func pause(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

func randomHex() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
