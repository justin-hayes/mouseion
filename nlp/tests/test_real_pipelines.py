"""Opt-in tests that build the real configured pipelines fully offline.

A default ``make test`` run skips these because no models are provisioned. To
run them, point ``MOUSEION_TEST_STANZA_RESOURCES_DIR`` at a directory already
provisioned by ``python -m mouseion_nlp.provision``, and export ``HF_HOME`` to
the Hugging Face cache holding ``nlpaueb/bert-base-greek-uncased-v1`` (required
for Modern Greek's accurate parser). ``HF_HOME`` must be set in the environment
before the test process starts: the Hugging Face cache location is resolved at
import time, so it cannot be patched from inside a test.
"""

from __future__ import annotations

import os
from unittest.mock import patch

import pytest

from mouseion_nlp import Producer

STANZA_RESOURCES_ENV = "MOUSEION_TEST_STANZA_RESOURCES_DIR"

requires_provisioned_models = pytest.mark.skipif(
    not os.environ.get(STANZA_RESOURCES_ENV),
    reason=(
        f"set {STANZA_RESOURCES_ENV} to an already-provisioned Stanza resource directory "
        "and export HF_HOME to the Hugging Face cache for Greek-BERT to run"
    ),
)


@pytest.mark.parametrize(
    ("language", "sentence"),
    [
        ("de", "Der Hund schläft."),
        ("it", "Il cane dorme."),
        ("el", "Τα παιδιά διαβάζουν."),
    ],
)
@requires_provisioned_models
def test_configured_pipeline_builds_and_analyzes_offline(language: str, sentence: str) -> None:
    from mouseion_nlp import producer as producer_module

    # STANZA_RESOURCES_DIR is read when the pipeline is constructed, so the
    # opt-in path can be injected here.
    environment = {"STANZA_RESOURCES_DIR": os.environ[STANZA_RESOURCES_ENV]}
    producer_module._stanza_pipeline.cache_clear()
    try:
        with patch.dict(os.environ, environment, clear=False):
            producer = Producer()
            producer.warmup(language)
            artifact = producer.analyze(sentence, language)
    finally:
        producer_module._stanza_pipeline.cache_clear()

    assert artifact.language == language
    tokens = [token for analyzed in artifact.sentences for token in analyzed.tokens]
    assert tokens
    assert all(token.canonical_lemma for token in tokens)


# A dialogue dash the Greek tagger labels SYM, followed by the pronoun `του`,
# which Stanza's contextual lemma classifier disambiguates. Stanza 1.15.0 raised
# ``KeyError: 'SYM'`` for this sentence because the classifier's UPOS table has
# no SYM entry; 1.15.1 maps an unseen tag to the padding index.
GREEK_SYM_WITH_CLASSIFIER_PRONOUN = (
    "— Εσύ όλο μεγιστάνες και βασιλιάδες ονειρεύεσαι, είπε με παραξενιά ο Βασιλιάς, "
    "που συλλογίζουνταν τα καράβια του και τον θείο του τον Βασιλιά και το γράμμα "
    "του Λαγόκαρδου, και ήταν στις κακές του."
)


@requires_provisioned_models
def test_greek_lemma_classifier_pronoun_with_sym_tagged_dash_does_not_fail() -> None:
    from mouseion_nlp import producer as producer_module

    environment = {"STANZA_RESOURCES_DIR": os.environ[STANZA_RESOURCES_ENV]}
    producer_module._stanza_pipeline.cache_clear()
    try:
        with patch.dict(os.environ, environment, clear=False):
            producer = Producer()
            artifact = producer.analyze(GREEK_SYM_WITH_CLASSIFIER_PRONOUN, "el")
            pipeline = producer_module._stanza_pipeline("el")
            words = [
                word
                for sentence in pipeline(GREEK_SYM_WITH_CLASSIFIER_PRONOUN).sentences
                for word in sentence.words
            ]
    finally:
        producer_module._stanza_pipeline.cache_clear()

    # Guard the premise: the dash is tagged SYM and a classifier pronoun is present.
    assert any(word.upos == "SYM" for word in words)
    assert any(word.text == "του" for word in words)
    tokens = [token for analyzed in artifact.sentences for token in analyzed.tokens]
    assert tokens
    assert all(token.canonical_lemma for token in tokens)
