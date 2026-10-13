"""Effective Stanza model configuration for each analysis language."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from pathlib import Path
from typing import Any

DEFAULT_PROCESSORS = "tokenize,pos,lemma,depparse"
GREEK_BERT_MODEL = "nlpaueb/bert-base-greek-uncased-v1"
GREEK_LEMMA_PRETRAIN = "conll17"
Package = str | Mapping[str, str]
PretrainDependencies = Mapping[str, str]
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

    def marker_payload(self) -> dict[str, Any]:
        """Return the stable, JSON-compatible marker representation."""
        package: Package | None = self.package
        if isinstance(package, Mapping):
            package = dict(sorted(package.items()))
        pretrain_dependencies: PretrainDependencies | None = self.pretrain_dependencies
        if isinstance(pretrain_dependencies, Mapping):
            pretrain_dependencies = dict(sorted(pretrain_dependencies.items()))
        return {
            "external_model_dependencies": list(self.external_model_dependencies),
            "language": self.language,
            "model_version": self.model_version,
            "package": package,
            "pretrain_dependencies": pretrain_dependencies,
            "processors": self.processors,
        }

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
    "it": LanguageModelConfig(language="it"),
}


def model_config_for_language(language: str) -> LanguageModelConfig:
    """Resolve a language, preserving the old Stanza defaults when unknown."""
    code = language.strip().lower()
    return _LANGUAGE_MODEL_CONFIGS.get(code, LanguageModelConfig(language=code))
