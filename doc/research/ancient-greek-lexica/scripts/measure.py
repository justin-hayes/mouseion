"""Book 1 candidate-set measurement (research source discovery, not acceptance).

For every vocabulary identity in the pinned OGA file's Xenophon, Anabasis 1
tokens, reports whether each source has entries under the same canonical
headword. A "candidate set" is every entry sharing that headword. Nothing here
resolves a homograph, maps a sense, or measures sense accuracy. Homograph digits
are never a match criterion.

Usage (works from any working directory):
  python3 -I /abs/path/scripts/measure.py OGA_CONLLU KAIKKI_GRC_TOP_JSONL LSJ_DIR ML_XML OUT_JSON
"""
import collections
import glob
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)
from betacode import beta_to_greek, canonical  # noqa: E402

OGA, KAIKKI, LSJ_DIR, ML_XML, OUT = sys.argv[1:6]
MISC_BOOK1 = re.compile(r'ref=Xen\. Anab\._1\.(\d+)\.(\d+)')
TRAIL = re.compile(r'(\d+)$')
# Perseus LSJ passage URNs: tlg0032 (Xenophon) tlg006 (Anabasis), book 1.
URN_B1 = re.compile(r'urn:cts:greekLit:tlg0032\.tlg006\.perseus-grc\d:1:[0-9:]+')
VOCAB = {'n': 'NOUN', 'a': 'ADJ', 'd': 'ADV', 'v': 'VERB'}  # native AGDT letters, heuristic
KAIKKI_CLASS = {'noun': 'NOUN', 'name': 'NOUN', 'adj': 'ADJ', 'adv': 'ADV', 'verb': 'VERB'}
EIMI = canonical('εἰμί')


def iter_senses(senses):
    for s in senses or []:
        yield s
        yield from iter_senses(s.get('senses'))


# ---- OGA: Book 1 tokens ----------------------------------------------------
native = 0          # every token whose MISC carries a Book 1 ref (includes '-')
letters = collections.Counter()
dash = 0
excluded = collections.Counter()
vocab = []          # (form, lemma, class)
with open(OGA, encoding='utf-8') as f:
    for line in f:
        line = line.rstrip('\n')
        if not line or line.startswith('#'):
            continue
        cols = line.split('\t')
        if len(cols) != 10 or not cols[0].isdigit():
            continue
        if not MISC_BOOK1.search(cols[9]):
            continue
        native += 1
        form, lemma, pos = cols[1], cols[2], cols[3]
        letters[pos] += 1
        if pos == '-':
            dash += 1
            continue
        if pos not in VOCAB:
            excluded['pos=' + pos] += 1
            continue
        vocab.append((form, lemma, VOCAB[pos]))

ident = collections.Counter()
ident_cap = collections.Counter()
for form, lemma, cls in vocab:
    head = TRAIL.sub('', lemma)
    d = TRAIL.search(lemma)
    disc = int(d.group(1)) if d else None
    canon = canonical(head)
    if canon == EIMI:
        excluded['eimi (exact lemma)'] += 1
        continue
    key = (canon, disc, cls)
    ident[key] += 1
    if form[:1].isupper():
        ident_cap[key] += 1
n_ident = len(ident)
numbered = sum(1 for k in ident if k[1] is not None)
occ_total = sum(ident.values())

# ---- Kaikki (raw Wiktextract, top-level grc) ------------------------------
kaikki = collections.defaultdict(list)
kaikki_entries = 0
anab_any_refs = 0      # every sense example whose ref names Xenophon, Anabasis
anab1_refs = 0         # ... restricted to Anabasis book 1
anab1_headwords = set()
with open(KAIKKI, encoding='utf-8') as f:
    for line in f:
        d = json.loads(line)
        if d.get('lang_code') != 'grc':
            continue
        kaikki_entries += 1
        hw = canonical(d['word'])
        b1 = 0
        for s in iter_senses(d.get('senses')):
            for e in s.get('examples') or []:
                ref = e.get('ref') or ''
                if 'Xenophon, Anabasis' in ref:
                    anab_any_refs += 1
                if 'Xenophon, Anabasis 1.' in ref:
                    b1 += 1
        anab1_refs += b1
        if b1:
            anab1_headwords.add(hw)
        kaikki[hw].append({'pos': d.get('pos'), 'etym': d.get('etymology_number'), 'b1': b1})


# ---- Perseus TEI-P4 dictionaries (LSJ, Middle Liddell) ---------------------
def load_perseus(paths):
    idx = collections.defaultdict(list)
    unknown_canon = set()   # canonical forms of keys excluded for unknown chars
    unknown_keys = []
    for path in paths:
        s = open(path, encoding='utf-8').read()
        for m in re.finditer(r'<(entryFree|entry)([^>]*)>(.*?)</\1>', s, re.S):
            attrs, body = m.group(2), m.group(3)
            if 'type="main"' not in attrs:
                continue
            k = re.search(r'key="([^"]*)"', attrs).group(1)
            g, num, unk = beta_to_greek(k)
            if unk:
                # Converter drops unknown characters, so these keys could also
                # collide with real headwords. Keep them out of matching entirely.
                unknown_keys.append(k)
                unknown_canon.add(canonical(g))
                continue
            idx[canonical(g)].append({'key': k, 'digit': num, 'urns': len(URN_B1.findall(body))})
    return idx, unknown_canon, unknown_keys


lsj_files = sorted(glob.glob(os.path.join(LSJ_DIR, 'grc.lsj.perseus-eng*.xml')))
lsj_idx, lsj_unk_canon, lsj_unk_keys = load_perseus(lsj_files)
lsj_urn_files_total = sum(len(URN_B1.findall(open(p, encoding='utf-8').read())) for p in lsj_files)
ml_idx, ml_unk_canon, ml_unk_keys = load_perseus([ML_XML])  # ML has no structured passage URNs


# ---- candidate-set measurements -------------------------------------------
def kaikki_cands(canon, cls):
    return kaikki.get(canon, [])


def perseus_cands(idx, canon, cls):
    return idx.get(canon, [])


def summarize(name, cands, unk_canon, pos_ok=None, urn=False):
    """Candidate-level counts for one source. Keys with unknown chars are not in cands."""
    out = collections.Counter()
    out['identities'] = n_ident
    for (canon, disc, cls) in ident:
        c = cands(canon, cls)
        if c:
            out['headword_any_entry'] += 1
            if len(c) > 1:
                out['headword_multi_entry_candidate_set'] += 1
        elif canon in unk_canon:
            out['only_excluded_unknown_char_key'] += 1
        if pos_ok and any(pos_ok(e, cls) for e in c):
            out['headword_pos_compatible_entry'] += 1
        if disc is None:
            out['plain_identities'] += 1
            if any(e.get('digit') is None for e in c if 'digit' in e):
                out['plain_identity_unnumbered_key_present'] += 1
        else:
            out['numbered_identities'] += 1
            if any(e.get('digit') is not None for e in c if 'digit' in e):
                out['numbered_identity_any_homograph_candidate'] += 1
            if any(e.get('digit') == disc for e in c if 'digit' in e):
                # Not a match criterion; disclosed because digit overlap is the tempting shortcut.
                out['numbered_identity_equal_digit_UNVERIFIED'] += 1
        if urn and any(e.get('urns') for e in c):
            out['identity_with_book1_urn_candidate'] += 1
        if ident_cap[(canon, disc, cls)] == ident[(canon, disc, cls)]:
            out['capitalised_only_identities'] += 1
            if c:
                out['capitalised_only_any_entry'] += 1
    tok = collections.Counter()
    for (canon, disc, cls), n in ident.items():
        if cands(canon, cls):
            tok['occurrences_with_any_entry'] += n
    out['occurrences'] = occ_total
    out['occurrences_with_any_entry'] = tok['occurrences_with_any_entry']
    return dict(out)


def pos_ok_kaikki(e, cls):
    return KAIKKI_CLASS.get(e['pos']) == cls


res = {
    'oga': {
        'native_book1_tokens_all': native,
        'pos_dash_empty_nodes_excluded': dash,
        'printed_book1_tokens': native - dash,
        'vocabulary_occurrences_after_pos_and_eimi': occ_total,
        'excluded': dict(excluded),
        'pos_letter_counts': dict(letters),
        'identities': n_ident,
        'numbered_identities': numbered,
        'plain_identities': n_ident - numbered,
        'capitalised_only_identities': sum(1 for k in ident if ident_cap[k] == ident[k]),
    },
    'kaikki_grc': {
        'top_level_entries': kaikki_entries,
        'distinct_canonical_headwords': len(kaikki),
        'headwords_with_more_than_one_entry': sum(1 for v in kaikki.values() if len(v) > 1),
        'anabasis_example_refs_any_book': anab_any_refs,
        'anabasis_book1_example_refs': anab1_refs,
        'anabasis_book1_distinct_headwords': len(anab1_headwords),
        'identities_whose_headword_has_book1_example': sum(
            1 for (c, d, k) in ident if c in anab1_headwords),
    },
    'lsj': {
        'main_entries_indexed': sum(len(v) for v in lsj_idx.values()),
        'keys_excluded_unknown_chars': len(lsj_unk_keys),
        'urn_book1_strings_all_lsj_files': lsj_urn_files_total,
    },
    'middle_liddell': {
        'main_entries_indexed': sum(len(v) for v in ml_idx.values()),
        'keys_excluded_unknown_chars': len(ml_unk_keys),
    },
}

res['candidate_sets'] = {
    'kaikki_grc': summarize('kaikki', kaikki_cands, set(), pos_ok_kaikki),
    'lsj': summarize('lsj', lambda c, cls: perseus_cands(lsj_idx, c, cls), lsj_unk_canon, urn=True),
    'middle_liddell': summarize('ml', lambda c, cls: perseus_cands(ml_idx, c, cls), ml_unk_canon),
}

json.dump(res, open(OUT, 'w', encoding='utf-8'), ensure_ascii=False, indent=1)
print(json.dumps(res, ensure_ascii=False, indent=1))
