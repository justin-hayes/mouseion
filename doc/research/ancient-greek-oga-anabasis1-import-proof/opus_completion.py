#!/usr/bin/env python3
"""Fresh-owner completion pass for OGA Anabasis Book 1, issue #2052 (v2 review revision).

Research-only. It hash-checks every retained input, fails closed on the native
join, reading-coverage, sentence-coverage and group-nesting gates, interprets the producer's
lemma labels and native POS for all 10,462 printed nodes under versioned rules,
segments complete sentences over the verified reading text, models Mouseion's
3/15 selection with empty Known/Reserved, runs the real Go selection code on the
same analysis, chooses deterministic sentence inputs and compares everything
with the committed summary and private outputs.

    python3 -I opus_completion.py           # verify and compare; writes nothing tracked or private
    python3 -I opus_completion.py --no-go   # same, without the Go probe (exit 1: parity unverified)
    python3 -I opus_completion.py --write   # regenerate the v2 summary and private/opus-completion/out-v2/

The v1 result (opus-completion-summary.json, private/opus-completion/out/) is the
immutable 0e839900 dry run; reproduce it from that commit. This version never
writes it.

The Go probe writes its input and output under
~/.cache/scratch/greek-oga-anabasis1-import-proof/opus-completion/ and removes
them afterwards. Earlier passes (mapping_completion*.py) are not imported for
their verdicts; only the v2 Beta-code decoder and index loader are reused.
"""

from __future__ import annotations

import argparse
import bisect
import collections
import difflib
import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unicodedata
import xml.etree.ElementTree as ET
from pathlib import Path

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:  # python3 -I does not add the script directory
    sys.path.insert(0, str(HERE))

import mapping_completion_v2 as v2  # noqa: E402

REPO = HERE.parents[2]
PRIVATE = HERE / "private"
OWN = PRIVATE / "opus-completion"
OUT_DIR = OWN / "out-v2"  # private/opus-completion/out/ keeps the 0e839900 v1 outputs
SUMMARY = HERE / "opus-completion-v2-summary.json"  # opus-completion-summary.json is v1, historical
REGISTRY = HERE / "opus-completion-inputs.json"
IMPORT_LEDGER = HERE / "import-ledger.json"
NATIVE_NODES = PRIVATE / "import" / "native-nodes.json"
READING = PRIVATE / "import" / "reading.txt"
NODE_TO_TEI = PRIVATE / "import" / "node-to-tei.json"
CONLLU = v2.CONLLU
QUERY_README = PRIVATE / "archive-members/outer/opera_graeca_adnotata_v0.2.0/query/README.md"
STEMSRC = OWN / "morpheus-stemsrc"
GORMAN = [OWN / "gorman" / "Xen_Anab_book_1.1-5.xml", OWN / "gorman" / "Xen_Anab_book_1.6-9.xml"]
UD_PERSEUS = [OWN / "udperseus" / f"grc_perseus-ud-{s}.conllu" for s in ("train", "dev", "test")]
SCRATCH = Path.home() / ".cache/scratch/greek-oga-anabasis1-import-proof/opus-completion"
PROBE = "./doc/research/ancient-greek-oga-anabasis1-import-proof/_selectionprobe"

VERSION = "oga-anabasis1-opus-completion/2"
RULESETS = {
    "identity": "oga-greta-lemma/2",
    "pos": "oga-agdt-upos/2",
    "sentence": "oga-reading-terminal/2",
    "selection_model": "mouseion-3-15-single-book-empty-known/1",
    "card_input": "grc-earliest-capped/1",
    "normalization_profile": "ancient-greek/1",
}
PINS = {
    NATIVE_NODES: "2a5aca90602428b23f0f88cbcbfbd1ac787c87d25bd9bd87c67c7a29ae0f942b",
    READING: "0f18cae963899218fda6c62726d6a0b0429763f390597c1781a93d80c2ddd907",
    NODE_TO_TEI: "1f9016a011f9f62ef87931a9e58d655c5152d0feae3e69b85f0eaa0885b7b631",
    CONLLU: "0f9cbba8e4fdf28ddcedc70259c18f3d00573bc9a82d3acc6a9a050d17542780",
    QUERY_README: "2287f7fa97f286e9b11dbfd1dd4edcba585d40960b33fc4c5b572e5e565eac87",
    v2.INDEX: "57f81c8627c654411f185f2c2d491a958c8acdd5684400fe3c1696ebb2b9d5c2",
    v2.MANIFEST: "bb672f881d8bdb2c0db4783605b65aeaa4bc7fcb562c2ecf34fa764cf8b4568f",
}
LANGUAGE = "grc"
SOURCE_DOCUMENT_ID = "oga-v0.2.0:tlg0032.tlg006.perseus-grc2:book1:reading"
DISCRIMINATOR_SCHEME = "morpheus"  # Perseus Morpheus lemma key homograph number
ALLOWED_POS = frozenset({"NOUN", "VERB", "ADJ", "ADV"})  # selection.DefaultConfig
MIN_IN_BOOK, MIN_ACROSS_BOOKS = 3, 15  # selection.DefaultRecurringMinOccurrences / DefaultAcrossBooks
MAX_UNITS, MAX_CODE_POINTS = 50, 400  # #2034 provisional caps
TERMINALS = frozenset({".", ";"})  # Perseus ASCII ';' is the Greek question mark
MEDIAL = frozenset({"·"})  # ano teleia is medial (#2034)
CLOSERS = frozenset({")", "]", "»", "’", "”"})
UNSUPPORTED_TERMINALS = frozenset({"!", "?", "\u037e", "\u0387"})
EIMI = "εἰμί"
# Producer-lineage conversion: AGDT letters the UD Perseus release maps to one UPOS.
EXPECTED_LETTER_UPOS = {"a": "ADJ", "d": "ADV", "i": "INTJ", "l": "DET", "n": "NOUN",
                        "p": "PRON", "r": "ADP", "u": "PUNCT", "g": "PART", "m": "NUM"}
CONTEXT_LETTERS = {"c": {"CCONJ", "SCONJ"}, "v": {"VERB", "AUX"}}
# UD grc DET/PART documentation examples, for the not-adopted definitional sensitivity only.
UD_GRC_DET_EXAMPLES = ("ὁ", "ἐκεῖνος", "οὗτος", "ὅδε", "αὐτός", "τοσόσδε", "ἐμός", "σός",
                       "πότερος", "ὅς", "ἄλλος", "πᾶς")
UD_GRC_PART_EXAMPLES = ("ἀτάρ", "αὖ", "δή")
# Reviewer diagnostic (not used for identity): stem of the Book 1 surfaces of the
# numbered εἰμί/εἶμι compound labels. εἶμι 'go' shows ἰ-/ᾐ-/ῄ- (ἰών, ἰόντ-, ἴασι,
# ἴμεν, ᾔει, ᾖσαν); εἰμί 'be' shows ἦν/ἦσαν, ὤν/ὄντ-/οὖσ-, εἶναι (Smyth 768-773).
COMPOUND_SURFACE_BASE = {
    "εἶμι": ("προσιὼν", "προσιόντος", "προσῇσαν", "προσιόντων", "ἐπῇσαν", "ἐπῄει",
             "ἐπιοῦσαν", "ἐπίασιν", "ἐπιόντας", "προϊόντων", "προϊόντες", "προῄει",
             "ἄπιμεν", "ἀπῄει", "ἀπιὼν", "ἀπιόντες", "ἀπιόντας", "συνῄει", "παρῄει"),
    "εἰμί": ("παρῆν", "παρῆσαν", "παροῦσι", "παρόντες", "παροῦσιν", "περιῆν", "περιεῖναι"),
}
# Reviewer reading (diagnostic only, never used by the rule) of the εἰμί contexts
# where the native-syntax rule output differs from, or is not settled by, a reading
# of the sentence. All other εἰμί contexts were read and agree with the rule.
EIMI_REVIEWER_READINGS = {
    "t_2739": ("AUX", "copular 'it is (not) time': predicate ὥρα not attached to εἶναι"),
    "t_5651": ("AUX", "copular: predicate adjective χρήσιμον not attached"),
    "t_7640": ("AUX", "copular: clausal predicate introduced by ὡς is attached as AuxC"),
    "t_9711": ("AUX", "copular quantification: numeral τέτταρες not attached"),
    "t_10265": ("AUX", "copular: interrogative predicate τί is labelled SBJ"),
    "t_10412": ("AUX", "copular quantification: numeral τετρακόσιαι not attached"),
    "t_5989": ("VERB", "pure existential; the μετὰ ταῦτα PP is a temporal adjunct"),
    "t_3351": ("AUX|VERB", "dative possessor with ὑποψία ἦν: possession or existential"),
    "t_6641": ("AUX|VERB", "ἔστι ἡμῖν ἀρχή with extent PPs: possession or existential"),
}
ACUTE, GRAVE, CIRCUMFLEX = "\u0301", "\u0300", "\u0342"
SMOOTH, ROUGH = "\u0313", "\u0314"


class InputError(SystemExit):
    """A pinned input is missing or differs from its recorded hash."""


# --- inputs -------------------------------------------------------------------


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with Path(path).open("rb") as handle:
        for block in iter(lambda: handle.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def check_pin(path: Path, expected: str) -> None:
    if not Path(path).is_file():
        raise InputError(f"input missing: {path}")
    actual = sha256_file(path)
    if actual != expected:
        raise InputError(f"pin mismatch for {Path(path).name}: got {actual}, expected {expected}")


def verify_inputs() -> dict:
    """Every original pin, every new registry entry and the Morpheus manifest agree."""
    for path, digest in PINS.items():
        check_pin(path, digest)
    registry = json.loads(REGISTRY.read_text(encoding="utf-8"))
    for entry in registry["inputs"]:
        check_pin(HERE / entry["path"], entry["sha256"])
    manifest = json.loads(v2.MANIFEST.read_text(encoding="utf-8"))
    for path in sorted(STEMSRC.iterdir()):
        if manifest["stemsrc_files"].get(path.name) != sha256_file(path):
            raise InputError(f"stem file {path.name} differs from the Morpheus manifest")
    producers = v2.verify_producer_docs()
    return {"original_pins": {p.name: d for p, d in sorted(PINS.items(), key=lambda kv: kv[0].name)},
            "new_inputs": len(registry["inputs"]), "producer_documents": sorted(producers),
            "morpheus_pin": v2.MORPHEUS_PIN}


# --- normalization and citation forms -------------------------------------------


def profile_key(text: str) -> str:
    """Mouseion ancient-greek/1: NFC, Unicode lowercase, final sigma to sigma, NFC."""
    lowered = unicodedata.normalize("NFC", text).lower().replace("ς", "σ")
    return unicodedata.normalize("NFC", lowered)


def split_label(raw: str) -> tuple[str, str]:
    match = re.fullmatch(r"(.+?)(\d*)", raw)
    if match is None:
        raise ValueError(f"empty lemma label {raw!r}")
    return unicodedata.normalize("NFC", match.group(1)), match.group(2)


def citation_form_flags(text: str) -> list[str]:
    """Orthographic invariants of a Greek citation form; flags never repair spelling."""
    flags: set[str] = set()
    letters: list[tuple[str, list[str]]] = []
    for char in unicodedata.normalize("NFD", text):
        if unicodedata.combining(char):
            if not letters:
                flags.add("stray-combining-mark")
                continue
            letters[-1][1].append(char)
        else:
            letters.append((char, []))
    marks = [m for _, ms in letters for m in ms]
    if sum(1 for m in marks if m in (ACUTE, GRAVE, CIRCUMFLEX)) > 1:
        flags.add("multiple-accents")
    if GRAVE in marks:
        flags.add("grave-accent")
    for index, (letter, ms) in enumerate(letters):
        if index >= 2 and letter.lower() != "ρ" and (SMOOTH in ms or ROUGH in ms):
            flags.add("medial-breathing")
    if any("GREEK" not in unicodedata.name(letter, "") for letter, _ in letters):
        flags.add("non-greek-character")
    return sorted(flags)


# --- identity layer: oga-greta-lemma/1 ----------------------------------------------


def compound_notation_index(rows: list[dict]) -> dict[str, list[dict]]:
    """Morpheus compound records keyed by Unicode prefix+base, the hyphen omitted.

    ``su/n-le/gw2 sulle/gw`` gives key ``σύνλέγω2``. Multi-prefix records are left out.
    """
    index: dict[str, list[dict]] = collections.defaultdict(list)
    for row in rows:
        derivation = row["derivation"]
        if row["class"] not in ("compound", "lsj-compound") or derivation.count("-") != 1:
            continue
        prefix, base = derivation.split("-")
        if "," in prefix or row["head"] is None:
            continue
        try:
            prefix_head, prefix_digits = v2.decode_beta(prefix)
            base_head, base_digits = v2.decode_beta(base)
        except v2.BetaError:
            continue
        if prefix_digits:
            continue
        index[v2.citation_key(prefix_head + base_head) + base_digits].append(row)
    return dict(index)


def compound_base(row: dict) -> str | None:
    """Decoded base lemma of a single-prefix compound record (e.g. εἰμί)."""
    if row["class"] not in ("compound", "lsj-compound") or row["derivation"].count("-") != 1:
        return None
    try:
        head, digits = v2.decode_beta(row["derivation"].split("-")[1])
    except v2.BetaError:
        return None
    return head + digits


def stem_entry_classes(row: dict, stem_lines: dict[str, list[str]]) -> list[str]:
    """Keyword lines of one ``:le:`` block, e.g. ``wd:conj`` or ``aj``."""
    lines = stem_lines.get(row["file"])
    if lines is None:
        raise InputError(f"stem source {row['file']} needed for {row['key']} is not retained")
    classes = []
    for line in lines[row["line"]:]:
        if line.startswith(":le:") or not line.strip():
            break
        match = re.match(r":([a-z]{2}):\S*\s*(.*)", line)
        if not match:
            continue
        keyword, rest = match.groups()
        if keyword == "wd":
            tail = [w for w in rest.split() if w in ("conj", "exclam", "adverb", "prep", "particle")]
            classes.append("wd:" + (tail[0] if tail else "form"))
        else:
            classes.append(keyword)
    return sorted(set(classes))


def declared_meaning(rows: list[dict], stem_lines: dict[str, list[str]]) -> dict:
    """What the producer scheme declares about one exact key: stem classes or bases."""
    bases = sorted({b for r in rows if (b := compound_base(r))})
    classes = sorted({c for r in rows if r["class"] == "le-block" for c in stem_entry_classes(r, stem_lines)})
    return {"compound_bases": bases, "stem_classes": classes,
            "witnesses": [f"{r['file']}:{r['line']}" for r in rows]}


def interpret_lemma(raw: str, by_head: dict, compounds: dict, stem_lines: dict) -> dict:
    """One OGA lemma label under oga-greta-lemma/2. Unknown shapes stay unresolved.

    A plain label is a usable citation representation unless it is orthographically
    anomalous (``citation_form_flags``) and no Morpheus key attests that exact
    spelling. Attested anomalies (enclitic ``-δε`` forms, koronis crasis) stay
    usable; unattested ones are unsupported, never repaired or accent-folded.
    """
    headword, digits = split_label(raw)
    exact = [r for r in by_head.get(v2.citation_key(headword), []) if r["digits"] == digits and not r["loose"]]
    notation = compounds.get(v2.citation_key(headword) + digits, []) if not exact else []
    result = {"raw_lemma": raw, "status": "resolved", "discriminator": None, "qualifications": []}
    if digits and exact:
        meaning = declared_meaning(exact, stem_lines)
        result.update(rule="I-NUM-MORPHEUS-KEY", headword=headword,
                      discriminator={"scheme": DISCRIMINATOR_SCHEME, "value": digits},
                      morpheus_key=exact[0]["key"], declared=meaning)
        if len(meaning["compound_bases"]) > 1:
            result["qualifications"].append("scheme-derivation-ambiguous")
    elif len(notation) == 1:
        target = notation[0]
        decoded, target_digits = v2.decode_beta(target["key"])
        result.update(rule="I-COMPOUND-RECORD", headword=decoded, morpheus_key=target["key"],
                      declared={"compound_record": f"{target['file']}:{target['line']}",
                                "derivation": target["derivation"]})
        if target_digits:
            result["discriminator"] = {"scheme": DISCRIMINATOR_SCHEME, "value": target_digits}
    elif digits:
        result.update(rule="I-UNRESOLVED-NUMBERED", headword=None, status="unresolved",
                      reason="numbered label has no exact Morpheus key or unique compound record")
    elif citation_form_flags(headword) and not exact:
        result.update(rule="I-UNSUPPORTED-CITATION-FORM", headword=None, status="unresolved",
                      predicted_form=headword,
                      reason="orthographically anomalous citation form with no attesting Morpheus key "
                             "and no unique compound record; not repaired")
    else:
        result.update(rule="I-PLAIN", headword=headword, attested_key=bool(exact))
    result["raw_flags"] = citation_form_flags(headword)
    result["citation_flags"] = []
    if result["headword"] is not None:
        result["key"] = profile_key(result["headword"])
        if result["rule"] == "I-PLAIN" and not exact:
            result["citation_flags"] = citation_form_flags(result["headword"])
    return result


def identity_string(lemma: dict) -> str:
    """Probe serialization of (canonical lemma, discriminator); UPOS is a separate field."""
    disc = lemma["discriminator"]
    return lemma["key"] + (f"#{disc['scheme']}:{disc['value']}" if disc else "")


# --- POS layer: oga-agdt-upos/1 -------------------------------------------------------


def read_conllu_rows(path: Path) -> list[list[list[str]]]:
    sentences, current = [], []
    with Path(path).open(encoding="utf-8") as handle:
        for line in handle:
            line = line.rstrip("\n")
            if not line:
                if current:
                    sentences.append(current)
                current = []
            elif not line.startswith("#"):
                current.append(line.split("\t"))
    if current:
        sentences.append(current)
    return sentences


def ud_perseus_evidence(paths: list[Path]) -> dict:
    """Token-paired AGDT letter (XPOS) and UPOS counts in the UD Perseus conversion."""
    by_letter: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    by_pair: dict[tuple[str, str], collections.Counter] = collections.defaultdict(collections.Counter)
    aux: collections.Counter = collections.Counter()
    tokens = 0
    for path in paths:
        for sentence in read_conllu_rows(path):
            for cols in sentence:
                if "-" in cols[0] or "." in cols[0]:
                    continue
                tokens += 1
                letter, upos, key = cols[4][:1], cols[3], profile_key(cols[2])
                by_letter[letter][upos] += 1
                by_pair[(letter, key)][upos] += 1
                if upos == "AUX":
                    aux[(unicodedata.normalize("NFC", cols[2]), cols[7])] += 1
    letters = {}
    for letter, expected in EXPECTED_LETTER_UPOS.items():
        observed = dict(by_letter.get(letter, {}))
        if set(observed) != {expected}:
            raise InputError(f"UD Perseus letter {letter!r} is no longer single-valued: {observed}")
        letters[letter] = observed
    for letter, allowed in CONTEXT_LETTERS.items():
        if set(by_letter[letter]) != allowed:
            raise InputError(f"UD Perseus letter {letter!r} has unexpected UPOS {dict(by_letter[letter])}")
    num_lemmas = sorted({k for (letter, k), c in by_pair.items() if c.get("NUM")})
    return {"tokens": tokens, "letters": letters,
            "context_letters": {k: dict(by_letter[k]) for k in CONTEXT_LETTERS},
            "pairs": by_pair, "aux": {f"{lemma}|{rel}": n for (lemma, rel), n in sorted(aux.items())},
            "num_lemmas": num_lemmas}


def pnom_reachable(cols: list[str], children: dict[str, list[list[str]]]) -> bool:
    """PNOM child directly or through coordination/apposition members (AGDT COORD/APOS)."""
    stack = list(children.get(cols[0], []))
    while stack:
        child = stack.pop()
        if child[7] == "PNOM":
            return True
        if child[7] in ("COORD", "APOS"):
            stack.extend(children.get(child[0], []))
    return False


def eimi_context(cols: list[str], children: dict[str, list[list[str]]]) -> dict:
    """Native-syntax evidence for a nonverbal predicate under one verb node."""
    kids = children.get(cols[0], [])
    return {"pnom": pnom_reachable(cols, children),
            "pp": any(k[7] == "AuxP" for k in kids),
            "adverbial": any(k[7] == "ADV" for k in kids),
            "infinitive_subject": any(k[7] == "SBJ" and k[3] == "v" and "Mood=n" in k[5] for k in kids)}


def eimi_rule(context: dict) -> tuple[str, str]:
    """UD AUX/VERB for εἰμί from the UD definitions applied to native AGDT syntax.

    AUX is a copula linking a nonverbal predicate (UD AUX, cop) or the grc
    periphrastic auxiliary; AGDT marks both with PNOM. UD's one-verb analysis
    also treats location, possession and benefaction phrases, and existentials
    with a location, as predicates of the copula: AGDT gives these as AuxP or
    ADV dependents. Impersonal ἔστι + infinitive has no nonverbal predicate, and
    the grc AUX page admits no modal auxiliary. A pure existential has "no
    predicate other than the verb" (simple-syntax), so it is neither copula nor
    auxiliary.
    """
    if context["pnom"]:
        return "AUX", "P-V-EIMI-COP-PNOM"
    if context["infinitive_subject"] and not context["pp"]:
        return "VERB", "P-V-EIMI-IMPERSONAL-INFINITIVE"
    if context["pp"] or context["adverbial"]:
        return "AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE"
    return "VERB", "P-V-EIMI-EXISTENTIAL"


def interpret_pos(native: str, head_key: str, relation: str, context: dict, evidence: dict) -> dict:
    """Canonical UPOS for one printed node. Unknown tags and unmapped contexts fail closed."""
    pair = dict(evidence["pairs"].get((native, head_key), {}))
    if native in EXPECTED_LETTER_UPOS:
        upos = EXPECTED_LETTER_UPOS[native]
        return {"upos": upos, "candidates": [upos], "rule": f"P-LETTER-{native}",
                "status": "resolved", "pair_attested": pair.get(upos, 0)}
    if native == "c":
        if len(pair) == 1:
            upos = next(iter(pair))
            return {"upos": upos, "candidates": [upos], "rule": "P-C-PAIR", "status": "resolved",
                    "pair_attested": pair[upos]}
        return {"upos": None, "candidates": ["CCONJ", "SCONJ"], "rule": "P-C-UNDECIDED",
                "status": "ambiguous-outside-allowed", "pair_attested": sum(pair.values())}
    if native == "v":
        if relation == "AuxV" and head_key != profile_key(EIMI):
            return {"upos": None, "candidates": ["AUX", "VERB"], "rule": "P-V-AUXV-UNMAPPED",
                    "status": "unresolved", "pair_attested": 0}
        if head_key == profile_key(EIMI):
            upos, rule = eimi_rule(context)
            return {"upos": upos, "candidates": [upos], "rule": rule, "status": "resolved",
                    "pair_attested": pair.get(upos, 0), "context": context}
        return {"upos": "VERB", "candidates": ["VERB"], "rule": "P-V-VERB", "status": "resolved",
                "pair_attested": pair.get("VERB", 0)}
    return {"upos": None, "candidates": [], "rule": "P-UNSUPPORTED-TAG", "status": "unresolved",
            "pair_attested": 0}


def vocabulary_status(lemma: dict, pos: dict) -> str:
    if pos["status"] == "unresolved":
        return "unresolved-pos"
    if pos["status"] == "ambiguous-outside-allowed" or pos["upos"] not in ALLOWED_POS:
        return "non-vocabulary"
    if lemma["status"] != "resolved":
        return "unresolved-identity"
    return "eligible"


# --- sentences: oga-reading-terminal/1 ----------------------------------------------------


def segment(text: str) -> list[dict]:
    """Complete sentences over the exact reading text; terminal plus following closers."""
    for char in UNSUPPORTED_TERMINALS:
        if char in text:
            raise InputError(f"terminal {char!r} has no reviewed convention in this source")
    tokens = [(m.start(), m.end(), m.group()) for m in re.finditer(r"\S+", text)]
    sentences: list[dict] = []
    start, index = None, 0
    while index < len(tokens):
        begin, end, token = tokens[index]
        start = begin if start is None else start
        index += 1
        if token not in TERMINALS:
            continue
        while index < len(tokens) and tokens[index][2] in CLOSERS:
            end = tokens[index][1]
            index += 1
        sentences.append({"start": start, "end": end, "terminal": token})
        start = None
    if start is not None:
        raise InputError("reading text does not end on a terminal")
    for number, sentence in enumerate(sentences):
        body = text[sentence["start"]:sentence["end"]]
        stripped = body.strip()
        sentence.update(index=number, units=len(stripped.split()), code_points=len(stripped),
                        punctuation_units=sum(1 for u in stripped.split() if not any(c.isalpha() for c in u)))
    return sentences


def check_native_join(nodes: list[dict], groups: list[list[list[str]]]) -> dict:
    """Required gate: unique native IDs; each printed node joins exactly one CoNLL-U row
    with the same surface, raw lemma and native POS; no Book 1 group row lacks a node."""
    ids = [n["id"] for n in nodes]
    if len(ids) != len(set(ids)):
        raise InputError("duplicate native node IDs")
    rows: dict[str, list[list[str]]] = collections.defaultdict(list)
    for group in groups:
        for cols in group:
            rows[cols[9]].append(cols)
    known = set(ids)
    if any(row_id not in known for row_id in rows):
        raise InputError("a Book 1 native group row has no native node")
    for node in nodes:
        found = rows.get(node["id"], [])
        if len(found) > 1 or (not node["empty"] and len(found) != 1):
            raise InputError(f"node {node['id']} joins {len(found)} CoNLL-U rows")
        if node["empty"]:
            continue
        cols = found[0]
        if (cols[1], cols[2], cols[3]) != (node["surface"], node["raw_lemma"], node["native_morphology"]["pos"]):
            raise InputError(f"node {node['id']} differs from its CoNLL-U row")
    return {"native_ids_unique": True, "printed_rows_joined": sum(1 for n in nodes if not n["empty"]),
            "empty_rows_joined": sum(1 for n in nodes if n["empty"] and rows.get(n["id"]))}


def check_coverage(text: str, printed: list[dict]) -> dict:
    """Required gate: every non-whitespace reading character lies in exactly one printed
    node span, each span reproduces its surface, and spans never overlap."""
    owner = [0] * len(text)
    for node in printed:
        start, end = node["reading_span"]
        if not 0 <= start < end <= len(text) or text[start:end] != node["surface"]:
            raise InputError(f"reading span of {node['id']} does not reproduce its surface")
        for i in range(start, end):
            owner[i] += 1
    if any(c > 1 for c in owner):
        raise InputError("printed node spans overlap")
    uncovered = [i for i, ch in enumerate(text) if not ch.isspace() and owner[i] == 0]
    if uncovered:
        raise InputError(f"{len(uncovered)} non-whitespace reading characters are not covered, first at {uncovered[0]}")
    return {"covered_non_whitespace": sum(1 for ch in text if not ch.isspace()), "overlaps": 0}


def check_sentence_coverage(text: str, sentences: list[dict]) -> dict:
    """Required gate: sentences are ordered, disjoint, end on a terminal, and leave only
    whitespace between them and after the last."""
    position = 0
    for sentence in sentences:
        if text[position:sentence["start"]].strip() or sentence["start"] < position:
            raise InputError(f"text outside sentences before {sentence['start']}")
        if sentence["terminal"] not in TERMINALS:
            raise InputError(f"sentence {sentence['index']} has no terminal")
        position = sentence["end"]
    if text[position:].strip():
        raise InputError("non-whitespace text after the last sentence")
    return {"sentences": len(sentences), "tail_is_whitespace": True}


def sentence_of(nodes: list[dict], sentences: list[dict]) -> dict[str, int]:
    starts = [s["start"] for s in sentences]
    out = {}
    for node in nodes:
        start, end = node["reading_span"]
        position = bisect.bisect_right(starts, start) - 1
        if not sentences[position]["start"] <= start < end <= sentences[position]["end"]:
            raise InputError(f"node {node['id']} crosses a sentence boundary")
        out[node["id"]] = position
    return out


def native_group_check(groups: list[list[list[str]]], where: dict[str, int], nodes_by_id: dict) -> dict:
    """Every complete sentence is a union of whole native groups; none crosses."""
    crossing, per_sentence = [], collections.Counter()
    for number, group in enumerate(groups):
        indices = {where[c[9]] for c in group if c[9] in where}
        if len(indices) > 1:
            crossing.append(number)
        for index in indices:
            per_sentence[index] += 1
    if crossing:
        raise InputError(f"native groups cross complete-sentence boundaries: {crossing[:5]}")
    empties = collections.defaultdict(list)
    for group in groups:
        printed = [where[c[9]] for c in group if c[9] in where]
        for cols in group:
            if cols[9] in nodes_by_id and nodes_by_id[cols[9]]["empty"] and printed:
                empties[printed[0]].append(cols[9])
    return {"native_groups": len(groups), "groups_crossing_sentences": crossing,
            "groups_per_sentence": dict(sorted(collections.Counter(per_sentence.values()).items())),
            "empty_nodes_by_sentence": {k: v for k, v in sorted(empties.items())}}


def word_internal_spaces(nodes: list[dict], tei: dict[str, list[int]]) -> list[str]:
    """Nodes followed by a reading-text space that is not in the original TEI (οὐ δὲ)."""
    found = []
    for a, b in zip(nodes, nodes[1:], strict=False):
        if a["cts"] != b["cts"] or a["native_morphology"]["pos"] == "u" or b["native_morphology"]["pos"] == "u":
            continue
        if tei[a["id"]][-1] + 1 == tei[b["id"]][0]:
            found.append(a["id"])
    return found


def editorial_by_sentence(nodes: list[dict], where: dict, tei: dict, ledger: dict) -> dict[int, list[str]]:
    """Editorial elements whose TEI span lies within a sentence's node range in a section."""
    ranges: dict[tuple[int, str], list[int]] = {}
    for node in nodes:
        key = (where[node["id"]], node["cts"])
        lo, hi = tei[node["id"]][0], tei[node["id"]][-1]
        old = ranges.get(key, [lo, hi])
        ranges[key] = [min(old[0], lo), max(old[1], hi)]
    out: dict[int, list[str]] = collections.defaultdict(list)
    for section in ledger["sections"]:
        for element in section["editorial_elements"]:
            lo, hi = element["xml_string_span"]
            for (index, cts), (first, last) in ranges.items():
                if cts == section["cts"] and first <= lo and hi <= last + 1:
                    out[index].append(f"{element['element']}@{cts}")
    return {k: sorted(set(v)) for k, v in out.items()}


# --- selection and card inputs ------------------------------------------------------------


def selection_model(occurrences: dict[tuple[str, str], list[str]]) -> list[dict]:
    """3/15 with one analyzed Book: the across-Books total equals the in-Book count."""
    rows = []
    for key, upos in sorted(occurrences, key=lambda k: (k[0].encode(), k[1].encode())):
        count = len(occurrences[(key, upos)])
        across = count if count < MIN_IN_BOOK else 0
        selected = count >= MIN_IN_BOOK or (count >= 1 and across >= MIN_ACROSS_BOOKS)
        rows.append({"key": key, "upos": upos, "occurrences": count, "selected": selected})
    return rows


def selection_tsv(rows: list[dict]) -> str:
    lines = ["canonical_key\tupos\toccurrences\tselected"]
    lines += [f"{r['key']}\t{r['upos']}\t{r['occurrences']}\t{'yes' if r['selected'] else 'no'}" for r in rows]
    return "\n".join(lines) + "\n"


def card_inputs(selected: list[dict], occurrences: dict, ledger: dict, sentences: list[dict], text: str) -> list[dict]:
    """Earliest qualifying occurrence per selected identity; over-cap sentences are never shortened."""
    out = []
    for row in selected:
        ids = sorted(occurrences[(row["key"], row["upos"])], key=lambda i: (ledger[i]["reading_span"][0], i))
        rejections, chosen = [], None
        for node_id in ids:
            reasons = card_rejections(ledger[node_id], sentences, text)
            if not reasons:
                chosen = node_id
                break
            rejections.append({"id": node_id, "reasons": reasons})
        entry = {"key": row["key"], "upos": row["upos"], "chosen": chosen,
                 "earlier_rejections": rejections if chosen else [], "omission": None}
        if chosen is None:
            entry["omission"] = sorted({r for rej in rejections for r in rej["reasons"]})
            entry["rejections"] = rejections
        else:
            entry["sentence"] = ledger[chosen]["sentence"]
        out.append(entry)
    return out


def card_rejections(node: dict, sentences: list[dict], text: str) -> list[str]:
    sentence = sentences[node["sentence"]]
    start, end = node["reading_span"]
    reasons = []
    if sentence.get("completeness", "complete") != "complete":
        reasons.append("sentence-incomplete")
    if sentence["units"] > MAX_UNITS:
        reasons.append("over-unit-cap")
    if sentence["code_points"] > MAX_CODE_POINTS:
        reasons.append("over-code-point-cap")
    if text[start:end] != node["surface"] or not sentence["start"] <= start < end <= sentence["end"]:
        reasons.append("target-span-unverified")
    if node["citation_flags"]:
        reasons.append("headword-unusable:" + "+".join(node["citation_flags"]))
    return reasons


# --- lineage and prediction diagnostics -------------------------------------------------------


def gorman_alignment(printed: list[dict]) -> dict:
    """Training-data lineage witness: same-section surface alignment to Gorman's Book 1 trees."""
    gorman: dict[str, list[tuple[str, str]]] = collections.defaultdict(list)
    for path in GORMAN:
        for sentence in ET.parse(path).getroot().iter("sentence"):
            for word in sentence.iter("word"):
                form = unicodedata.normalize("NFC", word.get("form") or "")
                if form and not form.startswith("["):
                    gorman[sentence.get("subdoc")].append(
                        (form, unicodedata.normalize("NFC", word.get("lemma") or ""), (word.get("postag") or "-")[:1]))
    by_section = collections.defaultdict(list)
    for node in printed:
        by_section[node["cts"].replace("_", ".")].append(node)
    aligned: dict[str, tuple[str, str]] = {}
    for section, nodes in by_section.items():
        theirs = gorman.get(section, [])
        matcher = difflib.SequenceMatcher(None, [n["surface"] for n in nodes], [w[0] for w in theirs], autojunk=False)
        for block in matcher.get_matching_blocks():
            for k in range(block.size):
                aligned[nodes[block.a + k]["id"]] = theirs[block.b + k][1:]
    lexical = [n for n in printed if n["id"] in aligned and n["native_morphology"]["pos"] != "u"]
    same = sum(1 for n in printed if n["id"] in aligned and aligned[n["id"]][0].lower() == n["raw_lemma"])

    def relation(node) -> str:
        """Gorman's label on the aligned token, as a category; Gorman strings stay private."""
        if node["id"] not in aligned:
            return "unaligned"
        theirs = aligned[node["id"]][0].lower()
        if theirs == node["raw_lemma"]:
            return "same-string"
        if theirs == split_label(node["raw_lemma"])[0]:
            return "unnumbered-same-headword"
        if split_label(theirs)[0] == split_label(node["raw_lemma"])[0]:
            return "other-number-same-headword"
        return "different-lemma"

    numbered = collections.Counter(
        (n["raw_lemma"], relation(n)) for n in printed if re.search(r"\d", n["raw_lemma"]))
    off_book = sorted(s for s in gorman if s not in by_section)
    return {"method": "same-section surface alignment (difflib), a local lexical alignment; "
                      "not a complete binding, a training-partition proof or an accuracy gold standard",
            "gorman_subdoc_labels": len(gorman), "oga_sections_without_gorman": sorted(set(by_section) - set(gorman)),
            "gorman_subdoc_labels_not_oga_book1_sections": off_book,
            "aligned_printed_nodes": len(aligned), "lemma_equal_after_lowercase": same,
            "aligned_lexical_nodes": len(lexical),
            "lexical_lemma_equal_after_lowercase": sum(1 for n in lexical if aligned[n["id"]][0].lower() == n["raw_lemma"]),
            "lexical_pos_letter_equal": sum(1 for n in lexical if aligned[n["id"]][1] == n["native_morphology"]["pos"]),
            "numbered_label_relations": [{"oga": a, "gorman_same_token": b, "count": c}
                                         for (a, b), c in sorted(numbered.items())],
            "gorman_numbered_label_tokens": sum(1 for words in gorman.values() for _, lemma, _ in words
                                                if re.search(r"\d", lemma) and not lemma.startswith("["))}


def compound_consistency(ledger: dict) -> list[dict]:
    """Numbered εἰμί/εἶμι compound predictions against the base their Morpheus key declares."""
    base_of = {s: b for b, surfaces in COMPOUND_SURFACE_BASE.items() for s in surfaces}
    rows = []
    for node in ledger.values():
        lemma = node.get("lemma")
        if not lemma or lemma["rule"] != "I-NUM-MORPHEUS-KEY" or not lemma["declared"]["compound_bases"]:
            continue
        bases = {b.rstrip("0123456789") for b in lemma["declared"]["compound_bases"]}
        if not bases <= {EIMI, "εἶμι"}:
            continue
        form_base = base_of.get(node["surface"])
        if len(bases) > 1:
            verdict = "scheme-ambiguous"
        elif form_base is None:
            verdict = "form-unclassified"
        else:
            verdict = "consistent" if form_base in bases else "inconsistent"
        rows.append({"id": node["id"], "cts": node["cts"], "raw_lemma": node["raw_lemma"],
                     "declared_base": sorted(bases), "form_base": form_base, "verdict": verdict})
    return sorted(rows, key=lambda r: int(r["id"][2:]))


STEM_CLASS_LETTERS = {"wd:conj": {"c"}, "wd:adverb": {"d"}, "wd:exclam": {"i"}, "aj": {"a"},
                      "no": {"n"}, "vs": {"v"}, "de": {"v"}, "vb": {"v"}}


def stem_class_consistency(ledger: dict) -> list[dict]:
    """Native POS of numbered simplex predictions against the keyed stem entry's class."""
    counts: collections.Counter = collections.Counter()
    for node in ledger.values():
        lemma = node.get("lemma")
        if not lemma or lemma["rule"] != "I-NUM-MORPHEUS-KEY" or not lemma["declared"]["stem_classes"]:
            continue
        letters = set().union(*(STEM_CLASS_LETTERS.get(c, set()) for c in lemma["declared"]["stem_classes"]))
        verdict = "consistent" if node["native_pos"] in letters else "native-tag-differs"
        counts[(node["raw_lemma"], ",".join(lemma["declared"]["stem_classes"]), node["native_pos"], verdict)] += 1
    return [{"raw_lemma": a, "stem_classes": b, "native_pos": c, "verdict": d, "count": n}
            for (a, b, c, d), n in sorted(counts.items())]


# --- pipeline ------------------------------------------------------------------------------


def load_stem_lines() -> dict[str, list[str]]:
    return {p.name: p.read_text(encoding="latin-1").splitlines() for p in sorted(STEMSRC.iterdir())}


def interpret_nodes(nodes, groups, evidence, lexicon) -> dict[str, dict]:
    """Per-node ledger for all 10,572 native nodes."""
    by_head, compounds, stem_lines = lexicon
    rows = {cols[9]: (cols, group) for group in groups for cols in group}
    lemma_cache: dict[str, dict] = {}
    ledger = {}
    for node in nodes:
        if node["empty"]:
            ledger[node["id"]] = {"id": node["id"], "cts": node["cts"], "empty": True, "status": "empty-unprinted"}
            continue
        cols, group = rows[node["id"]]
        raw = node["raw_lemma"]
        native = node["native_morphology"]["pos"]
        if native == "u":
            lemma = {"raw_lemma": raw, "status": "resolved", "rule": "I-PUNCTUATION", "headword": None,
                     "discriminator": None, "qualifications": [], "key": None, "citation_flags": [], "raw_flags": []}
        else:
            lemma = lemma_cache.setdefault(raw, interpret_lemma(raw, by_head, compounds, stem_lines))
        head_key = profile_key(lemma["headword"]) if lemma.get("headword") else profile_key(raw)
        children = collections.defaultdict(list)
        for c in group:
            children[c[6]].append(c)
        pos = interpret_pos(native, head_key, cols[7], eimi_context(cols, children), evidence)
        status = vocabulary_status(lemma, pos)
        ledger[node["id"]] = {
            "id": node["id"], "cts": node["cts"], "empty": False, "surface": node["surface"],
            "raw_lemma": raw, "native_pos": native, "relation": cols[7],
            "native_features": sorted(k for k in node["native_morphology"] if k != "pos"),
            "reading_span": node["reading_span"], "lemma": lemma, "pos": pos, "status": status,
            "identity": identity_string(lemma) if status == "eligible" else None,
            "citation_flags": lemma.get("citation_flags", []),
        }
    return ledger


def scenario(ledger: dict, override) -> dict:
    """Recount eligible occurrences and selection under an alternative, not-adopted rule."""
    occurrences: dict[tuple[str, str], list[str]] = collections.defaultdict(list)
    unresolved = 0
    for node in ledger.values():
        if node["empty"]:
            continue
        status, identity, upos = override(node)
        if status == "eligible":
            occurrences[(identity, upos)].append(node["id"])
        elif status.startswith("unresolved"):
            unresolved += 1
    rows = selection_model(occurrences)
    return {"eligible_occurrences": sum(len(v) for v in occurrences.values()), "identities": len(rows),
            "selected": sum(r["selected"] for r in rows), "unresolved_printed_nodes": unresolved}


def adopted(node):
    return node["status"], node["identity"], node["pos"]["upos"]


def sensitivities(ledger: dict, evidence: dict) -> dict:
    """Comparison scenarios. They are diagnostics of the adopted rules, not choices
    delegated to anyone: each names a different interpretation contract."""
    det = {profile_key(x) for x in UD_GRC_DET_EXAMPLES}
    part = {profile_key(x) for x in UD_GRC_PART_EXAMPLES}
    num = set(evidence["num_lemmas"])

    def predicted_forms(node):
        lemma = node["lemma"]
        if lemma["rule"] == "I-UNSUPPORTED-CITATION-FORM" and node["pos"]["upos"] in ALLOWED_POS:
            return "eligible", profile_key(lemma["predicted_form"]), node["pos"]["upos"]
        return adopted(node)

    def eimi_all_aux(node):
        if node["pos"]["rule"].startswith("P-V-EIMI"):
            return "non-vocabulary", None, None
        return adopted(node)

    def definitional(node):
        key = node["lemma"].get("key")
        if node["status"] == "eligible" and ((node["native_pos"] == "a" and key in det | num)
                                             or (node["native_pos"] == "d" and key in part)):
            return "non-vocabulary", None, None
        return adopted(node)

    def without_compound_rule(node):
        if node["status"] == "eligible" and node["lemma"]["rule"] == "I-COMPOUND-RECORD":
            return "unresolved-identity", None, None
        return adopted(node)

    return {"adopted_v2": scenario(ledger, adopted),
            "C_unsupported_forms_counted_as_predicted_strings_(0e839900_identity_handling)": scenario(ledger, predicted_forms),
            "D_eimi_all_AUX_ud_perseus_conversion_compatible": scenario(ledger, eimi_all_aux),
            "D_strict_ud_grc_DET_NUM_PART_examples": scenario(ledger, definitional),
            "D_without_compound_record_rule": scenario(ledger, without_compound_rule)}


def analysis_json(ledger: dict, sentences: list[dict], text: str) -> str:
    """Analyzer-shaped Go probe input: every resolved printed node of every complete sentence."""
    tokens = collections.defaultdict(list)
    for node in sorted((n for n in ledger.values() if not n["empty"]), key=lambda n: n["reading_span"][0]):
        if node["status"].startswith("unresolved"):
            continue
        upos = node["pos"]["upos"] or "X"
        key = identity_string(node["lemma"]) if node["lemma"].get("key") else node["raw_lemma"]
        start, end = node["reading_span"]
        tokens[node["sentence"]].append({
            "Surface": node["surface"], "RawLemma": node["raw_lemma"], "CanonicalLemma": key,
            "UPOS": upos, "Dependency": "", "Location": {"SourceDocumentID": SOURCE_DOCUMENT_ID,
                                                         "Section": node["cts"], "StartOffset": start,
                                                         "EndOffset": end}})
    analysis = {"Language": LANGUAGE, "Sentences": [
        {"Text": text[s["start"]:s["end"]], "Tokens": tokens[s["index"]],
         "Location": {"SourceDocumentID": SOURCE_DOCUMENT_ID, "StartOffset": s["start"], "EndOffset": s["end"]}}
        for s in sentences]}
    return json.dumps(analysis, ensure_ascii=False, sort_keys=True, separators=(",", ":")) + "\n"


def length_profile(sentences: list[dict]) -> dict:
    units = sorted(s["units"] for s in sentences)
    points = sorted(s["code_points"] for s in sentences)

    def stats(values):
        n = len(values)
        median = values[n // 2] if n % 2 else (values[n // 2 - 1] + values[n // 2]) / 2
        return {"min": values[0], "median": median, "p90": values[(9 * n + 9) // 10 - 1], "max": values[-1]}

    buckets = collections.Counter(min(u // 10, 10) for u in units)
    return {"sentences": len(sentences), "units": stats(units), "code_points": stats(points),
            "units_histogram_by_10": {f"{10 * k}-{10 * k + 9}" if k < 10 else "100+": v for k, v in sorted(buckets.items())},
            "at_most_5_units": sum(1 for u in units if u <= 5),
            "over_unit_cap": sum(1 for s in sentences if s["units"] > MAX_UNITS),
            "over_code_point_cap": sum(1 for s in sentences if s["code_points"] > MAX_CODE_POINTS),
            "over_code_point_cap_only": sum(1 for s in sentences if s["code_points"] > MAX_CODE_POINTS and s["units"] <= MAX_UNITS),
            "within_both_caps": sum(1 for s in sentences if s["units"] <= MAX_UNITS and s["code_points"] <= MAX_CODE_POINTS),
            "over_unit_cap_if_punctuation_units_excluded": sum(1 for s in sentences if s["units"] - s["punctuation_units"] > MAX_UNITS),
            "terminals": dict(collections.Counter(s["terminal"] for s in sentences))}


def examples(ledger, sentences, inputs, splits, editorial, group_check) -> dict:
    """Deterministic private review cases; each is the first match of a stated rule, or absent."""
    chosen = [c for c in inputs if c["chosen"]]
    chosen.sort(key=lambda c: (ledger[c["chosen"]]["reading_span"][0], c["chosen"]))
    first = lambda pred: next((c for c in chosen if pred(c, ledger[c["chosen"]])), None)  # noqa: E731
    split_sentences = {ledger[i]["sentence"] for i in splits}
    out = {
        "ordinary-complete-sentence": first(lambda c, n: 6 <= sentences[n["sentence"]]["units"] <= MAX_UNITS),
        "short-complete-sentence-chosen": first(lambda c, n: sentences[n["sentence"]]["units"] <= 5),
        "cap-rejection-then-later-occurrence": first(lambda c, n: bool(c["earlier_rejections"])),
        "repeated-target-in-sentence": first(lambda c, n: sum(
            1 for o in ledger.values() if not o["empty"] and o["identity"] == n["identity"]
            and o["pos"]["upos"] == n["pos"]["upos"] and o["sentence"] == n["sentence"]) > 1),
        "lexical-discriminator": first(lambda c, n: n["lemma"]["discriminator"] is not None),
        "compound-record-decoding": first(lambda c, n: n["lemma"]["rule"] == "I-COMPOUND-RECORD"),
        "eimi-aux-adverbial-predicate": next(({"node": n["id"]} for n in ledger.values() if not n["empty"]
                                              and n["pos"]["rule"] == "P-V-EIMI-COP-ADVERBIAL-PREDICATE"), None),
        "eimi-verb-existential": next(({"node": n["id"]} for n in ledger.values() if not n["empty"]
                                       and n["pos"]["rule"] == "P-V-EIMI-EXISTENTIAL"), None),
        "eimi-verb-impersonal-infinitive": next(({"node": n["id"]} for n in ledger.values() if not n["empty"]
                                                 and n["pos"]["rule"] == "P-V-EIMI-IMPERSONAL-INFINITIVE"), None),
        "lowercased-name-headword-as-NOUN": first(lambda c, n: n["pos"]["upos"] == "NOUN"
                                                  and n["surface"][:1].isupper() and n["lemma"]["headword"][:1].islower()),
        "particle-as-ADV": first(lambda c, n: n["lemma"]["key"] == "δέ" and n["pos"]["upos"] == "ADV"),
        "word-internal-space-in-sentence": first(lambda c, n: n["sentence"] in split_sentences),
        "elision-target": next(({"node": n["id"], "chosen_for_input": any(c["chosen"] == n["id"] for c in chosen)}
                                for n in sorted(ledger.values(), key=lambda o: o.get("reading_span", [0])[0])
                                if not n["empty"] and n["status"] == "eligible" and n["surface"].endswith("\u02bc")), None),
        "editorial-material-in-sentence": first(lambda c, n: n["sentence"] in editorial),
        "empty-node-in-sentence": first(lambda c, n: n["sentence"] in group_check["empty_nodes_by_sentence"]),
        "optional-morphology-absent": first(lambda c, n: n["pos"]["upos"] in ("NOUN", "VERB", "ADJ")
                                            and not n["native_features"]),
        "unsupported-citation-form": next(({"node": n["id"], "flags": n["lemma"]["raw_flags"]} for n in ledger.values()
                                           if not n["empty"] and n["lemma"]["rule"] == "I-UNSUPPORTED-CITATION-FORM"), None),
        "omission": next(({"identity": c["key"], "upos": c["upos"], "reasons": c["omission"]}
                          for c in inputs if c["omission"]), None),
        "nfd-polytonic-source": None,  # reading text is NFC throughout (checked in build)
    }
    return {k: v for k, v in out.items()}


def build() -> tuple[dict, dict]:
    """Return ``(summary, private_texts)``; nothing is written here."""
    inputs = verify_inputs()
    nodes = json.loads(NATIVE_NODES.read_text(encoding="utf-8"))
    import_ledger = json.loads(IMPORT_LEDGER.read_text(encoding="utf-8"))
    binding = v2.verify_binding(nodes, import_ledger)
    text = READING.read_text(encoding="utf-8")
    if not unicodedata.is_normalized("NFC", text):
        raise InputError("reading text is not NFC; the NFD example rule needs revisiting")
    tei = {r["id"]: r["tei_string_indices"] for r in json.loads(NODE_TO_TEI.read_text(encoding="utf-8"))}
    node_ids = {n["id"] for n in nodes}
    groups = [g for g in read_conllu_rows(CONLLU) if any(c[9] in node_ids for c in g)]
    gates = {"native_join": check_native_join(nodes, groups)}
    rows = v2.load_index_rows()
    by_head, _ = v2.index_maps(rows)
    lexicon = (by_head, compound_notation_index(rows), load_stem_lines())
    evidence = ud_perseus_evidence(UD_PERSEUS)
    ledger = interpret_nodes(nodes, groups, evidence, lexicon)

    printed = [n for n in nodes if not n["empty"]]
    gates["reading_coverage"] = check_coverage(text, printed)
    sentences = segment(text)
    gates["sentence_coverage"] = check_sentence_coverage(text, sentences)
    where = sentence_of(printed, sentences)
    for node_id, index in where.items():
        ledger[node_id]["sentence"] = index
    group_check = native_group_check(groups, where, {n["id"]: n for n in nodes})
    gates["native_groups_nest"] = {"groups": group_check["native_groups"], "crossing": 0}
    splits = word_internal_spaces(printed, tei)
    editorial = editorial_by_sentence(printed, where, tei, import_ledger)
    for sentence in sentences:
        sentence["word_internal_spaces"] = sum(1 for i in splits if where[i] == sentence["index"])
        sentence["editorial"] = editorial.get(sentence["index"], [])
        sentence["completeness"] = "complete" if sentence["terminal"] in TERMINALS else "unverified"

    occurrences: dict[tuple[str, str], list[str]] = collections.defaultdict(list)
    for node in ledger.values():
        if not node["empty"] and node["status"] == "eligible":
            occurrences[(node["identity"], node["pos"]["upos"])].append(node["id"])
    model = selection_model(occurrences)
    selected = [r for r in model if r["selected"]]
    inputs_ = card_inputs(selected, occurrences, ledger, sentences, text)
    analysis = analysis_json(ledger, sentences, text)
    sensitivity = sensitivities(ledger, evidence)
    probe_expected = (f"candidates={len(model)} selected={len(selected)} "
                      f"analyzable_tokens={sum(r['occurrences'] for r in model)} "
                      f"distinct_identities={len(model)} sentences={len(sentences)}")
    summary = summarize(locals())
    private = private_texts(locals())
    summary["private_outputs"] = {name: {"sha256": hashlib.sha256(t.encode("utf-8")).hexdigest(),
                                         "lines": t.count("\n")} for name, t in sorted(private.items())}
    return summary, private


def counter(items) -> dict:
    return dict(sorted(collections.Counter(items).items()))


def summarize(ctx: dict) -> dict:
    ledger, model, sentences, inputs_ = ctx["ledger"], ctx["model"], ctx["sentences"], ctx["inputs_"]
    printed = [n for n in ledger.values() if not n["empty"]]
    numbered = sorted({n["raw_lemma"] for n in printed if re.search(r"\d", n["raw_lemma"])})
    labels = []
    for raw in numbered:
        ids = [n["id"] for n in printed if n["raw_lemma"] == raw]
        lemma = ledger[ids[0]]["lemma"]
        labels.append({"raw_lemma": raw, "occurrences": len(ids), "rule": lemma["rule"], "status": lemma["status"],
                       "headword": lemma.get("headword"), "discriminator": lemma["discriminator"],
                       "morpheus_key": lemma.get("morpheus_key"), "declared": lemma.get("declared"),
                       "qualifications": lemma["qualifications"],
                       "native_pos": counter(ledger[i]["native_pos"] for i in ids),
                       "vocabulary_status": counter(ledger[i]["status"] for i in ids), "ids": ids})
    malformed = collections.defaultdict(list)
    for n in printed:
        if n["native_pos"] != "u" and n["lemma"].get("raw_flags"):
            malformed[(n["raw_lemma"], ",".join(n["lemma"]["raw_flags"]), n["lemma"]["rule"],
                       ",".join(n["citation_flags"]))].append(n["id"])
    split_identities = collections.defaultdict(lambda: collections.Counter())
    for n in printed:
        if n["status"] == "eligible":
            split_identities[(n["lemma"]["key"], n["pos"]["upos"])][n["identity"]] += 1
    selected_set = {(r["key"], r["upos"]) for r in model if r["selected"]}
    splits = [{"key": k, "upos": u, "identities": {i: {"occurrences": c, "selected": (i, u) in selected_set}
                                                    for i, c in sorted(v.items())}}
              for (k, u), v in sorted(split_identities.items()) if len(v) > 1]
    eligible_total = sum(r["occurrences"] for r in model)
    selected_occ = sum(r["occurrences"] for r in model if r["selected"])
    chosen = [c for c in inputs_ if c["chosen"]]
    unresolved = sorted((n["id"] for n in printed if n["status"].startswith("unresolved")), key=lambda i: int(i[2:]))
    unsupported = [{"id": n["id"], "cts": n["cts"], "raw_lemma": n["raw_lemma"], "surface": n["surface"],
                    "native_pos": n["native_pos"], "upos": n["pos"]["upos"], "flags": n["lemma"]["raw_flags"]}
                   for n in sorted(printed, key=lambda o: int(o["id"][2:]))
                   if n["lemma"]["rule"] == "I-UNSUPPORTED-CITATION-FORM"]
    eimi_review = [{"id": i, "rule_upos": ledger[i]["pos"]["upos"], "rule": ledger[i]["pos"]["rule"],
                    "reviewer_reading": r, "reason": why,
                    "agrees": ledger[i]["pos"]["upos"] in r.split("|")}
                   for i, (r, why) in sorted(EIMI_REVIEWER_READINGS.items(), key=lambda kv: int(kv[0][2:]))]
    compound = compound_consistency(ledger)
    pairs = ctx["evidence"]["pairs"]
    function_adv = [r for r in ctx["selected"] if r["upos"] == "ADV" and any(
        pairs.get((letter, r["key"].split("#")[0]), {}) for letter in ("g", "c"))]
    name_like = [r for r in ctx["selected"] if r["upos"] == "NOUN" and all(
        ledger[i]["surface"][:1].isupper() for i in ctx["occurrences"][(r["key"], r["upos"])])]
    return {
        "version": VERSION, "rulesets": RULESETS,
        "status": "research interpretation; not source acceptance, production mapping, or certified predictions",
        "reproduce": "python3 -I opus_completion.py",
        "inputs": ctx["inputs"], "binding": ctx["binding"],
        "identity_layer": {
            "verdict": "established-for-all-printed-lexical-nodes" if not unsupported
                       else "required-identity-unsupported-for-some-nodes",
            "unsupported_citation_forms": unsupported,
            "namespace": {"scheme": DISCRIMINATOR_SCHEME, "meaning": "Perseus Morpheus lemma-key homograph number",
                          "pin": v2.MORPHEUS_PIN},
            "rule_counts_printed_non_punctuation": counter(n["lemma"]["rule"] for n in printed if n["native_pos"] != "u"),
            "numbered_labels": labels,
            "numbered_occurrences": sum(lab["occurrences"] for lab in labels),
            "malformed_raw_lemma_predictions": [
                {"raw_lemma": a, "raw_flags": b, "rule": c, "decoded_headword_flags": d, "ids": ids}
                for (a, b, c, d), ids in sorted(malformed.items())],
            "split_identities_same_headword_and_upos": splits,
        },
        "pos_layer": {
            "verdict": "established-for-all-printed-nodes" if not any(n["status"] == "unresolved-pos" for n in printed) else "unresolved-nodes-remain",
            "ud_perseus": {"tokens": ctx["evidence"]["tokens"], "single_valued_letters": ctx["evidence"]["letters"],
                           "context_letters": ctx["evidence"]["context_letters"], "aux_lemma_relation": ctx["evidence"]["aux"]},
            "rule_counts": counter(n["pos"]["rule"] for n in printed),
            "upos_counts": counter(n["pos"]["upos"] or "|".join(n["pos"]["candidates"]) for n in printed),
            "pair_unattested_by_rule": counter(n["pos"]["rule"] for n in printed if n["pos"]["pair_attested"] == 0),
            "eimi": counter(n["pos"]["rule"] for n in printed if n["pos"]["rule"].startswith("P-V-EIMI")),
            "eimi_ids_by_rule": {r: sorted((n["id"] for n in printed if n["pos"]["rule"] == r), key=lambda i: int(i[2:]))
                                 for r in sorted({n["pos"]["rule"] for n in printed
                                                  if n["pos"]["rule"].startswith("P-V-EIMI")
                                                  and n["pos"]["rule"] != "P-V-EIMI-COP-PNOM"})},
            "eimi_reviewer_comparison": eimi_review,
        },
        "denominators": {
            "native_nodes": len(ledger), "empty_unprinted": sum(1 for n in ledger.values() if n["empty"]),
            "printed": len(printed), "punctuation": sum(1 for n in printed if n["native_pos"] == "u"),
            "vocabulary_status": counter(n["status"] for n in printed),
            "non_vocabulary_by_upos": counter(n["pos"]["upos"] or "|".join(n["pos"]["candidates"])
                                              for n in printed if n["status"] == "non-vocabulary" and n["native_pos"] != "u"),
            "unresolved_ids": unresolved,
        },
        "sentences": {**length_profile(sentences), "native_group_check": {
            k: v for k, v in ctx["group_check"].items() if k != "empty_nodes_by_sentence"},
            "sentences_with_empty_nodes": len(ctx["group_check"]["empty_nodes_by_sentence"]),
            "word_internal_space_nodes": ctx["splits"],
            "sentences_with_word_internal_spaces": sum(1 for s in sentences if s["word_internal_spaces"]),
            "sentences_with_editorial_elements": {str(k): v for k, v in sorted(ctx["editorial"].items())},
            "completeness": counter(s["completeness"] for s in sentences)},
        "selection": {
            "known": 0, "reserved": 0, "corrections": 0, "exclusions": 0,
            "identities": len(model), "identities_by_upos": counter(r["upos"] for r in model),
            "selected": len(ctx["selected"]), "selected_by_upos": counter(r["upos"] for r in ctx["selected"]),
            "eligible_occurrences": eligible_total, "selected_occurrences": selected_occ,
            "selected_occurrence_coverage": round(selected_occ / eligible_total, 4) if eligible_total else None,
            "known_token_coverage": 0.0, "unknown_identities": len(model),
            "selected_with_discriminator": sum(1 for r in ctx["selected"] if "#" in r["key"]),
            "top_identities": [{"key": r["key"], "upos": r["upos"], "occurrences": r["occurrences"]}
                               for r in sorted(model, key=lambda r: (-r["occurrences"], r["key"]))[:12]],
            "go_probe_expected_stdout": ctx["probe_expected"],
            "selection_model_sha256": hashlib.sha256(selection_tsv(model).encode("utf-8")).hexdigest(),
        },
        "card_inputs": {
            "selected": len(inputs_), "eligible": len(chosen),
            "omitted": [{"key": c["key"], "upos": c["upos"], "reasons": c["omission"],
                         "ids": [r["id"] for r in c["rejections"]]} for c in inputs_ if c["omission"]],
            "using_later_occurrence": sum(1 for c in chosen if c["earlier_rejections"]),
            "earlier_rejection_reasons": counter(r for c in chosen for rej in c["earlier_rejections"] for r in rej["reasons"]),
            "chosen_sentence_units": length_profile([sentences[c["sentence"]] for c in chosen])["units"] if chosen else None,
            "distinct_chosen_sentences": len({c["sentence"] for c in chosen}),
            "chosen_flags": {
                "sentence_has_word_internal_space": sum(1 for c in chosen if sentences[c["sentence"]]["word_internal_spaces"]),
                "sentence_has_editorial_element": sum(1 for c in chosen if sentences[c["sentence"]]["editorial"]),
                "lowercased_name_like_target": sum(1 for c in chosen if ledger[c["chosen"]]["surface"][:1].isupper()
                                                   and ledger[c["chosen"]]["pos"]["upos"] == "NOUN"),
                "at_most_5_units": sum(1 for c in chosen if sentences[c["sentence"]]["units"] <= 5)},
        },
        "required_gates": gate_report(ctx["gates"], unsupported, printed),
        "comparison_scenarios": ctx["sensitivity"],
        "practical_quality_diagnostics": {
            "selected_ADV_whose_lemma_UD_Perseus_also_tags_g_or_c": {
                "identities": len(function_adv), "occurrences": sum(r["occurrences"] for r in function_adv),
                "keys": [r["key"] for r in function_adv]},
            "selected_NOUN_always_capitalized_in_book": {
                "identities": len(name_like), "occurrences": sum(r["occurrences"] for r in name_like)},
        },
        "prediction_diagnostics": {
            "compound_eimi_consistency": counter(r["verdict"] for r in compound),
            "compound_eimi_inconsistent": [r for r in compound if r["verdict"] == "inconsistent"],
            "stem_class_consistency": stem_class_consistency(ledger),
            "gorman_training_lineage": gorman_alignment([n for n in ctx["nodes"] if not n["empty"]]),
        },
        "examples_present": {k: v is not None for k, v in examples(
            ledger, sentences, inputs_, ctx["splits"], ctx["editorial"], ctx["group_check"]).items()},
        "verdicts": verdicts(unsupported, inputs_),
    }


def gate_report(gates: dict, unsupported: list[dict], printed: list[dict]) -> dict:
    """Integrity gates raise before this point; the lexical gates are reported here."""
    pos_unresolved = [n["id"] for n in printed if n["status"] == "unresolved-pos"]
    return {**{name: {"pass": True, **detail} for name, detail in sorted(gates.items())},
            "canonical_pos_determinate": {"pass": not pos_unresolved, "unresolved_ids": pos_unresolved},
            "required_identity_usable": {"pass": not unsupported,
                                         "unsupported_ids": [u["id"] for u in unsupported]},
            "vocabulary_ready_accounting": {"pass": not unsupported and not pos_unresolved}}


def verdicts(unsupported: list[dict], inputs_: list[dict]) -> dict:
    ready = not unsupported
    return {
        "binding": "verified baseline oga-anabasis1-native-binding/1; join, coverage and nesting gates rechecked",
        "representation_identity": ("established under oga-greta-lemma/2" if ready else
                                    f"{len(unsupported)} printed nodes have no usable citation representation; "
                                    "required identity unsupported for them"),
        "representation_pos": ("determinate under oga-agdt-upos/2: producer-conversion-compatible lexical classes; "
                               "εἰμί AUX/VERB from UD definitions over native syntax"),
        "sentences": "complete under oga-reading-terminal/2; native groups nest",
        "vocabulary_ready": "pass" if ready else "blocked by unsupported required identities",
        "dry_run": ("measured" if ready else "conditional: unsupported nodes counted as unresolved, not as vocabulary")
                   + "; real Go and independent model compared at runtime",
        "card_inputs": f"{sum(1 for c in inputs_ if c['chosen'])} of {len(inputs_)} selected identities (dry run)",
        "prediction_quality": "measured concerns, not certified; see prediction and practical-quality diagnostics",
        "unperformed": "Gloss, translation, dictionary coverage, Anki/client rendering, human example review",
    }


def private_texts(ctx: dict) -> dict[str, str]:
    ledger, sentences, text = ctx["ledger"], ctx["sentences"], ctx["text"]

    def dump(obj) -> str:
        return json.dumps(obj, ensure_ascii=False, indent=1, sort_keys=True) + "\n"

    sentence_rows = [{**s, "text": text[s["start"]:s["end"]]} for s in sentences]
    found = examples(ledger, sentences, ctx["inputs_"], ctx["splits"], ctx["editorial"], ctx["group_check"])
    case_rows = {name: (dict(case) if case else None) for name, case in found.items()}
    for case in case_rows.values():
        node_id = (case or {}).get("chosen") or (case or {}).get("node")
        if node_id:
            node = ledger[node_id]
            case["sentence_text"] = text[sentences[node["sentence"]]["start"]:sentences[node["sentence"]]["end"]]
            case["target"] = {"id": node_id, "surface": node["surface"], "span": node["reading_span"],
                              "headword": node["lemma"].get("headword"), "identity": node["identity"],
                              "upos": node["pos"]["upos"], "cts": node["cts"]}
    return {
        "node-ledger.json": dump([ledger[k] for k in sorted(ledger, key=lambda i: (i[0], int(i[2:])))]),
        "sentences.json": dump(sentence_rows),
        "selection-model.tsv": selection_tsv(ctx["model"]),
        "card-inputs.json": dump(ctx["inputs_"]),
        "examples.json": dump(case_rows),
        "analysis.json": ctx["analysis"],
        "compound-consistency.json": dump(compound_consistency(ledger)),
    }


# --- Go parity, comparison and write guard -------------------------------------------------------


def run_go_probe(analysis: str, expected_tsv: str, expected_stdout: str) -> list[str]:
    """Run Mouseion's selection code on the analysis; return parity problems."""
    go = shutil.which("go") or "/usr/local/go/bin/go"
    if not Path(go).exists():
        return ["go toolchain not found; parity unverified"]
    SCRATCH.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix="go-parity-", dir=SCRATCH))
    try:
        (work / "analysis.json").write_text(analysis, encoding="utf-8")
        env = dict(os.environ, GOTMPDIR=os.environ.get("GOTMPDIR", str(Path.home() / ".cache/gotmp")))
        Path(env["GOTMPDIR"]).mkdir(parents=True, exist_ok=True)
        done = subprocess.run([go, "run", PROBE, str(work / "analysis.json"), str(work / "go.tsv")],
                              cwd=REPO, env=env, capture_output=True, text=True, check=False)
        if done.returncode != 0:
            return [f"go probe failed: {done.stderr.strip()[:400]}"]
        problems = []
        if (work / "go.tsv").read_text(encoding="utf-8") != expected_tsv:
            problems.append("go selection output differs from the Python model")
        if done.stdout.strip() != expected_stdout:
            problems.append(f"go statistics differ: {done.stdout.strip()}")
        return problems
    finally:
        shutil.rmtree(work, ignore_errors=True)


def reproduction_problems(committed: str | None, computed: str, private: dict, out_dir: Path) -> list[str]:
    problems = []
    if committed is None:
        problems.append("committed summary missing; regenerate with --write and review it")
    elif committed != computed:
        problems.append(f"summary differs in keys {v2.diff_keys(committed, computed) or ['formatting']}")
    for name, text in sorted(private.items()):
        path = Path(out_dir) / name
        if not path.is_file():
            problems.append(f"private output missing: {name}")
        elif path.read_text(encoding="utf-8") != text:
            problems.append(f"private output differs from computed: {name}")
    return problems


def guard_write_targets(paths: list[Path]) -> None:
    """Only the new summary and files under private/opus-completion/out/ may be written."""
    out_root = OUT_DIR.resolve()
    for path in paths:
        resolved = Path(path).resolve()
        if resolved != SUMMARY.resolve() and out_root not in resolved.parents:
            raise SystemExit(f"refusing to write {path}: not an opus-completion output")


def run(write: bool, use_go: bool) -> int:
    summary, private = build()
    summary_text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
    if write:
        guard_write_targets([SUMMARY] + [OUT_DIR / name for name in private])
        OUT_DIR.mkdir(parents=True, exist_ok=True)
        for name, text in private.items():
            (OUT_DIR / name).write_text(text, encoding="utf-8")
        SUMMARY.write_text(summary_text, encoding="utf-8")
        print(f"wrote {SUMMARY.name} and {len(private)} private outputs under {OUT_DIR}")
        return 0
    committed = SUMMARY.read_text(encoding="utf-8") if SUMMARY.is_file() else None
    problems = reproduction_problems(committed, summary_text, private, OUT_DIR)
    if use_go:
        problems += run_go_probe(private["analysis.json"], private["selection-model.tsv"],
                                 summary["selection"]["go_probe_expected_stdout"])
    else:
        problems.append("go parity not run (--no-go)")
    print(json.dumps({"identity": summary["identity_layer"]["verdict"], "pos": summary["pos_layer"]["verdict"],
                      "selected": summary["selection"]["selected"],
                      "card_inputs": summary["verdicts"]["card_inputs"],
                      "reproduction_problems": problems}, ensure_ascii=False, sort_keys=True))
    return 1 if problems else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--write", action="store_true", help="regenerate the summary and private outputs")
    parser.add_argument("--no-go", action="store_true", help="skip the Go selection probe")
    args = parser.parse_args(argv)
    return run(write=args.write, use_go=not args.no_go)


if __name__ == "__main__":
    sys.exit(main())
