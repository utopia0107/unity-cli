# unity-cli

CLI tool to control Unity Editor from the command line.

## Structure

```
cmd/                  # Go CLI — thin passthrough layer
  root.go             # Entry point, flag/arg parsing, default passthrough
  editor.go           # editor command (waitForReady polling)
  test.go             # test command (PlayMode result polling)
  status.go           # status, waitForAlive, heartbeat reading
  update.go           # self-update from GitHub releases
  version_check.go    # periodic update notice (12h interval)
internal/client/      # Unity HTTP client, instance discovery
unity-connector/      # C# Unity Editor package (UPM)
  Editor/
    Core/             # Shared utilities (Response, ParamCoercion, ToolParams, StringCaseUtility)
    Tools/            # Tool implementations (auto-registered via [UnityCliTool] attribute)
    TestRunner/       # Test runner (RunTests, TestRunnerState)
```

## Development

### Adding a Command

1. Add a C# tool in `unity-connector/Editor/Tools/` with `[UnityCliTool(Name = "command_name")]`
2. CLI command name matches the tool name — default passthrough handles dispatch
3. Positional args arrive as `args` array, flags as named params
4. Go-side code is only needed for polling/waiting logic (editor, test)

## Verification

Run all of the following before pushing:

```bash
go clean -testcache
gofmt -w .
~/go/bin/golangci-lint run ./...
~/go/bin/golangci-lint fmt --diff
go test ./...
```

### Integration Tests (requires Unity)

Integration tests are tagged with `//go:build integration` and excluded from the default test run.
Run them manually when Unity Editor is open:

```bash
go test -tags integration ./...
```

CI skips these since Unity is not available.

## Git

Commit all unstaged changes before finishing. Unrelated changes should be committed separately.

## CI

- `push/PR → main`: build, vet, test, lint, format
- `tag push (v*)`: cross-compile (linux/darwin/windows × amd64/arm64) + GitHub Release
