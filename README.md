# SportsAgentic - College Football Agent & MCP Server

Real-time College Football (CFB) agent powered by Go, exposing both an **SSE/HTTP REST Server** and a **Stdio MCP Server** for maximum client reach.

## Interfaces Supported

1. **SSE / HTTP Server Mode (`--mode=sse`):**
   - **SSE Endpoint (`/sse`):** Allows web frontends, remote AI clients, and network agents to stream MCP tool executions over HTTP.
   - **REST API Endpoint (`/api/scores`):** Direct HTTP JSON API for web apps, mobile clients, Slack/Discord bots, and `curl`.
2. **Stdio MCP Server Mode (`--mode=stdio`):**
   - Direct stdio protocol interface for local desktop AI clients (Claude Desktop, Cursor, Zed, Windsurf).

---

## Running the Server Locally

### Prerequisites
- Go 1.22+
- Free API Key from [CollegeFootballData.com](https://collegefootballdata.com/)

### Environment Setup
```bash
export CFBD_API_KEY="your_api_key_here"
```

### 1. Run as HTTP / SSE Server (Default - Network & Web Clients)
```bash
go run cmd/server/main.go --mode=sse --port=8080
```
- **Test REST API in browser or curl:**
  ```bash
  curl "http://localhost:8080/api/scores?team=olemiss"
  ```
- **SSE MCP Endpoint for Web Agents:** `http://localhost:8080/sse`

---

### 2. Run as Stdio MCP Server (Desktop AI Clients)
```bash
go run cmd/server/main.go --mode=stdio
```

---

## Connecting to Claude Desktop / Local MCP Clients

Add the binary execution entry to your `claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "college-football": {
      "command": "/path/to/repos/SportsAgentic/bin/sportsagentic",
      "args": ["--mode=stdio"],
      "env": {
        "CFBD_API_KEY": "your_api_key_here"
      }
    }
  }
}
```
