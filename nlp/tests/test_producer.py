from mouseion_nlp import Producer
from mouseion.v1 import normalized_corpus_pb2


def test_producer_returns_versioned_artifact() -> None:
    artifact = Producer().analyze("Ein Haus.", "de")

    assert isinstance(artifact, normalized_corpus_pb2.NormalizedCorpus)
    assert artifact.schema_version == "1.0.0"
    assert artifact.language == "de"
    assert artifact.sentences[0].text == "Ein Haus."


def test_normalized_corpus_round_trip() -> None:
    artifact = normalized_corpus_pb2.NormalizedCorpus(
        schema_version="1.0.0",
        language="de",
        source_documents=[
            normalized_corpus_pb2.SourceDocument(
                id="book-1", source_identifier="opds:42", title="Das Buch"
            )
        ],
        analysis=normalized_corpus_pb2.AnalysisProvenance(
            run_id="run-7",
            analyzed_at="2026-08-21T12:00:00Z",
            analyzer_name="stanza",
            analyzer_version="1.10.1",
        ),
        normalization_profile=normalized_corpus_pb2.NormalizationProfile(
            name="de-standard-1996", version="1.0.0"
        ),
        sentences=[
            normalized_corpus_pb2.Sentence(
                text="Das Haus steht.",
                location=normalized_corpus_pb2.SourceLocation(
                    source_document_id="book-1", chapter="1", section="1.1", end_offset=15
                ),
                tokens=[
                    normalized_corpus_pb2.Token(
                        surface="Haus",
                        raw_lemma="Haus",
                        canonical_lemma="haus",
                        pos="NOUN",
                        morphology={"Case": "Nom", "Number": "Sing"},
                        named_entity="BUILDING",
                        location=normalized_corpus_pb2.SourceLocation(
                            source_document_id="book-1",
                            chapter="1",
                            section="1.1",
                            start_offset=4,
                            end_offset=8,
                        ),
                    )
                ],
            )
        ],
    )

    encoded = artifact.SerializeToString(deterministic=True)
    decoded = normalized_corpus_pb2.NormalizedCorpus.FromString(encoded)

    assert decoded == artifact
    assert decoded.sentences[0].tokens[0].morphology["Case"] == "Nom"
    assert decoded.sentences[0].tokens[0].HasField("named_entity")
