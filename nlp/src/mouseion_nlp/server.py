"""Long-lived gRPC server for the Stanza analyzer."""

from __future__ import annotations

from concurrent import futures
from dataclasses import dataclass
import os
import sys
from typing import NoReturn

import grpc
import stanza
from mouseion.v1 import normalized_corpus_pb2, normalized_corpus_pb2_grpc
from stanza.models.common.constant import lcode2lang

from .producer import Producer, SourceDocument


SUPPORTED_FEATURES = ("tokenize", "pos", "lemma")


@dataclass
class LanguageDescriptor:
    """Configured language metadata and its independently tracked runtime state."""

    language: str
    display_name: str
    supported_features: tuple[str, ...]
    model_version: str = ""
    ready: bool = False


def configured_languages() -> list[LanguageDescriptor]:
    """Return configured languages in stable code order, honoring the legacy setting."""
    configured = os.getenv("MOUSEION_NLP_WARM_LANGUAGES")
    if configured is None:
        configured = os.getenv("MOUSEION_NLP_WARM_LANGUAGE", "de")
    codes = sorted({code.strip().lower() for code in configured.split(",") if code.strip()})
    if not codes:
        codes = ["de"]
    return [
        LanguageDescriptor(
            language=code,
            display_name=lcode2lang.get(code, code),
            supported_features=SUPPORTED_FEATURES,
        )
        for code in codes
    ]


class AnalyzerServicer(normalized_corpus_pb2_grpc.AnalyzerServiceServicer):
    """Adapt gRPC analyzer requests to one warm Producer instance."""

    def __init__(
        self,
        producer: Producer | None = None,
        languages: list[LanguageDescriptor] | None = None,
    ) -> None:
        self._producer = producer or Producer()
        self._languages = languages if languages is not None else configured_languages()

    def warmup(self, language: str) -> None:
        """Load (and, on first run, download) the Stanza pipeline for a language.

        Called at server startup so the slow model download/load happens before
        the server accepts analysis requests, instead of on the first Analyze
        call (which could otherwise exceed the caller's RPC deadline).
        """
        descriptor = next(
            (descriptor for descriptor in self._languages if descriptor.language == language), None
        )
        if descriptor is None:
            raise ValueError(f"language '{language}' is not configured")
        self._producer.warmup(language)
        model_version = getattr(self._producer, "model_version", None)
        descriptor.model_version = (
            model_version(language) if model_version is not None else stanza.__version__
        )
        descriptor.ready = True

    def GetCapabilities(self, request, context):  # noqa: ARG002, N802
        return normalized_corpus_pb2.GetCapabilitiesResponse(
            languages=[
                normalized_corpus_pb2.LanguageCapability(
                    language=descriptor.language,
                    display_name=descriptor.display_name,
                    model_version=descriptor.model_version,
                    supported_features=[
                        *descriptor.supported_features,
                        *(["ner"] if self._producer.enable_ner else []),
                    ],
                    ready=descriptor.ready,
                )
                for descriptor in self._languages
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


def create_server(
    producer: Producer | None = None,
    languages: list[LanguageDescriptor] | None = None,
) -> grpc.Server:
    """Create a server with an injectable producer for tests."""
    servicer = AnalyzerServicer(producer, languages)
    server = grpc.server(futures.ThreadPoolExecutor())
    normalized_corpus_pb2_grpc.add_AnalyzerServiceServicer_to_server(servicer, server)
    server._servicer = servicer  # keep a reference for warmup
    return server


def warm_configured_languages(servicer: AnalyzerServicer) -> None:
    """Attempt every configured warmup while preserving per-language state."""
    for descriptor in servicer._languages:
        language = descriptor.language
        print(f"warming Stanza pipeline for language '{language}'…", flush=True)
        try:
            servicer.warmup(language)
        except Exception as error:
            print(
                f"failed to warm Stanza pipeline for language '{language}': {error}",
                file=sys.stderr,
                flush=True,
            )
        else:
            print(f"Stanza pipeline for language '{language}' warm", flush=True)


def serve(address: str | None = None) -> NoReturn:
    """Serve until terminated, using MOUSEION_NLP_ADDR when set."""
    bind_address = address or os.getenv("MOUSEION_NLP_ADDR", "[::]:50051")
    server = create_server()
    # Pre-warm the Stanza pipeline (downloads the model on first run and loads it
    # into memory) BEFORE the server accepts analysis requests. Otherwise the
    # first Analyze call pays the download+load cost and can exceed the caller's
    # RPC deadline (observed as `DeadlineExceeded`).
    warm_configured_languages(server._servicer)
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
