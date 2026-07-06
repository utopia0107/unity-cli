package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/youngwoocho02/unity-cli/internal/client"
)

type instanceResolver func() (*client.Instance, error)

var statusPollInterval = 500 * time.Millisecond

func statusCmd(project string, ignoreVersionMismatch bool) error {
	instances, err := statusInstances(project)
	if err != nil {
		return err
	}
	if len(instances) == 0 {
		return fmt.Errorf("no Unity instances running")
	}
	for i := range instances {
		if i > 0 {
			fmt.Println()
		}
		printStatus(&instances[i])
		if err := checkConnectorVersion(&instances[i], Version, ignoreVersionMismatch); err != nil {
			return err
		}
	}
	return nil
}

func statusInstances(project string) ([]client.Instance, error) {
	instances, err := client.ScanInstances()
	if err != nil {
		return nil, fmt.Errorf("no Unity instances found")
	}

	var result []client.Instance
	projectNorm := client.NormalizeProjectPath(project)
	for _, inst := range instances {
		if inst.Timestamp <= 0 {
			continue
		}
		if projectNorm != "" {
			instNorm := client.NormalizeProjectPath(inst.ProjectPath)
			if instNorm != projectNorm && !strings.Contains(instNorm, projectNorm) {
				continue
			}
		}
		result = append(result, inst)
	}
	if project != "" && len(result) == 0 {
		return nil, fmt.Errorf("no Unity instance found for project: %s", project)
	}
	return result, nil
}

func printStatus(status *client.Instance) {
	age := time.Since(time.UnixMilli(status.Timestamp))
	if age > 3*time.Second {
		fmt.Fprintf(os.Stderr, "Unity: not responding (last heartbeat %s ago)\n", age.Truncate(time.Second))
		return
	}

	fmt.Printf("Unity: %s\n", status.State)
	fmt.Printf("  Project: %s\n", status.ProjectPath)
	fmt.Printf("  Version: %s\n", status.UnityVersion)
	fmt.Printf("  Connector: %s\n", connectorVersionLabel(status.ConnectorVersion))
	fmt.Printf("  PID:     %d\n", status.PID)
}

func connectorVersionLabel(version string) string {
	if strings.TrimSpace(version) == "" {
		return "unknown"
	}
	return version
}

func checkConnectorVersion(inst *client.Instance, cliVersion string, ignoreMismatch bool) error {
	if normalizeVersion(cliVersion) == "dev" {
		return nil
	}
	if ignoreMismatch {
		return nil
	}
	if inst == nil {
		return nil
	}

	connectorVersion := strings.TrimSpace(inst.ConnectorVersion)
	if connectorVersion == "" {
		return withCode(ExitVersionMismatch, fmt.Errorf("connector version is unknown; update the Unity Connector package to match unity-cli %s, or rerun with --ignore-version-mismatch", cliVersion))
	}
	if normalizeVersion(connectorVersion) != normalizeVersion(cliVersion) {
		return withCode(ExitVersionMismatch, fmt.Errorf("connector version mismatch: unity-cli %s, connector %s. Update both to the same release, or rerun with --ignore-version-mismatch", cliVersion, connectorVersion))
	}
	return nil
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return version
}

// waitForReady polls until the heartbeat state becomes "ready" or the deadline
// passes. Returns true if compilation had errors.
func waitForReady(resolve instanceResolver, deadline time.Time) (bool, error) {
	fmt.Fprintf(os.Stderr, "Waiting for compilation...\n")

	window := time.Until(deadline).Round(time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(statusPollInterval)
		status, err := resolve()
		if err != nil {
			continue
		}
		if status.State == "ready" {
			if status.CompileErrors {
				fmt.Fprintf(os.Stderr, "Compilation finished with errors.\n")
			} else {
				fmt.Fprintf(os.Stderr, "Compilation complete.\n")
			}
			return status.CompileErrors, nil
		}
	}

	return false, withCode(ExitTimeout, fmt.Errorf("timed out waiting for compilation (%s)", window))
}
