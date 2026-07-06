package cmd

import (
	"fmt"
	"sort"
	"strings"
)

// FlagSpec describes one subcommand flag for help text and the command manifest.
type FlagSpec struct {
	Name        string `json:"name"`
	Value       string `json:"value,omitempty"` // e.g. "<ms>"; empty = boolean flag
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
}

// CommandSpec is the single source of truth for one CLI command: help text,
// the offline manifest (unity-cli commands --json), and the agents guide are
// all generated from it. ConnectorTool names the C# tool the command maps to,
// bridging the gap between CLI names (editor play) and `unity-cli list`
// output (manage_editor).
type CommandSpec struct {
	Name          string     `json:"name"`
	Group         string     `json:"group"`
	Summary       string     `json:"summary"`
	Positional    string     `json:"positional,omitempty"`
	Flags         []FlagSpec `json:"flags,omitempty"`
	Examples      []string   `json:"examples,omitempty"`
	ConnectorTool string     `json:"connectorTool,omitempty"` // empty = local command
	RequiresUnity bool       `json:"requiresUnity"`
	Notes         string     `json:"notes,omitempty"`
}

// groupOrder fixes the display order of command groups in help output.
var groupOrder = []string{
	"Editor Control", "Console", "Execute C#", "Menu", "Screenshot",
	"Reserialize", "Tests", "Profiler", "Custom Tools", "Status", "CLI",
}

var commandRegistry = []CommandSpec{
	{
		Name: "editor play", Group: "Editor Control",
		Summary:       "Enter play mode",
		Flags:         []FlagSpec{{Name: "wait", Description: "Block until Unity fully enters play mode; without it, returns immediately after requesting"}},
		Examples:      []string{"unity-cli editor play --wait"},
		ConnectorTool: "manage_editor", RequiresUnity: true,
	},
	{
		Name: "editor stop", Group: "Editor Control",
		Summary:       "Exit play mode (no effect if not playing)",
		Examples:      []string{"unity-cli editor stop"},
		ConnectorTool: "manage_editor", RequiresUnity: true,
	},
	{
		Name: "editor pause", Group: "Editor Control",
		Summary:       "Pause play mode (idempotent; play mode only)",
		ConnectorTool: "manage_editor", RequiresUnity: true,
	},
	{
		Name: "editor resume", Group: "Editor Control",
		Summary:       "Resume paused play mode (idempotent; play mode only)",
		ConnectorTool: "manage_editor", RequiresUnity: true,
	},
	{
		Name: "editor refresh", Group: "Editor Control",
		Summary: "Refresh asset database (reimport changed assets)",
		Flags: []FlagSpec{
			{Name: "compile", Description: "Recompile scripts and wait until compilation finishes"},
			{Name: "force", Description: "Allow refresh during play mode and force asset update"},
		},
		Examples:      []string{"unity-cli editor refresh --compile", "unity-cli editor refresh --force"},
		ConnectorTool: "refresh_unity", RequiresUnity: true,
		Notes: "Blocked in play mode unless --force is set.",
	},
	{
		Name: "console", Group: "Console",
		Summary: "Read Unity console log entries",
		Flags: []FlagSpec{
			{Name: "lines", Value: "<N>", Description: "Limit to N entries"},
			{Name: "type", Value: "<types>", Description: "Comma-separated log types: error, warning, log", Default: "error,warning,log"},
			{Name: "stacktrace", Value: "<mode>", Description: "none: first line only / user: internal frames filtered / full: raw message", Default: "user"},
			{Name: "clear", Description: "Clear console"},
		},
		Examples: []string{
			"unity-cli console",
			"unity-cli console --lines 20 --type error,warning",
			"unity-cli console --type error --stacktrace full",
			"unity-cli console --clear",
		},
		ConnectorTool: "console", RequiresUnity: true,
	},
	{
		Name: "exec", Group: "Execute C#",
		Summary:    "Run C# code inside Unity Editor (return required for output)",
		Positional: `"<code>"`,
		Flags: []FlagSpec{
			{Name: "usings", Value: "<ns1,ns2>", Description: "Add extra using directives"},
			{Name: "csc", Value: "<path>", Description: "Path to csc compiler (csc.dll or csc.exe); auto-detected if omitted"},
			{Name: "dotnet", Value: "<path>", Description: "Path to dotnet runtime; auto-detected if omitted"},
			{Name: "timeout_sec", Value: "<sec>", Description: "Compile timeout in seconds (execution phase is not bounded)", Default: "30"},
		},
		Examples: []string{
			`unity-cli exec "return 1+1;"`,
			`unity-cli exec "return Application.dataPath;"`,
			`echo 'return EditorSceneManager.GetActiveScene().name;' | unity-cli exec`,
			`unity-cli exec "return World.All.Count;" --usings Unity.Entities`,
		},
		ConnectorTool: "exec", RequiresUnity: true,
		Notes: `Full access to UnityEngine, UnityEditor, and all loaded assemblies.
Use 'return' for output, 'return null;' for void operations.
Pipe code via stdin to avoid shell escaping: echo '<code>' | unity-cli exec
Identical snippets are compiled once and cached until the next domain reload.
Compile errors return structured data.errors with line numbers in YOUR code.

Default usings: System, System.Collections.Generic, System.IO, System.Linq,
  System.Reflection, System.Threading.Tasks, UnityEngine,
  UnityEngine.SceneManagement, UnityEditor, UnityEditor.SceneManagement,
  UnityEditorInternal`,
	},
	{
		Name: "menu", Group: "Menu",
		Summary:    "Execute a Unity menu item by path",
		Positional: `"<path>"`,
		Examples: []string{
			`unity-cli menu "File/Save Project"`,
			`unity-cli menu "Assets/Refresh"`,
			`unity-cli menu "Window/General/Console"`,
		},
		ConnectorTool: "menu", RequiresUnity: true,
		Notes: "File/Quit is blocked for safety.",
	},
	{
		Name: "screenshot", Group: "Screenshot",
		Summary: "Capture a screenshot of the Unity editor",
		Flags: []FlagSpec{
			{Name: "view", Value: "<mode>", Description: "scene or game", Default: "scene"},
			{Name: "width", Value: "<N>", Description: "Image width in pixels", Default: "1920"},
			{Name: "height", Value: "<N>", Description: "Image height in pixels", Default: "1080"},
			{Name: "output_path", Value: "<path>", Description: "Output path, absolute or relative to project root", Default: "Screenshots/screenshot.png"},
		},
		Examples: []string{
			"unity-cli screenshot",
			"unity-cli screenshot --view game",
			"unity-cli screenshot --view scene --width 3840 --height 2160",
		},
		ConnectorTool: "screenshot", RequiresUnity: true,
	},
	{
		Name: "reserialize", Group: "Reserialize",
		Summary:    "Force reserialize assets through Unity's YAML serializer (no args = entire project)",
		Positional: "[path...]",
		Examples: []string{
			"unity-cli reserialize",
			"unity-cli reserialize Assets/Prefabs/Player.prefab",
			"unity-cli reserialize Assets/Scenes/Main.unity Assets/Scenes/Lobby.unity",
		},
		ConnectorTool: "reserialize", RequiresUnity: true,
		Notes: "Run after editing .prefab, .unity, .asset, or .mat files as text.",
	},
	{
		Name: "test", Group: "Tests",
		Summary: "Run Unity tests via the Test Runner API",
		Flags: []FlagSpec{
			{Name: "mode", Value: "<EditMode|PlayMode>", Description: "Test mode", Default: "EditMode"},
			{Name: "filter", Value: "<name>", Description: "Filter by namespace, class, or full test name (full path, e.g. MyNamespace.MyClass)"},
			{Name: "allow-dirty-scenes", Description: "Run even when open scenes have unsaved changes"},
			{Name: "auto-save-scenes", Description: "Save dirty open scenes before running tests"},
			{Name: "timeout-sec", Value: "<sec>", Description: "EditMode run timeout; on expiry the request is released with a test_timeout error", Default: "300"},
		},
		Examples: []string{
			"unity-cli test",
			"unity-cli test --mode PlayMode",
			"unity-cli test --filter MyNamespace.MyTests",
		},
		ConnectorTool: "run_tests", RequiresUnity: true,
		Notes: `EditMode tests hold the connection open and return results directly.
PlayMode tests return immediately and poll a results file (domain reload safe).
By default, tests are blocked when any open scene has unsaved changes.
Requires the Unity Test Framework package (com.unity.test-framework).`,
	},
	{
		Name: "profiler hierarchy", Group: "Profiler",
		Summary: "Profiler samples (last frame by default)",
		Flags: []FlagSpec{
			{Name: "depth", Value: "<N>", Description: "Recursive depth (0=unlimited)", Default: "1"},
			{Name: "root", Value: "<name>", Description: "Set root by name (substring match, searches full tree)"},
			{Name: "frames", Value: "<N>", Description: "Average over last N frames (flat output, sorted by time)"},
			{Name: "from", Value: "<N>", Description: "Start frame index for range average"},
			{Name: "to", Value: "<N>", Description: "End frame index for range average"},
			{Name: "parent", Value: "<ID>", Description: "Drill into item by ID"},
			{Name: "min", Value: "<ms>", Description: "Filter items below threshold"},
			{Name: "sort", Value: "<col>", Description: "Sort by: total, self, calls", Default: "total"},
			{Name: "max", Value: "<N>", Description: "Max children per level", Default: "30"},
			{Name: "frame", Value: "<N>", Description: "Specific frame index"},
			{Name: "thread", Value: "<N>", Description: "Thread index (0=main)"},
		},
		Examples: []string{
			"unity-cli profiler hierarchy --depth 3",
			"unity-cli profiler hierarchy --root SimulationSystem --depth 3",
			"unity-cli profiler hierarchy --frames 30 --min 0.5 --sort self",
		},
		ConnectorTool: "profiler", RequiresUnity: true,
	},
	{
		Name: "profiler enable", Group: "Profiler",
		Summary:       "Start profiler recording",
		ConnectorTool: "profiler", RequiresUnity: true,
	},
	{
		Name: "profiler disable", Group: "Profiler",
		Summary:       "Stop profiler recording",
		ConnectorTool: "profiler", RequiresUnity: true,
	},
	{
		Name: "profiler status", Group: "Profiler",
		Summary:       "Show profiler state",
		ConnectorTool: "profiler", RequiresUnity: true,
	},
	{
		Name: "profiler clear", Group: "Profiler",
		Summary:       "Clear all captured frames",
		ConnectorTool: "profiler", RequiresUnity: true,
	},
	{
		Name: "list", Group: "Custom Tools",
		Summary:       "List all registered tools (built-in + custom) with parameter schemas",
		Examples:      []string{"unity-cli list"},
		ConnectorTool: "list", RequiresUnity: true,
		Notes: `Any [UnityCliTool] class in the project is auto-discovered and callable
directly: unity-cli <tool_name> --params '{"k":"v"}' (or --key value flags).
See "unity-cli help custom-tools" for how to write one.`,
	},
	{
		Name: "status", Group: "Status",
		Summary:  "Show Unity Editor state (ready, compiling, playing, ...)",
		Examples: []string{"unity-cli status"},
		Notes:    `Reads heartbeat files; reports "not responding" if the heartbeat is older than 3 seconds.`,
	},
	{
		Name: "update", Group: "CLI",
		Summary: "Update the CLI binary to the latest GitHub release",
		Flags: []FlagSpec{
			{Name: "check", Description: "Check for updates without installing"},
		},
		Examples: []string{"unity-cli update", "unity-cli update --check"},
	},
	{
		Name: "version", Group: "CLI",
		Summary: "Print the CLI version",
	},
	{
		Name: "commands", Group: "CLI",
		Summary: "List all commands; with --json, print the full machine-readable manifest (works offline)",
		Examples: []string{
			"unity-cli commands",
			"unity-cli commands --json",
		},
	},
	{
		Name: "help", Group: "CLI",
		Summary:    "Show help for a command or topic (agents, custom-tools, setup)",
		Positional: "[topic]",
		Examples:   []string{"unity-cli help exec", "unity-cli help agents"},
	},
}

// globalFlagDocs documents the global flags for help and the manifest.
var globalFlagDocs = []FlagSpec{
	{Name: "project", Value: "<path>", Description: "Select Unity instance by project path"},
	{Name: "timeout", Value: "<ms>", Description: "Request timeout in ms. When set explicitly, also bounds compile waits and PlayMode test polling (defaults: 5m / 10m)", Default: "120000"},
	{Name: "json", Description: "Emit one machine-readable JSON envelope on stdout"},
	{Name: "ignore-version-mismatch", Description: "Skip CLI/connector version check"},
}

type exitCodeDoc struct {
	Code    int    `json:"code"`
	Class   string `json:"class"`
	Meaning string `json:"meaning"`
}

var exitCodeDocs = []exitCodeDoc{
	{0, "", "success"},
	{int(ExitCommandFailed), "command_failed", "Unity executed the command but reported failure (test failures, compile errors, tool errors)"},
	{int(ExitUsage), "usage", "invalid flags or arguments, before contacting Unity"},
	{int(ExitConnection), "connection", "Unity not running, unreachable, or ambiguous instance selection"},
	{int(ExitVersionMismatch), "version_mismatch", "CLI and connector versions differ"},
	{int(ExitTimeout), "timeout", "deadline exceeded while waiting for the listener, compilation, or test results"},
}

// manifest is the offline machine-readable description of the CLI surface,
// emitted by `unity-cli commands --json`.
type manifest struct {
	ManifestVersion int               `json:"manifestVersion"`
	CLIVersion      string            `json:"cliVersion"`
	GlobalFlags     []FlagSpec        `json:"globalFlags"`
	ExitCodes       []exitCodeDoc     `json:"exitCodes"`
	Envelope        map[string]string `json:"envelope"`
	Commands        []CommandSpec     `json:"commands"`
	CustomTools     string            `json:"customTools"`
}

func buildManifest() manifest {
	return manifest{
		ManifestVersion: 1,
		CLIVersion:      Version,
		GlobalFlags:     globalFlagDocs,
		ExitCodes:       exitCodeDocs,
		Envelope: map[string]string{
			"success":     "bool",
			"message":     "string",
			"data":        "any (command-specific; omitted when null)",
			"error.class": "usage | connection | version_mismatch | timeout | command_failed (failures only)",
			"exitCode":    "int, matches the process exit code",
		},
		Commands:    commandRegistry,
		CustomTools: "Run 'unity-cli list' with Unity open to discover project-specific [UnityCliTool] tools and their parameter schemas. Call them as: unity-cli <tool_name> --key value",
	}
}

// usageToken renders the short usage form shown in the overview help.
func (c CommandSpec) usageToken() string {
	parts := []string{c.Name}
	if c.Positional != "" {
		parts = append(parts, c.Positional)
	}
	switch len(c.Flags) {
	case 0:
	case 1:
		f := c.Flags[0]
		if f.Value != "" {
			parts = append(parts, fmt.Sprintf("[--%s %s]", f.Name, f.Value))
		} else {
			parts = append(parts, fmt.Sprintf("[--%s]", f.Name))
		}
	default:
		parts = append(parts, "[options]")
	}
	return strings.Join(parts, " ")
}

func commandsByGroup() map[string][]CommandSpec {
	groups := map[string][]CommandSpec{}
	for _, c := range commandRegistry {
		groups[c.Group] = append(groups[c.Group], c)
	}
	return groups
}

// renderOverviewHelp generates the top-level --help text from the registry.
func renderOverviewHelp() string {
	var b strings.Builder
	fmt.Fprintf(&b, "unity-cli %s — Control Unity Editor from the command line\n\n", Version)
	b.WriteString("Usage: unity-cli <command> [subcommand] [options]\n")

	groups := commandsByGroup()
	for _, group := range groupOrder {
		cmds := groups[group]
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", group)
		for _, c := range cmds {
			fmt.Fprintf(&b, "  %-32s %s\n", c.usageToken(), c.Summary)
		}
	}

	b.WriteString("\nGlobal Options:\n")
	for _, f := range globalFlagDocs {
		b.WriteString(renderFlag(f, "  "))
	}

	b.WriteString(`
Exit Codes:
  0 success / 1 command failed / 2 usage error / 3 connection failure
  4 version mismatch / 5 timeout

Use "unity-cli <command> --help" for more information about a command.

Notes:
  - Unity must be open with the Connector package installed
  - Multiple Unity instances: use --project to select
  - Custom tools: any [UnityCliTool] class is auto-discovered; run 'list' to see all
  - Machine-readable: 'unity-cli commands --json' (offline), '--json' on any command
  - AI agent setup guide: 'unity-cli help agents'
`)
	return b.String()
}

// renderFlag renders one flag line (with wrapped description) for help text.
func renderFlag(f FlagSpec, indent string) string {
	head := "--" + f.Name
	if f.Value != "" {
		head += " " + f.Value
	}
	desc := f.Description
	if f.Default != "" {
		desc += fmt.Sprintf(" (default: %s)", f.Default)
	}
	if len(head) <= 22 {
		return fmt.Sprintf("%s%-22s %s\n", indent, head, desc)
	}
	return fmt.Sprintf("%s%s\n%s%-22s %s\n", indent, head, indent, "", desc)
}

// renderTopicHelp generates detailed help for a command or command family.
// Returns "" when the topic has no registry entries.
func renderTopicHelp(topic string) string {
	var entries []CommandSpec
	for _, c := range commandRegistry {
		if c.Name == topic || strings.HasPrefix(c.Name, topic+" ") {
			entries = append(entries, c)
		}
	}
	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	if len(entries) == 1 {
		c := entries[0]
		usage := "Usage: unity-cli " + c.Name
		if c.Positional != "" {
			usage += " " + c.Positional
		}
		if len(c.Flags) > 0 {
			usage += " [options]"
		}
		b.WriteString(usage + "\n\n" + c.Summary + "\n")
		if len(c.Flags) > 0 {
			b.WriteString("\nOptions:\n")
			for _, f := range c.Flags {
				b.WriteString(renderFlag(f, "  "))
			}
		}
		writeExamplesAndNotes(&b, c.Examples, c.Notes)
		return b.String()
	}

	// Command family (e.g. editor, profiler): one section per subcommand.
	var subNames []string
	for _, c := range entries {
		subNames = append(subNames, strings.TrimPrefix(c.Name, topic+" "))
	}
	fmt.Fprintf(&b, "Usage: unity-cli %s <%s> [options]\n\nSubcommands:\n", topic, strings.Join(subNames, "|"))
	var examples []string
	var notes []string
	for _, c := range entries {
		sub := strings.TrimPrefix(c.Name, topic+" ")
		fmt.Fprintf(&b, "  %-20s %s\n", sub, c.Summary)
		for _, f := range c.Flags {
			b.WriteString(renderFlag(f, "    "))
		}
		examples = append(examples, c.Examples...)
		if c.Notes != "" {
			notes = append(notes, c.Notes)
		}
	}
	writeExamplesAndNotes(&b, examples, strings.Join(notes, "\n"))
	return b.String()
}

func writeExamplesAndNotes(b *strings.Builder, examples []string, notes string) {
	if len(examples) > 0 {
		b.WriteString("\nExamples:\n")
		for _, e := range examples {
			b.WriteString("  " + e + "\n")
		}
	}
	if notes != "" {
		b.WriteString("\nNotes:\n")
		for _, line := range strings.Split(notes, "\n") {
			b.WriteString("  " + line + "\n")
		}
	}
}

// renderCommandsText renders the human-readable `unity-cli commands` listing,
// including the CLI-name → connector-tool mapping.
func renderCommandsText() string {
	var b strings.Builder
	fmt.Fprintf(&b, "unity-cli %s commands (offline catalog; run 'unity-cli commands --json' for the machine-readable manifest)\n\n", Version)

	nameWidth := 0
	for _, c := range commandRegistry {
		if len(c.Name) > nameWidth {
			nameWidth = len(c.Name)
		}
	}

	groups := commandsByGroup()
	for _, group := range groupOrder {
		cmds := groups[group]
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", group)
		for _, c := range cmds {
			tool := c.ConnectorTool
			if tool == "" {
				tool = "(local)"
			}
			fmt.Fprintf(&b, "  %-*s  →  %-15s %s\n", nameWidth, c.Name, tool, c.Summary)
		}
		b.WriteString("\n")
	}
	b.WriteString("→ shows the C# connector tool each command maps to ('unity-cli list' names).\nCustom [UnityCliTool] tools are discovered live: run 'unity-cli list' with Unity open.\n")
	return b.String()
}

// renderAgentsGuide generates a paste-ready markdown snippet for the end
// user's project CLAUDE.md / AGENTS.md, teaching an AI agent the full CLI
// contract: commands, output envelope, exit codes, and typical workflows.
func renderAgentsGuide() string {
	var b strings.Builder
	b.WriteString(`# Unity control via unity-cli

Unity Editor is controlled from the shell with ` + "`unity-cli`" + ` (CLI alternative to MCP).
Unity must be open with the Connector package installed. Results go to stdout,
errors and progress messages to stderr.

## Machine-readable output

Add ` + "`--json`" + ` to any command: exactly one JSON envelope is written to stdout,
success or failure, even when Unity is not running:

` + "```json" + `
{"success": true, "message": "OK", "data": {}, "exitCode": 0}
{"success": false, "message": "no Unity instances running", "error": {"class": "connection"}, "exitCode": 3}
` + "```" + `

## Exit codes / error.class

| Code | Class | Meaning |
|------|-------|---------|
`)
	for _, ec := range exitCodeDocs {
		class := ec.Class
		if class == "" {
			class = "—"
		}
		fmt.Fprintf(&b, "| %d | %s | %s |\n", ec.Code, class, ec.Meaning)
	}

	b.WriteString("\n## Commands\n\n")
	groups := commandsByGroup()
	for _, group := range groupOrder {
		for _, c := range groups[group] {
			fmt.Fprintf(&b, "- `%s` — %s", c.usageToken(), c.Summary)
			if len(c.Examples) > 0 {
				fmt.Fprintf(&b, " (e.g. `%s`)", c.Examples[0])
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n## Global flags\n\n")
	for _, f := range globalFlagDocs {
		head := "--" + f.Name
		if f.Value != "" {
			head += " " + f.Value
		}
		fmt.Fprintf(&b, "- `%s` — %s", head, f.Description)
		if f.Default != "" {
			fmt.Fprintf(&b, " (default: %s)", f.Default)
		}
		b.WriteString("\n")
	}

	b.WriteString(`
## Typical workflows

- After editing C# files: ` + "`unity-cli editor refresh --compile`" + ` — waits for
  compilation; on failure, inspect ` + "`unity-cli console --type error`" + `
- Run tests: ` + "`unity-cli test`" + ` (EditMode) or ` + "`unity-cli test --mode PlayMode`" + `
- After editing .prefab/.unity/.asset/.mat files as text: ` + "`unity-cli reserialize <path>`" + `
- Arbitrary C# in the editor: ` + "`echo 'return EditorSceneManager.GetActiveScene().name;' | unity-cli exec`" + `
- Visual check: ` + "`unity-cli screenshot --view game`" + `
- Check editor state first when commands fail: ` + "`unity-cli status`" + `

## Discovery

- ` + "`unity-cli commands --json`" + ` — full command manifest (works offline)
- ` + "`unity-cli list`" + ` — live tool schemas, including project-specific [UnityCliTool] tools;
  call custom tools directly: ` + "`unity-cli <tool_name> --key value`" + `
`)
	return b.String()
}

// registryNames returns all registered command names, sorted.
func registryNames() []string {
	names := make([]string, 0, len(commandRegistry))
	for _, c := range commandRegistry {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}
