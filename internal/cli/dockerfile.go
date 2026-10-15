package cli

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// dockerfile is a Dockerfile read well enough for lillamolnet check: the
// instructions of each stage, with line continuations joined and comments
// and heredoc bodies left out. It is not a full parser.
type dockerfile struct {
	// args are the ARG instructions before the first FROM.
	args   []instruction
	stages []stage
}

type instruction struct {
	// cmd is the instruction in upper case, such as EXPOSE.
	cmd  string
	args string
}

type stage struct {
	// base is the image or stage in FROM, and name what AS names the stage,
	// in lower case.
	base, name string
	instrs     []instruction
}

var heredocStart = regexp.MustCompile(`<<(-?)["']?([A-Za-z_][A-Za-z0-9_]*)["']?`)

func parseDockerfile(text string) dockerfile {
	var df dockerfile
	var pending string
	// heredoc is the word that ends a heredoc being skipped.
	var heredoc string
	var heredocTrim bool

	add := func(line string) {
		cmd, args, _ := strings.Cut(strings.TrimSpace(line), " ")
		in := instruction{cmd: strings.ToUpper(cmd), args: strings.TrimSpace(args)}

		if m := heredocStart.FindStringSubmatch(in.args); m != nil {
			heredoc, heredocTrim = m[2], m[1] == "-"
		}

		switch {
		case in.cmd == "FROM":
			df.stages = append(df.stages, parseFrom(in.args))
		case len(df.stages) == 0:
			if in.cmd == "ARG" {
				df.args = append(df.args, in)
			}
		default:
			last := &df.stages[len(df.stages)-1]
			last.instrs = append(last.instrs, in)
		}
	}

	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if heredoc != "" {
			end := line
			if heredocTrim {
				end = strings.TrimLeft(line, "\t")
			}
			if end == heredoc {
				heredoc = ""
			}
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if body, ok := strings.CutSuffix(strings.TrimRight(line, " \t"), `\`); ok {
			pending += body + " "
			continue
		}
		add(pending + line)
		pending = ""
	}
	if strings.TrimSpace(pending) != "" {
		add(pending)
	}
	return df
}

func parseFrom(args string) stage {
	var fields []string
	for _, f := range strings.Fields(args) {
		if !strings.HasPrefix(f, "--") {
			fields = append(fields, f)
		}
	}

	var s stage
	if len(fields) > 0 {
		s.base = fields[0]
	}
	if len(fields) >= 3 && strings.EqualFold(fields[1], "AS") {
		s.name = strings.ToLower(fields[2])
	}
	return s
}

// final returns the instructions that end up in the image: those of the last
// stage and of the stages it is built FROM, oldest first. image is the
// external image at the bottom, as FROM writes it.
func (df dockerfile) final() (instrs []instruction, image string) {
	if len(df.stages) == 0 {
		return nil, ""
	}

	byName := map[string]stage{}
	current := df.stages[len(df.stages)-1]
	for _, s := range df.stages[:len(df.stages)-1] {
		if s.name != "" {
			byName[s.name] = s
		}
	}

	for {
		instrs = append(append([]instruction{}, current.instrs...), instrs...)
		parent, ok := byName[strings.ToLower(current.base)]
		if !ok {
			return instrs, current.base
		}
		// A stage can only build on an earlier one, so this ends.
		delete(byName, parent.name)
		current = parent
	}
}

// all returns every instruction after the first FROM.
func (df dockerfile) all() []instruction {
	var all []instruction
	for _, s := range df.stages {
		all = append(all, s.instrs...)
	}
	return all
}

// command is the CMD and ENTRYPOINT of the final image as text, the last of
// each, or "" when the Dockerfile has neither.
func command(instrs []instruction) string {
	var cmd, entrypoint string
	for _, in := range instrs {
		switch in.cmd {
		case "CMD":
			cmd = execForm(in.args)
		case "ENTRYPOINT":
			entrypoint = execForm(in.args)
			// An ENTRYPOINT resets the CMD of the base.
			cmd = ""
		}
	}
	return strings.TrimSpace(entrypoint + " " + cmd)
}

// execForm turns ["node", "server.js"] into node server.js. The shell form is
// kept as it is.
func execForm(args string) string {
	var parts []string
	if strings.HasPrefix(args, "[") && json.Unmarshal([]byte(args), &parts) == nil {
		return strings.Join(parts, " ")
	}
	return args
}

// exposedPorts are the numbers in the image's EXPOSE instructions. Ports
// that are variables are left out.
func exposedPorts(instrs []instruction) []int {
	var ports []int
	for _, in := range instrs {
		if in.cmd != "EXPOSE" {
			continue
		}
		for _, f := range strings.Fields(in.args) {
			number, _, _ := strings.Cut(f, "/")
			if port, err := strconv.Atoi(number); err == nil {
				ports = append(ports, port)
			}
		}
	}
	return ports
}

var portVariable = regexp.MustCompile(`\$\{?PORT\b|^PORT\b`)

// usesPort reports whether an instruction reads or sets PORT, as $PORT in a
// command or ENV PORT.
func usesPort(instrs []instruction) bool {
	for _, in := range instrs {
		switch in.cmd {
		case "ENV", "ARG":
			if portVariable.MatchString(in.args) {
				return true
			}
		default:
			if strings.Contains(in.args, "$PORT") || strings.Contains(in.args, "${PORT") {
				return true
			}
		}
	}
	return false
}

// commandPort finds a port given in a command, such as --port 8080, -p 3000,
// --bind 0.0.0.0:80 or localhost:5000. -p must be followed by a number,
// since it means other things too.
var commandPort = regexp.MustCompile(
	`(?:--port[= ]\s*|(?:^|\s)-p\s+|(?:0\.0\.0\.0|127\.0\.0\.1|localhost|\[::\]|\*):)(\d{2,5})\b`)

func commandPorts(cmd string) []int {
	var ports []int
	for _, m := range commandPort.FindAllStringSubmatch(cmd, -1) {
		if port, err := strconv.Atoi(m[1]); err == nil {
			ports = append(ports, port)
		}
	}
	return ports
}

var loopback = regexp.MustCompile(`\b(127\.0\.0\.1|localhost)\b`)

// labelKey finds the keys in LABEL key=value "other key"=value.
var labelKey = regexp.MustCompile(`(?:^|\s)(?:"([^"]+)"|([^\s="]+))=`)

func labelKeys(instrs []instruction) []string {
	var keys []string
	for _, in := range instrs {
		if in.cmd != "LABEL" {
			continue
		}
		matches := labelKey.FindAllStringSubmatch(in.args, -1)
		if len(matches) == 0 {
			// The old form, LABEL key value.
			if fields := strings.Fields(in.args); len(fields) > 0 {
				keys = append(keys, strings.Trim(fields[0], `"`))
			}
			continue
		}
		for _, m := range matches {
			keys = append(keys, m[1]+m[2])
		}
	}
	return keys
}

// forbiddenLabels are the keys the platform rejects an image for. Traefik
// reads label keys without regard to case.
func forbiddenLabels(keys []string) []string {
	var forbidden []string
	for _, key := range keys {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "traefik.") || strings.HasPrefix(lower, "spwnr.") {
			forbidden = append(forbidden, key)
		}
	}
	return forbidden
}

// predefinedArgs are set by Docker, so they have a value without a default.
var predefinedArgs = map[string]bool{
	"TARGETPLATFORM": true, "TARGETOS": true, "TARGETARCH": true, "TARGETVARIANT": true,
	"BUILDPLATFORM": true, "BUILDOS": true, "BUILDARCH": true, "BUILDVARIANT": true,
	"HTTP_PROXY": true, "HTTPS_PROXY": true, "FTP_PROXY": true, "NO_PROXY": true, "ALL_PROXY": true,
	"http_proxy": true, "https_proxy": true, "ftp_proxy": true, "no_proxy": true, "all_proxy": true,
}

// argsWithoutDefault are the build args that are empty in the platform's
// build, which passes none. An ARG without a default in a stage can still get
// its value from a global ARG with the same name.
func argsWithoutDefault(df dockerfile) []string {
	global := map[string]bool{}
	for _, in := range df.args {
		if name, _, ok := strings.Cut(in.args, "="); ok {
			global[strings.TrimSpace(name)] = true
		}
	}

	seen := map[string]bool{}
	var names []string
	for _, in := range append(append([]instruction{}, df.args...), df.all()...) {
		if in.cmd != "ARG" {
			continue
		}
		for _, f := range strings.Fields(in.args) {
			if strings.Contains(f, "=") || predefinedArgs[f] || global[f] || seen[f] {
				continue
			}
			seen[f] = true
			names = append(names, f)
		}
	}
	return names
}

// defaultPorts are what common web server images listen on without
// configuration.
var defaultPorts = map[string]int{"nginx": 80, "httpd": 80, "caddy": 80}

// imageName is nginx for nginx:1.27-alpine or docker.io/library/nginx.
func imageName(image string) string {
	image, _, _ = strings.Cut(image, "@")
	if i := strings.LastIndex(image, ":"); i > strings.LastIndex(image, "/") {
		image = image[:i]
	}
	image = strings.TrimPrefix(image, "docker.io/")
	return strings.ToLower(strings.TrimPrefix(image, "library/"))
}
