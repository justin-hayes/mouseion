"""THROWAWAY: retain source-row/depth identity, including identical gloss text.

Shared gloss prefixes are evidence, not proof of equivalent construction senses.
Run directly to rebuild only the derived Kaikki candidates from the committed,
unchanged raw source_senses; no download or new dictionary source is introduced.
"""

import json
from pathlib import Path


def candidate_paths(source_senses):
    paths = []
    for source in source_senses:
        for depth in range(1, len(source.get("glosses", [])) + 1):
            paths.append(
                {
                    "id": source["id"] + f":path-{depth}",
                    "ordinal": len(paths) + 1,
                    "glosses": source["glosses"][:depth],
                    "source_paths": [{"source_sense_id": source["id"], "depth": depth}],
                    "tags": source.get("tags", [])
                    if depth == len(source["glosses"])
                    else [],
                    "raw_glosses": source.get("raw_glosses", [])[:depth],
                    "identity_kind": "distinct pinned raw-row/depth path, not a source-provided sense ID",
                }
            )
    return paths


if __name__ == "__main__":
    path = Path(__file__).resolve().parent / "inputs.json"
    data = json.loads(path.read_text())
    for entry in data["kaikki"]:
        entry["senses"] = candidate_paths(entry["source_senses"])
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n")
    print("Rebuilt only derived paths from retained source rows; no sense gold read.")
