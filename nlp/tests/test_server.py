import grpc
from mouseion.v1 import normalized_corpus_pb2, normalized_corpus_pb2_grpc
from mouseion_nlp.server import create_server


class StubProducer:
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
