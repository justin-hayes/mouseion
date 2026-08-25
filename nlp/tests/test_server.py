import os
from unittest.mock import patch

import grpc
from mouseion.v1 import normalized_corpus_pb2, normalized_corpus_pb2_grpc
from mouseion_nlp.server import (
    AnalyzerServicer,
    configured_languages,
    create_server,
    warm_configured_languages,
)


class StubProducer:
    enable_ner = False

    def warmup(self, language):
        assert language in {"de", "it"}

    def analyze(self, text, language, document):
        assert (text, language) == ("Goethe", "de")
        assert (document.id, document.source_identifier, document.title) == (
            "document-1",
            "opds:1",
            "Faust",
        )
        return normalized_corpus_pb2.NormalizedCorpus(
            schema_version="1.0.0",
            language=language,
            source_documents=[
                normalized_corpus_pb2.SourceDocument(id=document.id, title=document.title)
            ],
        )


def test_grpc_server_serves_a_normalized_corpus() -> None:
    server = create_server(StubProducer())
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = normalized_corpus_pb2_grpc.AnalyzerServiceStub(channel)
            corpus = stub.Analyze(
                normalized_corpus_pb2.AnalyzeRequest(
                    language="de",
                    document_text="Goethe",
                    source_document=normalized_corpus_pb2.SourceDocument(
                        id="document-1", source_identifier="opds:1", title="Faust"
                    ),
                )
            )
        assert corpus.schema_version == "1.0.0"
        assert corpus.source_documents[0].id == "document-1"
    finally:
        server.stop(None).wait()


def test_grpc_server_reports_language_capabilities() -> None:
    with patch.dict(os.environ, {"MOUSEION_NLP_WARM_LANGUAGES": "it, de, it"}, clear=False):
        server = create_server(StubProducer())
    server._servicer.warmup("de")
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        with grpc.insecure_channel(f"127.0.0.1:{port}") as channel:
            stub = normalized_corpus_pb2_grpc.AnalyzerServiceStub(channel)
            response = stub.GetCapabilities(normalized_corpus_pb2.GetCapabilitiesRequest())
        assert [capability.language for capability in response.languages] == ["de", "it"]
        german, italian = response.languages
        assert (german.display_name, german.ready) == ("German", True)
        assert german.model_version
        assert german.supported_features == ["tokenize", "pos", "lemma"]
        assert (italian.display_name, italian.model_version, italian.ready) == (
            "Italian",
            "",
            False,
        )
    finally:
        server.stop(None).wait()


def test_configured_languages_supports_legacy_setting() -> None:
    with patch.dict(os.environ, {"MOUSEION_NLP_WARM_LANGUAGE": "it"}, clear=True):
        assert [descriptor.language for descriptor in configured_languages()] == ["it"]


def test_warmup_failure_does_not_mark_another_language_ready(capsys) -> None:
    class PartiallyFailingProducer(StubProducer):
        def warmup(self, language):
            if language == "de":
                raise RuntimeError("model unavailable")

    with patch.dict(os.environ, {"MOUSEION_NLP_WARM_LANGUAGES": "de,it"}, clear=True):
        servicer = AnalyzerServicer(PartiallyFailingProducer())
    warm_configured_languages(servicer)

    capabilities = servicer.GetCapabilities(None, None).languages
    assert [(item.language, item.ready) for item in capabilities] == [("de", False), ("it", True)]
    assert capabilities[0].model_version == ""
    assert capabilities[1].model_version
    assert "failed to warm Stanza pipeline for language 'de': model unavailable" in capsys.readouterr().err
