"""Effective Stanza model configuration for each analysis language."""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any


DEFAULT_PROCESSORS = "tokenize,pos,lemma,depparse"
GREEK_BERT_MODEL = "nlpaueb/bert-base-greek-uncased-v1"
Package = str | Mapping[str, str]


@dataclass(frozen=True)
class LanguageModelConfig:
    """The model inputs shared by provisioning and pipeline construction."""

    language: str
    processors: str = DEFAULT_PROCESSORS
    package: Package | None = None
    external_model_dependencies: tuple[str, ...] = ()
    model_version: str | None = None

    def marker_payload(self) -> dict[str, Any]:
        """Return the stable, JSON-compatible marker representation."""
        package: str | dict[str, str] | None = self.package
        if isinstance(package, Mapping):
            package = dict(sorted(package.items()))
        return {
            "external_model_dependencies": list(self.external_model_dependencies),
            "language": self.language,
            "model_version": self.model_version,
            "package": package,
            "processors": self.processors,
        }

    @classmethod
    def from_marker_payload(cls, payload: object) -> LanguageModelConfig | None:
        """Parse a marker config, returning ``None`` for any invalid shape."""
        if not isinstance(payload, dict):
            return None
        language = payload.get("language")
        processors = payload.get("processors")
        package = payload.get("package")
        dependencies = payload.get("external_model_dependencies")
        model_version = payload.get("model_version")
        if not isinstance(language, str) or not language:
            return None
        if not isinstance(processors, str) or not processors:
            return None
        if package is not None and not isinstance(package, (str, dict)):
            return None
        if isinstance(package, dict) and not all(
            isinstance(key, str) and isinstance(value, str)
            for key, value in package.items()
        ):
            return None
        if not isinstance(dependencies, list) or not all(
            isinstance(dependency, str) for dependency in dependencies
        ):
            return None
        if model_version is not None and not isinstance(model_version, str):
            return None
        return cls(
            language=language,
            processors=processors,
            package=package,
            external_model_dependencies=tuple(dependencies),
            model_version=model_version,
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
        external_model_dependencies=(GREEK_BERT_MODEL,),
        model_version="stanza-1.14.0-gdt-accurate",
    ),
    "it": LanguageModelConfig(language="it"),
}


def model_config_for_language(language: str) -> LanguageModelConfig:
    """Resolve a language, preserving the old Stanza defaults when unknown."""
    code = language.strip().lower()
    return _LANGUAGE_MODEL_CONFIGS.get(code, LanguageModelConfig(language=code))
