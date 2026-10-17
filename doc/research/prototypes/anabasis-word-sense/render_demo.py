"""THROWAWAY: render a single offline evidence/review file; no server needed."""

import json
from pathlib import Path

HERE = Path(__file__).resolve().parent
text = (HERE / "demo-template.html").read_text()
for marker, name in (
    ("INPUTS", "inputs.json"),
    ("PREDICTIONS", "predictions.json"),
    ("PROPOSALS", "review-proposals.json"),
):
    path = HERE / name
    payload = json.loads(path.read_text()) if path.exists() else None
    safe = (
        json.dumps(payload, ensure_ascii=False)
        .replace("<", "\\u003c")
        .replace("\u2028", "\\u2028")
        .replace("\u2029", "\\u2029")
    )
    text = text.replace("/*" + marker + "*/null", safe)
(HERE / "evidence.html").write_text(text)
print(
    "Open doc/research/prototypes/anabasis-word-sense/evidence.html directly in a browser."
)
