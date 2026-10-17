import json
import logging
from datetime import UTC, datetime
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import stanza
from google.protobuf.json_format import MessageToDict
from mouseion.v1 import normalized_corpus_pb2
from stanza.pipeline.core import DownloadMethod

from mouseion_nlp import Producer, SourceDocument
from mouseion_nlp.model_config import LanguageModelConfig, model_config_for_language
from mouseion_nlp.producer import merge_french_dialogue_sentences


def word(  # noqa: PLR0913 - mirrors the Stanza Word fields the fake token exposes
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
                tokens=[
                    SimpleNamespace(words=[word("Goethe", "Goethe", "PROPN", "Case=Nom", 0, 6)]),
                    SimpleNamespace(
                        words=[word("schrieb", "schreiben", "VERB", "Tense=Past", 7, 14)]
                    ),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 14, 15)]),
                ],
            )
        ]
    )

    def factory(language: str):
        assert language == "de"

        def pipeline(text: str):
            calls.append(text)
            return stanza_result

        return pipeline

    artifact = Producer(pipeline_factory=factory).analyze(
        "Goethe schrieb.",
        "de",
        SourceDocument("book-1", "opds:42", "Das Buch", "1", "opening"),
        run_id="run-7",
        analyzed_at=datetime(2026, 8, 21, 12, tzinfo=UTC),
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
                        words=[word("L'", "il", "DET", None, 0, 2, id=1, head=2, deprel="det")]
                    ),
                    SimpleNamespace(
                        words=[
                            word("uomo", "uomo", "NOUN", None, 2, 6, id=2, head=3, deprel="nsubj")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "mangia",
                                "mangiare",
                                "VERB",
                                None,
                                7,
                                13,
                                id=3,
                                head=0,
                                deprel="root",
                            )
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
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

    tokens = producer.analyze("L'uomo mangia dell'acqua.", "it").sentences[0].tokens

    assert [token.dependency for token in tokens] == [
        "det",
        "nsubj",
        "root",
        "case",
        "det",
        "obj",
        "punct",
    ]
    assert [token.head for token in tokens] == [1, 2, 2, 5, 5, 2, 2]


def test_cleans_edge_punctuation_without_changing_offsets_or_internal_punctuation() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="‹die Besten› O'Neill-like",
                tokens=[
                    SimpleNamespace(words=[word("‹die", "die", "DET", None, 0, 4)]),
                    SimpleNamespace(words=[word("Besten›", "gut", "ADJ", None, 5, 12)]),
                    SimpleNamespace(
                        words=[word("O'Neill-like", "O'Neill-like", "PROPN", None, 13, 25)]
                    ),
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

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
                    SimpleNamespace(
                        words=[word(surface, surface, "NOUN", None, i, i + len(surface))]
                    )
                    for i, surface in enumerate(surfaces)
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

    tokens = producer.analyze(" ".join(surfaces), "de").sentences[0].tokens

    assert [token.surface for token in tokens] == [
        "Wort",
        "Haus",
        "Baum",
        "O'Neill-like",
        "L'",
        ".",
    ]
    assert [token.canonical_lemma for token in tokens] == [
        "wort",
        "haus",
        "baum",
        "o'neill-like",
        "l'",
        ".",
    ]


def test_cleans_reported_lemma_boundaries_and_preserves_internal_punctuation() -> None:
    first_text = (
        "Die Versammlung der Phaiaken verfügt damit über eine gewissermaßen ‹passive Souveränität›."
    )
    second_text = "Beide Voraussetzungen wären beispielsweise in Al Mina gegeben, wo in dem entscheidenden Zeitraum griechische Händler und/oder Söldner angesiedelt waren."
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text=first_text,
                tokens=[
                    SimpleNamespace(
                        words=[word("Souveränität›", "Souveränität›", "NOUN", None, 76, 89)]
                    )
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
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

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
    assert (second_tokens[0].location.start_offset, second_tokens[0].location.end_offset) == (
        117,
        125,
    )


def test_pipe_separated_lemma_uses_first_alternative_and_preserves_raw_lemma(caplog) -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="Nausikaa geleitet ihn.",
                tokens=[
                    SimpleNamespace(
                        words=[word("geleitet", "geleiten|leiten", "VERB", None, 9, 17)]
                    )
                ],
            )
        ]
    )
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

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
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

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
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

    artifact = producer.analyze("Straße, Maße, Masse, daß, Haß, Eßzimmer", "de-DE")

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
                    SimpleNamespace(
                        words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(words=[word("ist", "sein", "AUX", None, 3, 6, id=2, head=0)]),
                    SimpleNamespace(
                        words=[
                            word(
                                "aufgestanden",
                                "aufstehen",
                                "VERB",
                                None,
                                7,
                                19,
                                id=3,
                                head=2,
                                deprel="xcomp",
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 19, 20, id=4, head=2, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Ich stehe auf.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Ich", "ich", "PRON", None, 0, 3, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(
                        words=[word("stehe", "stehen", "VERB", None, 4, 9, id=2, head=0)]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "auf",
                                "auf",
                                "ADV",
                                None,
                                10,
                                13,
                                id=3,
                                head=2,
                                deprel="compound:prt",
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 13, 14, id=4, head=2, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Er stellt das wieder her.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(
                        words=[word("stellt", "stellen", "VERB", None, 3, 9, id=2, head=0)]
                    ),
                    SimpleNamespace(
                        words=[word("das", "das", "DET", None, 10, 13, id=3, head=2, deprel="det")]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "wieder",
                                "wieder",
                                "ADV",
                                None,
                                14,
                                20,
                                id=4,
                                head=2,
                                deprel="compound:prt",
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "her",
                                "her",
                                "ADV",
                                None,
                                21,
                                24,
                                id=5,
                                head=2,
                                deprel="compound:prt",
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 24, 25, id=6, head=2, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Die Tür ist auf.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Die", "die", "DET", None, 0, 3, id=1, head=2, deprel="det")]
                    ),
                    SimpleNamespace(
                        words=[word("Tür", "Tür", "NOUN", None, 4, 7, id=2, head=3, deprel="nsubj")]
                    ),
                    SimpleNamespace(words=[word("ist", "sein", "AUX", None, 8, 11, id=3, head=0)]),
                    SimpleNamespace(
                        words=[
                            word("auf", "auf", "ADV", None, 12, 15, id=4, head=3, deprel="xcomp")
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 15, 16, id=5, head=3, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Er steht auf dem Berg.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(
                        words=[word("steht", "stehen", "VERB", None, 3, 8, id=2, head=0)]
                    ),
                    SimpleNamespace(
                        words=[word("auf", "auf", "ADP", None, 9, 12, id=3, head=5, deprel="case")]
                    ),
                    SimpleNamespace(
                        words=[word("dem", "der", "DET", None, 13, 16, id=4, head=5, deprel="det")]
                    ),
                    SimpleNamespace(
                        words=[
                            word("Berg", "Berg", "NOUN", None, 17, 21, id=5, head=2, deprel="obl")
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 21, 22, id=6, head=2, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Es bleibt nach wie vor.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Es", "es", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(
                        words=[word("bleibt", "bleiben", "VERB", None, 3, 9, id=2, head=0)]
                    ),
                    SimpleNamespace(
                        words=[
                            word("nach", "nach", "ADV", None, 10, 14, id=3, head=2, deprel="advmod")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("wie", "wie", "ADV", None, 15, 18, id=4, head=2, deprel="advmod")
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word("vor", "vor", "ADV", None, 19, 22, id=5, head=2, deprel="advmod")
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 22, 23, id=6, head=2, deprel="punct")]
                    ),
                ],
            ),
            SimpleNamespace(
                text="Er geht zack.",
                tokens=[
                    SimpleNamespace(
                        words=[word("Er", "er", "PRON", None, 0, 2, id=1, head=2, deprel="nsubj")]
                    ),
                    SimpleNamespace(
                        words=[word("geht", "gehen", "VERB", None, 3, 7, id=2, head=0)]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "zack",
                                "zack",
                                "ADV",
                                None,
                                8,
                                12,
                                id=3,
                                head=2,
                                deprel="compound:prt",
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 12, 13, id=4, head=2, deprel="punct")]
                    ),
                ],
            ),
        ]
    )
    producer = Producer(pipeline_factory=lambda language: lambda text: result)

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


def test_modern_greek_canonicalization_matches_identity_contract() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="ΟΔΟΣ οδός οδοσ πού που.",
                tokens=[
                    SimpleNamespace(words=[word("ΟΔΟΣ", "ΟΔΟΣ", "NOUN", None, 0, 4)]),
                    SimpleNamespace(words=[word("οδός", "οδός", "NOUN", None, 5, 9)]),
                    SimpleNamespace(words=[word("οδοσ", "οδοσ", "NOUN", None, 10, 14)]),
                    SimpleNamespace(words=[word("πού", "που\u0301", "ADV", None, 15, 18)]),
                    SimpleNamespace(words=[word("που", "που", "SCONJ", None, 19, 22)]),
                    SimpleNamespace(words=[word(".", ".", "PUNCT", None, 22, 23)]),
                ],
            )
        ]
    )

    artifact = Producer(pipeline_factory=lambda _language: lambda _text: result).analyze(
        result.sentences[0].text, "el"
    )

    tokens = artifact.sentences[0].tokens
    assert [token.raw_lemma for token in tokens] == [
        "ΟΔΟΣ",
        "οδός",
        "οδοσ",
        "που\u0301",
        "που",
        ".",
    ]
    assert [token.canonical_lemma for token in tokens] == [
        "οδοσ",
        "οδόσ",
        "οδοσ",
        "πού",
        "που",
        ".",
    ]
    assert artifact.normalization_profile.name == "modern-greek"
    assert artifact.normalization_profile.version == "1"


def test_modern_greek_maps_mwt_words_and_dependency_data() -> None:
    result = SimpleNamespace(
        sentences=[
            SimpleNamespace(
                text="στο σπίτι.",
                tokens=[
                    SimpleNamespace(
                        words=[
                            word("σε", "σε", "ADP", None, 0, 2, id=1, head=3, deprel="case"),
                            word(
                                "το",
                                "ο",
                                "DET",
                                "Case=Acc|Gender=Neut|Number=Sing",
                                0,
                                2,
                                id=2,
                                head=3,
                                deprel="det",
                            ),
                        ]
                    ),
                    SimpleNamespace(
                        words=[
                            word(
                                "σπίτι",
                                "σπίτι",
                                "NOUN",
                                "Case=Acc|Gender=Neut|Number=Sing",
                                3,
                                8,
                                id=3,
                                head=0,
                            )
                        ]
                    ),
                    SimpleNamespace(
                        words=[word(".", ".", "PUNCT", None, 8, 9, id=4, head=3, deprel="punct")]
                    ),
                ],
            )
        ]
    )

    artifact = Producer(pipeline_factory=lambda _language: lambda _text: result).analyze(
        "στο σπίτι.", "el"
    )
    tokens = artifact.sentences[0].tokens

    assert [token.surface for token in tokens] == ["σε", "το", "σπίτι", "."]
    assert [token.pos for token in tokens] == ["ADP", "DET", "NOUN", "PUNCT"]
    assert tokens[1].morphology == {"Case": "Acc", "Gender": "Neut", "Number": "Sing"}
    assert [token.dependency for token in tokens] == ["case", "det", "root", "punct"]
    assert [token.head for token in tokens] == [2, 2, 2, 2]


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

    def factory(language: str):
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

    def factory(language: str):
        assert language == "it"
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


def test_pipeline_and_capability_model_version_use_language_configuration() -> None:
    config = LanguageModelConfig(
        language="el",
        processors="tokenize,mwt,pos,lemma,depparse",
        package={
            "tokenize": "gdt",
            "mwt": "gdt",
            "pos": "gdt_nocharlm",
            "lemma": "gdt_nocharlm",
            "depparse": "gdt_greek-bert",
        },
        pretrain_dependencies={"lemma": "conll17"},
        model_version="stanza-1.15.0-gdt-accurate",
    )

    with (
        patch("mouseion_nlp.producer.model_config_for_language", return_value=config),
        patch("mouseion_nlp.producer.stanza.Pipeline") as pipeline,
    ):
        from mouseion_nlp import producer as producer_module

        producer_module._stanza_pipeline.cache_clear()
        try:
            Producer().warmup("el")
            assert Producer().model_version("el") == config.model_version
        finally:
            producer_module._stanza_pipeline.cache_clear()

    pipeline.assert_called_once_with(
        lang="el",
        model_dir=stanza.resources.common.DEFAULT_MODEL_DIR,
        processors=config.processors,
        package=config.package,
        lemma_pretrain_path=str(
            Path(stanza.resources.common.DEFAULT_MODEL_DIR) / "el" / "pretrain" / "conll17.pt"
        ),
        download_method=DownloadMethod.NONE,
        verbose=False,
    )


def test_default_languages_build_the_pipeline_without_pretrain_paths() -> None:
    with patch("mouseion_nlp.producer.stanza.Pipeline") as pipeline:
        from mouseion_nlp import producer as producer_module

        producer_module._stanza_pipeline.cache_clear()
        try:
            Producer().warmup("de")
            Producer().warmup("it")
        finally:
            producer_module._stanza_pipeline.cache_clear()

    (german, italian) = pipeline.call_args_list
    assert german.kwargs == {
        "lang": "de",
        "model_dir": stanza.resources.common.DEFAULT_MODEL_DIR,
        "processors": "tokenize,pos,lemma,depparse",
        "download_method": DownloadMethod.NONE,
        "verbose": False,
    }
    assert italian.kwargs == {
        "lang": "it",
        "model_dir": stanza.resources.common.DEFAULT_MODEL_DIR,
        "processors": "tokenize,pos,lemma,depparse",
        "download_method": DownloadMethod.NONE,
        "verbose": False,
    }


def test_default_languages_keep_stanza_library_model_version() -> None:
    producer = Producer(pipeline_factory=lambda _language: lambda _text: None)

    assert producer.model_version("de") == stanza.__version__
    assert producer.model_version("it") == stanza.__version__


def test_greek_model_version_identifies_the_accurate_package() -> None:
    producer = Producer(pipeline_factory=lambda _language: lambda _text: None)

    assert producer.model_version("el") == "stanza-1.15.0-gdt-accurate"


def test_italian_linguistic_regression_fixture() -> None:
    """Lock the normalized contract to representative Italian Stanza output."""
    text = "L'uomo e le ragazze bevono dell'acqua. Maria portera`? No, porterà pane e dammelo!"
    sentences = [
        SimpleNamespace(
            text="L'uomo e le ragazze bevono dell'acqua.",
            tokens=[
                SimpleNamespace(
                    words=[
                        word(
                            "L'",
                            "il",
                            "DET",
                            "Definite=Def|Gender=Masc|Number=Sing|PronType=Art",
                            0,
                            2,
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word("uomo", "uomo", "NOUN", "Gender=Masc|Number=Sing", 2, 6)]
                ),
                SimpleNamespace(words=[word("e", "e", "CCONJ", None, 7, 8)]),
                SimpleNamespace(
                    words=[
                        word(
                            "le",
                            "il",
                            "DET",
                            "Definite=Def|Gender=Fem|Number=Plur|PronType=Art",
                            9,
                            11,
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word("ragazze", "ragazza", "NOUN", "Gender=Fem|Number=Plur", 12, 19)]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "bevono",
                            "bere",
                            "VERB",
                            "Mood=Ind|Number=Plur|Person=3|Tense=Pres|VerbForm=Fin",
                            20,
                            26,
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("dell'", "di", "ADP", None, 27, 32),
                        word(
                            "dell'",
                            "il",
                            "DET",
                            "Definite=Def|Gender=Fem|Number=Sing|PronType=Art",
                            27,
                            32,
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[word("acqua", "acqua", "NOUN", "Gender=Fem|Number=Sing", 32, 37)]
                ),
                SimpleNamespace(words=[word(".", ".", "PUNCT", None, 37, 38)]),
            ],
        ),
        SimpleNamespace(
            text="Maria portera`? No, porterà pane e dammelo!",
            tokens=[
                SimpleNamespace(
                    words=[word("Maria", "Maria", "PROPN", "Gender=Fem|Number=Sing", 39, 44)]
                ),
                SimpleNamespace(words=[word("portera`", "portera`", "X", None, 45, 53)]),
                SimpleNamespace(words=[word("?", "?", "PUNCT", None, 53, 54)]),
                SimpleNamespace(words=[word("No", "no", "ADV", None, 55, 57)]),
                SimpleNamespace(words=[word(",", ",", "PUNCT", None, 57, 58)]),
                SimpleNamespace(
                    words=[
                        word(
                            "porterà",
                            "portare",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Fut|VerbForm=Fin",
                            59,
                            66,
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word("pane", "pane", "NOUN", "Gender=Masc|Number=Sing", 67, 71)]
                ),
                SimpleNamespace(words=[word("e", "e", "CCONJ", None, 72, 73)]),
                SimpleNamespace(
                    words=[
                        word(
                            "damme",
                            "dare",
                            "VERB",
                            "Mood=Imp|Number=Sing|Person=2|VerbForm=Fin",
                            74,
                            79,
                        ),
                        word(
                            "lo",
                            "lo",
                            "PRON",
                            "Clitic=Yes|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            79,
                            81,
                        ),
                    ]
                ),
                SimpleNamespace(words=[word("!", "!", "PUNCT", None, 81, 82)]),
            ],
        ),
    ]
    result = SimpleNamespace(sentences=sentences)
    producer = Producer(pipeline_factory=lambda language: lambda value: result)

    artifact = producer.analyze(
        text,
        "it",
        SourceDocument("italian-book", "fixture:italian", "Italian fixture"),
        run_id="italian-regression",
        analyzed_at=datetime(2026, 8, 25, tzinfo=UTC),
    )

    expected = json.loads(
        (Path(__file__).parent / "testdata" / "italian_stanza_expected.json").read_text()
    )
    assert MessageToDict(artifact, preserving_proto_field_name=True) == expected


def test_greek_linguistic_regression_fixture() -> None:
    """Lock the normalized contract to representative Greek Stanza output."""
    first = "ΣΤΟ σπίτι ο Νίκος διαβάζει ιστορίες."
    second = "Οι φίλοι επιστρέφουν στην πλατεία, ενώ ο ΚΟΣΜΟΣ γελά."
    text = first + " " + second
    sentences = [
        SimpleNamespace(
            text=first,
            tokens=[
                SimpleNamespace(
                    words=[
                        word("σε", "σε", "ADP", None, 0, 3, id=1, head=3, deprel="case"),
                        word(
                            "το",
                            "ο",
                            "DET",
                            "Case=Acc|Gender=Neut|Number=Sing|PronType=Art",
                            0,
                            3,
                            id=2,
                            head=3,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "σπίτι",
                            "σπίτι",
                            "NOUN",
                            "Case=Acc|Gender=Neut|Number=Sing",
                            4,
                            9,
                            id=3,
                            head=6,
                            deprel="nsubj",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "ο",
                            "ο",
                            "DET",
                            "Case=Nom|Gender=Masc|Number=Sing|PronType=Art",
                            10,
                            11,
                            id=4,
                            head=5,
                            deprel="det",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "Νίκος",
                            "Νίκος",
                            "PROPN",
                            "Case=Nom|Gender=Masc|Number=Sing",
                            12,
                            17,
                            id=5,
                            head=6,
                            deprel="nsubj",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "διαβάζει",
                            "διαβάζω",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin",
                            18,
                            26,
                            id=6,
                            head=0,
                            deprel="root",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "ιστορίες",
                            "ιστορία",
                            "NOUN",
                            "Case=Acc|Gender=Fem|Number=Plur",
                            27,
                            35,
                            id=7,
                            head=6,
                            deprel="obj",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word(".", ".", "PUNCT", None, 35, 36, id=8, head=6, deprel="punct")]
                ),
            ],
        ),
        SimpleNamespace(
            text=second,
            tokens=[
                SimpleNamespace(
                    words=[
                        word(
                            "Οι",
                            "ο",
                            "DET",
                            "Case=Nom|Gender=Masc|Number=Plur|PronType=Art",
                            37,
                            39,
                            id=1,
                            head=2,
                            deprel="det",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "φίλοι",
                            "φίλος",
                            "NOUN",
                            "Case=Nom|Gender=Masc|Number=Plur",
                            40,
                            45,
                            id=2,
                            head=3,
                            deprel="nsubj",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "επιστρέφουν",
                            "επιστρέφω",
                            "VERB",
                            "Mood=Ind|Number=Plur|Person=3|Tense=Pres|VerbForm=Fin",
                            46,
                            57,
                            id=3,
                            head=0,
                            deprel="root",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("σε", "σε", "ADP", None, 58, 62, id=4, head=6, deprel="case"),
                        word(
                            "την",
                            "ο",
                            "DET",
                            "Case=Acc|Gender=Fem|Number=Sing|PronType=Art",
                            58,
                            62,
                            id=5,
                            head=6,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "πλατεία",
                            "πλατεία",
                            "NOUN",
                            "Case=Acc|Gender=Fem|Number=Sing",
                            63,
                            70,
                            id=6,
                            head=3,
                            deprel="obl",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word(",", ",", "PUNCT", None, 70, 71, id=7, head=3, deprel="punct")]
                ),
                SimpleNamespace(
                    words=[word("ενώ", "ενώ", "SCONJ", None, 72, 75, id=8, head=3, deprel="mark")]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "ο",
                            "ο",
                            "DET",
                            "Case=Nom|Gender=Masc|Number=Sing|PronType=Art",
                            76,
                            77,
                            id=9,
                            head=10,
                            deprel="det",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "ΚΟΣΜΟΣ",
                            "κόσμος",
                            "NOUN",
                            "Case=Nom|Gender=Masc|Number=Sing",
                            78,
                            84,
                            id=10,
                            head=11,
                            deprel="nsubj",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "γελά",
                            "γελάω",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin",
                            85,
                            89,
                            id=11,
                            head=0,
                            deprel="root",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word(".", ".", "PUNCT", None, 89, 90, id=12, head=11, deprel="punct")]
                ),
            ],
        ),
    ]
    artifact = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=sentences)
    ).analyze(
        text,
        "el",
        SourceDocument("greek-book", "fixture:greek", "Greek fixture"),
        run_id="greek-regression",
        analyzed_at=datetime(2026, 8, 25, tzinfo=UTC),
    )

    config = model_config_for_language("el")
    assert config.processors == "tokenize,mwt,pos,lemma,depparse"
    assert config.package == {
        "tokenize": "gdt",
        "mwt": "gdt",
        "pos": "gdt_nocharlm",
        "lemma": "gdt_nocharlm",
        "depparse": "gdt_greek-bert",
    }
    assert config.model_version == "stanza-1.15.0-gdt-accurate"
    expected = json.loads(
        (Path(__file__).parent / "testdata" / "greek_stanza_expected.json").read_text()
    )
    assert MessageToDict(artifact, preserving_proto_field_name=True) == expected


def test_french_model_configuration_uses_the_combined_package() -> None:
    config = model_config_for_language("fr")

    assert config.processors == "tokenize,mwt,pos,lemma,depparse"
    assert config.package == {
        "tokenize": "combined_nocharlm",
        "mwt": "combined",
        "pos": "combined_charlm",
        "lemma": "combined_nocharlm",
        "depparse": "combined_charlm",
    }
    assert config.model_version == "stanza-1.15.0-fr-combined"
    assert config.pretrain_dependencies is None
    assert config.external_model_dependencies == ()


def test_french_model_version_identifies_the_combined_package() -> None:
    producer = Producer(pipeline_factory=lambda _language: lambda _text: None)

    assert producer.model_version("fr") == "stanza-1.15.0-fr-combined"


def test_french_pipeline_builds_offline_with_the_combined_package() -> None:
    config = model_config_for_language("fr")

    with (
        patch("mouseion_nlp.producer.model_config_for_language", return_value=config),
        patch("mouseion_nlp.producer.stanza.Pipeline") as pipeline,
    ):
        from mouseion_nlp import producer as producer_module

        producer_module._stanza_pipeline.cache_clear()
        try:
            Producer().warmup("fr")
            assert Producer().model_version("fr") == config.model_version
        finally:
            producer_module._stanza_pipeline.cache_clear()

    pipeline.assert_called_once_with(
        lang="fr",
        model_dir=stanza.resources.common.DEFAULT_MODEL_DIR,
        processors=config.processors,
        package=config.package,
        tokenize_postprocessor=merge_french_dialogue_sentences,
        download_method=DownloadMethod.NONE,
        verbose=False,
    )


def test_french_multiword_tokens_expand_into_content_words() -> None:
    """The `du` contraction expands to `de` + `le` on the shared token span."""
    sentences = [
        SimpleNamespace(
            text="Du pain.",
            tokens=[
                SimpleNamespace(
                    words=[
                        word("de", "de", "ADP", None, 0, 2, id=1, head=3, deprel="det"),
                        word(
                            "le",
                            "le",
                            "DET",
                            "Definite=Def|Gender=Masc|Number=Sing|PronType=Art",
                            0,
                            2,
                            id=2,
                            head=3,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "pain",
                            "pain",
                            "NOUN",
                            "Gender=Masc|Number=Sing",
                            3,
                            7,
                            id=3,
                            head=0,
                            deprel="root",
                        )
                    ]
                ),
                SimpleNamespace(
                    words=[word(".", ".", "PUNCT", None, 7, 8, id=4, head=3, deprel="punct")]
                ),
            ],
        )
    ]
    producer = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=sentences)
    )

    artifact = producer.analyze("Du pain.", "fr")

    tokens = artifact.sentences[0].tokens
    assert [token.surface for token in tokens] == ["de", "le", "pain", "."]
    assert tokens[0].pos == "ADP"
    assert tokens[1].pos == "DET"
    assert (tokens[0].location.start_offset, tokens[0].location.end_offset) == (0, 2)
    assert (tokens[1].location.start_offset, tokens[1].location.end_offset) == (0, 2)
    assert tokens[0].head == tokens[1].head == 2


def test_french_dialogue_tokenizer_merger_rejoins_a_standalone_closing_guillemet() -> None:
    sentences = [
        ["«", "Je", "suis", "sûr", "qu’", "il", "viendra", "ce", "soir", "."],
        ["»"],
    ]

    assert merge_french_dialogue_sentences(sentences) == [
        ["«", "Je", "suis", "sûr", "qu’", "il", "viendra", "ce", "soir", ".", "»"]
    ]


def test_french_dialogue_tokenizer_merger_splits_a_guillemet_before_new_text() -> None:
    sentences = [
        ["«", "Je", "suis", "sûr", "qu’", "il", "viendra", "ce", "soir", "."],
        ["»", "Il", "faut", "partir", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == [
        ["«", "Je", "suis", "sûr", "qu’", "il", "viendra", "ce", "soir", ".", "»"],
        ["Il", "faut", "partir", "."],
    ]


def test_french_dialogue_tokenizer_merger_rejoins_a_guillemet_incise() -> None:
    sentences = [
        ["«", "Tu", "viens", "?"],
        ["»", "demanda", "-t-il", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == [
        ["«", "Tu", "viens", "?", "»", "demanda", "-t-il", "."]
    ]


def test_french_dialogue_tokenizer_merger_rejoins_an_em_dash_incise() -> None:
    sentences = [
        ["—", "Tu", "viens", "?"],
        ["demanda", "-t-il", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == [
        ["—", "Tu", "viens", "?", "demanda", "-t-il", "."]
    ]


def test_french_dialogue_tokenizer_merger_rejoins_an_incise_without_a_dash() -> None:
    sentences = [
        ["Quoi", "?"],
        ["dit", "-il", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == [["Quoi", "?", "dit", "-il", "."]]


def test_french_dialogue_tokenizer_merger_rejoins_a_leading_em_dash() -> None:
    sentences = [
        ["—"],
        ["Il", "est", "là", ",", "dit", "-elle", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == [
        ["—", "Il", "est", "là", ",", "dit", "-elle", "."]
    ]


def test_french_dialogue_tokenizer_merger_leaves_plain_prose_and_capitalised_text_alone() -> None:
    sentences = [
        ["Il", "partit", "."],
        ["Elle", "resta", "."],
        ["Quoi", "?"],
        ["Il", "partit", "."],
    ]

    assert merge_french_dialogue_sentences(sentences) == sentences


def test_french_dialogue_tokenizer_merger_preserves_multiword_tokens() -> None:
    sentences = [
        ["Il", "dit", "."],
        [("du", True), "pain", "."],
    ]

    merged = merge_french_dialogue_sentences(sentences)

    assert merged == sentences
    assert merged[1][0] == ("du", True)


def test_french_dialogue_tokenizer_merger_keeps_an_empty_sentence() -> None:
    sentences = [["Il", "partit", "."], []]

    assert merge_french_dialogue_sentences(sentences) == [["Il", "partit", "."], []]


def test_french_linguistic_regression_fixture() -> None:
    """Lock the normalized contract to representative French Stanza output.

    The configured French tokenizer postprocessor rejoins each dialogue
    sentence before the parser runs, so the pipeline emits these complete,
    source-exact sentences (ADR 0102); the merger is asserted directly in its
    own tests.
    """
    first = "Je vois l'homme près du marché."
    second = "Il va au marché avec sa sœur."
    guillemet = "« Je suis sûr qu’il viendra ce soir.\u202f»"
    after = "Il faut partir."
    incise = "« Tu viens ? » demanda-t-il."
    em_dash = "— Tu viens ? demanda-t-il."
    text = f"{first} {second} {guillemet}\n\n{after} {incise} {em_dash}"
    sentences = [
        SimpleNamespace(
            text=first,
            tokens=[
                SimpleNamespace(
                    words=[
                        word(
                            "Je",
                            "moi",
                            "PRON",
                            "Case=Nom|Emph=No|Number=Sing|Person=1|PronType=Prs",
                            0,
                            2,
                            id=1,
                            head=2,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "vois",
                            "voir",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin",
                            3,
                            7,
                            id=2,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "l'",
                            "le",
                            "DET",
                            "Definite=Def|Gender=Masc|Number=Sing|PronType=Art",
                            8,
                            10,
                            id=3,
                            head=4,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "homme",
                            "homme",
                            "NOUN",
                            "Gender=Masc|Number=Sing",
                            10,
                            15,
                            id=4,
                            head=2,
                            deprel="obj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("près", "près", "ADV", None, 16, 20, id=5, head=2, deprel="advmod"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("de", "de", "ADP", None, 21, 23, id=6, head=8, deprel="case"),
                        word(
                            "le",
                            "le",
                            "DET",
                            "Definite=Def|Gender=Masc|Number=Sing|PronType=Art",
                            21,
                            23,
                            id=7,
                            head=8,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "marché",
                            "marché",
                            "NOUN",
                            "Gender=Masc|Number=Sing",
                            24,
                            30,
                            id=8,
                            head=5,
                            deprel="obl:arg",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 30, 31, id=9, head=2, deprel="punct"),
                    ]
                ),
            ],
        ),
        SimpleNamespace(
            text=second,
            tokens=[
                SimpleNamespace(
                    words=[
                        word(
                            "Il",
                            "lui",
                            "PRON",
                            "Case=Nom|Emph=No|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            32,
                            34,
                            id=1,
                            head=2,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "va",
                            "aller",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin",
                            35,
                            37,
                            id=2,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("à", "à", "ADP", None, 38, 40, id=3, head=5, deprel="case"),
                        word(
                            "le",
                            "le",
                            "DET",
                            "Definite=Def|Gender=Masc|Number=Sing|PronType=Art",
                            38,
                            40,
                            id=4,
                            head=5,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "marché",
                            "marché",
                            "NOUN",
                            "Gender=Masc|Number=Sing",
                            41,
                            47,
                            id=5,
                            head=2,
                            deprel="obl:arg",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("avec", "avec", "ADP", None, 48, 52, id=6, head=8, deprel="case"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "sa",
                            "son",
                            "DET",
                            "Gender=Fem|Number=Sing|Number[psor]=Sing|Person[psor]=3|Poss=Yes|PronType=Prs",
                            53,
                            55,
                            id=7,
                            head=8,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "sœur",
                            "sœur",
                            "NOUN",
                            "Gender=Fem|Number=Sing",
                            56,
                            60,
                            id=8,
                            head=2,
                            deprel="obl:mod",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 60, 61, id=9, head=2, deprel="punct"),
                    ]
                ),
            ],
        ),
        SimpleNamespace(
            text=guillemet,
            tokens=[
                SimpleNamespace(
                    words=[
                        word("«", "«", "PUNCT", None, 62, 63, id=1, head=4, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "Je",
                            "moi",
                            "PRON",
                            "Case=Nom|Emph=No|Number=Sing|Person=1|PronType=Prs",
                            64,
                            66,
                            id=2,
                            head=4,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "suis",
                            "être",
                            "AUX",
                            "Mood=Ind|Number=Sing|Person=1|Tense=Pres|VerbForm=Fin",
                            67,
                            71,
                            id=3,
                            head=4,
                            deprel="cop",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "sûr",
                            "sûr",
                            "ADJ",
                            "Gender=Masc|Number=Sing",
                            72,
                            75,
                            id=4,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("qu’", "que", "SCONJ", None, 76, 79, id=5, head=7, deprel="mark"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "il",
                            "lui",
                            "PRON",
                            "Case=Nom|Emph=No|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            79,
                            81,
                            id=6,
                            head=7,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "viendra",
                            "venir",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Fut|VerbForm=Fin",
                            82,
                            89,
                            id=7,
                            head=4,
                            deprel="csubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "ce",
                            "ce",
                            "DET",
                            "Gender=Masc|Number=Sing|PronType=Dem",
                            90,
                            92,
                            id=8,
                            head=9,
                            deprel="det",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "soir",
                            "soir",
                            "NOUN",
                            "Gender=Masc|Number=Sing",
                            93,
                            97,
                            id=9,
                            head=7,
                            deprel="obl:mod",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 97, 98, id=10, head=4, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("»", "»", "PUNCT", None, 99, 100, id=11, head=4, deprel="punct"),
                    ]
                ),
            ],
        ),
        SimpleNamespace(
            text=after,
            tokens=[
                SimpleNamespace(
                    words=[
                        word(
                            "Il",
                            "lui",
                            "PRON",
                            "Case=Nom|Emph=No|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            102,
                            104,
                            id=1,
                            head=2,
                            deprel="expl:subj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "faut",
                            "falloir",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Pres|VerbForm=Fin",
                            105,
                            109,
                            id=2,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "partir",
                            "partir",
                            "VERB",
                            "VerbForm=Inf",
                            110,
                            116,
                            id=3,
                            head=2,
                            deprel="ccomp",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 116, 117, id=4, head=2, deprel="punct"),
                    ]
                ),
            ],
        ),
        SimpleNamespace(
            text=incise,
            tokens=[
                SimpleNamespace(
                    words=[
                        word("«", "«", "PUNCT", None, 118, 119, id=1, head=3, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "Tu",
                            "toi",
                            "PRON",
                            "Case=Nom|Emph=No|Number=Sing|Person=2|PronType=Prs",
                            120,
                            122,
                            id=2,
                            head=3,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "viens",
                            "venir",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin",
                            123,
                            128,
                            id=3,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("?", "?", "PUNCT", None, 129, 130, id=4, head=3, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("»", "»", "PUNCT", None, 131, 132, id=5, head=3, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "demanda",
                            "demander",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin",
                            133,
                            140,
                            id=6,
                            head=3,
                            deprel="parataxis:insert",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "-t-il",
                            "lui",
                            "PRON",
                            "Emph=No|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            140,
                            145,
                            id=7,
                            head=6,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 145, 146, id=8, head=3, deprel="punct"),
                    ]
                ),
            ],
        ),
        SimpleNamespace(
            text=em_dash,
            tokens=[
                SimpleNamespace(
                    words=[
                        word("—", "—", "PUNCT", None, 147, 148, id=1, head=3, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "Tu",
                            "toi",
                            "PRON",
                            "Case=Nom|Emph=No|Number=Sing|Person=2|PronType=Prs",
                            149,
                            151,
                            id=2,
                            head=3,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "viens",
                            "venir",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=2|Tense=Pres|VerbForm=Fin",
                            152,
                            157,
                            id=3,
                            head=0,
                            deprel="root",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word("?", "?", "PUNCT", None, 158, 159, id=4, head=3, deprel="punct"),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "demanda",
                            "demander",
                            "VERB",
                            "Mood=Ind|Number=Sing|Person=3|Tense=Past|VerbForm=Fin",
                            160,
                            167,
                            id=5,
                            head=3,
                            deprel="parataxis:insert",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(
                            "-t-il",
                            "lui",
                            "PRON",
                            "Emph=No|Gender=Masc|Number=Sing|Person=3|PronType=Prs",
                            167,
                            172,
                            id=6,
                            head=5,
                            deprel="nsubj",
                        ),
                    ]
                ),
                SimpleNamespace(
                    words=[
                        word(".", ".", "PUNCT", None, 172, 173, id=7, head=3, deprel="punct"),
                    ]
                ),
            ],
        ),
    ]
    artifact = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=sentences)
    ).analyze(
        text,
        "fr",
        SourceDocument("french-book", "fixture:french", "French fixture"),
        run_id="french-regression",
        analyzed_at=datetime(2026, 8, 25, tzinfo=UTC),
    )

    expected = json.loads(
        (Path(__file__).parent / "testdata" / "french_stanza_expected.json").read_text()
    )
    assert MessageToDict(artifact, preserving_proto_field_name=True) == expected


def _french_parity_rows() -> list[dict]:
    """The shared fixture the Go French profile reads in french_parity_test.go."""
    text = (
        Path(__file__).parent.parent / "testdata" / "french_normalization_parity.jsonl"
    ).read_text(encoding="utf-8")
    return [json.loads(line) for line in text.splitlines() if line.strip()]


def test_french_canonical_lemma_matches_the_shared_parity_fixture() -> None:
    """Accents stay significant; only the canonical lemma folds U+2019."""
    rows = _french_parity_rows()
    assert rows, "the parity fixture must not be empty"
    for row in rows:
        assert Producer._canonical_lemma("fr", row["raw"]) == row["canonical"], row
    canonical = {row["raw"]: row["canonical"] for row in rows}
    assert canonical["où"] != canonical["ou"], "où and ou stay distinct"
    assert canonical["sûr"] != canonical["sur"], "sûr and sur stay distinct"
    assert canonical["aujourd\u2019hui"] == canonical["aujourd'hui"], (
        "both apostrophes share one identity"
    )


def test_french_canonical_lemma_folds_apostrophe_but_keeps_the_raw_lemma() -> None:
    text = "aujourd\u2019hui"
    sentences = [
        SimpleNamespace(
            text=text,
            tokens=[
                SimpleNamespace(words=[word(text, text, "ADV", None, 0, len(text))]),
            ],
        )
    ]
    artifact = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=sentences)
    ).analyze(text, "fr")

    token = artifact.sentences[0].tokens[0]
    assert token.raw_lemma == "aujourd\u2019hui", "the raw lemma keeps the typographic apostrophe"
    assert token.canonical_lemma == "aujourd'hui", "only the canonical lemma folds U+2019"
    assert artifact.sentences[0].text == text, "the source text is never rewritten"


def test_french_normalization_profile_is_recorded() -> None:
    artifact = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=[])
    ).analyze("où", "fr")
    assert artifact.normalization_profile.name == "french"
    assert artifact.normalization_profile.version == "1"


def test_french_elision_contraction_and_clitic_expansion() -> None:
    """MWT expansion keeps each component and its dependency head."""
    text = "L'homme s'habille, d'accord, va-t-il."
    words = [
        ("L'", "le", "DET", 0, 2, 1, 2, "det"),
        ("homme", "homme", "NOUN", 2, 7, 2, 0, "root"),
        ("s'", "se", "PRON", 8, 10, 3, 4, "expl:pv"),
        ("habille", "habiller", "VERB", 10, 17, 4, 2, "acl"),
        ("d'", "de", "ADP", 19, 21, 5, 6, "case"),
        ("accord", "accord", "NOUN", 21, 27, 6, 2, "conj"),
        ("va", "aller", "VERB", 29, 31, 7, 0, "root"),
        # Edge punctuation is stripped from the surface, so -t- becomes t; the
        # raw lemma and its span are preserved.
        ("t", "il", "PRON", 31, 34, 8, 7, "expl"),
        ("il", "lui", "PRON", 34, 36, 9, 7, "nsubj"),
        (".", ".", "PUNCT", 36, 37, 10, 7, "punct"),
    ]
    sentences = [
        SimpleNamespace(
            text=text,
            tokens=[
                SimpleNamespace(
                    words=[
                        word(w[0], w[1], w[2], None, w[3], w[4], id=w[5], head=w[6], deprel=w[7])
                    ]
                )
                for w in words
            ],
        )
    ]
    producer = Producer(
        pipeline_factory=lambda _language: lambda _value: SimpleNamespace(sentences=sentences)
    )

    artifact = producer.analyze(text, "fr")

    tokens = artifact.sentences[0].tokens
    assert [token.surface for token in tokens] == [w[0] for w in words]
    by_surface = {token.surface: token for token in tokens}
    # Elision: the elided article and pronoun expand on the content word.
    assert by_surface["L'"].pos == "DET"
    assert by_surface["L'"].raw_lemma == "le"
    assert (by_surface["L'"].location.start_offset, by_surface["L'"].location.end_offset) == (0, 2)
    assert by_surface["s'"].raw_lemma == "se"
    assert by_surface["habille"].canonical_lemma == "habiller"
    # Contraction: d' + accord keep the preposition and the noun.
    assert by_surface["d'"].pos == "ADP"
    assert by_surface["accord"].canonical_lemma == "accord"
    # Inversion clitic: the verb keeps aller and the clitic stays a PRON.
    assert by_surface["va"].canonical_lemma == "aller"
    assert by_surface["t"].pos == "PRON"
    assert by_surface["il"].raw_lemma == "lui"
    # Morphology and dependency heads survive the mapping.
    assert tokens[1].head == 1, "homme is the root"
    assert tokens[4].dependency == "case"
    assert tokens[5].head == 1, "accord attaches to homme's ordinal"
    assert artifact.normalization_profile.name == "french"
