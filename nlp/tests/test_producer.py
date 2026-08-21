from mouseion_nlp import Producer


def test_producer_returns_versioned_artifact() -> None:
    artifact = Producer().analyze("Ein Haus.", "de")

    assert artifact["schema_version"] == "v1"
    assert artifact["language"] == "de"
