package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/youngwoocho02/unity-cli/internal/client"
)

// flagJSON switches all stdout output to a single JSON envelope per run.
var flagJSON bool

// stdout is injectable for tests.
var stdout io.Writer = os.Stdout

// jsonEnvelope is the --json output contract:
// {"success", "message", "data", "error": {"class"}, "exitCode"}.
type jsonEnvelope struct {
	Success  bool            `json:"success"`
	Message  string          `json:"message,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Error    *jsonError      `json:"error,omitempty"`
	ExitCode int             `json:"exitCode"`
}

type jsonError struct {
	Class string `json:"class"`
}

// errorClass maps an exit code to the envelope's error.class string (1:1).
func errorClass(code ExitCode) string {
	switch code {
	case ExitUsage:
		return "usage"
	case ExitConnection:
		return "connection"
	case ExitVersionMismatch:
		return "version_mismatch"
	case ExitTimeout:
		return "timeout"
	default:
		return "command_failed"
	}
}

// emitEnvelope writes a connector response as a JSON envelope on stdout.
func emitEnvelope(resp *client.CommandResponse) error {
	env := jsonEnvelope{Success: resp.Success, Message: resp.Message}
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		env.Data = resp.Data
	}
	if resp.Success {
		writeEnvelope(env)
		return nil
	}
	env.Error = &jsonError{Class: errorClass(ExitCommandFailed)}
	env.ExitCode = int(ExitCommandFailed)
	writeEnvelope(env)
	return withCode(ExitCommandFailed, errReported)
}

func writeEnvelope(env jsonEnvelope) {
	b, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		b = []byte(fmt.Sprintf(`{"success":false,"message":%q,"exitCode":1}`, err.Error()))
	}
	_, _ = fmt.Fprintln(stdout, string(b))
}
