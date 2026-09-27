"""Exercise real Codex policy enforcement with local, synthetic model/MCP servers.

No model API or login is used. Both permitted and forbidden calls are attempted
against harmless mock tools. The model response is scripted, not inferred.
"""

import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class ProbeHandler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass

    def reply(self, data, content_type="application/json"):
        self.send_response(200)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        if self.path == "/mcp":
            if "id" not in body:
                self.send_response(202)
                self.end_headers()
                return
            method = body.get("method")
            if method == "initialize":
                result = {"protocolVersion": "2024-11-05", "capabilities": {"tools": {}},
                          "serverInfo": {"name": "probe", "version": "1"}}
            elif method == "tools/list":
                result = {"tools": [{"name": name, "description": name,
                                     "inputSchema": {"type": "object", "properties": {}}}
                                    for name in ["read_item", "read_other", "write_item"]]}
            elif method == "tools/call":
                self.server.calls.append(body["params"]["name"])
                result = {"content": [{"type": "text", "text": "Mock call completed"}]}
            else:
                result = {}
            self.reply(json.dumps({"jsonrpc": "2.0", "id": body["id"], "result": result}).encode())
            return
        self.server.requests.append(body)
        if len(self.server.requests) == 1:
            code = """text({names: ALL_TOOLS.map(t => t.name), shell: typeof tools.exec_command, stdin: typeof tools.write_stdin});
for (const name of ['mcp__probe__read_item', 'mcp__probe__write_item']) {
  try { await tools[name]({}); text({name, called: true}); }
  catch (error) { text({name, called: false, error: String(error)}); }
}"""
            item = {"type": "custom_tool_call", "id": "ctc_probe", "call_id": "call_probe",
                    "name": "exec", "namespace": "functions", "input": code}
        else:
            item = {"type": "message", "id": "msg_probe", "role": "assistant", "status": "completed",
                    "content": [{"type": "output_text", "text": "Probe complete", "annotations": []}]}
        events = [
            {"type": "response.created", "response": {"id": "resp_probe", "output": []}},
            {"type": "response.output_item.added", "output_index": 0, "item": item},
            {"type": "response.output_item.done", "output_index": 0, "item": item},
            {"type": "response.completed", "response": {"id": "resp_probe", "status": "completed",
             "output": [item], "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}}},
        ]
        self.reply("".join("data: " + json.dumps(event) + "\n\n" for event in events).encode(), "text/event-stream")


def run_case(binary, model, name, policy, shell, expected, server):
    server.requests, server.calls = [], []
    with tempfile.TemporaryDirectory(prefix="codex-policy-") as directory:
        home, work = Path(directory) / "home", Path(directory) / "work"
        home.mkdir()
        work.mkdir()
        cache = Path.home() / ".codex/models_cache.json"
        if cache.exists():
            shutil.copy2(cache, home / "models_cache.json")
        config = f'''model_provider = "probe"
web_search = "disabled"
[model_providers.probe]
name = "Local policy probe"
base_url = "http://127.0.0.1:{server.server_port}/v1"
wire_api = "responses"
requires_openai_auth = false
experimental_bearer_token = "local-probe"
request_max_retries = 0
stream_max_retries = 0
[features]
shell_tool = {str(shell).lower()}
multi_agent = false
multi_agent_v2 = false
[agents]
enabled = false
[tools]
experimental_request_user_input = {{ enabled = false }}
[mcp_servers.probe]
url = "http://127.0.0.1:{server.server_port}/mcp"
required = true
default_tools_approval_mode = "approve"
{policy}
'''
        (home / "config.toml").write_text(config, encoding="utf-8")
        # Desktop's CODEX_CI can suppress MCP startup in child CLIs.
        env = {k: v for k, v in os.environ.items() if not k.startswith("CODEX_") and k != "OPENAI_API_KEY"}
        env["CODEX_HOME"] = str(home)
        result = subprocess.run(
            [binary, "exec", "--json", "--ephemeral", "--skip-git-repo-check", "--strict-config",
             "--dangerously-bypass-approvals-and-sandbox", "-m", model, "-C", str(work), "Run the local policy probe."],
            env=env, stdin=subprocess.DEVNULL, capture_output=True, text=True, encoding="utf-8", timeout=45,
        )
        assert result.returncode == 0, (name, result.stderr, result.stdout)
        assert len(server.requests) == 2, (name, "expected scripted tool round trip")
        outputs = [part["text"] for item in server.requests[-1]["input"]
                   if item.get("type") == "custom_tool_call_output"
                   for part in item["output"] if part.get("type") == "input_text"]
        catalog = next(json.loads(text) for text in outputs if text.startswith('{"names":'))
        actual = sorted(n.removeprefix("mcp__probe__") for n in catalog["names"] if n.startswith("mcp__probe__"))
        assert actual == sorted(expected), (name, actual, expected)
        assert catalog["shell"] == ("function" if shell else "undefined"), catalog
        assert catalog["stdin"] == ("function" if shell else "undefined"), catalog
        assert "apply_patch" in catalog["names"], "native file editing must remain available"
        calls = sorted(server.calls)
        assert calls == sorted(set(expected) & {"read_item", "write_item"}), (name, calls)
        return {"model": model, "case": name, "native_shell": shell, "visible_mcp_tools": actual,
                "executed_mcp_tools": calls, "passed": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--codex", required=True, help="Path to the real Codex executable")
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    all_tools = ["read_item", "read_other", "write_item"]
    cases = [
        ("baseline", "", True, all_tools),
        ("bridged_shell", "", False, all_tools),
        ("exact_allow", 'enabled_tools = ["read_item"]', False, ["read_item"]),
        ("exact_deny", 'disabled_tools = ["write_item"]', False, ["read_item", "read_other"]),
        ("deny_wins", 'enabled_tools = ["read_item"]\ndisabled_tools = ["read_item"]', False, []),
        ("empty_allow", 'enabled_tools = []', False, []),
        ("allow_glob_is_literal", 'enabled_tools = ["read_*"]', False, []),
        ("deny_glob_is_literal", 'disabled_tools = ["write_*"]', False, all_tools),
    ]
    server = ThreadingHTTPServer(("127.0.0.1", 0), ProbeHandler)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    rows = []
    try:
        for model in ["gpt-6-astra", "gpt-6-sol"]:
            for case in cases:
                row = run_case(args.codex, model, *case, server)
                rows.append(row)
                print(f"PASS {model}: {case[0]}", flush=True)
    finally:
        server.shutdown()
        server.server_close()
        thread.join()
    version = subprocess.check_output([args.codex, "--version"], text=True).strip()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"version": version, "synthetic_model": True, "results": rows}, indent=2) + "\n", encoding="utf-8")


if __name__ == "__main__":
    main()
