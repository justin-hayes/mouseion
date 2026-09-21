from pathlib import Path
import re


COMPOSE_FILE = Path(__file__).parents[2] / "compose.yaml"
DEFAULT_LANGUAGES = "MOUSEION_NLP_WARM_LANGUAGES: ${MOUSEION_NLP_WARM_LANGUAGES:-de,it,el}"


def _service_block(compose: str, service: str) -> str:
    """Return one service section without requiring a YAML parser dependency."""
    match = re.search(rf"^  {re.escape(service)}:\n", compose, re.MULTILINE)
    assert match is not None, f"Compose service {service!r} is missing"
    remainder = compose[match.end() :]
    next_service = re.search(r"^  [a-z][a-z0-9-]*:\n", remainder, re.MULTILINE)
    return remainder[: next_service.start()] if next_service else remainder


def test_standard_nlp_services_share_the_configured_model_caches() -> None:
    compose = COMPOSE_FILE.read_text(encoding="utf-8")

    for service in ("nlp-init", "nlp"):
        block = _service_block(compose, service)
        assert DEFAULT_LANGUAGES in block
        assert "STANZA_RESOURCES_DIR: /opt/stanza_resources" in block
        assert "HF_HOME: /opt/huggingface" in block
        assert "- stanza-data:/opt/stanza_resources" in block
        assert "- huggingface-data:/opt/huggingface" in block

    assert re.search(r"^  huggingface-data:\s*$", compose, re.MULTILINE)


def test_nlp_waits_for_successful_model_provisioning() -> None:
    nlp = _service_block(COMPOSE_FILE.read_text(encoding="utf-8"), "nlp")

    assert "    depends_on:\n      nlp-init:\n        condition: service_completed_successfully" in nlp
