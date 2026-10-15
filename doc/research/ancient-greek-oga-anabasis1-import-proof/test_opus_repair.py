"""Focused tests for opus_repair.py (#2052 authorized research-repair pass).

Synthetic fixtures exercise one precondition each; they are not source
observations. No temporary files are needed. The last class rebuilds the full
pass from the private inputs and is skipped when they are absent.
"""

from __future__ import annotations

import collections
import copy
import json
import unittest

import mapping_completion_v2 as v2
import opus_completion as oc
import opus_repair as rp

RECORDS = rp.load_records()


def lexicon(**overrides) -> dict:
    rows = [v2.parse_row("vbs.cmp.ml", 6459, "compound", "sulle/gw", "su/n-le/gw2")]
    base = {"rows_by_location": {("vbs.cmp.ml", 6459): rows[0]},
            "stem_lines": {"vbs.cmp.ml": [""] * 6458 + ["su/n-le/gw2 sulle/gw"]},
            "by_head": v2.index_maps(rows)[0],
            "ud_lemma_forms": collections.Counter({("συλλέγω", "συνέλεγεν"): 1, ("συλλέγω", "συνελέγοντο"): 1,
                                                   ("συλλέγω", "συλλέξας"): 1, ("σύν-λέγω", "συλλέξας"): 2})}
    base.update(overrides)
    return base


def unsupported_node(node_id="t_437", raw="σύλλέγω", surface="συνέλεξεν", native="v", cts="1_1_9"):
    return {"id": node_id, "cts": cts, "empty": False, "raw_lemma": raw, "surface": surface, "native_pos": native,
            "relation": "PRED", "citation_flags": [],
            "lemma": {"raw_lemma": raw, "rule": "I-UNSUPPORTED-CITATION-FORM", "status": "unresolved",
                      "headword": None, "discriminator": None, "qualifications": [], "raw_flags": ["multiple-accents"],
                      "citation_flags": [], "predicted_form": raw},
            "pos": {"upos": "VERB", "candidates": ["VERB"], "rule": "P-V-VERB", "status": "resolved"},
            "status": "unresolved-identity", "identity": None}


def only(records: dict, citation=("t_437",), pos=()) -> dict:
    out = copy.deepcopy(records)
    out["citation_repairs"] = [r for r in out["citation_repairs"] if r["id"] in citation]
    out["pos_repairs"] = [r for r in out["pos_repairs"] if r["id"] in pos]
    return out


class CitationRepairs(unittest.TestCase):
    def test_exact_repair_applies_and_keeps_raw(self):
        ledger = {"t_437": unsupported_node()}
        applied = rp.apply_repairs(ledger, only(RECORDS), lexicon())
        node = ledger["t_437"]
        self.assertEqual(applied["citation"], ["t_437"])
        self.assertEqual((node["lemma"]["headword"], node["lemma"]["key"], node["status"]),
                         ("συλλέγω", "συλλέγω", "eligible"))
        self.assertEqual(node["raw_lemma"], "σύλλέγω")
        self.assertEqual(node["lemma"]["repair"]["before_predicted_form"], "σύλλέγω")
        self.assertIsNone(node["lemma"]["discriminator"])

    def test_wrong_raw_value_diacritic_surface_tag_or_place_fails(self):
        for change in ({"raw": "σύλλεγω"}, {"surface": "συνέλεξε"}, {"native": "n"}, {"cts": "1_1_8"}):
            with self.subTest(change=change), self.assertRaises(SystemExit):
                rp.apply_repairs({"t_437": unsupported_node(**change)}, only(RECORDS), lexicon())

    def test_unknown_node_fails(self):
        with self.assertRaises(SystemExit):
            rp.apply_repairs({"t_999": unsupported_node("t_999")}, only(RECORDS), lexicon())

    def test_node_no_longer_unsupported_fails(self):
        node = unsupported_node()
        node["lemma"]["rule"] = "I-PLAIN"
        with self.assertRaises(SystemExit):
            rp.apply_repairs({"t_437": node}, only(RECORDS), lexicon())

    def test_missing_repair_leaves_the_node_unresolved(self):
        ledger = {"t_437": unsupported_node()}
        rp.apply_repairs(ledger, only(RECORDS, citation=()), lexicon())
        self.assertEqual(ledger["t_437"]["status"], "unresolved-identity")

    def test_evidence_mismatches_fail(self):
        lines = {"vbs.cmp.ml": [""] * 6458 + ["su/n-le/gw2 sulle/gw1"]}
        counts = collections.Counter({("συλλέγω", "συνέλεγεν"): 2})
        for lex in (lexicon(rows_by_location={}), lexicon(stem_lines=lines), lexicon(ud_lemma_forms=counts)):
            with self.assertRaises(SystemExit):
                rp.apply_repairs({"t_437": unsupported_node()}, only(RECORDS), lex)

    def test_digit_or_headword_mismatch_fails(self):
        for edit in ({"discriminator": {"scheme": "morpheus", "value": "2"}}, {"headword": "συλλεγω"},
                     {"morpheus_key": "sulle/gw2"}):
            records = only(RECORDS)
            records["citation_repairs"][0]["after"].update(edit)
            with self.subTest(edit=edit), self.assertRaises(SystemExit):
                rp.apply_repairs({"t_437": unsupported_node()}, records, lexicon())

    def test_profile_unchanged_grave_stays_distinct(self):
        self.assertNotEqual(oc.profile_key("ἑπτὰ"), oc.profile_key("ἑπτά"))
        self.assertEqual(oc.citation_form_flags("ἑπτὰ"), ["grave-accent"])

    def test_formed_key_decodes_to_recorded_headword(self):
        item = next(r for r in RECORDS["citation_repairs"] if r["id"] == "t_5145")
        self.assertEqual(v2.decode_beta(item["after"]["formed_key"]), ("συνεῖδον", ""))

    def test_component_revision_binding(self):
        rp.check_component(RECORDS) if oc.CONLLU.is_file() else None
        for field in ("conllu_member_sha256", "native_nodes_sha256"):
            records = copy.deepcopy(RECORDS)
            records["component"][field] = "0" * 64
            with self.subTest(field=field), self.assertRaises(SystemExit):
                rp.check_component(records)

    def test_record_version(self):
        self.assertEqual(len(RECORDS["citation_repairs"]), 7)
        self.assertEqual(len(RECORDS["pos_repairs"]), 6)


def eimi_node(rule="P-V-EIMI-EXISTENTIAL", upos="VERB"):
    return {"id": "t_2739", "cts": "1_3_11", "empty": False, "raw_lemma": "εἰμί", "surface": "εἶναι",
            "native_pos": "v", "relation": "OBJ", "citation_flags": [],
            "lemma": {"raw_lemma": "εἰμί", "rule": "I-PLAIN", "status": "resolved", "headword": "εἰμί",
                      "key": "εἰμί", "discriminator": None, "raw_flags": []},
            "pos": {"upos": upos, "candidates": [upos], "rule": rule, "status": "resolved"},
            "status": "eligible", "identity": "εἰμί"}


class PosRepairs(unittest.TestCase):
    def test_reviewed_repair_applies(self):
        ledger = {"t_2739": eimi_node()}
        rp.apply_repairs(ledger, only(RECORDS, citation=(), pos=("t_2739",)), lexicon())
        node = ledger["t_2739"]
        self.assertEqual((node["pos"]["upos"], node["pos"]["rule"], node["status"]), ("AUX", "R-POS-REPAIR", "non-vocabulary"))
        self.assertEqual(node["pos"]["repair"]["before_rule"], "P-V-EIMI-EXISTENTIAL")

    def test_rule_output_precondition(self):
        for node in (eimi_node(rule="P-V-EIMI-IMPERSONAL-INFINITIVE"), eimi_node(rule="P-V-EIMI-COP-PNOM", upos="AUX")):
            with self.assertRaises(SystemExit):
                rp.apply_repairs({"t_2739": node}, only(RECORDS, citation=(), pos=("t_2739",)), lexicon())

    def test_no_op_repair_fails(self):
        records = only(RECORDS, citation=(), pos=("t_2739",))
        records["pos_repairs"][0]["after_upos"] = "VERB"
        with self.assertRaises(SystemExit):
            rp.apply_repairs({"t_2739": eimi_node()}, records, lexicon())


NONE = {"pnom_lexical": False, "pnom_verbal_or_empty": False, "pp": False, "adverbial": False,
        "infinitive_subject": False, "nominal_subject": False}


class EimiRuleV3(unittest.TestCase):
    def test_table(self):
        cases = [
            ({}, ("VERB", "P-V-EIMI-EXISTENTIAL")),
            ({"pnom_lexical": True}, ("AUX", "P-V-EIMI-COP-PNOM")),
            ({"pnom_verbal_or_empty": True, "nominal_subject": True}, ("VERB", "P-V-EIMI-IMPERSONAL-INFINITIVE")),
            ({"pnom_verbal_or_empty": True, "pp": True}, ("AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE")),
            ({"infinitive_subject": True}, ("VERB", "P-V-EIMI-IMPERSONAL-INFINITIVE")),
            ({"infinitive_subject": True, "nominal_subject": True, "adverbial": True},
             ("AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE")),
            ({"infinitive_subject": True, "pnom_lexical": True}, ("AUX", "P-V-EIMI-COP-PNOM")),
            ({"adverbial": True}, ("AUX", "P-V-EIMI-COP-ADVERBIAL-PREDICATE")),
        ]
        for change, expected in cases:
            self.assertEqual(rp.eimi_rule_v3({**NONE, **change}), expected, change)

    def test_context_from_native_rows(self):
        verb = ["1", "ἦν", "εἰμί", "v", "", "_", "0", "PRED", "_", "t_1"]
        inf = ["2", "θεάσασθαι", "θεάομαι", "v", "", "Mood=n", "1", "PNOM", "_", "t_2"]
        empty = ["3", "[0]", "[0]", "-", "", "_", "1", "PNOM", "_", "e_1"]
        noun = ["4", "μέρος", "μέρος", "n", "", "Case=n", "1", "SBJ", "_", "t_4"]
        got = rp.eimi_context_v3(verb, {"1": [inf, noun]}, set())
        self.assertEqual((got["pnom_lexical"], got["pnom_verbal_or_empty"], got["nominal_subject"]), (False, True, True))
        self.assertTrue(rp.eimi_context_v3(verb, {"1": [empty]}, {"e_1"})["pnom_verbal_or_empty"])


class Guards(unittest.TestCase):
    def test_write_guard_protects_earlier_outputs(self):
        rp.guard_write_targets([rp.SUMMARY, rp.OUT_DIR / "x.json"])
        for bad in (oc.SUMMARY, oc.OUT_DIR / "node-ledger.json", oc.OWN / "out" / "node-ledger.json",
                    oc.HERE / "opus-completion-summary.json", oc.HERE / "opus-repair-records.json"):
            with self.assertRaises(SystemExit):
                rp.guard_write_targets([bad])


@unittest.skipUnless(oc.NATIVE_NODES.is_file() and rp.SUMMARY.is_file(), "private inputs not present")
class FullRepairBuild(unittest.TestCase):
    def test_rebuild_matches_committed_outputs_and_gates(self):
        summary, private = rp.build()
        text = json.dumps(summary, ensure_ascii=False, indent=1, sort_keys=True) + "\n"
        self.assertEqual(oc.reproduction_problems(rp.SUMMARY.read_text(encoding="utf-8"), text, private, rp.OUT_DIR), [])
        self.assertTrue(all(g["pass"] for g in summary["required_gates"].values()))
        self.assertEqual(summary["denominators"]["unresolved_ids"], [])
        self.assertEqual(summary["denominators"]["printed"], 10462)
        self.assertEqual(summary["eimi"]["nodes"], 178)


if __name__ == "__main__":
    unittest.main()
