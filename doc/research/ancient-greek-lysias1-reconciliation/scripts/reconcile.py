#!/usr/bin/env python3
"""Reconcile the #2048 binding evidence and the #2049 mapping evidence for AGDT Lysias I.

Usage: python3 -I reconcile.py <inputs-dir> <out-dir>

<inputs-dir> is the directory written by fetch_inputs.sh. The script:

1. joins all 2,834 native nodes one-to-one across AGDT v2.1, v2.0 and v1.6 (same pinned
   treebank commit), the binding manifest (dc3e5ed7) and the mapping manifest (50f4ad6c);
2. re-verifies every claimed text correspondence against the pinned grc2 running text;
3. evaluates the reconciliation rules (XR-*) defined in RULES below; and
4. assesses six text/annotation routes node by node and writes a readiness ledger.

Outputs are ASCII. Native strings appear only as U+XXXX code-point sequences, so raw
and NFC forms stay distinguishable (this is a technical choice, not a rights mechanism).
Nothing here selects vocabulary, measures card coverage or repairs Greek.
"""
import collections
import csv
import hashlib
import json
import os
import re
import sys
import unicodedata
import xml.etree.ElementTree as ET

SCHEMA = "lysias1-reconciliation/1"
BINDING_COMMIT = "dc3e5ed74681a24157d99316c25f2e18c141f748"
MAPPING_COMMIT = "50f4ad6ccb5df46daf603a14dc1d1393cc274c52"
TREEBANK_COMMIT = "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f"
CANON_COMMIT = "01b725d835e6e733062ffd79e0efdbae1ba06e5c"
CANON_HIST_COMMIT = "aa0dd9e6a25c848c04e75169e525ec413f033499"
DOC_V21 = f"treebank_data@{TREEBANK_COMMIT[:8]}:v2.1/Greek/texts/tlg0540.tlg001.perseus-grc1.tb.xml"
DOC_V16 = f"treebank_data@{TREEBANK_COMMIT[:8]}:v1.6/greek/data/tlg0540.tlg001.perseus-grc1.tb.xml"
DOC_CANON = f"canonical-greekLit@{CANON_COMMIT[:8]}:data/tlg0540/tlg001/tlg0540.tlg001.perseus-grc2.xml"

BIND = "evidence/binding/doc/research/ancient-greek-lysias1-binding"
MAP = "evidence/mapping/doc/research/ancient-greek-lysias1-vocabulary-mapping"

ELISION_V2 = "̓"     # COMBINING COMMA ABOVE, as carried by AGDT v2.0/v2.1
ELISION_V16 = "ʼ"    # MODIFIER LETTER APOSTROPHE, as carried by AGDT v1.6 and grc2

# Reconciliation rules. Each is evaluated per node; none is an accepted Mouseion mapping.
RULES = {
    "XR-ELISION-1": "v2.x node form = v1.6 form with its final U+02BC replaced by U+0313, all other "
                    "fields of the node equal, v2.0 = v2.1, and the v1.6 form equals the bound grc2 "
                    "token code point for code point. Evidence: C-ELISION (50f4ad6c), AGDT-V16-LYS, "
                    "AGDT-README-20, AGDT-HDR-LYS. A release mark mapping proven node by node, not a "
                    "comparison fold and not a Mouseion normalization profile.",
    "XR-SPAN-SHARED-1": "Two hyphen-notated component nodes share the whole span of one grc2 token when "
                        "(a) the preceding and following word nodes are exact bindings to the adjacent "
                        "word tokens, so the block holds exactly these two nodes and this one token, "
                        "(b) the node order is the printed order (fragment then host, the C-CRASIS notation "
                        "50f4ad6c attests 91 times in 11 files), and (c) the hyphen-stripped forms concatenate to the token in "
                        "node order. (c) is consistent evidence, not proof; the anchors carry the proof. "
                        "No component substring offsets are assigned.",
    "XR-DISC-1": "Splice rule for a v2.1-primary analysis: take the Morpheus homograph number from the "
                 "v1.6 lemma of the same node id when the v1.6 lemma equals the v2.1 lemma plus trailing "
                 "digits (after NFC) and the POS letter is equal. Field origins are kept separately.",
    "XR-V16-PRIMARY-1": "v1.6 as primary annotation: native lemma decoded by moving trailing Morpheus "
                        "digits into a morpheus-homograph discriminator (AGDT20-LEMNUM, HAMLEDT-LID). The "
                        "50f4ad6c UPOS/headword rules carry over only because every rule input (POS letter, "
                        "relation, head, artificial, decoded lemma) is verified identical node by node.",
}

ROUTES = {
    "T1A1": "grc2 @01b725d8 text + AGDT v2.1 annotation, input statuses as published (baseline)",
    "T1A1X": "grc2 @01b725d8 text + AGDT v2.1 annotation + versioned splice rules XR-ELISION-1, "
             "XR-DISC-1, XR-SPAN-SHARED-1",
    "T1A2": "grc2 @01b725d8 text + AGDT v1.6 as primary annotation (v2.1 retained as evidence); "
            "XR-V16-PRIMARY-1, XR-SPAN-SHARED-1",
    "T2A1": "annotation-carried text from AGDT v2.1 forms",
    "T2A2": "annotation-carried text from AGDT v1.6 forms",
    "T3A2": "historical grc1 @aa0dd9e6 text + AGDT v1.6 (op-level assessment only; no span manifest)",
}

BLOCKERS = {
    "BT-S011-ORDER": ("binding", "factual_gap",
                      "Anchors pin each pair to one grc2 token; the annotation puts the -te fragment before its host, "
                      "against the text and against the 6 corpus cases that put -X after its host (none outside Lysias I "
                      "precedes it). A shared span "
                      "would silently pick an interpretation the producer's notation contradicts.",
                      "Inspect the pinned source of the tokenizer named in the v1.6 header (perseids llt/segtok) for "
                      "its enclitic split order; no new witness is needed."),
    "BT-S125-VARIANT": ("binding", "route_inherent",
                        "Positional correspondence is anchored, but the pinned grc2 token differs by one breathing code "
                        "point; every consulted witness prints the annotation reading. No check can make the pinned "
                        "token equal; Lamb's print would only explain the grc2 reading.",
                        "None factual. Either an explicit variant-binding record is accepted for this non-vocabulary "
                        "node, or the T1 route stays non-exact here."),
    "BS-PUNCT-ANNOTATION-ONLY": ("segmentation", "explained",
                                 "Annotation punctuation without a grc2 counterpart: 5 rubric stops (printed by Hude) and "
                                 "the s002 AuxK mark deleted by b01744e8. Not vocabulary; affects sentence boundaries.",
                                 "None for vocabulary. A reviewed segmentation convention must say how these boundaries "
                                 "are drawn from the bound text."),
    "BT-ELISION-UNADOPTED": ("binding", "resolved_by_XR-ELISION-1", "Baseline only.", ""),
    "BT-CRASIS-CANDIDATE": ("binding", "resolved_by_XR-SPAN-SHARED-1", "Baseline only.", ""),
    "BI-DISC-CONFLATED": ("identity", "resolved_by_v1.6_numbers", "Baseline/v2.1 only.", ""),
    "BI-ELIG-PARTICLE": ("identity", "factual_gap_with_unperformed_checks",
                         "AGDT tags these d (adverb); UD grc PART/CCONJ are lexical classes. No documented native-to-UD "
                         "rule for these records; full Smyth particle list (2769-3003) not inspected.",
                         "Per distinct lemma, compare the pinned UD grc PART/CCONJ/ADV documentation and the full Smyth "
                         "particle list; record a lemma-and-relation rule only where documented."),
    "BI-ELIG-EIMI": ("identity", "factual_gap",
                     "einai with no effective PNOM: UD allows AUX for existential be only as the same lexeme; no AGDT rule.",
                     "Check the pinned UD grc cop/AUX documentation for existential/locative einai."),
    "BI-ELIG-NAME": ("identity", "factual_gap_with_unperformed_checks",
                     "Capitalized or name-joined nouns not in Morpheus nom.proper; other Morpheus noun files unchecked.",
                     "Check the remaining pinned Morpheus noun stem files for these 5 lemmas."),
    "BI-HEADWORD-CONDITIONAL": ("identity", "factual_gap", "Compound decoded by decomposition match only (H3).",
                                "Check Morpheus vbs.cmp.lsj / lsj.vbs for a printed or tabled compound row."),
    "BI-HEADWORD-UNRESOLVED": ("identity", "factual_gap",
                               "v1.6 homograph 1 disagrees with the only Morpheus compound row (homograph 2).",
                               "Same Morpheus tables; record unresolved if they disagree."),
    "BT-T2-NO-SOURCE-SPACING": ("text_authority", "route_inherent",
                                "AGDT carries no whitespace, paragraphing, labels or editorial markup.", "None."),
    "BT-T2-COMPONENT-NOTATION": ("text_authority", "route_inherent", "Hyphen fragments are not printed surfaces.", "None."),
    "BT-T2-ELISION-ENCODING": ("text_authority", "route_inherent", "v2.x U+0313 elision; NFC turns 5 into rho+psili.", "None."),
    "BT-T2-EDITORIAL-MARKUP": ("text_authority", "route_inherent", "TEI del/add status is absent from AGDT.", "None."),
    "BT-T3-NO-SPAN-MANIFEST": ("binding", "unperformed",
                               "Only op-level comparison exists for grc1 @aa0dd9e6; no span manifest was produced.",
                               "Run the pinned binding probe extractor on the aa0dd9e6 blob (markup differs: quote/title rubrics)."),
    "BT-T3-VARIANT": ("binding", "route_inherent", "grc1 2014 lacks the iota subscript (s060w009) and has the rough breathing (s125w015).", "None."),
}

TEXT_OK = {"exact", "exact_nfc_equivalent", "label_exact", "bound_xr_elision", "shared_span_anchor"}
PARTICLE_RULES = {"R03N", "R03C", "R03A", "R03S", "R03P"}
NAME_RULES = {"R01C", "R01M", "R01A"}


def cp(s):
    return " ".join(f"U+{ord(c):04X}" for c in s)


def esc(s):
    """Escape non-ASCII characters inside otherwise ASCII text as <U+XXXX>."""
    return "".join(c if ord(c) < 128 else f"<U+{ord(c):04X}>" for c in s)


def nfc(s):
    return unicodedata.normalize("NFC", s)


def sha(s):
    return hashlib.sha256(s.encode("utf-8")).hexdigest()


def sha_file(path):
    with open(path, "rb") as f:
        return hashlib.sha256(f.read()).hexdigest()


def read_tsv(path):
    with open(path, encoding="utf-8", newline="") as f:
        return list(csv.DictReader(f, delimiter="\t"))


def write_tsv(path, rows, fields):
    with open(path, "w", encoding="ascii", newline="") as f:
        w = csv.DictWriter(f, fields, delimiter="\t", lineterminator="\n")
        w.writeheader()
        for r in rows:
            w.writerow({k: r.get(k, "") for k in fields})


def node_key(sid, wid):
    return f"s{int(sid):03d}w{int(wid):03d}"


def load_agdt(path):
    order, nodes, sentences = [], {}, {}
    for s in ET.parse(path).getroot().iter("sentence"):
        sentences[int(s.get("id"))] = dict(s.attrib)
        for w in s.findall("word"):
            k = node_key(s.get("id"), w.get("id"))
            assert k not in nodes, f"duplicate native node {k} in {path}"
            nodes[k] = dict(w.attrib)
            order.append(k)
    return order, nodes, sentences


def split_lemma_number(lemma):
    m = re.match(r"^(.*?)(\d+)$", lemma)
    return (m.group(1), m.group(2)) if m else (lemma, "")


def classify_form(v21, v16):
    if v21 == v16:
        return "equal"
    if v16.endswith(ELISION_V16) and v21 == v16[:-1] + ELISION_V2:
        return "elision_mark"
    if nfc(v21) == nfc(v16):
        return "nfc_equivalent"
    return "other"


def classify_lemma(v21, v16, is_punct, is_artificial):
    if v21 == v16:
        return "equal"
    if is_punct:
        return "punct_levelled"
    base, num = split_lemma_number(v16)
    if num and nfc(base) == v21:
        return "morpheus_number" if not is_artificial else "morpheus_number_unprinted"
    if nfc(v16) == v21:
        return "nfc_equivalent"
    return "other"


def word_index(canon_key):
    m = re.match(r"^cg2:w(\d{4})$", canon_key or "")
    return int(m.group(1)) if m else None


def identity_status(upos_status, eligibility, headword_status, disc_ok):
    """Mirror of the 50f4ad6c propagation: resolved only when all four layers are resolved."""
    if eligibility not in ("eligible", "conditional"):
        return "n/a"
    if headword_status == "unresolved" or upos_status == "unresolved":
        return "unresolved"
    if (upos_status == "resolved" and eligibility == "eligible"
            and headword_status in ("source_plain", "decoded") and disc_ok):
        return "resolved"
    return "conditional"


def check_anchor(words, i, j, bnode, tok_index):
    """Nodes words[i..j] (inclusive) must sit between exact anchors bound to tok_index-1 and +1."""
    if i == 0 or j + 1 >= len(words):
        return False
    prev, nxt = bnode[words[i - 1]], bnode[words[j + 1]]
    return (prev["correspondence"] == "exact_equal" and nxt["correspondence"] == "exact_equal"
            and word_index(prev["canon_keys"]) == tok_index - 1
            and word_index(nxt["canon_keys"]) == tok_index + 1)


def load_inputs(inp):
    sys.path.insert(0, os.path.join(inp, BIND, "scripts"))
    import binding_probe  # exported from the pinned binding commit; provides the grc2 extractor

    data = {"probe": binding_probe}
    for rel in ("v2.1", "v2.0", "v1.6"):
        data[rel] = load_agdt(os.path.join(inp, "agdt", f"{rel}.xml"))
    data["canon"] = binding_probe.canonical(os.path.join(inp, "canon", "grc2.xml"))
    data["bsum"] = json.load(open(os.path.join(inp, BIND, "manifest", "summary.json"), encoding="utf-8"))
    for name in ("occurrence_binding", "canonical_tokens", "rubric_bindings", "discrepancies"):
        data[name] = read_tsv(os.path.join(inp, BIND, "manifest", f"{name}.tsv"))
    for name in ("witness_word_ledger", "witness_punct_ledger", "witness_point_checks",
                 "witness_named_checks", "agdt_v16_vs_annotated_base"):
        data[name] = read_tsv(os.path.join(inp, BIND, "collation", f"{name}.tsv"))
    data["wsum"] = json.load(open(os.path.join(inp, BIND, "collation", "witness_summary.json"), encoding="utf-8"))
    for name in ("lysias1-nodes", "lysias1-components", "lysias1-rules", "lysias1-evidence"):
        data[name] = read_tsv(os.path.join(inp, MAP, "manifest", f"{name}.tsv"))
    data["msum"] = json.load(open(os.path.join(inp, MAP, "manifest", "lysias1-summary.json"), encoding="utf-8"))
    data["mlineage"] = json.load(open(os.path.join(inp, MAP, "manifest", "lysias1-lineage.json"), encoding="utf-8"))
    return data


def join(data):
    """Return (order, rows, checks). One row per native node; raises on any join defect."""
    order21, n21, s21 = data["v2.1"]
    order20, n20, _ = data["v2.0"]
    order16, n16, s16 = data["v1.6"]
    bnode = {r["key"]: r for r in data["occurrence_binding"]}
    mnode = {r["binding_join_key"]: r for r in data["lysias1-nodes"]}
    checks = collections.OrderedDict()
    checks["native_nodes_v2.1"] = len(order21)
    checks["duplicate_keys"] = {
        "binding": len(data["occurrence_binding"]) - len(bnode),
        "mapping": len(data["lysias1-nodes"]) - len(mnode),
    }
    checks["same_node_order_v2.1_v2.0_v1.6"] = order21 == order20 == order16
    checks["binding_keys_equal_native"] = set(bnode) == set(order21)
    checks["mapping_keys_equal_native"] = set(mnode) == set(order21)
    checks["mapping_node_key_consistent"] = all(
        node_key(*m["node_key"].split(":")) == k for k, m in mnode.items())
    checks["mapping_doc_pins_v2.1"] = {m["doc"] for m in mnode.values()} == {
        f"tlg0540.tlg001.perseus-grc1@treebank_data:{TREEBANK_COMMIT[:8]}:v2.1"}
    checks["binding_source_sha256"] = data["bsum"]["source_agdt"]["sha256"]
    checks["mapping_source_sha256"] = data["msum"]["inputs"]["lysias_sha256"]
    for name, ok in list(checks.items()):
        if name.startswith(("same_", "binding_keys", "mapping_keys", "mapping_node", "mapping_doc")):
            assert ok is True, name
    assert checks["duplicate_keys"] == {"binding": 0, "mapping": 0}
    assert checks["binding_source_sha256"] == checks["mapping_source_sha256"]

    field_mismatch = collections.Counter()
    rows = collections.OrderedDict()
    for idx, k in enumerate(order21, 1):
        a, b20, o, b, m = n21[k], n20[k], n16[k], bnode[k], mnode[k]
        is_art = a.get("artificial", "") != ""
        is_punct = a.get("postag", "")[:1] == "u"
        # Native fields must agree with what both investigations recorded.
        if b["postag"] != a.get("postag", "") or b["relation"] != a.get("relation", "") or b["head"] != a.get("head", ""):
            field_mismatch["binding_native_fields"] += 1
        if m["native_postag"] != a.get("postag", "") or m["native_relation"] != a.get("relation", "") \
                or m["head"] != a.get("head", "") or m["native_lemma"] != a.get("lemma", ""):
            field_mismatch["mapping_native_fields"] += 1
        if m["form"] != nfc(a.get("form", "")):
            field_mismatch["mapping_form_not_nfc_of_raw"] += 1
        if b["form_sha256"] != sha(a.get("form", "")):
            field_mismatch["binding_form_sha256"] += 1
        for f in ("postag", "relation", "head", "artificial"):
            if (f != "postag" and a.get(f, "") != o.get(f, "")) or (f == "postag" and a.get(f, "")[:1] != o.get(f, "")[:1]):
                field_mismatch[f"v1.6_{f}{'_letter' if f == 'postag' else ''}"] += 1
        if any(a.get(f, "") != b20.get(f, "") for f in ("form", "lemma", "postag", "relation", "head", "artificial")):
            field_mismatch["v2.0_vs_v2.1_any_field"] += 1
        form21, form16 = a.get("form", ""), o.get("form", "")
        lem21, lem16 = a.get("lemma", ""), o.get("lemma", "")
        pos_diff = [str(i + 1) for i, (x, y) in enumerate(zip(a.get("postag", ""), o.get("postag", ""))) if x != y]
        rows[k] = {
            "node_key": k, "doc_order": idx, "sentence_id": int(k[1:4]), "word_id": int(k[5:8]),
            "node_class": b["node_class"], "source_scope": m["source_scope"],
            "artificial": a.get("artificial", ""),
            "form_v21_cp": cp(form21), "form_v21_is_nfc": form21 == nfc(form21),
            "form_v21_nfc_cp": cp(nfc(form21)) if form21 != nfc(form21) else "",
            "form_v21_sha256": sha(form21),
            "form_v16_rel": classify_form(form21, form16),
            "form_v16_cp": cp(form16) if form16 != form21 else "",
            "form_v20_equal_v21": form21 == b20.get("form", ""),
            "lemma_v21_cp": cp(lem21),
            "lemma_v16_rel": classify_lemma(lem21, lem16, is_punct, is_art),
            "lemma_v16_cp": cp(lem16) if lem16 != lem21 else "",
            "postag_v21": a.get("postag", ""), "postag_v16_diff_positions": ",".join(pos_diff),
            "relation": a.get("relation", ""), "head": a.get("head", ""),
            "b_correspondence": b["correspondence"], "b_surface": b["surface"],
            "b_convention": b["convention"], "b_status": b["occurrence_status"],
            "b_canon_keys": b["canon_keys"], "b_canon_doc": b["canon_doc"],
            "b_span": f"{b['span_start']}:{b['span_end']}" if b["span_start"] != "" else "",
            "b_block": b["block"], "b_reason": b["reason"],
            "m_rule": m["rule_id"], "m_upos_candidates": m["upos_candidates"], "m_upos_status": m["upos_status"],
            "m_eligibility": m["eligibility"], "m_headword_status": m["headword_status"],
            "m_headword_rule": m["headword_rule"], "m_headword_cp": cp(m["citation_headword"]),
            "m_key_cp": cp(m["canonical_key"]), "m_disc_status": m["discriminator_status"],
            "m_disc_value": m["discriminator_value"], "m_identity_status": m["identity_status"],
            "m_identity_candidates_cp": cp(m["identity_candidates"]),
            "m_evidence_ids": m["evidence_ids"], "m_flags": esc(m["flags"]),
            "_form21": form21, "_form16": form16, "_lem21": lem21, "_lem16": lem16,
            "_is_punct": is_punct, "_is_art": is_art,
        }
    checks["field_mismatch"] = dict(sorted(field_mismatch.items()))
    assert not field_mismatch, field_mismatch
    checks["sentence_attrs"] = {
        "v2.1_document_ids": sorted({s["document_id"] for s in s21.values()}),
        "v1.6_document_ids": sorted({s["document_id"] for s in s16.values()}),
        "subdocs": sorted({s.get("subdoc", "") for s in s21.values()}),
        "word_cite_attributes_v2.1": sum(1 for x in n21.values() if x.get("cite")),
    }
    return order21, rows, checks


def attach_evidence(data, rows):
    """Link collation, lineage and component findings to native nodes."""
    links = collections.defaultdict(list)
    ckey_nodes = collections.defaultdict(list)
    for k, r in rows.items():
        for ck in filter(None, r["b_canon_keys"].split(";")):
            ckey_nodes[ck].append(k)
    for i, r in enumerate(data["witness_word_ledger"], 1):
        ref = f"WL{i:03d}:{r['witness']}:{r['op']}:{r['class']}"
        targets = set(filter(None, r["node_keys"].split(";")))
        for ck in filter(None, r["canon_keys"].split(";")):
            targets.update(ckey_nodes.get(ck, []))
        for k in sorted(targets):
            links[k].append(ref)
    for i, r in enumerate(data["witness_punct_ledger"], 1):
        for k in filter(None, r["node_keys"].split(";")):
            links[k].append(f"PL{i:03d}:{r['witness']}:{r['class']}")
    for r in data["witness_point_checks"]:
        targets = ckey_nodes.get(r["point"], [])
        for k in targets:
            links[k].append(f"HUDE:{r['point']}:C={r['C_vs_hude']}:AGDT={r['AGDT_vs_hude']}")
    for r in data["witness_named_checks"]:
        for k in filter(None, r["node_keys"].split(";")):
            links[k].append(f"NC:{r['check']}:MN={r['MN_status']}:WS={r['WS_status']}")
    for i, r in enumerate(data["agdt_v16_vs_annotated_base"], 1):
        for k in filter(None, r["node_keys"].split(",")):
            rows[k]["lineage_ref"] = f"LB{i:02d}:{r['op']}:aa0dd9e6"
    comp = {r["binding_join_key"]: r for r in data["lysias1-components"]}
    flags_by_ckey = {r["key"]: r["flags"] for r in data["canonical_tokens"]}
    for k, r in rows.items():
        r["collation_refs"] = ";".join(links.get(k, []))
        r.setdefault("lineage_ref", "")
        c = comp.get(k)
        r["m_component"] = f"{c['convention_id']}:{c['group_id']}:{c['role']}:{c['status']}" if c else ""
        r["canon_flags"] = ";".join(sorted({flags_by_ckey.get(ck, "") for ck in r["b_canon_keys"].split(";")} - {""}))


def verify_text(data, order, rows):
    """Re-verify every binding claim against the pinned grc2 text; evaluate XR rules."""
    probe, canon = data["probe"], data["canon"]
    text = canon["text"]
    assert sha(text) == data["bsum"]["running_text"]["sha256"], "grc2 running text hash"
    tokens = {r["key"]: r for r in data["canonical_tokens"]}
    labels = {f"cg2:label{lab['ord']:02d}": lab for lab in canon["labels"]}
    bnode = {r["key"]: r for r in data["occurrence_binding"]}
    words = [k for k in order if rows[k]["node_class"] == "word" and rows[k]["source_scope"] == "running_text"]
    pos_in_words = {k: i for i, k in enumerate(words)}
    stats = collections.Counter()

    def tok_text(ck):
        t = tokens[ck]
        return text[int(t["span_start"]):int(t["span_end"])]

    groups = collections.defaultdict(list)
    for k in order:
        r = rows[k]
        corr = r["b_correspondence"]
        r["xr_elision"] = r["xr_span"] = r["canon_cp"] = r["v16_text_rel"] = ""
        if corr in ("exact_equal", "punct_gap"):
            t = tok_text(r["b_canon_keys"])
            assert nfc(r["_form21"]) == t, k
            stats[f"{corr}_raw_equal" if r["_form21"] == t else f"{corr}_nfc_only"] += 1
            r["v16_text_rel"] = "raw_equal" if r["_form16"] == t else ("nfc_equal" if nfc(r["_form16"]) == t else "differs")
        elif corr == "rubric_sequence":
            rb = next(x for x in data["rubric_bindings"] if x["key"] == k)
            assert nfc(r["_form21"]) == nfc(labels[rb["label_doc"]]["text"]), k
            r["v16_text_rel"] = "raw_equal" if r["_form16"] == labels[rb["label_doc"]]["text"] else "nfc_equal"
            stats["rubric_label_equal"] += 1
        elif corr == "fold_equal":
            t = tok_text(r["b_canon_keys"])
            r["canon_cp"] = cp(t)
            ok = (r["form_v16_rel"] == "elision_mark" and r["_form16"] == t and r["form_v20_equal_v21"]
                  and r["lemma_v16_rel"] in ("equal", "morpheus_number", "nfc_equivalent")
                  and r["postag_v16_diff_positions"] in ("", "9", "6", "2", "3"))
            r["xr_elision"] = "verified" if ok else "failed"
            r["v16_text_rel"] = "raw_equal" if r["_form16"] == t else "differs"
            stats[f"xr_elision_{r['xr_elision']}"] += 1
            if not r["form_v21_is_nfc"]:
                stats["xr_elision_nfc_composes_v21"] += 1
        elif corr in ("concat_window", "concat_reversed", "positional_block"):
            groups[r["b_canon_keys"]].append(k)
    for ck, ks in groups.items():
        t = tok_text(ck)
        idx = word_index(ck)
        ks.sort(key=lambda x: rows[x]["doc_order"])
        i, j = pos_in_words[ks[0]], pos_in_words[ks[-1]]
        assert j - i + 1 == len(ks), (ck, ks)
        anchored = check_anchor(words, i, j, bnode, idx)
        pieces = [rows[x]["_form21"].strip("-") for x in ks]
        forward = "".join(pieces) == t
        reverse = "".join(reversed(pieces)) == t
        hyph = [rows[x]["_form21"] for x in ks]
        fragment_first = len(ks) == 2 and hyph[0].endswith("-") and not hyph[1].startswith("-")
        enclitic_first = len(ks) == 2 and hyph[0].startswith("-")
        for x in ks:
            r = rows[x]
            r["canon_cp"] = cp(t)
            r["anchor_verified"] = anchored
            if len(ks) == 1:
                r["xr_span"] = "single_node_variant" if anchored else "single_node_unanchored"
            elif anchored and forward and fragment_first:
                r["xr_span"] = "shared_span_verified"
            elif anchored and reverse and enclitic_first:
                r["xr_span"] = "anchored_order_conflict"
            else:
                r["xr_span"] = "unverified"
            stats[f"xr_span_{r['xr_span']}"] += 1
            r["v16_text_rel"] = "component" if len(ks) > 1 else ("raw_equal" if r["_form16"] == t else "differs")
    stats["component_groups"] = sum(1 for ks in groups.values() if len(ks) > 1)
    for r in rows.values():
        r.setdefault("anchor_verified", "")
    return dict(sorted(stats.items())), words


def a2_carryover(rows):
    """XR-V16-PRIMARY-1: check that every 50f4ad6c rule input is identical when v1.6 is primary."""
    bad = []
    for k, r in rows.items():
        if r["_is_punct"]:
            continue
        base, _ = split_lemma_number(r["_lem16"])
        if nfc(base) != r["_lem21"]:
            bad.append(k)
    return bad


def text_status(route, r):
    """Text correspondence status of one node under one route, with the rule that decides it."""
    corr = r["b_correspondence"]
    if r["_is_art"]:
        return "not_printed", "ADR0094-empty-node"
    component = r["m_component"].startswith(("C-CRASIS", "C-ENCLITIC"))
    if route.startswith("T2"):
        if component:
            return "producer_notation_not_printable", "C-CRASIS/C-ENCLITIC undocumented; no spacing/joining evidence in AGDT"
        if route == "T2A1" and r["form_v16_rel"] == "elision_mark":
            return "nonstandard_elision_encoding", "v2.x U+0313 elision; NFC composes 5 forms"
        return "own_surface_unspaced", "AGDT carries no whitespace or editorial markup; running text needs a validated reconstruction"
    if route == "T3A2":
        lb = r["lineage_ref"]
        if component:
            return ("op_level_order_conflict" if r["m_component"].startswith("C-ENCLITIC") else "op_level_component"), lb
        if lb.endswith(":delete:aa0dd9e6"):
            return "annotation_only_print_attested", lb
        if lb:
            return "op_level_variant", lb
        return "op_level_equal_no_span", "agdt_v16_vs_annotated_base.tsv (no span manifest for aa0dd9e6)"
    # T1 routes: the pinned grc2 text.
    if corr in ("exact_equal", "punct_gap"):
        if route == "T1A2" and r["v16_text_rel"] == "nfc_equal":
            return "exact_nfc_equivalent", "NFC canonical equivalence (#2033); raw v1.6 coordinates kept"
        return "exact", "binding exact_equal/punct_gap re-verified"
    if corr == "rubric_sequence":
        return "label_exact", "rubric_sequence re-verified against label document"
    if corr == "fold_equal":
        if route == "T1A1":
            return "conditional_fold", "input status conditional (fold, no adopted convention)"
        if route == "T1A2":
            return ("exact", "v1.6 form equals grc2 token") if r["v16_text_rel"] == "raw_equal" else ("unbound", "v1.6 differs")
        return ("bound_xr_elision", "XR-ELISION-1") if r["xr_elision"] == "verified" else ("unbound", "XR-ELISION-1 failed")
    if corr == "concat_window":
        if route == "T1A1":
            return "candidate_concat", "input status candidate; input piece spans not adopted"
        return ("shared_span_anchor", "XR-SPAN-SHARED-1") if r["xr_span"] == "shared_span_verified" else ("candidate_concat", "XR-SPAN-SHARED-1 not met")
    if corr == "concat_reversed":
        return "blocked_component_order", "anchored to one token each; node order contradicts text and attested host-first practice"
    if corr == "positional_block":
        return "variant_positional", "anchored; one breathing code point differs from grc2"
    if r["b_status"] == "annotation_only":
        return "annotation_only_print_attested", "rubric stop: absent from grc1/grc2, printed by Hude"
    if r["b_status"] == "unaligned":
        return "annotation_only_deleted_in_grc2", "mark present in grc1 2014, deleted by b01744e8 (2021)"
    raise AssertionError((route, r["node_key"], corr))


def identity_for_route(route, r):
    """(identity_status, discriminator_status, discriminator_origin) under one route."""
    disc, origin = r["m_disc_status"], ""
    if route == "T1A1X" and disc in ("stripped_in_v2.1_witness_v1.6", "conflated"):
        disc, origin = "spliced_v1.6_XR-DISC-1", "v1.6"
    elif route in ("T1A2", "T2A2", "T3A2") and disc != "n/a":
        if r["lemma_v16_rel"] == "morpheus_number":
            disc, origin = "native_v1.6", "v1.6"
        else:
            disc = "absent_in_source"
    if r["m_eligibility"] not in ("eligible", "conditional"):
        return "n/a", disc, origin
    return identity_status(r["m_upos_status"], r["m_eligibility"], r["m_headword_status"], disc != "conflated"), disc, origin


def blockers_for(route, r, tstatus):
    out = []
    lexical = not (r["_is_punct"] or r["_is_art"])
    if r["_is_art"]:
        return out
    if tstatus not in TEXT_OK:
        if tstatus.startswith("annotation_only"):
            out.append("BS-PUNCT-ANNOTATION-ONLY")
        elif tstatus == "blocked_component_order" or tstatus == "op_level_order_conflict":
            out.append("BT-S011-ORDER")
        elif tstatus == "variant_positional":
            out.append("BT-S125-VARIANT")
        elif tstatus == "conditional_fold":
            out.append("BT-ELISION-UNADOPTED")
        elif tstatus == "candidate_concat":
            out.append("BT-CRASIS-CANDIDATE")
        elif tstatus == "own_surface_unspaced":
            out.append("BT-T2-NO-SOURCE-SPACING")
        elif tstatus == "producer_notation_not_printable":
            out.extend(["BT-T2-NO-SOURCE-SPACING", "BT-T2-COMPONENT-NOTATION"])
        elif tstatus == "nonstandard_elision_encoding":
            out.extend(["BT-T2-NO-SOURCE-SPACING", "BT-T2-ELISION-ENCODING"])
        elif tstatus.startswith("op_level"):
            out.append("BT-T3-NO-SPAN-MANIFEST")
            if tstatus == "op_level_variant":
                out.append("BT-T3-VARIANT")
        else:
            out.append("BT-OTHER")
        if route.startswith("T2") and set(r["canon_flags"].split(";")) & {"add", "del"}:
            out.append("BT-T2-EDITORIAL-MARKUP")
    if lexical and r["m_eligibility"] in ("eligible", "conditional"):
        if r["m_rule"] in PARTICLE_RULES:
            out.append("BI-ELIG-PARTICLE")
        elif r["m_rule"] == "R05X":
            out.append("BI-ELIG-EIMI")
        elif r["m_rule"] in NAME_RULES:
            out.append("BI-ELIG-NAME")
        elif r["m_eligibility"] == "conditional":
            out.append("BI-ELIG-OTHER")
        if r["m_headword_status"] == "conditional":
            out.append("BI-HEADWORD-CONDITIONAL")
        elif r["m_headword_status"] == "unresolved":
            out.append("BI-HEADWORD-UNRESOLVED")
    return out


def assess(order, rows):
    route_rows = []
    per_route = {}
    for route in ROUTES:
        counts = collections.Counter()
        blocker_nodes = collections.defaultdict(list)
        ident = collections.Counter()
        id_keys = collections.defaultdict(set)
        for k in order:
            r = rows[k]
            ts, trule = text_status(route, r)
            ist, disc, origin = identity_for_route(route, r)
            bl = blockers_for(route, r, ts)
            if ist == "conditional" and r["m_disc_status"] == "conflated" and disc == "conflated":
                bl.append("BI-DISC-CONFLATED")
            vocab = r["m_eligibility"] if r["m_eligibility"] != "unprinted" else "unprinted"
            if r["_is_art"]:
                ready = "unprinted"
            elif r["_is_punct"]:
                ready = "punct_ok" if not bl else "punct_blocked"
            elif vocab == "non_vocab":
                ready = "non_vocab_ok" if not bl else "non_vocab_blocked"
            else:
                ready = "vocab_ready" if not bl and ist == "resolved" else "vocab_blocked"
            assert not (ready == "vocab_ready" and ist != "resolved")
            assert not (ready == "vocab_blocked" and not bl), (route, k, ist)
            counts[ready] += 1
            counts[f"text:{ts}"] += 1
            ident[ist] += 1
            if ist in ("resolved", "conditional", "unresolved") and r["m_eligibility"] == "eligible" and ist == "resolved":
                disc_val = (split_lemma_number(r["_lem16"])[1] if origin else "")
                id_keys["resolved"].add((r["m_key_cp"], r["m_upos_candidates"], disc_val))
            for b in bl:
                blocker_nodes[b].append(k)
            route_rows.append({
                "route": route, "node_key": k, "text_status": ts, "text_rule": trule,
                "vocab_class": vocab, "identity_status": ist, "discriminator_status": disc,
                "discriminator_origin": origin, "readiness": ready, "blockers": ";".join(bl),
            })
        per_route[route] = {
            "readiness": dict(sorted(counts.items())),
            "identity_status": dict(sorted(ident.items())),
            "resolved_eligible_identities": len(id_keys["resolved"]),
            "blockers": {b: ks for b, ks in sorted(blocker_nodes.items())},
        }
    return route_rows, per_route


def overlap(per_route, rows, route):
    bl = per_route[route]["blockers"]
    names = sorted(bl)
    sets = {b: set(bl[b]) for b in names}
    union = set().union(*sets.values()) if sets else set()
    lexical = {k for k in union if not rows[k]["_is_punct"]}
    vocab_rows = {k for k in union if rows[k]["m_eligibility"] in ("eligible", "conditional")}
    pairs = {f"{a}&{b}": sorted(sets[a] & sets[b]) for i, a in enumerate(names) for b in names[i + 1:] if sets[a] & sets[b]}
    exclusive = {b: len(sets[b] - set().union(*(sets[c] for c in names if c != b))) for b in names}
    return {
        "blocker_counts": {b: len(sets[b]) for b in names},
        "exclusive_counts": exclusive,
        "pairwise_intersections": pairs,
        "union_nodes": len(union),
        "union_lexical_nodes": len(lexical),
        "union_punctuation_nodes": len(union) - len(lexical),
        "union_vocabulary_candidate_nodes": len(vocab_rows),
        "union_vocabulary_candidate_excluding_segmentation_only": len(
            {k for k in vocab_rows if set(b for b in names if k in sets[b]) - {"BS-PUNCT-ANNOTATION-ONLY"}}),
    }


def sentences_state(order, rows, route_rows, data):
    lac = data["canon"]["lacunae"]
    by_sent = collections.defaultdict(list)
    for k in order:
        by_sent[rows[k]["sentence_id"]].append(k)
    rr = {(x["route"], x["node_key"]): x for x in route_rows}
    out = []
    for sid, ks in sorted(by_sent.items()):
        printed = [k for k in ks if not rows[k]["_is_art"]]
        spans = [tuple(map(int, rows[k]["b_span"].split(":"))) for k in printed
                 if rows[k]["b_span"] and rows[k]["b_canon_doc"] == "cg2:run"]
        lo = min(s for s, _ in spans) if spans else None
        hi = max(e for _, e in spans) if spans else None
        last = printed[-1] if printed else ""
        out.append({
            "sentence_id": sid, "nodes": len(ks), "printed": len(printed),
            "punct": sum(1 for k in ks if rows[k]["_is_punct"]), "unprinted": len(ks) - len(printed),
            "scope": ",".join(sorted({rows[k]["source_scope"] for k in printed})),
            "last_printed_node": last, "last_printed_relation": rows[last]["relation"] if last else "",
            "last_printed_t1_status": rr[("T1A2", last)]["text_status"] if last else "",
            "t1_span": f"{lo}:{hi}" if spans else "",
            "lacuna_inside_t1_span": any(lo is not None and lo < x < hi for x in lac),
            "editorial_flags": ",".join(sorted({f for k in printed for f in rows[k]["canon_flags"].split(";") if f in ("add", "del", "add;q", "del;q")})),
            "t1a2_text_blocked_nodes": sum(1 for k in printed if rr[("T1A2", k)]["text_status"] not in TEXT_OK
                                           and not rr[("T1A2", k)]["text_status"].startswith("annotation_only")),
            "t1a2_annotation_only_punct": sum(1 for k in printed if rr[("T1A2", k)]["text_status"].startswith("annotation_only")),
            "t1a2_vocab_blocked_nodes": sum(1 for k in printed if rr[("T1A2", k)]["readiness"] == "vocab_blocked"),
            "t1a2_vocab_ready_nodes": sum(1 for k in printed if rr[("T1A2", k)]["readiness"] == "vocab_ready"),
        })
    return out


def canonical_coverage(data, rows, route_rows):
    rr = {(x["route"], x["node_key"]): x for x in route_rows}
    out = []
    for t in data["canonical_tokens"]:
        nodes = [k for k in t["node_keys"].split(";") if k]
        row = {"key": t["key"], "kind": t["kind"], "doc": t["doc"], "span": f"{t['span_start']}:{t['span_end']}",
               "flags": t["flags"], "node_keys": t["node_keys"], "T1A1_input_status": t["coverage_status"]}
        for route in ("T1A1X", "T1A2"):
            sts = sorted({rr[(route, k)]["text_status"] for k in nodes})
            row[f"{route}_status"] = ("covered" if nodes and all(s in TEXT_OK for s in sts)
                                      else ("uncovered" if not nodes else "blocked:" + "|".join(sts)))
        out.append(row)
    return out


def rights_and_docs(inp):
    def has(rel, needle):
        with open(os.path.join(inp, rel), encoding="utf-8") as f:
            return needle in f.read()
    listing = open(os.path.join(inp, "canon", "aa0dd9e6-root-listing.txt"), encoding="utf-8").read().split()
    facts = {
        "treebank_root_readme_cc_by_sa_3_us": has("agdt-docs/root-README.md", "Attribution-ShareAlike 3.0 United States"),
        "treebank_root_readme_third_party_clause": has("agdt-docs/root-README.md", "third-parties do not hold copyright"),
        "v1_greek_docs_readme_by_nc_sa_2_5": has("agdt-docs/v1-greek-docs-README.txt", "NonCommercial-ShareAlike 2.5"),
        "v1.6_readme_describes_schema_transform": has("agdt-docs/v1.6-README.md", "1.6 version of the treebank schema"),
        "v1.6_readme_licence_statement": has("agdt-docs/v1.6-README.md", "icense"),
        "v1.6_file_licence_element": has("agdt/v1.6.xml", "icence") or has("agdt/v1.6.xml", "icense"),
        "v2.1_file_licence_element": has("agdt/v2.1.xml", "icence") or has("agdt/v2.1.xml", "icense"),
        "v2.0_readme_character_normalization": has("agdt-docs/v2.0-Greek-README.MD", "checked/normalized for the Greek characters"),
        "v2.0_readme_semi_automatic": has("agdt-docs/v2.0-Greek-README.MD", "semi-automatically annotated"),
        "v2.1_header_names_readme_xml": has("agdt/v2.1.xml", "README.xml"),
        "v2.1_header_v1.6_pre_normalization": has("agdt/v2.1.xml", "before normalization"),
        "v1.6_header_segtok_tool": has("agdt/v1.6.xml", "llt/segtok"),
        "v1.6_header_annotator_gorman": has("agdt/v1.6.xml", "<name>Vanessa Gorman</name>"),
        "grc2_tei_licence_cc_by_sa_4": has("canon/grc2.xml", "creativecommons.org/licenses/by-sa/4.0"),
        "canonical_license_md_cc_by_sa_4": has("canon/license.md", "4.0"),
        "grc1_aa0dd9e6_licence_element": has("canon/grc1-aa0dd9e6.xml", "icence") or has("canon/grc1-aa0dd9e6.xml", "icense"),
        "aa0dd9e6_root_has_licence_file": any(x.lower().startswith("licen") for x in listing),
    }
    return facts


def ledger(per_route, rows, checks, stats, rights, data, carry_bad):
    ov = {r: overlap(per_route, rows, r) for r in ROUTES}
    ws = data["wsum"]
    authority = {
        "T1": {
            "text": DOC_CANON, "status": "pass_with_qualifications",
            "facts": [
                "one Perseus digitization of Lamb 1930 (canonical history replayed 2014-2026; 2 token changes)",
                "full-speech collation vs Mnemosyne and Wikisource: bounds, 50 sections, 5 rubrics, lacuna equal",
                f"55 Hude OCT point checks: grc2 agrees {ws['hude_point_checks']['by_stream']['C']['equal_raw'] + ws['hude_point_checks']['by_stream']['C']['equal_key']}, differs {ws['hude_point_checks']['by_stream']['C']['differs']}",
                "11 grc2/Hude departures are documented edition alternatives; 5 have no apparatus support "
                "(cg2:w1034 w2083 w2102 w2180 w2239)",
            ],
            "qualifications": ["Lamb 1930 print UNPERFORMED (not retried)", "full Hude collation UNPERFORMED",
                               "witnesses MN/WS share the Hude tradition; MN base undeclared"],
        },
        "T2": {"text": "AGDT forms", "status": "fail",
               "facts": ["AGDT has no whitespace, editorial del/add/gap markup, label structure or paragraphing",
                         "26 component nodes are producer notation, not printed surfaces",
                         "every reconstruction would have to be validated against T1/T3 text, i.e. a composite"]},
        "T3": {"text": f"canonical-greekLit@{CANON_HIST_COMMIT[:8]} grc1 (declared URN current on the annotation date; "
                       "NOT proven to be the producer's input revision)", "status": "not_assessed_at_span_level",
               "facts": ["differs from grc2 by 2 tokens: s060w009 iota subscript absent (fixed 2016), mark after cg2:w0042 present",
                         "rubrics at section ends as quote/title, as Hude prints them",
                         "no licence element in the blob and no licence file in the tree at aa0dd9e6"]},
    }
    rights_ledger = [
        {"component": "grc2 running text", "source": DOC_CANON, "declaration": "CC BY-SA 4.0 (TEI licence + license.md)",
         "verified": rights["grc2_tei_licence_cc_by_sa_4"] and rights["canonical_license_md_cc_by_sa_4"],
         "private_research": "permitted by declaration", "private_personal_use": "permitted by declaration",
         "public_redistribution_or_adaptation": "eligible after #2035 exact-component review (not performed here)"},
        {"component": "grc1 @aa0dd9e6 text", "source": "canonical-greekLit@aa0dd9e6", "declaration": "none at that revision",
         "verified": not rights["grc1_aa0dd9e6_licence_element"] and not rights["aa0dd9e6_root_has_licence_file"],
         "private_research": "lineage comparison only", "private_personal_use": "unresolved",
         "public_redistribution_or_adaptation": "unresolved (later repository licence's reach not reviewed)"},
        {"component": "AGDT v2.1 annotation", "source": DOC_V21,
         "declaration": "root README CC BY-SA 3.0 US, third-party clause; no file-level element",
         "verified": rights["treebank_root_readme_cc_by_sa_3_us"] and not rights["v2.1_file_licence_element"],
         "private_research": "permitted by declaration", "private_personal_use": "permitted by declaration",
         "public_redistribution_or_adaptation": "eligible after #2035 exact-component review (not performed here)"},
        {"component": "AGDT v1.6 annotation", "source": DOC_V16,
         "declaration": "root README CC BY-SA 3.0 US covers repository contents; v1 Greek docs README declares "
                        "CC BY-NC-SA 2.5 for AGDT 1.7 - its reach to this schema-1.6 Lysias file is NOT established",
         "verified": rights["v1_greek_docs_readme_by_nc_sa_2_5"] and rights["v1.6_readme_describes_schema_transform"]
                     and not rights["v1.6_readme_licence_statement"] and not rights["v1.6_file_licence_element"],
         "private_research": "permitted under either candidate declaration",
         "private_personal_use": "non-commercial personal use is permitted under either candidate declaration; "
                                 "no review outcome recorded (#2035)",
         "public_redistribution_or_adaptation": "unresolved: depends on whether the NC declaration reaches the file"},
        {"component": "Morpheus stemlib/C sources (headword evidence H1/H3)", "source": "perseids-tools/morpheus@ab6898ff",
         "declaration": "MPL-2.0 (as recorded by 50f4ad6c)", "verified": "not re-read here",
         "private_research": "read as evidence only", "private_personal_use": "no Morpheus data copied into identities",
         "public_redistribution_or_adaptation": "not applicable unless lexicon data is redistributed"},
        {"component": "Witness readings (Mnemosyne, Wikisource, Hude)", "source": "dc3e5ed7 collation/",
         "declaration": "MN all rights reserved; WS CC BY-SA 4.0; Hude not reviewed",
         "verified": "as recorded by dc3e5ed7", "private_research": "witness research only",
         "private_personal_use": "not product data", "public_redistribution_or_adaptation": "not product data"},
    ]
    review = {
        "lemma_pos_layers": "file-level human annotation by a named annotator (Vanessa Gorman, 2014), Morpheus-assisted "
                            "(v2.0 README: semi-automatically annotated); release-editor normalization in v2.0",
        "pos_letters_v1.6_vs_v2.1": "identical on all 2,834 nodes",
        "lexical_lemmas_v1.6_vs_v2.1": "identical after NFC except 83 Morpheus numbers stripped in v2.0",
        "second_annotation": "none for Lysias I (not required)",
        "per_token_certification": "no source statement (not required)",
        "syntax_review": "not evidence of lemma/POS review",
        "acceptance": "the MVP human-reviewed bar is decided in #2030; this reconciliation records claims only",
    }
    routes = {}
    for route, desc in ROUTES.items():
        p = per_route[route]
        tfam = route[:2]
        vocab_blockers = {b: len(v) for b, v in p["blockers"].items() if b != "BS-PUNCT-ANNOTATION-ONLY"}
        routes[route] = {
            "description": desc,
            "text_authority": authority[tfam]["status"],
            "full_scope": {"nodes": 2834, "printed": 2802, "unprinted": 32, "shortened": False},
            "exact_occurrences": {
                "text_ok_printed_nodes": sum(v for s, v in p["readiness"].items() if s.startswith("text:") and s[5:] in TEXT_OK),
                "text_status": {s[5:]: v for s, v in p["readiness"].items() if s.startswith("text:")},
            },
            "identities": {"node_identity_status": p["identity_status"],
                           "resolved_eligible_identities": p["resolved_eligible_identities"]},
            "segmentation": "native AGDT sentences only; no reviewed versioned segmentation convention adopted",
            "readiness_counts": {s: v for s, v in p["readiness"].items() if not s.startswith("text:")},
            "blockers": {b: len(v) for b, v in p["blockers"].items()},
            "overlap": ov[route],
            "vocabulary_ready": not vocab_blockers and authority[tfam]["status"] != "fail",
            "sentence_card_ready": "not assessed (separate gate; requires vocabulary readiness and a reviewed segmentation)",
        }
    scope = {}
    for b in ("BI-ELIG-PARTICLE", "BI-ELIG-NAME", "BI-ELIG-EIMI", "BI-HEADWORD-CONDITIONAL", "BI-HEADWORD-UNRESOLVED"):
        ks = per_route["T1A2"]["blockers"].get(b, [])
        by_rule = collections.Counter(rows[k]["m_rule"] for k in ks)
        scope[b] = {"nodes": len(ks), "distinct_lemmas": len({rows[k]["_lem21"] for k in ks}),
                    "by_rule": dict(sorted(by_rule.items()))}
    editorial = []
    for k, r in rows.items():
        fl = set(r["canon_flags"].split(";")) & {"add", "del"}
        if fl:
            editorial.append({"node_key": k, "canon_keys": r["b_canon_keys"], "flag": ",".join(sorted(fl)),
                              "eligibility": r["m_eligibility"],
                              "T1A2_readiness": next(x for x in per_route["_rows"] if x["route"] == "T1A2" and x["node_key"] == k)["readiness"]})
    return {
        "schema": SCHEMA,
        "blocker_definitions": {b: dict(zip(("layer", "nature", "evidence", "minimum_next_check"), v)) for b, v in BLOCKERS.items()},
        "blocker_scope_T1A2": scope,
        "editorial_occurrences": {"note": "TEI del/add tokens are exact in grc2 and bound; their bracket status must be "
                                          "carried to any sentence presentation. Lacuna: one gap inside AGDT sentence 59's span.",
                                  "rows": editorial, "lacuna_offsets": data["canon"]["lacunae"]},
        "inputs": {"binding_commit": BINDING_COMMIT, "mapping_commit": MAPPING_COMMIT,
                   "treebank_commit": TREEBANK_COMMIT, "canonical_commit": CANON_COMMIT,
                   "canonical_historical_commit": CANON_HIST_COMMIT},
        "join_checks": checks, "verification": stats,
        "a2_rule_input_carryover_failures": carry_bad,
        "rules": RULES, "text_authority": authority, "rights": rights_ledger, "rights_facts": rights,
        "review_provenance": review, "routes": routes,
        "preferred_route": "T1A2",
        "verdict": {
            "vocabulary_ready_any_route": any(v["vocabulary_ready"] for v in routes.values()),
            "binding_text_ready_T1A2": all(b.startswith(("BI-", "BS-")) for b in per_route["T1A2"]["blockers"]) ,
        },
    }


def main(inp, out):
    data = load_inputs(inp)
    order, rows, checks = join(data)
    attach_evidence(data, rows)
    stats, _ = verify_text(data, order, rows)
    carry_bad = a2_carryover(rows)
    assert not carry_bad, carry_bad
    route_rows, per_route = assess(order, rows)
    per_route["_rows"] = route_rows
    rights = rights_and_docs(inp)
    os.makedirs(out, exist_ok=True)
    node_fields = [
        "node_key", "doc_order", "sentence_id", "word_id", "node_class", "source_scope", "artificial",
        "form_v21_cp", "form_v21_is_nfc", "form_v21_nfc_cp", "form_v21_sha256", "form_v16_rel", "form_v16_cp",
        "form_v20_equal_v21", "lemma_v21_cp", "lemma_v16_rel", "lemma_v16_cp", "postag_v21",
        "postag_v16_diff_positions", "relation", "head",
        "b_correspondence", "b_surface", "b_convention", "b_status", "b_canon_keys", "b_canon_doc", "b_span",
        "b_block", "b_reason", "canon_flags", "canon_cp", "v16_text_rel", "anchor_verified", "xr_elision", "xr_span",
        "collation_refs", "lineage_ref",
        "m_rule", "m_upos_candidates", "m_upos_status", "m_eligibility", "m_headword_status", "m_headword_rule",
        "m_headword_cp", "m_key_cp", "m_disc_status", "m_disc_value", "m_identity_status", "m_identity_candidates_cp",
        "m_component", "m_evidence_ids", "m_flags",
    ]
    write_tsv(os.path.join(out, "nodes.tsv"), [rows[k] for k in order], node_fields)
    write_tsv(os.path.join(out, "route_nodes.tsv"), route_rows,
              ["route", "node_key", "text_status", "text_rule", "vocab_class", "identity_status",
               "discriminator_status", "discriminator_origin", "readiness", "blockers"])
    write_tsv(os.path.join(out, "sentences.tsv"), sentences_state(order, rows, route_rows, data),
              ["sentence_id", "nodes", "printed", "punct", "unprinted", "scope", "last_printed_node",
               "last_printed_relation", "last_printed_t1_status", "t1_span", "lacuna_inside_t1_span",
               "editorial_flags", "t1a2_text_blocked_nodes", "t1a2_annotation_only_punct",
               "t1a2_vocab_blocked_nodes", "t1a2_vocab_ready_nodes"])
    write_tsv(os.path.join(out, "canonical_coverage.tsv"), canonical_coverage(data, rows, route_rows),
              ["key", "kind", "doc", "span", "flags", "node_keys", "T1A1_input_status", "T1A1X_status", "T1A2_status"])
    blockers = []
    for route in ROUTES:
        for b, ks in per_route[route]["blockers"].items():
            blockers.append({"route": route, "blocker": b, "count": len(ks),
                             "lexical": sum(1 for k in ks if not rows[k]["_is_punct"]),
                             "vocab_candidates": sum(1 for k in ks if rows[k]["m_eligibility"] in ("eligible", "conditional")),
                             "node_keys": ";".join(ks)})
    write_tsv(os.path.join(out, "blockers.tsv"), blockers, ["route", "blocker", "count", "lexical", "vocab_candidates", "node_keys"])
    led = ledger(per_route, rows, checks, stats, rights, data, carry_bad)
    with open(os.path.join(out, "route_ledger.json"), "w", encoding="ascii") as f:
        json.dump(led, f, indent=1, sort_keys=True, ensure_ascii=True)
        f.write("\n")
    return led


if __name__ == "__main__":
    main(sys.argv[1], sys.argv[2])
