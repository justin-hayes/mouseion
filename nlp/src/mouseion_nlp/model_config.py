"""Effective Stanza model configuration for each analysis language."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

DEFAULT_PROCESSORS = "tokenize,pos,lemma,depparse"
GREEK_BERT_MODEL = "nlpaueb/bert-base-greek-uncased-v1"
GREEK_LEMMA_PRETRAIN = "conll17"
FRENCH_MODEL_VERSION = "stanza-1.15.0-fr-combined"
# sha256 of the eight pinned French runtime artifacts, fetched at the manifest
# ``stanfordnlp/stanza-resources@569db9d`` and Hugging Face
# ``stanfordnlp/stanza-fr@caeb7742`` revisions (ADR 0102). Paths are relative to
# the Stanza resource directory.
FRENCH_ARTIFACT_CHECKSUMS = {
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
Package = str | Mapping[str, str]
PretrainDependencies = Mapping[str, str]
ArtifactChecksums = Mapping[str, str]
# Distinguishes an absent marker field from an explicit JSON null.
_FIELD_ABSENT = object()


def stanza_pretrain_path(model_dir: str | Path, language: str, package: str) -> Path:
    """Return the provisioned file for a declared Stanza pretrain package."""
    return Path(model_dir) / language / "pretrain" / f"{package}.pt"


@dataclass(frozen=True)
class LanguageModelConfig:
    """The model inputs shared by provisioning and pipeline construction."""

    language: str
    processors: str = DEFAULT_PROCESSORS
    package: Package | None = None
    pretrain_dependencies: PretrainDependencies | None = None
    external_model_dependencies: tuple[str, ...] = ()
    model_version: str | None = None
    artifact_checksums: ArtifactChecksums | None = None

    def marker_payload(self) -> dict[str, Any]:
        """Return the stable, JSON-compatible marker representation."""
        package: Package | None = self.package
        if isinstance(package, Mapping):
            package = dict(sorted(package.items()))
        pretrain_dependencies: PretrainDependencies | None = self.pretrain_dependencies
        if isinstance(pretrain_dependencies, Mapping):
            pretrain_dependencies = dict(sorted(pretrain_dependencies.items()))
        artifact_checksums: ArtifactChecksums | None = self.artifact_checksums
        if isinstance(artifact_checksums, Mapping):
            artifact_checksums = dict(sorted(artifact_checksums.items()))
        payload: dict[str, Any] = {
            "external_model_dependencies": list(self.external_model_dependencies),
            "language": self.language,
            "model_version": self.model_version,
            "package": package,
            "pretrain_dependencies": pretrain_dependencies,
            "processors": self.processors,
        }
        # Declared checksums are recorded so changing them re-provisions. A
        # language without them keeps the marker shape it had before.
        if artifact_checksums is not None:
            payload["artifact_checksums"] = artifact_checksums
        return payload

    @classmethod
    def from_marker_payload(cls, payload: object) -> LanguageModelConfig | None:
        """Parse a marker config, returning ``None`` for any invalid shape.

        ``pretrain_dependencies`` must be present, so a marker written before the
        field existed is rejected and its volume re-provisions (ADR 0073).
        """
        if not isinstance(payload, dict):
            return None
        language = payload.get("language")
        processors = payload.get("processors")
        package = payload.get("package")
        pretrain_dependencies = payload.get("pretrain_dependencies", _FIELD_ABSENT)
        dependencies = payload.get("external_model_dependencies")
        model_version = payload.get("model_version")
        artifact_checksums = payload.get("artifact_checksums")
        if (
            not isinstance(language, str)
            or not language
            or not isinstance(processors, str)
            or not processors
        ):
            return None
        if not _valid_package(package):
            return None
        if (
            pretrain_dependencies is _FIELD_ABSENT
            or not _valid_pretrain_dependencies(pretrain_dependencies)
            or not isinstance(dependencies, list)
            or not all(isinstance(dependency, str) for dependency in dependencies)
            or not _valid_artifact_checksums(artifact_checksums)
        ):
            return None
        if model_version is not None and not isinstance(model_version, str):
            return None
        return cls(
            language=language,
            processors=processors,
            package=package,
            pretrain_dependencies=pretrain_dependencies,
            external_model_dependencies=tuple(dependencies),
            model_version=model_version,
            artifact_checksums=artifact_checksums,
        )


def _valid_package(package: object) -> bool:
    """Accept an absent package, a single name, or a processor-to-name mapping."""
    if package is None or isinstance(package, str):
        return True
    return isinstance(package, dict) and all(
        isinstance(key, str) and isinstance(value, str) for key, value in package.items()
    )


def _valid_pretrain_dependencies(dependencies: object) -> bool:
    """Accept an absent pretrain set or a processor-to-package mapping."""
    if dependencies is None:
        return True
    return isinstance(dependencies, dict) and all(
        isinstance(key, str) and isinstance(value, str) for key, value in dependencies.items()
    )


def _valid_artifact_checksums(checksums: object) -> bool:
    """Accept an absent checksum set or an artifact-path-to-sha256 mapping."""
    if checksums is None:
        return True
    return isinstance(checksums, dict) and all(
        isinstance(path, str) and isinstance(checksum, str) for path, checksum in checksums.items()
    )


_LANGUAGE_MODEL_CONFIGS = {
    "de": LanguageModelConfig(language="de"),
    "el": LanguageModelConfig(
        language="el",
        processors="tokenize,mwt,pos,lemma,depparse",
        package={
            "tokenize": "gdt",
            "mwt": "gdt",
            "pos": "gdt_nocharlm",
            "lemma": "gdt_nocharlm",
            "depparse": "gdt_greek-bert",
        },
        pretrain_dependencies={"lemma": GREEK_LEMMA_PRETRAIN},
        external_model_dependencies=(GREEK_BERT_MODEL,),
        model_version="stanza-1.15.0-gdt-accurate",
    ),
    "fr": LanguageModelConfig(
        language="fr",
        processors="tokenize,mwt,pos,lemma,depparse",
        package={
            "tokenize": "combined_nocharlm",
            "mwt": "combined",
            "pos": "combined_charlm",
            "lemma": "combined_nocharlm",
            "depparse": "combined_charlm",
        },
        model_version=FRENCH_MODEL_VERSION,
        artifact_checksums=FRENCH_ARTIFACT_CHECKSUMS,
    ),
    "it": LanguageModelConfig(language="it"),
}


def model_config_for_language(language: str) -> LanguageModelConfig:
    """Resolve a language, preserving the old Stanza defaults when unknown."""
    code = language.strip().lower()
    return _LANGUAGE_MODEL_CONFIGS.get(code, LanguageModelConfig(language=code))
