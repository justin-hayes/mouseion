#!/usr/bin/env python3
"""Tests for lexical_completion.py (#2049).

Synthetic tests always run. Source-backed tests run only when both environment variables are
set: LEX_MANIFEST (the #2048 reconciliation manifest directory) and LEX_SOURCES (the directory
holding the pinned sources, verified against inputs/sha256sums.txt).

Run: LEX_MANIFEST=... LEX_SOURCES=... python3 -I -m unittest -v test_lexical_completion.py
"""
import importlib.util
import os
import shutil
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC = importlib.util.spec_from_file_location('lexical_completion', os.path.join(HERE, 'lexical_completion.py'))
lc = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(lc)

SCRATCH = os.path.expanduser('~/.cache/scratch/greek-lysias1-lexical-completion')


class Synthetic(unittest.TestCase):
    def test_codepoint_roundtrip(self):
        text = 'καί'
        self.assertEqual(lc.str2cp(text), 'U+03BA U+03B1 U+03AF')
        self.assertEqual(lc.cp2str(lc.str2cp(text)), text)

    def test_split_homograph(self):
        self.assertEqual(lc.split_homograph('su/n-le/gw2'), ('su/n-le/gw', '2'))
        self.assertEqual(lc.split_homograph('a)nti/'), ('a)nti/', ''))

    def test_lemma_status_keeps_accent_variants_apart(self):
        tokens = ['καί', 'τέ']
        self.assertEqual(lc.lemma_status('καί', tokens), 'exact')
        self.assertEqual(lc.lemma_status('καὶ', tokens), 'accent_variant')
        self.assertEqual(lc.lemma_status('τε', tokens), 'accent_variant')
        self.assertEqual(lc.lemma_status('ἄν', tokens), 'absent')

    def test_doc_tokens_respect_word_boundaries(self):
        tokens = lc.doc_tokens('<p>ὅτε καί</p>')
        self.assertIn('καί', tokens)
        self.assertNotIn('τε', tokens)

    def test_agdt_list_skips_inline_markers(self):
        guideline = ('4.1.12.1 Particle\n'
                     'Particle are here defined as those adverbs such as:\n'
                     'ἀλλά\nδέ  tree1\nτε\n'
                     'Some of these particles can be adverbs.\nαὖ\n')
        self.assertEqual(lc.agdt_conjunction_particles(guideline), ['ἀλλά', 'δέ', 'τε'])

    def test_smyth_span_stops_at_next_heading(self):
        page = ('PARTICLES\nSOME SIDEBAR\nἀλλά\nἄρα\n'
                'MAIN TEXT\nPARTICLES\n\nἀλλά\n\nγάρ\n(\nδέ\n)\nAdverbial\nμέν\n'
                'SOME GRAMMATICAL AND RHETORICAL FIGURES\nοὖν\n')
        tokens = lc.smyth_particle_tokens(page)
        self.assertEqual(tokens, ['ἀλλά', 'γάρ', 'δέ', 'μέν'])

    def test_le_block_and_h1_check(self):
        text = ':le:bole/w\n:de:bol ew_denom pb:a)nti/\n\n:le:a)ntibole/w\n:de:x\n'
        blocks = lc.le_blocks(text)
        self.assertEqual([k for k, _, _ in blocks], ['bole/w', 'a)ntibole/w'])
        self.assertTrue(lc.has_token(blocks[0][2], 'pb:a)nti/'))
        self.assertFalse(lc.has_token(blocks[1][2], 'pb:a)nti/'))

    def test_decisions_by_rule(self):
        ctx = {'smyth': {'καί': 'exact', 'καὶ': 'accent_variant', 'ἄν': 'absent', 'γάρ': 'exact',
                         'οὐ': 'absent', 'εἰμί': 'absent'},
               'named_grc': {'καί': ['ud_grc_cconj'], 'καὶ': [], 'ἄν': [], 'γάρ': [], 'οὐ': [], 'εἰμί': []},
               'named_gen': {}, 'agdt': {}, 'name': {}, 'hw': {}}

        def node(rule, lemma, postag='d--------'):
            return {'m_rule': rule, 'lemma': lemma, 'postag_v21': postag, 'm_headword_cp': ''}

        open_c = lc.decide(node('R03C', 'καί'), ctx)
        self.assertEqual((open_c['pid'], open_c['upos']), ('P02-CONJPART-OPEN', ''))
        hold = lc.decide(node('R03P', 'καὶ'), ctx)
        self.assertEqual(hold['pid'], 'P04-VARIANT-HOLD')
        part = lc.decide(node('R03P', 'ἄν'), ctx)
        self.assertEqual((part['pid'], part['upos']), ('P01-PARTICLE-ADV', 'ADV'))
        self.assertIn('Smyth 2769 PARTICLES: absent', part['unsupported'])
        neg = lc.decide(node('R03N', 'οὐ'), ctx)
        self.assertEqual(neg['upos'], 'PART')
        self.assertEqual(lc.decide(node('R05X', 'εἰμί', 'v3spoa---'), ctx)['upos'], 'VERB')
        self.assertEqual(lc.decide(node('R05X', 'εἰμί', 'v3srim---'), ctx)['upos'], 'AUX')

    def test_labels(self):
        self.assertEqual(lc.labels({'upos': '', 'unsupported': '', 'unperformed': ''}), 'exact_fact')
        self.assertEqual(lc.labels({'upos': 'ADV', 'unsupported': 'x', 'unperformed': 'y'}),
                         'exact_fact;proposed_interpretation;unsupported_case;unperformed_check')

    def test_verify_inputs_detects_mismatch(self):
        os.makedirs(SCRATCH, exist_ok=True)
        tmp = tempfile.mkdtemp(dir=SCRATCH)
        try:
            os.makedirs(os.path.join(tmp, 'a'))
            with open(os.path.join(tmp, 'a', 'f.txt'), 'w') as f:
                f.write('x')
            sums = os.path.join(tmp, 'sums.txt')
            with open(sums, 'w') as f:
                f.write('0' * 64 + '  a/f.txt\n')
            original = lc.SUMS
            lc.SUMS = sums
            try:
                with self.assertRaises(SystemExit):
                    lc.verify_inputs(tmp)
            finally:
                lc.SUMS = original
        finally:
            shutil.rmtree(tmp)

    def test_write_tsv_is_ascii(self):
        os.makedirs(SCRATCH, exist_ok=True)
        tmp = tempfile.mkdtemp(dir=SCRATCH)
        try:
            path = os.path.join(tmp, 'x.tsv')
            lc.write_tsv(path, [{'a': 'καί', 'b': 'x'}])
            with open(path, 'rb') as f:
                data = f.read()
            data.decode('ascii')
            self.assertIn(b'U+03BA', data)
        finally:
            shutil.rmtree(tmp)


SRC_ENV = ('LEX_MANIFEST', 'LEX_SOURCES')
HAVE_SOURCES = all(os.environ.get(k) for k in SRC_ENV)


@unittest.skipUnless(HAVE_SOURCES, 'set LEX_MANIFEST and LEX_SOURCES for source-backed tests')
class SourceBacked(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        os.makedirs(SCRATCH, exist_ok=True)
        cls.tmp = tempfile.mkdtemp(dir=SCRATCH)
        cls.out = os.path.join(cls.tmp, 'out')
        cls.summary, cls.rows = lc.build(os.environ['LEX_MANIFEST'], os.environ['LEX_SOURCES'], cls.out)

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tmp)

    def test_totals_and_blockers(self):
        s = self.summary
        self.assertEqual(s['nodes'], 2834)
        self.assertEqual(s['node_classes'], {'artificial': 32, 'punct': 374, 'word': 2428})
        self.assertEqual(s['t1a2_vocab_blocked'], 235)
        self.assertEqual(s['blocker_counts']['BI-ELIG-PARTICLE'], 222)
        self.assertEqual(s['blocker_counts']['BI-ELIG-NAME'], 8)
        self.assertEqual(s['blocker_counts']['BI-ELIG-EIMI'], 2)
        self.assertEqual(s['overlap_BI_ELIG_PARTICLE_and_BT_S011_ORDER'], ['s011w026', 's011w029'])

    def test_proposals_partition_the_235(self):
        self.assertEqual(sum(self.summary['proposal_counts'].values()), 235)
        self.assertEqual(len(self.rows), 235)

    def test_no_eligibility_changes(self):
        self.assertEqual(self.summary['eligibility_changes'], 0)
        self.assertTrue(all(r['adoption_status'].startswith('not_adopted') for r in self.rows))

    def test_smyth_and_agdt_occurrence_counts(self):
        self.assertEqual(self.summary['particle_occurrences_smyth_2769'],
                         {'absent': 65, 'accent_variant': 17, 'exact': 140})
        self.assertEqual(self.summary['particle_occurrences_agdt_4_1_12_1'],
                         {'absent': 144, 'accent_variant': 8, 'exact': 70})
        self.assertEqual(len(self.summary['agdt_conjunction_particles']), 13)

    def test_regeneration_is_byte_identical(self):
        second = os.path.join(self.tmp, 'out2')
        lc.build(os.environ['LEX_MANIFEST'], os.environ['LEX_SOURCES'], second)
        for name in sorted(os.listdir(self.out)):
            with open(os.path.join(self.out, name), 'rb') as a, open(os.path.join(second, name), 'rb') as b:
                self.assertEqual(a.read(), b.read(), name)


if __name__ == '__main__':
    unittest.main()
