"""Candidate-tier counts for the proposed OGA identity crosswalk (research only).

Reuses measure.py's identity rules by importing its helpers would run its whole
report, so the identity and candidate rules are restated here and must stay in
step with measure.py. The OGA discriminator (morpheus digit) is never compared
with a dictionary number. Tiers describe how many candidates exist, not which
sense is right.

Usage (any working directory; absolute paths):
  python3 -I /abs/path/scripts/crosswalk_tiers.py OGA_CONLLU KAIKKI_GRC_TOP_JSONL LSJ_DIR ML_XML OUT_JSON
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
VOCAB = {'n': 'NOUN', 'a': 'ADJ', 'd': 'ADV', 'v': 'VERB'}
KAIKKI_CLASS = {'noun': 'NOUN', 'name': 'NOUN', 'adj': 'ADJ', 'adv': 'ADV', 'verb': 'VERB'}
EIMI = canonical('εἰμί')

# OGA identities, same rules as measure.py
idents = set()
with open(OGA, encoding='utf-8') as f:
    for line in f:
        line = line.rstrip('\n')
        if not line or line.startswith('#'):
            continue
        cols = line.split('\t')
        if len(cols) != 10 or not cols[0].isdigit() or not MISC_BOOK1.search(cols[9]):
            continue
        lemma, pos = cols[2], cols[3]
        if pos not in VOCAB:
            continue
        head = TRAIL.sub('', lemma)
        d = TRAIL.search(lemma)
        canon = canonical(head)
        if canon == EIMI:
            continue
        idents.add((canon, int(d.group(1)) if d else None, VOCAB[pos]))

# Kaikki: one list of (pos class) per canonical headword
kaikki = collections.defaultdict(list)
with open(KAIKKI, encoding='utf-8') as f:
    for line in f:
        d = json.loads(line)
        if d.get('lang_code') == 'grc':
            kaikki[canonical(d['word'])].append(KAIKKI_CLASS.get(d.get('pos')))

# LSJ / ML: headword key only. Sparse, locally scoped POS tags are not mapped here.
def perseus_heads(paths):
    heads = collections.Counter()
    for path in paths:
        s = open(path, encoding='utf-8').read()
        for m in re.finditer(r'<(entryFree|entry)([^>]*)>', s):
            attrs = m.group(2)
            if 'type="main"' not in attrs:
                continue
            k = re.search(r'key="([^"]*)"', attrs).group(1)
            g, _, unk = beta_to_greek(k)
            if not unk:
                heads[canonical(g)] += 1
    return heads

lsj = perseus_heads(sorted(glob.glob(os.path.join(LSJ_DIR, 'grc.lsj.perseus-eng*.xml'))))
ml = perseus_heads([ML_XML])

def tier_kaikki(canon, cls):
    rows = kaikki.get(canon, [])
    if not rows:
        return 'no_entry'
    pos_ok = [r for r in rows if r == cls]
    if not pos_ok:
        return 'entry_but_no_pos_match'
    return 'unique_pos_match' if len(pos_ok) == 1 else 'multi_pos_match_homograph_ambiguous'

def tier_perseus(counter, canon):
    n = counter.get(canon, 0)
    if n == 0:
        return 'no_entry'
    return 'unique_headword_entry' if n == 1 else 'multi_headword_entry_homograph_ambiguous'

res = {'identities': len(idents), 'kaikki_pos_tiers': collections.Counter(),
       'lsj_headword_tiers': collections.Counter(), 'ml_headword_tiers': collections.Counter()}
for canon, disc, cls in idents:
    res['kaikki_pos_tiers'][tier_kaikki(canon, cls)] += 1
    res['lsj_headword_tiers'][tier_perseus(lsj, canon)] += 1
    res['ml_headword_tiers'][tier_perseus(ml, canon)] += 1
for k in ('kaikki_pos_tiers', 'lsj_headword_tiers', 'ml_headword_tiers'):
    res[k] = dict(sorted(res[k].items()))

with open(OUT, 'w', encoding='utf-8') as f:
    json.dump(res, f, ensure_ascii=False, indent=1)
print(json.dumps(res, ensure_ascii=False, indent=1))
