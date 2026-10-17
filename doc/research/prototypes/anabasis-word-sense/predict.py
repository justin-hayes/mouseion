"""THROWAWAY fixed baselines. Reads evidence, never human sense labels.

No embeddings, external provider, bilingual seed list or trained aligner.
"""

import hashlib
import json
import re
from pathlib import Path

HERE = Path(__file__).resolve().parent
STOP = {
    "a",
    "an",
    "the",
    "to",
    "of",
    "in",
    "on",
    "at",
    "by",
    "for",
    "with",
    "from",
    "and",
    "or",
    "as",
    "be",
    "is",
    "was",
    "were",
    "being",
    "been",
    "have",
    "has",
    "had",
    "it",
    "its",
    "them",
    "their",
    "him",
    "his",
    "one",
    "that",
    "this",
    "which",
    "who",
}
POS = {"noun": "NOUN", "verb": "VERB", "adj": "ADJ", "adv": "ADV", "name": "PROPN"}


def words(text):
    """Generic English overlap, not semantic equivalence. No hand-made synonyms."""
    out = set()
    for word in re.findall(r"[a-z]+", text.lower()):
        if word in STOP:
            continue
        if len(word) > 5 and word.endswith("ies"):
            word = word[:-3] + "y"
        elif len(word) > 5 and word.endswith("ing"):
            word = word[:-3]
        elif len(word) > 4 and word.endswith("ed"):
            word = word[:-2]
        elif len(word) > 3 and word.endswith("s") and not word.endswith("ss"):
            word = word[:-1]
        out.add(word)
    return out


def feature_rank(target, sense):
    """Only explicit source labels; no inference from placeholder dependencies.

    Rank tuple: matching number/voice labels, then usage-label penalty, then source
    order. Native morphology positions follow oga-agdt-feats/1. A missing label is
    unknown, not a mismatch. Incompatible explicit labels exclude the candidate.
    """
    # LSJ's own prose can mix paradigms, comparative examples and multiple voices
    # within one sense. Treating any <gram> mention as an exclusive restriction
    # would discard valid senses. Retain it for human review, not a guessed gate.
    labels = {tag.lower() for tag in sense.get("tags", [])}
    postag = target["native_postag"]
    matching = 0
    constraints = [
        ({"in-plural", "plural"}, postag[2] == "p"),
        ({"in-singular", "singular"}, postag[2] == "s"),
        ({"passive", "pass."}, postag[5] == "p"),
        ({"middle", "mid."}, postag[5] in {"m", "e"}),
        ({"active", "act."}, postag[5] == "a"),
    ]
    for markers, agrees in constraints:
        if labels & markers:
            if not agrees:
                return None
            matching += 1
    if "participle" in labels:
        if postag[4] != "p":
            return None
        matching += 1
    tense_labels = {
        "present": "p",
        "imperfect": "i",
        "aorist": "a",
        "perfect": "r",
        "pluperfect": "l",
        "future": "f",
    }
    specified_tenses = {
        value for label, value in tense_labels.items() if label in labels
    }
    if specified_tenses:
        if postag[3] not in specified_tenses:
            return None
        matching += 1
    for label, degree in (("comparative", "c"), ("superlative", "s")):
        if label in labels:
            if postag[8] != degree:
                return None
            matching += 1
    penalty = len(labels & {"poetic", "homeric", "epic", "ionic", "doric", "aeolic"})
    return matching, -penalty


def candidates(data, target, lexicon):
    entries = [e for e in data[lexicon] if e["key"] == target["canonical_lemma"]]
    if lexicon == "kaikki":
        entries = [e for e in entries if POS.get(e.get("pos")) == target["import_upos"]]
    # Never use equal OGA/dictionary digits. Multiple unresolved source entries
    # cannot be merged into one sense inventory by any automatic strategy.
    if target["lexical_discriminator"]:
        return [], "OGA discriminator has no reviewed dictionary crosswalk"
    if len(entries) != 1:
        return (
            [],
            "no compatible entry"
            if not entries
            else "multiple unresolved source entries",
        )
    entry = entries[0]
    out = []
    for sense in entry["senses"]:
        if not sense.get("glosses"):
            continue
        rank = feature_rank(target, sense)
        if rank is None:
            continue
        # Each Kaikki candidate is an exact prefix node; the final gloss is that
        # node's cue. A matching parent never silently selects a narrower child.
        gloss = (
            sense["glosses"][-1] if lexicon == "kaikki" else "; ".join(sense["glosses"])
        )
        out.append(
            {
                "id": sense["id"],
                "entry_id": entry["id"],
                "gloss": gloss,
                "morphology_label_rank": list(rank),
                "ordinal": sense["ordinal"],
                "overlap": sorted(
                    words(" ".join(target["aligned_english"])) & words(gloss)
                ),
                "pos_evidence": entry.get("pos")
                if lexicon == "kaikki"
                else entry.get("entry_pos", []),
            }
        )
    return (
        out,
        "one source entry; LSJ POS may be unknown"
        if lexicon == "lsj"
        else "one explicit POS-compatible source entry",
    )


def choose(available, strategy):
    if strategy == "no-contextual-gloss":
        return None, "abstain; dictionary evidence is not a contextual Gloss"
    if not available:
        return None, "no identity-safe, gloss-bearing, morphology-compatible candidates"
    prior = sorted(
        available,
        key=lambda c: (
            -c["morphology_label_rank"][0],
            -c["morphology_label_rank"][1],
            c["ordinal"],
        ),
    )
    if strategy == "morphology-order":
        return prior[0], "explicit morphology/usage labels, then dictionary order"
    supported = [c for c in prior if c["overlap"]]
    if not supported:
        return None, "no aligned-English/leaf-gloss content-word overlap"
    if strategy == "unique-projection" and len(supported) != 1:
        return None, "more than one sense has lexical overlap; not uniquely supported"
    ranked = sorted(supported, key=lambda c: -len(c["overlap"]))
    return (
        ranked[0],
        "overlap count; morphology/order breaks ties"
        if strategy == "projection"
        else "exactly one sense has positive lexical overlap",
    )


def main():
    raw = (HERE / "inputs.json").read_bytes()
    data = json.loads(raw)
    outputs = []
    for target in data["targets"]:
        for lexicon in ("kaikki", "lsj"):
            available, identity = candidates(data, target, lexicon)
            for strategy in (
                "morphology-order",
                "projection",
                "unique-projection",
                "no-contextual-gloss",
            ):
                selected, reason = choose(available, strategy)
                outputs.append(
                    {
                        "target_id": target["id"],
                        "lexicon": lexicon,
                        "strategy": strategy,
                        "identity_status": identity,
                        "chosen": selected,
                        "reason": reason,
                        "candidates": available,
                    }
                )
    asset = {
        "schema": "anabasis-word-sense-predictions/1",
        "inputs_sha256": hashlib.sha256(raw).hexdigest(),
        "predictor_sha256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
        "alignment_assumption": "human-confirmed target links supplied as oracle input; not an automatic alignment result",
        "evaluation_status": "unscored; dictionary sense labels require a separate live human checkpoint",
        "predictions": outputs,
    }
    (HERE / "predictions.json").write_text(
        json.dumps(asset, ensure_ascii=False, indent=2) + "\n"
    )
    print(
        "Predictions frozen without reading any human sense labels; no accuracy claim."
    )


if __name__ == "__main__":
    main()
