#!/usr/bin/env python3
"""Lexical evidence for the Lysias I T1A2 vocabulary-blocked occurrences (#2049).

Reads the #2048 reconciliation manifest (route T1A2) and the pinned primary sources, runs
the named checks, and writes per-occurrence before/after evidence. Stdlib only, deterministic,
ASCII output (Greek as U+XXXX). Nothing is adopted: every eligibility, UPOS, headword and
discriminator stays as in the 50f4ad6c mapping, and each proposal is recorded separately
for a parent decision.

Usage: python3 -I lexical_completion.py <reconciliation-manifest-dir> <sources-dir> <out-dir>
"""
import csv
import hashlib
import json
import os
import re
import sys
import unicodedata
from collections import Counter, OrderedDict

ROUTE = 'T1A2'
EXPECT = {'nodes': 2834, 'word': 2428, 'punct': 374, 'artificial': 32, 'vocab_blocked': 235}
HERE = os.path.dirname(os.path.abspath(__file__))
SUMS = os.path.join(HERE, '..', 'inputs', 'sha256sums.txt')
GREEK = re.compile('[Ͱ-Ͽἀ-῿]')
WORD = re.compile(r'(?:[^\W_]|[̀-ͯ])+')
MORPH = 'ext/perseids-tools/morpheus/stemlib/Greek/'

SRC = {
    'agdt_gl': 'sources/AGDT2/guidelines/Greek_guidelines.md',
    'smyth': 'smyth/perseus-smyth-2769.txt',
    'ud_grc_adv': 'ext/UniversalDependencies/docs/_grc/pos/ADV.md',
    'ud_grc_aux': 'ext/UniversalDependencies/docs/_grc/pos/AUX_.md',
    'ud_grc_cconj': 'ext/UniversalDependencies/docs/_grc/pos/CCONJ.md',
    'ud_grc_noun': 'ext/UniversalDependencies/docs/_grc/pos/NOUN.md',
    'ud_grc_part': 'ext/UniversalDependencies/docs/_grc/pos/PART.md',
    'ud_grc_propn': 'ext/UniversalDependencies/docs/_grc/pos/PROPN.md',
    'ud_grc_sconj': 'ext/UniversalDependencies/docs/_grc/pos/SCONJ.md',
    'ud_grc_verb': 'ext/UniversalDependencies/docs/_grc/pos/VERB.md',
    'ud_part_gen': 'ext/UniversalDependencies/docs/_u-pos/PART.md',
    'ud_propn_gen': 'ext/UniversalDependencies/docs/_u-pos/PROPN.md',
    'ud_cop': 'ext/UniversalDependencies/docs/_u-dep/cop.md',
    'mor_lemlist': MORPH + 'stemsrc/lemlist',
    'mor_nomproper': MORPH + 'stemsrc/nom.proper',
    'mor_vbs_simp': MORPH + 'stemsrc/vbs.simp.ml',
    'mor_vbs_cmp': MORPH + 'stemsrc/vbs.cmp.ml',
    'mor_lsj_vbs': MORPH + 'stemsrc/lsj.vbs',
    'mor_entitylist': MORPH + 'steminds/entitylist.txt',
}
UD_GRC = ['ud_grc_adv', 'ud_grc_aux', 'ud_grc_cconj', 'ud_grc_noun', 'ud_grc_part',
          'ud_grc_propn', 'ud_grc_sconj', 'ud_grc_verb']
UD_GEN = ['ud_part_gen', 'ud_propn_gen', 'ud_cop']

# Lemmas that are the grave-accent spelling of a rule-list lemma. Reported, never folded.
VARIANT_HOLD = {'καὶ': 'καί', 'μὴ': 'μή'}
# Hand-written Beta Code keys for the six name lemmas (Morpheus: '*' capital, '(' rough,
# ')' smooth, '/' acute, '=' circumflex).
NAME_BETA = OrderedDict([
    ('Ἑλλάς', '*(ella/s'),
    ('Θεσμοφόρια', '*qesmofo/ria'),
    ('Ἀθηναῖος', '*)aqhnai=os'),
    ('Ἄρειας', '*)/areias'),
    ('πᾶγος', 'pa=gos'),
    ('πάγος', 'pa/gos'),
])
# Headword checks: (headword as the node carries it, Morpheus prefix P, stem L, v1.6 homograph).
HEADWORDS = [
    ('ἀντί-βολέω', 'a)nti/', 'bole/w', ''),
    ('σύν-λέγω', 'su/n', 'le/gw', '1'),
]


def nfc(text):
    return unicodedata.normalize('NFC', text)


def unaccented(text):
    # Reporting only. Never used to build a key, a rule, or a status.
    return ''.join(c for c in unicodedata.normalize('NFD', text) if not unicodedata.combining(c))


def cp2str(field):
    return ''.join(chr(int(t[2:], 16)) for t in field.split()) if field else ''


def str2cp(text):
    return ' '.join('U+%04X' % ord(c) for c in text)


def ascii_cell(value):
    text = '' if value is None else str(value)
    return text if all(ord(c) < 128 for c in text) else str2cp(text)


def read_text(path):
    with open(path, encoding='utf-8') as f:
        return f.read()


def read_tsv(path):
    with open(path, encoding='utf-8', newline='') as f:
        return list(csv.DictReader(f, delimiter='\t'))


def sha256(path):
    with open(path, 'rb') as f:
        return hashlib.sha256(f.read()).hexdigest()


def verify_inputs(sources):
    entries = [line.split('  ', 1) for line in read_text(SUMS).splitlines() if line.strip()]
    bad = [rel for digest, rel in entries if sha256(os.path.join(sources, rel)) != digest]
    if bad:
        raise SystemExit('sha256 mismatch: ' + ', '.join(bad))
    return len(entries)


# ---------------------------------------------------------------- source parsers

def agdt_conjunction_particles(guideline):
    """Items of the AGDT 4.1.12.1 conjunction-particle list, in document order."""
    lines = [l.strip() for l in re.sub(r'<[^>]+>', '\n', nfc(guideline)).split('\n')]
    start = lines.index('4.1.12.1 Particle')
    begin = next(i for i in range(start, len(lines)) if lines[i].endswith('such as:'))
    items = []
    for line in lines[begin + 1:]:
        if line.startswith('Some of these'):
            break
        # Lines may carry an inline image marker ("δέ  tree1"): keep the first Greek token.
        greek = [t for t in line.split() if GREEK.search(t)]
        if greek:
            items.append(greek[0])
    return items


def smyth_particle_tokens(page):
    """Greek headings of the Smyth 2769 PARTICLES list in the main text.

    The sidebar also has a PARTICLES entry followed by ἀλλά; the list is the last such heading.
    It ends at the next all-caps section heading (SOME GRAMMATICAL AND RHETORICAL FIGURES).
    Inline sub-labels in English are skipped; Greek variants inside parentheses are kept.
    """
    lines = [l.strip() for l in nfc(page).split('\n')]
    nonblank = [i for i, l in enumerate(lines) if l]
    heads = [i for i in nonblank if lines[i] == 'PARTICLES' and
             lines[nonblank[nonblank.index(i) + 1]] == 'ἀλλά']
    tokens = []
    for i in nonblank[nonblank.index(heads[-1]) + 1:]:
        if re.match(r'^[A-Z][A-Z ,.\-]{6,}$', lines[i]):
            break
        line = lines[i].replace('<*>', '').replace('_', '')
        tokens.extend(t for t in re.split(r'[,\s()（）]+', line) if t and GREEK.search(t))
    return tokens


def doc_tokens(text):
    return set(nfc(m) for m in WORD.findall(nfc(re.sub(r'<[^>]+>', ' ', text))))


def lemma_status(lemma, tokens):
    """exact | accent_variant | absent. The variant test is reporting only."""
    if lemma in tokens:
        return 'exact'
    if any(unaccented(t) == unaccented(lemma) for t in tokens):
        return 'accent_variant'
    return 'absent'


def morph_entries(text, kind):
    """(line_no, key, line) for each Morpheus entry, by the file's own entry format."""
    out = []
    for no, raw in enumerate(text.split('\n'), 1):
        line = raw.rstrip()
        if kind == 'lemlist':
            key = line.strip()
        elif kind == 'entitylist':
            key = line.split('\t')[0].strip()
        elif kind == 'le':
            if not line.startswith(':le:'):
                continue
            key = line[4:].strip()
        else:  # vbs.cmp.ml: "<compound> <head>"
            key = line.split(' ')[0] if line else ''
        if key:
            out.append((no, key, line))
    return out


def le_blocks(text):
    """(key, first_line_no, block_text) for each ':le:' entry; a block ends at a blank line."""
    lines = text.split('\n')
    out = []
    for i, raw in enumerate(lines):
        if raw.startswith(':le:'):
            j = i + 1
            while j < len(lines) and lines[j].strip():
                j += 1
            out.append((raw[4:].strip(), i + 1, '\n'.join(lines[i:j])))
    return out


def has_token(block, token):
    return any(t in (token, 'r' + token) for t in block.split())


def split_homograph(key):
    m = re.match(r'^(.*?)(\d*)$', key)
    return m.group(1), m.group(2)


def beta_name(key):
    # Reporting only: capital marker and breathings removed, accents kept.
    return re.sub(r'[*()]', '', key)


# ---------------------------------------------------------------- checks

def name_checks(base, name_nodes):
    lemlist = morph_entries(read_text(os.path.join(base, SRC['mor_lemlist'])), 'lemlist')
    nomp = morph_entries(read_text(os.path.join(base, SRC['mor_nomproper'])), 'le')
    ent = morph_entries(read_text(os.path.join(base, SRC['mor_entitylist'])), 'entitylist')
    rows, results = [], {}
    for lemma, beta in NAME_BETA.items():
        occ = [n for n in name_nodes if n['lemma'] == lemma]
        exact = {
            'lemlist': [n for n, k, _ in lemlist if k == beta],
            'nom_proper': [n for n, k, _ in nomp if k == beta],
            'entitylist': [(n, l.split('\t', 1)[1]) for n, k, l in ent if k == beta],
        }
        near = [n for n, k, _ in nomp if k != beta and beta_name(k) == beta_name(beta)]
        results[lemma] = {'nom_proper_exact': bool(exact['nom_proper']),
                          'nom_proper_near': bool(near),
                          'lemlist_exact': bool(exact['lemlist'])}
        rows.append(OrderedDict([
            ('lemma_v21_cp', str2cp(lemma)), ('occurrences', len(occ)),
            ('nodes', ';'.join(o['node_key'] for o in occ)),
            ('morpheus_beta_key', beta),
            ('lemlist_exact_lines', ';'.join(map(str, exact['lemlist']))),
            ('nom_proper_exact_lines', ';'.join(map(str, exact['nom_proper']))),
            ('nom_proper_near_lines_breathing_insensitive', ';'.join(map(str, near))),
            ('entitylist_exact', ';'.join(t.replace('\t', ' ') for _, t in exact['entitylist'])),
        ]))
    return rows, results


def headword_checks(base):
    simp = read_text(os.path.join(base, SRC['mor_vbs_simp']))
    lsj = read_text(os.path.join(base, SRC['mor_lsj_vbs']))
    cmp_rows = morph_entries(read_text(os.path.join(base, SRC['mor_vbs_cmp'])), 'cmp')
    rows, results = [], {}
    for hw, P, L, hom in HEADWORDS:
        all_blocks = [(k, n, b, 'vbs.simp.ml') for k, n, b in le_blocks(simp)] + \
                     [(k, n, b, 'lsj.vbs') for k, n, b in le_blocks(lsj)]
        # H1: the stem L's own :le: entry carries pb:P or rpb:P.
        h1 = [(f, n) for k, n, b, f in all_blocks if split_homograph(k)[0] == L and has_token(b, 'pb:' + P)]
        # H2: a vbs.cmp row "P-L" whose homograph number matches the v1.6 witness.
        h2_rows = [(n, k) for n, k, _ in cmp_rows if split_homograph(k)[0] == P + '-' + L]
        h2 = [(n, k) for n, k in h2_rows if not hom or split_homograph(k)[1] == hom]
        # H3: :le: entries carrying pb:P (candidate count; the L stem/class condition is not decided here).
        h3 = [(f, n, k) for k, n, b, f in all_blocks if has_token(b, 'pb:' + P)]
        results[hw] = {'H1': bool(h1), 'H2': bool(h2), 'H3_candidates': len(h3)}
        rows.append(OrderedDict([
            ('headword_v16_cp', str2cp(hw)), ('morpheus_P', P), ('morpheus_L', L),
            ('v16_homograph', hom or 'none'),
            ('H1_L_entry_with_pb_P_lines', ';'.join('%s:%d' % (f, n) for f, n in h1)),
            ('H2_cmp_rows_P_hyphen_L', ';'.join('%d:%s' % (n, k) for n, k in h2_rows)),
            ('H2_homograph_match_lines', ';'.join('%d:%s' % (n, k) for n, k in h2)),
            ('H3_entries_with_pb_P', ';'.join('%s:%d:%s' % (f, n, k) for f, n, k in h3)),
        ]))
    return rows, results


def eimi_checks(eimi_nodes):
    rows = []
    for n in eimi_nodes:
        pos = n['postag_v21']
        tense, voice = pos[3], pos[5]
        rows.append(OrderedDict([
            ('node_key', n['node_key']), ('form_v21_cp', n['form_v21_cp']), ('postag_v21', pos),
            ('tense_letter', tense), ('voice_letter', voice), ('relation', n['relation']),
            ('ud_aux_perfect_middle', tense == 'r' and voice in 'mp'),
            ('cop_relation', n['relation'] == 'cop'),
        ]))
    return rows


# ---------------------------------------------------------------- decisions

def decide(node, ctx):
    """The proposal (if any) and the open points for one occurrence. Never adopted."""
    rule, lemma = node['m_rule'], node['lemma']
    smyth = ctx['smyth'][lemma]
    named_grc = ctx['named_grc'][lemma]
    d = {'pid': '', 'upos': '', 'basis': '', 'unsupported': '', 'unperformed': ''}
    particle_basis = ('AGDT 4.1.12.1: particles are "adverbs, typically sentence adverbs"; '
                      'lemma is outside the AGDT conjunction-particle list')
    smyth_note = '' if smyth == 'exact' else 'Smyth 2769 PARTICLES: %s' % smyth
    if rule == 'R03P':
        if lemma in VARIANT_HOLD:
            d.update(pid='P04-VARIANT-HOLD', unsupported='grave-accent spelling of a rule-list lemma; '
                     'reported, not folded; no proposal')
        else:
            d.update(pid='P01-PARTICLE-ADV', upos='ADV', basis=particle_basis,
                     unsupported=smyth_note)
    elif rule == 'R03A':
        if lemma in ('γάρ', 'οὖν', 'τοίνυν'):
            d.update(pid='P03-GAR-OUN-TOINUN-ADV', upos='ADV', basis=particle_basis,
                     unsupported=smyth_note)
        else:
            d.update(unsupported='no pinned rule for this lemma under R03A')
    elif rule == 'R03C':
        d.update(pid='P02-CONJPART-OPEN', upos='',
                 unsupported='AGDT 4.1.12.1 lists the lemma as a conjunction particle; the text says '
                 'conjunctions "have to be always annotated as conjunctions" but "some of these '
                 'particles can of course be adverbs". No pinned rule decides AuxY/AuxZ.')
    elif rule == 'R03N':
        d.update(pid='P05-NEG-PART', upos='PART',
                 basis='UD general PART doc lists negation as a PART category',
                 unsupported='UD grc docs do not name this lemma' if not named_grc
                 else 'UD grc docs name this lemma: ' + ';'.join(named_grc))
    elif rule == 'R03S':
        d.update(unsupported='no pinned rule for this lemma with AuxY/AuxZ (candidates ADV|PART|SCONJ)')
    elif rule == 'R05X':
        tense, voice = node['postag_v21'][3], node['postag_v21'][5]
        ok = tense == 'r' and voice in 'mp'
        d.update(pid='P06-EIMI-VERB', upos='VERB' if not ok else 'AUX',
                 basis='UD grc AUX doc: AUX only for perfect middle εἰμί and periphrastic use; '
                 'form tense letter %s voice letter %s' % (tense, voice),
                 unsupported='' if not ok else 'form is perfect middle; AUX rule applies')
    elif rule == 'R01A':
        d.update(pid='P07-SUBST-ADJ-ADJ', upos='ADJ',
                 basis='AGDT 2.2: adjective used as noun with no separate lemma is categorized as '
                 'adjective; UD grc NOUN doc cites the lemma as a substantivized adjective',
                 unperformed='LSJ separate-entry check not performed',
                 unsupported='Morpheus nom.proper: breathing-insensitive near match only, no exact key')
    elif rule == 'R01C':
        if lemma == 'Ἑλλάς':
            d.update(pid='P08-PLACE-PROPN', upos='PROPN',
                     basis='UD grc PROPN doc: unique object such as a person or a country',
                     unsupported='Morpheus nom.proper has no entry (lemlist only)')
        else:
            d.update(unsupported='capitalized lemma with no proposal; name-component reading not decided'
                     + ('; festival, not a person or country' if lemma == 'Θεσμοφόρια' else
                        '; Morpheus lemlist only, no nom.proper entry'))
    elif rule == 'R01M':
        d.update(unsupported='R01M NOUN|PROPN; no primary check decides the name-component status',
                 unperformed='syntactic join to the adjacent R01C word taken from the 50f4ad6c rule')
    elif rule == 'R06':
        h = ctx['hw'][lemma]
        facts = 'H1 %s; H2 %s; H3 candidates %d' % (h['H1'], h['H2'], h['H3_candidates'])
        if lemma == 'ἀντί-βολέω':
            d.update(pid='P10-HW-H3-CANDIDATE',
                     unsupported='%s; the H3 stem-and-class condition is not decided; '
                     'headword status unchanged' % facts,
                     unperformed='H3 stem-and-class condition not checked')
        else:
            d.update(pid='P11-HW-NONE', unsupported='%s; H2 row has homograph 2, node has 1' % facts)
    else:
        d.update(unsupported='rule %s outside the vocabulary-blocked set' % rule)
    return d


def labels(d):
    out = ['exact_fact']
    if d['upos']:
        out.append('proposed_interpretation')
    if d['unsupported']:
        out.append('unsupported_case')
    if d['unperformed']:
        out.append('unperformed_check')
    return ';'.join(out)


# ---------------------------------------------------------------- build

def build(manifest_dir, sources, out_dir):
    n_inputs = verify_inputs(sources)
    nodes_all = read_tsv(os.path.join(manifest_dir, 'nodes.tsv'))
    nodes = {r['node_key']: r for r in nodes_all}
    route_rows = [r for r in read_tsv(os.path.join(manifest_dir, 'route_nodes.tsv')) if r['route'] == ROUTE]
    blockers = [r for r in read_tsv(os.path.join(manifest_dir, 'blockers.tsv')) if r['route'] == ROUTE]

    classes = Counter(r['node_class'] for r in nodes_all)
    assert len(nodes_all) == EXPECT['nodes'], len(nodes_all)
    assert (classes['word'], classes['punct'], classes['artificial']) == \
        (EXPECT['word'], EXPECT['punct'], EXPECT['artificial']), classes
    vocab = [r['node_key'] for r in route_rows if r['readiness'] == 'vocab_blocked']
    assert len(vocab) == EXPECT['vocab_blocked'], len(vocab)
    vocab.sort(key=lambda k: int(nodes[k]['doc_order']))
    vocab_set = set(vocab)
    bi = {b['blocker']: set(b['node_keys'].split(';')) for b in blockers if b['blocker'].startswith('BI-')}
    assert set().union(*bi.values()) == vocab_set, 'BI blockers do not reconcile to vocab_blocked'
    s011 = {k for b in blockers if b['blocker'] == 'BT-S011-ORDER' for k in b['node_keys'].split(';')}
    blocker_of = {k: sorted(b for b, keys in bi.items() if k in keys) for k in vocab}
    for k in vocab:
        if k in s011:
            blocker_of[k].append('BT-S011-ORDER')

    cand = []
    for k in vocab:
        n = dict(nodes[k])
        n['lemma'] = cp2str(n['lemma_v21_cp'])
        n['form'] = cp2str(n['form_v21_cp'])
        cand.append(n)

    smyth_tokens = smyth_particle_tokens(read_text(os.path.join(sources, SRC['smyth'])))
    agdt = agdt_conjunction_particles(read_text(os.path.join(sources, SRC['agdt_gl'])))
    agdt_tokens = [nfc(x) for x in agdt]
    ud_tokens = {k: doc_tokens(read_text(os.path.join(sources, SRC[k]))) for k in UD_GRC + UD_GEN}

    lemmas = sorted({n['lemma'] for n in cand})
    ctx = {'smyth': {}, 'named_grc': {}, 'named_gen': {}, 'agdt': {}}
    lemma_rows = []
    for lemma in lemmas:
        ctx['smyth'][lemma] = lemma_status(lemma, smyth_tokens)
        ctx['agdt'][lemma] = lemma_status(lemma, agdt_tokens)
        ctx['named_grc'][lemma] = [k for k in UD_GRC if lemma in ud_tokens[k]]
        ctx['named_gen'][lemma] = [k for k in UD_GEN if lemma in ud_tokens[k]]
        lemma_rows.append(OrderedDict([
            ('lemma_v21_cp', str2cp(lemma)),
            ('occurrences', sum(1 for n in cand if n['lemma'] == lemma)),
            ('smyth_2769_particles', ctx['smyth'][lemma]),
            ('agdt_4_1_12_1_conjunction_list', ctx['agdt'][lemma]),
            ('ud_grc_named_in', ';'.join(ctx['named_grc'][lemma])),
            ('ud_general_named_in', ';'.join(ctx['named_gen'][lemma])),
        ]))

    name_nodes = [n for n in cand if n['m_rule'] in ('R01A', 'R01C', 'R01M')]
    eimi_nodes = [n for n in cand if n['m_rule'] == 'R05X']
    name_rows, name_res = name_checks(sources, name_nodes)
    hw_rows, hw_res = headword_checks(sources)
    eimi_rows = eimi_checks(eimi_nodes)
    ctx['name'] = name_res
    ctx['hw'] = hw_res

    occ_rows = []
    proposal_counts = Counter()
    label_counts = Counter()
    for n in cand:
        d = decide(n, ctx)
        lab = labels(d)
        proposal_counts[d['pid'] or 'none'] += 1
        for item in lab.split(';'):
            label_counts[item] += 1
        checks = ['AGDT-4.1.12.1', 'SMYTH-2769'] + ['UD-' + k for k in ctx['named_grc'][n['lemma']]] + \
            ['UD-' + k for k in ctx['named_gen'][n['lemma']]]
        if n['m_rule'] in ('R01A', 'R01C', 'R01M'):
            checks.append('MOR-NOMPROPER')
        if n['m_rule'] == 'R05X':
            checks.append('UD-AUX-COP')
        if n['m_rule'] == 'R06':
            checks.append('MOR-HEADWORD')
        occ_rows.append(OrderedDict([
            ('document', 'tlg0540.tlg001.perseus-grc1'),
            ('primary_version', 'v1.6'), ('evidence_version', 'v2.1'),
            ('sentence_id', n['sentence_id']), ('word_id', n['word_id']),
            ('node_key', n['node_key']), ('doc_order', n['doc_order']),
            ('form_v21_cp', n['form_v21_cp']),
            ('form_v21_nfc_cp', n['form_v21_nfc_cp'] or n['form_v21_cp']),
            ('lemma_v21_cp', n['lemma_v21_cp']),
            ('lemma_v16_cp', n['lemma_v16_cp'] or '(equal to v2.1)'),
            ('postag_v21', n['postag_v21']), ('relation', n['relation']), ('head', n['head']),
            ('rule_before', n['m_rule']), ('upos_candidates_before', n['m_upos_candidates']),
            ('upos_status_before', n['m_upos_status']), ('eligibility_before', n['m_eligibility']),
            ('headword_before_cp', n['m_headword_cp']), ('headword_status_before', n['m_headword_status']),
            ('discriminator_before', n['m_disc_status'] + ':' + n['m_disc_value']),
            ('blockers_before', ';'.join(blocker_of[n['node_key']])),
            ('proposal_id', d['pid']), ('upos_proposed', d['upos']), ('proposal_basis', d['basis']),
            ('unsupported_or_open', d['unsupported']), ('unperformed', d['unperformed']),
            ('finding_labels', lab), ('evidence_checks', ';'.join(checks)),
            ('adoption_status', 'not_adopted (parent decision)'),
            ('upos_after', n['m_upos_candidates']), ('eligibility_after', n['m_eligibility']),
            ('headword_after_cp', n['m_headword_cp']), ('headword_status_after', n['m_headword_status']),
            ('discriminator_after', n['m_disc_status'] + ':' + n['m_disc_value']),
            ('blockers_after', ';'.join(blocker_of[n['node_key']])),
            ('status_after', 'vocab_blocked (unchanged)'),
            ('evidence_ids_before', n['m_evidence_ids']),
        ]))

    summary = OrderedDict([
        ('inputs_verified', n_inputs),
        ('nodes', len(nodes_all)),
        ('node_classes', OrderedDict(sorted(classes.items()))),
        ('t1a2_vocab_blocked', len(cand)),
        ('t1a2_vocab_blocked_rules', OrderedDict(sorted(Counter(n['m_rule'] for n in cand).items()))),
        ('blocker_counts', OrderedDict(sorted(Counter(b for k in vocab for b in blocker_of[k]).items()))),
        ('overlap_BI_ELIG_PARTICLE_and_BT_S011_ORDER',
         sorted(k for k in bi['BI-ELIG-PARTICLE'] if k in s011)),
        ('proposal_counts', OrderedDict(sorted(proposal_counts.items()))),
        ('finding_label_counts', OrderedDict(sorted(label_counts.items()))),
        ('eligibility_changes', sum(1 for o in occ_rows if o['eligibility_after'] != o['eligibility_before'])),
        ('smyth_2769_heading_tokens', len(smyth_tokens)),
        ('particle_occurrences', sum(1 for n in cand if n['m_rule'].startswith('R03'))),
        ('particle_occurrences_smyth_2769',
         OrderedDict(sorted(Counter(ctx['smyth'][n['lemma']] for n in cand
                                    if n['m_rule'].startswith('R03')).items()))),
        ('particle_occurrences_agdt_4_1_12_1',
         OrderedDict(sorted(Counter(ctx['agdt'][n['lemma']] for n in cand
                                    if n['m_rule'].startswith('R03')).items()))),
        ('agdt_conjunction_particles', agdt),
        ('headword_results', hw_res),
        ('name_results', name_res),
    ])

    os.makedirs(out_dir, exist_ok=True)
    write_tsv(os.path.join(out_dir, 'occurrence_patches.tsv'), occ_rows)
    write_tsv(os.path.join(out_dir, 'lemma_evidence.tsv'), lemma_rows)
    write_tsv(os.path.join(out_dir, 'name_checks.tsv'), name_rows)
    write_tsv(os.path.join(out_dir, 'headword_checks.tsv'), hw_rows)
    write_tsv(os.path.join(out_dir, 'eimi_checks.tsv'), eimi_rows)
    with open(os.path.join(out_dir, 'summary.json'), 'w', encoding='ascii') as f:
        json.dump(summary, f, indent=2, ensure_ascii=True)
        f.write('\n')
    return summary, occ_rows


def write_tsv(path, rows):
    with open(path, 'w', encoding='ascii', newline='') as f:
        w = csv.writer(f, delimiter='\t', lineterminator='\n')
        w.writerow(list(rows[0].keys()))
        for r in rows:
            w.writerow([ascii_cell(v) for v in r.values()])


def main(argv):
    if len(argv) != 4:
        print(__doc__)
        return 2
    summary, _ = build(argv[1], argv[2], argv[3])
    print(json.dumps(summary, ensure_ascii=True, indent=2))
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
