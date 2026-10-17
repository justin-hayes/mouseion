"""SHA256 of a JSON output with volatile fields removed (scratch only).

Drops timing, Torch thread counts, machine-specific resource and provenance paths, so two runs
of the same pinned pipeline on the same inputs can be compared. Raw file digests are not
byte-reproducible because those fields change between runs.

Usage: python3 -I canonical_digest.py <file.json>
"""

import hashlib
import json
import sys

VOLATILE = {"elapsed_seconds", "pipeline", "provenance", "resources_dir", "torch_threads"}


def strip(value):
    if isinstance(value, dict):
        return {k: strip(v) for k, v in value.items() if k not in VOLATILE}
    if isinstance(value, list):
        return [strip(v) for v in value]
    return value


data = json.load(open(sys.argv[1], encoding="utf-8"))
canonical = json.dumps(strip(data), ensure_ascii=False, sort_keys=True, separators=(",", ":"))
print(hashlib.sha256(canonical.encode("utf-8")).hexdigest(), sys.argv[1])
