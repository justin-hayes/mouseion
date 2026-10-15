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
from unittest import mock

import mapping_completion_v2 as v2
import opus_completion as oc
import opus_repair as rp

RECORDS = rp.load_records()


def lexicon(**overrides) -> dict:
    rows = [v2.parse_row("vbs.cmp.ml", 6459, "compound", "sulle/gw", "su/n-le/gw2")]
    base = {"rows_by_location": {("vbs.cmp.ml", 6459): rows[0]},
            "stem_lines": {"vbs.cmp.ml": [""] * 6458 + ["su/n-le/gw2 sulle/gw"]},
            "by_head": v2.index_maps(rows)[0],
            "native_edges": {"t_2738": ("ὥρα", "PNOM", "t_2736"), "t_2739": ("εἶναι", "OBJ", "t_2736"),
                             "t_2736": ("δοκεῖ", "PRED", "ROOT")},
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


def parallel(node_id, raw, surface):
    node = unsupported_node(node_id, raw, surface)
    node["lemma"] = {"raw_lemma": raw, "rule": "I-PLAIN", "status": "resolved", "headword": raw,
                     "key": oc.profile_key(raw), "discriminator": None, "raw_flags": []}
    return node


def citation_ledger(**changes) -> dict:
    """The t_437 target plus the two OGA parallels its record cites."""
    return {"t_437": unsupported_node(**changes), "t_402": parallel("t_402", "συλλέγω", "συνελέγετο"),
            "t_4272": parallel("t_4272", "συλλέγω", "συνέλεξε")}


def only(records: dict, citation=("t_437",), pos=()) -> dict:
    out = copy.deepcopy(records)
    out["citation_repairs"] = [r for r in out["citation_repairs"] if r["id"] in citation]
    out["pos_repairs"] = [r for r in out["pos_repairs"] if r["id"] in pos]
    return out


class CitationRepairs(unittest.TestCase):
    def test_exact_repair_applies_and_keeps_raw(self):
        ledger = citation_ledger()
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
                rp.apply_repairs(citation_ledger(**change), only(RECORDS), lexicon())

    def test_unknown_node_fails(self):
        with self.assertRaises(SystemExit):
            rp.apply_repairs({"t_999": unsupported_node("t_999")}, only(RECORDS), lexicon())

    def test_node_no_longer_unsupported_fails(self):
        node = unsupported_node()
        node["lemma"]["rule"] = "I-PLAIN"
        ledger = citation_ledger()
        ledger["t_437"] = node
        with self.assertRaises(SystemExit):
            rp.apply_repairs(ledger, only(RECORDS), lexicon())

    def test_missing_repair_leaves_the_node_unresolved(self):
        ledger = {"t_437": unsupported_node()}
        rp.apply_repairs(ledger, only(RECORDS, citation=()), lexicon())
        self.assertEqual(ledger["t_437"]["status"], "unresolved-identity")

    def test_evidence_mismatches_fail(self):
        lines = {"vbs.cmp.ml": [""] * 6458 + ["su/n-le/gw2 sulle/gw1"]}
        counts = collections.Counter({("συλλέγω", "συνέλεγεν"): 2})
        for lex in (lexicon(rows_by_location={}), lexicon(stem_lines=lines), lexicon(ud_lemma_forms=counts)):
            with self.assertRaises(SystemExit):
                rp.apply_repairs(citation_ledger(), only(RECORDS), lex)

    def test_digit_or_headword_mismatch_fails(self):
        for edit in ({"discriminator": {"scheme": "morpheus", "value": "2"}}, {"headword": "συλλεγω"},
                     {"morpheus_key": "sulle/gw2"}):
            records = only(RECORDS)
            records["citation_repairs"][0]["after"].update(edit)
            with self.subTest(edit=edit), self.assertRaises(SystemExit):
                rp.apply_repairs(citation_ledger(), records, lexicon())

    def test_cited_oga_parallel_must_carry_the_headword(self):
        for raw in ("σύλλέγω", "συλλεγω", "συλλέγω1"):
            ledger = citation_ledger()
            ledger["t_402"]["raw_lemma"] = raw
            with self.subTest(raw=raw), self.assertRaises(SystemExit):
                rp.apply_repairs(ledger, only(RECORDS), lexicon())
        ledger = citation_ledger()
        del ledger["t_4272"]
        with self.assertRaises(SystemExit):
            rp.apply_repairs(ledger, only(RECORDS), lexicon())

    def test_convention_nodes_checked(self):
        item = next(r for r in RECORDS["citation_repairs"] if r["id"] == "t_5145")
        ledger = {"t_8572": parallel("t_8572", "εἶδον", "ἰδεῖν"), "t_8932": parallel("t_8932", "εἶδον", "ἰδεῖν")}
        rp.verify_oga_nodes(item, item["evidence"], ledger)
        ledger["t_8932"]["surface"] = "ἰδὼν"
        with self.assertRaises(SystemExit):
            rp.verify_oga_nodes(item, item["evidence"], ledger)

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

    def test_native_edge_witness_must_match(self):
        bad_edges = (
            {"t_2738": ("ὥρα", "PNOM", "t_2739")},       # wrong head
            {"t_2738": ("ὥρα", "SBJ", "t_2736")},        # wrong relation
            {"t_2738": ("ωρα", "PNOM", "t_2736")},       # one diacritic
            {"t_2738": ("ὥρᾳ", "PNOM", "t_2736")},       # iota subscript
            {"t_2739": None},                            # missing node
        )
        for change in bad_edges:
            lex = lexicon()
            lex["native_edges"] = {**lex["native_edges"], **change}
            if None in change.values():
                lex["native_edges"] = {k: v for k, v in lex["native_edges"].items() if v is not None}
            with self.subTest(change=change), self.assertRaises(SystemExit):
                rp.apply_repairs({"t_2739": eimi_node()}, only(RECORDS, citation=(), pos=("t_2739",)), lex)

    def test_missing_witness_fails(self):
        records = only(RECORDS, citation=(), pos=("t_2739",))
        records["pos_repairs"][0]["evidence"] = {}
        with self.assertRaises(SystemExit):
            rp.apply_repairs({"t_2739": eimi_node()}, records, lexicon())

    def test_t7640_chain_runs_through_the_elided_node(self):
        item = next(r for r in RECORDS["pos_repairs"] if r["id"] == "t_7640")
        heads = {e[0]: e[3] for e in item["evidence"]["native_edges"]}
        self.assertEqual([heads["t_7647"], heads["t_7648"], heads["e_88"], heads["t_7641"]],
                         ["t_7648", "e_88", "t_7641", "t_7640"])

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


class Whitelist(unittest.TestCase):
    def test_exact_whitelist(self):
        rp.check_whitelist(RECORDS)
        duplicate = copy.deepcopy(RECORDS)
        duplicate["citation_repairs"].append(copy.deepcopy(duplicate["citation_repairs"][0]))
        missing = copy.deepcopy(RECORDS)
        missing["pos_repairs"].pop()
        extra = copy.deepcopy(RECORDS)
        extra["pos_repairs"][0]["id"] = "t_9999"
        overlap = copy.deepcopy(RECORDS)
        overlap["pos_repairs"][0]["id"] = "t_437"
        for name, records in (("duplicate", duplicate), ("missing", missing), ("extra", extra), ("overlap", overlap)):
            with self.subTest(name), self.assertRaises(SystemExit):
                rp.check_whitelist(records)


class RunExitStatus(unittest.TestCase):
    """A failed required gate must make the default run fail even when outputs reproduce."""

    def summary(self, gate_pass: bool) -> dict:
        return {"required_gates": {"required_identity_usable": {"pass": gate_pass}, "native_join": {"pass": True}},
                "selection": {"selected": 1, "go_probe_expected_stdout": "x"}, "card_inputs": {"eligible": 1}}

    def run_with(self, gate_pass: bool) -> int:
        private = {"analysis.json": "a", "selection-model.tsv": "s"}
        with mock.patch.object(rp, "build", return_value=(self.summary(gate_pass), private)), \
                mock.patch.object(oc, "reproduction_problems", return_value=[]), \
                mock.patch.object(oc, "run_go_probe", return_value=[]), \
                mock.patch.object(rp.SUMMARY.__class__, "is_file", return_value=False), \
                mock.patch("builtins.print"):
            return rp.run(write=False, use_go=True)

    def test_failed_gate_is_nonzero_even_when_everything_reproduces(self):
        self.assertEqual(self.run_with(gate_pass=False), 1)
        self.assertEqual(self.run_with(gate_pass=True), 0)
        self.assertEqual(rp.failing_gates(self.summary(False)), ["required_identity_usable"])


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
        self.assertEqual(rp.failing_gates(summary), [])
        self.assertEqual(summary["denominators"]["unresolved_ids"], [])
        self.assertEqual(summary["denominators"]["printed"], 10462)
        self.assertEqual(summary["eimi"]["nodes"], 178)


if __name__ == "__main__":
    unittest.main()
