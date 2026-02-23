# DevOpsClaw Pro: Production-Ready Architecture

> Answering every criticism in "DevOpsClaw and OpenClaw Are Not Infrastructure" — 
> without abandoning the lightweight DNA that made DevOpsClaw viral.

---

## Executive Summary

DevOpsClaw Pro transforms DevOpsClaw from a **single-machine personal assistant** into 
a **fleet-aware automation platform** while keeping the single-binary, low-resource 
philosophy. It adds six new packages that address every gap identified in the critique:

| Criticism | DevOpsClaw Pro Answer | Package |
|-----------|-------------------|---------|
| Single-machine only | Fleet management + fan-out execution | `pkg/fleet` |
| No NAT traversal | Outbound relay tunnels (no port forwarding) | `pkg/relay` |
| `map[string]interface{}` tools | Typed, validated, versioned tool contracts | `pkg/contracts` |
| No RBAC or audit | Role-based access control + audit log | `pkg/rbac` |
| No retries/circuit breakers | Full resilience pipeline | `pkg/resilience` |
| No metrics/tracing | Prometheus metrics + structured tracing | `pkg/observability` |
| No browser automation | Browser tool contracts (Playwright/CDP) | `pkg/contracts` |

**All 30 tests pass. All 6 packages compile. Zero external dependencies added.**

---

## Architecture: Before vs After

### Before (DevOpsClaw)

```
[Telegram / Discord / WhatsApp]
               │
               ▼
      [Single Agent Machine]   ← everything runs here
               │
               ▼
           [Cloud LLM]
```

### After (DevOpsClaw Pro)

```
[CLI / Telegram / Discord / Slack / Web API]
                    │
                    ▼
              [Typed SDK]
                    │
                    ▼
     ┌──── [Control Plane] ─────┐
     │    ┌─────────────────┐   │
     │    │  Relay Server   │   │      ← NAT-safe connectivity
     │    │  RBAC Enforcer  │   │      ← permission checks
     │    │  Fleet Executor │   │      ← fan-out orchestration
     │    │  Metrics/Traces │   │      ← observability
     │    │  Audit Logger   │   │      ← compliance
     │    │  Resilience     │   │      ← circuit breakers, retries
     │    └─────────────────┘   │
     └──────────┬───────────────┘
     ┌──────────┼───────────┐
     ▼          ▼           ▼
  [Node A]  [Node B]   [Node N]       ← fleet nodes
  devopsclaw  devopsclaw   devopsclaw          (outbound connection)
   agent     agent      agent
```

---

## Package Guide

### 1. `pkg/fleet` — Fleet Management

**Files:** `types.go`, `executor.go`, `node_manager.go`, `store_memory.go`

The fleet package introduces multi-machine orchestration:

- **Node** — Registered machine with hostname, labels, groups, capabilities, resources
- **NodeManager** — Registration, deregistration, heartbeat tracking, GC for stale nodes
- **TargetSelector** — Select nodes by ID, group, labels, or `all`; with `MaxConcurrency` and `MaxNodes` limits
- **Executor** — Fan-out commands with concurrency control, timeout enforcement, result aggregation
- **Store** — Pluggable persistence (memory for dev, SQLite/Postgres for prod)

**Typed commands** instead of raw strings:
```go
// Instead of: args map[string]interface{}
type ShellCommand struct {
    Command    string            `json:"command"`
    WorkDir    string            `json:"work_dir,omitempty"`
    Env        map[string]string `json:"env,omitempty"`
    TimeoutSec int              `json:"timeout_sec,omitempty"`
}

type DeployCommand struct {
    Service    string `json:"service"`
    Version    string `json:"version"`
    Strategy   string `json:"strategy"` // "rolling", "blue-green", "canary"
}
```

**Usage:**
```go
executor := fleet.NewExecutor(store, relayClient, logger)

result, err := executor.Execute(ctx, &fleet.ExecRequest{
    ID:     "deploy-v2.1",
    Target: fleet.TargetSelector{Groups: []fleet.GroupName{"prod-web"}},
    Command: fleet.TypedCommand{Type: "shell", Data: shellJSON},
    Timeout: 30 * time.Second,
    Requester: "ops-team",
})

// result.Summary.Total = 12, .Success = 11, .Failed = 1
```

---

### 2. `pkg/relay` — NAT-Traversal Relay

**File:** `relay.go`

Solves the #1 problem with "control your server from Telegram": NAT traversal.

- **Server** — Accepts outbound connections from fleet nodes, maintains tunnel registry
- **Agent** — Runs on each node, connects outbound (NAT-friendly), auto-reconnects
- **TunnelRelayClient** — Implements `fleet.RelayClient` by routing commands through tunnels

**How it works:**
```
Node behind NAT ──outbound──► Relay Server ◄──command── Control Plane
                                   │
                              mTLS + auth
                              session mgmt
                              auto-reconnect
```

No port forwarding. No VPN. No "good luck." The node makes an outbound connection. Done.

---

### 3. `pkg/contracts` — Typed Tool Contracts

**File:** `contracts.go`

Replaces `map[string]interface{}` with compile-time type safety using Go generics:

```go
type ToolContract[Req any, Resp any] struct {
    ToolName    string
    Validate    func(req *Req) error
    Execute     func(req *Req) (*Resp, error)
}
```

**Pre-defined contracts for every operation category:**

| Category | Request Type | Response Type |
|----------|-------------|---------------|
| Shell | `ShellExecRequest` | `ShellExecResponse` |
| File Read | `FileReadRequest` | `FileReadResponse` |
| File Write | `FileWriteRequest` | `FileWriteResponse` |
| Deploy | `DeployRequest` | `DeployResponse` |
| Docker | `DockerExecRequest` | `DockerExecResponse` |
| Kubernetes | `K8sRequest` | `K8sResponse` |
| Browser | `BrowserRequest` | `BrowserResponse` |
| Fleet | `FleetExecRequest` | `FleetExecResponse` |

**The bridge to the LLM:**
```go
registry := contracts.NewRegistry()
contracts.Register(registry, shellContract, shellMeta)

// LLM sends raw JSON tool call → typed execution → typed response → JSON back to LLM
result, err := registry.Execute("shell_exec", rawJSON)
```

This means:
- Inputs are validated before execution
- Outputs have guaranteed schemas
- Versioning is tracked per tool
- No more "JSON roulette"

---

### 4. `pkg/rbac` — Role-Based Access Control

**File:** `rbac.go`

Deny-by-default permission system with audit logging:

**Pre-defined roles:**
| Role | Permissions |
|------|------------|
| `admin` | Everything (`admin:*`) |
| `operator` | Fleet exec, deploy, shell, docker, k8s, browser, cron |
| `viewer` | Read-only: fleet view, file read, docker/k8s view, audit |
| `agent` | AI agent tool permissions: fleet view/exec, shell, files |

**40+ granular permissions:**
```go
PermFleetExec, PermFleetDeploy, PermFleetManage,
PermShellExec, PermShellExecSudo,
PermFileRead, PermFileWrite, PermFileDelete,
PermDockerView, PermDockerManage,
PermK8sView, PermK8sManage,
PermBrowserExec,
...
```

**Scope restrictions** limit users to specific node groups:
```go
enforcer.RegisterUser(&rbac.User{
    ID:    "junior-ops",
    Roles: []rbac.RoleName{"operator"},
    Scopes: []rbac.ResourceScope{
        {NodeGroups: []string{"staging"}}, // can only touch staging
    },
    ChannelIDs: map[string]string{
        "telegram": "12345",
        "discord":  "67890",
    },
})
```

**Cross-channel identity:** Same user across Telegram + Discord + Slack is resolved to one identity.

**Full audit trail:** Every allow/deny decision is logged with timestamp, user, permission, resource, and reason.

---

### 5. `pkg/resilience` — Production Reliability

**File:** `resilience.go`

The "boring features" that keep systems alive at 3am:

| Primitive | What It Does |
|-----------|-------------|
| **CircuitBreaker** | Stops calling failing services; auto-recovers via half-open state |
| **Retry** | Exponential backoff with jitter; configurable max attempts; retriable error classification |
| **RateLimiter** | Token bucket per-user/per-provider; burst support |
| **Bulkhead** | Concurrency limiter prevents resource exhaustion |
| **IdempotencyController** | Prevents duplicate execution of the same command |
| **WithTimeout** | Deadline enforcement for any operation |
| **Pipeline** | Composes all primitives into a single execution wrapper |

**Composed pipeline:**
```go
pipeline := resilience.NewPipeline(logger,
    resilience.WithRateLimit(rateLimiter),
    resilience.WithBulkhead(bulkhead),
    resilience.WithCircuitBreaker(circuitBreaker),
    resilience.WithRetry(resilience.RetryConfig{
        MaxAttempts: 3,
        InitialDelay: 100 * time.Millisecond,
        Multiplier: 2.0,
    }),
    resilience.WithPipelineTimeout(30 * time.Second),
)

err := pipeline.Execute(ctx, func(ctx context.Context) error {
    return provider.Chat(ctx, messages, tools, model, opts)
})
```

Execution order: rate limit → bulkhead → circuit breaker → retry → timeout → fn

---

### 6. `pkg/observability` — Metrics, Tracing, History

**File:** `observability.go`

**Prometheus-compatible metrics:**
```
devopsclaw_messages_received_total
devopsclaw_llm_calls_total
devopsclaw_llm_latency_seconds (histogram)
devopsclaw_tool_calls_total
devopsclaw_fleet_nodes_online
devopsclaw_fleet_exec_total
devopsclaw_circuit_breaker_trips_total
devopsclaw_rate_limit_rejects_total
...
```

Exposed via `/metrics` HTTP endpoint, scrapable by Prometheus.

**Structured tracing:**
```go
ctx, span := tracer.StartSpan(ctx, "fleet.execute", map[string]string{
    "target": "prod-web",
    "command": "deploy",
})
defer tracer.EndSpan(span, err)
```

Spans support parent-child relationships, events, and queryable history.

**Task history** — Every agent action is recorded for replay and debugging:
```go
taskHistory.Record(&observability.TaskRecord{
    ID:      "task-123",
    TraceID: span.TraceID,
    UserID:  "ops-team",
    Action:  "fleet_exec",
    Input:   requestJSON,
    Output:  resultJSON,
})
```

---

## Addressing Every Critique Point-by-Point

### "Single-machine trap" → Fleet Executor + Node Manager
Commands fan out to dozens/hundreds of nodes with concurrency control, timeout enforcement, and aggregated results. Nodes are organized by groups and labels.

### "NAT traversal: good luck" → Relay Server
Nodes make outbound connections. No port forwarding. No VPN. Auto-reconnect with exponential backoff.

### "No browser automation" → Browser Contracts
Typed `BrowserRequest`/`BrowserResponse` contracts with actions: navigate, click, type, screenshot, evaluate, wait_for, extract. Session persistence with cookies.

### "map[string]interface{} tools" → Typed Contracts
Go generics enforce compile-time type safety. Every tool has validated input, guaranteed output schema, and version tracking.

### "No RBAC" → Full Role-Based Access Control
4 built-in roles, 40+ permissions, scope restrictions, cross-channel identity resolution, full audit trail.

### "No retries/circuit breakers" → Resilience Pipeline
Circuit breakers, exponential backoff retry, token bucket rate limiting, bulkheads, idempotency control. Composable pipeline.

### "No observability" → Prometheus Metrics + Tracing
30+ metrics, histogram latency tracking, structured spans, task history for replay.

### "Stars ≠ Production" → Tests + Audit + Safety
30 tests across 3 packages. Every permission check is audited. Every fleet command is recorded. Type safety prevents "JSON roulette."

---

## What Stays the Same

DevOpsClaw Pro preserves everything that made DevOpsClaw great:

- **Single Go binary** — still compiles to one static binary
- **Low resource usage** — new packages are pure Go, zero external dependencies
- **12 channel integrations** — Telegram, Discord, Slack, WhatsApp, etc.
- **17+ LLM providers** — OpenAI, Claude, Gemini, etc.
- **Skill system** — workspace-based, installable from registries
- **Embedded hardware** — I2C/SPI tools for SBCs
- **Fun to use** — the "personal assistant" mode still works perfectly

The difference: now it *also* works when you need it to control 200 machines at 3am.

---

## Migration Path

DevOpsClaw Pro is **additive, not breaking**. Existing DevOpsClaw deployments work unchanged:

1. **Solo mode** (existing) — Single binary, local tools, chat channels. No fleet config = no fleet overhead.
2. **Fleet mode** (new) — Add `fleet` section to config, register nodes, use fleet tools.
3. **Control plane mode** (new) — Run relay server, connect nodes, use typed SDK.

```json
{
  "fleet": {
    "enabled": true,
    "relay": {
      "listen_addr": ":9443",
      "auth_token": "fleet-secret-xxx"
    },
    "rbac": {
      "enabled": true,
      "users": [...]
    }
  }
}
```

---

## File Map

```
pkg/
├── fleet/
│   ├── types.go           # Node, TargetSelector, TypedCommand, ExecResult, Store interface
│   ├── executor.go        # Fan-out execution with concurrency control
│   ├── node_manager.go    # Node lifecycle, health tracking, GC
│   ├── store_memory.go    # In-memory store (dev/test)
│   └── fleet_test.go      # 9 tests
├── relay/
│   └── relay.go           # Server (tunnel registry) + Agent (outbound connector)
├── contracts/
│   └── contracts.go       # Typed tool contracts with generic Registry
├── rbac/
│   ├── rbac.go            # Enforcer, roles, permissions, audit logger
│   └── rbac_test.go       # 9 tests
├── resilience/
│   ├── resilience.go      # CircuitBreaker, Retry, RateLimiter, Bulkhead, Pipeline
│   └── resilience_test.go # 12 tests
└── observability/
    └── observability.go   # Metrics, Tracer, TaskHistory, /metrics endpoint
```

---

## The Bottom Line

> "Infrastructure is not about what runs. It's about what survives."

DevOpsClaw Pro survives:
- **Unreliable networks** → relay with auto-reconnect
- **Partial failures** → circuit breakers + fallback chains
- **Unsafe execution** → RBAC + typed contracts + deny patterns
- **Auditability needs** → full audit trail on every action
- **Multi-user teams** → roles, scopes, cross-channel identity
- **Fleet scale** → fan-out with concurrency control
- **Accidental destruction** → idempotency + dry-run + validation
- **3am debugging** → metrics, traces, task history, structured logs

The $10 board still runs the agent. The cloud still runs the AI.  
But now there's real infrastructure in between.
