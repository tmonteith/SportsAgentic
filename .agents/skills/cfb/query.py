#!/usr/bin/env python3
import sys
import json
import urllib.request
import threading

SERVER_URL = "http://localhost:8080/sse"

def call_mcp_tool(tool_name, arguments):
    endpoint_event = threading.Event()
    endpoint = None

    def listen():
        nonlocal endpoint
        try:
            req = urllib.request.Request(SERVER_URL)
            with urllib.request.urlopen(req, timeout=5) as resp:
                for line in resp:
                    l = line.decode("utf-8").strip()
                    if l.startswith("data:"):
                        endpoint = l.split("data:", 1)[1].strip()
                        endpoint_event.set()
        except Exception as e:
            endpoint_event.set()

    t = threading.Thread(target=listen, daemon=True)
    t.start()
    
    if not endpoint_event.wait(timeout=4) or not endpoint:
        return {"error": "Could not connect to SportsAgentic MCP server at http://localhost:8080. Is it running?"}

    post_data = json.dumps({
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
            "name": tool_name,
            "arguments": arguments
        }
    }).encode("utf-8")

    try:
        post_req = urllib.request.Request(endpoint, data=post_data, headers={"Content-Type": "application/json"})
        with urllib.request.urlopen(post_req, timeout=10) as post_resp:
            res = json.loads(post_resp.read().decode("utf-8"))
            if "result" in res and "content" in res["result"]:
                for item in res["result"]["content"]:
                    if item.get("type") == "text":
                        return item.get("text")
            return res
    except Exception as e:
        return {"error": str(e)}

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: query.py <tool_name> [json_arguments]")
        print("Tools: get_cfb_scores, get_team_schedule, get_cfb_rankings")
        sys.exit(1)

    tool = sys.argv[1]
    args = {}
    if len(sys.argv) > 2:
        try:
            args = json.loads(sys.argv[2])
        except Exception:
            args = {}

    result = call_mcp_tool(tool, args)
    if isinstance(result, str):
        print(result)
    else:
        print(json.dumps(result, indent=2))
