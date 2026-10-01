"""Build a verified, idempotent repair plan for one offline session."""

import datetime
import json
from pathlib import Path

from .rollout import message_usage, read_turns

FIELDS = {"in": "inputTokens", "out": "outputTokens", "cacheRead": "cacheReadTokens", "cacheWrite": "cacheWriteTokens"}


def adjust_rollup(row, correction, calls, model):
    buckets = [row, row["byKind"]["chat"], row["byModel"]["codex-cli|" + model]]
    for bucket in buckets:
        for key, delta in correction.items():
            bucket[key] = bucket.get(key, 0) + delta
            if bucket[key] < 0:
                raise ValueError("Correction would produce negative usage")
    row["providerCalls"] = row.get("providerCalls", row["calls"]) + calls


def plan_repair(data_dir, workspace, session_id, rollout, timezone_hours=3):
    store = Path(data_dir) / "workspaces" / workspace / "store"
    session_dir = store / "sessions" / session_id
    session = json.loads((session_dir / "session.json").read_text(encoding="utf-8"))
    if session.get("id") != session_id or not session.get("cliSessionId"):
        raise ValueError("The target has no verified CLI session identity")
    messages_path = session_dir / "messages.jsonl"
    messages = [json.loads(line) for line in messages_path.read_text(encoding="utf-8").splitlines()]
    assistants = [message for message in messages if message["role"] == "assistant"]
    turns = read_turns(rollout, session["cliSessionId"])
    if len(assistants) != len(turns):
        raise ValueError("Message and rollout turn counts differ; refusing to guess")
    totals = dict.fromkeys(FIELDS, 0)
    correction = dict.fromkeys(FIELDS.values(), 0)
    days = {}
    states = set()
    original_totals = dict.fromkeys(FIELDS.values(), 0)
    tz = datetime.timezone(datetime.timedelta(hours=timezone_hours))
    for message, turn in zip(assistants, turns):
        expected = message_usage(turn)
        old = message.get("usage", {})
        for key, field in FIELDS.items():
            original_totals[field] += old.get(key, 0)
        for key in totals:
            totals[key] += expected[key]
        if all(old.get(key, 0) == expected[key] for key in totals):
            if expected != totals:
                states.add("repaired")
            continue
        if any(old.get(key, 0) != totals[key] for key in totals):
            raise ValueError("Usage is neither the measured turn nor the verified cumulative counter")
        states.add("cumulative")
        day = datetime.datetime.fromtimestamp(message["createdAt"], tz).date().isoformat()
        entry = days.setdefault(day, {"correction": dict.fromkeys(FIELDS.values(), 0), "calls": 0, "model": message["model"]})
        if entry["model"] != message["model"]:
            raise ValueError("Multiple models in a day are not supported by this repair")
        for key, field in FIELDS.items():
            delta = expected[key] - old.get(key, 0)
            correction[field] += delta
            entry["correction"][field] += delta
        message["usage"] = {key: value for key, value in expected.items() if value}
    # The session originally records one provider call per message; measured
    # rollout counts include the internal model/tool loop. Correct it even when
    # the first turn's token totals already match.
    session_usage_path = store / "session-usage" / (session_id + ".json")
    session_usage = json.loads(session_usage_path.read_text(encoding="utf-8"))
    actual_calls = sum(turn["calls"] for turn in turns)
    if len(states) > 1:
        raise ValueError("Partially repaired session; refusing to adjust shared daily totals twice")
    if session_usage.get("agentId") != session.get("agentId"):
        raise ValueError("Session usage belongs to another agent")
    if any(session_usage.get(key, 0) != value for key, value in original_totals.items()):
        raise ValueError("Session rollup does not match its messages")
    recorded_calls = session_usage.get("providerCalls", len(turns))
    if recorded_calls not in (len(turns), actual_calls):
        raise ValueError("Unexpected provider call count")
    if states == {"cumulative"} and recorded_calls == actual_calls:
        raise ValueError("Partially repaired provider counters")
    if states == {"repaired"} and recorded_calls != actual_calls:
        raise ValueError("Partially repaired provider counters; daily call ownership is ambiguous")
    if not any(correction.values()) and session_usage.get("providerCalls") == actual_calls:
        return {}, {"changed": False, "providerCalls": actual_calls}
    for message, turn in zip(assistants, turns):
        day = datetime.datetime.fromtimestamp(message["createdAt"], tz).date().isoformat()
        entry = days.setdefault(day, {"correction": dict.fromkeys(FIELDS.values(), 0), "calls": 0, "model": message["model"]})
        if recorded_calls != actual_calls:
            entry["calls"] += turn["calls"] - 1
    if len({message["model"] for message in assistants}) != 1:
        raise ValueError("Multiple models in the session are not supported by this repair")
    adjust_rollup(session_usage, correction, actual_calls - session_usage.get("providerCalls", len(turns)), assistants[0]["model"])
    files = {
        messages_path: "".join(json.dumps(message, ensure_ascii=False) + "\n" for message in messages),
        session_usage_path: json.dumps(session_usage, ensure_ascii=False, indent=2) + "\n",
    }
    for day, entry in days.items():
        path = store / "usage" / (session_usage["agentId"] + "__" + day + ".json")
        row = json.loads(path.read_text(encoding="utf-8"))
        adjust_rollup(row, entry["correction"], entry["calls"], entry["model"])
        files[path] = json.dumps(row, ensure_ascii=False, indent=2) + "\n"
    debug_path = session_dir / "debug.jsonl"
    debug = [json.loads(line) for line in debug_path.read_text(encoding="utf-8").splitlines()]
    calls = [row for row in debug if row.get("type") == "llm_call" and row.get("kind") == "chat"]
    if len(calls) != len(turns):
        raise ValueError("Debug call and rollout turn counts differ")
    for row, turn in zip(calls, turns):
        row.update(message_usage(turn))
        row["think"] = turn["usage"].get("reasoning_output_tokens", 0)
        row["calls"] = turn["calls"]
    files[debug_path] = "".join(json.dumps(row, ensure_ascii=False) + "\n" for row in debug)
    return files, {"changed": True, "providerCalls": actual_calls, "tokens": sum(totals.values()), "files": len(files)}
