"""UD_Latin-CIRCSE r2.19 profile, and its overlap with OLA v0.2.0 for the same
Works. usage: measure_circse.py <circse-dir> <ola-conllu-dir> <out.json> (counts only)"""
import sys, re, json, collections, difflib, unicodedata
cd, od, out = sys.argv[1:4]
WORKS = {  # CIRCSE sent_id stem -> OLA URN (urn_cts.xml titles)
    'Latin_SenecaYounger_HercF': 'phi1017.phi001.perseus-lat2',
    'Latin_SenecaYounger_Ag': 'phi1017.phi007.perseus-lat2',
    'Latin_SenecaYounger_Oedip': 'phi1017.phi006.perseus-lat2',
    'SenPhoen-P-17': 'phi1017.phi003.perseus-lat2',
    'TacGerma-Q-01': 'phi1351.phi002.perseus-lat1'}
def fold(s):
    s = unicodedata.normalize('NFD', s.lower())
    return ''.join(c for c in s if not unicodedata.combining(c)).replace('j', 'i').replace('v', 'u')
sent = []  # (sent_id, comments, rows)
for split in ('train', 'dev', 'test'):
    cur = {'c': [], 'r': []}
    for line in open(f'{cd}/la_circse-ud-{split}.conllu', encoding='utf-8'):
        line = line.rstrip('\n')
        if not line:
            if cur['r']: sent.append((split, cur['c'], cur['r']))
            cur = {'c': [], 'r': []}; continue
        (cur['c'] if line.startswith('#') else cur['r']).append(line if line.startswith('#') else line.split('\t'))
ids = collections.Counter(); res = {}
agg = collections.defaultdict(collections.Counter)
stems = {}
for split, com, rows in sent:
    sid = next(c.split(' = ', 1)[1] for c in com if c.startswith('# sent_id'))
    stem = re.sub(r'(_poetry|_prose)?-\d+(-bis)?$', '', sid)
    kind = 'poetry' if '_poetry-' in sid else 'prose'
    a = agg[stem]; a['sentences'] += 1
    a['comment_keys:' + ','.join(sorted({c.split(' ')[1] for c in com}))] += 1
    mwt = [r for r in rows if '-' in r[0]]; empty = [r for r in rows if '.' in r[0]]
    words = [r for r in rows if r[0].isdigit()]
    a['tokens'] += len(words); a['mwt_rows'] += len(mwt); a['empty_nodes'] += len(empty)
    a['punct'] += sum(1 for r in words if r[3] == 'PUNCT')
    a['enhanced_deps_filled'] += sum(1 for r in words if r[8] != '_')
    a['xpos_' + 'filled'] += sum(1 for r in words if r[4] != '_')
    for r in words:
        a['rel:' + r[7]] += 1; a['upos:' + r[3]] += 1
        a['rel_base:' + r[7].split(':')[0]] += 1
        if r[2][:1].isupper(): a['lemma_upper'] += 1
    a['roots_ne_1'] += sum(1 for _ in [0] if sum(1 for r in words if r[6] == '0') != 1)
    stems.setdefault(stem, []).append((sid, split, words))
    a['kind_' + kind] += 1
    a['split_' + split] += 1
res['works'] = {k: {kk: vv for kk, vv in v.items() if not kk.startswith(('rel:', 'upos:', 'comment_keys'))} for k, v in agg.items()}
res['rel_inventory'] = dict(sum((collections.Counter({kk[4:]: vv for kk, vv in v.items() if kk.startswith('rel:')}) for v in agg.values()), collections.Counter()))
res['upos_inventory'] = dict(sum((collections.Counter({kk[5:]: vv for kk, vv in v.items() if kk.startswith('upos:')}) for v in agg.values()), collections.Counter()))
res['comment_keys'] = {k: dict((kk, vv) for kk, vv in v.items() if kk.startswith('comment_keys')) for k, v in agg.items()}
# overlap with OLA: concatenate CIRCSE sentences in sent_id number order, compare forms with OLA's non-punct tokens in order.
def ola_tokens(urn):
    toks = []
    for line in open(f'{od}/sentence-seg.ola.{urn}.tok01_sentence-seg01.predicted.conllu', encoding='utf-8'):
        c = line.rstrip('\n').split('\t')
        if len(c) == 10 and c[3] != 'u': toks.append((fold(c[1]), fold(c[2]), c[3], c[7]))
    return toks
res['overlap'] = {}
for stem, urn in WORKS.items():
    sents = sorted(stems[stem], key=lambda s: (int(re.search(r'-(\d+)(-bis)?$', s[0]).group(1)), s[0].endswith('-bis')))
    cw = [(fold(r[1]), fold(r[2]), r[3]) for _, _, ws in sents for r in ws if r[3] != 'PUNCT']
    ot = ola_tokens(urn)
    sm = difflib.SequenceMatcher(None, [t[0] for t in cw], [t[0] for t in ot], autojunk=False)
    matched = sum(b.size for b in sm.get_matching_blocks())
    # lemma agreement on aligned pairs
    agree = n = 0
    for b in sm.get_matching_blocks():
        for k in range(b.size):
            n += 1; agree += cw[b.a + k][1] == ot[b.b + k][1]
    res['overlap'][stem] = dict(ola_urn=urn, circse_tokens=len(cw), ola_nonpunct_tokens=len(ot), aligned_forms=matched,
                                aligned_pct_of_circse=round(100 * matched / len(cw), 1), lemma_agree_on_aligned=round(100 * agree / max(n, 1), 1))
json.dump(res, open(out, 'w'), indent=1, sort_keys=True)
for k, v in res['works'].items(): print(k, {kk: vv for kk, vv in v.items() if not kk.startswith(('rel_base', 'lemma'))})
print(json.dumps(res['overlap'], indent=1)); print(res['upos_inventory']); print(sorted(res['rel_inventory'].items(), key=lambda x: -x[1])[:60])
print(res['comment_keys'])
