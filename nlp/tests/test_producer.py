from datetime import datetime, timezone
import json
import logging
from pathlib import Path
from types import SimpleNamespace

from google.protobuf.json_format import MessageToDict
from mouseion.v1 import normalized_corpus_pb2
from mouseion_nlp import Producer, SourceDocument


def word(
    text: str,
    lemma: str,
    upos: str,
    feats: str | None,
    start: int,
    end: int,
    *,
    id: int | None = None,
    head: int = 0,
    deprel: str = "root",
):
    return SimpleNamespace(
        text=text,
        lemma=lemma,
        upos=upos,
        feats=feats,
        start_char=start,
        end_char=end,
        id=id,
        head=head,
        deprel=deprel,
    )


def test_maps_one_batch_stanza_result_to_versioned_artifact() -> None:
    calls = []
    stanza_result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Goethe schrieb.",
                ents=[],
                tokens=[
                    SimpleNamespace(
                        words=[word("Goethe", "Goethe", "PROPN", "Case=Nom", 0, 6)], ner="B-PER"
                    ),
                    SimpleNamespace(
                        words=[word("schrieb", "schreiben", "VERB", "Tense=Past", 7, 14)], ner="O"
                    ),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 14, 15)], ner="O"),
                ],
            )
        ]
    )

    def factory(language: str, enable_ner: bool):
        assert (language, enable_ner) == ("de", True)

        def pipeline(text: str):
            calls.append(text)
            return stanza_result

        return pipeline

    artifact = Producer(enable_ner=True, pipeline_factory=factory).analyze(
        "Goethe schrieb.",
        "de",
        SourceDocument("book-1", "opds:42", "Das Buch", "1", "opening"),
        run_id="run-7",
        analyzed_at=datetime(2026, 8, 21, 12, tzinfo=timezone.utc),
    )

    assert calls == ["Goethe schrieb."]
    assert artifact.schema_version == "1.1.0"
    assert artifact.source_documents[0].id == "book-1"
    assert artifact.analysis.analyzer_name == "stanza"
    assert artifact.analysis.analyzed_at == "2026-08-21T12:00:00Z"
    assert [token.surface for token in artifact.sentences[0].tokens] == ["Goethe", "schrieb", "."]
    assert artifact.sentences[0].tokens[1].raw_lemma == "schreiben"
    assert artifact.sentences[0].tokens[1].pos == "VERB"
    assert artifact.sentences[0].tokens[1].morphology == {"Tense": "Past"}
    assert artifact.sentences[0].tokens[0].dependency == "root"
    assert artifact.sentences[0].tokens[0].head == 0
    assert artifact.sentences[0].tokens[0].named_entity == "B-PER"
    assert not artifact.sentences[0].tokens[1].HasField("named_entity")
    assert artifact.sentences[0].tokens[1].location.start_offset == 7
    assert artifact.sentences[0].location.end_offset == 15
    assert artifact.sentences[0].location.chapter == "1"


def test_maps_dependency_heads_through_multiword_tokens() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="L'uomo mangia dell'acqua.",
                tokens=[
                    SimpleNamespace(
                        words=[
                            word("L'", "il", "DET", None, 0, 2, id=1, head=2, deprel="det")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("uomo", "uomo", "NOUN", None, 2, 6, id=2, head=3, deprel="nsubj")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("mangia", "mangiare", "VERB", None, 7, 13, id=3, head=0, deprel="root")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("dell'", "di", "ADP", None, 14, 19, id=4, head=6, deprel="case"),
                            word("dell'", "il", "DET", None, 14, 19, id=5, head=6, deprel="det"),
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("acqua", "acqua", "NOUN", None, 19, 24, id=6, head=3, deprel="obj")
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 24, 25, id=7, head=3, deprel="punct")]
                    ),
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    tokens = producer.analyze("L'uomo mangia dell'acqua.", "it").sentences[0].tokens

    assert [token.dependency for token in tokens] == ["det", "nsubj", "root", "case", "det", "obj", "punct"]
    assert [token.head for token in tokens] == [1, 2, 2, 5, 5, 2, 2]


def test_ner_is_not_emitted_when_disabled() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Berlin",
                tokens=[
                    SimpleNamespace(
                        words=[word("Berlin", "Berlin", "PROPN", None, 0, 6)], ner="S-LOC"
                    )
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)
    token = producer.analyze("Berlin", "de").sentences[0].tokens[0]
    assert not token.HasField("named_entity")


def test_cleans_edge_punctuation_without_changing_offsets_or_internal_punctuation() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="‹die Besten› O'Neill-like",
                tokens=[
                    SimpleNamespace(words=[word("‹die", "die", "DET", None, 0, 4)]),
                    SimpleNamespace(words=[word("Besten›", "gut", "ADJ", None, 5, 12)]),
                    SimpleNamespace(words=[word("O'Neill-like", "O'Neill-like", "PROPN", None, 13, 25)]),
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    artifact = producer.analyze("‹die Besten› O'Neill-like", "de")
    tokens = artifact.sentences[0].tokens

    assert [token.surface for token in tokens] == ["die", "Besten", "O'Neill-like"]
    assert [token.location.start_offset for token in tokens] == [0, 5, 13]
    assert [token.location.end_offset for token in tokens] == [4, 12, 25]


def test_cleans_all_unicode_punctuation_and_symbol_edges() -> None:
    surfaces = ["—Wort—", "$Haus€", "【Baum】", "„O'Neill-like“", "L'", "."]
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text=" ".join(surfaces),
                tokens=[
                    SimpleNamespace(words=[word(surface, surface, "NOUN", None, i, i + len(surface))])
                    for i, surface in enumerate(surfaces)
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    tokens = producer.analyze(" ".join(surfaces), "de").sentences[0].tokens

    assert [token.surface for token in tokens] == ["Wort", "Haus", "Baum", "O'Neill-like", "L'", "."]
    assert [token.canonical_lemma for token in tokens] == [
        "wort", "haus", "baum", "o'neill-like", "l'", "."
    ]


def test_cleans_reported_lemma_boundaries_and_preserves_internal_punctuation() -> None:
    first_text = "Die Versammlung der Phaiaken verfügt damit über eine gewissermaßen ‹passive Souveränität›."
    second_text = "Beide Voraussetzungen wären beispielsweise in Al Mina gegeben, wo in dem entscheidenden Zeitraum griechische Händler und/oder Söldner angesiedelt waren."
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text=first_text,
                tokens=[
                    SimpleNamespace(words=[word("Souveränität›", "Souveränität›", "NOUN", None, 76, 89)])
                ],
            ),
            SimpleNamespace(
                text=second_text,
                tokens=[
                    SimpleNamespace(words=[word("und/oder", "/oder", "CCONJ", None, 117, 125)]),
                ],
            ),
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    artifact = producer.analyze(first_text + " " + second_text, "de")
    first_token = artifact.sentences[0].tokens[0]
    second_tokens = artifact.sentences[1].tokens

    assert first_token.raw_lemma == "Souveränität›"
    assert first_token.surface == "Souveränität"
    assert first_token.canonical_lemma == "souveränität"
    assert (first_token.location.start_offset, first_token.location.end_offset) == (76, 89)
    assert second_tokens[0].raw_lemma == "/oder"
    assert second_tokens[0].surface == "und/oder"
    assert second_tokens[0].canonical_lemma == "oder"
    assert (second_tokens[0].location.start_offset, second_tokens[0].location.end_offset) == (117, 125)


def test_pipe_separated_lemma_uses_first_alternative_and_preserves_raw_lemma(caplog) -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Nausikaa geleitet ihn.",
                tokens=[
                    SimpleNamespace(words=[word("geleitet", "geleiten|leiten", "VERB", None, 9, 17)])
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    with caplog.at_level(logging.WARNING, logger="mouseion_nlp.producer"):
        token = producer.analyze("Nausikaa geleitet ihn.", "de").sentences[0].tokens[0]

    assert token.raw_lemma == "geleiten|leiten"
    assert token.canonical_lemma == "geleiten"
    assert "differing lemma alternatives" in caplog.text


def test_rejects_token_with_no_usable_pipe_lemma(caplog) -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Wort",
                tokens=[SimpleNamespace(words=[word("Wort", " | ", "NOUN", None, 0, 4)])],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    with caplog.at_level(logging.WARNING, logger="mouseion_nlp.producer"):
        sentence = producer.analyze("Wort", "de").sentences[0]

    assert list(sentence.tokens) == []
    assert "no usable lemma alternative" in caplog.text


def test_german_normalization_preserves_modern_sharp_s_and_maps_historical_forms() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
        text="Straße, Maße, Masse, daß, Haß, Eßzimmer",
                tokens=[
                    SimpleNamespace(words=[word("Straße", "Straße", "NOUN", None, 0, 6)]),
                    SimpleNamespace(words=[word("Maße", "Maße", "NOUN", None, 8, 12)]),
                    SimpleNamespace(words=[word("Masse", "Masse", "NOUN", None, 14, 19)]),
                    SimpleNamespace(words=[word("daß", "daß", "SCONJ", None, 21, 24)]),
                    SimpleNamespace(words=[word("Haß", "Haß", "NOUN", None, 26, 29)]),
                    SimpleNamespace(words=[word("Eßzimmer", "Eßzimmer", "NOUN", None, 31, 39)]),
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    artifact = producer.analyze("Straße, Maße, Masse, daß", "de-DE")

    assert [token.canonical_lemma for token in artifact.sentences[0].tokens] == [
        "straße",
        "maße",
        "masse",
        "dass",
        "hass",
        "esszimmer",
    ]
    assert artifact.normalization_profile.name == "german-standard-post-1996"
    assert artifact.normalization_profile.version == "6"


def test_german_separable_verbs_reattach_and_ignore_homographs() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Er ist aufgestanden.",
                tokens=[
                    SimpleNamespace(words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("ist", "sein", "AUX", None, 3, 6, id=2, head=0)]),
                    SimpleNamespace(words=[word("aufgestanden", "aufstehen", "VERB", None, 7, 19, id=3, head=2, deprel="xcomp")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 19, 20, id=4, head=2, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Ich stehe auf.",
                tokens=[
                    SimpleNamespace(words=[word("Ich", "ich", "PRON", None, 0, 3, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("stehe", "stehen", "VERB", None, 4, 9, id=2, head=0)]),
                    SimpleNamespace(words=[word("auf", "auf", "ADV", None, 10, 13, id=3, head=2, deprel="compound:prt")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 13, 14, id=4, head=2, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Er stellt das wieder her.",
                tokens=[
                    SimpleNamespace(words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("stellt", "stellen", "VERB", None, 3, 9, id=2, head=0)]),
                    SimpleNamespace(words=[word("das", "das", "DET", None, 10, 13, id=3, head=2, deprel="det")]),
                    SimpleNamespace(words=[word("wieder", "wieder", "ADV", None, 14, 20, id=4, head=2, deprel="compound:prt")]),
                    SimpleNamespace(words=[word("her", "her", "ADV", None, 21, 24, id=5, head=2, deprel="compound:prt")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 24, 25, id=6, head=2, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Die Tür ist auf.",
                tokens=[
                    SimpleNamespace(words=[word("Die", "die", "DET", None, 0, 3, id=1, head=2, deprel="det")]),
                    SimpleNamespace(words=[word("Tür", "Tür", "NOUN", None, 4, 7, id=2, head=3, deprel="nsubj")]),
                    SimpleNamespace(words=[word("ist", "sein", "AUX", None, 8, 11, id=3, head=0)]),
                    SimpleNamespace(words=[word("auf", "auf", "ADV", None, 12, 15, id=4, head=3, deprel="xcomp")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 15, 16, id=5, head=3, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Er steht auf dem Berg.",
                tokens=[
                    SimpleNamespace(words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("steht", "stehen", "VERB", None, 3, 8, id=2, head=0)]),
                    SimpleNamespace(words=[word("auf", "auf", "ADP", None, 9, 12, id=3, head=5, deprel="case")]),
                    SimpleNamespace(words=[word("dem", "der", "DET", None, 13, 16, id=4, head=5, deprel="det")]),
                    SimpleNamespace(words=[word("Berg", "Berg", "NOUN", None, 17, 21, id=5, head=2, deprel="obl")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 21, 22, id=6, head=2, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Es bleibt nach wie vor.",
                tokens=[
                    SimpleNamespace(words=[word("Es", "es", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("bleibt", "bleiben", "VERB", None, 3, 9, id=2, head=0)]),
                    SimpleNamespace(words=[word("nach", "nach", "ADV", None, 10, 14, id=3, head=2, deprel="advmod")]),
                    SimpleNamespace(words=[word("wie", "wie", "ADV", None, 15, 18, id=4, head=2, deprel="advmod")]),
                    SimpleNamespace(words=[word("vor", "vor", "ADV", None, 19, 22, id=5, head=2, deprel="advmod")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 22, 23, id=6, head=2, deprel="punct")]),
                ],
            ),
            SimpleNamespace(
                text="Er geht zack.",
                tokens=[
                    SimpleNamespace(words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]),
                    SimpleNamespace(words=[word("geht", "gehen", "VERB", None, 3, 7, id=2, head=0)]),
                    SimpleNamespace(words=[word("zack", "zack", "ADV", None, 8, 12, id=3, head=2, deprel="compound:prt")]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 12, 13, id=4, head=2, deprel="punct")]),
                ],
            ),
        ]
    )
    producer = Producer(pipeline_factory=lambda language, enable_ner: lambda text: result)

    artifact = producer.analyze("fixture", "de")

    attached, separated, multiple, predicative, prepositional, fixed, unknown = [
        sentence.tokens for sentence in artifact.sentences
    ]
    assert attached[2].raw_lemma == "aufstehen"
    assert attached[2].canonical_lemma == "aufstehen"
    assert separated[1].raw_lemma == "stehen"
    assert separated[1].canonical_lemma == "aufstehen"
    assert separated[2].canonical_lemma == "auf"
    assert multiple[1].canonical_lemma == "wiederherstellen"
    assert predicative[2].canonical_lemma == "sein"
    assert prepositional[1].canonical_lemma == "stehen"
    assert fixed[1].canonical_lemma == "bleiben"
    assert unknown[1].canonical_lemma == "gehen"
    assert artifact.normalization_profile.version == "6"


def test_normalized_corpus_round_trip() -> None:
    artifact = normalized_corpus_pb2.NormalizedCorpus(
        schema_version="1.1.0",
        language="de",
        sentences=[normalized_corpus_pb2.Sentence(text="Das Haus steht.")],
    )
    assert (
        normalized_corpus_pb2.NormalizedCorpus.FromString(
            artifact.SerializeToString(deterministic=True)
        )
        == artifact
    )


def test_warmup_loads_the_pipeline_for_a_language() -> None:
    loaded: list[str] = []

    def factory(language: str, enable_ner: bool):
        loaded.append(language)

        def pipeline(text: str):
            return text

        return pipeline

    producer = Producer(pipeline_factory=factory)
    assert loaded == []
    producer.warmup("de")
    assert loaded == ["de"]
    # the default factory is lru-cached; an injected one is not, but warmup
    # still loads the pipeline for the given language
    producer.warmup("de")
    assert loaded == ["de", "de"]


def test_italian_warmup_and_analyze_use_the_injected_pipeline() -> None:
    loaded: list[str] = []
    analyzed: list[str] = []
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="La casa.",
                tokens=[
                    SimpleNamespace(words=[word("La", "il", "DET", None, 0, 2)]),
                    SimpleNamespace(words=[word("casa", "casa", "NOUN", None, 3, 7)]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 7, 8)]),
                ],
            )
        ]
    )

    def factory(language: str, enable_ner: bool):
        assert (language, enable_ner) == ("it", False)
        loaded.append(language)

        def pipeline(text: str):
            analyzed.append(text)
            return result

        return pipeline

    producer = Producer(pipeline_factory=factory)
    producer.warmup("it")
    artifact = producer.analyze("La casa.", "it")

    assert loaded == ["it", "it"]
    assert analyzed == ["La casa."]
    assert artifact.language == "it"
    assert [token.raw_lemma for token in artifact.sentences[0].tokens] == ["il", "casa", "."]


def test_italian_linguistic_regression_fixture() -> None:
    """Lock the normalized contract to representative Italian Stanza output."""
    text = "L'uomo e le ragazze bevono dell'acqua. Maria portera`? No, porterà pane e dammelo!"
    sentences = [
        SimpleNamespace(
            text="L'uomo e le ragazze bevono dell'acqua.",
            tokens=[
                SimpleNamespace(words=[word("L'", "il", "DET", "Definite=Def|Gender=Masc|Number=Sing|PronType=Art", 0, 2)], ner="O"),
                SimpleNamespace(words=[word("uomo", "uomo", "NOUN", "Gender=Masc|Number=Sing", 2, 6)], ner="O"),
                SimpleNamespace(words=[word("e", "e", "CCONJ", None, 7, 8)], ner="O"),
                SimpleNamespace(words=[word("le", "il", "DET", "Definite=Def|Gender=Fem|Number=Plur|PronType=Art", 9, 11)], ner="O"),
                SimpleNamespace(words=[word("ragazze", "ragazza", "NOUN", "Gender=Fem|Number=Plur", 12, 19)], ner="O"),
                SimpleNamespace(words=[word("bevono", "bere", "VERB", "Mood=Ind|Number=Plur|Person=3|Tense=Pres|VerbForm=Fin", 20, 26)], ner="O"),
                SimpleNamespace(words=[word("dell'", "di", "ADP", None, 27, 32), word("dell'", "il", "DET", "Definite=Def|Gender=Fem|Number=Sing|PronType=Art", 27, 32)], ner="O"),
                SimpleNamespace(words=[word("acqua", "acqua", "NOUN", "Gender=Fem|Number=Sing", 32, 37)], ner="O"),
                SimpleNamespace(words=[word(".", ".", "PUNCT", None, 37, 38)], ner="O"),
            ],
        ),
        SimpleNamespace(
            text="Maria portera`? No, porterà pane e dammelo!",
            tokens=[
                SimpleNamespace(words=[word("Maria", "Maria", "PROPN", "Gender=Fem|Number=Sing", 39, 44)], ner="S-PER"),
                SimpleNamespace(words=[word("portera`", "portera`", "X", None, 45, 53)], ner="O"),
                SimpleNamespace(words=[word("?", "?", "PUNCT", None, 53, 54)], ner="O"),
                SimpleNamespace(words=[word("No", "no", "ADV", None, 55, 57)], ner="O"),
                SimpleNamespace(words=[word(",", ",", "PUNCT", None, 57, 58)], ner="O"),
                SimpleNamespace(words=[word("porterà", "portare", "VERB", "Mood=Ind|Number=Sing|Person=3|Tense=Fut|VerbForm=Fin", 59, 66)], ner="O"),
                SimpleNamespace(words=[word("pane", "pane", "NOUN", "Gender=Masc|Number=Sing", 67, 71)], ner="O"),
                SimpleNamespace(words=[word("e", "e", "CCONJ", None, 72, 73)], ner="O"),
                SimpleNamespace(words=[word("damme", "dare", "VERB", "Mood=Imp|Number=Sing|Person=2|VerbForm=Fin", 74, 79), word("lo", "lo", "PRON", "Clitic=Yes|Gender=Masc|Number=Sing|Person=3|PronType=Prs", 79, 81)], ner="O"),
                SimpleNamespace(words=[word("!", "!", "PUNCT", None, 81, 82)], ner="O"),
            ],
        ),
    ]
    result = SimpleNamespace(sentences=sentences)
    producer = Producer(enable_ner=True, pipeline_factory=lambda language, enable_ner: lambda value: result)

    artifact = producer.analyze(
        text,
        "it",
        SourceDocument("italian-book", "fixture:italian", "Italian fixture"),
        run_id="italian-regression",
        analyzed_at=datetime(2026, 8, 25, tzinfo=timezone.utc),
    )

    expected = json.loads((Path(__file__).parent / "testdata" / "italian_stanza_expected.json").read_text())
    assert MessageToDict(artifact, preserving_proto_field_name=True) == expected
