from datetime import datetime, timezone
from types import SimpleNamespace

from mouseion.v1 import normalized_corpus_pb2
from mouseion_nlp import Producer, SourceDocument


def word(text: str, lemma: str, upos: str, feats: str | None, start: int, end: int):
    return SimpleNamespace(
        text=text, lemma=lemma, upos=upos, feats=feats, start_char=start, end_char=end
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
    assert artifact.schema_version == "1.0.0"
    assert artifact.source_documents[0].id == "book-1"
    assert artifact.analysis.analyzer_name == "stanza"
    assert artifact.analysis.analyzed_at == "2026-08-21T12:00:00Z"
    assert [token.surface for token in artifact.sentences[0].tokens] == ["Goethe", "schrieb", "."]
    assert artifact.sentences[0].tokens[1].raw_lemma == "schreiben"
    assert artifact.sentences[0].tokens[1].pos == "VERB"
    assert artifact.sentences[0].tokens[1].morphology == {"Tense": "Past"}
    assert artifact.sentences[0].tokens[0].named_entity == "B-PER"
    assert not artifact.sentences[0].tokens[1].HasField("named_entity")
    assert artifact.sentences[0].tokens[1].location.start_offset == 7
    assert artifact.sentences[0].location.end_offset == 15
    assert artifact.sentences[0].location.chapter == "1"


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


def test_normalized_corpus_round_trip() -> None:
    artifact = normalized_corpus_pb2.NormalizedCorpus(
        schema_version="1.0.0",
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
