from types import SimpleNamespace

from mouseion_nlp.precision import ReportProvenance, measure, render_report


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


def test_measure_counts_recovered_advmod_particles() -> None:
    verb = token("sieht", "sehen", "VERB")
    verb.canonical_lemma = "fernsehen"
    corpus = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.",
                tokens=[
                    token("Großmutter", "Großmutter", "NOUN", head=1, dependency="nsubj"),
                    verb,
                    token("im", "in", "ADP", head=3, dependency="case"),
                    token("Wohnzimmer", "Wohnzimmer", "NOUN", head=1, dependency="obl"),
                    token("fern", "fern", "ADV", head=1, dependency="advmod"),
                    token("ohne", "ohne", "ADP", head=6, dependency="case"),
                    token("Bild", "Bild", "NOUN", head=1, dependency="obl"),
                    token("und", "und", "CCONJ", head=6, dependency="cc"),
                    token("Ton", "Ton", "NOUN", head=6, dependency="conj"),
                    token(".", ".", "PUNCT", head=1, dependency="punct"),
                ],
            )
        ]
    )

    result = measure(corpus)

    assert result.advmod_recovery_count == 1
    assert result.accepted_particle_count == 0
    assert result.skipped_particle_count == 0
    assert result.reattachment_count == 1
    assert result.lemma_pairs == (("sehen", "fernsehen", 1),)
    assert result.spot_checks[0].particles == ("fern",)
    assert result.spot_checks[0].review_flag == "advmod-recovered"


def test_measure_leaves_true_adverbs_and_unrecovered_particles_alone() -> None:
    adverbial = token("liegt", "liegen", "VERB")
    adverbial.canonical_lemma = "liegen"
    unrecovered = token("steht", "stehen", "VERB")
    unrecovered.canonical_lemma = "stehen"
    corpus = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Das Dorf liegt fern der Stadt.",
                tokens=[
                    token("Das", "der", "DET", head=1, dependency="det"),
                    token("Dorf", "Dorf", "NOUN", head=2, dependency="nsubj"),
                    adverbial,
                    token("fern", "fern", "ADV", head=2, dependency="advmod"),
                    token("der", "der", "DET", head=5, dependency="det"),
                    token("Stadt", "Stadt", "NOUN", head=3, dependency="nmod"),
                ],
            ),
            SimpleNamespace(
                text="Er steht fest.",
                tokens=[
                    token("Er", "er", "PRON", head=1, dependency="nsubj"),
                    unrecovered,
                    token("fest", "fest", "ADV", head=1, dependency="advmod"),
                ],
            ),
        ]
    )

    result = measure(corpus)

    assert result.advmod_recovery_count == 0
    assert result.reattachment_count == 0
    assert result.spot_checks == ()


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
        ReportProvenance(
            source_title="Sample",
            source_url="https://example.test/sample",
            source_sha256="abc",
            stanza_version="1.14.0",
            generator_command="generate",
        ),
    )

    assert "Reattachment events (changed verb tokens)" in report
    assert "Accepted particle observations" in report
    assert "Skipped particle observations" in report
    assert "ADVMod particle recoveries" in report
    assert "debatable" in report
    assert "Er steht vor." in report
