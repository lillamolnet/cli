package cli

import (
	"os"
	"strings"
	"testing"
)

func TestAgentsHelpTopic(t *testing.T) {
	_, help, _ := run(t, "--help")
	if !strings.Contains(help, "lillamolnet help agents") {
		t.Errorf("--help does not point to the topic:\n%s", help)
	}

	code, stdout, _ := run(t, "help", "agents")
	if code != exitOK || !strings.Contains(stdout, "lillamolnet deploy <app> --wait") {
		t.Errorf("exit code = %d, stdout:\n%s", code, stdout)
	}
}

// The README has the same text as the topic, so that they never drift apart.
func TestReadmeHasTheAgentsHelp(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}

	body := strings.TrimPrefix(agentsHelp, "For coding agents\n\n")
	if !strings.Contains(string(readme), "```text\n"+body+"\n```") {
		t.Error("the README's section For coding agents differs from agentsHelp in agents.go")
	}
}

func TestRequirementsHelpTopic(t *testing.T) {
	_, help, _ := run(t, "--help")
	if !strings.Contains(help, "lillamolnet help requirements") {
		t.Errorf("--help does not point to the topic:\n%s", help)
	}

	code, stdout, _ := run(t, "help", "requirements")
	if code != exitOK || !strings.Contains(stdout, "A Dockerfile in the root of the repository") {
		t.Errorf("exit code = %d, stdout:\n%s", code, stdout)
	}
	if !strings.Contains(agentsHelp, "lillamolnet help requirements") {
		t.Error("help agents does not point to help requirements")
	}
}

// The README has the same text as the topic.
func TestReadmeHasTheRequirements(t *testing.T) {
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}

	body := strings.TrimPrefix(requirementsHelp, "Repository requirements\n\n")
	if !strings.Contains(string(readme), "```text\n"+body+"\n```") {
		t.Error("the README's section Repository requirements differs from requirementsHelp in requirements.go")
	}
}
