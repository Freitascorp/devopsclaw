package tools

import (
	"strings"
	"testing"
)

func TestEnrichErrorOutput_TerraformLabelsBlock(t *testing.T) {
	output := `
╷
│ Error: Unsupported argument
│
│   on main.tf line 85, in resource "docker_container" "services":
│   85:   labels = merge(
│
│ An argument named "labels" is not expected here. Did you mean to define a
│ block of type "labels"?
╵

Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "HINT (Terraform Docker provider v3+)") {
		t.Errorf("Expected Docker provider v3+ hint for labels error, got: %s", result)
	}
	if !strings.Contains(result, `dynamic "labels"`) {
		t.Errorf("Expected dynamic block suggestion, got: %s", result)
	}
	// Original output should still be present
	if !strings.Contains(result, "Unsupported argument") {
		t.Errorf("Expected original error to be preserved, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformPortsBlock(t *testing.T) {
	output := `An argument named "ports" is not expected here. Did you mean to define a block of type "ports"?
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "HINT (Terraform Docker provider v3+)") {
		t.Errorf("Expected Docker provider v3+ hint for ports error, got: %s", result)
	}
	if !strings.Contains(result, "ports {") {
		t.Errorf("Expected ports block suggestion, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformVolumesBlock(t *testing.T) {
	output := `An argument named "volumes" is not expected here. Did you mean to define a block of type "volumes"?
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "HINT (Terraform Docker provider v3+)") {
		t.Errorf("Expected Docker provider v3+ hint for volumes error, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformGenericArgToBlock(t *testing.T) {
	output := `An argument named "env" is not expected here. Did you mean to define a block of type "env"?
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "HINT (Terraform)") {
		t.Errorf("Expected generic Terraform hint, got: %s", result)
	}
	if !strings.Contains(result, "dynamic") {
		t.Errorf("Expected dynamic block suggestion in generic hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformInit(t *testing.T) {
	output := `This configuration requires provider registry.terraform.io/kreuzwerker/docker
Please run "terraform init".
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, `Run "terraform init"`) {
		t.Errorf("Expected terraform init hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformStateLock(t *testing.T) {
	output := `Error acquiring the state lock
Lock Info:
  ID:        12345
  Path:      terraform.tfstate
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "force-unlock") {
		t.Errorf("Expected force-unlock hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_DockerDaemonNotRunning(t *testing.T) {
	output := `Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "Docker daemon is not running") {
		t.Errorf("Expected Docker daemon hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_DockerPortConflict(t *testing.T) {
	output := `Bind for 0.0.0.0:8080 failed: port is already allocated
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "port is already in use") {
		t.Errorf("Expected port conflict hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_CommandNotFound(t *testing.T) {
	output := `sh: terraform: command not found
Exit code: exit status 127`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "Command not found") {
		t.Errorf("Expected command not found hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_PermissionDenied(t *testing.T) {
	output := `/usr/local/bin/deploy.sh: permission denied
Exit code: exit status 126`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "Permission denied") {
		t.Errorf("Expected permission denied hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_NoMatch(t *testing.T) {
	output := `Some random error that doesn't match any pattern
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	// Should return output unchanged when no patterns match
	if result != output {
		t.Errorf("Expected output unchanged when no hints match.\nOriginal: %s\nGot: %s", output, result)
	}
}

func TestEnrichErrorOutput_EmptyString(t *testing.T) {
	result := enrichErrorOutput("")
	if result != "" {
		t.Errorf("Expected empty string for empty input, got: %s", result)
	}
}

func TestEnrichErrorOutput_NoDuplicateHints(t *testing.T) {
	// The labels error matches both the specific Docker labels hint and the
	// generic arg-to-block hint. Ensure no duplicate hints are emitted.
	output := `An argument named "labels" is not expected here. Did you mean to define a block of type "labels"?`

	result := enrichErrorOutput(output)

	// Count occurrences of "HINT"
	hintCount := strings.Count(result, "HINT")
	if hintCount > 2 {
		t.Errorf("Expected at most 2 hints (specific + generic), got %d in: %s", hintCount, result)
	}
}

func TestEnrichErrorOutput_KubectlConnection(t *testing.T) {
	output := `The connection to the server localhost:6443 was refused - did you specify the right host or port?
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "kubectl cannot reach the cluster") {
		t.Errorf("Expected kubectl connection hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_TerraformDeprecatedInterpolation(t *testing.T) {
	output := `Warning: Interpolation-only expressions are deprecated
Exit code: exit status 0`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "Remove the surrounding") {
		t.Errorf("Expected interpolation deprecation hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_HelmReleaseNotFound(t *testing.T) {
	output := `Error: release: "my-app" not found
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	if !strings.Contains(result, "helm list -A") {
		t.Errorf("Expected helm release hint, got: %s", result)
	}
}

func TestEnrichErrorOutput_PreservesOriginalOutput(t *testing.T) {
	output := `Cannot connect to the Docker daemon at unix:///var/run/docker.sock.
Exit code: exit status 1`

	result := enrichErrorOutput(output)

	// The full original output should appear before the hints
	if !strings.HasPrefix(result, output) {
		t.Errorf("Expected enriched output to start with original output")
	}
	// Hints should be separated by "---"
	if strings.Contains(result, "HINT") && !strings.Contains(result, "---") {
		t.Errorf("Expected hints to be separated by '---' divider")
	}
}
