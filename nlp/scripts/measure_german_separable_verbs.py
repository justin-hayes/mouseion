#!/usr/bin/env python3
"""Generate precision evidence for German separable-verb reattachment.

Exit status:
- 0: the run completed and the report was written.
- 3: the German Stanza model is unavailable, so nothing was measured and no
  report was written. The committed report is left untouched. Callers that
  treat a missing model as acceptable must check for this status explicitly;
  any other non-zero status is a failure.
- 2: invalid arguments (argparse).

The measurement itself lives in mouseion_nlp.separable_measurement.
"""

from __future__ import annotations

import argparse
from pathlib import Path

from mouseion_nlp.separable_measurement import run


DEFAULT_INPUT = Path("nlp/testdata/german_prose_werther_excerpt.txt")
DEFAULT_OUTPUT = Path("doc/evidence/german-separable-verb-precision.md")


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    return run(args.input, args.output)


if __name__ == "__main__":
    raise SystemExit(main())
