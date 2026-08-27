"""Coarse, batch-oriented Stanza NLP producer boundary."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime, timezone
from functools import lru_cache
from typing import Any, Callable
from uuid import uuid4

import stanza
from mouseion.v1 import normalized_corpus_pb2


@dataclass(frozen=True)
class SourceDocument:
    """Metadata and structural provenance for one analyzed document."""

    id: str = "document"
    source_identifier: str = ""
    title: str = ""
    chapter: str = ""
    section: str = ""


PipelineFactory = Callable[[str, bool], Any]

GERMAN_NORMALIZATION_PROFILE = "german-standard-post-1996"
GERMAN_NORMALIZATION_VERSION = "2"
DEFAULT_NORMALIZATION_PROFILE = "unicode-casefold"
DEFAULT_NORMALIZATION_VERSION = "1.0.0"

# Exact lexical rules avoid collapsing modern, distinct lemmas such as Maße
# and Masse. Keep this table in sync with internal/canonicalization/german.go.
GERMAN_POST_1996_EQUIVALENCES = {
    "daß": "dass",
    "muß": "muss",
    "mußt": "musst",
    "müßt": "müsst",
    "fluß": "fluss",
    "kuß": "kuss",
    "nuß": "nuss",
    "naß": "nass",
    "schluß": "schluss",
    "schloß": "schloss",
    "thür": "tür",
    "thüre": "türe",
}


@lru_cache(maxsize=None)
def _stanza_pipeline(language: str, enable_ner: bool) -> Any:
    processors = "tokenize,pos,lemma,ner" if enable_ner else "tokenize,pos,lemma"
    return stanza.Pipeline(lang=language, processors=processors, verbose=False)


def _default_pipeline_factory(language: str, enable_ner: bool) -> Any:
    return _stanza_pipeline(language, enable_ner)


def _morphology(feats: str | None) -> dict[str, str]:
    if not feats:
        return {}
    return dict(part.split("=", 1) for part in feats.split("|") if "=" in part)


# Quotation and bracketing marks that Stanza may attach to a lexical token.
# Apostrophes and ordinary sentence punctuation are intentionally excluded.
_EDGE_QUOTES = "\"‘’‚‛“”„‟‹›«»「」『』《》〈〉【】〔〕〖〗〘〙〚〛()[]{}"


def _clean_surface(surface: str) -> str:
    """Remove attached quotation/bracketing marks without altering punctuation."""
    return surface.strip(_EDGE_QUOTES)


def _primary_lemma(lemma: str) -> str:
    """Return the first usable alternative from Stanza's pipe lemma form."""
    return next((alternative.strip() for alternative in lemma.split("|") if alternative.strip()), "")


class Producer:
    """Run Stanza once per complete document and emit the protobuf contract."""

    def __init__(
        self,
        *,
        enable_ner: bool = False,
        normalization_profile: str | None = None,
        normalization_version: str | None = None,
        pipeline_factory: PipelineFactory = _default_pipeline_factory,
    ) -> None:
        self.enable_ner = enable_ner
        self.normalization_profile = normalization_profile
        self.normalization_version = normalization_version
        self._pipeline_factory = pipeline_factory

    def warmup(self, language: str) -> None:
        """Load (and, on first run, download) the Stanza pipeline for a language.

        Delegates to the (lru-cached) pipeline factory so a subsequent analyze()
        for the same language reuses the warmed pipeline.
        """
        self._pipeline_factory(language, self.enable_ner)

    def model_version(self, language: str) -> str:  # noqa: ARG002
        """Return the Stanza model release used by the configured pipeline."""
        return stanza.__version__

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
        stanza_document = self._pipeline_factory(language, self.enable_ner)(text)
        sentences = [
            self._map_sentence(sentence, source, language)
            for sentence in stanza_document.sentences
        ]
        profile_name, profile_version = self._normalization_profile(language)

        return normalized_corpus_pb2.NormalizedCorpus(
            schema_version="1.0.0",
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
        entities = getattr(sentence, "ents", ()) if self.enable_ner else ()
        tokens = []
        for token in sentence.tokens:
            for word in token.words:
                start = getattr(word, "start_char", None)
                end = getattr(word, "end_char", None)
                if start is None:
                    start = getattr(token, "start_char", 0)
                if end is None:
                    end = getattr(token, "end_char", start + len(word.text))
                ner = getattr(token, "ner", None) if self.enable_ner else None
                if not ner:
                    ner = next(
                        (
                            entity.type
                            for entity in entities
                            if entity.start_char <= start and end <= entity.end_char
                        ),
                        None,
                    )
                surface = _clean_surface(word.text)
                lemma = word.lemma or word.text
                primary_lemma = _primary_lemma(lemma)
                value = normalized_corpus_pb2.Token(
                    surface=surface,
                    raw_lemma=lemma,
                    canonical_lemma=self._canonical_lemma(language, primary_lemma),
                    pos=word.upos or "",
                    morphology=_morphology(word.feats),
                    location=self._location(source, start, end),
                )
                if ner and ner != "O":
                    value.named_entity = ner
                tokens.append(value)

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
        if language.lower().replace("_", "-").split("-", 1)[0] == "de":
            return GERMAN_NORMALIZATION_PROFILE, GERMAN_NORMALIZATION_VERSION
        return DEFAULT_NORMALIZATION_PROFILE, DEFAULT_NORMALIZATION_VERSION

    @staticmethod
    def _canonical_lemma(language: str, lemma: str) -> str:
        if language.lower().replace("_", "-").split("-", 1)[0] != "de":
            return lemma.casefold()
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
