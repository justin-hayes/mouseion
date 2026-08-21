"""Coarse, batch-oriented NLP producer boundary."""


class Producer:
    """Placeholder producer to be backed by Stanza in a later issue."""

    def analyze(self, text: str, language: str) -> dict[str, object]:
        """Return a minimal normalized artifact for pipeline wiring."""
        return {"schema_version": "v1", "language": language, "sentences": [text]}
