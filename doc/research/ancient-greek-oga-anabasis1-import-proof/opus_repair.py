#!/usr/bin/env python3
"""Authorized bounded research-repair pass for OGA Anabasis Book 1, issue #2052.

Builds on the v2 interpreter (opus_completion.py, unchanged) and then:

1. applies the εἰμί rule ``oga-agdt-upos/3`` to all 178 εἰμί nodes. It refines
   the impersonal-infinitive rule: a PNOM that is an infinitive or an empty node
   no longer makes ἦν + infinitive a copula, and a nominal subject blocks the
   impersonal reading;
2. applies the exact whitelist in ``opus-repair-records.json``: seven citation
   repairs and six εἰμί POS repairs. Each repair is bound to the component hashes
   and the node's raw values, fails closed on any mismatch, and keeps the raw
   prediction in separate fields;
3. reruns every whole-unit gate, the real Go selection probe, the 3/15 model and
   the sentence inputs, and compares them with the uncorrected v2 result.

    python3 -I opus_repair.py           # verify and compare; writes nothing
    python3 -I opus_repair.py --no-go   # same without the Go probe (exit 1)
    python3 -I opus_repair.py --write   # regenerate opus-repair-summary.json and private/opus-repair/out/

This script never writes the v1/v2 summaries or private/opus-completion/.
"""

from __future__ import annotations

import argparse
import collections
import hashlib
import json
import sys
import unicodedata
from pathlib import Path

HERE = Path(__file__).resolve().parent
if str(HERE) not in sys.path:  # python3 -I does not add the script directory
    sys.path.insert(0, str(HERE))

import mapping_completion_v2 as v2  # noqa: E402
import opus_completion as oc  # noqa: E402

VERSION = "oga-anabasis1-opus-repair/1"
RECORDS = HERE / "opus-repair-records.json"
SUMMARY = HERE / "opus-repair-summary.json"
OUT_DIR = oc.PRIVATE / "opus-repair" / "out"
EIMI_KEY = oc.profile_key(oc.EIMI)
# The exact authorized whitelist (#2052 bounded repair exception).
AUTHORIZED_CITATION = ("t_437", "t_4624", "t_4638", "t_5775", "t_4930", "t_5121", "t_5145")
AUTHORIZED_POS = ("t_2739", "t_3351", "t_7640", "t_10265", "t_10412", "t_5989")


class RepairError(SystemExit):
    """A repair precondition or evidence check failed."""


# --- εἰμί rule v3 -----------------------------------------------------------------


def eimi_context_v3(cols: list[str], children: dict[str, list[list[str]]], empty_ids: set[str]) -> dict:
    """Native-syntax evidence under one εἰμί node, separating lexical from verbal PNOMs."""
    kids = children.get(cols[0], [])
    pnoms, stack = [], list(kids)
    while stack:
        child = stack.pop()
        if child[7] == "PNOM":
            pnoms.append(child)
        elif child[7] in ("COORD", "APOS"):
            stack.extend(children.get(child[0], []))

    def infinitive(c):
        return c[3] == "v" and "Mood=n" in c[5]

    subjects = [k for k in kids if k[7] == "SBJ"]
    return {"pnom_lexical": any(not infinitive(p) and p[9] not in empty_ids for p in pnoms),
            "pnom_verbal_or_empty": any(infinitive(p) or p[9] in empty_ids for p in pnoms),
            "pp": any(k[7] == "AuxP" for k in kids),
            "adverbial": any(k[7] == "ADV" for k in kids),
            "infinitive_subject": any(infinitive(k) for k in subjects),
            "nominal_subject": any(not infinitive(k) and k[3] != "v" for k in subjects)}


def eimi_rule_v3(ctx: dict) -> tuple[str, str]:
    """AUX/VERB for εἰμί from UD definitions over native AGDT syntax (version 3).

    Impersonal ἔστι + infinitive ('it is possible to') has no nonverbal predicate.
    The impersonal reading needs an infinitive subject (or an infinitive or empty
    PNOM), no lexical PNOM, no PP and no nominal subject, unless the only predicate
    candidate is an infinitive or empty PNOM. Otherwise v2 order applies.
    """
    impersonal = not ctx["pnom_lexical"] and not ctx["pp"] and (
        ctx["pnom_verbal_or_empty"] or (ctx["infinitive_subject"] and not ctx["nominal_subject"]))
    if impersonal:
        return "VERB", "P-V-EIMI-IMPERSONAL-INFINITIVE"
    if ctx["pnom_lexical"]:
        return "AUX", "P-V-EIMI-COP-PNOM"
    if ctx["pp"] or ctx["adverbial"]:
        return "AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE"
    return "VERB", "P-V-EIMI-EXISTENTIAL"


# --- repairs -------------------------------------------------------------------------


def load_records(path: Path = RECORDS) -> dict:
    records = json.loads(Path(path).read_text(encoding="utf-8"))
    if records.get("version") != "oga-anabasis1-research-repairs/1":
        raise RepairError("unknown repair-record version")
    return records


def check_whitelist(records: dict) -> None:
    """Records must name exactly the authorized IDs, once each, with no overlap."""
    citation = [r["id"] for r in records["citation_repairs"]]
    pos = [r["id"] for r in records["pos_repairs"]]
    if len(set(citation)) != len(citation) or len(set(pos)) != len(pos):
        raise RepairError("duplicate repair IDs")
    if set(citation) & set(pos):
        raise RepairError("a node has both a citation and a POS repair")
    if sorted(citation) != sorted(AUTHORIZED_CITATION) or sorted(pos) != sorted(AUTHORIZED_POS):
        raise RepairError("repair IDs differ from the authorized whitelist")


def native_edge_index(groups: list) -> dict[str, tuple[str, str, str]]:
    """Node ID -> (form, native relation, head node ID or ROOT) from the pinned groups."""
    index = {}
    for group in groups:
        by_number = {c[0]: c[9] for c in group}
        for cols in group:
            index[cols[9]] = (unicodedata.normalize("NFC", cols[1]), cols[7], by_number.get(cols[6], "ROOT"))
    return index


def verify_native_edges(item: dict, edges: dict) -> None:
    """Each cited native edge must match the parse exactly. This checks the citation of
    the parse, not the Greek interpretation built on it."""
    cited = item.get("evidence", {}).get("native_edges")
    if not cited:
        raise RepairError(f"{item['id']}: POS repair has no native edge witness")
    for node_id, form, relation, head in cited + item["evidence"].get("native_parallel_pnom_datives", []):
        if edges.get(node_id) != (unicodedata.normalize("NFC", form), relation, head):
            raise RepairError(f"{item['id']}: native edge {node_id} is {edges.get(node_id)}, record says "
                              f"{(form, relation, head)}")


def verify_oga_nodes(item: dict, evidence: dict, ledger: dict) -> None:
    """Cited OGA parallels carry the repaired headword; convention nodes their stated lemma/surfaces."""
    target = unicodedata.normalize("NFC", item["after"]["headword"])
    for node_id in evidence.get("oga_same_lexeme_nodes", []):
        node = ledger.get(node_id)
        if node is None or node["empty"] or unicodedata.normalize("NFC", node["raw_lemma"]) != target:
            raise RepairError(f"{item['id']}: OGA parallel {node_id} does not carry {target}")
    convention = evidence.get("oga_convention_nodes")
    if convention:
        for node_id in convention["ids"]:
            node = ledger.get(node_id)
            if (node is None or node["empty"] or node["raw_lemma"] != convention["lemma"]
                    or node["surface"] not in convention["surfaces"]):
                raise RepairError(f"{item['id']}: OGA convention node {node_id} differs from the record")


def check_component(records: dict) -> None:
    """Repairs bind to the exact OGA component revision."""
    component = records["component"]
    if component["conllu_member_sha256"] != oc.PINS[oc.CONLLU]:
        raise RepairError("repair records name a different CoNLL-U member revision")
    if component["native_nodes_sha256"] != oc.PINS[oc.NATIVE_NODES]:
        raise RepairError("repair records name a different native-node revision")
    for path, digest in ((oc.CONLLU, component["conllu_member_sha256"]),
                         (oc.NATIVE_NODES, component["native_nodes_sha256"])):
        oc.check_pin(path, digest)


def check_preconditions(node: dict, expected: dict, kind: str) -> None:
    """Exact raw values, compared after NFC only; any other difference fails."""
    actual = {"raw_lemma": node["raw_lemma"], "surface": node["surface"], "native_pos": node["native_pos"],
              "v2_rule": node["lemma"]["rule"], "relation": node.get("relation"),
              "rule_upos": node["pos"]["upos"], "rule": node["pos"]["rule"]}
    for field, value in expected.items():
        have = actual[field]
        if not isinstance(have, str) or unicodedata.normalize("NFC", have) != unicodedata.normalize("NFC", value):
            raise RepairError(f"{kind} repair for {node['id']}: {field} is {have!r}, expected {value!r}")


def verify_evidence(item: dict, records: dict, lexicon: dict) -> dict:
    """Every cited witness must be present in the retained pinned evidence."""
    evidence = item.get("evidence") or next(
        r["evidence"] for r in records["citation_repairs"] if r["id"] == item["same_evidence_as"])
    rows = lexicon["rows_by_location"]
    for file, line, key, derivation in evidence.get("morpheus_rows", []):
        row = rows.get((file, line))
        if row is None or (row["key"], row["derivation"]) != (key, derivation):
            raise RepairError(f"{item['id']}: Morpheus witness {file}:{line} {key} not found")
    for file, line, text in evidence.get("stem_lines", []):
        lines = lexicon["stem_lines"].get(file)
        if lines is None or lines[line - 1].strip() != text:
            raise RepairError(f"{item['id']}: stem line {file}:{line} differs")
    for lemma, form, count in evidence.get("ud_perseus_lemma_forms", []):
        got = lexicon["ud_lemma_forms"].get((unicodedata.normalize("NFC", lemma), unicodedata.normalize("NFC", form)), 0)
        if got != count:
            raise RepairError(f"{item['id']}: UD Perseus has {got} × {lemma}/{form}, record says {count}")
    return evidence


def repaired_lemma(item: dict, original: dict, lexicon: dict) -> dict:
    """The repaired citation. The headword comes from the evidence, never from the raw label."""
    after = item["after"]
    key = after.get("morpheus_key") or after.get("formed_key")
    headword, digits = v2.decode_beta(key)
    if headword != unicodedata.normalize("NFC", after["headword"]):
        raise RepairError(f"{item['id']}: key {key} decodes to {headword}, record says {after['headword']}")
    if digits or after["discriminator"] is not None:
        raise RepairError(f"{item['id']}: citation repairs in this pass carry no discriminator")
    if after.get("morpheus_key") and not lexicon["by_head"].get(v2.citation_key(headword)):
        raise RepairError(f"{item['id']}: repaired headword has no Morpheus key")
    return {"raw_lemma": original["raw_lemma"], "status": "resolved", "rule": "R-CITATION-REPAIR",
            "headword": headword, "key": oc.profile_key(headword), "discriminator": None,
            "qualifications": ["research-repair"], "citation_flags": [], "raw_flags": original["raw_flags"],
            "repair": {"id": item["id"], "kind": item["repair_kind"], "before_rule": original["rule"],
                       "before_predicted_form": original.get("predicted_form"),
                       "morpheus_key": after.get("morpheus_key"), "formed_key": after.get("formed_key"),
                       "uncertainty": item["uncertainty"]}}


def apply_eimi_v3(ledger: dict, groups: list, empty_ids: set[str]) -> list[dict]:
    """Recompute UPOS for every εἰμί node; return the nodes whose rule changed."""
    changes = []
    for group in groups:
        children = collections.defaultdict(list)
        for cols in group:
            children[cols[6]].append(cols)
        for cols in group:
            node = ledger.get(cols[9])
            if not node or node["empty"] or not node["pos"]["rule"].startswith("P-V-EIMI"):
                continue
            ctx = eimi_context_v3(cols, children, empty_ids)
            upos, rule = eimi_rule_v3(ctx)
            if rule != node["pos"]["rule"]:
                changes.append({"id": node["id"], "v2_rule": node["pos"]["rule"], "v2_upos": node["pos"]["upos"],
                                "v3_rule": rule, "v3_upos": upos})
            node["pos"] = {**node["pos"], "upos": upos, "candidates": [upos], "rule": rule, "context_v3": ctx}
    return sorted(changes, key=lambda c: int(c["id"][2:]))


def apply_repairs(ledger: dict, records: dict, lexicon: dict) -> dict:
    """Apply exactly the whitelisted repairs. Unknown IDs or failed checks stop the run."""
    applied = {"citation": [], "pos": []}
    for item in records["citation_repairs"]:
        node = ledger.get(item["id"])
        if node is None or node["empty"] or node["cts"] != item["cts"]:
            raise RepairError(f"citation repair names an unknown or misplaced node {item['id']}")
        check_preconditions(node, item["expected"], "citation")
        verify_oga_nodes(item, verify_evidence(item, records, lexicon), ledger)
        node["lemma"] = repaired_lemma(item, node["lemma"], lexicon)
        node["citation_flags"] = []
        applied["citation"].append(item["id"])
    for item in records["pos_repairs"]:
        node = ledger.get(item["id"])
        if node is None or node["empty"] or node["cts"] != item["cts"]:
            raise RepairError(f"POS repair names an unknown or misplaced node {item['id']}")
        check_preconditions(node, item["expected"], "pos")
        verify_native_edges(item, lexicon["native_edges"])
        if item["after_upos"] not in ("AUX", "VERB") or item["after_upos"] == node["pos"]["upos"]:
            raise RepairError(f"POS repair for {item['id']} does not change the εἰμί UPOS")
        node["pos"] = {**node["pos"], "upos": item["after_upos"], "candidates": [item["after_upos"]],
                       "rule": "R-POS-REPAIR", "repair": {"before_rule": node["pos"]["rule"],
                                                          "before_upos": node["pos"]["upos"],
                                                          "ud_category": item["ud_category"],
                                                          "uncertainty": item["uncertainty"]}}
        applied["pos"].append(item["id"])
    for node in ledger.values():
        if node["empty"]:
            continue
        node["status"] = oc.vocabulary_status(node["lemma"], node["pos"])
        node["identity"] = oc.identity_string(node["lemma"]) if node["status"] == "eligible" else None
    return applied


def ud_lemma_forms(paths: list[Path]) -> dict:
    counts: collections.Counter = collections.Counter()
    for path in paths:
        for sentence in oc.read_conllu_rows(path):
            for cols in sentence:
                if "-" in cols[0] or "." in cols[0]:
                    continue
                counts[(unicodedata.normalize("NFC", cols[2]), unicodedata.normalize("NFC", cols[1]))] += 1
    return counts


# --- whole-unit build ------------------------------------------------------------------


def build() -> tuple[dict, dict]:
    inputs = oc.verify_inputs()
    records = load_records()
    check_whitelist(records)
    check_component(records)
    nodes = json.loads(oc.NATIVE_NODES.read_text(encoding="utf-8"))
    import_ledger = json.loads(oc.IMPORT_LEDGER.read_text(encoding="utf-8"))
    binding = v2.verify_binding(nodes, import_ledger)
    text = oc.READING.read_text(encoding="utf-8")
    tei = {r["id"]: r["tei_string_indices"] for r in json.loads(oc.NODE_TO_TEI.read_text(encoding="utf-8"))}
    node_ids = {n["id"] for n in nodes}
    groups = [g for g in oc.read_conllu_rows(oc.CONLLU) if any(c[9] in node_ids for c in g)]
    gates = {"native_join": oc.check_native_join(nodes, groups)}
    rows = v2.load_index_rows()
    by_head, _ = v2.index_maps(rows)
    stem_lines = oc.load_stem_lines()
    evidence = oc.ud_perseus_evidence(oc.UD_PERSEUS)
    ledger = oc.interpret_nodes(nodes, groups, evidence, (by_head, oc.compound_notation_index(rows), stem_lines))
    uncorrected = {k: (n["status"], n["identity"], n["pos"]["upos"]) for k, n in ledger.items() if not n["empty"]}
    lexicon = {"rows_by_location": {(r["file"], r["line"]): r for r in rows}, "stem_lines": stem_lines,
               "by_head": by_head, "ud_lemma_forms": ud_lemma_forms(oc.UD_PERSEUS),
               "native_edges": native_edge_index(groups)}
    eimi_changes = apply_eimi_v3(ledger, groups, {n["id"] for n in nodes if n["empty"]})
    applied = apply_repairs(ledger, records, lexicon)

    printed = [n for n in nodes if not n["empty"]]
    gates["reading_coverage"] = oc.check_coverage(text, printed)
    sentences = oc.segment(text)
    gates["sentence_coverage"] = oc.check_sentence_coverage(text, sentences)
    where = oc.sentence_of(printed, sentences)
    for node_id, index in where.items():
        ledger[node_id]["sentence"] = index
    group_check = oc.native_group_check(groups, where, {n["id"]: n for n in nodes})
    gates["native_groups_nest"] = {"groups": group_check["native_groups"], "crossing": 0}
    splits = oc.word_internal_spaces(printed, tei)
    editorial = oc.editorial_by_sentence(printed, where, tei, import_ledger)
    for sentence in sentences:
        sentence["word_internal_spaces"] = sum(1 for i in splits if where[i] == sentence["index"])
        sentence["editorial"] = editorial.get(sentence["index"], [])
        sentence["completeness"] = "complete" if sentence["terminal"] in oc.TERMINALS else "unverified"

    occurrences: dict[tuple[str, str], list[str]] = collections.defaultdict(list)
    for node in ledger.values():
        if not node["empty"] and node["status"] == "eligible":
            occurrences[(node["identity"], node["pos"]["upos"])].append(node["id"])
    model = oc.selection_model(occurrences)
    selected = [r for r in model if r["selected"]]
    inputs_ = oc.card_inputs(selected, occurrences, ledger, sentences, text)
    analysis = oc.analysis_json(ledger, sentences, text)
    ctx = dict(locals())
    summary = summarize(ctx)
    private = private_texts(ctx)
    summary["private_outputs"] = {name: {"sha256": hashlib.sha256(t.encode("utf-8")).hexdigest(),
                                         "lines": t.count("\n")} for name, t in sorted(private.items())}
    return summary, private


def counter(items) -> dict:
    return dict(sorted(collections.Counter(items).items()))


def comparison(ctx: dict) -> dict:
    """Uncorrected v2 versus repaired: identities, selection and changed occurrences."""
    before: dict[tuple[str, str], list[str]] = collections.defaultdict(list)
    for node_id, (status, identity, upos) in ctx["uncorrected"].items():
        if status == "eligible":
            before[(identity, upos)].append(node_id)
    before_model = {(r["key"], r["upos"]): r for r in oc.selection_model(before)}
    after_model = {(r["key"], r["upos"]): r for r in ctx["model"]}
    changed = []
    for key in sorted(set(before_model) | set(after_model)):
        b, a = before_model.get(key), after_model.get(key)
        if (b and (b["occurrences"], b["selected"])) != (a and (a["occurrences"], a["selected"])):
            changed.append({"key": key[0], "upos": key[1],
                            "before": b and {"occurrences": b["occurrences"], "selected": b["selected"]},
                            "after": a and {"occurrences": a["occurrences"], "selected": a["selected"]}})
    nodes_changed = sorted((k for k, v in ctx["uncorrected"].items()
                            if v != (ctx["ledger"][k]["status"], ctx["ledger"][k]["identity"], ctx["ledger"][k]["pos"]["upos"])),
                           key=lambda i: int(i[2:]))
    return {"uncorrected_v2": {"eligible_occurrences": sum(len(v) for v in before.values()),
                               "identities": len(before_model),
                               "selected": sum(r["selected"] for r in before_model.values()),
                               "unresolved_printed_nodes": sum(1 for v in ctx["uncorrected"].values()
                                                               if v[0].startswith("unresolved"))},
            "nodes_with_changed_interpretation": nodes_changed,
            "identity_changes": changed}


def summarize(ctx: dict) -> dict:
    ledger, model, sentences, inputs_ = ctx["ledger"], ctx["model"], ctx["sentences"], ctx["inputs_"]
    printed = [n for n in ledger.values() if not n["empty"]]
    unsupported = [{"id": n["id"], "raw_lemma": n["raw_lemma"]} for n in printed
                   if n["lemma"]["rule"] == "I-UNSUPPORTED-CITATION-FORM"]
    unresolved = sorted((n["id"] for n in printed if n["status"].startswith("unresolved")), key=lambda i: int(i[2:]))
    chosen = [c for c in inputs_ if c["chosen"]]
    eligible_total = sum(r["occurrences"] for r in model)
    selected_occ = sum(r["occurrences"] for r in model if r["selected"])
    repaired = [n for n in printed if n["lemma"]["rule"] == "R-CITATION-REPAIR" or n["pos"]["rule"] == "R-POS-REPAIR"]
    by_identity = {(c["key"], c["upos"]): c for c in inputs_}
    repair_targets = []
    for n in sorted(repaired, key=lambda o: int(o["id"][2:])):
        entry = {"id": n["id"], "cts": n["cts"], "raw_lemma": n["raw_lemma"], "native_pos": n["native_pos"],
                 "lemma_rule": n["lemma"]["rule"], "headword": n["lemma"].get("headword"),
                 "key": n["lemma"].get("key"), "upos": n["pos"]["upos"], "pos_rule": n["pos"]["rule"],
                 "status": n["status"], "identity": n["identity"]}
        hit = by_identity.get((n["identity"], n["pos"]["upos"])) if n["identity"] else None
        entry["identity_selected"] = bool(hit)
        entry["identity_card_input"] = hit and hit["chosen"]
        repair_targets.append(entry)
    eimi = [n for n in printed if n["raw_lemma"] == oc.EIMI or n["pos"]["rule"].startswith("P-V-EIMI")
            or (n["pos"]["rule"] == "R-POS-REPAIR")]
    pos_unresolved = [n["id"] for n in printed if n["status"] == "unresolved-pos"]
    gates = {**{name: {"pass": True, **detail} for name, detail in sorted(ctx["gates"].items())},
             "canonical_pos_determinate": {"pass": not pos_unresolved, "unresolved_ids": pos_unresolved},
             "required_identity_usable": {"pass": not unsupported, "unsupported_ids": [u["id"] for u in unsupported]},
             "all_whitelisted_repairs_applied": {"pass": sorted(ctx["applied"]["citation"]) == sorted(AUTHORIZED_CITATION)
                                                         and sorted(ctx["applied"]["pos"]) == sorted(AUTHORIZED_POS),
                                                 "citation": ctx["applied"]["citation"], "pos": ctx["applied"]["pos"]},
             "vocabulary_ready_accounting": {"pass": not unresolved}}
    return {
        "version": VERSION, "status": "research repair pass; not source acceptance, production mapping or learner state",
        "reproduce": "python3 -I opus_repair.py",
        "rulesets": {**oc.RULESETS, "pos": "oga-agdt-upos/3 (εἰμί rule v3) + research repairs",
                     "repairs": "oga-anabasis1-research-repairs/1"},
        "inputs": ctx["inputs"], "binding": ctx["binding"],
        "learner_state": {"known": 0, "reserved": 0, "corrections": 0, "exclusions": 0,
                          "note": "research repairs are a separate interpretation layer, not learner corrections"},
        "required_gates": gates,
        "repairs": {"citation_applied": ctx["applied"]["citation"], "pos_applied": ctx["applied"]["pos"],
                    "targets": repair_targets},
        "eimi": {"nodes": len(eimi), "rules": counter(n["pos"]["rule"] for n in eimi),
                 "upos": counter(n["pos"]["upos"] for n in eimi), "v3_rule_changes_vs_v2": ctx["eimi_changes"],
                 "ids_by_rule": {r: sorted((n["id"] for n in eimi if n["pos"]["rule"] == r), key=lambda i: int(i[2:]))
                                 for r in sorted({n["pos"]["rule"] for n in eimi}) if r != "P-V-EIMI-COP-PNOM"}},
        "denominators": {"native_nodes": len(ledger), "empty_unprinted": sum(1 for n in ledger.values() if n["empty"]),
                         "printed": len(printed), "vocabulary_status": counter(n["status"] for n in printed),
                         "non_vocabulary_by_upos": counter(n["pos"]["upos"] or "|".join(n["pos"]["candidates"])
                                                           for n in printed if n["status"] == "non-vocabulary"
                                                           and n["native_pos"] != "u"),
                         "lemma_rules": counter(n["lemma"]["rule"] for n in printed if n["native_pos"] != "u"),
                         "unresolved_ids": unresolved},
        "sentences": {**oc.length_profile(sentences), "native_groups": ctx["group_check"]["native_groups"],
                      "completeness": counter(s["completeness"] for s in sentences)},
        "selection": {"identities": len(model), "identities_by_upos": counter(r["upos"] for r in model),
                      "selected": len(ctx["selected"]), "selected_by_upos": counter(r["upos"] for r in ctx["selected"]),
                      "eligible_occurrences": eligible_total, "selected_occurrences": selected_occ,
                      "selected_occurrence_coverage": round(selected_occ / eligible_total, 4),
                      "known_token_coverage": 0.0, "unknown_identities": len(model),
                      "go_probe_expected_stdout": (f"candidates={len(model)} selected={len(ctx['selected'])} "
                                                   f"analyzable_tokens={eligible_total} distinct_identities={len(model)} "
                                                   f"sentences={len(sentences)}"),
                      "selection_model_sha256": hashlib.sha256(oc.selection_tsv(model).encode("utf-8")).hexdigest()},
        "card_inputs": {"selected": len(inputs_), "eligible": len(chosen),
                        "omitted": [{"key": c["key"], "upos": c["upos"], "reasons": c["omission"],
                                     "ids": [r["id"] for r in c["rejections"]]} for c in inputs_ if c["omission"]],
                        "using_later_occurrence": sum(1 for c in chosen if c["earlier_rejections"]),
                        "earlier_rejection_reasons": counter(r for c in chosen for rej in c["earlier_rejections"]
                                                             for r in rej["reasons"]),
                        "chosen_sentence_units": oc.length_profile([sentences[c["sentence"]] for c in chosen])["units"],
                        "distinct_chosen_sentences": len({c["sentence"] for c in chosen})},
        "comparison_with_uncorrected_v2": comparison(ctx),
        "verdicts": {
            "binding_and_unit_gates": "pass (join, coverage, sentences, nesting re-run)",
            "identity": "every printed lexical node has a usable identity" if not unsupported else "unsupported nodes remain",
            "pos": "determinate; εἰμί v3 rule plus six reviewed repairs; other classes producer-compatible",
            "vocabulary_ready_accounting": "complete with research repairs" if not unresolved else "blocked",
            "note": "Measured with an explicit research-repair layer; not the strict uncorrected proof. "
                    "Source acceptance and usefulness remain #2030.",
        },
    }


def private_texts(ctx: dict) -> dict[str, str]:
    ledger, sentences, text = ctx["ledger"], ctx["sentences"], ctx["text"]

    def dump(obj) -> str:
        return json.dumps(obj, ensure_ascii=False, indent=1, sort_keys=True) + "\n"

    repaired_inputs = []
    for c in ctx["inputs_"]:
        if any(ledger[i]["lemma"]["rule"] == "R-CITATION-REPAIR" or ledger[i]["pos"]["rule"] == "R-POS-REPAIR"
               for i in ctx["occurrences"][(c["key"], c["upos"])]):
            entry = dict(c)
            if c["chosen"]:
                node = ledger[c["chosen"]]
                s = sentences[node["sentence"]]
                entry["sentence_text"] = text[s["start"]:s["end"]]
                entry["target"] = {"id": node["id"], "surface": node["surface"], "span": node["reading_span"],
                                   "headword": node["lemma"].get("headword")}
            repaired_inputs.append(entry)
    return {
        "node-ledger.json": dump([ledger[k] for k in sorted(ledger, key=lambda i: (i[0], int(i[2:])))]),
        "selection-model.tsv": oc.selection_tsv(ctx["model"]),
        "card-inputs.json": dump(ctx["inputs_"]),
        "repaired-identity-inputs.json": dump(repaired_inputs),
        "analysis.json": ctx["analysis"],
    }


def guard_write_targets(paths: list[Path]) -> None:
    out_root = OUT_DIR.resolve()
    for path in paths:
        resolved = Path(path).resolve()
        if resolved != SUMMARY.resolve() and out_root not in resolved.parents:
            raise SystemExit(f"refusing to write {path}: not an opus-repair output")


def failing_gates(summary: dict) -> list[str]:
    return sorted(name for name, gate in summary["required_gates"].items() if not gate["pass"])


def run(write: bool, use_go: bool) -> int:
    """Exit 0 only when outputs reproduce, Go parity holds and every required gate passes.

    ``--write`` still writes diagnostics for a failing gate but then exits 1, so a
    regenerated summary never reads as a passing proof.
    """
    summary, private = build()
    summary_text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
    failing = failing_gates(summary)
    if write:
        guard_write_targets([SUMMARY] + [OUT_DIR / name for name in private])
        OUT_DIR.mkdir(parents=True, exist_ok=True)
        for name, text in private.items():
            (OUT_DIR / name).write_text(text, encoding="utf-8")
        SUMMARY.write_text(summary_text, encoding="utf-8")
        print(f"wrote {SUMMARY.name} and {len(private)} private outputs under {OUT_DIR}; failing gates: {failing}")
        return 1 if failing else 0
    committed = SUMMARY.read_text(encoding="utf-8") if SUMMARY.is_file() else None
    problems = oc.reproduction_problems(committed, summary_text, private, OUT_DIR)
    if use_go:
        problems += oc.run_go_probe(private["analysis.json"], private["selection-model.tsv"],
                                    summary["selection"]["go_probe_expected_stdout"])
    else:
        problems.append("go parity not run (--no-go)")
    print(json.dumps({"gates_failing": failing, "selected": summary["selection"]["selected"],
                      "card_inputs": summary["card_inputs"]["eligible"],
                      "reproduction_problems": problems}, ensure_ascii=False, sort_keys=True))
    return 1 if problems or failing else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--write", action="store_true", help="regenerate the repair summary and private outputs")
    parser.add_argument("--no-go", action="store_true", help="skip the Go selection probe")
    args = parser.parse_args(argv)
    return run(write=args.write, use_go=not args.no_go)


if __name__ == "__main__":
    sys.exit(main())
