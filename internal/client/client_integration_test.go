//go:build integration

package client_test

import (
	"testing"

	"github.com/utopia0107/unity-cli/internal/client"
)

func TestDiscoverInstance(t *testing.T) {
	inst, err := client.DiscoverInstance("")
	if err != nil {
		t.Fatalf("failed to discover Unity instance: %v", err)
	}
	if inst.Port == 0 {
		t.Error("expected non-zero port")
	}
	t.Logf("found Unity project: %s", inst.ProjectPath)
}

func TestSendExec(t *testing.T) {
	inst, err := client.DiscoverInstance("")
	if err != nil {
		t.Fatalf("failed to discover Unity instance: %v", err)
	}

	resp, err := client.Send(inst, "exec", map[string]interface{}{
		"code": "return (1 + 1).ToString();",
	}, 5000)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success, got: %s", resp.Message)
	}
	t.Logf("response: %s", resp.Data)
}

func TestSendConsole(t *testing.T) {
	inst, err := client.DiscoverInstance("")
	if err != nil {
		t.Fatalf("failed to discover Unity instance: %v", err)
	}

	resp, err := client.Send(inst, "console", map[string]interface{}{
		"lines": 5,
	}, 5000)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success, got: %s", resp.Message)
	}
	t.Logf("console response: %s", resp.Data)
}

func TestSendUnknownCommand(t *testing.T) {
	inst, err := client.DiscoverInstance("")
	if err != nil {
		t.Fatalf("failed to discover Unity instance: %v", err)
	}

	resp, err := client.Send(inst, "nonexistent_command", map[string]interface{}{}, 5000)
	if err != nil {
		t.Fatalf("unexpected connection error: %v", err)
	}
	if resp.Success {
		t.Error("expected failure for unknown command")
	}
}
