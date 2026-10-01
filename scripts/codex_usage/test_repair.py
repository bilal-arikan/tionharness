"""Regression tests for cumulative usage repair, isolated from live data."""

import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

from codex_usage.repair import plan_repair
from codex_usage.artifacts import plan_artifact_repair


class RepairTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.store = self.root / "workspaces/WS1/store"
        self.session = self.store / "sessions/SES1"
        self.rollout = self.root / "rollout.jsonl"
        self.write(self.session / "session.json", {"id": "SES1", "agentId": "AGT1", "cliSessionId": "thread-1"})
        self.messages = [
            {"role": "user", "text": "Keep this content", "createdAt": 1},
            {"role": "assistant", "text": "First", "model": "test", "createdAt": 1, "usage": {"in": 20, "out": 10, "cacheRead": 80}},
            {"role": "assistant", "text": "Second", "model": "test", "createdAt": 2, "usage": {"in": 40, "out": 30, "cacheRead": 260}},
        ]
        self.jsonl(self.session / "messages.jsonl", self.messages)
        self.jsonl(self.session / "debug.jsonl", [
            {"type": "tool_call", "input": "Keep this tool trace"},
            {"type": "llm_call", "kind": "chat", "in": 20, "out": 10, "cacheRead": 80},
            {"type": "llm_call", "kind": "chat", "in": 40, "out": 30, "cacheRead": 260},
        ])
        self.write(self.store / "session-usage/SES1.json", self.rollup(60, 40, 340, 2))
        self.write(self.store / "usage/AGT1__1970-01-01.json", self.rollup(110, 57, 350, 3))
        rows = [{"type": "session_meta", "payload": {"id": "thread-1"}}]
        for turn, usages in enumerate([[(50, 40, 5), (50, 40, 5)], [(70, 60, 7), (70, 60, 7), (60, 60, 6)]]):
            rows.append({"type": "event_msg", "payload": {"type": "task_started"}})
            for call, (input_tokens, cached, output) in enumerate(usages):
                rows.append({"type": "token_usage_record", "payload": {"response_id": f"{turn}-{call}", "usage": {"input_tokens": input_tokens, "cached_input_tokens": cached, "output_tokens": output}}})
            rows.append({"type": "event_msg", "payload": {"type": "task_complete"}})
        self.jsonl(self.rollout, rows)

    def write(self, path, data):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(data), encoding="utf-8")

    def jsonl(self, path, rows):
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("".join(json.dumps(row) + "\n" for row in rows), encoding="utf-8")

    def rollup(self, incoming, outgoing, cached, calls):
        bucket = {"calls": calls, "inputTokens": incoming, "outputTokens": outgoing, "cacheReadTokens": cached}
        return {**bucket, "agentId": "AGT1", "providerCalls": calls, "byKind": {"chat": dict(bucket)}, "byModel": {"codex-cli|test": dict(bucket)}}

    def plan(self):
        return plan_repair(self.root, "WS1", "SES1", self.rollout)

    def test_backup_repair_preserves_other_usage_and_is_idempotent(self):
        files, summary = self.plan()
        self.assertEqual(summary["providerCalls"], 5)
        spec = importlib.util.spec_from_file_location("repair_cli", Path(__file__).parents[1] / "repair-codex-usage.py")
        cli = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(cli)
        old_messages = (self.session / "messages.jsonl").read_bytes()
        backup = Path(cli.apply_plan(self.root, files))
        self.assertEqual((backup / (self.session / "messages.jsonl").relative_to(self.root)).read_bytes(), old_messages)
        repaired = [json.loads(line) for line in (self.session / "messages.jsonl").read_text().splitlines()]
        self.assertEqual(repaired[0], self.messages[0])
        self.assertEqual(repaired[2]["usage"], {"in": 20, "out": 20, "cacheRead": 180})
        daily = json.loads((self.store / "usage/AGT1__1970-01-01.json").read_text())
        self.assertEqual((daily["inputTokens"], daily["outputTokens"], daily["cacheReadTokens"], daily["providerCalls"]), (90, 47, 270, 6))
        self.assertEqual(self.plan(), ({}, {"changed": False, "providerCalls": 5}))

    def test_rejects_unverified_identity_and_rollup(self):
        self.write(self.session / "session.json", {"id": "SES1", "agentId": "AGT1", "cliSessionId": "another-thread"})
        with self.assertRaisesRegex(ValueError, "another CLI session"):
            self.plan()
        self.write(self.session / "session.json", {"id": "SES1", "agentId": "AGT1", "cliSessionId": "thread-1"})
        self.write(self.store / "session-usage/SES1.json", self.rollup(999, 40, 340, 2))
        with self.assertRaisesRegex(ValueError, "rollup does not match"):
            self.plan()

    def test_rejects_unknown_usage_without_writing(self):
        self.messages[2]["usage"]["out"] = 999
        self.jsonl(self.session / "messages.jsonl", self.messages)
        before = (self.session / "messages.jsonl").read_bytes()
        with self.assertRaisesRegex(ValueError, "neither"):
            self.plan()
        self.assertEqual((self.session / "messages.jsonl").read_bytes(), before)

    def test_rejects_tokens_repaired_without_provider_counters(self):
        self.messages[2]["usage"] = {"in": 20, "out": 20, "cacheRead": 180}
        self.jsonl(self.session / "messages.jsonl", self.messages)
        self.write(self.store / "session-usage/SES1.json", self.rollup(40, 30, 260, 2))
        with self.assertRaisesRegex(ValueError, "Partially repaired provider"):
            self.plan()

    def test_recovers_owned_file_artifact_without_changing_the_project(self):
        row = {"id": "ART1", "sessionId": "SES1", "kind": "file", "content": "Latest document", "sourcePath": "artifacts/SES1/old.md"}
        path = self.store / "artifacts/ART1.json"
        self.write(path, row)
        source = self.store.parent / "workspace/artifacts/SES1/old.md"
        source.parent.mkdir(parents=True)
        source.write_text("Old revision", encoding="utf-8")
        files, recovered = plan_artifact_repair(self.root, "WS1", "SES1")
        self.assertEqual(recovered, ["ART1"])
        self.assertEqual(files[source], "Latest document")
        self.assertEqual(json.loads(files[path])["content"], "")
        for target, body in files.items():
            target.write_text(body, encoding="utf-8")
        self.assertEqual(plan_artifact_repair(self.root, "WS1", "SES1"), ({}, []))

    def test_rejects_shared_artifact_sources(self):
        source = "artifacts/SES1/shared.md"
        for ident in ("ART1", "ART2"):
            self.write(self.store / f"artifacts/{ident}.json", {"id": ident, "sessionId": "SES1", "kind": "file", "content": "new", "sourcePath": source})
        with self.assertRaisesRegex(ValueError, "shared"):
            plan_artifact_repair(self.root, "WS1", "SES1")


if __name__ == "__main__":
    unittest.main()
