````skill
---
name: coding
description: "Write, review, debug, refactor, and explain code using AI coding assistants (Claude, Gemini, Codex). Multi-language support: Python, Go, TypeScript, Rust, Java, C, shell scripts, and more. Use for code generation, pair programming, code review, test writing, and architecture guidance."
metadata: {"nanobot":{"emoji":"💻"}}
---

# Coding Skill

Leverage AI coding models (Claude, Gemini, Codex) for software engineering tasks. This skill provides workflows for code generation, debugging, refactoring, review, testing, and architecture.

## Model Selection

Choose the right model for the task:

| Task | Recommended Model | Why |
|------|-------------------|-----|
| Complex architecture, nuanced refactoring | Claude (Opus/Sonnet) | Strong reasoning, follows constraints |
| Large codebase analysis, long context | Gemini (Pro/Flash) | Large context window (1M+ tokens) |
| Code completion, quick edits | Codex / GPT-4o | Fast, good at fill-in-the-middle |
| Rapid prototyping, boilerplate | Gemini Flash / Claude Haiku | Fast and cheap |
| Security-sensitive code review | Claude Opus | Careful, thorough analysis |

## Code Generation

When generating code, always specify:
1. Language and version (e.g., Python 3.12, Go 1.22, TypeScript 5.x)
2. Framework/library constraints (e.g., FastAPI, Gin, React)
3. Error handling expectations
4. Whether tests are needed

### Workflow: Generate New Code

```
1. Gather requirements (language, framework, constraints)
2. Generate code with clear structure
3. Add error handling and input validation
4. Include docstrings/comments for public APIs
5. Generate corresponding tests
6. Review for security and edge cases
```

### Example: Python Function

When asked to write a function, produce complete, production-ready code:

```python
def fetch_with_retry(
    url: str,
    max_retries: int = 3,
    backoff_factor: float = 0.5,
    timeout: int = 30,
) -> dict:
    """Fetch JSON from a URL with exponential backoff retry.

    Args:
        url: The URL to fetch.
        max_retries: Maximum number of retry attempts.
        backoff_factor: Multiplier for exponential backoff.
        timeout: Request timeout in seconds.

    Returns:
        Parsed JSON response as a dictionary.

    Raises:
        requests.HTTPError: If all retries fail.
    """
    import time
    import requests

    for attempt in range(max_retries + 1):
        try:
            resp = requests.get(url, timeout=timeout)
            resp.raise_for_status()
            return resp.json()
        except requests.RequestException:
            if attempt == max_retries:
                raise
            time.sleep(backoff_factor * (2 ** attempt))
```

### Example: Go Function

```go
// FetchWithRetry fetches JSON from a URL with exponential backoff.
func FetchWithRetry(ctx context.Context, url string, maxRetries int) ([]byte, error) {
    var lastErr error
    for attempt := 0; attempt <= maxRetries; attempt++ {
        req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
        if err != nil {
            return nil, fmt.Errorf("creating request: %w", err)
        }
        resp, err := http.DefaultClient.Do(req)
        if err != nil {
            lastErr = err
            time.Sleep(time.Duration(1<<attempt) * 500 * time.Millisecond)
            continue
        }
        defer resp.Body.Close()
        if resp.StatusCode >= 500 {
            lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
            time.Sleep(time.Duration(1<<attempt) * 500 * time.Millisecond)
            continue
        }
        return io.ReadAll(resp.Body)
    }
    return nil, fmt.Errorf("all %d retries exhausted: %w", maxRetries, lastErr)
}
```

## Debugging

### Workflow: Debug an Issue

```
1. Reproduce — get the exact error message, stack trace, or unexpected behavior
2. Isolate — narrow down to the smallest failing unit
3. Hypothesize — form 2–3 likely root causes
4. Verify — add logging, assertions, or write a minimal reproducer
5. Fix — apply the smallest correct change
6. Validate — run tests, confirm the fix, check for regressions
```

### Common Debug Prompts

- "Here's the error and code. What's wrong and how do I fix it?"
- "This function returns incorrect results for input X. Debug it."
- "This code is slow. Profile and suggest optimizations."

When debugging, always ask for:
- The full error message / stack trace
- The relevant code snippet
- What was expected vs what happened
- Language version and OS if relevant

## Code Review

### Workflow: Review Code

Check for these categories in order of severity:

1. **Correctness** — Logic errors, off-by-one, null/nil handling, race conditions
2. **Security** — Injection, auth bypass, secrets in code, unsafe deserialization
3. **Performance** — N+1 queries, unbounded loops, missing indexes, memory leaks
4. **Maintainability** — Naming, complexity, duplication, missing abstractions
5. **Testing** — Coverage gaps, missing edge cases, brittle tests
6. **Style** — Formatting, idioms, linting compliance

### Review Output Format

```
## Review Summary

### Critical
- [file:line] Description of critical issue

### Suggestions
- [file:line] Description of improvement

### Positive
- Good use of X pattern in Y
```

## Refactoring

### Workflow: Refactor Code

```
1. Ensure tests exist (write them first if missing)
2. Identify the smell (duplication, long method, god class, etc.)
3. Apply a named refactoring pattern
4. Run tests after each transformation
5. Verify behavior is preserved
```

### Common Refactoring Patterns

| Smell | Refactoring |
|-------|-------------|
| Long function | Extract Method |
| Duplicate code | Extract shared function/module |
| Deep nesting | Early returns / guard clauses |
| God class | Split by responsibility (SRP) |
| Primitive obsession | Introduce value objects / types |
| Feature envy | Move method to owning class |
| Shotgun surgery | Consolidate related changes |

## Test Writing

### Workflow: Write Tests

```
1. Identify the unit under test (function, method, class)
2. List test cases: happy path, edge cases, error cases
3. Write tests following the Arrange-Act-Assert pattern
4. Add table-driven tests for multiple inputs (especially in Go)
5. Mock external dependencies
6. Aim for behavior testing, not implementation testing
```

### Go Table-Driven Test Example

```go
func TestAdd(t *testing.T) {
    tests := []struct {
        name     string
        a, b     int
        expected int
    }{
        {"positive", 2, 3, 5},
        {"zero", 0, 0, 0},
        {"negative", -1, -2, -3},
        {"mixed", -1, 5, 4},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := Add(tt.a, tt.b)
            if got != tt.expected {
                t.Errorf("Add(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.expected)
            }
        })
    }
}
```

### Python Pytest Example

```python
import pytest

@pytest.mark.parametrize("a,b,expected", [
    (2, 3, 5),
    (0, 0, 0),
    (-1, -2, -3),
])
def test_add(a, b, expected):
    assert add(a, b) == expected

def test_add_raises_on_non_numeric():
    with pytest.raises(TypeError):
        add("a", 1)
```

## Architecture & Design

When asked about architecture decisions:

1. **Clarify constraints** — Scale, team size, timeline, existing tech stack
2. **Propose options** — Present 2–3 approaches with tradeoffs
3. **Recommend** — Pick one with clear rationale
4. **Outline structure** — Directory layout, key interfaces, data flow

### Common Patterns

- **Repository pattern** — Abstract data access behind interfaces
- **Service layer** — Business logic separate from transport/storage
- **CQRS** — Separate read and write models for complex domains
- **Event-driven** — Decouple components via events/messages
- **Clean architecture** — Dependencies point inward, domain at center

## Multi-Language Quick Reference

### Python
```bash
# Run
python3 main.py

# Test
pytest -v
pytest --cov=src tests/

# Lint & format
ruff check . && ruff format .

# Type check
mypy src/
```

### Go
```bash
# Run
go run .

# Test
go test ./...
go test -race -cover ./...

# Lint
golangci-lint run

# Build
go build -o bin/app .
```

### TypeScript / Node.js
```bash
# Run
npx tsx src/index.ts

# Test
npm test
npx vitest run

# Lint & format
npx eslint . && npx prettier --check .

# Build
npx tsc --build
```

### Rust
```bash
# Run
cargo run

# Test
cargo test

# Lint
cargo clippy -- -D warnings

# Build
cargo build --release
```

### Shell Scripts
```bash
# Lint
shellcheck script.sh

# Test (with bats)
bats tests/
```

## Best Practices

- **Always handle errors** — Never silently ignore errors; log or propagate them
- **Write tests alongside code** — Not after; tests clarify intent and catch regressions
- **Keep functions small** — Each function does one thing; aim for <30 lines
- **Use meaningful names** — Code is read far more than it's written
- **Prefer composition over inheritance** — Especially in Go and modern Python
- **Document the "why"** — Code shows "what"; comments explain "why"
- **Use types** — Leverage type systems (Go, TypeScript, Rust, Python type hints)
- **Fail fast** — Validate inputs early, return errors at boundaries
- **Avoid premature optimization** — Profile first, optimize the hot path
- **Security by default** — Sanitize inputs, parameterize queries, use least privilege

## Bundled Scripts

- Run all checks: `{baseDir}/scripts/run-checks.sh` (auto-detects project type, runs lint/format/typecheck/test)
- Scaffold project: `{baseDir}/scripts/scaffold.sh -t go-cli -n myapp` (supports go-cli, go-api, python-cli, python-api, ts-node, ts-api, rust-cli)
````
