package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/youngwoocho02/unity-cli/internal/client"
)

// captureStdout redirects the package stdout writer for one test.
func captureStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	orig := stdout
	buf := &bytes.Buffer{}
	stdout = buf
	t.Cleanup(func() { stdout = orig })
	return buf
}

// withJSONMode enables --json for one test.
func withJSONMode(t *testing.T) {
	t.Helper()
	orig := flagJSON
	flagJSON = true
	t.Cleanup(func() { flagJSON = orig })
}

func decodeEnvelope(t *testing.T, buf *bytes.Buffer) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not a single JSON object: %v\noutput: %s", err, buf.String())
	}
	return env
}

func TestEmitEnvelopeSuccessWithData(t *testing.T) {
	buf := captureStdout(t)

	err := emitEnvelope(&client.CommandResponse{
		Success: true,
		Message: "OK",
		Data:    json.RawMessage(`{"value": 42}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, buf)
	if env["success"] != true {
		t.Errorf("success: got %v, want true", env["success"])
	}
	if env["message"] != "OK" {
		t.Errorf("message: got %v, want OK", env["message"])
	}
	if env["exitCode"] != float64(0) {
		t.Errorf("exitCode: got %v, want 0", env["exitCode"])
	}
	if _, hasError := env["error"]; hasError {
		t.Error("success envelope should not contain error field")
	}
	data, ok := env["data"].(map[string]interface{})
	if !ok || data["value"] != float64(42) {
		t.Errorf("data: got %v, want {value: 42}", env["data"])
	}
}

func TestEmitEnvelopeNullDataOmitted(t *testing.T) {
	buf := captureStdout(t)

	if err := emitEnvelope(&client.CommandResponse{Success: true, Message: "done", Data: json.RawMessage("null")}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, buf)
	if _, hasData := env["data"]; hasData {
		t.Errorf("null data should be omitted, got %v", env["data"])
	}
}

func TestEmitEnvelopeCommandFailure(t *testing.T) {
	buf := captureStdout(t)

	err := emitEnvelope(&client.CommandResponse{Success: false, Message: "tool exploded"})
	if err == nil {
		t.Fatal("expected reported error")
	}
	if !IsReported(err) {
		t.Fatal("failure should be marked reported")
	}
	if got := ExitCodeFor(err); got != ExitCommandFailed {
		t.Fatalf("exit code: got %d, want %d", got, ExitCommandFailed)
	}

	env := decodeEnvelope(t, buf)
	if env["success"] != false {
		t.Errorf("success: got %v, want false", env["success"])
	}
	if env["exitCode"] != float64(1) {
		t.Errorf("exitCode: got %v, want 1", env["exitCode"])
	}
	errObj, ok := env["error"].(map[string]interface{})
	if !ok || errObj["class"] != "command_failed" {
		t.Errorf("error.class: got %v, want command_failed", env["error"])
	}
}

func TestFinishErrEmitsConnectionEnvelope(t *testing.T) {
	buf := captureStdout(t)
	withJSONMode(t)

	inErr := withCode(ExitConnection, errors.New("no Unity instances running"))
	err := finishErr(inErr)
	if err == nil || !IsReported(err) {
		t.Fatalf("expected reported error, got %v", err)
	}
	if got := ExitCodeFor(err); got != ExitConnection {
		t.Fatalf("exit code: got %d, want %d", got, ExitConnection)
	}

	env := decodeEnvelope(t, buf)
	errObj, ok := env["error"].(map[string]interface{})
	if !ok || errObj["class"] != "connection" {
		t.Errorf("error.class: got %v, want connection", env["error"])
	}
	if env["exitCode"] != float64(3) {
		t.Errorf("exitCode: got %v, want 3", env["exitCode"])
	}
	if !strings.Contains(env["message"].(string), "no Unity instances running") {
		t.Errorf("message should carry the cause, got %v", env["message"])
	}
}

func TestFinishErrPassthroughInTextMode(t *testing.T) {
	buf := captureStdout(t)

	inErr := withCode(ExitTimeout, errors.New("timed out"))
	if err := finishErr(inErr); !errors.Is(err, inErr) && err != inErr {
		t.Fatalf("text mode should pass the error through, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("text mode must not write an envelope, got %s", buf.String())
	}
}

func TestFinishErrSkipsAlreadyReported(t *testing.T) {
	buf := captureStdout(t)
	withJSONMode(t)

	inErr := withCode(ExitCommandFailed, errReported)
	_ = finishErr(inErr)
	if buf.Len() != 0 {
		t.Fatalf("reported errors must not emit a second envelope, got %s", buf.String())
	}
}

func TestErrorClassMatchesExitCodes(t *testing.T) {
	want := map[ExitCode]string{
		ExitCommandFailed:   "command_failed",
		ExitUsage:           "usage",
		ExitConnection:      "connection",
		ExitVersionMismatch: "version_mismatch",
		ExitTimeout:         "timeout",
	}
	for code, class := range want {
		if got := errorClass(code); got != class {
			t.Errorf("errorClass(%d) = %q, want %q", code, got, class)
		}
	}
}

func TestStatusJSONSuccessShape(t *testing.T) {
	buf := captureStdout(t)
	withJSONMode(t)
	origVersion := Version
	Version = "dev"
	t.Cleanup(func() { Version = origVersion })

	instances := []client.Instance{{
		State:       "ready",
		ProjectPath: "/projects/current",
		Port:        8090,
		PID:         100,
		Timestamp:   timeNowMilli(),
	}}
	if err := statusJSON(instances, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, buf)
	if env["success"] != true {
		t.Fatalf("success: got %v, want true", env["success"])
	}
	list, ok := env["data"].([]interface{})
	if !ok || len(list) != 1 {
		t.Fatalf("data: got %v, want 1 instance", env["data"])
	}
	entry := list[0].(map[string]interface{})
	if entry["projectPath"] != "/projects/current" {
		t.Errorf("projectPath: got %v", entry["projectPath"])
	}
	if entry["responding"] != true {
		t.Errorf("responding: got %v, want true", entry["responding"])
	}
}

func TestStatusJSONVersionMismatch(t *testing.T) {
	buf := captureStdout(t)
	withJSONMode(t)
	origVersion := Version
	Version = "v9.9.9"
	t.Cleanup(func() { Version = origVersion })

	instances := []client.Instance{{
		State:            "ready",
		ProjectPath:      "/projects/current",
		ConnectorVersion: "0.0.1",
		Timestamp:        timeNowMilli(),
	}}
	err := statusJSON(instances, false)
	if err == nil || !IsReported(err) {
		t.Fatalf("expected reported mismatch error, got %v", err)
	}
	if got := ExitCodeFor(err); got != ExitVersionMismatch {
		t.Fatalf("exit code: got %d, want %d", got, ExitVersionMismatch)
	}

	env := decodeEnvelope(t, buf)
	errObj := env["error"].(map[string]interface{})
	if errObj["class"] != "version_mismatch" {
		t.Errorf("error.class: got %v, want version_mismatch", errObj["class"])
	}
	if _, hasData := env["data"]; !hasData {
		t.Error("mismatch envelope should still carry instance data")
	}
}

func timeNowMilli() int64 {
	return time.Now().UnixMilli()
}
