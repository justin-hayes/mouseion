#!/usr/bin/env python3
"""Numbered-lemma and canonical POS mapping completion for OGA Anabasis Book 1.

Research-only companion to ``ancient-greek-oga-anabasis1-import-proof.md``
(issue #2052). It reuses the retained, verified Book 1 binding in ``private/``
and tests two required layers that the earlier lexical stop could not decide:

* **identity**: each numbered raw lemma (``λέγω3``) is compared with the full
  key index of the pinned Morpheus Greek stem library. A key is resolved only
  when exactly one producer derivation or lexical record carries that
  headword-plus-digit key. Digits are never stripped or inherited.
* **canonical POS**: each printed native POS (AGDT) is given the UPOS candidates
  the retained AGDT guidelines and UD Perseus notes support. A node is resolved
  only when its candidates are a single UPOS or all lie outside the default
  ``AllowedPOS`` (``internal/selection/selection.go``). Any candidate set that
  mixes an eligible and an ineligible UPOS, or that has two eligible UPOS, is
  reported as blocked with its rule id.

Subcommands::

    python3 -I mapping_completion.py extract --morpheus DIR   # full key index (private)
    python3 -I mapping_completion.py run                      # offline reproduction

``run`` needs only retained private inputs and the index; it verifies the index
hash, writes private ledgers and the tracked summary, and calls no network
service, model, dictionary, or Greek provider.
"""

from __future__ import annotations

import argparse
import collections
import hashlib
import json
import re
import sys
import unicodedata
from pathlib import Path

HERE = Path(__file__).resolve().parent
PRIVATE = HERE / "private"
MAP_DIR = PRIVATE / "mapping-completion"
INDEX = MAP_DIR / "morpheus-index.tsv"
MANIFEST = MAP_DIR / "morpheus-manifest.json"
NATIVE_NODES = PRIVATE / "import" / "native-nodes.json"
CONLLU = (
    PRIVATE
    / "archive-members/workspace/conllu/conllu"
    / "tlg0032.tlg006.perseus-grc2.tok01_sentence-seg01_annotated_lemma.conllu"
)
LEDGER_OUT = MAP_DIR / "mapping-ledger.json"
IDENTITY_OUT = MAP_DIR / "identity-ledger.json"
SUMMARY_OUT = HERE / "mapping-completion-summary.json"
MORPHEUS_PIN = "b1b33c56ef2338fe0dcd1893628ed638f00c0986"
# SHA-256 of the extracted index, recorded after `extract` against MORPHEUS_PIN.
INDEX_SHA256 = "57f81c8627c654411f185f2c2d491a958c8acdd5684400fe3c1696ebb2b9d5c2"
# Default AllowedPOS from internal/selection/selection.go (NewSelectionConfig).
ALLOWED_POS = frozenset({"NOUN", "VERB", "ADJ", "ADV"})
# Spelled prefix-compound whose numbered OGA label has no producer key.
NEAR_MISS = {"σύνλέγω2": {"prefix": "su/n", "base": "le/gw2"}}

# --- TLG beta code -> Greek (lemma keys only) -------------------------------

_LETTERS = {
    "a": "α", "b": "β", "g": "γ", "d": "δ", "e": "ε", "z": "ζ", "h": "η",
    "q": "θ", "i": "ι", "k": "κ", "l": "λ", "m": "μ", "n": "ν", "c": "ξ",
    "o": "ο", "p": "π", "r": "ρ", "s": "σ", "t": "τ", "u": "υ", "f": "φ",
    "x": "χ", "y": "ψ", "w": "ω",
}
_DIACRITICS = {
    ")": "̓", "(": "̔", "/": "́", "\\": "̀",
    "=": "͂", "+": "̈", "|": "ͅ",
}


class BetaError(ValueError):
    """Raised for any beta-code construct this decoder does not define."""


def decode_beta(key: str) -> tuple[str, str]:
    """Return ``(headword, discriminator)`` for a Morpheus lemma key.

    The trailing digits are a discriminator, not part of the spelling. Diacritics
    attach to the immediately preceding letter, which places diphthong marks on
    the second vowel as Morpheus writes them. Unknown characters fail closed.
    """
    match = re.fullmatch(r"(.*?)(\d*)", key)
    body, digits = match.group(1), match.group(2)
    out: list[str] = []
    capitalise = False
    pending = ""  # diacritics written before a capital ("*)ikma" -> "Ἴκμα")
    for index, char in enumerate(body):
        if char == "*":
            if capitalise:
                raise BetaError(f"doubled capital marker in {key!r}")
            capitalise = True
            continue
        if char in _LETTERS:
            letter = _LETTERS[char]
            if char == "s" and (index + 1 == len(body) or not body[index + 1].isalpha()):
                letter = "ς"
            if capitalise:
                letter = letter.upper()
                capitalise = False
            out.append(letter + pending)
            pending = ""
            continue
        if char in _DIACRITICS:
            if capitalise or not out:
                pending += _DIACRITICS[char]
                continue
            out[-1] += _DIACRITICS[char]
            continue
        raise BetaError(f"unsupported beta character {char!r} in {key!r}")
    if not out or pending or capitalise:
        raise BetaError(f"incomplete key {key!r}")
    headword = unicodedata.normalize("NFC", "".join(out))
    return norm_head(headword), digits


def norm_head(text: str) -> str:
    """Lower-case NFC with a word-final sigma, as the accepted profile requires."""
    text = unicodedata.normalize("NFC", text).lower()
    if text.endswith("σ"):
        text = text[:-1] + "ς"
    return text


def bare(text: str) -> str:
    """Accent-, breathing-, and final-sigma-insensitive key for matching only."""
    text = unicodedata.normalize("NFD", norm_head(text))
    text = "".join(c for c in text if not unicodedata.combining(c))
    return text.replace("ς", "σ")


# --- retained inputs --------------------------------------------------------


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def numbered_labels(nodes: list[dict]) -> dict[str, dict]:
    """Group printed numbered raw lemmas by their raw string."""
    labels: dict[str, dict] = {}
    for node in nodes:
        raw = node["raw_lemma"]
        match = re.fullmatch(r"(.+?)(\d+)", raw or "")
        if not match or node["empty"]:
            continue
        entry = labels.setdefault(
            raw,
            {"raw_lemma": raw, "headword": norm_head(match.group(1)),
             "discriminator": match.group(2), "occurrences": []},
        )
        entry["occurrences"].append(node["id"])
    return labels


def load_conllu() -> dict[str, list[str]]:
    """Token rows keyed by node id; each row gets its sentence number appended.

    CoNLL-U head indices are sentence-local, so sentence boundaries (a new
    token index 1) are recorded and used to scope every head lookup.
    """
    rows: dict[str, list[str]] = {}
    sentence = 0
    with CONLLU.open(encoding="utf-8") as handle:
        for line in handle:
            line = line.rstrip("\n")
            if not line or line.startswith("#"):
                continue
            cols = line.split("\t")
            if cols[0] == "1":
                sentence += 1
            rows[cols[9]] = cols + [str(sentence)]
    return rows


# --- full Morpheus key index (private) ---------------------------------------


def index_rows(morpheus: Path) -> tuple[list[dict], collections.Counter]:
    """Every key-bearing record in every stemsrc file, with its file and line.

    Classes: ``lemlist`` (one key per line), ``le-block`` (``:le:`` key),
    ``compound`` (``vbs.cmp.ml``: derivation then output), and ``lsj-compound``
    (``vbs.cmp.lsj``: output then derivation). Undecodable tokens are counted,
    never silently dropped.
    """
    stem_dir = morpheus / "stemlib" / "Greek" / "stemsrc"
    rows: list[dict] = []
    skipped: collections.Counter = collections.Counter()
    for path in sorted(stem_dir.iterdir()):
        if not path.is_file():
            continue
        lines = path.read_text(encoding="utf-8", errors="replace").splitlines()
        for number, raw in enumerate(lines, 1):
            text = raw.strip()
            if not text:
                continue
            if path.name == "lemlist":
                rows.append({"file": path.name, "line": number, "class": "lemlist",
                             "key": text, "derivation": ""})
            elif text.startswith(":le:"):
                rows.append({"file": path.name, "line": number, "class": "le-block",
                             "key": text[4:].strip(), "derivation": ""})
            elif path.name == "vbs.cmp.lsj" and len(text.split()) == 2:
                out_key, derivation = text.split()
                rows.append({"file": path.name, "line": number, "class": "lsj-compound",
                             "key": out_key, "derivation": derivation})
            elif path.name == "vbs.cmp.ml" and len(text.split()) == 2:
                derivation, out_key = text.split()
                rows.append({"file": path.name, "line": number, "class": "compound",
                             "key": out_key, "derivation": derivation})
            else:
                continue
    for row in rows:
        try:
            decode_beta(row["key"])
        except BetaError:
            skipped[row["class"]] += 1
    return rows, skipped


def write_index(rows: list[dict]) -> None:
    body = ["file\tline\tclass\tkey\tderivation"]
    body += [f"{r['file']}\t{r['line']}\t{r['class']}\t{r['key']}\t{r['derivation']}" for r in rows]
    INDEX.write_text("\n".join(body) + "\n", encoding="utf-8")


def load_index() -> list[dict]:
    """Decoded index rows. Keys beta cannot decode exactly are retried loosely.

    A loose row (hyphenated or Latin-script name) is flagged and never resolves
    an identity; it only makes the matching headword conservatively unresolved.
    Rows that remain undecodable get ``head`` None and are ignored for matching.
    """
    rows = []
    for line in INDEX.read_text(encoding="utf-8").splitlines()[1:]:
        file, number, klass, key, derivation = line.split("\t")
        row = {"file": file, "line": int(number), "class": klass, "key": key,
               "derivation": derivation, "head": None, "headword": None, "digits": None,
               "starred": key.startswith("*"), "loose": False}
        try:
            head, digits = decode_beta(key)
        except BetaError:
            try:
                head, digits = decode_beta(re.sub(r"[^A-Za-z*()/\\=+|0-9]", "", key))
                row["loose"] = True
            except BetaError:
                rows.append(row)
                continue
        row.update({"head": bare(head), "headword": head, "digits": digits})
        rows.append(row)
    return rows


# --- identity layer ----------------------------------------------------------


def identity_decision(label: dict, index: list[dict]) -> dict:
    """Classify one numbered raw lemma against the full Morpheus key index."""
    head = bare(label["headword"])
    digits = label["discriminator"]
    same_head = [r for r in index if r["head"] == head]
    loose_exact = [r for r in same_head if r["digits"] == digits and r["loose"]]
    exact = [r for r in same_head if r["digits"] == digits and not r["loose"]]
    lexical = [r for r in exact if r["class"] in ("lemlist", "le-block")]
    derivations = collections.defaultdict(list)
    for r in exact:
        if r["class"] in ("compound", "lsj-compound"):
            derivations[r["derivation"]].append(r)
    witnesses = [
        {"file": r["file"], "line": r["line"], "class": r["class"], "key": r["key"],
         "derivation": r["derivation"]}
        for r in exact
    ]
    base = {"raw_lemma": label["raw_lemma"], "headword": label["headword"],
            "discriminator": digits, "occurrences": len(label["occurrences"]),
            "exact_witnesses": witnesses,
            "loose_witnesses": [{"file": r["file"], "line": r["line"], "key": r["key"]}
                                for r in loose_exact],
            "same_headword_discriminators": sorted({r["digits"] for r in same_head if r["digits"]}),
            "same_headword_keys": sorted({r["key"] for r in same_head})}
    if loose_exact:
        decision, reason = ("not-established-loose-record",
                            "a producer record with this headword and digit is not decodable exactly")
    elif exact:
        if len(derivations) >= 2:
            decision, reason = ("ambiguous-shared-key",
                                f"same key from {len(derivations)} distinct producer derivations")
        elif lexical or derivations:
            decision, reason = "key-resolved", "one producer key with this headword and digit"
        else:  # pragma: no cover - exact implies lexical or derivation
            decision, reason = "not-established", "unclassified exact record"
    elif same_head:
        decision, reason = ("digit-absent-in-pinned-source",
                            "headword present in pinned source, but not with this digit")
    else:
        near = NEAR_MISS.get(label["raw_lemma"])
        near_rows = []
        if near:
            near_rows = [
                {"file": r["file"], "line": r["line"], "key": r["key"],
                 "derivation": r["derivation"]}
                for r in index
                if r["derivation"].split("-", 1)[0] == near["prefix"]
                and r["derivation"].split("-", 1)[-1] == near["base"]
            ]
        if near_rows:
            decision, reason = ("not-established-no-key",
                                "the spelled prefix-compound derivation has no digit-bearing output key")
            base["near_derivations"] = near_rows
        else:
            decision, reason = ("headword-absent-in-pinned-source",
                                "no producer key or derivation with this headword")
    base["decision"] = decision
    base["reason"] = reason
    base["derivations"] = {d: [{"file": r["file"], "line": r["line"], "key": r["key"]}
                               for r in rows] for d, rows in sorted(derivations.items())}
    return base


# --- canonical POS layer -----------------------------------------------------


def upos_status(candidates: list[str]) -> str:
    """Single UPOS, or only ineligible UPOS, is resolved; anything else is blocked."""
    eligible = [c for c in candidates if c in ALLOWED_POS]
    if len(candidates) == 1:
        return "resolved-eligible" if eligible else "resolved-outside-allowed-pos"
    if not eligible:
        return "resolved-outside-allowed-pos"
    return "blocked"


def pos_decision(native: str, deprel: str, lemma: str, has_pnom_child: bool,
                 propn: str) -> dict:
    """Map one printed AGDT POS and relation to UPOS candidates with a rule id.

    ``propn`` is the Morpheus proper-name evidence for the lemma: ``proper``,
    ``common``, or ``both-or-neither`` (see ``propn_evidence``).
    """
    def out(candidates: list[str], rule: str, reason: str) -> dict:
        return {"candidates": candidates, "status": upos_status(candidates),
                "rule": rule, "reason": reason}

    if native == "u":
        return out(["PUNCT"], "pos-u-punctuation", "AGDT 2.11; UD PUNCT")
    if native == "i":
        return out(["INTJ"], "pos-i-interjection", "AGDT 2.10; UD INTJ")
    if native == "l":
        return out(["DET"], "pos-l-article", "AGDT 2.1 article; UD DET")
    if native == "p":
        return out(["PRON", "DET"], "pos-p-pronoun",
                   "AGDT 2.3 pronoun; UD PRON or DET, both outside AllowedPOS")
    if native == "a":
        if deprel in {"ADV", "AuxY", "AuxZ"}:
            return out(["ADJ", "ADV"], "pos-a-adverb-status-conflict",
                       "AGDT 2.6: adjective with adverb status is annotated as adverb, "
                       "but native POS is a and the function is adverbial")
        return out(["ADJ"], "pos-a-adjective", "AGDT 2.3 adjective; UD ADJ")
    if native == "n":
        if deprel == "ADV":
            return out(["NOUN", "ADV"], "pos-n-adverb-status-conflict",
                       "AGDT 2.6: noun with adverb status is annotated as adverb, "
                       "but native POS is n and the function is adverbial")
        if propn == "proper":
            return out(["PROPN"], "pos-n-proper-morpheus-evidence",
                       "Morpheus starred (proper-name) key only; UD PROPN")
        if propn == "common":
            return out(["NOUN"], "pos-n-common-morpheus-evidence",
                       "Morpheus unstarred key only; UD NOUN")
        return out(["NOUN", "PROPN"], "pos-n-proper-undecided",
                   "Morpheus proper/common evidence is mixed or absent")
    if native == "v":
        if lemma == "εἰμί" and has_pnom_child:
            return out(["AUX"], "pos-v-copula-aux",
                       "copula εἰμί with PNOM dependent; UD Perseus changelog: copulae VERB→AUX")
        return out(["VERB"], "pos-v-verb", "AGDT 2.5 verb; UD VERB (non-copula)")
    if native == "d":
        if deprel in {"ADV", "ATR", "OBJ", "OCOMP", "PNOM", "AuxY", "AuxZ"}:
            return out(["ADV"], "pos-d-adverb",
                       "AGDT 2.6 adverb, including sentence adverbs/particles tagged as adverbs; "
                       "UD ADV")
        if deprel in {"COORD", "AuxC", "AuxP"}:
            return out(["ADV", "CCONJ" if deprel == "COORD" else
                        ("SCONJ" if deprel == "AuxC" else "ADP")],
                       "pos-d-function-conflict",
                       f"AGDT POS d (adverb) with conjunction/adposition function {deprel}")
        return out(["ADV", "PART"], "pos-d-unrecognised-relation",
                   f"relation {deprel!r} not in the consulted AGDT tables")
    if native == "c":
        if deprel == "COORD":
            return out(["CCONJ"], "pos-c-coordinating", "AGDT 3.9 COORD; UD CCONJ")
        if deprel == "AuxC":
            return out(["SCONJ"], "pos-c-subordinating", "AGDT 3.13 AuxC; UD SCONJ")
        if deprel == "AuxY":
            return out(["CCONJ"], "pos-c-auxy-coordinating",
                       "AGDT 3.17: AuxY on a non-final conjunction of a multi-conjunction")
        if deprel == "ADV":
            return out(["SCONJ", "ADV"], "pos-c-adverb-function-conflict",
                       "AGDT POS c (conjunction) with adverbial function ADV")
        return out(["CCONJ", "SCONJ"], "pos-c-unrecognised-relation",
                   f"relation {deprel!r} not in the consulted AGDT tables")
    if native == "r":
        if deprel == "AuxP":
            return out(["ADP"], "pos-r-adposition", "AGDT 3.12 AuxP; UD ADP")
        return out(["ADP", "SCONJ"], "pos-r-adposition-conjunction",
                   f"AGDT POS r with relation {deprel!r}; both ADP and SCONJ are outside AllowedPOS")
    return out([native.upper()], "pos-unknown-native", f"no rule for native POS {native!r}")


def propn_evidence(lemma: str, index_by_bare: dict[str, list[dict]]) -> str:
    """Morpheus proper-name evidence for one lemma: starred keys mark proper names."""
    rows = index_by_bare.get(bare(lemma), [])
    if any(r["loose"] for r in rows):
        return "both-or-neither"
    starred = any(r["starred"] for r in rows)
    unstarred = any(not r["starred"] for r in rows)
    if starred and not unstarred:
        return "proper"
    if unstarred and not starred:
        return "common"
    return "both-or-neither"


# --- pipeline ----------------------------------------------------------------


def run() -> dict:
    if sha256(INDEX) != INDEX_SHA256:
        raise SystemExit("morpheus index hash mismatch; re-run extract against the pin")
    nodes = json.loads(NATIVE_NODES.read_text(encoding="utf-8"))
    conllu = load_conllu()
    index = load_index()
    by_bare: dict[str, list[dict]] = collections.defaultdict(list)
    for r in index:
        if r["head"] is not None:
            by_bare[r["head"]].append(r)

    gate = {"nodes": len(nodes), "aligned": 0, "mismatch": []}
    for node in nodes:
        cols = conllu.get(node["id"])
        if node["empty"]:
            gate["aligned"] += 1
            continue
        same = (cols is not None and cols[1] == node["surface"]
                and cols[2] == node["raw_lemma"] and cols[3] == node["native_morphology"]["pos"])
        if same:
            gate["aligned"] += 1
        else:
            gate["mismatch"].append(node["id"])
    if gate["mismatch"]:
        raise SystemExit(f"native/conllu alignment failed: {gate['mismatch'][:5]}")

    labels = numbered_labels(nodes)
    identity = {raw: identity_decision(entry, index) for raw, entry in sorted(labels.items())}

    # Copula evidence: which tokens have a PNOM dependent.
    pnom_heads = {(cols[10], cols[6]) for cols in conllu.values() if cols[7] == "PNOM"}

    per_node = []
    for node in nodes:
        if node["empty"]:
            per_node.append({"id": node["id"], "cts": node["cts"], "status": "empty-unprinted"})
            continue
        cols = conllu[node["id"]]
        native = node["native_morphology"]["pos"]
        deprel = cols[7]
        lemma = node["raw_lemma"]
        # The spelling is the headword: a numbered label's digit is a discriminator.
        spelling = re.sub(r"\d+$", "", lemma)
        evidence = propn_evidence(spelling, by_bare) if native == "n" else "n/a"
        has_pnom = (cols[10], cols[0]) in pnom_heads
        decision = pos_decision(native, deprel, spelling, has_pnom, evidence)
        entry = {
            "id": node["id"], "cts": node["cts"], "surface": node["surface"],
            "raw_lemma": lemma, "native_pos": native, "deprel": deprel,
            "pos_candidates": decision["candidates"], "pos_status": decision["status"],
            "pos_rule": decision["rule"], "propn_evidence": evidence,
        }
        if lemma in identity:
            entry["identity_decision"] = identity[lemma]["decision"]
        per_node.append(entry)

    printed = [n for n in per_node if n.get("status") != "empty-unprinted"]
    pos_status = collections.Counter(n["pos_status"] for n in printed)
    rule_table: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    blocked_pos = collections.defaultdict(list)
    for n in printed:
        rule_table[n["pos_rule"]][n["pos_status"]] += 1
        if n["pos_status"] == "blocked":
            blocked_pos[n["pos_rule"]].append(n["id"])
    numbered = [n for n in printed if n["raw_lemma"] in identity]
    identity_status = collections.Counter(identity[n["raw_lemma"]]["decision"] for n in numbered)
    blocked_identity = {
        raw: {"occurrences": identity[raw]["occurrences"], "decision": identity[raw]["decision"]}
        for raw in identity if identity[raw]["decision"] != "key-resolved"
    }
    blocked_identity_ids = sorted(n["id"] for n in numbered
                                  if identity[n["raw_lemma"]]["decision"] != "key-resolved")

    MAP_DIR.mkdir(parents=True, exist_ok=True)
    LEDGER_OUT.write_text(json.dumps(per_node, ensure_ascii=False, indent=1, sort_keys=True) + "\n",
                          encoding="utf-8")
    IDENTITY_OUT.write_text(json.dumps(list(identity.values()), ensure_ascii=False, indent=1,
                                       sort_keys=True) + "\n", encoding="utf-8")
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    skipped = manifest.get("undecodable_keys_skipped", {})

    identity_pass = not blocked_identity_ids
    pos_pass = not any(blocked_pos.values())
    summary = {
        "version": "mapping-completion/2",
        "morpheus_pin": MORPHEUS_PIN,
        "morpheus_index_sha256": INDEX_SHA256,
        "morpheus_manifest": manifest,
        "inputs": {
            "native_nodes_sha256": sha256(NATIVE_NODES),
            "conllu_sha256": sha256(CONLLU),
            "index_records": len(index),
        },
        "gates": {
            "native_nodes": gate["nodes"],
            "aligned_with_conllu_surface_lemma_pos": gate["aligned"],
            "empty_nodes_unprinted": gate["nodes"] - len(printed),
        },
        "identity_layer": {
            "result": "passed" if identity_pass else "failed",
            "distinct_numbered_labels": len(identity),
            "numbered_occurrences": len(numbered),
            "occurrence_decisions": dict(sorted(identity_status.items())),
            "blocked_occurrences": len(blocked_identity_ids),
            "blocked_occurrence_ids": blocked_identity_ids,
            "labels": [
                {k: v for k, v in identity[raw].items() if k != "derivations"}
                | {"derivations": identity[raw]["derivations"]}
                for raw in sorted(identity)
            ],
        },
        "pos_layer": {
            "result": "passed" if pos_pass else "failed",
            "printed_nodes": len(printed),
            "status_totals": dict(sorted(pos_status.items())),
            "rules": {r: dict(sorted(c.items())) for r, c in sorted(rule_table.items())},
            "blocked_by_rule": {r: {"count": len(ids), "ids": sorted(ids)}
                                for r, ids in sorted(blocked_pos.items())},
            "allowed_pos_default": sorted(ALLOWED_POS),
        },
        "downstream": "stopped-before-selection-sentence-cap-omission" if not (identity_pass and pos_pass)
        else "required layers passed; remaining checks to be run",
    }
    SUMMARY_OUT.write_text(json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n",
                           encoding="utf-8")
    return summary


def extract(morpheus: Path) -> int:
    """Write the full private key index and a manifest of every consulted file."""
    stem_dir = morpheus / "stemlib" / "Greek" / "stemsrc"
    rows, skipped = index_rows(morpheus)
    MAP_DIR.mkdir(parents=True, exist_ok=True)
    write_index(rows)
    files = {p.name: sha256(p) for p in sorted(stem_dir.iterdir()) if p.is_file()}
    MANIFEST.write_text(json.dumps({
        "pin": MORPHEUS_PIN,
        "stemsrc_files": files,
        "index_sha256": sha256(INDEX),
        "undecodable_keys_skipped": dict(sorted(skipped.items())),
        "records_by_class": dict(sorted(collections.Counter(r["class"] for r in rows).items())),
    }, indent=1, sort_keys=True) + "\n", encoding="utf-8")
    return len(rows)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[1])
    sub = parser.add_subparsers(dest="command", required=True)
    ext = sub.add_parser("extract", help="write the private Morpheus key index")
    ext.add_argument("--morpheus", required=True, type=Path)
    sub.add_parser("run", help="offline reproduction from retained inputs")
    args = parser.parse_args(argv)
    if args.command == "extract":
        count = extract(args.morpheus)
        print(f"wrote {count} key records to {INDEX}; index sha256 {sha256(INDEX)}")
        return 0
    summary = run()
    print(json.dumps({"identity": summary["identity_layer"]["result"],
                      "pos": summary["pos_layer"]["result"]}, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
