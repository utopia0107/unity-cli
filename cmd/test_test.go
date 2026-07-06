package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/utopia0107/unity-cli/internal/client"
)

func TestTestCmd_ForwardsDirtySceneOptions(t *testing.T) {
	var captured map[string]interface{}
	send := func(cmd string, params interface{}) (*client.CommandResponse, error) {
		if cmd != "run_tests" {
			t.Fatalf("send called with command %q, want run_tests", cmd)
		}
		var ok bool
		captured, ok = params.(map[string]interface{})
		if !ok {
			t.Fatalf("params type = %T, want map[string]interface{}", params)
		}
		return &client.CommandResponse{Success: true}, nil
	}

	resp, err := testCmd([]string{"--allow-dirty-scenes", "--auto-save-scenes"}, send, nil)
	if err != nil {
		t.Fatalf("testCmd returned error: %v", err)
	}
	if resp == nil || !resp.Success {
		t.Fatalf("testCmd response = %#v, want success", resp)
	}
	if captured["allow_dirty_scenes"] != true {
		t.Errorf("allow_dirty_scenes = %v, want true", captured["allow_dirty_scenes"])
	}
	if captured["auto_save_scenes"] != true {
		t.Errorf("auto_save_scenes = %v, want true", captured["auto_save_scenes"])
	}
	if captured["run_id"] == "" {
		t.Error("run_id should be sent")
	}
}

func TestPollTestResultsStopsWhenProjectInstanceDisappears(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	origPollInterval := statusPollInterval
	statusPollInterval = time.Millisecond
	t.Cleanup(func() { statusPollInterval = origPollInterval })

	_, err := pollTestResults("missing", func() (*client.Instance, error) {
		return nil, fmt.Errorf("no Unity instance found for project: /projects/current")
	}, time.Now().Add(10*time.Second))
	if err == nil {
		t.Fatal("expected stopped editor error")
	}
	if !strings.Contains(err.Error(), "unity editor has stopped") {
		t.Fatalf("expected stopped editor error, got %v", err)
	}
}

func TestPollTestResultsHonorsDeadline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	start := time.Now()
	_, err := pollTestResults("missing", func() (*client.Instance, error) {
		return &client.Instance{State: "ready"}, nil
	}, time.Now().Add(700*time.Millisecond))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := ExitCodeFor(err); got != ExitTimeout {
		t.Fatalf("exit code: got %d, want %d", got, ExitTimeout)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("deadline not honored, took %s", elapsed)
	}
}

func TestWaitForReadyHonorsDeadline(t *testing.T) {
	origPoll := statusPollInterval
	statusPollInterval = time.Millisecond
	t.Cleanup(func() { statusPollInterval = origPoll })

	_, err := waitForReady(func() (*client.Instance, error) {
		return &client.Instance{State: "compiling"}, nil
	}, time.Now().Add(20*time.Millisecond))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if got := ExitCodeFor(err); got != ExitTimeout {
		t.Fatalf("exit code: got %d, want %d", got, ExitTimeout)
	}
}

func TestOperationDeadlineUsesFallbackWhenTimeoutUnset(t *testing.T) {
	origSet := flagTimeoutSet
	flagTimeoutSet = false
	t.Cleanup(func() { flagTimeoutSet = origSet })

	deadline := operationDeadline(5 * time.Minute)
	if remaining := time.Until(deadline); remaining < 4*time.Minute {
		t.Fatalf("expected ~5m fallback window, got %s", remaining)
	}
}

func TestOperationDeadlineUsesExplicitTimeout(t *testing.T) {
	origSet := flagTimeoutSet
	origTimeout := flagTimeout
	flagTimeoutSet = true
	flagTimeout = 1000
	t.Cleanup(func() {
		flagTimeoutSet = origSet
		flagTimeout = origTimeout
	})

	deadline := operationDeadline(5 * time.Minute)
	if remaining := time.Until(deadline); remaining > 2*time.Second {
		t.Fatalf("expected ~1s explicit window, got %s", remaining)
	}
}

func TestTestCmd_PlayModePollsRunIDResult(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	statusDir := filepath.Join(home, ".unity-cli", "status")
	if err := os.MkdirAll(statusDir, 0755); err != nil {
		t.Fatalf("failed to create status dir: %v", err)
	}

	send := func(cmd string, params interface{}) (*client.CommandResponse, error) {
		captured := params.(map[string]interface{})
		runID := captured["run_id"].(string)
		resp := client.CommandResponse{Success: true, Message: "done"}
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("failed to marshal response: %v", err)
		}
		if err := os.WriteFile(filepath.Join(statusDir, "test-results-"+runID+".json"), data, 0644); err != nil {
			t.Fatalf("failed to write results: %v", err)
		}
		return &client.CommandResponse{Success: true, Message: "running"}, nil
	}

	resp, err := testCmd([]string{"--mode", "PlayMode"}, send, nil)
	if err != nil {
		t.Fatalf("testCmd returned error: %v", err)
	}
	if resp.Message != "done" {
		t.Fatalf("Message = %q, want done", resp.Message)
	}
}
