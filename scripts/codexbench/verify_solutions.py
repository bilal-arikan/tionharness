"""Independently check every saved graph solution on all three-node graphs."""
import json
import subprocess
import sys
import tempfile
from pathlib import Path

oracle = '''from itertools import product
from solution import dependency_layers
nodes = ('a', 'b', 'c')
count = 0
for bits in product((False, True), repeat=9):
    graph = {node: [dep for j, dep in enumerate(nodes) if bits[i*3+j]] for i, node in enumerate(nodes)}
    remaining = set(nodes)
    expected = []
    while remaining:
        ready = sorted(node for node in remaining if not remaining.intersection(graph[node]))
        if not ready:
            expected = None
            break
        expected.append(ready)
        remaining.difference_update(ready)
    try:
        actual = dependency_layers(graph)
    except ValueError:
        assert expected is None, graph
    else:
        assert expected is not None and actual == expected, (graph, actual, expected)
    count += 1
print(count)
'''

path = Path(sys.argv[1])
data = json.loads(path.read_text(encoding="utf-8"))
for row in data["results"]:
    if row["task"] != "dependency_layers":
        continue
    with tempfile.TemporaryDirectory(prefix="tion-bench-oracle-") as temp:
        work = Path(temp)
        (work / "solution.py").write_text(row["solution"], encoding="utf-8")
        (work / "oracle.py").write_text(oracle, encoding="utf-8")
        check = subprocess.run([sys.executable, "-B", "oracle.py"], cwd=work,
                               capture_output=True, text=True, timeout=15)
    row["supplementalGraphOracle"] = {
        "passed": check.returncode == 0 and check.stdout.strip() == "512",
        "cases": 512,
        "output": check.stdout + check.stderr,
    }
path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
if not all(r.get("supplementalGraphOracle", {"passed": True})["passed"] for r in data["results"]):
    raise SystemExit("Supplemental graph verification failed")
