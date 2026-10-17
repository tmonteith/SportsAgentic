# 🏈 SportsAgentic — Universal College Football Agent & MCP Server

An open-source, real-time **College Football (CFB) Agent & MCP Server** written in Go.

Exposes live scores, team schedules, rankings, and stats across **all major AI tools and LLM ecosystems**:
- 🟣 **Claude** (Claude Desktop & Claude Code via MCP)
- 🟢 **OpenAI & ChatGPT** (Custom GPT Actions via OpenAPI & Assistants API)
- 🔷 **Delta** (Pre-packaged Delta Skill `/cfb`)
- 💻 **Cursor, Zed & Windsurf** (Native IDE MCP)
- 🐳 **Docker & Container Ready** (Run anywhere with zero Go/Python installation)

---

## ⚡ Quickstart with Docker (Easiest)

You don't need Go or Python installed if you have Docker.

```bash
git clone https://github.com/tmonteith/SportsAgentic.git
cd SportsAgentic

# Set your free API key (https://collegefootballdata.com/key)
export CFBD_API_KEY="your_api_key_here"

# Run with Docker Compose
docker compose up
```
The server is now live at `http://localhost:8080` (serving MCP SSE and REST endpoints).

---

## 🛠️ Local Setup (Without Docker)

### Prerequisites
- **Go 1.22+**
- **Free API Key** from [CollegeFootballData.com/key](https://collegefootballdata.com/key)

```bash
export CFBD_API_KEY="your_api_key_here"

# Run HTTP / SSE Server (for Delta, Web & Network Agents)
go run cmd/server/main.go --mode=sse --port=8080

# Or build single binary
go build -o bin/sportsagentic cmd/server/main.go
```

---

## 🔌 Connecting to Your Agent Platform

### 1. 🟣 Claude (Desktop & CLI)

#### Claude Desktop
Add this to your `claude_desktop_config.json`:
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

### 2. 🟢 OpenAI & ChatGPT

#### A. ChatGPT Custom GPT Actions
You can connect this agent directly to a **Custom GPT** in ChatGPT:
1. In ChatGPT, go to **Explore GPTs > Create a GPT > Configure > Actions > Create new action**.
2. Copy and paste the contents of [`openapi.yaml`](./openapi.yaml) into the Schema box.
3. If running locally, expose your port via a tunnel (e.g. `ngrok http 8080`) and set the server URL in `openapi.yaml`.
4. Now ChatGPT can query live games, schedules, and rankings in your custom GPT!

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

### 3. 🔷 Delta

When you open this repository as a project in Delta, the `/cfb` skill is automatically detected from `.agents/skills/cfb/`.

To install the skill globally across all your Delta conversations:
```bash
mkdir -p ~/.agents/skills/cfb
cp .agents/skills/cfb/* ~/.agents/skills/cfb/
```

Now in Delta, ask anytime:
* `/cfb who is playing today?`
* `/cfb what are the scores for Bama?`
* `/cfb check rankings for week 7`

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
Contributions are welcome! Please open an issue or pull request.

## 📄 License
MIT License. See [LICENSE](LICENSE) for details.
