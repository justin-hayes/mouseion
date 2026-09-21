"""Coarse, batch-oriented Stanza NLP producer boundary."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from functools import lru_cache
import json
import logging
import os
from pathlib import Path
from typing import Any, Callable
import unicodedata
from uuid import uuid4

import stanza
from mouseion.v1 import normalized_corpus_pb2
from stanza.pipeline.core import DownloadMethod
from stanza.resources.common import DEFAULT_MODEL_DIR

from .model_config import model_config_for_language


@dataclass(frozen=True)
class SourceDocument:
    """Metadata and structural provenance for one analyzed document."""

    id: str = "document"
    source_identifier: str = ""
    title: str = ""
    chapter: str = ""
    section: str = ""


PipelineFactory = Callable[[str], Any]

GERMAN_NORMALIZATION_PROFILE = "german-standard-post-1996"
GERMAN_NORMALIZATION_VERSION = "6"
GREEK_NORMALIZATION_PROFILE = "modern-greek"
GREEK_NORMALIZATION_VERSION = "1"
DEFAULT_NORMALIZATION_PROFILE = "unicode-casefold"
DEFAULT_NORMALIZATION_VERSION = "1.2.0"

logger = logging.getLogger(__name__)

def load_german_normalization_policy() -> dict[str, dict[str, str]]:
    """Load the same reviewed policy consumed by the Go runtime and index."""
    repository_policy = Path(__file__).resolve().parents[3] / "internal" / "canonicalization" / "german_post1996.json"
    container_policy = Path("/src/internal/canonicalization/german_post1996.json")
    policy_path = repository_policy if repository_policy.exists() else container_policy
    with policy_path.open(encoding="utf-8") as source:
        return json.load(source)


GERMAN_NORMALIZATION_POLICY = load_german_normalization_policy()
GERMAN_POST_1996_EQUIVALENCES = {
    **GERMAN_NORMALIZATION_POLICY["equivalences"],
    **GERMAN_NORMALIZATION_POLICY.get("v6_equivalences", {}),
}

# Keep this closed: the dependency relation alone is not sufficient to
# distinguish separable particles from free adverbs and homographs.
GERMAN_SEPARABLE_PREFIXES = frozenset(
    {
        "ab",
        "an",
        "auf",
        "aus",
        "auseinander",
        "bei",
        "durch",
        "ein",
        "empor",
        "entgegen",
        "fest",
        "fort",
        "frei",
        "gegen",
        "her",
        "herab",
        "herauf",
        "heraus",
        "herein",
        "herum",
        "herunter",
        "hin",
        "hinab",
        "hinauf",
        "hinaus",
        "hinein",
        "hinweg",
        "hinunter",
        "hinzu",
        "hinter",
        "heim",
        "los",
        "mit",
        "nach",
        "nieder",
        "über",
        "um",
        "unter",
        "vor",
        "voran",
        "vorbei",
        "voraus",
        "vorüber",
        "weg",
        "weiter",
        "wieder",
        "zu",
        "zurück",
        "zusammen",
    }
)


@lru_cache(maxsize=None)
def _stanza_pipeline(language: str) -> Any:
    config = model_config_for_language(language)
    kwargs = {
        "lang": config.language,
        "model_dir": os.getenv("STANZA_RESOURCES_DIR", DEFAULT_MODEL_DIR),
        "processors": config.processors,
        "download_method": DownloadMethod.NONE,
        "verbose": False,
    }
    if config.package is not None:
        kwargs["package"] = config.package
    return stanza.Pipeline(**kwargs)


def _default_pipeline_factory(language: str) -> Any:
    return _stanza_pipeline(language)


def _morphology(feats: str | None) -> dict[str, str]:
    if not feats:
        return {}
    return dict(part.split("=", 1) for part in feats.split("|") if "=" in part)


def _clean_surface(surface: str) -> str:
    """Remove Unicode punctuation/symbol edges while preserving lexical internals."""
    def is_edge_decoration(character: str) -> bool:
        # Apostrophes are lexical in elided forms such as Italian L' and dell'.
        return character not in {"'", "’"} and unicodedata.category(character)[0] in {"P", "S"}

    start, end = 0, len(surface)
    while start < end and is_edge_decoration(surface[start]):
        start += 1
    while end > start and is_edge_decoration(surface[end - 1]):
        end -= 1
    # Preserve standalone punctuation tokens; only lexical surfaces are derived.
    return surface[start:end] or surface


def _primary_lemma(lemma: str) -> str:
    """Return the first usable alternative from Stanza's pipe lemma form."""
    alternatives = [alternative.strip() for alternative in lemma.split("|") if alternative.strip()]
    if len(set(alternatives)) > 1:
        logger.warning(
            "analyzer returned differing lemma alternatives; using the first",
            extra={"raw_lemma": lemma, "lemma_alternatives": alternatives},
        )
    return alternatives[0] if alternatives else ""


class Producer:
    """Run Stanza once per complete document and emit the protobuf contract."""

    def __init__(
        self,
        *,
        normalization_profile: str | None = None,
        normalization_version: str | None = None,
        pipeline_factory: PipelineFactory = _default_pipeline_factory,
    ) -> None:
        self.normalization_profile = normalization_profile
        self.normalization_version = normalization_version
        self._pipeline_factory = pipeline_factory

    def warmup(self, language: str) -> None:
        """Load (and, on first run, download) the Stanza pipeline for a language.

        Delegates to the (lru-cached) pipeline factory so a subsequent analyze()
        for the same language reuses the warmed pipeline.
        """
        self._pipeline_factory(language)

    def model_version(self, language: str) -> str:
        """Return the configured model tag, or Stanza's version by default."""
        return model_config_for_language(language).model_version or stanza.__version__

    def analyze(
        self,
        text: str,
        language: str,
        document: SourceDocument | None = None,
        *,
        run_id: str | None = None,
        analyzed_at: datetime | None = None,
    ) -> normalized_corpus_pb2.NormalizedCorpus:
        """Analyze a whole document in one Stanza pipeline invocation.

        Stanza character offsets are copied as half-open Unicode code-point
        offsets. Sentence offsets use their first and last token.
        """
        source = document or SourceDocument()
        analyzed = analyzed_at or datetime.now(timezone.utc)
        stanza_document = self._pipeline_factory(language)(text)
        sentences = [
            self._map_sentence(sentence, source, language)
            for sentence in stanza_document.sentences
        ]
        profile_name, profile_version = self._normalization_profile(language)

        return normalized_corpus_pb2.NormalizedCorpus(
            schema_version="1.1.0",
            language=language,
            sentences=sentences,
            source_documents=[
                normalized_corpus_pb2.SourceDocument(
                    id=source.id, source_identifier=source.source_identifier, title=source.title
                )
            ],
            analysis=normalized_corpus_pb2.AnalysisProvenance(
                run_id=run_id or str(uuid4()),
                analyzed_at=analyzed.astimezone(timezone.utc).isoformat().replace("+00:00", "Z"),
                analyzer_name="stanza",
                analyzer_version=stanza.__version__,
            ),
            normalization_profile=normalized_corpus_pb2.NormalizationProfile(
                name=profile_name, version=profile_version
            ),
        )

    def _map_sentence(self, sentence: Any, source: SourceDocument, language: str) -> Any:
        words = []
        for token in sentence.tokens:
            for word in token.words:
                start = getattr(word, "start_char", None)
                end = getattr(word, "end_char", None)
                if start is None:
                    start = getattr(token, "start_char", 0)
                if end is None:
                    end = getattr(token, "end_char", start + len(word.text))
                surface = _clean_surface(word.text)
                lemma = word.lemma or word.text
                primary_lemma = _primary_lemma(lemma)
                if not primary_lemma:
                    logger.warning(
                        "analyzer token has no usable lemma alternative; rejecting token",
                        extra={"raw_lemma": lemma},
                    )
                    continue
                words.append((word, token, start, end, surface, lemma, primary_lemma))

        word_ordinals = {
            getattr(word, "id", None) or ordinal + 1: ordinal
            for ordinal, (word, *_rest) in enumerate(words)
        }
        tokens = []
        for ordinal, (word, _token, start, end, surface, lemma, primary_lemma) in enumerate(words):
            head_id = getattr(word, "head", 0)
            if head_id == 0:
                dependency = "root"
                head = ordinal
            else:
                dependency = getattr(word, "deprel", "")
                head = word_ordinals.get(head_id)
            if not dependency or head is None:
                raise ValueError(
                    f"dependency parse did not resolve token {ordinal} in sentence {sentence.text!r}"
                )
            value = normalized_corpus_pb2.Token(
                surface=surface,
                raw_lemma=lemma,
                canonical_lemma=self._canonical_lemma(language, _clean_surface(primary_lemma)),
                pos=word.upos or "",
                morphology=_morphology(word.feats),
                location=self._location(source, start, end),
                dependency=dependency,
                head=head,
            )
            tokens.append(value)

        if self._is_german(language):
            self._reattach_separable_verbs(tokens)

        start = tokens[0].location.start_offset if tokens else 0
        end = tokens[-1].location.end_offset if tokens else start
        return normalized_corpus_pb2.Sentence(
            text=sentence.text,
            tokens=tokens,
            location=self._location(source, start, end),
        )

    def _normalization_profile(self, language: str) -> tuple[str, str]:
        if self.normalization_profile is not None:
            return self.normalization_profile, self.normalization_version or ""
        if self._is_german(language):
            return GERMAN_NORMALIZATION_PROFILE, GERMAN_NORMALIZATION_VERSION
        if self._is_greek(language):
            return GREEK_NORMALIZATION_PROFILE, GREEK_NORMALIZATION_VERSION
        return DEFAULT_NORMALIZATION_PROFILE, DEFAULT_NORMALIZATION_VERSION

    @classmethod
    def _reattach_separable_verbs(cls, tokens: list[Any]) -> None:
        for verb_ordinal, verb in enumerate(tokens):
            if verb.pos != "VERB":
                continue
            particles = [
                token
                for token in tokens
                if token.head == verb_ordinal
                and token.dependency == "compound:prt"
                and token.surface.casefold() in GERMAN_SEPARABLE_PREFIXES
            ]
            if particles:
                verb.canonical_lemma = cls._canonical_lemma(
                    "de", "".join(token.canonical_lemma for token in particles) + verb.canonical_lemma
                )

    @staticmethod
    def _is_german(language: str) -> bool:
        return language.lower().replace("_", "-").split("-", 1)[0] == "de"

    @staticmethod
    def _is_greek(language: str) -> bool:
        return language.lower().replace("_", "-").split("-", 1)[0] == "el"

    @staticmethod
    def _canonical_lemma(language: str, lemma: str) -> str:
        if not Producer._is_german(language):
            return " ".join(unicodedata.normalize("NFC", lemma).casefold().split())
        lowered = lemma.lower()
        return GERMAN_POST_1996_EQUIVALENCES.get(lowered, lowered)

    @staticmethod
    def _location(source: SourceDocument, start: int, end: int) -> Any:
        return normalized_corpus_pb2.SourceLocation(
            source_document_id=source.id,
            chapter=source.chapter,
            section=source.section,
            start_offset=start,
            end_offset=end,
        )
