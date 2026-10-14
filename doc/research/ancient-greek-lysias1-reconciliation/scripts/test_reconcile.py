#!/usr/bin/env python3
"""Tests for reconcile.py.

Run: python3 -I test_reconcile.py
Synthetic unit tests always run. With RECON_INPUTS=<fetch_inputs.sh dir>, the source-backed
fixture rows, full-population invariants and byte-identical regeneration also run; the
regenerated copy is written inside RECON_INPUTS (scratch), never to /tmp.
"""
import csv
import filecmp
import json
import os
import shutil
import sys
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
import reconcile as rc  # noqa: E402

ROOT = os.path.dirname(HERE)
MANIFEST = os.path.join(ROOT, "manifest")
FIXTURE = os.path.join(ROOT, "fixtures", "expected-route-nodes.tsv")
INPUTS = os.environ.get("RECON_INPUTS")

RHO, ALPHA, PI = "ρ", "α", "π"


def node(**kw):
    base = {"node_key": "s001w001", "_is_punct": False, "_is_art": False, "m_component": "",
            "b_correspondence": "exact_equal", "b_status": "accepted", "v16_text_rel": "raw_equal",
            "xr_elision": "", "xr_span": "", "lineage_ref": "", "canon_flags": "", "form_v16_rel": "equal",
            "lemma_v16_rel": "equal", "m_eligibility": "eligible", "m_rule": "R06", "m_upos_status": "resolved",
            "m_headword_status": "source_plain", "m_disc_status": "absent_in_source"}
    base.update(kw)
    return base


class FormAndLemma(unittest.TestCase):
    def test_elision_mark_is_exact_substitution(self):
        v16 = PI + ALPHA + RHO + rc.ELISION_V16
        self.assertEqual(rc.classify_form(PI + ALPHA + RHO + rc.ELISION_V2, v16), "elision_mark")
        self.assertEqual(rc.classify_form(PI + ALPHA + rc.ELISION_V2, v16), "other")
        self.assertEqual(rc.classify_form(v16, v16), "equal")

    def test_nfc_composes_rho_plus_u0313_so_raw_must_be_kept(self):
        raw = PI + ALPHA + RHO + rc.ELISION_V2
        self.assertNotEqual(rc.nfc(raw), raw)
        self.assertEqual(rc.nfc(raw)[-1], "ῤ")  # rho with psili: no longer a separate elision mark

    def test_nfc_equivalent_form(self):
        self.assertEqual(rc.classify_form("ἐ", "ἐ"), "nfc_equivalent")

    def test_lemma_number_and_nfc(self):
        self.assertEqual(rc.split_lemma_number("x12"), ("x", "12"))
        self.assertEqual(rc.classify_lemma("x", "x2", False, False), "morpheus_number")
        self.assertEqual(rc.classify_lemma(".", "punc1", True, False), "punct_levelled")
        self.assertEqual(rc.classify_lemma("ἐ", "ἐ", False, False), "nfc_equivalent")

    def test_escape_keeps_ascii(self):
        self.assertEqual(rc.esc("aα"), "a<U+03B1>")
        self.assertEqual(rc.cp("αa"), "U+03B1 U+0061")


class Propagation(unittest.TestCase):
    def test_identity_needs_all_layers(self):
        self.assertEqual(rc.identity_status("resolved", "eligible", "source_plain", True), "resolved")
        self.assertEqual(rc.identity_status("resolved", "eligible", "decoded", True), "resolved")
        self.assertEqual(rc.identity_status("resolved", "eligible", "source_plain", False), "conditional")
        self.assertEqual(rc.identity_status("conditional", "conditional", "source_plain", True), "conditional")
        self.assertEqual(rc.identity_status("resolved", "eligible", "conditional", True), "conditional")
        self.assertEqual(rc.identity_status("resolved", "eligible", "unresolved", True), "unresolved")
        self.assertEqual(rc.identity_status("resolved", "non_vocab", "source_plain", True), "n/a")

    def test_conflation_resolved_only_by_v16_routes(self):
        r = node(m_disc_status="conflated", lemma_v16_rel="morpheus_number")
        self.assertEqual(rc.identity_for_route("T1A1", r)[0], "conditional")
        self.assertEqual(rc.identity_for_route("T1A1X", r), ("resolved", "spliced_v1.6_XR-DISC-1", "v1.6"))
        self.assertEqual(rc.identity_for_route("T1A2", r), ("resolved", "native_v1.6", "v1.6"))

    def test_conditional_eligibility_is_never_relabelled(self):
        r = node(m_eligibility="conditional", m_rule="R03P", m_upos_status="conditional")
        for route in rc.ROUTES:
            self.assertEqual(rc.identity_for_route(route, r)[0], "conditional")
            self.assertIn("BI-ELIG-PARTICLE", rc.blockers_for(route, r, "exact"))

    def test_non_vocab_conjunction_needs_text_only(self):
        r = node(m_eligibility="non_vocab", m_rule="R08X", m_upos_status="conditional")
        self.assertEqual(rc.blockers_for("T1A2", r, "exact"), [])
        self.assertEqual(rc.blockers_for("T1A2", r, "variant_positional"), ["BT-S125-VARIANT"])

    def test_unprinted_has_no_blockers(self):
        r = node(_is_art=True, m_eligibility="unprinted")
        self.assertEqual(rc.text_status("T1A2", r)[0], "not_printed")
        self.assertEqual(rc.blockers_for("T1A2", r, "not_printed"), [])


class Anchors(unittest.TestCase):
    def setUp(self):
        self.words = ["a", "b", "c", "d"]
        self.bnode = {"a": {"correspondence": "exact_equal", "canon_keys": "cg2:w0009"},
                      "b": {"correspondence": "concat_window", "canon_keys": "cg2:w0010"},
                      "c": {"correspondence": "concat_window", "canon_keys": "cg2:w0010"},
                      "d": {"correspondence": "exact_equal", "canon_keys": "cg2:w0011"}}

    def test_anchored_pair(self):
        self.assertTrue(rc.check_anchor(self.words, 1, 2, self.bnode, 10))

    def test_gap_between_anchors_fails(self):
        self.bnode["d"]["canon_keys"] = "cg2:w0012"
        self.assertFalse(rc.check_anchor(self.words, 1, 2, self.bnode, 10))

    def test_non_exact_neighbour_fails(self):
        self.bnode["a"]["correspondence"] = "fold_equal"
        self.assertFalse(rc.check_anchor(self.words, 1, 2, self.bnode, 10))

    def test_edge_of_sequence_fails(self):
        self.assertFalse(rc.check_anchor(self.words, 0, 1, self.bnode, 10))


class RouteText(unittest.TestCase):
    def test_baseline_keeps_input_statuses(self):
        self.assertEqual(rc.text_status("T1A1", node(b_correspondence="fold_equal", xr_elision="verified"))[0], "conditional_fold")
        self.assertEqual(rc.text_status("T1A1", node(b_correspondence="concat_window", xr_span="shared_span_verified"))[0], "candidate_concat")

    def test_reconciled_routes_need_verification(self):
        self.assertEqual(rc.text_status("T1A1X", node(b_correspondence="fold_equal", xr_elision="failed"))[0], "unbound")
        self.assertEqual(rc.text_status("T1A1X", node(b_correspondence="concat_window", xr_span="unverified"))[0], "candidate_concat")
        self.assertEqual(rc.text_status("T1A2", node(b_correspondence="concat_window", xr_span="shared_span_verified"))[0], "shared_span_anchor")

    def test_reversed_components_stay_blocked_everywhere(self):
        r = node(b_correspondence="concat_reversed", xr_span="anchored_order_conflict", m_component="C-ENCLITIC:G02:host:x")
        for route in ("T1A1", "T1A1X", "T1A2"):
            self.assertEqual(rc.text_status(route, r)[0], "blocked_component_order")
        self.assertEqual(rc.text_status("T2A2", r)[0], "producer_notation_not_printable")

    def test_annotation_carried_text_is_never_exact(self):
        self.assertEqual(rc.text_status("T2A2", node())[0], "own_surface_unspaced")
        self.assertNotIn("own_surface_unspaced", rc.TEXT_OK)

    def test_t2_editorial_markup_blocker(self):
        r = node(canon_flags="del")
        self.assertIn("BT-T2-EDITORIAL-MARKUP", rc.blockers_for("T2A2", r, "own_surface_unspaced"))

    def test_v16_nfc_equivalent_only_in_a2(self):
        r = node(v16_text_rel="nfc_equal")
        self.assertEqual(rc.text_status("T1A2", r)[0], "exact_nfc_equivalent")
        self.assertEqual(rc.text_status("T1A1X", r)[0], "exact")


@unittest.skipUnless(INPUTS, "set RECON_INPUTS to a fetch_inputs.sh directory")
class SourceBacked(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.out = os.path.join(INPUTS, "regenerated-manifest")
        shutil.rmtree(cls.out, ignore_errors=True)
        cls.ledger = rc.main(INPUTS, cls.out)
        with open(os.path.join(cls.out, "route_nodes.tsv"), encoding="ascii") as f:
            cls.route = {(r["route"], r["node_key"]): r for r in csv.DictReader(f, delimiter="\t")}
        with open(os.path.join(cls.out, "nodes.tsv"), encoding="ascii") as f:
            cls.nodes = list(csv.DictReader(f, delimiter="\t"))

    def test_byte_identical_to_committed(self):
        names = sorted(os.listdir(MANIFEST))
        self.assertEqual(names, sorted(os.listdir(self.out)))
        _, mismatch, errors = filecmp.cmpfiles(MANIFEST, self.out, names, shallow=False)
        self.assertEqual((mismatch, errors), ([], []))

    def test_population_one_to_one(self):
        self.assertEqual(len(self.nodes), 2834)
        self.assertEqual(len({r["node_key"] for r in self.nodes}), 2834)
        cls = {}
        for r in self.nodes:
            cls[r["node_class"]] = cls.get(r["node_class"], 0) + 1
        self.assertEqual(cls, {"word": 2428, "punct": 374, "artificial": 32})
        self.assertEqual(len(self.route), 2834 * len(rc.ROUTES))

    def test_every_route_accounts_for_every_node(self):
        for route, v in self.ledger["routes"].items():
            self.assertEqual(sum(v["readiness_counts"].values()), 2834, route)
            self.assertEqual(sum(v["exact_occurrences"]["text_status"].values()), 2834, route)

    def test_verification_counts(self):
        v = self.ledger["verification"]
        self.assertEqual(v["xr_elision_verified"], 50)
        self.assertEqual(v["xr_elision_nfc_composes_v21"], 5)
        self.assertEqual(v["xr_span_shared_span_verified"], 22)
        self.assertEqual(v["xr_span_anchored_order_conflict"], 4)
        self.assertEqual(v["exact_equal_raw_equal"], 2346)

    def test_mapping_counts_preserved_in_baseline(self):
        ids = self.ledger["routes"]["T1A1"]["identities"]["node_identity_status"]
        self.assertEqual((ids["resolved"], ids["conditional"], ids["unresolved"]), (1145, 249, 1))

    def test_no_route_passes_vocabulary_readiness(self):
        self.assertFalse(self.ledger["verdict"]["vocabulary_ready_any_route"])

    def test_fixture_rows(self):
        with open(FIXTURE, encoding="ascii") as f:
            for exp in csv.DictReader(f, delimiter="\t"):
                got = self.route[(exp["route"], exp["node_key"])]
                for field in ("text_status", "identity_status", "readiness", "blockers"):
                    self.assertEqual(got[field], exp[field], (exp["route"], exp["node_key"], field, exp["why"]))

    def test_outputs_are_ascii(self):
        for name in os.listdir(self.out):
            with open(os.path.join(self.out, name), "rb") as f:
                self.assertTrue(all(b < 128 for b in f.read()), name)

    def test_ledger_is_json(self):
        with open(os.path.join(self.out, "route_ledger.json"), encoding="ascii") as f:
            self.assertEqual(json.load(f)["preferred_route"], "T1A2")


if __name__ == "__main__":
    unittest.main(verbosity=1)
