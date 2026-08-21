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
