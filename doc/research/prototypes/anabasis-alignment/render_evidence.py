"""THROWAWAY: render measured results into a standalone live-review demo."""

import json
from pathlib import Path

root = Path(__file__).resolve().parent
asset = {
    "gold": json.loads((root / "gold.json").read_text()),
    "held_out": json.loads((root / "results.json").read_text()),
    "all_sections": json.loads((root / "results-transductive.json").read_text()),
}
template = (root / "evidence-template.html").read_text()
(root / "evidence.html").write_text(template.replace(
    "/*EVIDENCE*/null", json.dumps(asset, ensure_ascii=False).replace("<", "\\u003c"),
))
print("Standalone evidence demo generated; scores and annotations unchanged.")
