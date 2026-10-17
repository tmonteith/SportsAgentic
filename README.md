# 🏈 SportsAgentic — College Football Agent & MCP Server

An open-source, real-time **College Football (CFB) Agent & MCP Server** built in Go.

Exposes college football game scores, schedules, national rankings, and team stats via the **Model Context Protocol (MCP)** and **HTTP / SSE**, making it instantly pluggable into AI agents and coding tools (Delta, Claude Desktop, Cursor, Zed, Windsurf, or custom web frontends).

---

## ⚡ Features

- **Multi-Client MCP Support:**
  - **SSE / HTTP Server Mode:** Network-accessible streamable MCP protocol for Delta, web apps, Slack/Discord bots, and remote agents.
  - **Stdio Mode:** Direct standard-in/out transport for desktop clients like Claude Desktop and Cursor.
- **Delta Agent Skill Included:** Pre-configured `.agents/skills/cfb/` skill for Delta so any Delta user can immediately ask CFB queries using `/cfb`.
- **Smart Team Entity Resolution:** Automatically resolves common team nicknames and abbreviations (e.g., `"Ole Miss"` → `"Mississippi"`, `"Bama"` → `"Alabama"`, `"OSU"` → `"Ohio State"`, `"UGA"` → `"Georgia"`).
- **Comprehensive CFB Data:** Powered by [CollegeFootballData.com (CFBD)](https://collegefootballdata.com/).

---

## 🛠️ Prerequisites

- **Go 1.22+**
- **Python 3.8+** (for the Delta skill runner)
- **Free API Key** from [CollegeFootballData.com/key](https://collegefootballdata.com/key)

---

## 🚀 Quickstart

### 1. Clone the repository
```bash
git clone https://github.com/tmonteith/SportsAgentic.git
cd SportsAgentic
```

### 2. Configure your API Key
```bash
export CFBD_API_KEY="your_api_key_here"
```

### 3. Run the Server

#### Option A: HTTP / SSE Mode (Recommended for Delta & Web Clients)
```bash
go run cmd/server/main.go --mode=sse --port=8080
```
- **SSE Endpoint:** `http://localhost:8080/sse`
- **JSON-RPC Message Endpoint:** `http://localhost:8080/message`

#### Option B: Build Single Binary
```bash
go build -o bin/sportsagentic cmd/server/main.go
./bin/sportsagentic --mode=sse --port=8080
```

---

## 🔌 Connecting to Your Agentic Tools

### 1. Delta
When you open this repository as a project in Delta, the `/cfb` skill is automatically detected from `.agents/skills/cfb/`.

To use the skill globally across all your Delta projects:
```bash
mkdir -p ~/.agents/skills/cfb
cp .agents/skills/cfb/* ~/.agents/skills/cfb/
```

Now in Delta, simply ask:
* `/cfb who is playing today?`
* `/cfb what was the score of the Alabama game?`
* `/cfb show me the latest top 25 rankings`

---

### 2. Claude Desktop / Cursor / Windsurf / Zed (Stdio Mode)

Add the server to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "college-football": {
      "command": "/absolute/path/to/SportsAgentic/bin/sportsagentic",
      "args": ["--mode=stdio"],
      "env": {
        "CFBD_API_KEY": "your_api_key_here"
      }
    }
  }
}
```

---

### 3. Direct CLI Queries
You can also invoke tools directly from your terminal:
```bash
# Get scores by date
python3 .agents/skills/cfb/query.py get_cfb_scores '{"date": "2026-10-17"}'

# Get scores / results for a specific team
python3 .agents/skills/cfb/query.py get_cfb_scores '{"team": "Ole Miss"}'

# Get team schedule
python3 .agents/skills/cfb/query.py get_team_schedule '{"team": "Georgia"}'

# Get rankings
python3 .agents/skills/cfb/query.py get_cfb_rankings '{"week": 7}'
```

---

## 📦 Exposed MCP Tools

| Tool | Parameters | Description |
| :--- | :--- | :--- |
| **`get_cfb_scores`** | `year`, `week`, `date` (YYYY-MM-DD), `team` | Fetch game scores and results for any team, week, or specific date. |
| **`get_team_schedule`**| `team` (required), `year` | Get full season schedule and results for any college team. |
| **`get_cfb_rankings`** | `year`, `week` | Retrieve national AP Top 25 and CFP rankings. |

---

## 🤝 Contributing

Pull requests are welcome! If you want to add tools (e.g. transfer portal tracker, player injury availability reports, advanced analytics), feel free to open an issue or PR.

## 📄 License
MIT License. See [LICENSE](LICENSE) for details.
