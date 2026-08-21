"""Long-lived gRPC server for the Stanza analyzer."""

from __future__ import annotations

from concurrent import futures
import os
from typing import NoReturn

import grpc
from mouseion.v1 import normalized_corpus_pb2_grpc

from .producer import Producer, SourceDocument


class AnalyzerServicer(normalized_corpus_pb2_grpc.AnalyzerServiceServicer):
    """Adapt gRPC analyzer requests to one warm Producer instance."""

    def __init__(self, producer: Producer | None = None) -> None:
        self._producer = producer or Producer()

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
    server = grpc.server(futures.ThreadPoolExecutor())
    normalized_corpus_pb2_grpc.add_AnalyzerServiceServicer_to_server(
        AnalyzerServicer(producer), server
    )
    return server


def serve(address: str | None = None) -> NoReturn:
    """Serve until terminated, using MOUSEION_NLP_ADDR when set."""
    bind_address = address or os.getenv("MOUSEION_NLP_ADDR", "[::]:50051")
    server = create_server()
    port = server.add_insecure_port(bind_address)
    if port == 0:
        raise RuntimeError(f"could not bind NLP gRPC server to {bind_address}")
    server.start()
    display_address = bind_address
    if bind_address.endswith(":0"):
        display_address = f"{bind_address[:-1]}{port}"
    print(f"mouseion NLP gRPC server listening on {display_address}", flush=True)
    server.wait_for_termination()
    raise RuntimeError("gRPC server stopped unexpectedly")


if __name__ == "__main__":
    serve()
