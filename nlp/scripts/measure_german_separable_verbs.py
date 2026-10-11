#!/usr/bin/env python3
"""Generate precision evidence for German separable-verb reattachment.

Exit status:
- 0: the report was written.
- EXIT_SKIPPED (3): the German Stanza model is unavailable, so no report was
  written. Callers that accept a missing model must check for this status.
- Any other non-zero status is a failure.
"""

from __future__ import annotations

import argparse
import hashlib
from pathlib import Path
import sys

from mouseion_nlp import Producer, SourceDocument
from mouseion_nlp.precision import measure, render_report


SOURCE_TITLE = "Die Leiden des jungen Werther — Band 1"
SOURCE_URL = "https://www.gutenberg.org/ebooks/2407"
DEFAULT_INPUT = Path("nlp/testdata/german_prose_werther_excerpt.txt")
DEFAULT_OUTPUT = Path("doc/evidence/german-separable-verb-precision.md")
EXIT_COMPLETED = 0
EXIT_SKIPPED = 3


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", type=Path, default=DEFAULT_INPUT)
    parser.add_argument("--output", type=Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()

    text = args.input.read_text(encoding="utf-8")
    producer = Producer()
    try:
        producer.warmup("de")
    except Exception as error:  # noqa: BLE001 - model availability is the gate
        print(f"SKIP: German Stanza model unavailable: {error}", file=sys.stderr)
        return EXIT_SKIPPED

    corpus = producer.analyze(
        text,
        "de",
        SourceDocument(
            id="werther-excerpt",
            source_identifier=SOURCE_URL,
            title=SOURCE_TITLE,
        ),
    )
    report = render_report(
        measure(corpus),
        source_title=SOURCE_TITLE,
        source_url=SOURCE_URL,
        source_sha256=hashlib.sha256(text.encode("utf-8")).hexdigest(),
        stanza_version=producer.model_version("de"),
        generator_command="PYTHONPATH=nlp/src:gen/python .venv/bin/python nlp/scripts/measure_german_separable_verbs.py",
    )
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(report, encoding="utf-8")
    print(f"Wrote {args.output}")
    return EXIT_COMPLETED


if __name__ == "__main__":
    raise SystemExit(main())
