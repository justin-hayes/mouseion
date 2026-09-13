from types import SimpleNamespace

from mouseion_nlp.precision import measure, render_report


def token(surface, lemma, pos, *, head=0, dependency="root"):
    return SimpleNamespace(
        surface=surface,
        raw_lemma=lemma,
        canonical_lemma=lemma,
        pos=pos,
        head=head,
        dependency=dependency,
    )


def test_measure_counts_changed_verbs_and_skipped_particles() -> None:
    verb = token("stehe", "stehen", "VERB")
    verb.canonical_lemma = "aufstehen"
    corpus = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Ich stehe auf zack.",
                tokens=[
                    token("Ich", "ich", "PRON", head=1, dependency="nsubj"),
                    verb,
                    token("auf", "auf", "ADV", head=1, dependency="compound:prt"),
                    token("zack", "zack", "ADV", head=1, dependency="compound:prt"),
                ],
            )
        ]
    )

    result = measure(corpus)

    assert result.reattachment_count == 1
    assert result.accepted_particle_count == 1
    assert result.skipped_particle_count == 1
    assert result.prefix_counts == (("auf", 1),)
    assert result.lemma_pairs == (("stehen", "aufstehen", 1),)
    assert result.spot_checks[0].sentence == "Ich stehe auf zack."


def test_render_report_explains_review_flags_and_denominator() -> None:
    corpus = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Er steht vor.",
                tokens=[
                    token("steht", "stehen", "VERB"),
                    token("vor", "vor", "ADV", head=0, dependency="compound:prt"),
                ],
            )
        ]
    )
    corpus.sentences[0].tokens[0].canonical_lemma = "vorstehen"

    report = render_report(
        measure(corpus),
        source_title="Sample",
        source_url="https://example.test/sample",
        source_sha256="abc",
        stanza_version="1.14.0",
        generator_command="generate",
    )

    assert "Reattachment events (changed verb tokens)" in report
    assert "Accepted particle observations" in report
    assert "debatable" in report
    assert "Er steht vor." in report
