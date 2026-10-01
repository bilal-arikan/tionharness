"""Read authoritative Codex per-response usage without retaining prompt bodies."""

import json


def read_turns(path, expected_id=None):
    turns = []
    current = None
    seen = set()
    session_id = None
    with open(path, encoding="utf-8") as stream:
        for line in stream:
            row = json.loads(line)
            payload = row.get("payload", {})
            if row.get("type") == "session_meta":
                session_id = payload.get("id")
            if row.get("type") == "event_msg" and payload.get("type") == "task_started":
                if current is not None and not current["completed"]:
                    raise ValueError("Overlapping or incomplete turns")
                current = {"usage": {}, "calls": 0, "completed": False}
                turns.append(current)
            if row.get("type") == "token_usage_record" and current is not None:
                response_id = payload.get("response_id")
                if not response_id:
                    raise ValueError("A measured response has no identity")
                if response_id in seen:
                    continue
                seen.add(response_id)
                for key, value in payload["usage"].items():
                    current["usage"][key] = current["usage"].get(key, 0) + value
                current["calls"] += 1
            if row.get("type") == "event_msg" and payload.get("type") == "task_complete":
                if current is None:
                    raise ValueError("Completion without a started turn")
                current["completed"] = True
    if not turns or any(not turn["completed"] or not turn["calls"] for turn in turns):
        raise ValueError("The rollout must contain only completed, measured turns")
    if expected_id is not None and session_id != expected_id:
        raise ValueError("The rollout belongs to another CLI session")
    for turn in turns:
        usage = turn["usage"]
        if any(not isinstance(value, int) or value < 0 for value in usage.values()):
            raise ValueError("Invalid measured usage")
        if message_usage(turn)["in"] < 0:
            raise ValueError("Cached usage exceeds input usage")
    return turns


def message_usage(turn):
    usage = turn["usage"]
    return {
        "in": usage["input_tokens"] - usage.get("cached_input_tokens", 0) - usage.get("cache_write_input_tokens", 0),
        "out": usage["output_tokens"],
        "cacheRead": usage.get("cached_input_tokens", 0),
        "cacheWrite": usage.get("cache_write_input_tokens", 0),
    }
