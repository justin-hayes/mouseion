import re
from pathlib import Path

REPO_ROOT = Path(__file__).parents[2]
COMPOSE_FILE = REPO_ROOT / "compose.yaml"
NLP_DOCKERFILE = REPO_ROOT / "nlp" / "Dockerfile"
DEFAULT_LANGUAGES = "MOUSEION_NLP_WARM_LANGUAGES: ${MOUSEION_NLP_WARM_LANGUAGES:-de,it,el}"
NLP_UID = "10001"


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

    assert (
        "    depends_on:\n      nlp-init:\n        condition: service_completed_successfully" in nlp
    )


def test_nlp_image_runs_as_a_dedicated_non_root_user() -> None:
    dockerfile = NLP_DOCKERFILE.read_text(encoding="utf-8")

    user_directives = re.findall(r"^USER\s+(\S+)\s*$", dockerfile, re.MULTILINE)
    assert user_directives == [f"{NLP_UID}:{NLP_UID}"]
    assert f"--uid {NLP_UID}" in dockerfile
    assert f"chown {NLP_UID}:{NLP_UID} /opt/stanza_resources /opt/huggingface" in dockerfile
    # The USER directive must come after every root-only install step and before the entrypoint.
    assert dockerfile.index(f"USER {NLP_UID}:{NLP_UID}") < dockerfile.index("ENTRYPOINT")
    assert dockerfile.index(f"USER {NLP_UID}:{NLP_UID}") > dockerfile.index("pip install")


def test_nlp_services_do_not_override_the_image_user_to_root() -> None:
    compose = COMPOSE_FILE.read_text(encoding="utf-8")

    nlp = _service_block(compose, "nlp")
    assert not re.search(r"^    user:", nlp, re.MULTILINE)


def test_nlp_init_fixes_model_volume_ownership_before_dropping_privileges() -> None:
    # Volumes created before the non-root image were populated as root, so the
    # provisioner must chown them before it runs as the NLP user.
    init = _service_block(COMPOSE_FILE.read_text(encoding="utf-8"), "nlp-init")

    assert 'user: "0:0"' in init
    assert f"chown -R {NLP_UID}:{NLP_UID} /opt/stanza_resources /opt/huggingface" in init
    assert f"setpriv --reuid={NLP_UID} --regid={NLP_UID} --clear-groups" in init
    assert "mouseion_nlp.provision" in init
