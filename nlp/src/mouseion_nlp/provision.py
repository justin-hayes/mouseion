"""One-shot provisioner for the Stanza model bundle on a persistent volume.

The NLP service owns the configured language set (ADR 0023) and advertises the
public runtime processor set. This entrypoint fills ``STANZA_RESOURCES_DIR``
with each language's effective Stanza package and processors, and verifies
external model dependencies in the ``HF_HOME`` cache, so the serving container
never downloads at startup.

Provisioning is idempotent and refresh-aware via a marker file written into the
resource directory:

- no marker (e.g. a volume from an image that only baked some processors) means
  the directory is wiped and every language is provisioned fresh;
- a changed Stanza version or model configuration means the directory is wiped
  and re-provisioned;
- a grown language set only downloads the missing languages.

Any download failure propagates and aborts the process with a non-zero exit.
Run it as ``python -m mouseion_nlp.provision``.
"""

from __future__ import annotations

import hashlib
import json
import os
import shutil
import sys
from collections.abc import Callable, Iterable
from dataclasses import dataclass
from pathlib import Path

import stanza
from huggingface_hub import snapshot_download
from huggingface_hub.errors import LocalEntryNotFoundError
from stanza.resources.common import DEFAULT_MODEL_DIR

from .model_config import LanguageModelConfig, model_config_for_language, stanza_pretrain_path
from .server import configured_languages

MARKER_FILENAME = ".mouseion-stanza-provision.json"
DEFAULT_HF_HOME = Path("/opt/huggingface")


class ArtifactChecksumMismatchError(RuntimeError):
    """Raised when a fetched artifact's sha256 differs from its declaration."""


def configured_hf_home() -> Path:
    """Return the explicit Hugging Face cache root for this process."""
    return Path(os.getenv("HF_HOME", DEFAULT_HF_HOME))


@dataclass(frozen=True)
class Marker:
    """The inputs that identify the resources recorded on disk."""

    stanza_version: str
    languages: tuple[str, ...]
    model_configs: tuple[LanguageModelConfig, ...] = ()


@dataclass(frozen=True)
class ProvisionPlan:
    """What a provision run must do, derived from the existing marker."""

    wipe: bool
    downloads: tuple[str, ...]
    provisioned: tuple[str, ...]


@dataclass(frozen=True)
class ExternalModels:
    """Where external (non-Stanza) models are fetched and cached."""

    download: Callable[..., object] = snapshot_download
    hf_home: Path | None = None

    def cache_root(self) -> Path:
        """Resolve the Hugging Face cache root for this run."""
        return Path(self.hf_home) if self.hf_home is not None else configured_hf_home()


def marker_path(resources_dir: Path) -> Path:
    """Return the marker location inside a Stanza resource directory."""
    return resources_dir / MARKER_FILENAME


def read_marker(resources_dir: Path) -> Marker | None:
    """Read the marker, treating a missing or unreadable marker as absent."""
    path = marker_path(resources_dir)
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
        return _marker_from_payload(payload)
    except (OSError, ValueError, TypeError):
        return None


def _marker_from_payload(payload: object) -> Marker | None:
    """Build a marker from a decoded payload, rejecting any invalid shape."""
    if not isinstance(payload, dict):
        return None
    stanza_version = payload.get("stanza_version")
    raw_languages = payload.get("languages")
    raw_configs = payload.get("model_configs")
    if not isinstance(stanza_version, str) or not isinstance(raw_languages, list):
        return None
    if not all(isinstance(language, str) for language in raw_languages):
        return None
    languages = tuple(sorted(set(raw_languages)))
    if not isinstance(raw_configs, list):
        return None
    model_configs = tuple(
        sorted(
            (
                config
                for payload_config in raw_configs
                if (config := LanguageModelConfig.from_marker_payload(payload_config)) is not None
            ),
            key=lambda config: config.language,
        )
    )
    if (
        len(model_configs) != len(raw_configs)
        or tuple(config.language for config in model_configs) != languages
    ):
        return None
    return Marker(stanza_version=stanza_version, languages=languages, model_configs=model_configs)


def write_marker(resources_dir: Path, marker: Marker) -> None:
    """Persist the marker describing what the directory now holds."""
    resources_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "stanza_version": marker.stanza_version,
        "languages": list(marker.languages),
        "model_configs": [config.marker_payload() for config in marker.model_configs],
    }
    marker_path(resources_dir).write_text(
        json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8"
    )


def plan_provisioning(
    marker: Marker | None, stanza_version: str, languages: Iterable[str]
) -> ProvisionPlan:
    """Decide whether to wipe and which languages to download.

    A missing marker, changed Stanza version, or changed configuration for an
    already-provisioned language invalidates everything already on disk.
    Otherwise only languages absent from the marker are downloaded.
    """
    configured = tuple(
        dict.fromkeys(model_config_for_language(language).language for language in languages)
    )
    if marker is None or marker.stanza_version != stanza_version:
        return ProvisionPlan(wipe=True, downloads=configured, provisioned=configured)
    marker_configs = {config.language: config for config in marker.model_configs}
    expected_configs = {language: model_config_for_language(language) for language in configured}
    if any(
        language in marker.languages and marker_configs.get(language) != config
        for language, config in expected_configs.items()
    ):
        return ProvisionPlan(wipe=True, downloads=configured, provisioned=configured)
    missing = tuple(language for language in configured if language not in marker.languages)
    return ProvisionPlan(wipe=False, downloads=missing, provisioned=configured)


def wipe_resources(resources_dir: Path) -> None:
    """Remove every entry in the resource directory, keeping the directory."""
    if not resources_dir.exists():
        return
    for entry in resources_dir.iterdir():
        if entry.is_dir() and not entry.is_symlink():
            shutil.rmtree(entry)
        else:
            entry.unlink()


def ensure_external_dependency(dependency: str, external: ExternalModels) -> None:
    """Verify a cached Hugging Face dependency and fetch it if absent."""
    cache_dir = str(external.cache_root() / "hub")
    try:
        snapshot_download(dependency, cache_dir=cache_dir, local_files_only=True)
    except LocalEntryNotFoundError:
        print(f"provisioning Hugging Face model '{dependency}'…", flush=True)
        external.download(dependency, cache_dir=cache_dir)


def ensure_stanza_pretrains(resources_dir: Path, config: LanguageModelConfig) -> None:
    """Fail if a declared Stanza pretrain is absent after a language download.

    Stanza's ``resources.json`` omits a pretrain dependency for some packages
    (notably the Greek ``gdt_nocharlm`` lemma), so the file is not guaranteed to
    be part of the download list. The dependency that does pull it in is the
    ``pos`` package, so a plain existence check is enough; raising here keeps the
    marker unwritten and the process non-zero, like a download failure.
    """
    for package in (config.pretrain_dependencies or {}).values():
        pretrain_path = stanza_pretrain_path(resources_dir, config.language, package)
        if not pretrain_path.exists():
            raise FileNotFoundError(
                f"Stanza pretrain '{package}' for language '{config.language}' is missing "
                f"after download: {pretrain_path}"
            )


def verify_artifact_checksums(resources_dir: Path, config: LanguageModelConfig) -> None:
    """Verify every artifact declared by a language configuration.

    Declared paths are relative to the Stanza resource directory. A missing
    artifact or a digest mismatch raises, so the marker stays unwritten and the
    process exits non-zero instead of serving unverified weights.
    """
    for relative_path, expected in sorted((config.artifact_checksums or {}).items()):
        artifact = resources_dir / relative_path
        if not artifact.is_file():
            raise FileNotFoundError(
                f"Declared artifact for language '{config.language}' is missing "
                f"after download: {artifact}"
            )
        with artifact.open("rb") as handle:
            actual = hashlib.file_digest(handle, "sha256").hexdigest()
        if actual != expected:
            raise ArtifactChecksumMismatchError(
                f"Artifact checksum mismatch for language '{config.language}': "
                f"{artifact} expected sha256 {expected}, got {actual}"
            )


def provision(
    resources_dir: Path,
    languages: Iterable[str],
    *,
    stanza_version: str,
    download: Callable[..., object] | None = None,
    external: ExternalModels | None = None,
) -> ProvisionPlan:
    """Provision the resource directory for the configured languages.

    Returns the executed plan; raises whatever the download step raises so a
    failed provisioning run cannot write a marker and exits non-zero.
    """
    download = download or stanza.download
    external = external or ExternalModels()
    resources_dir = Path(resources_dir)
    plan = plan_provisioning(read_marker(resources_dir), stanza_version, languages)
    resources_dir.mkdir(parents=True, exist_ok=True)
    if plan.wipe:
        wipe_resources(resources_dir)
    for language in plan.downloads:
        print(f"provisioning Stanza models for language '{language}'…", flush=True)
        config = model_config_for_language(language)
        kwargs = {
            "model_dir": str(resources_dir),
            "processors": config.processors,
            "verbose": False,
        }
        if config.package is not None:
            kwargs["package"] = config.package
        download(language, **kwargs)
        ensure_stanza_pretrains(resources_dir, config)
        verify_artifact_checksums(resources_dir, config)
    dependencies = {
        dependency
        for language in plan.provisioned
        for dependency in model_config_for_language(language).external_model_dependencies
    }
    for dependency in sorted(dependencies):
        ensure_external_dependency(dependency, external)
    model_configs = tuple(model_config_for_language(language) for language in plan.provisioned)
    write_marker(
        resources_dir,
        Marker(
            stanza_version=stanza_version,
            languages=plan.provisioned,
            model_configs=model_configs,
        ),
    )
    return plan


def main() -> int:
    """Provision the configured languages as a one-shot init command."""
    resources_dir = Path(os.getenv("STANZA_RESOURCES_DIR", DEFAULT_MODEL_DIR))
    languages = [descriptor.language for descriptor in configured_languages()]
    try:
        provision(
            resources_dir,
            languages,
            stanza_version=stanza.__version__,
            external=ExternalModels(hf_home=configured_hf_home()),
        )
    except Exception as error:  # noqa: BLE001
        print(f"stanza provisioning failed: {error}", file=sys.stderr, flush=True)
        return 1
    print(
        f"provisioned Stanza models for {', '.join(languages)} in {resources_dir}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
