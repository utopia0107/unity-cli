package cmd

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/utopia0107/unity-cli/internal/client"
)

func TestExitCodeFor(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ExitCode
	}{
		{"nil is success", nil, ExitOK},
		{"untagged error is command failure", errors.New("boom"), ExitCommandFailed},
		{"usage", withCode(ExitUsage, errors.New("bad flag")), ExitUsage},
		{"connection tag", withCode(ExitConnection, errors.New("no Unity instances running")), ExitConnection},
		{"version mismatch", withCode(ExitVersionMismatch, errors.New("mismatch")), ExitVersionMismatch},
		{"timeout", withCode(ExitTimeout, errors.New("timed out")), ExitTimeout},
		{"untagged ErrConnection classifies as connection", fmt.Errorf("send: %w", client.ErrConnection), ExitConnection},
		{"outermost tag wins over inner ErrConnection", withCode(ExitTimeout, fmt.Errorf("timed out: %w", client.ErrConnection)), ExitTimeout},
		{"wrapped coded error survives", fmt.Errorf("context: %w", withCode(ExitUsage, errors.New("bad"))), ExitUsage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCodeFor(tt.err); got != tt.want {
				t.Errorf("ExitCodeFor(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}

func TestWithCodeNilStaysNil(t *testing.T) {
	if withCode(ExitUsage, nil) != nil {
		t.Fatal("withCode(nil) should stay nil")
	}
}

func TestIsReported(t *testing.T) {
	if IsReported(errors.New("plain")) {
		t.Fatal("plain error should not be reported")
	}
	if !IsReported(withCode(ExitCommandFailed, errReported)) {
		t.Fatal("errReported wrapped in CodedError should be detected")
	}
}

func TestCheckConnectorVersionMismatchTagsExitCode(t *testing.T) {
	err := checkConnectorVersion(&client.Instance{ConnectorVersion: "0.1.0"}, "v0.2.0", false)
	if err == nil {
		t.Fatal("expected version mismatch error")
	}
	if got := ExitCodeFor(err); got != ExitVersionMismatch {
		t.Fatalf("exit code: got %d, want %d", got, ExitVersionMismatch)
	}

	err = checkConnectorVersion(&client.Instance{}, "v0.2.0", false)
	if err == nil {
		t.Fatal("expected unknown connector version error")
	}
	if got := ExitCodeFor(err); got != ExitVersionMismatch {
		t.Fatalf("exit code: got %d, want %d", got, ExitVersionMismatch)
	}
}

func TestTestCmdInvalidModeTagsUsage(t *testing.T) {
	_, err := testCmd([]string{"--mode", "Nope"}, nil, nil)
	if err == nil {
		t.Fatal("expected usage error")
	}
	if got := ExitCodeFor(err); got != ExitUsage {
		t.Fatalf("exit code: got %d, want %d", got, ExitUsage)
	}
}

func TestEditorCmdUnknownActionTagsUsage(t *testing.T) {
	_, err := editorCmd([]string{"fly"}, nil, nil)
	if err == nil {
		t.Fatal("expected usage error")
	}
	if got := ExitCodeFor(err); got != ExitUsage {
		t.Fatalf("exit code: got %d, want %d", got, ExitUsage)
	}
}

func TestResolveReadyTimeoutTagsExitTimeout(t *testing.T) {
	origPoll := statusPollInterval
	statusPollInterval = time.Millisecond
	t.Cleanup(func() { statusPollInterval = origPoll })

	_, err := resolveReady(func() (*client.Instance, error) {
		return nil, errors.New("no Unity instances running")
	}, 5)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := ExitCodeFor(err); got != ExitTimeout {
		t.Fatalf("exit code: got %d, want %d", got, ExitTimeout)
	}
}
