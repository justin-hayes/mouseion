"""Run the German separable-verb precision measurement and write its report."""

from __future__ import annotations

import hashlib
import sys
from pathlib import Path

from mouseion_nlp import Producer, SourceDocument
from mouseion_nlp.precision import ReportProvenance, measure, render_report

SOURCE_TITLE = "Die Leiden des jungen Werther — Band 1"
SOURCE_URL = "https://www.gutenberg.org/ebooks/2407"
GENERATOR_COMMAND = (
    "PYTHONPATH=nlp/src:gen/python .venv/bin/python nlp/scripts/measure_german_separable_verbs.py"
)
EXIT_OK = 0
EXIT_SKIPPED = 3


def run(input_path: Path, output_path: Path) -> int:
    text = input_path.read_text(encoding="utf-8")
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
        ReportProvenance(
            source_title=SOURCE_TITLE,
            source_url=SOURCE_URL,
            source_sha256=hashlib.sha256(text.encode("utf-8")).hexdigest(),
            stanza_version=producer.model_version("de"),
            generator_command=GENERATOR_COMMAND,
        ),
    )
    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(report, encoding="utf-8")
    print(f"Wrote {output_path}")
    return EXIT_OK
