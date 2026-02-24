package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// errorHint pairs a compiled regex pattern with an actionable hint message
// that is appended to exec tool error output to guide the LLM toward a fix.
type errorHint struct {
	pattern *regexp.Regexp
	hint    string
}

// errorHints is the ordered list of known error patterns and their recovery
// hints. Patterns are tested in order; all matching hints are appended.
var errorHints = []errorHint{
	// ── Terraform / OpenTofu ────────────────────────────────────────

	// Docker provider v3+: labels changed from argument to block
	{
		pattern: regexp.MustCompile(`(?is)An argument named "labels" is not expected here.*Did you mean to define a\s*.*block of type "labels"\?`),
		hint: `HINT (Terraform Docker provider v3+): The "labels" attribute was changed from a map argument to a block type.
Replace:
  labels = merge(local.common_labels, { ... })
With a dynamic block:
  dynamic "labels" {
    for_each = merge(local.common_labels, { ... })
    content {
      label = labels.key
      value = labels.value
    }
  }
The same applies to "ports", "volumes", "upload", "env", "host", and "devices" blocks.`,
	},

	// Docker provider v3+: ports changed from argument to block
	{
		pattern: regexp.MustCompile(`(?is)An argument named "ports" is not expected here.*Did you mean to define a\s*.*block of type "ports"\?`),
		hint: `HINT (Terraform Docker provider v3+): The "ports" attribute was changed from a list argument to a block type.
Replace:
  ports = [{ internal = 80, external = 8080 }]
With:
  ports {
    internal = 80
    external = 8080
  }
Or use a dynamic block for variable port lists.`,
	},

	// Docker provider v3+: volumes changed from argument to block
	{
		pattern: regexp.MustCompile(`(?is)An argument named "volumes" is not expected here.*Did you mean to define a\s*.*block of type "volumes"\?`),
		hint: `HINT (Terraform Docker provider v3+): The "volumes" attribute was changed from a list argument to a block type.
Replace:
  volumes = [{ host_path = "/data", container_path = "/app/data" }]
With:
  volumes {
    host_path      = "/data"
    container_path = "/app/data"
  }
Or use a dynamic block for variable volume lists.`,
	},

	// Generic Terraform "argument not expected, did you mean block" pattern
	{
		pattern: regexp.MustCompile(`(?is)An argument named "\w+" is not expected here.*Did you mean to define a\s*.*block of type`),
		hint: `HINT (Terraform): This attribute was changed from an argument to a block type in a newer provider version.
Convert the assignment (attr = value) to a block syntax:
  attr {
    key1 = val1
    key2 = val2
  }
If the value is dynamic, use:
  dynamic "attr" {
    for_each = var.items
    content {
      key1 = attr.value.key1
      key2 = attr.value.key2
    }
  }`,
	},

	// Terraform init required
	{
		pattern: regexp.MustCompile(`(?i)Module not installed|provider registry\.terraform\.io/.+ is not available|This configuration requires provider|run "terraform init"`),
		hint: `HINT: Run "terraform init" (or "tofu init") first to download required providers and modules before running plan/apply.`,
	},

	// Terraform backend initialization
	{
		pattern: regexp.MustCompile(`(?i)Backend initialization required|backend configuration changed|run.*terraform init`),
		hint: `HINT: The Terraform backend configuration changed. Run "terraform init -reconfigure" or "terraform init -migrate-state" to reinitialize.`,
	},

	// Terraform state lock
	{
		pattern: regexp.MustCompile(`(?i)Error acquiring the state lock|Lock Info|state is already locked`),
		hint: `HINT: Terraform state is locked by another process. Wait for it to finish, or if you're certain no other process is running:
  terraform force-unlock <LOCK_ID>`,
	},

	// Terraform deprecated interpolation syntax
	{
		pattern: regexp.MustCompile(`(?i)Interpolation-only expressions are deprecated`),
		hint: `HINT: Remove the surrounding "${...}" wrapper. In Terraform 0.12+, use bare references:
  Replace: "${var.name}" → var.name`,
	},

	// ── Docker / Docker Compose ─────────────────────────────────────

	// Docker daemon not running
	{
		pattern: regexp.MustCompile(`(?i)Cannot connect to the Docker daemon|Is the docker daemon running|docker\.sock.*permission denied|connection refused.*docker`),
		hint: `HINT: Docker daemon is not running or inaccessible.
  macOS: open -a Docker (or start Docker Desktop)
  Linux: sudo systemctl start docker
  Permission: sudo usermod -aG docker $USER (then re-login)`,
	},

	// Docker compose version mismatch
	{
		pattern: regexp.MustCompile(`(?i)docker-compose.*not found|'compose' is not a docker command`),
		hint: `HINT: Docker Compose v2 uses "docker compose" (space, not hyphen). Try:
  docker compose up -d
Install if missing: https://docs.docker.com/compose/install/`,
	},

	// Docker port already allocated
	{
		pattern: regexp.MustCompile(`(?i)Bind for.*failed: port is already allocated|address already in use`),
		hint: `HINT: The port is already in use. Find the process:
  lsof -i :<PORT> (macOS/Linux) or netstat -ano | findstr <PORT> (Windows)
Then either stop it or change the port mapping.`,
	},

	// ── Kubernetes / kubectl ────────────────────────────────────────

	// kubectl context not set
	{
		pattern: regexp.MustCompile(`(?i)The connection to the server.*was refused|Unable to connect to the server|no configuration has been provided`),
		hint: `HINT: kubectl cannot reach the cluster. Check:
  kubectl config current-context
  kubectl cluster-info
Ensure kubeconfig is correct and the cluster is reachable.`,
	},

	// kubectl resource not found
	{
		pattern: regexp.MustCompile(`(?i)error: the server doesn't have a resource type`),
		hint: `HINT: The resource type is not available. Check if a CRD needs to be installed or if the API group is correct:
  kubectl api-resources | grep <resource>`,
	},

	// ── Ansible ─────────────────────────────────────────────────────

	// Ansible unreachable host
	{
		pattern: regexp.MustCompile(`(?i)UNREACHABLE!.*Failed to connect to the host`),
		hint: `HINT: Ansible cannot reach the target host. Check:
  1. SSH connectivity: ssh <user>@<host>
  2. Inventory file host/IP is correct
  3. SSH key or password is configured`,
	},

	// ── Helm ────────────────────────────────────────────────────────

	// Helm release not found
	{
		pattern: regexp.MustCompile(`(?i)Error: release:.*not found`),
		hint: `HINT: Helm release not found. Use "helm list -A" to see all releases. If upgrading, use "helm install" first.`,
	},

	// ── General ─────────────────────────────────────────────────────

	// Permission denied
	{
		pattern: regexp.MustCompile(`(?i)permission denied|EACCES`),
		hint: `HINT: Permission denied. Try running with sudo, or check file permissions with "ls -la".`,
	},

	// Command not found
	{
		pattern: regexp.MustCompile(`(?i)command not found|not recognized as.*command`),
		hint: `HINT: Command not found. Check if the tool is installed and in PATH:
  which <command> (macOS/Linux) or where <command> (Windows)`,
	},
}

// enrichErrorOutput appends actionable hints to error output from exec commands.
// Only called when the command exits with a non-zero status.
// Returns the original output with any matching hints appended.
func enrichErrorOutput(output string) string {
	if output == "" {
		return output
	}

	var hints []string
	seen := make(map[string]bool) // deduplicate hints

	for _, eh := range errorHints {
		if eh.pattern.MatchString(output) {
			if !seen[eh.hint] {
				hints = append(hints, eh.hint)
				seen[eh.hint] = true
			}
		}
	}

	if len(hints) == 0 {
		return output
	}

	return fmt.Sprintf("%s\n\n---\n%s", output, strings.Join(hints, "\n\n"))
}
