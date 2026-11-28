"""Regression for Stanza's contextual lemma classifier and unseen UPOS tags.

Stanza 1.15.0 raised ``KeyError: 'SYM'`` from ``convert_tags`` when the tagger
assigned a UPOS the classifier's training data never had (the Greek classifier
has no SYM or INTJ), failing a whole document. Stanza 1.15.1 maps such a tag to
the padding index. The deployed models are unchanged, so this guards the pinned
Stanza code, with the tagger output stubbed so no models are needed.
"""

from __future__ import annotations

from stanza.models.lemma_classifier import utils as classifier_utils
from stanza.models.lemma_classifier.lstm_model import LemmaClassifierLSTM


def test_unseen_upos_in_a_classifier_sentence_maps_to_padding_not_keyerror() -> None:
    # A classifier UPOS table without SYM or INTJ, like the Greek one.
    upos_to_id = {"<PAD>": 0, "NOUN": 1, "PRON": 2, "VERB": 3, "PUNCT": 4}
    classifier = LemmaClassifierLSTM.__new__(LemmaClassifierLSTM)
    classifier.upos_to_id = upos_to_id
    classifier.unknown_upos_id = classifier_utils.unknown_upos_id(upos_to_id)

    # `— ... του`: the dash tagged SYM co-occurs with a classifier target pronoun.
    ids = classifier.convert_tags([["SYM", "NOUN", "PRON"], ["INTJ", "VERB"]])

    assert ids == [[0, 1, 2], [0, 3]]
    assert classifier.unknown_upos_id == classifier_utils.PAD_ID
