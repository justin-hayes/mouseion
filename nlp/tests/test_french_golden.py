"""Opt-in pinned-model golden check for French, the release gate for `fr`.

This is not part of default CI. It runs the committed 42-string French
regression corpus (``testdata/french_golden_corpus.json``) through the real,
pinned Stanza 1.15.0 combined pipeline and asserts the expected sentence
boundaries, tokens, lemmas and target anchors, including the 17 targets only the
persisted-token anchor reaches. The corpus holds the reviewed observations from
the #2152 probe plus the ADR 0102 dialogue repairs.

Run it after provisioning the pinned models into a persistent directory:

```sh
STANZA_RESOURCES_DIR="$PWD/.stanza_resources" HF_HOME="$PWD/.huggingface" \\
  MOUSEION_NLP_WARM_LANGUAGES=fr .venv/bin/python -m mouseion_nlp.provision
MOUSEION_TEST_STANZA_RESOURCES_DIR="$PWD/.stanza_resources" \\
  PYTHONPATH=nlp/src:gen/python .venv/bin/pytest -q nlp/tests/test_french_golden.py
```

``make french-golden`` wraps the test run. ``HF_HOME`` must be exported before
the test process starts, because the Hugging Face cache location is resolved at
import time.
"""

from __future__ import annotations

import json
import os
import unicodedata
from pathlib import Path
from unittest.mock import patch

import pytest
import stanza

from mouseion_nlp import Producer
from mouseion_nlp.model_config import model_config_for_language

STANZA_RESOURCES_ENV = "MOUSEION_TEST_STANZA_RESOURCES_DIR"
CORPUS = json.loads(
    (Path(__file__).parent / "testdata" / "french_golden_corpus.json").read_text(encoding="utf-8")
)
EXPECTED_STRINGS = 42
EXPECTED_TOKENS = 318
EXPECTED_TARGETS = 108
EXPECTED_OFFSET_ANCHORED = 17

requires_provisioned_models = pytest.mark.skipif(
    not os.environ.get(STANZA_RESOURCES_ENV),
    reason=(
        f"set {STANZA_RESOURCES_ENV} to a directory provisioned with the pinned French "
        "models (see the module docstring) to run the release gate"
    ),
)


def _clean_lexical_surface(surface: str) -> str:
    """Mirror ``textmatch.CleanLexicalSurface`` for the persisted-token anchor.

    The Go anchor compares a target against each persisted token's cleaned
    surface; the same cleaning must decide the anchor here, so the check
    verifies the offsets the product actually consumes.
    """
    surface = surface.strip()
    if not surface:
        return ""
    runes = list(surface)
    start, end = 0, len(runes)

    def is_edge_decoration(character: str) -> bool:
        return character not in {"'", "\u2019"} and unicodedata.category(character)[0] in {
            "P",
            "S",
        }

    while start < end and is_edge_decoration(runes[start]):
        start += 1
    while end > start and is_edge_decoration(runes[end - 1]):
        end -= 1
    if start == end:
        return surface
    return "".join(runes[start:end])


def _first_token_anchor(sentence: str, target: str, tokens: list) -> list | None:
    """Return the span of the first persisted token the product anchors ``target`` on.

    Mirrors ``cardexport.tokenAnchorSpan``: the first token whose cleaned
    surface case-folds to the target, accepted only when the sentence text at
    the token's own span cleans the same way.
    """
    cleaned = _clean_lexical_surface(target)
    for token in tokens:
        if _clean_lexical_surface(token.surface).casefold() != cleaned.casefold():
            continue
        start, end = token.location.start_offset, token.location.end_offset
        if _clean_lexical_surface(sentence[start:end]).casefold() != cleaned.casefold():
            return None
        return [start, end]
    return None


def test_french_golden_corpus_is_self_consistent() -> None:
    """The committed corpus stays complete and its anchors slice to their forms.

    This runs in default CI, without models: it guards the corpus itself rather
    than the pinned-model analysis, which the opt-in tests below assert.
    """
    assert len(CORPUS["strings"]) == EXPECTED_STRINGS
    total_tokens = 0
    total_targets = 0
    offset_anchored = 0
    for entry in CORPUS["strings"]:
        assert entry["sentences"], entry["id"]
        for sentence in entry["sentences"]:
            assert sentence["text"] == entry["text"], entry["id"]
            for token in sentence["tokens"]:
                total_tokens += 1
                start, end = token["span"]
                assert 0 <= start < end <= len(sentence["text"]), (entry["id"], token)
            for target in sentence["targets"]:
                total_targets += 1
                if not target["word_bounded"]:
                    offset_anchored += 1
                start, end = target["span"]
                observed = _clean_lexical_surface(sentence["text"][start:end]).casefold()
                expected = _clean_lexical_surface(target["form"]).casefold()
                assert observed == expected, (entry["id"], target)
    assert total_tokens == EXPECTED_TOKENS
    assert total_targets == EXPECTED_TARGETS
    assert offset_anchored == EXPECTED_OFFSET_ANCHORED


@pytest.fixture(scope="module")
def french_producer() -> Producer:
    """Build the real French pipeline from the provisioned pinned models."""
    from mouseion_nlp import producer as producer_module

    producer_module._stanza_pipeline.cache_clear()
    environment = {"STANZA_RESOURCES_DIR": os.environ[STANZA_RESOURCES_ENV]}
    try:
        with patch.dict(os.environ, environment, clear=False):
            producer = Producer()
            producer.warmup("fr")
            yield producer
    finally:
        producer_module._stanza_pipeline.cache_clear()


@requires_provisioned_models
def test_french_golden_models_are_the_pinned_ones(french_producer: Producer) -> None:
    config = model_config_for_language("fr")
    assert stanza.__version__ == CORPUS["model"]["stanza_version"]
    assert french_producer.model_version("fr") == CORPUS["model"]["model_version"]
    assert config.processors == CORPUS["model"]["processors"]
    assert config.package == CORPUS["model"]["package"]


@requires_provisioned_models
def test_french_golden_corpus_matches_the_pinned_models(french_producer: Producer) -> None:
    anchored = 0
    offset_anchored = 0
    for entry in CORPUS["strings"]:
        artifact = french_producer.analyze(entry["text"], "fr")
        assert artifact.normalization_profile.name == "french", entry["id"]
        sentences = list(artifact.sentences)
        assert [sentence.text for sentence in sentences] == [
            expected["text"] for expected in entry["sentences"]
        ], entry["id"]
        for expected, sentence in zip(entry["sentences"], sentences, strict=True):
            assert sentence.text == expected["text"], (entry["id"], expected["text"], sentence.text)
            tokens = list(sentence.tokens)
            assert len(tokens) == len(expected["tokens"]), (entry["id"], expected["tokens"])
            for expected_token, token in zip(expected["tokens"], tokens, strict=True):
                observed = {
                    "surface": token.surface,
                    "raw_lemma": token.raw_lemma,
                    "canonical_lemma": token.canonical_lemma,
                    "pos": token.pos,
                    "span": [token.location.start_offset, token.location.end_offset],
                }
                assert observed == expected_token, (entry["id"], expected_token, observed)
            for target in expected["targets"]:
                anchor = _first_token_anchor(sentence.text, target["form"], list(sentence.tokens))
                assert anchor == target["span"], (entry["id"], target, anchor)
                anchored += 1
                if not target["word_bounded"]:
                    offset_anchored += 1
    assert anchored == EXPECTED_TARGETS
    assert offset_anchored == EXPECTED_OFFSET_ANCHORED
