package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// dispatchedCommands pins the set of command names the CLI dispatches or
// documents. Adding a command requires a registry entry (and vice versa).
var dispatchedCommands = []string{
	"editor play", "editor stop", "editor pause", "editor resume", "editor refresh",
	"console", "exec", "menu", "screenshot", "reserialize", "test",
	"profiler hierarchy", "profiler enable", "profiler disable", "profiler status", "profiler clear",
	"list", "status", "update", "version", "commands", "help",
}

func TestRegistryCoversAllDispatchedCommands(t *testing.T) {
	names := map[string]bool{}
	for _, n := range registryNames() {
		names[n] = true
	}
	for _, want := range dispatchedCommands {
		if !names[want] {
			t.Errorf("registry missing dispatched command %q", want)
		}
	}
	if len(commandRegistry) != len(dispatchedCommands) {
		t.Errorf("registry has %d entries, dispatched list has %d — keep them in sync", len(commandRegistry), len(dispatchedCommands))
	}
}

func TestRegistryEntriesAreComplete(t *testing.T) {
	validGroups := map[string]bool{}
	for _, g := range groupOrder {
		validGroups[g] = true
	}
	for _, c := range commandRegistry {
		if c.Name == "" || c.Group == "" || c.Summary == "" {
			t.Errorf("registry entry %+v missing Name/Group/Summary", c)
		}
		if !validGroups[c.Group] {
			t.Errorf("command %q uses group %q not in groupOrder — it would not render in help", c.Name, c.Group)
		}
		if c.ConnectorTool != "" && !c.RequiresUnity {
			t.Errorf("command %q maps to connector tool %q but is not marked RequiresUnity", c.Name, c.ConnectorTool)
		}
	}
}

func TestRegistryConnectorToolWhitelist(t *testing.T) {
	valid := map[string]bool{
		"": true, "manage_editor": true, "refresh_unity": true, "run_tests": true,
		"console": true, "exec": true, "menu": true, "screenshot": true,
		"reserialize": true, "profiler": true, "list": true,
	}
	for _, c := range commandRegistry {
		if !valid[c.ConnectorTool] {
			t.Errorf("command %q maps to unknown connector tool %q", c.Name, c.ConnectorTool)
		}
	}
}

func TestOverviewHelpContainsEveryCommand(t *testing.T) {
	help := renderOverviewHelp()
	for _, c := range commandRegistry {
		if !strings.Contains(help, c.Name) {
			t.Errorf("generated help is missing command %q", c.Name)
		}
	}
}

func TestTopicHelpRendersForEveryTopLevelCommand(t *testing.T) {
	topics := map[string]bool{}
	for _, c := range commandRegistry {
		topics[strings.SplitN(c.Name, " ", 2)[0]] = true
	}
	for topic := range topics {
		if renderTopicHelp(topic) == "" {
			t.Errorf("renderTopicHelp(%q) returned empty", topic)
		}
	}
	if renderTopicHelp("no-such-topic") != "" {
		t.Error("unknown topic should render empty")
	}
}

func TestManifestRoundTrips(t *testing.T) {
	m := buildManifest()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("manifest does not marshal: %v", err)
	}
	var decoded manifest
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("manifest does not round-trip: %v", err)
	}
	if decoded.ManifestVersion != 1 {
		t.Errorf("manifestVersion: got %d, want 1", decoded.ManifestVersion)
	}
	if len(decoded.Commands) != len(commandRegistry) {
		t.Errorf("manifest commands: got %d, want %d", len(decoded.Commands), len(commandRegistry))
	}
	if len(decoded.ExitCodes) != 6 {
		t.Errorf("manifest exit codes: got %d, want 6", len(decoded.ExitCodes))
	}
}

func TestAgentsGuideCoversContract(t *testing.T) {
	guide := renderAgentsGuide()
	for _, c := range commandRegistry {
		if !strings.Contains(guide, "`"+c.usageToken()+"`") {
			t.Errorf("agents guide missing command %q", c.Name)
		}
	}
	for _, want := range []string{"--json", "exitCode", "command_failed", "connection", "version_mismatch", "timeout", "usage", "unity-cli commands --json", "unity-cli list"} {
		if !strings.Contains(guide, want) {
			t.Errorf("agents guide missing %q", want)
		}
	}
}

func TestCommandsTextShowsConnectorMapping(t *testing.T) {
	text := renderCommandsText()
	if !strings.Contains(text, "manage_editor") {
		t.Error("commands text should show the connector tool mapping")
	}
	if !strings.Contains(text, "(local)") {
		t.Error("commands text should mark local commands")
	}
}
