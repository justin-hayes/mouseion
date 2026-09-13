"""Measurement helpers for German separable-verb reattachment evidence."""

from __future__ import annotations

from collections import Counter
from dataclasses import dataclass
from typing import Any

from .producer import GERMAN_SEPARABLE_PREFIXES, Producer


# These prefixes have common non-particle uses in ordinary prose. They are not
# rejected by the producer, but are called out for a human review of the sample.
DEBATABLE_PREFIXES = frozenset(
    {
        "durch",
        "gegen",
        "hinter",
        "über",
        "um",
        "unter",
        "vor",
        "wieder",
    }
)


@dataclass(frozen=True)
class Reattachment:
    """One changed verb token and its complete sentence context."""

    sentence_number: int
    token_number: int
    sentence: str
    particles: tuple[str, ...]
    raw_lemma: str
    full_lemma: str
    review_flag: str


@dataclass(frozen=True)
class Measurement:
    """Deterministic counts and rows extracted from one normalized corpus."""

    sentence_count: int
    token_count: int
    reattachment_count: int
    accepted_particle_count: int
    skipped_particle_count: int
    prefix_counts: tuple[tuple[str, int], ...]
    lemma_pairs: tuple[tuple[str, str, int], ...]
    spot_checks: tuple[Reattachment, ...]


def measure(corpus: Any) -> Measurement:
    """Measure producer reattachments in a normalized corpus artifact."""
    prefix_counts: Counter[str] = Counter()
    lemma_pairs: Counter[tuple[str, str]] = Counter()
    spot_checks: list[Reattachment] = []
    reattachment_count = 0
    accepted_particle_count = 0
    skipped_particle_count = 0
    token_count = 0

    for sentence_number, sentence in enumerate(corpus.sentences, start=1):
        tokens = list(sentence.tokens)
        token_count += len(tokens)
        for token_number, verb in enumerate(tokens, start=1):
            if verb.pos != "VERB":
                continue
            particles = [
                token
                for token_index, token in enumerate(tokens)
                if token.head == token_number - 1
                and token.dependency == "compound:prt"
                and token_index != token_number - 1
            ]
            accepted = [
                token
                for token in particles
                if token.surface.casefold() in GERMAN_SEPARABLE_PREFIXES
            ]
            skipped_particle_count += len(particles) - len(accepted)
            if not accepted:
                continue

            accepted_particle_count += len(accepted)
            for particle in accepted:
                prefix_counts[particle.surface.casefold()] += 1

            verb_index = token_number - 1
            accepted_ids = {id(token) for token in accepted}
            particle_distance = {
                id(token): abs(token_index - verb_index)
                for token_index, token in enumerate(tokens)
                if id(token) in accepted_ids
            }

            normalized_raw = Producer._canonical_lemma("de", verb.raw_lemma)
            if verb.canonical_lemma == normalized_raw:
                continue

            reattachment_count += 1
            lemma_pairs[(verb.raw_lemma, verb.canonical_lemma)] += 1
            spot_checks.append(
                Reattachment(
                    sentence_number=sentence_number,
                    token_number=token_number,
                    sentence=" ".join(sentence.text.split()),
                    particles=tuple(token.surface for token in accepted),
                    raw_lemma=verb.raw_lemma,
                    full_lemma=verb.canonical_lemma,
                    review_flag=(
                        "debatable"
                        if any(
                            token.surface.casefold() in DEBATABLE_PREFIXES
                            or particle_distance[id(token)] > 8
                            for token in accepted
                        )
                        else ""
                    ),
                )
            )

    return Measurement(
        sentence_count=len(corpus.sentences),
        token_count=token_count,
        reattachment_count=reattachment_count,
        accepted_particle_count=accepted_particle_count,
        skipped_particle_count=skipped_particle_count,
        prefix_counts=tuple(sorted(prefix_counts.items())),
        lemma_pairs=tuple(
            (raw_lemma, full_lemma, count)
            for (raw_lemma, full_lemma), count in sorted(lemma_pairs.items())
        ),
        spot_checks=tuple(spot_checks),
    )


def render_report(
    measurement: Measurement,
    *,
    source_title: str,
    source_url: str,
    source_sha256: str,
    stanza_version: str,
    generator_command: str,
) -> str:
    """Render the committed Markdown evidence report."""
    accepted = measurement.accepted_particle_count
    skipped = measurement.skipped_particle_count
    observed = accepted + skipped
    review_rows = sum(bool(row.review_flag) for row in measurement.spot_checks)

    lines = [
        "# German Separable-Verb Precision Evidence",
        "",
        "This report is generated by the real Mouseion NLP producer over a",
        "committed public-domain German prose sample. The producer's closed",
        "prefix guard accepts a particle only when its surface is in the curated",
        "German separable-prefix set.",
        "",
        "## Provenance",
        "",
        f"- Source: [{source_title}]({source_url})",
        f"- Sample SHA-256: `{source_sha256}`",
        f"- Stanza version: `{stanza_version}`",
        f"- Regenerate: `{generator_command}`",
        "- An unavailable Stanza German model causes the generator to print `SKIP` and exit successfully; it does not replace this committed report.",
        "",
        "## Counts",
        "",
        "| Measure | Count |",
        "| --- | ---: |",
        f"| Sentences | {measurement.sentence_count} |",
        f"| Tokens | {measurement.token_count} |",
        f"| Reattachment events (changed verb tokens) | {measurement.reattachment_count} |",
        f"| Accepted particle observations | {accepted} |",
        f"| Skipped particle observations | {skipped} |",
        f"| Particle observations reviewed by the guard | {observed} |",
        f"| Spot-check rows flagged debatable | {review_rows} |",
        "",
        "`Skipped particle observations` are `compound:prt` dependents whose",
        "surface was not in the curated prefix set. The reattachment count is",
        "the number of verb tokens whose canonical lemma changed; a verb with",
        "multiple accepted particles still counts as one event.",
        "",
        "## Prefix Breakdown",
        "",
        "| Prefix | Accepted observations |",
        "| --- | ---: |",
    ]
    lines.extend(f"| `{prefix}` | {count} |" for prefix, count in measurement.prefix_counts)
    lines.extend(
        [
            "",
            "## Reattached Lemma Pairs",
            "",
            "| Base lemma | Full lemma | Occurrences |",
            "| --- | --- | ---: |",
        ]
    )
    lines.extend(
        f"| `{raw_lemma}` | `{full_lemma}` | {count} |"
        for raw_lemma, full_lemma, count in measurement.lemma_pairs
    )
    lines.extend(
        [
            "",
            "## Spot Checks",
            "",
            "Each row contains the complete Stanza sentence context. `debatable`",
            "flags prefixes with frequent non-particle uses; blank flags are not",
            "an assertion that the row is correct. Reviewers can determine the",
            "false-positive rate by judging these rows and counting any false",
            "positives against the reattachment-event count above.",
            "",
            "| Sentence | Particles | Base -> full lemma | Review flag | Full sentence context |",
            "| ---: | --- | --- | --- | --- |",
        ]
    )
    for row in measurement.spot_checks:
        context = row.sentence.replace("|", "\\|")
        particles = ", ".join(row.particles)
        lines.append(
            f"| {row.sentence_number} | `{particles}` | `{row.raw_lemma}` -> `{row.full_lemma}` | "
            f"{row.review_flag} | {context} |"
        )
    lines.append("")
    return "\n".join(lines)
