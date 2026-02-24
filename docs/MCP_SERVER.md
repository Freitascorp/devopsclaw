# MCP Server — Gemini CLI & Claude Desktop Integration

DevOpsClaw exposes its tools via the **Model Context Protocol (MCP)** stdio transport, allowing external AI clients to call devopsclaw's tools directly.

## Quick Start

```bash
# Verify MCP server works
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"test"}}}' \
  | devopsclaw mcp 2>/dev/null | python3 -m json.tool
```

## Gemini CLI

Add to `~/.gemini/settings.json`:

```json
{
  "mcpServers": {
    "devopsclaw": {
      "command": "devopsclaw",
      "args": ["mcp"]
    }
  }
}
```

If `devopsclaw` is not in your `$PATH`, use the full path:

```json
{
  "mcpServers": {
    "devopsclaw": {
      "command": "/usr/local/bin/devopsclaw",
      "args": ["mcp"]
    }
  }
}
```

After configuration, Gemini CLI can use devopsclaw tools. For example:

```
> Read the file at ~/project/main.go
> Run `terraform plan` in ~/infra
> Search the web for "Kubernetes pod eviction causes"
```

### Using BMAD Method with Gemini CLI

Once connected, Gemini CLI gains access to devopsclaw's exec, file, and web tools.
You can invoke BMAD workflows by instructing Gemini to read the BMAD skill files
and follow the method:

```
> Read the file at ~/.devopsclaw/workspace/skills/bmad-method/SKILL.md and follow
  the BMAD method to analyze my project at ~/project
```

Or run the BMAD CLI through exec:

```
> Run `cd ~/.devopsclaw/workspace && npx bmad-method@latest install` to set up BMAD
```

## Claude Desktop

Add to Claude Desktop's MCP configuration (`~/Library/Application Support/Claude/claude_desktop_config.json` on macOS):

```json
{
  "mcpServers": {
    "devopsclaw": {
      "command": "devopsclaw",
      "args": ["mcp"]
    }
  }
}
```

## Exposed Tools

| Tool | Description |
|------|-------------|
| `exec` | Execute shell commands |
| `read_file` | Read file contents |
| `write_file` | Create/overwrite files |
| `edit_file` | Find-and-replace in files |
| `append_file` | Append to files |
| `list_dir` | List directory contents |
| `web_search` | Search the web (Brave, Tavily, DuckDuckGo, Perplexity) |
| `web_fetch` | Fetch and extract content from URLs |

## How It Works

```
┌─────────────┐    stdin/stdout    ┌──────────────┐
│  Gemini CLI  │◄──── JSON-RPC ───►│  devopsclaw   │
│  (MCP client)│    (MCP protocol) │  mcp server   │
└─────────────┘                    └──────┬───────┘
                                          │
                                   ToolRegistry
                                   (exec, files, web)
```

The MCP server starts when you run `devopsclaw mcp`. It reads JSON-RPC 2.0 messages
from stdin and writes responses to stdout. Gemini CLI (or any MCP-compatible client)
manages the process lifecycle automatically.

## Debug Mode

```bash
devopsclaw mcp --debug
```

Debug logs go to stderr (stdout is reserved for MCP protocol traffic).
