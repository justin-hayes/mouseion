"""Coarse, batch-oriented NLP producer boundary."""

from mouseion.v1 import normalized_corpus_pb2


class Producer:
    """Placeholder producer to be backed by Stanza in a later issue."""

    def analyze(self, text: str, language: str) -> normalized_corpus_pb2.NormalizedCorpus:
        """Return a minimal normalized artifact for pipeline wiring.

        A later issue will replace this placeholder analysis with Stanza while
        retaining this generated Protobuf message as the producer contract.
        """
        return normalized_corpus_pb2.NormalizedCorpus(
            schema_version="1.0.0",
            language=language,
            sentences=[normalized_corpus_pb2.Sentence(text=text)],
        )
