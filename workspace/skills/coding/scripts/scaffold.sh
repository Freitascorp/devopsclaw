#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage: scaffold.sh -t type -n name [options]

Scaffold a new project with standard structure.

Supported types:
  go-cli          Go CLI application (cobra)
  go-api          Go HTTP API (net/http or gin)
  python-cli      Python CLI (click/typer)
  python-api      Python API (FastAPI)
  ts-node         TypeScript Node.js project
  ts-api          TypeScript API (Express/Fastify)
  rust-cli        Rust CLI (clap)

Options:
  -t, --type       Project type (required)
  -n, --name       Project name (required)
  -d, --dir        Parent directory (default: current dir)
  -h, --help       Show this help
USAGE
}

proj_type=""
proj_name=""
parent_dir="."

while [[ $# -gt 0 ]]; do
  case "$1" in
    -t|--type) proj_type="${2-}"; shift 2 ;;
    -n|--name) proj_name="${2-}"; shift 2 ;;
    -d|--dir)  parent_dir="${2-}"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1" >&2; usage; exit 1 ;;
  esac
done

if [[ -z "$proj_type" || -z "$proj_name" ]]; then
  echo "type and name are required" >&2
  usage
  exit 1
fi

proj_dir="$parent_dir/$proj_name"

if [[ -d "$proj_dir" ]]; then
  echo "Directory already exists: $proj_dir" >&2
  exit 1
fi

mkdir -p "$proj_dir"
cd "$proj_dir"

echo "Scaffolding $proj_type project: $proj_name"

case "$proj_type" in
  go-cli)
    if ! command -v go >/dev/null 2>&1; then
      echo "go not found in PATH" >&2; exit 1
    fi
    go mod init "$proj_name"
    mkdir -p cmd/"$proj_name" internal pkg
    cat > cmd/"$proj_name"/main.go <<'GOEOF'
package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	fmt.Println("Hello from $proj_name")
	return nil
}
GOEOF
    sed -i '' "s/\$proj_name/$proj_name/g" cmd/"$proj_name"/main.go 2>/dev/null || true
    cat > .gitignore <<'EOF'
bin/
*.exe
.DS_Store
EOF
    cat > Makefile <<MKEOF
.PHONY: build test lint run

build:
	go build -o bin/$proj_name ./cmd/$proj_name

test:
	go test -race ./...

lint:
	golangci-lint run ./...

run:
	go run ./cmd/$proj_name
MKEOF
    echo "  Created: go.mod, cmd/$proj_name/main.go, Makefile"
    ;;

  go-api)
    if ! command -v go >/dev/null 2>&1; then
      echo "go not found in PATH" >&2; exit 1
    fi
    go mod init "$proj_name"
    mkdir -p cmd/server internal/handler internal/middleware pkg
    cat > cmd/server/main.go <<'GOEOF'
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, `{"status":"ok"}`)
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("server starting", "port", port)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	slog.Info("server stopped")
}
GOEOF
    echo "  Created: go.mod, cmd/server/main.go"
    ;;

  python-cli)
    mkdir -p src/"$proj_name" tests
    cat > src/"$proj_name"/__init__.py <<EOF
"""$proj_name"""
__version__ = "0.1.0"
EOF
    cat > src/"$proj_name"/cli.py <<'PYEOF'
"""CLI entry point."""
import click

@click.group()
@click.version_option()
def cli():
    """Project CLI."""
    pass

@cli.command()
@click.argument("name", default="world")
def hello(name: str):
    """Say hello."""
    click.echo(f"Hello, {name}!")

if __name__ == "__main__":
    cli()
PYEOF
    cat > pyproject.toml <<TOMLEOF
[project]
name = "$proj_name"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["click>=8.0"]

[project.scripts]
$proj_name = "$proj_name.cli:cli"

[tool.ruff]
target-version = "py311"

[tool.pytest.ini_options]
testpaths = ["tests"]
TOMLEOF
    cat > tests/__init__.py <<EOF
EOF
    cat > .gitignore <<'EOF'
__pycache__/
*.pyc
.venv/
dist/
*.egg-info/
.DS_Store
EOF
    echo "  Created: pyproject.toml, src/$proj_name/, tests/"
    ;;

  python-api)
    mkdir -p src/"$proj_name" tests
    cat > src/"$proj_name"/__init__.py <<EOF
"""$proj_name"""
__version__ = "0.1.0"
EOF
    cat > src/"$proj_name"/main.py <<'PYEOF'
"""FastAPI application."""
from fastapi import FastAPI

app = FastAPI()

@app.get("/health")
async def health():
    return {"status": "ok"}

@app.get("/")
async def root():
    return {"message": "Hello, world!"}
PYEOF
    cat > pyproject.toml <<TOMLEOF
[project]
name = "$proj_name"
version = "0.1.0"
requires-python = ">=3.11"
dependencies = ["fastapi>=0.100", "uvicorn[standard]>=0.20"]

[tool.ruff]
target-version = "py311"

[tool.pytest.ini_options]
testpaths = ["tests"]
TOMLEOF
    cat > Makefile <<MKEOF
.PHONY: dev test lint

dev:
	uvicorn $proj_name.main:app --reload --port 8000

test:
	pytest -v

lint:
	ruff check . && ruff format --check .
MKEOF
    echo "  Created: pyproject.toml, src/$proj_name/main.py, Makefile"
    ;;

  ts-node)
    cat > package.json <<JSONEOF
{
  "name": "$proj_name",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "build": "tsc",
    "dev": "tsx watch src/index.ts",
    "start": "node dist/index.js",
    "test": "vitest run",
    "lint": "eslint src/"
  }
}
JSONEOF
    cat > tsconfig.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "declaration": true
  },
  "include": ["src"]
}
EOF
    mkdir -p src tests
    cat > src/index.ts <<'EOF'
console.log("Hello from $proj_name");
EOF
    cat > .gitignore <<'EOF'
node_modules/
dist/
.DS_Store
EOF
    echo "  Created: package.json, tsconfig.json, src/index.ts"
    echo "  Run: npm install && npm install -D typescript tsx vitest eslint"
    ;;

  ts-api)
    cat > package.json <<JSONEOF
{
  "name": "$proj_name",
  "version": "0.1.0",
  "type": "module",
  "scripts": {
    "build": "tsc",
    "dev": "tsx watch src/server.ts",
    "start": "node dist/server.js",
    "test": "vitest run",
    "lint": "eslint src/"
  }
}
JSONEOF
    cat > tsconfig.json <<'EOF'
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "bundler",
    "outDir": "dist",
    "rootDir": "src",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "declaration": true
  },
  "include": ["src"]
}
EOF
    mkdir -p src tests
    cat > src/server.ts <<'TSEOF'
import { createServer } from "node:http";

const port = parseInt(process.env.PORT ?? "8080", 10);

const server = createServer((req, res) => {
  if (req.url === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok" }));
    return;
  }
  res.writeHead(200, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ message: "Hello, world!" }));
});

server.listen(port, () => {
  console.log(`Server running on http://localhost:${port}`);
});
TSEOF
    cat > .gitignore <<'EOF'
node_modules/
dist/
.DS_Store
EOF
    echo "  Created: package.json, tsconfig.json, src/server.ts"
    echo "  Run: npm install && npm install -D typescript tsx vitest eslint"
    ;;

  rust-cli)
    if ! command -v cargo >/dev/null 2>&1; then
      echo "cargo not found in PATH" >&2; exit 1
    fi
    cargo init --name "$proj_name" .
    echo "  Created: Cargo.toml, src/main.rs"
    ;;

  *)
    echo "Unknown project type: $proj_type" >&2
    echo "Supported: go-cli, go-api, python-cli, python-api, ts-node, ts-api, rust-cli" >&2
    exit 1
    ;;
esac

# Initialize git if not in a repo
if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git init -q
  echo "  Initialized git repository"
fi

echo ""
echo "✓ Project scaffolded at: $(pwd)"
