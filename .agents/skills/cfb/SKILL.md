---
name: cfb
description: Retrieve live College Football (CFB) scores, team schedules, rankings, and stats by querying the local SportsAgentic MCP server. Use whenever the user asks about college football games, scores, rankings, or team schedules.
---

# College Football (CFB) Agent Skill

This skill connects Delta to your local `SportsAgentic` Go MCP server running at `http://localhost:8080`.

## How to Query the Server

Use the `terminal` tool to run the query script located beside this skill:

```bash
python3 .agents/skills/cfb/query.py <tool_name> '<json_arguments>'
```
*(Or `python3 ~/.agents/skills/cfb/query.py` if installed globally)*

### Available Tools:

1. **`get_cfb_scores`** - Get game scores and results by team, year, week, or specific date.
   - For today's games: check current date with `date +%Y-%m-%d` and pass `date` and/or `year` and `week`.
   ```bash
   python3 .agents/skills/cfb/query.py get_cfb_scores '{"date": "2026-10-17"}'
   python3 .agents/skills/cfb/query.py get_cfb_scores '{"team": "Ole Miss"}'
   ```

2. **`get_team_schedule`** - Get a team's full schedule and game results.
   ```bash
   python3 .agents/skills/cfb/query.py get_team_schedule '{"team": "Alabama"}'
   ```

3. **`get_cfb_rankings`** - Get national AP Top 25 or CFP rankings.
   ```bash
   python3 .agents/skills/cfb/query.py get_cfb_rankings '{"week": 7}'
   ```

## Instructions:
- Team names can be common abbreviations or nicknames (e.g. "Bama", "Ole Miss", "OSU", "UGA", "Longhorns").
- When asked about scores, schedule, or who is playing today, execute the query and format the matchups clearly by conference and venue.
- If a 401 Unauthorized error is returned, remind the user to set their `CFBD_API_KEY` environment variable when running the server.
