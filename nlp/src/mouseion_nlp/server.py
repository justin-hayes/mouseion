"""Long-lived gRPC server for the Stanza analyzer."""

from __future__ import annotations

from concurrent import futures
import os
from typing import NoReturn

import grpc
import stanza
from mouseion.v1 import normalized_corpus_pb2, normalized_corpus_pb2_grpc

from .producer import Producer, SourceDocument


class AnalyzerServicer(normalized_corpus_pb2_grpc.AnalyzerServiceServicer):
    """Adapt gRPC analyzer requests to one warm Producer instance."""

    def __init__(self, producer: Producer | None = None) -> None:
        self._producer = producer or Producer()
        self._ready_languages: set[str] = set()

    def warmup(self, language: str) -> None:
        """Load (and, on first run, download) the Stanza pipeline for a language.

        Called at server startup so the slow model download/load happens before
        the server accepts analysis requests, instead of on the first Analyze
        call (which could otherwise exceed the caller's RPC deadline).
        """
        self._producer.warmup(language)
        self._ready_languages.add(language)

    def GetCapabilities(self, request, context):  # noqa: ARG002, N802
        language = os.getenv("MOUSEION_NLP_WARM_LANGUAGE", "de")
        display_names = {"de": "German"}
        features = ["tokenize", "pos", "lemma"]
        if self._producer.enable_ner:
            features.append("ner")
        return normalized_corpus_pb2.GetCapabilitiesResponse(
            languages=[
                normalized_corpus_pb2.LanguageCapability(
                    language=language,
                    display_name=display_names.get(language, language),
                    model_version=stanza.__version__,
                    supported_features=features,
                    ready=language in self._ready_languages,
                )
            ]
        )

    def Analyze(self, request, context):  # noqa: N802
        source = request.source_document
        try:
            return self._producer.analyze(
                request.document_text,
                request.language,
                SourceDocument(
                    id=source.id,
                    source_identifier=source.source_identifier,
                    title=source.title,
                ),
            )
        except Exception as error:
            context.abort(grpc.StatusCode.INTERNAL, f"analysis failed: {error}")


def create_server(producer: Producer | None = None) -> grpc.Server:
    """Create a server with an injectable producer for tests."""
    servicer = AnalyzerServicer(producer)
    server = grpc.server(futures.ThreadPoolExecutor())
    normalized_corpus_pb2_grpc.add_AnalyzerServiceServicer_to_server(servicer, server)
    server._servicer = servicer  # keep a reference for warmup
    return server


def serve(address: str | None = None) -> NoReturn:
    """Serve until terminated, using MOUSEION_NLP_ADDR when set."""
    bind_address = address or os.getenv("MOUSEION_NLP_ADDR", "[::]:50051")
    server = create_server()
    # Pre-warm the Stanza pipeline (downloads the model on first run and loads it
    # into memory) BEFORE the server accepts analysis requests. Otherwise the
    # first Analyze call pays the download+load cost and can exceed the caller's
    # RPC deadline (observed as `DeadlineExceeded`).
    warm_language = os.getenv("MOUSEION_NLP_WARM_LANGUAGE", "de")
    print(f"warming Stanza pipeline for language '{warm_language}'…", flush=True)
    server._servicer.warmup(warm_language)
    print("Stanza pipeline warm", flush=True)
    port = server.add_insecure_port(bind_address)
    if port == 0:
        raise RuntimeError(f"could not bind NLP gRPC server to {bind_address}")
    server.start()
    display_address = bind_address
    if display_address.endswith(":0"):
        display_address = f"{bind_address[:-1]}{port}"
    print(f"mouseion NLP gRPC server listening on {display_address}", flush=True)
    server.wait_for_termination()
    raise RuntimeError("gRPC server stopped unexpectedly")


if __name__ == "__main__":
    serve()
