#!/usr/bin/env python3
"""Tests for the Lysias I vocabulary-mapping research (rules v2).

Stdlib only. Run after fetch_inputs.sh:

  LYSIAS1_INPUTS=<dest> python3 -I test_mapping.py

Without LYSIAS1_INPUTS only the unit and committed-manifest tests run; input-dependent tests are skipped.
"""
import csv
import json
import os
import re
import shutil
import sys
import tempfile
import unittest
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)  # the script directory is not on sys.path under python3 -I

import derive_evidence as de  # noqa: E402

RESEARCH = os.path.abspath(os.path.join(HERE, ".."))
MANIFEST = os.path.join(RESEARCH, "manifest")
FIXTURES = os.path.join(RESEARCH, "fixtures", "lysias1-expected.tsv")
REPO_ROOT = os.path.abspath(os.path.join(RESEARCH, "..", "..", ".."))
SELECTION_GO = os.path.join(REPO_ROOT, "internal", "selection", "selection.go")
INPUTS = os.environ.get("LYSIAS1_INPUTS", "")
SCRATCH = os.path.expanduser(os.environ.get("LYSIAS1_SCRATCH", "~/.cache/scratch/lysias1-mapping-test"))


def read_tsv(path):
    with open(path, encoding="utf-8", newline="") as handle:
        return list(csv.DictReader(handle, delimiter="\t"))


def read_json(path):
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def have_inputs():
    return bool(INPUTS) and os.path.isfile(os.path.join(INPUTS, de.V21))


def node(sid="1", wid="1", head="0", lemma="λόγος", postag="n-s---mn-", relation="OBJ", position=None, artificial=""):
    return {"sentence_id": sid, "subdoc": "", "word_id": wid, "position": position or int(wid), "raw_form": "x",
            "form": "x", "lemma": lemma, "postag": postag, "relation": relation, "head": head, "artificial": artificial}


class FakeMorpheus:
    proper = {"*sw/stratos": 1}


def classify(nodes, index, rubrics=()):
    return de.classify(nodes[index], de.Tree(nodes), FakeMorpheus(), set(rubrics))


class KeyAndBetaTests(unittest.TestCase):
    def test_canonical_key_case_and_sigma_only(self):
        self.assertEqual(de.canonical_key("Νόμος"), "νόμοσ")
        self.assertNotEqual(de.canonical_key("καί"), de.canonical_key("καὶ"))

    def test_beta_round_trip(self):
        for word, beta in (("καθεύδω", "kaqeu/dw"), ("ἀμφισβητέω", "a)mfisbhte/w"), ("Ἐρατοσθένης", "*)eratosqe/nhs"),
                           ("Ἁρμόδιος", "*(armo/dios"), ("σύν", "su/n"), ("διῃτώμην", "dih|tw/mhn")):
            self.assertEqual(de.to_beta(word), beta)
            self.assertEqual(de.from_beta(beta), word)
        self.assertEqual(de.from_beta("sulle/gw2"), "συλλέγω")


class MorpheusDecodeTests(unittest.TestCase):
    def setUp(self):
        self.root = tempfile.mkdtemp()
        stems = os.path.join(self.root, "stemlib/Greek/stemsrc")
        os.makedirs(stems)
        files = {
            "vbs.simp.ml": ":le:kaqeu/dw\n:vs:kaqeud w_stem rpb:kata/ imperf ind\n\n:le:a)ntibole/w\n:de:bol ew_denom pb:a)nti/\n",
            "lsj.vbs": ":le:bole/w\n:de:bol ew_denom\n\n:le:le/gw2\n:de:leg reg_conj\n",
            "vbs.irreg": "",
            "vbs.cmp.ml": "su/n-le/gw2 sulle/gw\n",
            "vbs.cmp.lsj": "",
            "nom.proper": ":le:*sw/stratos\n",
        }
        for name, text in files.items():
            with open(os.path.join(stems, name), "w", encoding="utf-8") as handle:
                handle.write(text)
        self.m = de.Morpheus(self.root)

    def tearDown(self):
        shutil.rmtree(self.root)

    def test_root_preverb_decodes_to_base(self):
        status, rule, word, _, _ = self.m.decode("κατά-καθεύδω", "")
        self.assertEqual((status, rule, word), ("decoded", "H1", "καθεύδω"))

    def test_compound_table_requires_same_homograph_number(self):
        self.assertEqual(self.m.decode("σύν-λέγω", "2")[:3], ("decoded", "H2", "συλλέγω"))
        status, rule, word, cands, _ = self.m.decode("σύν-λέγω", "1")
        self.assertEqual((status, rule, word, cands), ("unresolved", "H4", "", ["συλλέγω"]))

    def test_decomposition_match_is_only_conditional(self):
        self.assertEqual(self.m.decode("ἀντί-βολέω", "")[:3], ("conditional", "H3", "ἀντιβολέω"))

    def test_unknown_is_unresolved_not_dehyphenated(self):
        status, _, word, _, _ = self.m.decode("ἀπό-ξενόω", "")
        self.assertEqual((status, word), ("unresolved", ""))


class UposRuleTests(unittest.TestCase):
    def test_copula_through_auxp_and_coordination(self):
        auxp = [node(wid="1", lemma="εἰμί", postag="v3spia---", relation="PRED"),
                node(wid="2", head="1", lemma="ἐν", postag="r--------", relation="AuxP"),
                node(wid="3", head="2", lemma="οἶκος", relation="PNOM")]
        self.assertEqual(classify(auxp, 0)[:2], ("R05C", ["AUX"]))
        coord = [node(wid="1", lemma="εἰμί", postag="v3spia---", relation="PRED"),
                 node(wid="2", head="1", lemma="καί", postag="c--------", relation="COORD"),
                 node(wid="3", head="2", lemma="ἀγαθός", postag="a-s---mn-", relation="PNOM_CO"),
                 node(wid="4", head="2", lemma="καλός", postag="a-s---mn-", relation="PNOM_CO")]
        self.assertEqual(classify(coord, 0)[0], "R05C")

    def test_eimi_without_pnom_and_become_verbs(self):
        bare = [node(wid="1", lemma="εἰμί", postag="v3spia---", relation="PRED")]
        self.assertEqual(classify(bare, 0)[:2], ("R05X", ["VERB", "AUX"]))
        become = [node(wid="1", lemma="γίγνομαι", postag="v3saim---", relation="PRED"),
                  node(wid="2", head="1", lemma="ἀγαθός", postag="a-s---mn-", relation="PNOM")]
        self.assertEqual(classify(become, 0)[:2], ("R06", ["VERB"]))

    def test_particles(self):
        verb = node(wid="9", lemma="ποιέω", postag="v3siia---", relation="PRED")
        cases = {("καί", "AuxZ"): "R03K", ("οὐ", "AuxZ"): "R03N", ("δέ", "AuxY"): "R03C", ("γάρ", "AuxY"): "R03A",
                 ("μέν", "AuxY"): "R03P", ("καὶ", "AuxZ"): "R03P", ("νῦν", "ADV"): "R03"}
        for (lemma, rel), rule in cases.items():
            nodes = [node(wid="1", head="9", lemma=lemma, postag="d--------", relation=rel), verb]
            self.assertEqual(classify(nodes, 0)[0], rule, lemma)

    def test_correlative_needs_repeated_coordinator(self):
        nodes = [node(wid="1", head="3", lemma="καί", postag="d--------", relation="AuxY"),
                 node(wid="2", head="3", lemma="δέ", postag="d--------", relation="AuxY"),
                 node(wid="3", lemma="καί", postag="c--------", relation="COORD")]
        self.assertEqual(classify(nodes, 0)[:2], ("R03Y", ["CCONJ"]))
        self.assertEqual(classify(nodes, 1)[0], "R03C")

    def test_nouns(self):
        self.assertEqual(classify([node(lemma="Σώστρατος")], 0)[:2], ("R01P", ["PROPN"]))
        self.assertEqual(classify([node(lemma="Ἑλλάς")], 0)[:2], ("R01C", ["NOUN", "PROPN"]))
        self.assertEqual(classify([node(lemma="Ἀθηναῖος")], 0)[0], "R01A")
        self.assertEqual(classify([node(lemma="Νόμος", relation="ExD")], 0, rubrics={"1"})[:2], ("R01R", ["NOUN"]))
        self.assertEqual(classify([node()], 0)[:2], ("R01", ["NOUN"]))

    def test_auxc_needs_a_verbal_clause(self):
        clause = [node(wid="1", lemma="ὅτι", postag="c--------", relation="AuxC"),
                  node(wid="2", head="1", lemma="ποιέω", postag="v3siia---", relation="OBJ")]
        self.assertEqual(classify(clause, 0)[0], "R08")
        phrase = [node(wid="1", lemma="ἤ", postag="c--------", relation="AuxC"),
                  node(wid="2", head="1", lemma="ἀνήρ", relation="ADV")]
        self.assertEqual(classify(phrase, 0)[0], "R08X")

    def test_eligibility(self):
        self.assertEqual(de.eligibility_of(["ADV"], "δέ", "AuxY"), "eligible")
        self.assertEqual(de.eligibility_of(["ADV", "PART"], "δέ", "AuxY"), "conditional")
        self.assertEqual(de.eligibility_of(["PROPN"], "Σώστρατος", "SBJ"), "non_vocab")
        self.assertFalse(de.repo_eligible("VERB", "λύω", "compound:prt"))

    def test_allowed_pos_mirrors_repo(self):
        self.assertEqual(de.ALLOWED_UPOS, frozenset({"NOUN", "VERB", "ADJ", "ADV"}))
        if os.path.isfile(SELECTION_GO):
            with open(SELECTION_GO, encoding="utf-8") as handle:
                match = re.search(r'AllowedPOS: map\[string\]bool\{([^}]*)\}', handle.read())
            self.assertEqual(set(re.findall(r'"([A-Z]+)": true', match.group(1))), set(de.ALLOWED_UPOS))


class CommittedManifestTests(unittest.TestCase):
    """Input-independent checks of the committed manifests."""

    @classmethod
    def setUpClass(cls):
        cls.rows = read_tsv(os.path.join(MANIFEST, "lysias1-nodes.tsv"))
        cls.ids = read_tsv(os.path.join(MANIFEST, "lysias1-identities.tsv"))
        cls.summary = read_json(os.path.join(MANIFEST, "lysias1-summary.json"))

    def test_totals_reconcile(self):
        s = self.summary
        self.assertEqual((s["nodes_total"], s["nodes_printed"], s["nodes_unprinted_elliptic"]), (2834, 2802, 32))
        self.assertTrue(all(s["reconciliation"].values()))
        self.assertTrue(s["node_classification_complete"])
        self.assertEqual(len(self.rows), 2834)

    def test_conditional_upos_never_yields_resolved_identity(self):
        for r in self.rows:
            if r["upos_status"] != "resolved" or r["headword_status"] in ("conditional", "unresolved") \
                    or r["eligibility"] == "conditional" or r["discriminator_status"] == "conflated":
                self.assertNotEqual(r["identity_status"], "resolved", r["node_key"])

    def test_identity_aggregates_match_nodes(self):
        resolved = sum(1 for r in self.rows if r["identity_status"] == "resolved")
        self.assertEqual(sum(int(i["occurrences_resolved"]) for i in self.ids), resolved)
        for i in self.ids:
            if i["identity_status"] == "resolved":
                self.assertGreater(int(i["occurrences_resolved"]), 0, i["identity"])

    def test_unresolved_headwords_keep_raw_lemma(self):
        for r in self.rows:
            if r["headword_status"] == "unresolved":
                self.assertIn("-", r["native_lemma"])
                self.assertEqual(r["canonical_key"], "")

    def test_source_backed_fixtures(self):
        rows = {r["node_key"]: r for r in self.rows}
        for f in read_tsv(FIXTURES):
            r = rows[f["node_key"]]
            for col in ("rule_id", "upos_candidates", "eligibility", "citation_headword", "headword_status",
                        "discriminator_value", "discriminator_status", "identity", "identity_status"):
                self.assertEqual(r[col], f[col], f"{f['node_key']} {col}")
            for ev in f["evidence_ids_include"].split():
                self.assertIn(ev, r["evidence_ids"].split(), f["node_key"])

    def test_no_per_token_ud_fields_committed(self):
        with open(os.path.join(MANIFEST, "lysias1-nodes.tsv"), encoding="utf-8") as handle:
            header = handle.readline()
        self.assertNotIn("ud_", header)
        self.assertNotIn("feats", header)


@unittest.skipUnless(have_inputs(), "set LYSIAS1_INPUTS to the fetch_inputs.sh destination")
class InputTests(unittest.TestCase):
    def test_pinned_scheme_is_1_1(self):
        root = ET.parse(os.path.join(INPUTS, de.V21)).getroot()
        self.assertIn("1.1", root.findtext("header/annotationScheme"))

    def test_lysias1_absent_from_ud(self):
        self.assertEqual(read_json(os.path.join(MANIFEST, "ud-crosscheck.json"))["lysias1_sentences_in_ud"], 0)

    def test_regeneration_is_byte_identical(self):
        os.makedirs(SCRATCH, exist_ok=True)
        out = tempfile.mkdtemp(dir=SCRATCH)
        try:
            de.derive(INPUTS, out)
            self.assertEqual(sorted(os.listdir(MANIFEST)), sorted(os.listdir(out)))
            for name in sorted(os.listdir(MANIFEST)):
                with open(os.path.join(MANIFEST, name), "rb") as a, open(os.path.join(out, name), "rb") as b:
                    self.assertEqual(a.read(), b.read(), name)
        finally:
            shutil.rmtree(out, ignore_errors=True)


if __name__ == "__main__":
    unittest.main(verbosity=2)
