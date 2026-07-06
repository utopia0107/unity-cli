package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/youngwoocho02/unity-cli/internal/client"
)

var Version = "dev"

var (
	flagProject               string
	flagTimeout               int
	flagIgnoreVersionMismatch bool
	flagTimeoutSet            bool
)

func Execute() error {
	flag.StringVar(&flagProject, "project", "", "Select Unity instance by project path")
	flag.IntVar(&flagTimeout, "timeout", 120000, "Request timeout in milliseconds")
	flag.BoolVar(&flagIgnoreVersionMismatch, "ignore-version-mismatch", false, "Skip CLI/connector version check")
	flag.BoolVar(&flagJSON, "json", false, "Emit a machine-readable JSON envelope on stdout")

	flag.Usage = func() { printHelp() }

	args := os.Args[1:]
	flagArgs, cmdArgs := splitArgs(args)
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		return withCode(ExitUsage, fmt.Errorf("flag parse error: %w", err))
	}
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "timeout" {
			flagTimeoutSet = true
		}
	})

	// In --json mode every outcome, including early failures, becomes an
	// envelope on stdout; the exit code still reflects the failure class.
	return finishErr(executeParsed(args, cmdArgs))
}

func executeParsed(args, cmdArgs []string) error {
	if err := rejectRemovedFlags(args); err != nil {
		return withCode(ExitUsage, err)
	}

	if len(cmdArgs) == 0 {
		printHelp()
		return nil
	}

	category := cmdArgs[0]
	subArgs := cmdArgs[1:]

	// --help / -h on any command
	for _, a := range subArgs {
		if a == "--help" || a == "-h" {
			printTopicHelp(category)
			return nil
		}
	}

	switch category {
	case "help", "--help", "-h":
		if len(subArgs) > 0 {
			printTopicHelp(subArgs[0])
		} else {
			printHelp()
		}
		return nil
	case "version", "--version", "-v":
		return versionCmd()
	case "commands":
		return commandsCmd()
	case "update":
		return updateCmd(subArgs)
	case "status":
		statusErr := statusCmd(flagProject, flagIgnoreVersionMismatch)
		if !flagJSON {
			printUpdateNotice()
		}
		return statusErr
	}

	inst, err := client.DiscoverInstance(flagProject)
	if err != nil {
		return withCode(ExitConnection, err)
	}

	targetProject := flagProject
	if targetProject == "" {
		targetProject = inst.ProjectPath
	}

	resolve := func() (*client.Instance, error) {
		return client.DiscoverInstance(targetProject)
	}

	alive, err := resolveReadyTimeout(resolve, flagTimeout)
	if err != nil {
		return err
	}
	if err := checkConnectorVersion(alive, Version, flagIgnoreVersionMismatch); err != nil {
		return err
	}

	timeout := flagTimeout
	send := func(command string, params interface{}) (*client.CommandResponse, error) {
		return sendWithRetry(resolve, command, params, timeout)
	}

	var resp *client.CommandResponse

	switch category {
	case "editor":
		resp, err = editorCmd(subArgs, send, resolve)
	case "test":
		resp, err = testCmd(subArgs, send, resolve)
	case "exec":
		subArgs = readStdinIfPiped(subArgs)
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			resp, err = send("exec", params)
		}
	default:
		var params map[string]interface{}
		params, err = buildParams(subArgs, nil)
		if err == nil {
			resp, err = send(category, params)
		}
	}

	if err != nil {
		return err
	}

	return finishResponse(resp)
}

func versionCmd() error {
	if flagJSON {
		data, _ := json.Marshal(map[string]string{"version": Version})
		return emitEnvelope(&client.CommandResponse{Success: true, Message: "unity-cli " + Version, Data: data})
	}
	fmt.Println("unity-cli " + Version)
	return nil
}

// commandsCmd prints the offline command catalog; with --json, the full manifest.
func commandsCmd() error {
	if flagJSON {
		data, err := json.Marshal(buildManifest())
		if err != nil {
			return err
		}
		return emitEnvelope(&client.CommandResponse{Success: true, Message: "command manifest", Data: data})
	}
	fmt.Print(renderCommandsText())
	return nil
}

// finishResponse prints a command response in the active output mode.
func finishResponse(resp *client.CommandResponse) error {
	if flagJSON {
		return emitEnvelope(resp)
	}
	printResponse(resp)
	printUpdateNotice()
	if !resp.Success {
		return withCode(ExitCommandFailed, errReported)
	}
	return nil
}

// finishErr converts a not-yet-reported error into a JSON envelope when --json
// is active, so stdout always carries exactly one envelope.
func finishErr(err error) error {
	if err == nil || !flagJSON || IsReported(err) {
		return err
	}
	code := ExitCodeFor(err)
	writeEnvelope(jsonEnvelope{
		Success:  false,
		Message:  err.Error(),
		Error:    &jsonError{Class: errorClass(code)},
		ExitCode: int(code),
	})
	return withCode(code, errReported)
}

// sendFn is the function signature for sending a command to Unity.
// Injected into each command function so they can be tested without a real Unity connection.
type sendFn func(command string, params interface{}) (*client.CommandResponse, error)

var (
	resolveReadyTimeout = resolveReady
	sendCommand         = client.Send
	healthCheck         = client.Health
)

func resolveReady(resolve instanceResolver, timeoutMs int) (*client.Instance, error) {
	return resolveReadyUntil(resolve, commandDeadline(timeoutMs))
}

// resolveReadyUntil polls resolve → version check → health until the listener is
// ready or the deadline passes. With --ignore-version-mismatch, a missing health
// endpoint (legacy connector) falls back to the raw heartbeat instance.
func resolveReadyUntil(resolve instanceResolver, deadline time.Time) (*client.Instance, error) {
	var lastErr error
	for {
		inst, err := resolve()
		if err != nil {
			lastErr = err
			if !sleepUntilNextPoll(deadline) {
				break
			}
			continue
		}
		if err := checkConnectorVersion(inst, Version, flagIgnoreVersionMismatch); err != nil {
			return nil, err
		}
		health, err := healthCheck(inst, remainingMs(deadline, time.Second))
		if err != nil {
			if flagIgnoreVersionMismatch && errors.Is(err, client.ErrHealthEndpointUnavailable) {
				return inst, nil
			}
			lastErr = err
			if !sleepUntilNextPoll(deadline) {
				break
			}
			continue
		}
		if err := checkConnectorVersion(health, Version, flagIgnoreVersionMismatch); err != nil {
			return nil, err
		}
		return health, nil
	}
	if lastErr != nil {
		return nil, withCode(ExitTimeout, fmt.Errorf("timed out waiting for Unity listener: %w", lastErr))
	}
	return nil, withCode(ExitTimeout, fmt.Errorf("timed out waiting for Unity listener"))
}

func sendWithRetry(resolve instanceResolver, command string, params interface{}, timeoutMs int) (*client.CommandResponse, error) {
	deadline := commandDeadline(timeoutMs)
	inst, err := resolveReadyUntil(resolve, deadline)
	if err != nil {
		return nil, err
	}
	resp, err := sendCommand(inst, command, params, remainingMs(deadline, 0))
	if err != nil {
		return nil, fmt.Errorf("failed sending command to Unity: %w", err)
	}
	return resp, nil
}

func commandDeadline(timeoutMs int) time.Time {
	if timeoutMs <= 0 {
		timeoutMs = 120000
	}
	return time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
}

// operationDeadline returns the deadline for long-running waits (compile wait,
// PlayMode test polling). An explicit --timeout governs them; otherwise the
// operation-specific fallback applies so the 120s default doesn't silently
// truncate multi-minute compiles or test runs.
func operationDeadline(fallback time.Duration) time.Time {
	if flagTimeoutSet {
		return commandDeadline(flagTimeout)
	}
	return time.Now().Add(fallback)
}

// remainingMs returns the milliseconds left until deadline, at least 1,
// optionally capped (maxWait > 0) for short probe requests.
func remainingMs(deadline time.Time, maxWait time.Duration) int {
	remaining := time.Until(deadline)
	if maxWait > 0 && remaining > maxWait {
		remaining = maxWait
	}
	ms := int(remaining / time.Millisecond)
	if ms < 1 {
		return 1
	}
	return ms
}

func sleepUntilNextPoll(deadline time.Time) bool {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return false
	}
	sleep := statusPollInterval
	if remaining < sleep {
		sleep = remaining
	}
	time.Sleep(sleep)
	return time.Now().Before(deadline)
}

func printResponse(resp *client.CommandResponse) {
	if !resp.Success {
		msg := resp.Message
		if msg == "" {
			msg = "unknown error"
		}
		if len(resp.Data) > 0 && string(resp.Data) != "null" {
			fmt.Fprintf(os.Stderr, "Error: %s\nDetails: %s\n", msg, string(resp.Data))
		} else {
			fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
		}
		return
	}

	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		var pretty interface{}
		if json.Unmarshal(resp.Data, &pretty) == nil {
			// If data is a plain string, print it raw (preserves newlines for tree output etc.)
			if s, ok := pretty.(string); ok {
				fmt.Println(s)
			} else {
				b, _ := json.MarshalIndent(pretty, "", "  ")
				fmt.Println(string(b))
			}
		} else {
			fmt.Println(string(resp.Data))
		}
	} else if resp.Message != "" {
		fmt.Println(resp.Message)
	}
}

// parseFlagsAndArgs parses --key value, --key=value, and --flag (boolean) pairs
// plus positional args from subcommand args.
func parseFlagsAndArgs(args []string) (flags map[string]string, positional []string) {
	flags = map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			positional = append(positional, a)
			continue
		}
		key := a[2:]
		if eq := strings.Index(key, "="); eq >= 0 {
			flags[key[:eq]] = key[eq+1:]
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
			flags[key] = args[i+1]
			i++
		} else {
			flags[key] = "true"
		}
	}
	return flags, positional
}

// parseSubFlags parses subcommand flags; positional args are silently ignored.
func parseSubFlags(args []string) map[string]string {
	flags, _ := parseFlagsAndArgs(args)
	return flags
}

// buildParams parses --flag value pairs and positional args from args and merges with base params.
func buildParams(args []string, base map[string]interface{}) (map[string]interface{}, error) {
	params := map[string]interface{}{}
	for k, v := range base {
		params[k] = v
	}

	flags, positional := parseFlagsAndArgs(args)

	if raw, ok := flags["params"]; ok {
		if jsonErr := json.Unmarshal([]byte(raw), &params); jsonErr != nil {
			return nil, withCode(ExitUsage, fmt.Errorf("invalid JSON in --params: %w", jsonErr))
		}
	}
	for k, v := range flags {
		if k == "params" {
			continue
		}
		if _, exists := params[k]; exists {
			continue
		}
		if n, err := strconv.Atoi(v); err == nil {
			params[k] = n
		} else if v == "true" {
			params[k] = true
		} else if v == "false" {
			params[k] = false
		} else {
			params[k] = v
		}
	}

	if len(positional) > 0 {
		params["args"] = positional
	}

	return params, nil
}

func rejectRemovedFlags(args []string) error {
	for _, arg := range args {
		if arg == "--port" || strings.HasPrefix(arg, "--port=") {
			return fmt.Errorf("--port was removed; select Unity by project path with --project")
		}
	}

	return nil
}

// readStdinIfPiped reads stdin when piped and prepends it as the first positional arg.
func readStdinIfPiped(args []string) []string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return args
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return args // interactive terminal, not piped
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil || len(data) == 0 {
		return args
	}
	code := strings.TrimRight(string(data), "\n\r")
	return append([]string{code}, args...)
}

// globalFlagSpec maps each global flag name to whether it takes a value.
var globalFlagSpec = map[string]bool{
	"project":                 true,
	"timeout":                 true,
	"ignore-version-mismatch": false,
	"json":                    false,
}

// splitArgs separates global flags from subcommand args.
// Global flags must be parsed by flag.CommandLine before the subcommand runs.
// Both "--name value" and "--name=value" forms are recognized.
func splitArgs(args []string) (flags, commands []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			commands = append(commands, a)
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if eq := strings.Index(name, "="); eq >= 0 {
			name = name[:eq]
			if _, ok := globalFlagSpec[name]; ok {
				flags = append(flags, a)
			} else {
				commands = append(commands, a)
			}
			continue
		}
		takesValue, ok := globalFlagSpec[name]
		if !ok {
			commands = append(commands, a)
			continue
		}
		flags = append(flags, a)
		if takesValue && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return
}

func printHelp() {
	fmt.Print(renderOverviewHelp())
}

func printTopicHelp(topic string) {
	switch topic {
	case "custom-tools", "custom", "tools":
		fmt.Print(customToolsHelp)
	case "setup", "install":
		fmt.Print(setupHelp)
	default:
		if s := renderTopicHelp(topic); s != "" {
			fmt.Print(s)
			return
		}
		fmt.Printf("Unknown help topic: %s\n\nUse \"unity-cli --help\" for available commands.\n", topic)
	}
}

const customToolsHelp = `How to write custom tools for unity-cli

Custom tools are C# classes that run inside Unity Editor. The CLI
discovers them automatically via reflection.

Create a static class with [UnityCliTool] in any Editor assembly:

    using UnityCliConnector;
    using Newtonsoft.Json.Linq;

    [UnityCliTool(Description = "Spawn an enemy at a position")]
    public static class SpawnEnemy
    {
        public class Parameters
        {
            [ToolParameter("X world position", Required = true)]
            public float X { get; set; }
        }

        public static object HandleCommand(JObject parameters)
        {
            float x = parameters["x"]?.Value<float>() ?? 0;
            var go = Object.Instantiate(prefab, new Vector3(x, 0, 0), Quaternion.identity);
            return new SuccessResponse("Spawned", new { name = go.name });
        }
    }

Rules:
  - Class must be static
  - Must have: public static object HandleCommand(JObject parameters)
  - Return SuccessResponse(message, data) or ErrorResponse(message)
  - Add Parameters class with [ToolParameter] for discoverability
  - Class name auto-converts to snake_case (SpawnEnemy → spawn_enemy)
  - Override name: [UnityCliTool(Name = "my_name")]
  - Runs on Unity main thread — all Unity APIs are safe
  - Discovered on Editor start and after every script recompilation
  - Duplicate tool names are detected and logged as errors (first wins)
`

const setupHelp = `Installation and Unity setup

CLI Installation:
  # Linux / macOS
  curl -fsSL https://raw.githubusercontent.com/youngwoocho02/unity-cli/master/install.sh | sh

  # Windows (PowerShell)
  irm https://raw.githubusercontent.com/youngwoocho02/unity-cli/master/install.ps1 | iex

  # Go install (any platform)
  go install github.com/youngwoocho02/unity-cli@latest

Unity Setup:
  1. Window → Package Manager → + → Add package from git URL
  2. Paste: https://github.com/youngwoocho02/unity-cli.git?path=unity-connector
  The Connector starts automatically when Unity opens.

Verify:
  unity-cli list
`
