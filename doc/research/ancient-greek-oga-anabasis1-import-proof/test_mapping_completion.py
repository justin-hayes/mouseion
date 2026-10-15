"""Invented Morpheus-shaped fixtures: identity and POS rules must fail closed."""

import unittest

import mapping_completion as mc


def row(key, cls="lemlist", derivation="", line=1, file="lemlist"):
    head, digits = mc.decode_beta(key)
    return {"file": file, "line": line, "class": cls, "key": key, "derivation": derivation,
            "head": mc.bare(head), "headword": head, "digits": digits,
            "starred": key.startswith("*"), "loose": False}


def label(raw):
    head, digits = raw.rstrip("0123456789"), raw[len(raw.rstrip("0123456789")):]
    return {"raw_lemma": raw, "headword": mc.norm_head(head), "discriminator": digits,
            "occurrences": ["t_1"]}


class DecodeTests(unittest.TestCase):
    def test_digits_are_discriminators_not_spelling(self):
        self.assertEqual(mc.decode_beta("le/gw3"), ("λέγω", "3"))
        self.assertEqual(mc.decode_beta("h)/1"), ("ἤ", "1"))
        self.assertEqual(mc.decode_beta("su/neimi2"), ("σύνειμι", "2"))

    def test_starred_key_with_breathing_decodes(self):
        self.assertEqual(mc.decode_beta("*)ikma/lios"), ("ἰκμάλιος", ""))

    def test_unknown_characters_fail_closed(self):
        with self.assertRaises(mc.BetaError):
            mc.decode_beta("pro/-ei)mi/")


class IdentityTests(unittest.TestCase):
    def test_single_key_resolves(self):
        index = [row("le/gw3")]
        self.assertEqual(mc.identity_decision(label("λέγω3"), index)["decision"], "key-resolved")

    def test_digit_mutation_does_not_resolve(self):
        index = [row("le/gw3")]
        self.assertEqual(mc.identity_decision(label("λέγω2"), index)["decision"],
                         "digit-absent-in-pinned-source")

    def test_shared_output_key_from_two_derivations_is_ambiguous(self):
        index = [row("pro/seimi1", "compound", "pro/s-ei)=mi", 5639, "vbs.cmp.ml"),
                 row("pro/seimi1", "compound", "pro/s-ei)mi/", 5643, "vbs.cmp.ml")]
        decision = mc.identity_decision(label("πρόσειμι1"), index)
        self.assertEqual(decision["decision"], "ambiguous-shared-key")

    def test_loose_record_never_resolves(self):
        loose = row("le/gw3")
        loose["loose"] = True
        decision = mc.identity_decision(label("λέγω3"), [loose])
        self.assertEqual(decision["decision"], "not-established-loose-record")

    def test_absent_headword_is_not_called_resolved(self):
        decision = mc.identity_decision(label("σύνλέγω2"), [row("sulle/gw", "compound",
                                                                 "su/n-le/gw2", 6459, "vbs.cmp.ml")])
        self.assertEqual(decision["decision"], "not-established-no-key")
        self.assertEqual(len(decision["near_derivations"]), 1)


class PosTests(unittest.TestCase):
    def test_punctuation_is_outside_allowed_pos(self):
        decision = mc.pos_decision("u", "AuxK", "·", False, "n/a")
        self.assertEqual(decision["status"], "resolved-outside-allowed-pos")

    def test_adjective_adverb_status_conflict_blocks(self):
        decision = mc.pos_decision("a", "ADV", "δαρεῖος", False, "n/a")
        self.assertEqual(decision["status"], "blocked")

    def test_proper_common_ambiguity_blocks_noun(self):
        self.assertEqual(mc.pos_decision("n", "SBJ", "κῦρος", False, "both-or-neither")["status"],
                         "blocked")
        self.assertEqual(mc.pos_decision("n", "SBJ", "κῦρος", False, "common")["status"],
                         "resolved-eligible")
        self.assertEqual(mc.pos_decision("n", "SBJ", "δαρεῖος", False, "proper")["status"],
                         "resolved-outside-allowed-pos")

    def test_copula_requires_pnom_dependent(self):
        self.assertEqual(mc.pos_decision("v", "PRED", "εἰμί", True, "n/a")["candidates"], ["AUX"])
        self.assertEqual(mc.pos_decision("v", "PRED", "εἰμί", False, "n/a")["candidates"], ["VERB"])

    def test_particle_with_conjunction_function_blocks(self):
        self.assertEqual(mc.pos_decision("d", "COORD", "δέ", False, "n/a")["status"], "blocked")

    def test_upos_status_requires_single_or_outside(self):
        self.assertEqual(mc.upos_status(["NOUN"]), "resolved-eligible")
        self.assertEqual(mc.upos_status(["NOUN", "PROPN"]), "blocked")
        self.assertEqual(mc.upos_status(["PRON", "DET"]), "resolved-outside-allowed-pos")


if __name__ == "__main__":
    unittest.main()
