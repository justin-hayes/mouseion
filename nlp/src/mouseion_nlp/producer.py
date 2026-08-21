"""Coarse, batch-oriented NLP producer boundary."""


class Producer:
    """Placeholder producer to be backed by Stanza in a later issue."""

    def analyze(self, text: str, language: str) -> dict[str, object]:
        """Return a minimal normalized artifact for pipeline wiring.

        The returned shape mirrors the v1 `NormalizedCorpus` protobuf contract
        (`proto/mouseion/v1/normalized_corpus.proto`): `sentences` is a list of
        `Sentence`-shaped dicts with `text` and `tokens`.
        """
        return {
            "schema_version": "v1",
            "language": language,
            "sentences": [{"text": text, "tokens": []}],
        }
