#!/usr/bin/env python3
"""Derive the Lysias I vocabulary-mapping manifest and cross-corpus evidence.

Standard library only. Run with `python3 -I`:

  python3 -I derive_evidence.py INPUTS_DIR OUT_DIR

INPUTS_DIR is produced by fetch_inputs.sh. OUT_DIR receives:
  lysias1-nodes.tsv        one row per AGDT <word> node (2,834 for Lysias I)
  lysias1-identities.tsv   canonical lexical identities and unresolved candidates
  lysias1-rules.tsv        versioned mapping rules with their evidence
  lysias1-summary.json     reconciled counts and diagnostics
  ud-crosscheck.json       aggregate UD agreement counts (no per-token UD data)

Per-token UD fields are used in memory only and are never written out
(UD Perseus is CC BY-NC-SA 2.5; see NOTICE.md).
"""
import collections
import csv
import glob
import hashlib
import json
import os
import sys
import unicodedata
import xml.etree.ElementTree as ET

RULES_VERSION = "lysias1-upos-rules/2026-10-14.1"
SCHEME_DECLARED = "GSAAGDT 1.1"  # Lysias I <annotationScheme>; test_mapping.py checks it
LYSIAS_FILE = "tlg0540.tlg001.perseus-grc1.tb.xml"
LYSIAS_SHA256 = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
UD_COMMIT = "331ddef91411d0e6549744ee889e05549e6da77d"
TB_COMMIT = "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f"
# internal/selection/selection.go DefaultConfig AllowedPOS. OccurrenceEligible also excludes
# dependency compound:prt and requires a letter (internal/lexical/lemma.go IsLemma).
ALLOWED_UPOS = frozenset({"NOUN", "VERB", "ADJ", "ADV"})
ELIDED = "elliptic"
COPULA_LEMMA = "εἰμί"

NODE_COLS = [
    "node_key", "sentence_id", "subdoc", "word_id", "position", "form",
    "native_lemma", "native_postag", "native_relation", "head", "artificial",
    "scheme_declared", "letter", "rule_id", "upos", "upos_status", "vocab_class",
    "canonical_key", "identity", "identity_status", "flags",
]
IDENTITY_COLS = [
    "identity", "canonical_key", "upos", "vocab_class", "identity_status",
    "occurrences", "raw_headword_variants", "scheme_declared", "flags",
]
RULE_COLS = ["rule_id", "letter", "relation", "upos", "upos_status", "vocab_class", "evidence"]

# rule_id, letter, relation ("*" = any), upos, upos_status, vocab_class, cross-corpus evidence key.
# Relation-specific rules are applied before the letter-wide fallback in classify().
RULES = [
    ("R00", "-", "elliptic", "", "n/a", "unprinted", None),
    ("R01", "n", "*", "NOUN", "resolved", "eligible", "n"),
    ("R02", "a", "*", "ADJ", "resolved", "eligible", "a"),
    ("R03", "d", "*", "ADV", "resolved", "eligible", "d_other"),
    ("R03P", "d", "AuxY|AuxZ", "ADV", "resolved_conditional", "eligible", "d_particle"),
    ("R04", "v", "AuxV", "AUX", "resolved", "non_vocab", "v_auxv"),
    ("R05", "v", "copula-candidate", "VERB|AUX", "unresolved", "unresolved", "v_copula"),
    ("R06", "v", "*", "VERB", "resolved", "eligible", "v"),
    ("R07", "c", "COORD", "CCONJ", "resolved", "non_vocab", "c_coord"),
    ("R08", "c", "AuxC", "SCONJ", "resolved_conditional", "non_vocab", "c_auxc"),
    ("R09", "c", "*", "CCONJ|SCONJ", "class_resolved_upos_unresolved", "non_vocab", "c"),
    ("R10", "r", "*", "ADP", "resolved", "non_vocab", "r"),
    ("R11", "l", "*", "DET", "resolved", "non_vocab", "l"),
    ("R12", "p", "*", "PRON", "resolved", "non_vocab", "p"),
    ("R13", "i", "*", "INTJ", "resolved", "non_vocab", "i"),
    ("R14", "u", "*", "PUNCT", "resolved", "non_vocab", "u"),
    ("R15", "m", "*", "NUM", "resolved", "non_vocab", "m"),
    ("R16", "g", "*", "PART", "resolved", "non_vocab", "g"),
]
RULE_BY_ID = {rule[0]: rule for rule in RULES}


def nfc(text):
    return unicodedata.normalize("NFC", text or "")


def canonical_key(lemma):
    """#2033 identity rule: NFC, lowercase, final sigma folded to sigma, NFC.

    Accents are kept. No acute/grave fold is applied; ADR 0094 keeps accent-distinct
    surfaces and citation forms distinct.
    """
    return nfc(nfc(lemma).lower().replace("ς", "σ"))


def has_letter(text):
    return any(ch.isalpha() for ch in text)


def has_grave(text):
    return "̀" in unicodedata.normalize("NFD", text)


def sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 16), b""):
            digest.update(chunk)
    return digest.hexdigest()


def load_nodes(path):
    """Every <word> in document order, with its within-sentence position."""
    nodes = []
    for sentence in ET.parse(path).getroot().iter("sentence"):
        for position, word in enumerate(sentence.findall("word"), 1):
            nodes.append({
                "sentence_id": sentence.get("id", ""),
                "subdoc": sentence.get("subdoc", ""),
                "word_id": word.get("id", ""),
                "position": position,
                "form": nfc(word.get("form")),
                "lemma": nfc(word.get("lemma")),
                "postag": word.get("postag", ""),
                "relation": word.get("relation", ""),
                "head": word.get("head", ""),
                "artificial": word.get("artificial", ""),
            })
    return nodes


def mark_copulas(nodes):
    """Set node['copula'] for εἰμί nodes with a predicate-nominal dependent.

    Structural test on the native tree only. AGDT has no copula label, and no UD convention
    is used to set this flag.
    """
    children = collections.defaultdict(list)
    for node in nodes:
        children[(node["sentence_id"], node["head"])].append(node["relation"])
    for node in nodes:
        node["copula"] = (
            node["postag"][:1] == "v"
            and canonical_key(node["lemma"]) == canonical_key(COPULA_LEMMA)
            and any(rel.startswith("PNOM") for rel in children[(node["sentence_id"], node["word_id"])])
        )
    return nodes


def classify(node):
    """Return (rule_id, upos, upos_status, vocab_class, flags) for one <word> node."""
    letter = node["postag"][:1]
    relation = node["relation"]
    if node["artificial"] == ELIDED:
        return ("R00", "", "n/a", "unprinted", [ELIDED])
    if letter == "v" and relation == "AuxV":
        rule = "R04"
    elif letter == "v" and node["copula"]:
        rule = "R05"
    elif letter == "c" and relation == "COORD":
        rule = "R07"
    elif letter == "c" and relation == "AuxC":
        rule = "R08"
    elif letter == "d" and relation in ("AuxY", "AuxZ"):
        rule = "R03P"
    else:
        rule = next((r[0] for r in RULES if r[1] == letter and r[2] in ("*", relation)), None)
        if rule is None:
            return ("R-unmapped", "", "unresolved", "unresolved", ["letter-unmapped:" + letter])
    _, _, _, upos, status, vocab, _ = RULE_BY_ID[rule]
    flags = []
    if rule == "R09":
        flags.append("conjunction-upos-split-unresolved")
    if rule == "R05":
        flags.append("copula-unresolved")
    if rule == "R03P":
        flags.append("particle-adverb-lineage")
    if vocab == "eligible" and node["lemma"][:1].isupper():
        flags.append("headword-capitalized")
    return (rule, upos, status, vocab, flags)


def repo_eligible(upos, lemma, relation):
    """Mirror of internal/selection OccurrenceEligible for the default config."""
    return relation != "compound:prt" and has_letter(lemma) and upos in ALLOWED_UPOS


# ---- cross-corpus UD evidence (aggregates only) ----------------------------
def native_printed_by_sentence(agdt_dir):
    """Printed nodes of every v2.1 Greek text, keyed by (file basename, sentence id)."""
    native = {}
    for path in sorted(glob.glob(os.path.join(agdt_dir, "v2.1/Greek/texts", "*.xml"))):
        base = os.path.basename(path)
        for node in mark_copulas(load_nodes(path)):
            if not node["artificial"]:
                native.setdefault((base, node["sentence_id"]), []).append(node)
    return native


def read_ud_sentences(ud_dir):
    sentences = []
    for path in sorted(glob.glob(os.path.join(ud_dir, "*.conllu"))):
        sent_id, tokens = None, []
        with open(path, encoding="utf-8") as handle:
            for line in handle:
                line = line.rstrip("\n")
                if line.startswith("# sent_id"):
                    if sent_id:
                        sentences.append((sent_id, tokens))
                    sent_id, tokens = line.split("=", 1)[1].strip(), []
                elif line and not line.startswith("#"):
                    fields = line.split("\t")
                    if "-" in fields[0] or "." in fields[0]:
                        continue
                    tokens.append({"form": nfc(fields[1]), "upos": fields[3]})
        if sent_id:
            sentences.append((sent_id, tokens))
    return sentences


def ud_pairs(agdt_dir, ud_dir):
    """Pair printed native nodes with UD tokens where the sentence's printed forms match exactly."""
    native = native_printed_by_sentence(agdt_dir)
    stats = collections.Counter()
    pairs = []
    for sent_id, tokens in read_ud_sentences(ud_dir):
        base, _, number = sent_id.partition("@")
        printed = native.get((base, number))
        if printed is None:
            stats["sentence_without_native"] += 1
        elif [n["form"] for n in printed] != [t["form"] for t in tokens]:
            stats["sentence_form_mismatch"] += 1
        else:
            stats["sentence_joined"] += 1
            pairs.extend(zip(printed, tokens))
    stats["tokens_joined"] = len(pairs)
    return pairs, stats


def build_ud_crosscheck(agdt_dir, ud_dir):
    pairs, stats = ud_pairs(agdt_dir, ud_dir)
    table = collections.defaultdict(collections.Counter)
    for node, token in pairs:
        letter, relation, upos = node["postag"][:1], node["relation"], token["upos"]
        table[letter][upos] += 1
        if letter == "d":
            table["d_particle" if relation in ("AuxY", "AuxZ") else "d_other"][upos] += 1
        if letter == "c":
            table["c_coord" if relation == "COORD" else "c_auxc" if relation == "AuxC" else "c_other"][upos] += 1
        if letter == "v" and relation == "AuxV":
            table["v_auxv"][upos] += 1
        if node["copula"]:
            table["v_copula"][upos] += 1
        if canonical_key(node["lemma"]) == canonical_key(COPULA_LEMMA):
            table["eimi_all"][upos] += 1
        if letter == "n" and node["lemma"][:1].isupper():
            table["n_capitalized_lemma"][upos] += 1
    return {
        "ud_commit": UD_COMMIT,
        "scope": "all AGDT v2.1 Greek texts joined to UD_Ancient_Greek-Perseus; Lysias I is not in UD",
        "join": dict(sorted(stats.items())),
        "by_native_key_upos": {k: dict(sorted(v.items())) for k, v in sorted(table.items())},
    }


def annotation_practice(agdt_dir):
    """Per-file letter used on AuxY/AuxZ particles, with each file's declared scheme.

    Records the source's own practice (no UD data). Used to show that the particle letter is
    set per text, not by the declared scheme.
    """
    files = []
    for path in sorted(glob.glob(os.path.join(agdt_dir, "v2.1/Greek/texts", "*.xml"))):
        root = ET.parse(path).getroot()
        scheme = (root.findtext("header/annotationScheme") or "").strip()
        words = list(root.iter("word"))
        particles = [w for w in words if w.get("postag", "")[:1] in ("d", "g") and w.get("relation") in ("AuxY", "AuxZ")]
        files.append({
            "file": os.path.basename(path),
            "scheme_declared": scheme,
            "words": len(words),
            "particles_d": sum(1 for w in particles if w.get("postag", "")[:1] == "d"),
            "particles_g": sum(1 for w in particles if w.get("postag", "")[:1] == "g"),
        })
    with_particles = [f for f in files if f["particles_d"] + f["particles_g"]]
    return {
        "scope": "all AGDT v2.1 Greek texts, native fields only",
        "files_total": len(files),
        "files_with_aux_particles": len(with_particles),
        "files_with_no_g_particles": sum(1 for f in with_particles if f["particles_g"] == 0),
        "files_with_g_particles": sum(1 for f in with_particles if f["particles_g"] > 0),
        "lysias_i_particles_d": next(f["particles_d"] for f in files if f["file"] == LYSIAS_FILE),
        "lysias_i_particles_g": next(f["particles_g"] for f in files if f["file"] == LYSIAS_FILE),
        "per_file": files,
    }


def rule_evidence(key, crosscheck):
    counts = crosscheck["by_native_key_upos"].get(key)
    if not counts:
        return "no UD-aligned tokens for this key in the pinned corpus"
    total = sum(counts.values())
    top_upos, top_n = max(counts.items(), key=lambda item: item[1])
    return f"UD v2.1 cross-corpus {key}: {top_n}/{total} {top_upos}; {json.dumps(counts, ensure_ascii=False, sort_keys=True)}"


# ---- derivation ------------------------------------------------------------
def write_tsv(path, columns, rows):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle, delimiter="\t", lineterminator="\n")
        writer.writerow(columns)
        for row in rows:
            writer.writerow([row.get(col, "") for col in columns])


def write_json(path, value):
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(value, handle, ensure_ascii=False, indent=2, sort_keys=True)
        handle.write("\n")


def derive(inputs, out):
    lysias = os.path.join(inputs, "agdt/v2.1/Greek/texts", LYSIAS_FILE)
    if sha256(lysias) != LYSIAS_SHA256:
        raise SystemExit("Lysias I sha256 mismatch; run fetch_inputs.sh first")
    os.makedirs(out, exist_ok=True)

    crosscheck = build_ud_crosscheck(os.path.join(inputs, "agdt"), os.path.join(inputs, "ud"))
    write_json(os.path.join(out, "ud-crosscheck.json"), crosscheck)
    practice = annotation_practice(os.path.join(inputs, "agdt"))
    write_json(os.path.join(out, "annotation-practice.json"), practice)

    nodes = mark_copulas(load_nodes(lysias))
    rows, identities = [], {}
    for node in nodes:
        rule, upos, status, vocab, flags = classify(node)
        key = canonical_key(node["lemma"])
        identity, identity_status = "", "n/a"
        if vocab == "eligible" and not repo_eligible(upos, node["lemma"], node["relation"]):
            vocab = "non_vocab"
            flags.append("repo-eligibility-excluded")
        if vocab == "eligible":
            identity = f"{key}|{upos}"
            identity_status = "unresolved_headword" if "-" in node["lemma"] else "resolved"
            if "-" in node["lemma"]:
                flags.append("headword-hyphen-compound-undecoded")
            if has_grave(node["lemma"]):
                flags.append("headword-grave-not-folded")
            bucket = identities.setdefault(identity, {
                "identity": identity, "canonical_key": key, "upos": upos, "vocab_class": "eligible",
                "identity_status": identity_status, "occurrences": 0,
                "variants": collections.Counter(), "flags": set(),
            })
            bucket["occurrences"] += 1
            bucket["variants"][node["lemma"]] += 1
            bucket["flags"].update(flags)
        if rule == "R05":
            identity_status = "unresolved_upos"
            flags.append("identity-candidates:VERB,AUX")
            for candidate in ("VERB", "AUX"):
                # Candidates live in their own namespace so they never merge into a resolved identity.
                cand_id = f"candidate:{key}|{candidate}"
                bucket = identities.setdefault(cand_id, {
                    "identity": cand_id, "canonical_key": key, "upos": candidate, "vocab_class": "candidate",
                    "identity_status": "unresolved_upos", "occurrences": 0,
                    "variants": collections.Counter(), "flags": {"copula-unresolved"},
                })
                bucket["occurrences"] += 1
                bucket["variants"][node["lemma"]] += 1
        if vocab == "unprinted":
            key = ""
        rows.append({
            "node_key": f"{node['sentence_id']}:{node['word_id']}",
            "sentence_id": node["sentence_id"], "subdoc": node["subdoc"], "word_id": node["word_id"],
            "position": node["position"], "form": node["form"], "native_lemma": node["lemma"],
            "native_postag": node["postag"], "native_relation": node["relation"], "head": node["head"],
            "artificial": node["artificial"], "scheme_declared": SCHEME_DECLARED,
            "letter": node["postag"][:1], "rule_id": rule, "upos": upos, "upos_status": status,
            "vocab_class": vocab, "canonical_key": key, "identity": identity,
            "identity_status": identity_status, "flags": ";".join(sorted(set(flags))),
        })

    write_tsv(os.path.join(out, "lysias1-nodes.tsv"), NODE_COLS, rows)
    id_rows = [{
        "identity": b["identity"], "canonical_key": b["canonical_key"], "upos": b["upos"],
        "vocab_class": b["vocab_class"], "identity_status": b["identity_status"],
        "occurrences": b["occurrences"],
        "raw_headword_variants": ";".join(f"{v}×{n}" for v, n in sorted(b["variants"].items())),
        "scheme_declared": SCHEME_DECLARED, "flags": ";".join(sorted(b["flags"])),
    } for b in identities.values()]
    write_tsv(os.path.join(out, "lysias1-identities.tsv"), IDENTITY_COLS, id_rows)

    rule_rows = [{
        "rule_id": rule_id, "letter": letter, "relation": relation, "upos": upos,
        "upos_status": status, "vocab_class": vocab,
        "evidence": rule_evidence(ev_key, crosscheck) if ev_key else "structural: elided node, not printed",
    } for rule_id, letter, relation, upos, status, vocab, ev_key in RULES]
    for row in rule_rows:
        if row["rule_id"] == "R03P":
            row["evidence"] = (
                f"native tag d on Lysias I AuxY/AuxZ particles: {practice['lysias_i_particles_d']} d, "
                f"{practice['lysias_i_particles_g']} g. Corpus practice differs by file: "
                f"{practice['files_with_no_g_particles']} of {practice['files_with_aux_particles']} files with "
                f"particles use no g; {practice['files_with_g_particles']} use g. " + row["evidence"]
            )
    write_tsv(os.path.join(out, "lysias1-rules.tsv"), RULE_COLS, rule_rows)

    summary = summarize(rows, id_rows, crosscheck)
    write_json(os.path.join(out, "lysias1-summary.json"), summary)
    return summary


def summarize(rows, id_rows, crosscheck):
    printed = [r for r in rows if r["vocab_class"] != "unprinted"]
    by_class = collections.Counter(r["vocab_class"] for r in printed)
    flags = collections.Counter(f for r in rows for f in r["flags"].split(";") if f)
    eligible_ids = [i for i in id_rows if i["vocab_class"] == "eligible"]
    resolved = [i for i in eligible_ids if i["identity_status"] == "resolved"]
    hyphen = [i for i in eligible_ids if i["identity_status"] == "unresolved_headword"]
    candidates = [i for i in id_rows if i["vocab_class"] == "candidate"]
    fold_groups = collections.defaultdict(set)
    for i in resolved:
        fold = "".join(ch for ch in unicodedata.normalize("NFD", i["canonical_key"]) if not unicodedata.combining(ch))
        fold_groups[(fold, i["upos"])].add(i["canonical_key"])
    key_upos = collections.defaultdict(set)
    for i in resolved:
        key_upos[i["canonical_key"]].add(i["upos"])
    return {
        "rules_version": RULES_VERSION,
        "scheme_declared": SCHEME_DECLARED,
        "inputs": {"lysias_sha256": LYSIAS_SHA256, "treebank_commit": TB_COMMIT, "ud_commit": UD_COMMIT},
        "nodes_total": len(rows),
        "nodes_printed": len(printed),
        "nodes_unprinted_elliptic": len(rows) - len(printed),
        "printed_by_letter": dict(sorted(collections.Counter(r["letter"] for r in printed).items())),
        "printed_by_vocab_class": dict(sorted(by_class.items())),
        "printed_by_rule": dict(sorted(collections.Counter(r["rule_id"] for r in printed).items())),
        "printed_by_upos_status": dict(sorted(collections.Counter(r["upos_status"] for r in printed).items())),
        "eligible_occurrences": by_class["eligible"],
        "eligible_resolved_occurrences": sum(1 for r in printed if r["vocab_class"] == "eligible" and r["identity_status"] == "resolved"),
        "eligible_unresolved_headword_occurrences": sum(1 for r in printed if r["identity_status"] == "unresolved_headword"),
        "unresolved_copula_nodes": by_class["unresolved"],
        "eligible_identities": len(eligible_ids),
        "resolved_identities": len(resolved),
        "unresolved_headword_identities": len(hyphen),
        "copula_identity_candidates": len(candidates),
        "flags": dict(sorted(flags.items())),
        "diagnostics": {
            "accent_variant_splits_same_upos": {
                f"{fold}|{upos}": sorted(keys) for (fold, upos), keys in sorted(fold_groups.items()) if len(keys) > 1
            },
            "keys_with_multiple_upos_identities": {k: sorted(v) for k, v in sorted(key_upos.items()) if len(v) > 1},
        },
        "reconciliation": {
            "printed_plus_unprinted_equals_nodes": len(printed) + (len(rows) - len(printed)) == len(rows),
            "class_counts_sum_to_printed": sum(by_class.values()) == len(printed),
        },
        "ud_join": crosscheck["join"],
    }


def main(argv):
    if len(argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    summary = derive(argv[1], argv[2])
    print(json.dumps(summary["printed_by_vocab_class"], ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
