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

![SportsAgentic Architecture](docs/architecture.svg)

SportsAgentic is a high-performance Model Context Protocol (MCP) server written in Go 1.23:
- **Multi-Transport Engine**: Supports `stdio` (for Claude Desktop, Cursor, Zed, Windsurf) and HTTP/SSE on `:8080/sse` and `:8080/message` (for `SportsAgenticAgent`, Delta skills, and network clients).
- **Tool Dispatcher**: Exposes three core MCP tools:
  - `get_cfb_scores`: fetches live and historical scores by season, week, team, or date with mandatory parameter enforcement (preventing unbounded nationwide season queries).
  - `get_team_schedule`: retrieves full season game schedules for a specified team.
  - `get_cfb_rankings`: retrieves national AP Top 25 and College Football Playoff (CFP) rankings.
- **Smart Entity Resolution** (`pkg/resolver/team.go`): Normalizes informal aliases, abbreviations, and nicknames (e.g. "Bama" ➔ "Alabama", "CSU" ➔ "Colorado State", "Ole Miss" ➔ "Ole Miss") into official CFBD school identifiers.
- **CFBD Client** (`pkg/cfbd/client.go`): Communicates with the College Football Data REST API with Bearer token authentication.

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
| **`get_cfb_scores`** | `year`, `week`, `date` (YYYY-MM-DD), `team` | Fetch game scores and schedules for a team or week, optionally narrowed to one date. Calls without `team` or `week` return an error. |
| **`get_team_schedule`**| `team` (required), `year` | Get full season schedule and results for any college team. |
| **`get_cfb_rankings`** | `year`, `week` | Retrieve national AP Top 25 and CFP rankings. |

---

## ☸️ Kubernetes Deployment (Helm)

A production Helm chart is provided in [`charts/sportsagentic`](./charts/sportsagentic).

```bash
# Lint and preview rendered manifests
helm lint charts/sportsagentic --set secrets.existingSecret=mcp-secrets
helm template sportsagentic charts/sportsagentic --set secrets.existingSecret=mcp-secrets

# Keep the API key in a Secret that Helm doesn't manage
kubectl create namespace sportsagentic
kubectl create secret generic mcp-secrets \
  --namespace sportsagentic \
  --from-literal=CFBD_API_KEY="${CFBD_API_KEY}"

# Install or upgrade the release in your cluster
helm upgrade --install sportsagentic charts/sportsagentic \
  --namespace sportsagentic \
  --set secrets.existingSecret=mcp-secrets \
  --set-string image.tag=<git-sha>
```

The chart fails if neither `secrets.existingSecret` nor `secrets.cfbdApiKey` is set, so an empty key can't reach the cluster. Use `--set-string` for the image tag, because a git SHA made only of digits is otherwise read as a number.

---

## 🏞️ Lakehouse Ingestion (extractor and poller)

Two small commands feed the SportsAgentic lakehouse. Both reuse `pkg/cfbd`, count every CFBD request in their logs (the monthly quota is shared with the MCP tools) and retry `429`/`5xx` with backoff.

| Command | Runs as | Does |
| --- | --- | --- |
| `cmd/extractor` | nightly CronJob and backfill Job (namespace `data`) | Lands raw `/games`, `/plays` and `/teams` responses, gzipped, at `gs://${GCP_PROJECT}-sa-landing/cfb/<endpoint>/season=<YYYY>/week=<WW>/<fetched_at>.json.gz`. Never overwrites; skips a response already landed (same endpoint and query) unless `--force`. |
| `cmd/poller` | Deployment, 1 replica | Reads the schedule, sleeps until 15 minutes before the next kickoff, then polls `/scoreboard` every 60 s and publishes a game to Kafka topic `cfb.live_games` only when its state changed. |

```bash
go test ./...                                       # recorded fixtures only; no live API, GCS or Kafka

# Extractor (Application Default Credentials for GCS)
export CFBD_API_KEY=... GCP_PROJECT=... GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
go run ./cmd/extractor --season=2024 --weeks=all --dry-run    # what would be fetched
go run ./cmd/extractor --season=2024 --weeks=all              # backfill one season
go run ./cmd/extractor --season=current --weeks=recent --force  # what the nightly CronJob runs

# Poller
export CFBD_API_KEY=... KAFKA_BOOTSTRAP=localhost:9092
go run ./cmd/poller
```

Images: `cmd/extractor/Dockerfile` and `cmd/poller/Dockerfile` (distroless, non-root, `linux/amd64` and `linux/arm64`), built and pushed to OCIR as `sportsagentic/extractor` and `sportsagentic/poller` by `.github/workflows/ingest-images.yml` on every push to `main` that touches them, both with the same short-SHA tag. The Kubernetes manifests live in the infra repo (`k8s/data/ingest`).

---

## 🤝 Contributing
Pull requests are welcome! If you want to add new tools (such as transfer portal trackers, injury reports, or betting line analytics), feel free to open an issue or PR.

## 📄 License
MIT License. See [LICENSE](LICENSE) for details.
