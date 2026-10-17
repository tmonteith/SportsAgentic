# 🏈 SportsAgentic — Universal College Football Agent & MCP Server

An open-source, high-performance **College Football (CFB) Agent & MCP Server** written in Go.

Query live game scores, weekly matchups, full team schedules, and national rankings across **any AI agent ecosystem**:
- 🟣 **Claude** (Claude Desktop & Claude Code via native MCP)
- 🟢 **OpenAI & ChatGPT** (Custom GPT Actions via OpenAPI & Assistants API)
- 🔷 **Delta** (Pre-packaged Delta Skill `/cfb`)
- 💻 **Cursor, Zed & Windsurf** (Native IDE MCP)
- 🐳 **Docker Ready** (One-command setup with zero Go/Python installation)

---

## 🏗️ Architecture

```
  ┌──────────────────────────────────────────────────────────────┐
  │                        AI INTERFACES                         │
  │   [Claude Desktop]   [ChatGPT / OpenAI]   [Delta]   [Cursor] │
  └───────────────┬──────────────┬───────────────┬───────────────┘
                  │              │               │
     MCP (stdio)  │   OpenAPI    │   MCP (SSE)   │  MCP (stdio)
                  ▼              ▼               ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                SportsAgentic Core (Go Server)                │
  │  - Smart Team Entity Resolver ("Bama", "Ole Miss", "CSU")    │
  │  - Dynamic Season & Week Resolver                            │
  │  - Multi-Transport Engine (Stdio, SSE, REST)                 │
  └──────────────────────────────┬───────────────────────────────┘
                                 │
                                 ▼
  ┌──────────────────────────────────────────────────────────────┐
  │                 College Football Data (CFBD)                 │
  └──────────────────────────────────────────────────────────────┘
```

---

## ⚡ Quickstart with Docker (Easiest)

You don't need Go or Python installed if you have Docker.

```bash
git clone https://github.com/tmonteith/SportsAgentic.git
cd SportsAgentic

# 1. Create your .env file
cp .env.example .env
# Edit .env and paste your free key from https://collegefootballdata.com/key

# 2. Start the container
docker compose up
```

The server will build and start listening at `http://localhost:8080`.

---

## 🛠️ Local Setup (Without Docker)

### Prerequisites
- **Go 1.22+**
- **Free API Key** from [CollegeFootballData.com/key](https://collegefootballdata.com/key)

```bash
git clone https://github.com/tmonteith/SportsAgentic.git
cd SportsAgentic

export CFBD_API_KEY="your_api_key_here"

# Run HTTP / SSE Server (for Delta, Web & Network Agents)
go run cmd/server/main.go --mode=sse --port=8080

# Or build a standalone binary
go build -o bin/sportsagentic cmd/server/main.go
```

---

## 🔌 Connecting to Your Agent Platform

### 1. 🔷 Delta

When you open this repository as a project in Delta, the `/cfb` skill is automatically detected from `.agents/skills/cfb/`.

To install the skill globally across all your Delta conversations:
```bash
mkdir -p ~/.agents/skills/cfb
cp .agents/skills/cfb/* ~/.agents/skills/cfb/
```

Now in Delta, ask anytime:
* `/cfb who is playing today?`
* `/cfb what are the scores for Bama?`
* `/cfb what was the most recent CSU game score?`
* `/cfb check rankings for week 7`

---

### 2. 🟣 Claude (Desktop & CLI)

#### Claude Desktop
Add this entry to your `claude_desktop_config.json`:
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

#### Claude Code (Anthropic CLI)
```bash
claude mcp add college-football /path/to/sportsagentic -- --mode=stdio
```

---

### 3. 🟢 OpenAI & ChatGPT

#### A. ChatGPT Custom GPT Actions
You can connect this agent directly to a **Custom GPT** in ChatGPT:
1. In ChatGPT, go to **Explore GPTs > Create a GPT > Configure > Actions > Create new action**.
2. Copy and paste the contents of [`openapi.yaml`](./openapi.yaml) into the Schema box.
3. If running locally, expose your port via a tunnel (e.g. `ngrok http 8080`) and set the server URL in `openapi.yaml`.
4. Now ChatGPT can query live games, schedules, and rankings!

#### B. OpenAI SDK (Python / TypeScript)
Use standard tool calling with OpenAI:
```python
from openai import OpenAI
import requests

client = OpenAI()

# Call the local SportsAgentic endpoint
scores = requests.get("http://localhost:8080/api/scores?team=OleMiss").json()
```

---

### 4. 💻 Cursor / Zed / Windsurf

Add to your Cursor / Zed MCP configuration:
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

## 📦 Exposed MCP Tools & Endpoints

| Tool Name | Parameters | Description |
| :--- | :--- | :--- |
| **`get_cfb_scores`** | `year`, `week`, `date` (YYYY-MM-DD), `team` | Fetch game scores and schedules for any team, week, or specific date. |
| **`get_team_schedule`**| `team` (required), `year` | Get full season schedule and results for any college team. |
| **`get_cfb_rankings`** | `year`, `week` | Retrieve national AP Top 25 and CFP rankings. |

---

## 🤝 Contributing
Pull requests are welcome! If you want to add new tools (such as transfer portal trackers, injury reports, or betting line analytics), feel free to open an issue or PR.

## 📄 License
MIT License. See [LICENSE](LICENSE) for details.
