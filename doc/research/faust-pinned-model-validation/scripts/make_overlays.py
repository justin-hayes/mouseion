"""Generate scratch go -overlay JSON files for the newline-clause ablation (scratch only).

Nothing in the repository is modified. The candidate knockout is derived from the repo's
committed internal/cardexport/cardexport.go (git HEAD): the single newline clause
    if strings.Contains(trimmed, "\n") || 
is required to occur exactly once, is removed in a scratch copy, and the copy is checked to
differ from the original by exactly that clause. The probe test file is copied from this
directory into the work directory and injected through the overlay, never added to the repo.

Usage: python3 -I make_overlays.py <repo> <work_dir>
Writes: <work_dir>/overlay_base.json, <work_dir>/overlay_knockout.json,
        <work_dir>/cardexport_knockout.go, <work_dir>/zz_probe_gate_test.go
"""

import json
import os
import shutil
import subprocess
import sys

repo, work = os.path.abspath(sys.argv[1]), os.path.abspath(sys.argv[2])
here = os.path.dirname(os.path.abspath(__file__))
src_rel = "internal/cardexport/cardexport.go"
test_rel = "internal/cardexport/zz_probe_gate_test.go"
CLAUSE = r'if strings.Contains(trimmed, "\n") || '

original = subprocess.run(
    ["git", "-C", repo, "show", f"HEAD:{src_rel}"], check=True, capture_output=True
).stdout.decode("utf-8")
occurrences = original.count(CLAUSE)
if occurrences != 1:
    raise SystemExit(f"expected exactly one newline clause in HEAD:{src_rel}, found {occurrences}")
knockout = original.replace(CLAUSE, "if ", 1)
if len(original) - len(knockout) != len(CLAUSE) - len("if "):
    raise SystemExit("knockout does not differ by exactly the newline clause")

os.makedirs(work, exist_ok=True)
knockout_path = os.path.join(work, "cardexport_knockout.go")
test_path = os.path.join(work, "zz_probe_gate_test.go")
with open(knockout_path, "wb") as sink:
    sink.write(knockout.encode("utf-8"))
shutil.copyfile(os.path.join(here, "zz_probe_gate_test.go.txt"), test_path)

repo_src = os.path.join(repo, src_rel)
repo_test = os.path.join(repo, test_rel)
base = {"Replace": {repo_test: test_path}}
knock = {"Replace": {repo_src: knockout_path, repo_test: test_path}}
for name, body in (("overlay_base.json", base), ("overlay_knockout.json", knock)):
    with open(os.path.join(work, name), "w", encoding="utf-8") as sink:
        json.dump(body, sink, indent=1)
print("newline clause removed once; knockout differs from HEAD by that clause only")
print("wrote overlays to", work)
