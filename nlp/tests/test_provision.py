import dataclasses
import hashlib
import json
from unittest.mock import patch

import pytest
from huggingface_hub.errors import LocalEntryNotFoundError

from mouseion_nlp import provision as provision_module
from mouseion_nlp.model_config import (
    GREEK_BERT_MODEL,
    LanguageModelConfig,
    model_config_for_language,
    stanza_pretrain_path,
)
from mouseion_nlp.provision import (
    MARKER_FILENAME,
    ArtifactChecksumMismatchError,
    ExternalModels,
    ensure_external_dependency,
    main,
    provision,
    read_marker,
    wipe_resources,
)

STANZA_VERSION = "1.15.0"


class RecordingDownload:
    """Stand-in for ``stanza.download`` that records each invocation.

    The real download pulls every declared Stanza pretrain as a side effect of
    the processor dependency graph, so this fake materializes the declared
    pretrains too (resolving through the provisioner's config hook, so a patched
    resolver is honored) unless told to omit them.
    """

    def __init__(self, *, materialize_pretrains: bool = True) -> None:
        self.calls: list[tuple[str, dict]] = []
        self.materialize_pretrains = materialize_pretrains

    def __call__(self, language: str, **kwargs) -> None:
        self.calls.append((language, kwargs))
        if not self.materialize_pretrains:
            return
        config = provision_module.model_config_for_language(language)
        for package in (config.pretrain_dependencies or {}).values():
            pretrain_path = stanza_pretrain_path(kwargs["model_dir"], language, package)
            pretrain_path.parent.mkdir(parents=True, exist_ok=True)
            pretrain_path.write_text("pretrain", encoding="utf-8")

    @property
    def languages(self) -> list[str]:
        return [language for language, _kwargs in self.calls]


class RecordingExternalDownload:
    def __init__(self) -> None:
        self.calls: list[tuple[str, dict]] = []

    def __call__(self, repo_id: str, **kwargs) -> None:
        self.calls.append((repo_id, kwargs))


def sha256_hex(content: bytes) -> str:
    return hashlib.sha256(content).hexdigest()


def test_empty_directory_provisions_every_configured_language(tmp_path) -> None:
    download = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=download)

    assert download.languages == ["de", "it"]
    for _language, kwargs in download.calls:
        assert kwargs == {
            "model_dir": str(tmp_path),
            "processors": "tokenize,pos,lemma,depparse",
            "verbose": False,
        }
    marker = read_marker(tmp_path)
    assert (marker.stanza_version, marker.languages) == (STANZA_VERSION, ("de", "it"))
    assert marker.model_configs == (
        LanguageModelConfig(language="de"),
        LanguageModelConfig(language="it"),
    )

    assert json.loads((tmp_path / MARKER_FILENAME).read_text(encoding="utf-8")) == {
        "languages": ["de", "it"],
        "model_configs": [
            {
                "external_model_dependencies": [],
                "language": "de",
                "model_version": None,
                "package": None,
                "pretrain_dependencies": None,
                "processors": "tokenize,pos,lemma,depparse",
            },
            {
                "external_model_dependencies": [],
                "language": "it",
                "model_version": None,
                "package": None,
                "pretrain_dependencies": None,
                "processors": "tokenize,pos,lemma,depparse",
            },
        ],
        "stanza_version": STANZA_VERSION,
    }


def test_second_run_against_the_same_directory_is_a_no_op(tmp_path) -> None:
    provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload())
    rerun = RecordingDownload()

    provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=rerun)

    assert rerun.calls == []
    assert read_marker(tmp_path).languages == ("de",)


def test_changed_stanza_version_wipes_and_provisions_from_scratch(tmp_path) -> None:
    provision(tmp_path, ["de"], stanza_version="1.14.0", download=RecordingDownload())
    sentinel = tmp_path / "de" / "tokenize" / "stale.pt"
    sentinel.parent.mkdir(parents=True)
    sentinel.write_text("stale", encoding="utf-8")
    upgraded = RecordingDownload()

    provision(tmp_path, ["de"], stanza_version="1.15.0", download=upgraded)

    assert not sentinel.exists()
    assert upgraded.languages == ["de"]
    assert read_marker(tmp_path).stanza_version == "1.15.0"


@pytest.mark.parametrize(
    "changed_config",
    [
        LanguageModelConfig(language="de", processors="tokenize,pos,lemma"),
        LanguageModelConfig(language="de", package="accurate"),
        LanguageModelConfig(language="de", pretrain_dependencies={"lemma": "conll17"}),
        LanguageModelConfig(language="de", external_model_dependencies=("example/model",)),
    ],
)
def test_changed_model_configuration_wipes_and_provisions_from_scratch(
    tmp_path, changed_config
) -> None:
    provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload())
    sentinel = tmp_path / "de" / "tokenize" / "stale.pt"
    sentinel.parent.mkdir(parents=True)
    sentinel.write_text("stale", encoding="utf-8")
    original_resolver = model_config_for_language
    upgraded = RecordingDownload()
    external_download = RecordingExternalDownload()

    def changed_resolver(language: str) -> LanguageModelConfig:
        return changed_config if language == "de" else original_resolver(language)

    with patch("mouseion_nlp.provision.model_config_for_language", changed_resolver):
        provision(
            tmp_path,
            ["de"],
            stanza_version=STANZA_VERSION,
            download=upgraded,
            external=ExternalModels(download=external_download),
        )

    assert not sentinel.exists()
    assert upgraded.languages == ["de"]
    assert read_marker(tmp_path).model_configs == (changed_config,)


def test_external_model_dependencies_are_marker_data_not_stanza_downloads(tmp_path) -> None:
    config = LanguageModelConfig(language="el", external_model_dependencies=("example/model",))
    download = RecordingDownload()
    external_download = RecordingExternalDownload()

    with patch("mouseion_nlp.provision.model_config_for_language", return_value=config):
        provision(
            tmp_path,
            ["el"],
            stanza_version=STANZA_VERSION,
            download=download,
            external=ExternalModels(download=external_download, hf_home=tmp_path / "huggingface"),
        )

    assert download.languages == ["el"]
    assert download.calls[0][1]["processors"] == config.processors
    assert external_download.calls == [
        ("example/model", {"cache_dir": str(tmp_path / "huggingface" / "hub")})
    ]
    assert read_marker(tmp_path).model_configs == (config,)


def test_missing_external_cache_is_fetched_even_with_a_current_marker(tmp_path) -> None:
    external_download = RecordingExternalDownload()
    with patch(
        "mouseion_nlp.provision.snapshot_download", side_effect=LocalEntryNotFoundError("missing")
    ):
        ensure_external_dependency(
            "example/model", ExternalModels(download=external_download, hf_home=tmp_path)
        )

    assert external_download.calls == [("example/model", {"cache_dir": str(tmp_path / "hub")})]


def test_greek_provisions_stanza_package_and_greekbert(tmp_path) -> None:
    download = RecordingDownload()
    external_download = RecordingExternalDownload()

    provision(
        tmp_path,
        ["el"],
        stanza_version=STANZA_VERSION,
        download=download,
        external=ExternalModels(download=external_download, hf_home=tmp_path / "huggingface"),
    )

    assert download.calls == [
        (
            "el",
            {
                "model_dir": str(tmp_path),
                "processors": "tokenize,mwt,pos,lemma,depparse",
                "verbose": False,
                "package": {
                    "tokenize": "gdt",
                    "mwt": "gdt",
                    "pos": "gdt_nocharlm",
                    "lemma": "gdt_nocharlm",
                    "depparse": "gdt_greek-bert",
                },
            },
        )
    ]
    assert external_download.calls == [
        (GREEK_BERT_MODEL, {"cache_dir": str(tmp_path / "huggingface" / "hub")})
    ]
    assert (tmp_path / "el" / "pretrain" / "conll17.pt").is_file()
    assert read_marker(tmp_path).model_configs == (model_config_for_language("el"),)


def test_declared_pretrain_missing_after_download_aborts_without_writing_the_marker(
    tmp_path,
) -> None:
    download = RecordingDownload(materialize_pretrains=False)

    with pytest.raises(FileNotFoundError, match="conll17"):
        provision(
            tmp_path,
            ["el"],
            stanza_version=STANZA_VERSION,
            download=download,
            external=ExternalModels(
                download=RecordingExternalDownload(), hf_home=tmp_path / "huggingface"
            ),
        )

    assert read_marker(tmp_path) is None


def _provision_with_declared_artifact(
    tmp_path, content: bytes, *, declared_checksum: str | None = None
) -> LanguageModelConfig:
    relative_artifact = "de/tokenize/package.pt"
    checksum = sha256_hex(content) if declared_checksum is None else declared_checksum
    config = LanguageModelConfig(language="de", artifact_checksums={relative_artifact: checksum})

    def downloading(language: str, **kwargs) -> None:
        artifact = tmp_path / relative_artifact
        artifact.parent.mkdir(parents=True, exist_ok=True)
        artifact.write_bytes(content)

    with patch("mouseion_nlp.provision.model_config_for_language", return_value=config):
        provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=downloading)
    return config


def test_matching_declared_artifact_checksums_succeed_and_are_marked(tmp_path) -> None:
    config = _provision_with_declared_artifact(tmp_path, b"model bytes")

    assert read_marker(tmp_path).model_configs == (config,)
    payload = json.loads((tmp_path / MARKER_FILENAME).read_text(encoding="utf-8"))
    assert payload["model_configs"][0]["artifact_checksums"] == config.artifact_checksums


def test_mismatched_artifact_checksum_aborts_without_writing_the_marker(tmp_path) -> None:
    with pytest.raises(ArtifactChecksumMismatchError, match="expected sha256"):
        _provision_with_declared_artifact(
            tmp_path,
            b"tampered bytes",
            declared_checksum=sha256_hex(b"expected bytes"),
        )

    assert read_marker(tmp_path) is None


def test_missing_declared_artifact_aborts_without_writing_the_marker(tmp_path) -> None:
    config = LanguageModelConfig(
        language="de",
        artifact_checksums={"de/tokenize/package.pt": sha256_hex(b"model bytes")},
    )

    with (
        patch("mouseion_nlp.provision.model_config_for_language", return_value=config),
        pytest.raises(FileNotFoundError, match="missing after download"),
    ):
        provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=lambda *a, **k: None)

    assert read_marker(tmp_path) is None


def test_changed_artifact_checksums_wipe_and_provision_from_scratch(tmp_path) -> None:
    _provision_with_declared_artifact(tmp_path, b"first bytes")
    sentinel = tmp_path / "de" / "tokenize" / "stale.pt"
    sentinel.write_text("stale", encoding="utf-8")
    downloads: list[str] = []

    def downloading(language: str, **kwargs) -> None:
        downloads.append(language)
        artifact = tmp_path / "de" / "tokenize" / "package.pt"
        artifact.parent.mkdir(parents=True, exist_ok=True)
        artifact.write_bytes(b"second bytes")

    config = LanguageModelConfig(
        language="de",
        artifact_checksums={"de/tokenize/package.pt": sha256_hex(b"second bytes")},
    )
    with patch("mouseion_nlp.provision.model_config_for_language", return_value=config):
        provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=downloading)

    assert not sentinel.exists()
    assert downloads == ["de"]
    assert read_marker(tmp_path).model_configs == (config,)


EXPECTED_FRENCH_ARTIFACTS = {
    "fr/tokenize/combined_nocharlm.pt": (
        "622a7613d78623cda63e53fc72f84c49b2f64ad959c0f6aeb51355b963eda78f"
    ),
    "fr/mwt/combined.pt": "a46bc4149531762c2fd79d70eeaaff80479aa6852a08516ab0db567859c8cec0",
    "fr/lemma/combined_nocharlm.pt": (
        "31dd152c13d1d713085127e7b30d91e70e88652da4c2ea33c4b6f7d34d810177"
    ),
    "fr/pos/combined_charlm.pt": (
        "e40eb06786fa691e84aa6ee1ff2321b37f9ff9cb9bf3fe4de94b63793b626f89"
    ),
    "fr/depparse/combined_charlm.pt": (
        "5ad85cd10e385894152154863c14f621b7d747c67c84eb8597039da12832315a"
    ),
    "fr/pretrain/conll17.pt": "c8c456c1a48fba32d4ab827ae7d3fec40457f58ead7e1977b6267afc37474a3c",
    "fr/forward_charlm/newswiki.pt": (
        "a6874f7670f84c8bd9e57d5dcbafdfe7d7cbae2457c79975040709f1d69ca119"
    ),
    "fr/backward_charlm/newswiki.pt": (
        "3ae24cade5cdca7d68193196a942a177af81fbc5d3b5ebb4550ff7b89023b09c"
    ),
}


def test_french_declares_the_eight_pinned_artifact_checksums() -> None:
    config = model_config_for_language("fr")

    assert config.artifact_checksums == EXPECTED_FRENCH_ARTIFACTS


def test_french_provisioning_verifies_artifacts_and_records_the_selection(tmp_path) -> None:
    base = model_config_for_language("fr")
    assert base.artifact_checksums is not None
    contents = {path: f"bytes for {path}".encode() for path in base.artifact_checksums}
    config = dataclasses.replace(
        base,
        artifact_checksums={path: sha256_hex(content) for path, content in contents.items()},
    )

    def downloading(language: str, **kwargs) -> None:
        assert language == "fr"
        for path, content in contents.items():
            artifact = tmp_path / path
            artifact.parent.mkdir(parents=True, exist_ok=True)
            artifact.write_bytes(content)

    with patch("mouseion_nlp.provision.model_config_for_language", return_value=config):
        provision(tmp_path, ["fr"], stanza_version=STANZA_VERSION, download=downloading)

    marker = read_marker(tmp_path)
    assert marker is not None
    assert marker.languages == ("fr",)
    assert marker.model_configs == (config,)
    assert marker.model_configs[0].package == {
        "tokenize": "combined_nocharlm",
        "mwt": "combined",
        "pos": "combined_charlm",
        "lemma": "combined_nocharlm",
        "depparse": "combined_charlm",
    }
    assert marker.model_configs[0].model_version == "stanza-1.15.0-fr-combined"


def test_french_artifact_checksum_mismatch_aborts_without_writing_the_marker(tmp_path) -> None:
    artifact_path = "fr/pos/combined_charlm.pt"
    config = dataclasses.replace(
        model_config_for_language("fr"),
        artifact_checksums={artifact_path: sha256_hex(b"pinned bytes")},
    )

    def downloading(language: str, **kwargs) -> None:
        artifact = tmp_path / artifact_path
        artifact.parent.mkdir(parents=True, exist_ok=True)
        artifact.write_bytes(b"tampered bytes")

    with (
        patch("mouseion_nlp.provision.model_config_for_language", return_value=config),
        pytest.raises(ArtifactChecksumMismatchError, match="expected sha256"),
    ):
        provision(tmp_path, ["fr"], stanza_version=STANZA_VERSION, download=downloading)

    assert read_marker(tmp_path) is None


def test_added_language_downloads_only_the_missing_language(tmp_path) -> None:
    provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=RecordingDownload())
    grown = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=grown)

    assert grown.languages == ["it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_missing_marker_wipes_a_partially_baked_volume(tmp_path) -> None:
    partial = tmp_path / "de" / "tokenize"
    partial.mkdir(parents=True)
    (partial / "baked.pt").write_text("baked", encoding="utf-8")
    download = RecordingDownload()

    provision(tmp_path, ["de", "it"], stanza_version=STANZA_VERSION, download=download)

    assert not (tmp_path / "de").exists()
    assert download.languages == ["de", "it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_failed_download_aborts_without_writing_the_marker(tmp_path) -> None:
    def failing_download(language: str, **kwargs) -> None:
        raise RuntimeError("model source unreachable")

    with pytest.raises(RuntimeError, match="model source unreachable"):
        provision(tmp_path, ["de"], stanza_version=STANZA_VERSION, download=failing_download)

    assert read_marker(tmp_path) is None


def test_main_exits_nonzero_when_a_download_fails(tmp_path) -> None:
    with (
        patch.dict(
            "os.environ",
            {"MOUSEION_NLP_WARM_LANGUAGES": "de", "STANZA_RESOURCES_DIR": str(tmp_path)},
            clear=True,
        ),
        patch("mouseion_nlp.provision.stanza.download", side_effect=RuntimeError("offline")),
    ):
        assert main() == 1


def test_main_provisions_the_configured_languages(tmp_path) -> None:
    with (
        patch.dict(
            "os.environ",
            {"MOUSEION_NLP_WARM_LANGUAGES": "de,it", "STANZA_RESOURCES_DIR": str(tmp_path)},
            clear=True,
        ),
        patch("mouseion_nlp.provision.stanza.download") as download,
    ):
        assert main() == 0

    assert [call.args[0] for call in download.call_args_list] == ["de", "it"]
    assert read_marker(tmp_path).languages == ("de", "it")


def test_read_marker_treats_a_corrupt_marker_as_absent(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text("not json", encoding="utf-8")

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_a_wrong_marker_schema(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text('{"languages": ["de"]}', encoding="utf-8")

    assert read_marker(tmp_path) is None


def test_read_marker_treats_a_legacy_marker_as_absent(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps({"languages": ["de"], "stanza_version": STANZA_VERSION}),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


def test_wipe_resources_keeps_the_directory(tmp_path) -> None:
    (tmp_path / "de").mkdir()

    wipe_resources(tmp_path)

    assert tmp_path.is_dir()
    assert list(tmp_path.iterdir()) == []


def test_read_marker_ignores_a_non_string_language_set(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps({"stanza_version": STANZA_VERSION, "languages": 5}), encoding="utf-8"
    )

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_a_non_object_payload(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text("[]", encoding="utf-8")

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_a_non_string_language_in_the_list(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps({"stanza_version": STANZA_VERSION, "languages": ["de", 5], "model_configs": []}),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_a_language_without_a_matching_config(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps({"stanza_version": STANZA_VERSION, "languages": ["de"], "model_configs": []}),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


def test_read_marker_ignores_an_invalid_model_package(tmp_path) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [
                    {
                        "language": "de",
                        "processors": "tokenize,pos,lemma,depparse",
                        "package": 5,
                        "external_model_dependencies": [],
                        "model_version": None,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


def test_marker_round_trips_declared_artifact_checksums(tmp_path) -> None:
    config = LanguageModelConfig(
        language="de",
        artifact_checksums={
            "de/tokenize/package.pt": "a" * 64,
            "de/pretrain/conll17.pt": "b" * 64,
        },
    )
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [config.marker_payload()],
            }
        ),
        encoding="utf-8",
    )

    marker = read_marker(tmp_path)

    assert marker is not None
    assert marker.model_configs == (config,)


def test_read_marker_accepts_a_marker_without_declared_artifact_checksums(tmp_path) -> None:
    # Markers written before the field existed stay valid for languages that
    # declare no checksums, so upgrading does not force a re-provision.
    config = LanguageModelConfig(language="de")
    payload = config.marker_payload()
    assert "artifact_checksums" not in payload
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [payload],
            }
        ),
        encoding="utf-8",
    )

    marker = read_marker(tmp_path)

    assert marker is not None
    assert marker.model_configs == (config,)


@pytest.mark.parametrize(
    "invalid_checksums",
    [5, ["de/tokenize/package.pt"], {"de/tokenize/package.pt": 5}],
)
def test_read_marker_ignores_an_invalid_artifact_checksum_shape(
    tmp_path, invalid_checksums
) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [
                    {
                        "language": "de",
                        "processors": "tokenize,pos,lemma,depparse",
                        "package": None,
                        "pretrain_dependencies": None,
                        "external_model_dependencies": [],
                        "model_version": None,
                        "artifact_checksums": invalid_checksums,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


def test_marker_round_trips_declared_pretrain_dependencies(tmp_path) -> None:
    config = LanguageModelConfig(language="el", pretrain_dependencies={"lemma": "conll17"})
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["el"],
                "model_configs": [config.marker_payload()],
            }
        ),
        encoding="utf-8",
    )

    marker = read_marker(tmp_path)

    assert marker is not None
    assert marker.model_configs == (config,)


def test_read_marker_rejects_a_marker_without_pretrain_dependencies(tmp_path) -> None:
    # A marker written before the field existed must force re-provisioning.
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [
                    {
                        "language": "de",
                        "processors": "tokenize,pos,lemma,depparse",
                        "package": None,
                        "external_model_dependencies": [],
                        "model_version": None,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


@pytest.mark.parametrize("invalid_pretrain", [5, {"lemma": 5}, ["lemma"]])
def test_read_marker_ignores_an_invalid_pretrain_dependency_shape(
    tmp_path, invalid_pretrain
) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [
                    {
                        "language": "de",
                        "processors": "tokenize,pos,lemma,depparse",
                        "package": None,
                        "pretrain_dependencies": invalid_pretrain,
                        "external_model_dependencies": [],
                        "model_version": None,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None


@pytest.mark.parametrize(
    "invalid_identity",
    [{"language": 5, "processors": "tokenize"}, {"language": "de", "processors": ""}],
)
def test_read_marker_ignores_an_invalid_language_or_processor_shape(
    tmp_path, invalid_identity
) -> None:
    (tmp_path / MARKER_FILENAME).write_text(
        json.dumps(
            {
                "stanza_version": STANZA_VERSION,
                "languages": ["de"],
                "model_configs": [
                    {
                        "package": None,
                        "pretrain_dependencies": None,
                        "external_model_dependencies": [],
                        "model_version": None,
                        **invalid_identity,
                    }
                ],
            }
        ),
        encoding="utf-8",
    )

    assert read_marker(tmp_path) is None
