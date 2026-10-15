#!/usr/bin/env python3
"""Mapping-completion review revision (v2) for OGA Anabasis Book 1, issue #2052.

Research-only. It checks retained private inputs against pinned SHA-256 values,
recomputes exploratory identity candidates and canonical POS proposals, and
compares them with the committed v2 summary. It adopts no canonical identity
and no UPOS mapping. The eb2e9f70 evaluator (``mapping_completion.py``) is
historical and is not imported or run here.

    python3 -I mapping_completion_v2.py          # verify and compare; writes nothing
    python3 -I mapping_completion_v2.py --write  # regenerate v2 private outputs and summary

Identity matching: a candidate is an exact citation key match (NFC, lower case,
final sigma folded to sigma, every diacritic kept) on headword plus digits.
Accent-erased matches are listed as near witnesses only and never approve.
Loose-decoded records never approve.
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
V1_DIR = PRIVATE / "mapping-completion"  # v1 index and manifest; read-only for v2
INDEX = V1_DIR / "morpheus-index.tsv"
MANIFEST = V1_DIR / "morpheus-manifest.json"
PRODUCER_DIRS = (V1_DIR / "producer-docs", PRIVATE / "producer-docs")
PRODUCER_REGISTRY = HERE / "producer-docs.json"
NATIVE_NODES = PRIVATE / "import" / "native-nodes.json"
IMPORT_LEDGER = HERE / "import-ledger.json"
CONLLU = (
    PRIVATE / "archive-members/workspace/conllu/conllu"
    / "tlg0032.tlg006.perseus-grc2.tok01_sentence-seg01_annotated_lemma.conllu"
)
OUT_DIR = PRIVATE / "mapping-completion-v2"
LEDGER_OUT = OUT_DIR / "mapping-ledger-v2.json"
IDENTITY_OUT = OUT_DIR / "identity-ledger-v2.json"
SUMMARY = HERE / "mapping-completion-v2-summary.json"
MORPHEUS_PIN = "b1b33c56ef2338fe0dcd1893628ed638f00c0986"
PINS = {
    INDEX: "57f81c8627c654411f185f2c2d491a958c8acdd5684400fe3c1696ebb2b9d5c2",
    MANIFEST: "bb672f881d8bdb2c0db4783605b65aeaa4bc7fcb562c2ecf34fa764cf8b4568f",
    NATIVE_NODES: "2a5aca90602428b23f0f88cbcbfbd1ac787c87d25bd9bd87c67c7a29ae0f942b",
    CONLLU: "0f9cbba8e4fdf28ddcedc70259c18f3d00573bc9a82d3acc6a9a050d17542780",
}
# Standard UD UPOS tags (17). Candidate sets are checked against this closed set.
UPOS = frozenset({
    "ADJ", "ADP", "ADV", "AUX", "CCONJ", "DET", "INTJ", "NOUN", "NUM", "PART",
    "PRON", "PROPN", "PUNCT", "SCONJ", "SYM", "VERB", "X",
})
# AGDT POS letters that have a proposal rule in this pass. Others are unsupported.
KNOWN_NATIVE = frozenset({"a", "c", "d", "i", "l", "n", "p", "r", "u", "v"})
# Spelled prefix-compound whose numbered OGA label has no producer key.
NEAR_MISS = {"σύνλέγω2": {"prefix": "su/n", "base": "le/gw2"}}
NEAR_LIMIT = 20

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
    """Return ``(headword, digits)``; the trailing digits are a discriminator.

    Every diacritic is kept exactly as written. Unknown characters fail closed.
    """
    match = re.fullmatch(r"(.*?)(\d*)", key)
    body, digits = match.group(1), match.group(2)
    out: list[str] = []
    capitalise = False
    pending = ""  # diacritics written before a capital ("*)ikma")
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
    return unicodedata.normalize("NFC", "".join(out)), digits


def citation_key(text: str) -> str:
    """Exact matching key: NFC, lower case, final sigma folded to sigma.

    This is the only equivalence used for identity. Diacritics are not removed.
    It is not the accepted lemma profile, which this pass does not apply.
    """
    return unicodedata.normalize("NFC", text).lower().replace("ς", "σ")


def bare(text: str) -> str:
    """Accent-erased key. Used only to list near witnesses, never to approve."""
    decomposed = unicodedata.normalize("NFD", citation_key(text))
    return "".join(c for c in decomposed if not unicodedata.combining(c))


# --- retained inputs --------------------------------------------------------


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with Path(path).open("rb") as handle:
        for block in iter(lambda: handle.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def check_pin(path: Path, expected: str) -> None:
    actual = sha256(path)
    if actual != expected:
        raise SystemExit(f"pin mismatch for {Path(path).name}: got {actual}")


def verify_producer_docs() -> dict:
    """Check every registry entry against the retained file of the same name."""
    registry = json.loads(PRODUCER_REGISTRY.read_text(encoding="utf-8"))
    report = {}
    for name, record in sorted(registry.items()):
        found = [d / name for d in PRODUCER_DIRS if (d / name).is_file()]
        if not found:
            raise SystemExit(f"producer document missing: {name}")
        check_pin(found[0], record["sha256"])
        report[name] = {"sha256": record["sha256"], "bytes": record["bytes"], "url": record["url"]}
    return report


def verify_binding(nodes: list[dict], ledger: dict) -> dict:
    """Counts must match the retained import ledger, which is the binding authority."""
    computed = {
        "native_nodes": len(nodes),
        "empty_nodes": sum(1 for n in nodes if n["empty"]),
        "printed_nodes": sum(1 for n in nodes if not n["empty"]),
    }
    binding = ledger["binding"]
    for key, value in computed.items():
        if binding.get(key) != value:
            raise SystemExit(f"binding count mismatch: {key} is {value}, ledger has {binding.get(key)}")
    printed = collections.Counter(n["native_morphology"]["pos"] for n in nodes if not n["empty"])
    if dict(sorted(printed.items())) != ledger["native_pos_printed_counts"]:
        raise SystemExit("native POS counts differ from the import ledger")
    return computed


def load_conllu() -> dict[str, list[str]]:
    """Token rows keyed by node id (MISC column); each row gets its sentence number."""
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


def align(nodes: list[dict], conllu: dict[str, list[str]]) -> dict:
    """Printed nodes are joined to CoNLL-U rows; empty nodes are counted separately."""
    printed = [n for n in nodes if not n["empty"]]
    empty = [n for n in nodes if n["empty"]]
    joined = aligned = 0
    mismatches: list[str] = []
    for node in printed:
        cols = conllu.get(node["id"])
        if cols is None:
            mismatches.append(node["id"])
            continue
        joined += 1
        if (cols[1] == node["surface"] and cols[2] == node["raw_lemma"]
                and cols[3] == node["native_morphology"]["pos"]):
            aligned += 1
        else:
            mismatches.append(node["id"])
    empty_with_row = sum(1 for n in empty if n["id"] in conllu)
    node_ids = {n["id"] for n in nodes}
    outside = sum(1 for k in conllu if k not in node_ids)
    return {
        "printed": len(printed), "printed_joined": joined, "printed_aligned": aligned,
        "mismatches": mismatches, "empty": len(empty), "empty_with_conllu_row": empty_with_row,
        "conllu_rows": len(conllu),
        # Rows whose id is not a native node; scope of these rows is not checked in this pass.
        "conllu_rows_without_native_node": outside,
    }


def numbered_labels(nodes: list[dict]) -> dict[str, list[str]]:
    """Printed numbered raw lemmas (``λέγω3``) mapped to their node ids."""
    labels: dict[str, list[str]] = collections.defaultdict(list)
    for node in nodes:
        raw = node["raw_lemma"]
        if node["empty"] or not raw or not re.fullmatch(r"(.+?)(\d+)", raw):
            continue
        labels[raw].append(node["id"])
    return dict(labels)


def label_for(raw: str, occurrences: list[str] | tuple[str, ...] = ()) -> dict:
    match = re.fullmatch(r"(.+?)(\d+)", raw)
    if not match:
        raise ValueError(f"not a numbered lemma: {raw!r}")
    return {"raw_lemma": raw, "headword": unicodedata.normalize("NFC", match.group(1)),
            "discriminator": match.group(2), "occurrences": list(occurrences)}


# --- full Morpheus key index (private, read-only) ----------------------------


def parse_row(file: str, line: int, klass: str, key: str, derivation: str) -> dict:
    """One producer record. Undecodable keys keep ``head`` None and are never matched."""
    row = {"file": file, "line": line, "class": klass, "key": key, "derivation": derivation,
           "starred": key.startswith("*"), "loose": False, "headword": None, "head": None,
           "bare": None, "digits": None, "cite": None}
    try:
        headword, digits = decode_beta(key)
    except BetaError:
        try:
            headword, digits = decode_beta(re.sub(r"[^A-Za-z*()/\\=+|0-9]", "", key))
        except BetaError:
            return row
        row["loose"] = True
    row.update(headword=headword, head=citation_key(headword), bare=bare(headword), digits=digits)
    row["cite"] = row["head"] + digits
    return row


def load_index_rows() -> list[dict]:
    rows = []
    for text in INDEX.read_text(encoding="utf-8").splitlines()[1:]:
        file, number, klass, key, derivation = text.split("\t")
        rows.append(parse_row(file, int(number), klass, key, derivation))
    return rows


def index_maps(rows: list[dict]) -> tuple[dict, dict]:
    by_head: dict[str, list[dict]] = collections.defaultdict(list)
    by_bare: dict[str, list[dict]] = collections.defaultdict(list)
    for row in rows:
        if row["head"] is None:
            continue
        by_head[row["head"]].append(row)
        by_bare[row["bare"]].append(row)
    return dict(by_head), dict(by_bare)


# --- identity layer (exploratory candidates; nothing is adopted) -------------


def _witness(row: dict) -> dict:
    return {"file": row["file"], "line": row["line"], "class": row["class"],
            "key": row["key"], "derivation": row["derivation"]}


def identity_decision(label: dict, by_head: dict, by_bare: dict, rows: list[dict]) -> dict:
    """Classify one numbered lemma. Every outcome has ``canonical_identity`` not-adopted."""
    cite = citation_key(label["headword"])
    digits = label["discriminator"]
    same = by_head.get(cite, [])
    exact = [r for r in same if r["digits"] == digits and not r["loose"]]
    loose = [r for r in same if r["digits"] == digits and r["loose"]]
    other_digits = sorted({r["digits"] for r in same if r["digits"] != digits and not r["loose"]})
    near = [r for r in by_bare.get(bare(label["headword"]), []) if r["head"] != cite]
    origins = sorted({r["derivation"] for r in exact})
    if exact and len(origins) >= 2:
        decision = "source-key-collision"
        reason = f"same exact key from {len(origins)} distinct producer derivations or records"
    elif exact and loose:
        decision = "exact-plus-loose-conflict-not-established"
        reason = "an exact candidate exists beside a loose-decoded record with the same digit"
    elif exact:
        decision = "candidate-match-under-unvalidated-hypothesis"
        reason = "one exact key with this headword and digit; the digit correspondence is unvalidated"
    elif loose:
        decision = "loose-record-only-no-approval"
        reason = "only a loosely decoded record matches; loose records never approve"
    elif other_digits:
        decision = "digit-absent-in-pinned-source"
        reason = "headword present in the pinned source, but not with this digit"
    elif near:
        decision = "no-exact-source-key"
        reason = "only accent-erased near witnesses; not identity evidence"
    else:
        decision = "headword-absent-in-pinned-source"
        reason = "no producer key or derivation with this headword"
    result = {
        "raw_lemma": label["raw_lemma"], "headword": label["headword"],
        "discriminator": digits, "occurrences": len(label["occurrences"]),
        "decision": decision, "reason": reason, "canonical_identity": "not-adopted",
        "exact_witnesses": [_witness(r) for r in exact],
        "loose_witnesses": [_witness(r) for r in loose],
        "collision_derivations": origins if decision == "source-key-collision" else [],
        "same_headword_other_digits": other_digits,
        "accent_erased_near_witness_count": len(near),
        "accent_erased_near_witnesses": [_witness(r) for r in near[:NEAR_LIMIT]],
    }
    near_miss = NEAR_MISS.get(label["raw_lemma"])
    if near_miss:
        result["near_derivations"] = [
            _witness(r) for r in rows
            if r["derivation"].split("-", 1)[0] == near_miss["prefix"]
            and r["derivation"].split("-", 1)[-1] == near_miss["base"]
        ]
    return result


def exact_evidence(headword: str, by_head: dict) -> str:
    """Morpheus proper-name evidence from undiscriminated, exact, non-loose keys."""
    rows = [r for r in by_head.get(citation_key(headword), [])
            if r["digits"] == "" and not r["loose"]]
    starred = any(r["starred"] for r in rows)
    unstarred = any(not r["starred"] for r in rows)
    if starred and unstarred:
        return "both"
    if starred:
        return "exact-starred-only"
    if unstarred:
        return "exact-unstarred-only"
    return "absent"


# --- canonical POS layer (proposals only; nothing is adopted) ----------------


def propose(native: str, deprel: str, lemma: str, has_pnom_child: bool, propn: str) -> dict:
    """Candidate UPOS set for one printed AGDT POS. Every result is a proposal.

    ``propn`` is ``exact-starred-only``, ``exact-unstarred-only``, ``both``, or
    ``absent`` (see ``exact_evidence``). An unknown native tag returns status
    ``unsupported-native-tag`` with no candidates, never an upper-cased guess.
    """
    def out(candidates: list[str], rule: str, note: str) -> dict:
        if not set(candidates) <= UPOS:
            raise ValueError(f"candidate outside UPOS: {candidates}")
        status = "proposal-single-candidate" if len(candidates) == 1 else "proposal-multi-candidate"
        return {"candidates": candidates, "status": status, "rule": rule, "note": note}

    if native not in KNOWN_NATIVE:
        return {"candidates": [], "status": "unsupported-native-tag", "rule": "unsupported",
                "note": f"no proposal rule for AGDT POS {native!r} in this pass"}
    if native == "u":
        return out(["PUNCT"], "pos-u-punctuation", "AGDT punctuation; UD PUNCT")
    if native == "i":
        return out(["INTJ"], "pos-i-interjection", "AGDT interjection; UD INTJ")
    if native == "l":
        return out(["DET"], "pos-l-article", "AGDT article; UD DET")
    if native == "p":
        return out(["PRON", "DET"], "pos-p-pronoun",
                   "AGDT pronoun; UD PRON or DET is not chosen by this pass")
    if native == "a":
        if deprel in {"ADV", "AuxY", "AuxZ"}:
            return out(["ADJ", "ADV"], "pos-a-adverbial-function",
                       "adjective with adverbial function; AGDT 2.6 adverb status is not established")
        return out(["ADJ"], "pos-a-adjective", "AGDT adjective; UD ADJ")
    if native == "n":
        if deprel == "ADV":
            return out(["NOUN", "ADV"], "pos-n-adverbial-function",
                       "noun with adverbial function; a syntactic relation, not lexical adverb status")
        if propn == "exact-starred-only":
            return out(["PROPN"], "pos-n-starred-morpheus-key",
                       "exact starred Morpheus key only; UD PROPN equivalence unestablished")
        if propn == "exact-unstarred-only":
            return out(["NOUN"], "pos-n-unstarred-morpheus-key",
                       "exact unstarred Morpheus key only; UD NOUN")
        return out(["NOUN", "PROPN"], "pos-n-proper-undecided",
                   "Morpheus proper/common evidence is mixed or absent")
    if native == "v":
        if citation_key(lemma) == citation_key("εἰμί") and has_pnom_child:
            return out(["AUX"], "pos-v-copula-pnom",
                       "copula with PNOM dependent; UD Perseus changelog only, not a validated rule")
        return out(["VERB"], "pos-v-verb", "AGDT verb; UD VERB")
    if native == "d":
        if deprel in {"ADV", "ATR", "OBJ", "OCOMP", "PNOM", "AuxY", "AuxZ"}:
            return out(["ADV"], "pos-d-adverb", "AGDT adverb; UD ADV for particles is unvalidated")
        if deprel in {"COORD", "AuxC", "AuxP"}:
            other = {"COORD": "CCONJ", "AuxC": "SCONJ", "AuxP": "ADP"}[deprel]
            return out(["ADV", other], "pos-d-function-mismatch",
                       f"AGDT adverb with function {deprel}; function is not a lexical POS test")
        return out(["ADV", "PART"], "pos-d-unrecognised-relation",
                   f"relation {deprel!r} is not in the consulted tables")
    if native == "c":
        if deprel == "COORD":
            return out(["CCONJ"], "pos-c-coordinating", "AGDT conjunction COORD; UD CCONJ")
        if deprel == "AuxC":
            return out(["SCONJ"], "pos-c-subordinating", "AGDT conjunction AuxC; UD SCONJ")
        if deprel == "AuxY":
            return out(["CCONJ"], "pos-c-auxy", "AGDT AuxY conjunction; UD CCONJ")
        if deprel == "ADV":
            return out(["SCONJ", "ADV"], "pos-c-adverbial-function",
                       "conjunction with adverbial function; not decided by this pass")
        return out(["CCONJ", "SCONJ"], "pos-c-unrecognised-relation",
                   f"relation {deprel!r} is not in the consulted tables")
    # native == "r"
    if deprel == "AuxP":
        return out(["ADP"], "pos-r-adposition", "AGDT preposition AuxP; UD ADP")
    return out(["ADP", "SCONJ"], "pos-r-adposition-other",
               f"AGDT preposition with relation {deprel!r} is not decided by this pass")


# --- reproduction and write guard ---------------------------------------------


def diff_keys(committed: str, computed: str) -> list[str]:
    old, new = json.loads(committed), json.loads(computed)
    return sorted(k for k in set(old) | set(new) if old.get(k) != new.get(k))


def reproduction_problems(committed: str | None, computed: str, expected: dict,
                          out_dir: Path) -> list[str]:
    problems = []
    if committed is None:
        problems.append("committed summary missing; regenerate with --write and review it")
    elif committed != computed:
        keys = diff_keys(committed, computed)
        problems.append(f"summary differs in keys {keys}" if keys
                        else "summary differs in formatting only")
    for name, text in sorted(expected.items()):
        path = Path(out_dir) / name
        if not path.is_file():
            problems.append(f"private output missing: {name}")
        elif path.read_text(encoding="utf-8") != text:
            problems.append(f"private output differs from computed: {name}")
    return problems


def guard_write_targets(paths: list[Path]) -> None:
    """Only the v2 summary and files named ``*v2*`` under the v2 output directory."""
    out_root = OUT_DIR.resolve()
    for path in paths:
        resolved = Path(path).resolve()
        allowed = resolved == SUMMARY.resolve() or (
            out_root in resolved.parents and "v2" in resolved.name)
        if not allowed:
            raise SystemExit(f"refusing to write {path}: not a v2 output")


# --- pipeline ----------------------------------------------------------------


def numbered_identity(nodes: list[dict], by_head: dict, by_bare: dict, rows: list[dict]) -> dict:
    labels = numbered_labels(nodes)
    return {raw: identity_decision(label_for(raw, ids), by_head, by_bare, rows)
            for raw, ids in sorted(labels.items())}


def build() -> tuple[dict, dict]:
    """Return ``(summary, private_texts)``; nothing is written here."""
    for path, digest in PINS.items():
        check_pin(path, digest)
    manifest = json.loads(MANIFEST.read_text(encoding="utf-8"))
    if manifest.get("pin") != MORPHEUS_PIN or manifest.get("index_sha256") != PINS[INDEX]:
        raise SystemExit("morpheus manifest does not match the pinned index")
    producers = verify_producer_docs()
    nodes = json.loads(NATIVE_NODES.read_text(encoding="utf-8"))
    binding = verify_binding(nodes, json.loads(IMPORT_LEDGER.read_text(encoding="utf-8")))
    conllu = load_conllu()
    alignment = align(nodes, conllu)
    if alignment["mismatches"]:
        raise SystemExit(f"native/conllu alignment failed: {alignment['mismatches'][:5]}")

    rows = load_index_rows()
    by_head, by_bare = index_maps(rows)
    undecodable = collections.Counter(r["class"] for r in rows if r["head"] is None)
    loose_rows = sum(1 for r in rows if r["loose"])
    identity = numbered_identity(nodes, by_head, by_bare, rows)

    pnom_heads = {(c[10], c[6]) for c in conllu.values() if c[7] == "PNOM"}
    ledger = []
    for node in nodes:
        if node["empty"]:
            ledger.append({"id": node["id"], "cts": node.get("cts"), "status": "empty-unprinted"})
            continue
        cols = conllu[node["id"]]
        native = node["native_morphology"]["pos"]
        lemma = node["raw_lemma"]
        spelled = re.sub(r"\d+$", "", lemma)
        evidence = exact_evidence(spelled, by_head) if native == "n" else "not-applicable"
        proposal = propose(native, cols[7], spelled, (cols[10], cols[0]) in pnom_heads, evidence)
        ledger.append({
            "id": node["id"], "cts": node.get("cts"), "surface": node["surface"],
            "raw_lemma": lemma, "native_pos": native, "deprel": cols[7],
            "pos_candidates": proposal["candidates"], "pos_status": proposal["status"],
            "pos_rule": proposal["rule"], "pos_note": proposal["note"],
            "propn_evidence": evidence,
            "identity_hypothesis": identity[lemma]["decision"] if lemma in identity else None,
        })
    printed = [n for n in ledger if "pos_status" in n]
    identity_occ = collections.Counter(
        identity[n["raw_lemma"]]["decision"] for n in printed if n["raw_lemma"] in identity)
    pos_status = collections.Counter(n["pos_status"] for n in printed)
    rules: dict[str, collections.Counter] = collections.defaultdict(collections.Counter)
    for n in printed:
        rules[n["pos_rule"]][",".join(n["pos_candidates"]) or "none"] += 1
    copula = collections.Counter(
        ",".join(n["pos_candidates"]) for n in printed if citation_key(n["raw_lemma"]) == citation_key("εἰμί"))
    unsupported = sorted({n["native_pos"] for n in printed if n["pos_status"] == "unsupported-native-tag"})

    identity_ledger = [{"raw_lemma": raw, **{k: v for k, v in identity[raw].items()
                                              if k != "raw_lemma"}} for raw in sorted(identity)]
    ledger_text = json.dumps(ledger, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
    identity_text = json.dumps(identity_ledger, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
    private = {LEDGER_OUT.name: ledger_text, IDENTITY_OUT.name: identity_text}

    summary = {
        "version": "mapping-completion/v2",
        "status": "exploratory-diagnostics; no canonical identity or UPOS mapping is adopted",
        "reproduce": "python3 -I mapping_completion_v2.py",
        "pins": {
            "morpheus_pin": MORPHEUS_PIN,
            "files": {p.name: digest for p, digest in sorted(PINS.items(), key=lambda kv: kv[0].name)},
            "producer_documents": producers,
        },
        "binding": binding,
        "alignment": alignment,
        "index": {
            "records": len(rows), "loose_decoded_records": loose_rows,
            "undecodable_records_by_class": dict(sorted(undecodable.items())),
            "records_by_class": manifest["records_by_class"],
        },
        "identity_layer": {
            "verdict": "not-established",
            "verdict_reason": ("no OGA numbering-to-Morpheus numbering correspondence is established "
                               "by the retained evidence; all candidates are unvalidated hypotheses"),
            "canonical_identity_adopted": 0,
            "distinct_numbered_labels": len(identity),
            "numbered_occurrences": sum(identity[r]["occurrences"] for r in identity),
            "occurrence_decisions": dict(sorted(identity_occ.items())),
            "labels": identity_ledger,
        },
        "pos_layer": {
            "verdict": "unsupported" if unsupported else "not-established",
            "verdict_reason": ("proposals only; no UPOS mapping is adopted, and candidate sets are "
                               "exploratory diagnostics, not blockers or verified exclusions"),
            "canonical_pos_adopted": 0,
            "printed_nodes": len(printed),
            "native_pos_counts": dict(sorted(collections.Counter(n["native_pos"] for n in printed).items())),
            "proposal_status_counts": dict(sorted(pos_status.items())),
            "unsupported_native_tags": unsupported,
            "rules": {r: dict(sorted(c.items())) for r, c in sorted(rules.items())},
            "copula_eimi_candidates": dict(sorted(copula.items())),
            "propn_evidence_counts": dict(sorted(collections.Counter(
                n["propn_evidence"] for n in printed if n["native_pos"] == "n").items())),
        },
        "private_outputs": {name: {"sha256": hashlib.sha256(text.encode("utf-8")).hexdigest(),
                                   "lines": text.count("\n")}
                            for name, text in sorted(private.items())},
        "verdicts": {
            "canonical_identity": "not-established",
            "canonical_pos": "unsupported" if unsupported else "not-established",
            "sentence_selection_cap_omission": "not-run; canonical layers are not established",
        },
    }
    return summary, private


def run(write: bool) -> int:
    summary, private = build()
    summary_text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
    if write:
        guard_write_targets([LEDGER_OUT, IDENTITY_OUT, SUMMARY])
        OUT_DIR.mkdir(parents=True, exist_ok=True)
        for name, text in private.items():
            (OUT_DIR / name).write_text(text, encoding="utf-8")
        SUMMARY.write_text(summary_text, encoding="utf-8")
        print(f"wrote {SUMMARY.name} and {len(private)} private v2 outputs under {OUT_DIR}")
        return 0
    committed = SUMMARY.read_text(encoding="utf-8") if SUMMARY.is_file() else None
    problems = reproduction_problems(committed, summary_text, private, OUT_DIR)
    print(json.dumps({"identity": summary["identity_layer"]["verdict"],
                      "pos": summary["pos_layer"]["verdict"],
                      "reproduction_problems": problems}, ensure_ascii=False, sort_keys=True))
    return 1 if problems else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[1])
    parser.add_argument("--write", action="store_true",
                        help="regenerate the v2 summary and private outputs (default: compare only)")
    args = parser.parse_args(argv)
    return run(write=args.write)


if __name__ == "__main__":
    sys.exit(main())
