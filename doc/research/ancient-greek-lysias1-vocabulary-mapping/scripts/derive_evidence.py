#!/usr/bin/env python3
"""Derive the Lysias I vocabulary-mapping manifest and its evidence (rules v2).

Standard library only. Run with `python3 -I`:

  python3 -I derive_evidence.py INPUTS_DIR OUT_DIR

INPUTS_DIR is produced by fetch_inputs.sh. OUT_DIR receives:
  lysias1-nodes.tsv        one row per AGDT <word> node (2,834 for Lysias I)
  lysias1-identities.tsv   vocabulary identities, with resolved/conditional/unresolved occurrence counts
  lysias1-rules.tsv        versioned UPOS, headword, discriminator and component rules
  lysias1-evidence.tsv     evidence IDs -> pinned source, path, locator, sha256 and claim
  lysias1-components.tsv   component appendix: elision marks, crasis and enclitic splits
  lysias1-lineage.json     review/conversion lineage of the consumed lemma and POS layers
  lysias1-summary.json     reconciled counts
  annotation-practice.json per-file particle letter practice (native fields, counts only)
  ud-crosscheck.json       aggregate UD agreement counts (no per-token UD data)

Per-token UD fields are used in memory only and never written out (UD Perseus is
CC BY-NC-SA 2.5; see NOTICE.md).
"""
import collections
import csv
import glob
import hashlib
import io
import json
import os
import re
import sys
import unicodedata
import xml.etree.ElementTree as ET

RULES_VERSION = "lysias1-upos-rules/2026-10-14.2"
HEADWORD_RULES_VERSION = "lysias1-headword-rules/2026-10-14.1"
COMPONENT_RULES_VERSION = "lysias1-component-conventions/2026-10-14.1"
NORMALIZATION_PROFILE = "mouseion ancient-greek v1: NFC, Unicode lowercase, final sigma to sigma, NFC (#2033)"
UNICODE_VERSION = unicodedata.unidata_version
SCHEME_DECLARED = "GSAAGDT 1.1"  # Lysias I <annotationScheme>; test_mapping.py checks it
LYSIAS_FILE = "tlg0540.tlg001.perseus-grc1.tb.xml"
LYSIAS_DOC = "tlg0540.tlg001.perseus-grc1@treebank_data:bf4334f0:v2.1"
LYSIAS_SHA256 = "44ecf3a29e33fe8a173ef1d08b43dc9e52c9e066cacbd642050d95af83d7a568"
UD_COMMIT = "331ddef91411d0e6549744ee889e05549e6da77d"
TB_COMMIT = "bf4334f0af5e13d16b04c1cccd6237e683ac6f5f"
MORPHEUS_COMMIT = "ab6898ffed335fc6169fa02c9940657a9b5a78e0"
UDDOCS_COMMIT = "24add7d0b041d11432f6694424111f1b7f50aa86"
UDTOOLS_COMMIT = "77751a634188d0f9e2cfe104a24a65978f5bb1c4"
TREEX_COMMIT = "df815f19c4d8251f13d99b396bdf32fc4f188762"
INTERSET_COMMIT = "f3568ffb0c3d73330df750fb57dd78d03ff1c0da"

V21 = "agdt/v2.1/Greek/texts/" + LYSIAS_FILE
V16 = "sources/v1.6/greek/data/" + LYSIAS_FILE
V20 = "sources/v2.0/Greek/nonArethusaCompliant/" + LYSIAS_FILE
MORPH = "ext/perseids-tools/morpheus/"
UDDOCS = "ext/UniversalDependencies/docs/"
TREEX = "ext/ufal/treex/lib/Treex/"

# internal/selection/selection.go DefaultConfig AllowedPOS. OccurrenceEligible also excludes
# dependency compound:prt and requires a letter (internal/lexical/lemma.go IsLemma).
ALLOWED_UPOS = frozenset({"NOUN", "VERB", "ADJ", "ADV"})
ELIDED = "elliptic"
COPULA_LEMMA = "εἰμί"
# AGDT 2.0 §4.1.12.1: traditional particles that "have to be always annotated as conjunctions"
# when they function as conjunctions. Matched exactly (no accent folding).
AGDT_CONJ_PARTICLES = frozenset("ἀλλά ἀτάρ δέ εἴτε ἤ ἠδέ καί καίτοι οὐδέ οὔτε μηδέ μήτε τε".split())
# AGDT 2.0 §2.8/§3.9: causal, inferential and some adversative conjunctions "are all treated
# here as (sentence) adverbs". Smyth §2163 lists these lemmas among them.
AGDT_ADVERBIALIZED_CONJ = frozenset("γάρ οὖν τοίνυν".split())
NEGATORS = frozenset("οὐ μή".split())
# UD grc NOUN documentation names this lemma as a substantivized adjective.
UD_GRC_SUBST_ADJ = frozenset({"Ἀθηναῖος"})

NODE_COLS = [
    "node_key", "binding_join_key", "doc", "sentence_id", "subdoc", "word_id", "position", "form",
    "artificial", "native_lemma", "native_postag", "native_relation", "head", "head_relation",
    "scheme_declared", "letter", "native_pos", "rule_id", "upos", "upos_candidates", "upos_status",
    "eligibility", "lemma_notation", "lemma_preverbs", "lemma_base", "citation_headword",
    "headword_status", "headword_rule", "canonical_key", "discriminator_scheme",
    "discriminator_value", "discriminator_status", "identity", "identity_candidates",
    "identity_status", "source_scope", "component_convention", "evidence_ids", "flags",
]
IDENTITY_COLS = [
    "identity", "canonical_key", "upos", "citation_headwords", "identity_status",
    "occurrences_resolved", "occurrences_conditional", "occurrences_unresolved",
    "discriminator_witness_values", "rule_ids", "flags",
]
RULE_COLS = ["rule_id", "layer", "applies_to", "result", "status", "eligibility", "printed_nodes", "evidence_ids", "note"]
EVIDENCE_COLS = ["evidence_id", "kind", "source", "revision", "path", "locator", "sha256", "claim"]
COMPONENT_COLS = [
    "node_key", "binding_join_key", "convention_id", "group_id", "role", "form", "form_marks",
    "v16_form", "v16_marks", "nfc_changes_form", "native_lemma", "vocab_effect", "status", "evidence_ids",
]

# rule_id: layer, applies_to, result, status, eligibility, evidence ids, note
RULES = {
    "R00": ("upos", "artificial=elliptic", "-", "n/a", "unprinted", "ADR0094",
            "unprinted node; no surface, not vocabulary"),
    "R01": ("upos", "n, lowercase lemma, not a name component or rubric", "NOUN", "resolved", "eligible",
            "AGDT-TAGSETS-21 AGDT-README-17 UD-GRC-NOUN", "native noun letter"),
    "R01R": ("upos", "n, sole lexical word of an ExD rubric sentence", "NOUN", "resolved", "eligible",
             "AGDT20-EXD UD-PROPN-TITLES", "rubric heading noun keeps its common-noun tag; lemma case is normalized by #2033"),
    "R01P": ("upos", "n, lemma is a :le: entry in Morpheus nom.proper", "PROPN", "resolved", "non_vocab",
             "MORPH-NOMPROPER UD-PROPN UD-GRC-PROPN REPO-ELIG",
             "proper-name stem lexicon of the producer's morphology tool; PROPN is outside AllowedPOS"),
    "R01C": ("upos", "n, capitalized lemma not in Morpheus nom.proper", "NOUN|PROPN", "conditional",
             "conditional", "UD-PROPN UD-GRC-PROPN HAMLEDT-PROPN UD-PERSEUS-CAPS",
             "no producer proper-name evidence; capitalization alone is not PROPN"),
    "R01A": ("upos", "n, lemma named by UD grc as a substantivized adjective", "ADJ|NOUN|PROPN",
             "conditional", "conditional", "UD-GRC-NOUN AGDT20-SUBSTADJ UD-PROPN",
             "native n conflicts with target-scheme guidance on this lemma"),
    "R01M": ("upos", "n, lowercase lemma syntactically joined to an adjacent R01C name word", "NOUN|PROPN",
             "conditional", "conditional", "UD-PROPN", "possible component of a multiword name"),
    "R02": ("upos", "a", "ADJ", "resolved", "eligible", "AGDT-TAGSETS-21 AGDT-README-17", "native adjective letter"),
    "R03": ("upos", "d, relation not AuxY/AuxZ", "ADV", "resolved", "eligible",
            "AGDT-TAGSETS-21 AGDT-README-17 AGDT20-ADVLEM UD-GRC-ADV INTERSET-GRC", "native adverb letter"),
    "R03K": ("upos", "d, lemma καί, AuxZ", "ADV", "resolved", "eligible",
             "AGDT20-AUXZ AGDT11-AUXZ UD-GRC-PART SMYTH-2769",
             "AuxZ marks 'even/also'; UD grc PART excludes 'adverbs such as καί even'"),
    "R03Y": ("upos", "d or c, lemma in AGDT conjunction-particle list, AuxY, head is a later COORD with the same lemma", "CCONJ",
             "resolved", "non_vocab", "AGDT11-AUXY AGDT20-COORD AGDT20-PARTICLE UD-GRC-CCONJ",
             "non-head correlative coordinator attached to the final coordinator"),
    "R03N": ("upos", "d, negator οὐ/μή, AuxY/AuxZ", "ADV|PART", "conditional", "conditional",
             "AGDT11-AUXZ UD-PART UD-GRC-PART INTERSET-GRC", "UD lists negation as a PART example; grc docs give no negation rule"),
    "R03C": ("upos", "d, lemma in AGDT conjunction-particle list, AuxY/AuxZ, not under COORD", "ADV|CCONJ",
             "conditional", "conditional", "AGDT20-PARTICLE SMYTH-2769 UD-GRC-CCONJ",
             "AGDT says these are adverbs unless they function as conjunctions; UD CCONJ is lexical"),
    "R03A": ("upos", "d, γάρ/οὖν/τοίνυν, AuxY/AuxZ", "ADV|CCONJ|PART", "conditional", "conditional",
             "AGDT20-CONJ AGDT20-COORD SMYTH-2769 UD-GRC-PART",
             "AGDT treats causal/inferential conjunctions as sentence adverbs; UD grc gives no mapping"),
    "R03S": ("upos", "d, ὡς, AuxY/AuxZ", "ADV|PART|SCONJ", "conditional", "conditional",
             "AGDT20-PARTICIPLE-AUXZ UD-GRC-SCONJ", "ὡς with a participle is AuxZ in AGDT"),
    "R03P": ("upos", "d, any other AuxY/AuxZ particle", "ADV|PART", "conditional", "conditional",
             "AGDT11-PART AGDT20-PARTICLE UD-GRC-PART SMYTH-2769 AGDT-README-20",
             "AGDT morphology is adverb; UD grc PART is a Smyth list not mapped to native evidence"),
    "R04": ("upos", "v, relation AuxV", "AUX", "resolved", "non_vocab", "UD-GRC-AUX UD-AUX UDTOOLS-GRC-AUX",
            "periphrastic auxiliary"),
    "R05C": ("upos", "v, lemma εἰμί, an effective PNOM dependent", "AUX", "resolved", "non_vocab",
             "AGDT20-PNOM AGDT11-PNOM UD-COP UD-AUX UDTOOLS-GRC-AUX HAMLEDT-COP",
             "copula with a predicate nominal; effective dependents traverse COORD/APOS members and AuxP/AuxC"),
    "R05X": ("upos", "v, lemma εἰμί, no effective PNOM, not AuxV", "VERB|AUX", "conditional", "conditional",
             "UD-COP UD-AUX UD-PERSEUS-EIMI", "UD allows existential 'be' as copula only if it is the same verb; UD Perseus tags εἰμί AUX"),
    "R06": ("upos", "v, other", "VERB", "resolved", "eligible", "AGDT-TAGSETS-21 UD-GRC-VERB UD-COP",
            "includes PNOM-taking γίγνομαι etc.: UD says 'become' verbs are not copulas"),
    "R07": ("upos", "c, COORD", "CCONJ", "resolved", "non_vocab", "AGDT20-COORD AGDT20-CONJ UD-GRC-CCONJ", ""),
    "R08": ("upos", "c, AuxC with an effective verbal dependent", "SCONJ", "resolved", "non_vocab",
            "AGDT20-CONJ AGDT11-AUXC UD-GRC-SCONJ", "introduces a clause"),
    "R08X": ("upos", "c, AuxC without an effective verbal dependent", "SCONJ|CCONJ|ADP", "conditional",
             "non_vocab", "AGDT20-CONJ UD-GRC-SCONJ", "comparison/phrase-level AuxC (e.g. 'than', 'as')"),
    "R09": ("upos", "c, other relation", "CCONJ|SCONJ", "unresolved", "non_vocab", "AGDT20-CONJ",
            "no documented rule for this relation"),
    "R10": ("upos", "r", "ADP", "resolved", "non_vocab", "AGDT-TAGSETS-21 INTERSET-GRC", ""),
    "R11": ("upos", "l", "DET", "resolved", "non_vocab", "AGDT-TAGSETS-21 INTERSET-GRC", ""),
    "R12": ("upos", "p", "PRON", "resolved", "non_vocab", "AGDT-TAGSETS-21 INTERSET-GRC", ""),
    "R13": ("upos", "i", "INTJ", "resolved", "non_vocab", "AGDT-TAGSETS-21", ""),
    "R14": ("upos", "u", "PUNCT", "resolved", "non_vocab", "AGDT-TAGSETS-21 AGDT-README-20", ""),
    "R15": ("upos", "m", "NUM", "resolved", "non_vocab", "AGDT-TAGSETS-21", ""),
    "R16": ("upos", "g", "PART", "resolved", "non_vocab", "AGDT-TAGSETS-21 INTERSET-GRC", ""),
    "H0": ("headword", "plain lemma", "citation headword = NFC native lemma", "source_plain", "", "AGDT-README-17",
           "accent, case and breathing kept; no repair"),
    "H1": ("headword", "Morpheus 'P-L' where L's stemlib :le: entry carries rpb:/pb: P", "headword = L",
           "decoded", "", "MORPH-PRINT MORPH-RPB MORPH-STEMLIB", "preverb is internal to L's own entry"),
    "H2": ("headword", "Morpheus 'P-L' with a vbs.cmp row for P-L (same homograph number)", "headword = compound",
           "decoded", "", "MORPH-PRINT MORPH-CMP MORPH-BUILDLEMMA", "Morpheus' own compound table"),
    "H3": ("headword", "Morpheus 'P-L' matching exactly one :le: entry whose stem is pb:P + L's stem",
           "headword = that entry", "conditional", "", "MORPH-PRINT MORPH-STEMLIB MORPH-BUILDLEMMA",
           "decomposition match, not a printed or tabled decoding"),
    "H4": ("headword", "Morpheus 'P-L' with no documented decoding", "unresolved (candidates recorded)", "unresolved",
           "", "MORPH-PRINT MORPH-BUILDLEMMA", "raw lemma kept; no hyphen deletion"),
    "D0": ("discriminator", "v1.6 witness lemma has no Morpheus number", "absent", "absent_in_source", "",
           "AGDT-V16-LYS AGDT-HDR-LYS", ""),
    "D1": ("discriminator", "v1.6 witness lemma ends in a Morpheus number N", "morpheus-homograph:N",
           "stripped_in_v2.1_witness_v1.6", "", "AGDT-V16-LYS AGDT20-LEMNUM HAMLEDT-LID AGDT-HDR-LYS",
           "v2.0 normalization deleted the number; v1.6 is the documented pre-normalization file"),
    "D2": ("discriminator", "one key|UPOS spans different v1.6 numbers", "identity conditional", "conflated", "",
           "AGDT-V16-LYS AGDT20-LEMNUM", "plain or merged identity would erase a source-declared distinction"),
}

EVIDENCE = [
    # id, kind, source, revision, path (relative to INPUTS), needle (line locator) or locator, claim
    ("AGDT-TAGSETS-21", "producer tagset", "PerseusDL/treebank_data", TB_COMMIT, "sources/v2.1/Greek/TAGSETS.xml",
     "<postag>g</postag>", "postag position 1 letters: d adverb, g particle, c conjunction, l article, ..."),
    ("AGDT-README-17", "producer documentation", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/README.txt",
     "g\tparticle", "AGDT 1.7 postag table distinguishes d adverb and g particle; lemma is 'the base lemma'"),
    ("AGDT-README-20", "producer release notes", "PerseusDL/treebank_data", TB_COMMIT, "sources/v2.0/Greek/README.MD",
     "checked/normalized for the Greek characters",
     "v2.0: semi-automatic annotation; Greek characters and punctuation normalized; adverb/particle still unsettled"),
    ("AGDT-README-21", "producer release notes", "PerseusDL/treebank_data", TB_COMMIT, "sources/v2.1/Greek/README.MD",
     "no change has been made to the texts already contained in the version 2.0", "v2.1 does not change v2.0 texts"),
    ("AGDT-HDR-LYS", "producer file header", "PerseusDL/treebank_data", TB_COMMIT, V21, "annotator of the text",
     "Vanessa Gorman annotator (2014-08-21); Celano release editor normalization; scheme 1.1; pre-normalization text in v1.6"),
    ("AGDT-V16-LYS", "producer witness file", "PerseusDL/treebank_data", TB_COMMIT, V16, "<sentence",
     "pre-normalization Lysias I: same node ids, U+02BC elision, Morpheus homograph numbers in lemmas"),
    ("AGDT-V16-README", "producer documentation", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1.6/README.md",
     "Transformed versions of v1", "v1.6 = v1 files transformed to the 1.6 treebank schema"),
    ("AGDT-V20-LYS", "producer release file", "PerseusDL/treebank_data", TB_COMMIT, V20, "<sentence", "v2.0 Lysias I"),
    ("AGDT-STABLE", "producer corpus listing", "PerseusDL/treebank_data", TB_COMMIT, "sources/agdt2-stableannotations.txt",
     "AGDT2/corpus/stableannotations/readme.md", "double-annotated adjudicated AGDT2 texts; Lysias I not listed"),
    ("AGDT20-PNOM", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>3.7 PNOM</h4>", "PNOM: noun/adjective/infinitive equivalents dependent on a copulative verb"),
    ("AGDT20-COORD", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>3.9 COORD</h4>", "COORD for coordinate conjunctions; inferential/causal/some adversative treated as adverbs; non-final correlatives AuxY attached to the last"),
    ("AGDT20-CONJ", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>2.8 Conjunction</h4>", "conjunction set per Smyth 2163/2770; COORD vs AuxC distinguishes coordinating vs subordinating"),
    ("AGDT20-AUXY", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>3.17 AuxY</h4>", "AuxY: non-final correlative conjunctions and whole-sentence sentence adverbs"),
    ("AGDT20-AUXZ", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>3.18 AuxZ</h4>", "AuxZ: adverbial logical operators meaning 'not', 'even', 'also'"),
    ("AGDT20-EXD", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>3.19 ExD</h4>", "ExD: constituents not syntactically belonging to the sentence"),
    ("AGDT20-PARTICLE", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "<h4>4.1.12.1 Particle</h4>", "particles = (sentence) adverbs; listed conjunction-particles annotated as conjunctions when conjunctions"),
    ("AGDT20-LEMNUM", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "outputs lemmas followed by a number", "Morpheus lemma numbers are to be deleted from the lemma field"),
    ("AGDT20-ADVLEM", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "so the lemma for an adverb should be the corresponding adjective lemma", "adjective-derived adverbs take the adjective lemma"),
    ("AGDT20-SUBSTADJ", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "A substantive adjective is an adjective lemma", "LSJ adjective lemmas used as nouns (example spelled Ἀθαναῖος) are adjectives"),
    ("AGDT20-RI", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "will be added to every lemma which is a relative or interrogative", "2.0 .R/.I lemma suffixes (absent from this 1.1 file)"),
    ("AGDT20-PARTICIPLE-AUXZ", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/AGDT2/guidelines/Greek_guidelines.md",
     "depend on the participles", "ὡς, ὥσπερ etc. depend on participles as AuxZ"),
    ("AGDT11-PNOM", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/guidelines.pdf",
     "pdf:Predicatenominals(subjectcomplements)dependonaverbalhead", "1.1 §3.7"),
    ("AGDT11-AUXY", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/guidelines.pdf",
     "pdf:AuxYshouldalsobeusedtoannotatecoordinatorsthatarenotcommas", "1.1 §3.11: non-head coordinators take AuxY"),
    ("AGDT11-AUXZ", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/guidelines.pdf",
     "pdf:Thisgroupconsistslargelyofnegators", "1.1 §3.12: AuxZ emphasizing particles, largely negators"),
    ("AGDT11-PART", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/guidelines.pdf",
     "pdf:Aparticleisamorphologicalcategory", "1.1 §4.3: particle is a morphological category; mostly AuxY/AuxZ"),
    ("AGDT11-AUXC", "producer guideline", "PerseusDL/treebank_data", TB_COMMIT, "sources/v1/greek/docs/guidelines.pdf",
     "pdf:subordinateverbdependsonthesubordinating", "1.1: subordinate verb depends on the subordinating conjunction"),
    ("MORPH-PRINT", "producer tool source", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "src/anal/prntanal.c",
     'strcat(tmp,"-");', "lexicon output prints preverb, '-', lemma_of (comma-separated preverbs)"),
    ("MORPH-BUILDLEMMA", "producer tool source", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "src/anal/checkpreverb.c",
     "no longer called as of 6/7/88", "compound lemma building (build_lemma) is disabled"),
    ("MORPH-RPB", "producer tool source", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "src/morphlib/morphkeys.c",
     'strcmp(curkey,"rpb")', "rpb: marks a ROOT_PREVERB inside the lexeme's own stem; pb: a HAS_PREVERB stem"),
    ("MORPH-STEMLIB", "producer lexicon", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "stemlib/Greek/stemsrc/vbs.simp.ml",
     ":le:", ":le: entries with stems and rpb:/pb: preverb keys (also lsj.vbs, vbs.irreg)"),
    ("MORPH-CMP", "producer lexicon", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "stemlib/Greek/stemsrc/vbs.cmp.ml",
     "su/n-le/gw2 sulle/gw", "compound table 'P-L C' (vbs.cmp.lsj uses 'C P-L')"),
    ("MORPH-NOMPROPER", "producer lexicon", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "stemlib/Greek/stemsrc/nom.proper",
     ":le:*)eratosqe/nhs", "proper-name stem lexicon"),
    ("MORPH-ENTITY", "producer lexicon", "perseids-tools/morpheus", MORPHEUS_COMMIT, MORPH + "stemlib/Greek/steminds/entitylist.txt",
     "*no/mos\tperson", "named-entity list includes a person Νόμος: lexicon membership is not token classification"),
    ("UD-COP", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_u-dep/cop.md",
     "Verbal copulas are tagged [AUX]()", "copulas are AUX; 'become' verbs are not copulas; existential 'be' may be copula if same verb"),
    ("UD-AUX", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_u-pos/AUX_.md",
     "also include copulas", "AUX includes copulas; auxiliaries may have non-auxiliary uses"),
    ("UD-GRC-AUX", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/AUX_.md",
     "auxiliary verbs are restricted to forms of the verb εἰμί", "grc AUX: εἰμί in periphrastic forms"),
    ("UDTOOLS-GRC-AUX", "target scheme validator data", "UniversalDependencies/tools", UDTOOLS_COMMIT,
     "ext/UniversalDependencies/tools/data/data.json", '"auxiliaries"', "grc auxiliaries: εἰμί cop.AUX (documented), ἔχω Aspect=Perf"),
    ("UD-GRC-PART", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/PART.md",
     "only those words that do not clearly belong", "PART = Smyth 631-671 particles excluding conjunctions and adverbs such as καί 'even'"),
    ("UD-PART", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_u-pos/PART.md",
     "Negation particle", "negation particle is a PART example; language docs list PART words"),
    ("UD-GRC-CCONJ", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/CCONJ.md",
     "some conjunctions can also function as adverbs", "CCONJ examples καί, τε, ἀλλά"),
    ("UD-GRC-SCONJ", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/SCONJ.md",
     "introducing a subordinate clause", "SCONJ introduces a subordinate clause"),
    ("UD-GRC-ADV", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/ADV.md",
     "Adverbs have to be distinguished from particles", "ADV vs PART criteria not clear-cut"),
    ("UD-GRC-NOUN", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/NOUN.md",
     "substantivized adjectives (e.g., Ἀθηναῖος)", "Ἀθηναῖος named as a substantivized adjective"),
    ("UD-GRC-VERB", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/VERB.md",
     "participles also inflect", "VERB includes participles"),
    ("UD-GRC-PROPN", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_grc/pos/PROPN.md",
     "a person or a country", "PROPN refers to a (more or less) unique object"),
    ("UD-PROPN", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_u-pos/PROPN.md",
     "part of the name) of a specific individual", "PROPN is a name or part of a name"),
    ("UD-PROPN-TITLES", "target scheme", "UniversalDependencies/docs", UDDOCS_COMMIT, UDDOCS + "_u-pos/PROPN.md",
     "their original tags", "phrases used as names keep their original tags"),
    ("HAMLEDT-COP", "converter source", "ufal/treex", TREEX_COMMIT, TREEX + "Tool/PhraseBuilder/ToUD.pm",
     "sub detect_prague_copula", "a phrase with a Pnom dependent is a copula construction; Udep.pm tags verbal cop AUX"),
    ("HAMLEDT-PROPN", "converter source", "ufal/treex", TREEX_COMMIT, TREEX + "Block/HamleDT/HarmonizePerseus.pm",
     "sub detect_proper_nouns", "converter heuristic: capitalized noun lemma -> proper noun (not adopted)"),
    ("HAMLEDT-LID", "converter source", "ufal/treex", TREEX_COMMIT, TREEX + "Block/HamleDT/HarmonizePerseus.pm",
     "->{lid} = $2", "converter moves trailing lemma digits into a lemma id"),
    ("INTERSET-GRC", "converter source", "dan-zeman/interset", INTERSET_COMMIT,
     "ext/dan-zeman/interset/lib/Lingua/Interset/Tagset/GRC/Conll.pm", "'d' => ['pos' => 'adv']",
     "tag-literal decoding: d adverb, g particle, c conjunction"),
    ("UD-PERSEUS-README", "converted release notes", "UniversalDependencies/UD_Ancient_Greek-Perseus", UD_COMMIT,
     "ud/README.md", "Fixed UPOS tags of copulae", "lists Lysias 1; v2.11 copulae VERB->AUX; Lemmas/UPOS 'converted from manual'"),
    ("UD-PERSEUS-EIMI", "converted corpus aggregate", "UniversalDependencies/UD_Ancient_Greek-Perseus", UD_COMMIT,
     "ud-crosscheck.json", "derived", "εἰμί tokens in UD Perseus are AUX in nearly all uses (cross-corpus, not Lysias I)"),
    ("UD-PERSEUS-CAPS", "converted corpus aggregate", "UniversalDependencies/UD_Ancient_Greek-Perseus", UD_COMMIT,
     "ud-crosscheck.json", "derived", "capitalized-lemma n tokens are NOUN in UD Perseus; no PROPN emitted"),
    ("SMYTH-2769", "grammar (web, unpinned)", "Perseus Digital Library", "accessed 2026-10-14",
     "", "http://www.perseus.tufts.edu/hopper/text?doc=Smyth+grammar+2769",
     "particles include sentence adverbs and conjunctions; καί, οὐδέ, γάρ fluctuate between adverbial and conjunctional"),
    ("REPO-ELIG", "repository", "justin-hayes/mouseion", "this branch", "internal/selection/selection.go",
     "AllowedPOS lines 69-72", "default vocabulary selection allows NOUN, VERB, ADJ, ADV"),
    ("ADR0094", "repository", "justin-hayes/mouseion", "this branch", "doc/adr/0094-source-independent-annotation-and-provenance.md",
     "", "unprinted nodes are not printed vocabulary"),
    ("CORPUS-ATTEST", "producer corpus aggregate", "PerseusDL/treebank_data", TB_COMMIT, "agdt/v2.1/Greek/texts/*.xml",
     "derived", "hyphen-component practice counted across all pinned v2.1 Greek files"),
]


# ---- text helpers -----------------------------------------------------------
def nfc(text):
    return unicodedata.normalize("NFC", text or "")


def canonical_key(lemma):
    """#2033 identity rule: NFC, lowercase, final sigma folded to sigma, NFC. Accents kept."""
    return nfc(nfc(lemma).lower().replace("ς", "σ"))


def has_letter(text):
    return any(ch.isalpha() for ch in text)


def has_grave(text):
    return "̀" in unicodedata.normalize("NFD", text)


def marks(text):
    """Code points of a raw form that are not letters (elision marks, hyphens, combining marks)."""
    return " ".join("U+%04X" % ord(ch) for ch in text if not ch.isalpha() and unicodedata.category(ch) != "Mn"
                    or ch in "̓ʼ’")


def sha256(path):
    digest = hashlib.sha256()
    with open(path, "rb") as handle:
        for chunk in iter(lambda: handle.read(1 << 16), b""):
            digest.update(chunk)
    return digest.hexdigest()


BETA_LETTERS = dict(zip("αβγδεζηθικλμνξοπρσςτυφχψω", "abgdezhqiklmncoprsstufxyw"))
BETA_MARKS = {"̓": ")", "̔": "(", "̈": "+", "́": "/", "̀": "\\", "͂": "=", "ͅ": "|"}
MARK_ORDER = ")(+/\\=|"
UNBETA_LETTERS = {v: k for k, v in BETA_LETTERS.items() if k != "ς"}
UNBETA_MARKS = {v: k for k, v in BETA_MARKS.items()}


def to_beta(text):
    """Unicode Greek -> Morpheus beta code (lowercase letters, '*' for capitals, marks after the letter)."""
    out = []
    for base, mks in _clusters(unicodedata.normalize("NFD", nfc(text))):
        lower = base.lower()
        if lower not in BETA_LETTERS:
            out.append(base)
            continue
        code = "".join(sorted((BETA_MARKS[m] for m in mks if m in BETA_MARKS), key=MARK_ORDER.index))
        out.append("*" + code + BETA_LETTERS[lower] if base != lower else BETA_LETTERS[lower] + code)
    return "".join(out)


def from_beta(beta):
    """Morpheus beta code -> NFC Unicode. Trailing homograph digits are not part of the word."""
    out, i = [], 0
    beta = re.sub(r"\d+$", "", beta)
    while i < len(beta):
        upper = beta[i] == "*"
        i += upper
        pre = ""
        while upper and i < len(beta) and beta[i] in UNBETA_MARKS:
            pre += beta[i]
            i += 1
        letter = beta[i]
        i += 1
        post = ""
        while i < len(beta) and beta[i] in UNBETA_MARKS:
            post += beta[i]
            i += 1
        base = UNBETA_LETTERS.get(letter, letter)
        if letter == "s" and (i >= len(beta) or not beta[i].isalpha()):
            base = "ς"
        base = base.upper() if upper else base
        out.append(base + "".join(UNBETA_MARKS[m] for m in sorted(pre + post, key=MARK_ORDER.index)))
    return nfc("".join(out))


def _clusters(nfd):
    base, mks = None, []
    for ch in nfd:
        if unicodedata.combining(ch):
            mks.append(ch)
            continue
        if base is not None:
            yield base, mks
        base, mks = ch, []
    if base is not None:
        yield base, mks


def locate(path, needle):
    """1-based line number of the first line containing needle (exact substring)."""
    with open(path, encoding="utf-8") as handle:
        for number, line in enumerate(handle, 1):
            if needle in line:
                return number
    raise SystemExit(f"evidence needle not found in {path}: {needle!r}")


def pdf_compact_text(path):
    sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
    import extract_pdf_text  # noqa: E402
    buffer = io.StringIO()
    stdout, sys.stdout = sys.stdout, buffer
    try:
        extract_pdf_text.main(path)
    finally:
        sys.stdout = stdout
    return re.sub(r"\s", "", buffer.getvalue())


# ---- native data --------------------------------------------------------------
def load_nodes(path):
    """Every <word> in document order, with its within-sentence position and raw form."""
    nodes = []
    for sentence in ET.parse(path).getroot().iter("sentence"):
        for position, word in enumerate(sentence.findall("word"), 1):
            nodes.append({
                "sentence_id": sentence.get("id", ""),
                "subdoc": sentence.get("subdoc", ""),
                "word_id": word.get("id", ""),
                "position": position,
                "raw_form": word.get("form") or "",
                "form": nfc(word.get("form")),
                "lemma": nfc(word.get("lemma")),
                "postag": word.get("postag", ""),
                "relation": word.get("relation", ""),
                "head": word.get("head", ""),
                "artificial": word.get("artificial", ""),
            })
    return nodes


def rel_base(relation):
    return (relation or "").split("_")[0]


def is_member(node):
    return bool(re.search(r"_(CO|AP)", node["relation"] or ""))


class Tree:
    """Sentence-local navigation over native AGDT nodes."""

    def __init__(self, nodes):
        self.by_id = {(n["sentence_id"], n["word_id"]): n for n in nodes}
        self.children = collections.defaultdict(list)
        for n in nodes:
            self.children[(n["sentence_id"], n["head"])].append(n)

    def head(self, node):
        return self.by_id.get((node["sentence_id"], node["head"]))

    def kids(self, node):
        return self.children[(node["sentence_id"], node["word_id"])]

    def real(self, node):
        """Effective dependents through COORD/APOS members and AuxP/AuxC (Prague convention)."""
        base = rel_base(node["relation"])
        if base in ("COORD", "APOS"):
            for member in self.kids(node):
                if is_member(member):
                    yield from self.real(member)
        elif base in ("AuxP", "AuxC"):
            for child in self.kids(node):
                if rel_base(child["relation"]) not in ("AuxX", "AuxG", "AuxK"):
                    yield from self.real(child)
        else:
            yield node

    def effective_dependents(self, node):
        out = [r for child in self.kids(node) for r in self.real(child)]
        if is_member(node):
            coord = self.head(node)
            if coord is not None and rel_base(coord["relation"]) in ("COORD", "APOS"):
                for shared in self.kids(coord):
                    if not is_member(shared) and shared is not node:
                        out.extend(self.real(shared))
        return out


def is_copula(tree, node):
    return (node["postag"][:1] == "v" and node["lemma"] == COPULA_LEMMA
            and any(rel_base(d["relation"]) == "PNOM" for d in tree.effective_dependents(node)))


# ---- Morpheus lexicon -----------------------------------------------------------
class Morpheus:
    def __init__(self, root):
        self.entries = collections.defaultdict(list)  # beta lemma (with digits) -> stem lines
        for name in ("vbs.simp.ml", "lsj.vbs", "vbs.irreg"):
            current = None
            path = os.path.join(root, "stemlib/Greek/stemsrc", name)
            with open(path, encoding="utf-8", errors="replace") as handle:
                for number, line in enumerate(handle, 1):
                    line = line.rstrip("\n")
                    if line.startswith(":le:"):
                        current = line[4:].strip()
                        self.entries[current]
                        continue
                    if current is None or not line.startswith(":") or line.startswith(":le:"):
                        continue
                    tokens = line.split()
                    stem = tokens[0].split(":")[-1] if tokens else ""
                    klass = tokens[1] if len(tokens) > 1 else ""
                    rpb = next((t[4:] for t in tokens if t.startswith("rpb:")), "")
                    pb = next((t[3:] for t in tokens if t.startswith("pb:")), "")
                    self.entries[current].append({"file": name, "line": number, "stem": stem, "class": klass,
                                                  "rpb": rpb, "pb": pb})
        self.cmp = []
        for name in ("vbs.cmp.ml", "vbs.cmp.lsj"):
            path = os.path.join(root, "stemlib/Greek/stemsrc", name)
            with open(path, encoding="utf-8", errors="replace") as handle:
                for number, line in enumerate(handle, 1):
                    tokens = line.split()
                    keyed = [t for t in tokens if "-" in t]
                    plain = [t for t in tokens if "-" not in t]
                    if len(tokens) == 2 and len(keyed) == 1 and len(plain) == 1:
                        pre, base = keyed[0].split("-", 1)
                        self.cmp.append({"file": name, "line": number, "preverbs": pre, "base": base,
                                         "compound": plain[0]})
        self.proper = {}
        with open(os.path.join(root, "stemlib/Greek/stemsrc/nom.proper"), encoding="utf-8", errors="replace") as handle:
            for number, line in enumerate(handle, 1):
                if line.startswith(":le:"):
                    self.proper.setdefault(line[4:].strip(), number)

    def entries_for(self, lemma_beta, digit):
        out = []
        for key, stems in self.entries.items():
            if re.sub(r"\d+$", "", key) == lemma_beta and (not digit or key.endswith(digit) and key[-len(digit) - 1:-len(digit)].isalpha()):
                out.append((key, stems))
        return sorted(out)

    def decode(self, lemma, witness_digit):
        """Decode a Morpheus 'P-L' lemma. Returns (status, rule, headword, candidates, evidence)."""
        pre, base = lemma.split("-", 1)
        pb = ",".join(to_beta(p) for p in pre.split(","))
        lb = to_beta(base)
        for key, stems in self.entries_for(lb, witness_digit):
            for s in stems:
                if pb in (s["rpb"], s["pb"]):
                    ev = f"MORPH:{s['file']}:{s['line']}:le={key}:{'rpb' if s['rpb'] == pb else 'pb'}={pb}"
                    return ("decoded", "H1", base, [base], [ev])
        rows = [r for r in self.cmp if r["preverbs"] == pb and re.sub(r"\d+$", "", r["base"]) == lb]
        exact = [r for r in rows if re.sub(r"^\D+", "", r["base"]) == (witness_digit or "")]
        if len({r["compound"] for r in exact}) == 1:
            r = exact[0]
            word = from_beta(r["compound"])
            return ("decoded", "H2", word, [word], [f"MORPH:{r['file']}:{r['line']}:{r['preverbs']}-{r['base']}={r['compound']}"])
        base_stems = {(s["stem"], s["class"]) for _, stems in self.entries_for(lb, "") for s in stems if not s["pb"] and not s["rpb"]}
        matches = sorted({(key, s["file"], s["line"]) for key, stems in self.entries.items() if re.sub(r"\d+$", "", key) != lb
                          for s in stems if s["pb"] == pb and (s["stem"], s["class"]) in base_stems})
        if len({m[0] for m in matches}) == 1:
            key, fname, line = matches[0]
            word = from_beta(key)
            return ("conditional", "H3", word, [word], [f"MORPH:{fname}:{line}:le={key}:pb={pb}+stem-of-{lb}"])
        cands = sorted({from_beta(r["compound"]) for r in rows})
        ev = [f"MORPH:{r['file']}:{r['line']}:{r['preverbs']}-{r['base']}={r['compound']}:witness-number={witness_digit or 'none'}" for r in rows]
        return ("unresolved", "H4", "", cands, ev)


# ---- classification -------------------------------------------------------------
def classify(node, tree, morph, rubric_sentences):
    """Return (rule_id, candidates, extra_flags)."""
    letter = node["postag"][:1]
    relation = node["relation"]
    base = rel_base(relation)
    lemma = node["lemma"]
    flags = []
    if node["artificial"] == ELIDED:
        return "R00", [], [ELIDED]
    head = tree.head(node)
    head_base = rel_base(head["relation"]) if head is not None else ""
    if letter == "u":
        return "R14", ["PUNCT"], flags
    if letter == "v":
        if base == "AuxV":
            return "R04", ["AUX"], flags
        if lemma == COPULA_LEMMA:
            return ("R05C", ["AUX"], flags) if is_copula(tree, node) else ("R05X", ["VERB", "AUX"], flags)
        return "R06", ["VERB"], flags
    if letter == "n":
        if node["sentence_id"] in rubric_sentences:
            return "R01R", ["NOUN"], flags + ["rubric-label"]
        if lemma in UD_GRC_SUBST_ADJ:
            return "R01A", ["ADJ", "NOUN", "PROPN"], flags
        if lemma[:1].isupper():
            if to_beta(lemma) in morph.proper:
                return "R01P", ["PROPN"], flags + ["morpheus-nom.proper:%d" % morph.proper[to_beta(lemma)]]
            return "R01C", ["NOUN", "PROPN"], flags
        for other in [head] + tree.kids(node):
            if (other is not None and other["postag"][:1] == "n" and other["lemma"][:1].isupper()
                    and other["lemma"] not in UD_GRC_SUBST_ADJ and to_beta(other["lemma"]) not in morph.proper
                    and abs(other["position"] - node["position"]) == 1):
                return "R01M", ["NOUN", "PROPN"], flags
        return "R01", ["NOUN"], flags
    if letter == "a":
        return "R02", ["ADJ"], flags
    if (letter in ("d", "c") and base == "AuxY" and head_base == "COORD" and lemma in AGDT_CONJ_PARTICLES
            and head["lemma"] == lemma and int(node["word_id"]) < int(head["word_id"])):
        # Repeated coordinator (καί ... καί, οὔτε ... οὔτε): the earlier one is the non-head correlative.
        # A sentence particle attached to a coordinated predicate also has a COORD head, so the
        # head must repeat the same lemma.
        return "R03Y", ["CCONJ"], flags
    if letter == "d":
        if base not in ("AuxY", "AuxZ"):
            return "R03", ["ADV"], flags
        if lemma == "καί" and base == "AuxZ":
            return "R03K", ["ADV"], flags
        if has_grave(lemma):
            flags.append("lemma-grave-list-lookup-not-applied")
        if lemma in NEGATORS:
            return "R03N", ["ADV", "PART"], flags
        if lemma in AGDT_CONJ_PARTICLES:
            return "R03C", ["ADV", "CCONJ"], flags
        if lemma in AGDT_ADVERBIALIZED_CONJ:
            return "R03A", ["ADV", "CCONJ", "PART"], flags
        if lemma == "ὡς":
            return "R03S", ["ADV", "PART", "SCONJ"], flags
        return "R03P", ["ADV", "PART"], flags
    if letter == "c":
        if base == "COORD":
            return "R07", ["CCONJ"], flags
        if base == "AuxC":
            verbal = any(d["postag"][:1] == "v" for child in tree.kids(node) for d in tree.real(child))
            return ("R08", ["SCONJ"], flags) if verbal else ("R08X", ["SCONJ", "CCONJ", "ADP"], flags)
        return "R09", ["CCONJ", "SCONJ"], flags
    simple = {"r": ("R10", "ADP"), "l": ("R11", "DET"), "p": ("R12", "PRON"), "i": ("R13", "INTJ"),
              "m": ("R15", "NUM"), "g": ("R16", "PART")}
    if letter in simple:
        rule, upos = simple[letter]
        return rule, [upos], flags
    return "R-unmapped", [], flags + ["letter-unmapped:" + letter]


def repo_eligible(upos, lemma, relation):
    """Mirror of internal/selection OccurrenceEligible for the default config."""
    return relation != "compound:prt" and has_letter(lemma) and upos in ALLOWED_UPOS


def eligibility_of(candidates, lemma, relation):
    if not candidates:
        return "unprinted"
    allowed = [u for u in candidates if repo_eligible(u, lemma, relation)]
    if len(allowed) == len(candidates):
        return "eligible"
    return "conditional" if allowed else "non_vocab"


def rubric_sentence_ids(nodes):
    """Sentences whose only printed lexical node is an ExD noun (source rubric labels)."""
    by_sentence = collections.defaultdict(list)
    for n in nodes:
        if not n["artificial"] and n["postag"][:1] != "u":
            by_sentence[n["sentence_id"]].append(n)
    return {sid for sid, lex in by_sentence.items()
            if len(lex) == 1 and lex[0]["postag"][:1] == "n" and rel_base(lex[0]["relation"]) == "ExD"}


# ---- components -----------------------------------------------------------------
def component_groups(nodes):
    """Crasis splits ('κ-' + host) and enclitic splits ('-τε' + host) in node order."""
    groups = []
    printed = [n for n in nodes if not n["artificial"]]
    for i, n in enumerate(printed):
        f = n["raw_form"]
        if len(f) > 1 and has_letter(f) and f.endswith("-") and i + 1 < len(printed):
            groups.append(("C-CRASIS", n, printed[i + 1]))
        elif len(f) > 1 and has_letter(f) and f.startswith("-") and i + 1 < len(printed):
            groups.append(("C-ENCLITIC", n, printed[i + 1]))
    return groups


def corpus_component_practice(agdt_dir):
    stats = collections.Counter()
    files = collections.defaultdict(set)
    for path in sorted(glob.glob(os.path.join(agdt_dir, "v2.1/Greek/texts", "*.xml"))):
        base = os.path.basename(path)
        for sentence in ET.parse(path).getroot().iter("sentence"):
            words = [w for w in sentence.findall("word") if not w.get("artificial")]
            for i, w in enumerate(words):
                f = w.get("form") or ""
                if len(f) < 2 or not has_letter(f):
                    continue
                if f.endswith("-"):
                    letters = unicodedata.normalize("NFD", f[:-1])
                    if all(ch in "βγδζθκλμνξπρστφχψ" for ch in letters if not unicodedata.combining(ch)):
                        stats["crasis_fragment_nodes"] += 1
                        files["crasis"].add(base)
                    else:
                        stats["other_trailing_hyphen_nodes"] += 1
                elif f.startswith("-"):
                    prv = words[i - 1].get("form") if i else ""
                    if base == LYSIAS_FILE:
                        stats["enclitic_fragment_lysias1"] += 1
                    elif prv and not prv.endswith("-"):
                        stats["enclitic_fragment_after_host_elsewhere"] += 1
                        files["enclitic_after"].add(base)
                    else:
                        stats["enclitic_fragment_other_elsewhere"] += 1
                        files["enclitic_other"].add(base)
    return {
        "scope": "all pinned AGDT v2.1 Greek files, printed nodes; hyphen-marked forms containing a letter",
        "crasis_fragment_nodes": stats["crasis_fragment_nodes"], "crasis_files": len(files["crasis"]),
        "crasis_fragment_definition": "form = consonant letters + '-' (e.g. κ-, τ-)",
        "other_trailing_hyphen_nodes": stats["other_trailing_hyphen_nodes"],
        "enclitic_fragment_after_host_outside_lysias1": stats["enclitic_fragment_after_host_elsewhere"],
        "enclitic_after_host_files": sorted(files["enclitic_after"]),
        "enclitic_fragment_other_outside_lysias1": stats["enclitic_fragment_other_elsewhere"],
        "enclitic_other_files": sorted(files["enclitic_other"]),
        "enclitic_fragment_lysias1": stats["enclitic_fragment_lysias1"],
    }


# ---- UD cross-corpus evidence (aggregates only) -------------------------------------
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


def build_ud_crosscheck(agdt_dir, ud_dir):
    native = {}
    for path in sorted(glob.glob(os.path.join(agdt_dir, "v2.1/Greek/texts", "*.xml"))):
        base = os.path.basename(path)
        nodes = load_nodes(path)
        tree = Tree(nodes)
        for n in nodes:
            n["copula"] = is_copula(tree, n)
            if not n["artificial"]:
                native.setdefault((base, n["sentence_id"]), []).append(n)
    stats = collections.Counter()
    table = collections.defaultdict(collections.Counter)
    lysias_sentences = 0
    for sent_id, tokens in read_ud_sentences(ud_dir):
        base, _, number = sent_id.partition("@")
        lysias_sentences += base == LYSIAS_FILE
        printed = native.get((base, number))
        if printed is None:
            stats["sentence_without_native"] += 1
            continue
        if [n["form"] for n in printed] != [t["form"] for t in tokens]:
            stats["sentence_form_mismatch"] += 1
            continue
        stats["sentence_joined"] += 1
        for node, token in zip(printed, tokens):
            stats["tokens_joined"] += 1
            letter, relation, upos = node["postag"][:1], rel_base(node["relation"]), token["upos"]
            table[letter][upos] += 1
            if letter == "d":
                table["d_particle" if relation in ("AuxY", "AuxZ") else "d_other"][upos] += 1
            if letter == "c":
                table["c_coord" if relation == "COORD" else "c_auxc" if relation == "AuxC" else "c_other"][upos] += 1
            if letter == "v" and relation == "AuxV":
                table["v_auxv"][upos] += 1
            if node["lemma"] == COPULA_LEMMA:
                table["eimi_effective_copula" if node["copula"] else "eimi_no_effective_pnom"][upos] += 1
            if letter == "n" and node["lemma"][:1].isupper():
                table["n_capitalized_lemma"][upos] += 1
    return {
        "ud_commit": UD_COMMIT,
        "scope": "all AGDT v2.1 Greek texts joined to UD_Ancient_Greek-Perseus; cross-corpus evidence only",
        "lysias1_sentences_in_ud": lysias_sentences,
        "join": dict(sorted(stats.items())),
        "by_native_key_upos": {k: dict(sorted(v.items())) for k, v in sorted(table.items())},
    }


def annotation_practice(agdt_dir):
    files = []
    for path in sorted(glob.glob(os.path.join(agdt_dir, "v2.1/Greek/texts", "*.xml"))):
        root = ET.parse(path).getroot()
        words = list(root.iter("word"))
        particles = [w for w in words if w.get("postag", "")[:1] in ("d", "g") and w.get("relation") in ("AuxY", "AuxZ")]
        files.append({
            "file": os.path.basename(path),
            "scheme_declared": (root.findtext("header/annotationScheme") or "").strip(),
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


# ---- lineage --------------------------------------------------------------------
def lineage(inputs, v21, v16):
    root = ET.parse(os.path.join(inputs, V21)).getroot()
    header = root.find("header")
    resp = [(r.findtext("persName/name") or r.findtext("persName") or "").strip() + ": " + (r.findtext("resp") or "").strip()
            for r in header.iter("respStmt")]
    old = {(n["sentence_id"], n["word_id"]): n for n in v16}
    diff = collections.Counter()
    postag_positions = collections.Counter()
    for n in v21:
        o = old[(n["sentence_id"], n["word_id"])]
        lexical = n["postag"][:1] != "u" and not n["artificial"]
        if n["lemma"] != o["lemma"]:
            if not lexical:
                diff["lemma_changed_punctuation_or_unprinted"] += 1
            elif re.sub(r"\d+$", "", o["lemma"]) == n["lemma"]:
                diff["lemma_lexical_number_stripped"] += 1
            else:
                diff["lemma_lexical_other"] += 1
        if n["postag"] != o["postag"]:
            if n["postag"][:1] != o["postag"][:1]:
                diff["postag_pos_letter_changed"] += 1
            for i, (a, b) in enumerate(zip(n["postag"], o["postag"])):
                if a != b:
                    postag_positions[str(i + 1)] += 1
        if n["raw_form"] != o["raw_form"]:
            if n["raw_form"].replace("̓", "ʼ") == o["raw_form"]:
                diff["form_elision_mark_only"] += 1
            elif nfc(n["raw_form"]) == nfc(o["raw_form"]):
                diff["form_nfc_only"] += 1
            else:
                diff["form_other"] += 1
    with open(os.path.join(inputs, "sources/agdt2-stableannotations.txt"), encoding="utf-8") as handle:
        stable = sorted({line.split("/")[3] for line in handle if line.count("/") > 4})
    return {
        "scope": "lemma and part-of-speech layers consumed for Lysias I vocabulary identity",
        "file_header": {
            "annotation_scheme": (header.findtext("annotationScheme") or "").strip(),
            "annotation_date": (header.findtext("annotationDate") or "").strip(),
            "release_date": (header.findtext("releaseDate") or "").strip(),
            "responsibilities": resp,
            "evidence": "AGDT-HDR-LYS",
        },
        "release_claims": {
            "v2.0": "semi-automatically annotated; Greek characters and punctuation normalized; postag errors for nouns corrected; adverb/particle distinction still needs work (AGDT-README-20)",
            "v2.1": "no change to texts already in v2.0 (AGDT-README-21)",
            "ud_perseus": "Lemmas/UPOS 'converted from manual', XPOS 'manual native' (UD-PERSEUS-README); scope is the UD subset, which contains no Lysias I sentence",
        },
        "v1.6_to_v2.1_node_diff": dict(sorted(diff.items())),
        "v1.6_to_v2.1_postag_positions_changed": dict(sorted(postag_positions.items())),
        "agdt2_double_annotated_texts": stable,
        "lysias1_double_annotated": any("lysias" in s.lower() or "0540" in s for s in stable),
        "layer_status": {
            "lemma": "human annotation with Morpheus-proposed lemmas (file-level claim); release normalization stripped homograph numbers and levelled punctuation lemmas; no per-token review statement",
            "pos_letter": "human annotation, Morpheus-assisted (file-level claim); unchanged from v1.6 to v2.1",
            "morphological_features": "annotator values plus release-editor corrections (postag feature changes counted above)",
            "syntax": "manual (file-level claim); not evidence for lemma/POS review",
            "second_annotation": "none for Lysias I",
        },
    }


# ---- derivation -----------------------------------------------------------------
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


def evidence_rows(inputs):
    pdf_text = None
    rows = []
    for eid, kind, source, revision, path, needle, claim in EVIDENCE:
        full = os.path.join(inputs, path) if path and "*" not in path and not path.startswith(("internal/", "doc/")) and path != "ud-crosscheck.json" else ""
        locator, digest = needle, ""
        if full:
            digest = sha256(full)
            if needle.startswith("pdf:"):
                if pdf_text is None:
                    pdf_text = pdf_compact_text(full)
                if needle[4:] not in pdf_text:
                    raise SystemExit(f"PDF quote not found: {needle}")
                locator = "PDF text contains: " + needle[4:]
            elif needle not in ("derived", ""):
                locator = f"line {locate(full, needle)}: {needle}"
        rows.append({"evidence_id": eid, "kind": kind, "source": source, "revision": revision, "path": path,
                     "locator": locator, "sha256": digest, "claim": claim})
    return rows


def derive(inputs, out):
    if sha256(os.path.join(inputs, V21)) != LYSIAS_SHA256:
        raise SystemExit("Lysias I sha256 mismatch; run fetch_inputs.sh first")
    os.makedirs(out, exist_ok=True)
    crosscheck = build_ud_crosscheck(os.path.join(inputs, "agdt"), os.path.join(inputs, "ud"))
    write_json(os.path.join(out, "ud-crosscheck.json"), crosscheck)
    write_json(os.path.join(out, "annotation-practice.json"), annotation_practice(os.path.join(inputs, "agdt")))
    morph = Morpheus(os.path.join(inputs, MORPH))

    nodes = load_nodes(os.path.join(inputs, V21))
    witness = {(n["sentence_id"], n["word_id"]): n for n in load_nodes(os.path.join(inputs, V16))}
    if len(witness) != len(nodes):
        raise SystemExit("v1.6 witness node count differs")
    tree = Tree(nodes)
    rubrics = rubric_sentence_ids(nodes)
    components = component_groups(nodes)
    comp_role = {}
    for i, (conv, first, second) in enumerate(components, 1):
        comp_role[id(first)] = (conv, "G%02d" % i, "fragment")
        comp_role[id(second)] = (conv, "G%02d" % i, "host")

    rows = []
    for n in nodes:
        key2 = (n["sentence_id"], n["word_id"])
        w = witness[key2]
        rule, cands, flags = classify(n, tree, morph, rubrics)
        layer = RULES.get(rule, ("upos", "", "", "unresolved", "", "", ""))
        upos_status = layer[3] if rule != "R00" else "n/a"
        eligibility = eligibility_of(cands, n["lemma"], n["relation"])
        evidence = set(layer[5].split()) if rule in RULES else set()
        # headword
        lemma = n["lemma"]
        notation, preverbs, base, headword, hw_status, hw_rule = "plain", "", "", lemma, "source_plain", "H0"
        wdigit = (re.search(r"(\d+)$", w["lemma"]) or [None, ""])[1] if n["postag"][:1] != "u" else ""
        if rule == "R00":
            headword, hw_status, hw_rule = "", "n/a", ""
        elif "-" in lemma and has_letter(lemma) and n["postag"][:1] != "u":
            notation = "morpheus_preverb"
            preverbs, base = lemma.split("-", 1)
            hw_status, hw_rule, headword, cand_words, ev = morph.decode(lemma, wdigit)
            evidence.update(ev)
            evidence.update(RULES[hw_rule][5].split())
            if hw_status == "unresolved":
                flags.append("headword-candidates:" + ",".join(cand_words))
        if has_grave(lemma) and rule != "R00":
            flags.append("lemma-has-grave")
        if lemma[:1].isupper() and rule != "R00":
            flags.append("lemma-capitalized")
        key = canonical_key(headword) if hw_status in ("source_plain", "decoded", "conditional") else ""
        # discriminator
        if rule == "R00" or n["postag"][:1] == "u":
            d_scheme, d_value, d_status = "", "", "n/a"
        elif wdigit:
            d_scheme, d_value, d_status = "morpheus-homograph", wdigit, RULES["D1"][3]
            evidence.update(RULES["D1"][5].split())
        else:
            d_scheme, d_value, d_status = "morpheus-homograph", "", RULES["D0"][3]
        conv = comp_role.get(id(n))
        if conv:
            evidence.add("CORPUS-ATTEST")
        allowed = [u for u in cands if u in ALLOWED_UPOS]
        id_cands = [f"{key}|{u}" for u in cands] if key else [f"unresolved-headword:{lemma}|{u}" for u in cands]
        rows.append({
            "_node": n, "node_key": f"{n['sentence_id']}:{n['word_id']}",
            "binding_join_key": "s%03dw%03d" % (int(n["sentence_id"]), int(n["word_id"])),
            "doc": LYSIAS_DOC, "sentence_id": n["sentence_id"], "subdoc": n["subdoc"], "word_id": n["word_id"],
            "position": n["position"], "form": n["form"], "artificial": n["artificial"], "native_lemma": lemma,
            "native_postag": n["postag"], "native_relation": n["relation"], "head": n["head"],
            "head_relation": (tree.head(n) or {}).get("relation", ""), "scheme_declared": SCHEME_DECLARED,
            "letter": n["postag"][:1], "native_pos": NATIVE_POS.get(n["postag"][:1], ""), "rule_id": rule,
            "upos": cands[0] if len(cands) == 1 else "", "upos_candidates": "|".join(cands),
            "upos_status": upos_status, "eligibility": eligibility, "lemma_notation": notation,
            "lemma_preverbs": preverbs, "lemma_base": base, "citation_headword": headword,
            "headword_status": hw_status, "headword_rule": hw_rule, "canonical_key": key,
            "discriminator_scheme": d_scheme, "discriminator_value": d_value, "discriminator_status": d_status,
            "_allowed": allowed, "_id_cands": id_cands, "_wdigit": wdigit,
            "source_scope": "rubric_label" if n["sentence_id"] in rubrics and n["postag"][:1] != "u" else
                            ("rubric_punctuation" if n["sentence_id"] in rubrics else ("unprinted" if rule == "R00" else "running_text")),
            "component_convention": "%s:%s:%s" % conv if conv else "",
            "_evidence": evidence, "_flags": flags,
        })

    # identity status, with discriminator conflation across the whole document
    groups = collections.defaultdict(set)
    for r in rows:
        if r["eligibility"] == "eligible" and len(r["_allowed"]) == 1 and r["canonical_key"]:
            groups[(r["canonical_key"], r["_allowed"][0])].add(r["_wdigit"] or "-")
    conflated = {k for k, v in groups.items() if len(v) > 1}
    for r in rows:
        flags = r["_flags"]
        if r["eligibility"] in ("non_vocab", "unprinted"):
            r["identity"], r["identity_candidates"], r["identity_status"] = "", "", "n/a"
        else:
            vocab_cands = [c for c, u in zip(r["_id_cands"], r["upos_candidates"].split("|")) if u in ALLOWED_UPOS]
            r["identity_candidates"] = ";".join(r["_id_cands"])
            if r["headword_status"] == "unresolved" or r["upos_status"] == "unresolved":
                status = "unresolved"
            elif r["upos_status"] == "conditional" or r["headword_status"] == "conditional" or r["eligibility"] == "conditional":
                status = "conditional"
            else:
                status = "resolved"
            single = len(vocab_cands) == 1 and r["eligibility"] == "eligible"
            if single and (r["canonical_key"], r["_allowed"][0]) in conflated:
                status = "conditional" if status == "resolved" else status
                r["discriminator_status"] = RULES["D2"][3]
                flags.append("discriminator-conflated-in-v2.1")
                r["_evidence"].update(RULES["D2"][5].split())
            r["identity"] = vocab_cands[0] if single and status != "unresolved" else ""
            r["identity_status"] = status
        r["evidence_ids"] = " ".join(sorted(r["_evidence"]))
        r["flags"] = ";".join(sorted(set(flags)))

    write_tsv(os.path.join(out, "lysias1-nodes.tsv"), NODE_COLS, rows)

    # identities
    ids = {}
    for r in rows:
        if r["identity_status"] == "n/a":
            continue
        cands = r["identity"] and [r["identity"]] or [c for c, u in zip(r["_id_cands"], r["upos_candidates"].split("|")) if u in ALLOWED_UPOS]
        for c in cands:
            key, _, upos = c.rpartition("|")
            b = ids.setdefault(c, {"identity": c, "canonical_key": "" if key.startswith("unresolved-headword:") else key,
                                   "upos": upos, "heads": collections.Counter(), "res": 0, "cond": 0, "unres": 0,
                                   "disc": collections.Counter(), "rules": set(), "flags": set()})
            b["heads"][r["citation_headword"] or r["native_lemma"]] += 1
            b["res" if r["identity_status"] == "resolved" else "cond" if r["identity_status"] == "conditional" else "unres"] += 1
            b["disc"][r["discriminator_value"] or "-"] += 1
            b["rules"].add(r["rule_id"])
            b["rules"].add(r["headword_rule"])
            b["flags"].update(f for f in r["flags"].split(";") if f.startswith(("discriminator", "lemma-", "rubric")))
    id_rows = []
    for b in ids.values():
        status = "resolved" if b["res"] else "conditional" if b["cond"] else "unresolved"
        if (b["canonical_key"], b["upos"]) in conflated:
            status = "conditional"
        id_rows.append({
            "identity": b["identity"], "canonical_key": b["canonical_key"], "upos": b["upos"],
            "citation_headwords": ";".join(f"{h}×{c}" for h, c in sorted(b["heads"].items())),
            "identity_status": status, "occurrences_resolved": b["res"], "occurrences_conditional": b["cond"],
            "occurrences_unresolved": b["unres"],
            "discriminator_witness_values": ";".join(f"{d}×{c}" for d, c in sorted(b["disc"].items())),
            "rule_ids": " ".join(sorted(x for x in b["rules"] if x)), "flags": ";".join(sorted(b["flags"])),
        })
    id_rows.sort(key=lambda x: (x["identity_status"], x["identity"]))
    write_tsv(os.path.join(out, "lysias1-identities.tsv"), IDENTITY_COLS, id_rows)

    printed = [r for r in rows if r["rule_id"] != "R00"]
    rule_counts = collections.Counter(r["rule_id"] for r in printed)
    hw_counts = collections.Counter(r["headword_rule"] for r in printed)
    disc_counts = collections.Counter(r["discriminator_status"] for r in printed)
    rule_rows = []
    for rid, (layer, applies, result, status, elig, ev, note) in RULES.items():
        count = rule_counts[rid] if layer == "upos" else hw_counts[rid] if layer == "headword" else \
            disc_counts[status] if layer == "discriminator" else 0
        if rid == "R00":
            count = len(rows) - len(printed)
        rule_rows.append({"rule_id": rid, "layer": layer, "applies_to": applies, "result": result, "status": status,
                          "eligibility": elig, "printed_nodes": count, "evidence_ids": ev, "note": note})
    for cid, applies, status, ev in COMPONENT_RULES:
        rule_rows.append({"rule_id": cid, "layer": "component", "applies_to": applies, "result": "", "status": status,
                          "eligibility": "", "printed_nodes": "", "evidence_ids": ev, "note": ""})
    write_tsv(os.path.join(out, "lysias1-rules.tsv"), RULE_COLS, rule_rows)
    write_tsv(os.path.join(out, "lysias1-evidence.tsv"), EVIDENCE_COLS, evidence_rows(inputs))

    # component appendix
    practice = corpus_component_practice(os.path.join(inputs, "agdt"))
    comp_rows = []
    by_node = {id(r["_node"]): r for r in rows}
    for n in nodes:
        w = witness[(n["sentence_id"], n["word_id"])]
        r = by_node[id(n)]
        if "̓" in n["raw_form"] and n["postag"][:1] != "u" and not n["artificial"]:
            mapped = n["raw_form"].replace("̓", "ʼ") == w["raw_form"]
            comp_rows.append({
                "node_key": r["node_key"], "binding_join_key": r["binding_join_key"], "convention_id": "C-ELISION",
                "group_id": "", "role": "elided_word", "form": n["raw_form"], "form_marks": marks(n["raw_form"]),
                "v16_form": w["raw_form"], "v16_marks": marks(w["raw_form"]),
                "nfc_changes_form": str(nfc(n["raw_form"]) != n["raw_form"]), "native_lemma": n["lemma"],
                "vocab_effect": "none: lemma is the full citation form",
                "status": "release_mark_mapping_supported" if mapped else "unexplained",
                "evidence_ids": "AGDT-README-20 AGDT-V16-LYS AGDT-HDR-LYS",
            })
    for i, (conv, first, second) in enumerate(components, 1):
        for role, n in (("fragment", first), ("host", second)):
            w = witness[(n["sentence_id"], n["word_id"])]
            r = by_node[id(n)]
            if conv == "C-CRASIS":
                status = "attested_practice_undocumented"
            else:
                status = "unsupported_order_exception"
            comp_rows.append({
                "node_key": r["node_key"], "binding_join_key": r["binding_join_key"], "convention_id": conv,
                "group_id": "G%02d" % i, "role": role, "form": n["raw_form"], "form_marks": marks(n["raw_form"]),
                "v16_form": w["raw_form"], "v16_marks": marks(w["raw_form"]),
                "nfc_changes_form": str(nfc(n["raw_form"]) != n["raw_form"]), "native_lemma": n["lemma"],
                "vocab_effect": f"{r['eligibility']}:{r['identity'] or r['upos_candidates']}",
                "status": status, "evidence_ids": "CORPUS-ATTEST AGDT-V16-LYS",
            })
    comp_rows.sort(key=lambda x: (x["convention_id"], int(x["node_key"].split(":")[0]), int(x["node_key"].split(":")[1])))
    write_tsv(os.path.join(out, "lysias1-components.tsv"), COMPONENT_COLS, comp_rows)

    lin = lineage(inputs, nodes, list(witness.values()))
    lin["component_practice"] = practice
    write_json(os.path.join(out, "lysias1-lineage.json"), lin)
    summary = summarize(rows, id_rows, comp_rows, crosscheck, practice)
    write_json(os.path.join(out, "lysias1-summary.json"), summary)
    return summary


NATIVE_POS = {"n": "noun", "v": "verb", "a": "adjective", "d": "adverb", "l": "article", "g": "particle",
              "c": "conjunction", "r": "preposition", "p": "pronoun", "m": "numeral", "i": "interjection",
              "u": "punctuation", "": ""}

COMPONENT_RULES = [
    ("C-ELISION", "v2.x form carries U+0313 where the v1.6 witness has U+02BC at the same node",
     "release_mark_mapping_supported", "AGDT-README-20 AGDT-V16-LYS AGDT-HDR-LYS"),
    ("C-CRASIS", "form 'X-' followed by host remainder (e.g. κ- + ἀγὼ)", "attested_practice_undocumented",
     "CORPUS-ATTEST AGDT-V16-LYS"),
    ("C-ENCLITIC", "form '-X' (split enclitic); elsewhere it follows its host, in Lysias I it precedes μή",
     "unsupported_order_exception", "CORPUS-ATTEST AGDT-V16-LYS"),
]


def summarize(rows, id_rows, comp_rows, crosscheck, practice):
    printed = [r for r in rows if r["rule_id"] != "R00"]
    c = lambda field, subset=printed: dict(sorted(collections.Counter(r[field] for r in subset).items()))
    vocab = [r for r in printed if r["identity_status"] != "n/a"]
    id_status = collections.Counter(i["identity_status"] for i in id_rows)
    res_ids = [i for i in id_rows if i["identity_status"] == "resolved"]
    return {
        "versions": {"upos_rules": RULES_VERSION, "headword_rules": HEADWORD_RULES_VERSION,
                     "component_conventions": COMPONENT_RULES_VERSION, "normalization_profile": NORMALIZATION_PROFILE,
                     "python_unicodedata": UNICODE_VERSION},
        "scheme_declared": SCHEME_DECLARED,
        "inputs": {"lysias_sha256": LYSIAS_SHA256, "treebank_commit": TB_COMMIT, "ud_commit": UD_COMMIT,
                   "morpheus_commit": MORPHEUS_COMMIT, "ud_docs_commit": UDDOCS_COMMIT, "ud_tools_commit": UDTOOLS_COMMIT,
                   "treex_commit": TREEX_COMMIT, "interset_commit": INTERSET_COMMIT},
        "nodes_total": len(rows),
        "nodes_printed": len(printed),
        "nodes_unprinted_elliptic": len(rows) - len(printed),
        "node_classification_complete": all(r["rule_id"] in RULES for r in rows),
        "printed_by_letter": c("letter"),
        "printed_by_rule": c("rule_id"),
        "printed_by_upos_status": c("upos_status"),
        "printed_by_eligibility": c("eligibility"),
        "printed_by_source_scope": c("source_scope"),
        "vocabulary_nodes_by_identity_status": c("identity_status", vocab),
        "headword_status": c("headword_status", vocab),
        "discriminator_status": c("discriminator_status", printed),
        "eligible_and_identity_resolved_occurrences": sum(1 for r in vocab if r["eligibility"] == "eligible" and r["identity_status"] == "resolved"),
        "identities_by_status": dict(sorted(id_status.items())),
        "identities_resolved": len(res_ids),
        "identities_resolved_with_conditional_occurrences": sum(1 for i in res_ids if int(i["occurrences_conditional"])),
        "components_by_convention_status": dict(sorted(collections.Counter(
            (r["convention_id"] + ":" + r["status"]) for r in comp_rows).items())),
        "component_corpus_practice": practice,
        "reconciliation": {
            "printed_plus_unprinted_equals_nodes": len(printed) + (len(rows) - len(printed)) == len(rows),
            "eligibility_counts_sum_to_printed": sum(collections.Counter(r["eligibility"] for r in printed).values()) == len(printed),
            "identity_occurrences_cover_vocab_nodes": sum(int(i["occurrences_resolved"]) for i in id_rows) ==
                sum(1 for r in vocab if r["identity_status"] == "resolved"),
        },
        "ud_join": crosscheck["join"],
        "ud_lysias1_sentences": crosscheck["lysias1_sentences_in_ud"],
    }


def main(argv):
    if len(argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    summary = derive(argv[1], argv[2])
    print(json.dumps({k: summary[k] for k in ("printed_by_eligibility", "vocabulary_nodes_by_identity_status",
                                              "identities_by_status")}, ensure_ascii=False, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
