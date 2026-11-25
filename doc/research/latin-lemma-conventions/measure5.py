"""Archaic-orthography effect: match rates and rule-rescue for Plautus, Terence, Lucretius, Varro RR (the Cato texts are not in the pool).
usage: LEXICA=... measure5.py <conllu-dir> <outdir>"""
import sys, os, re, json, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
strip = lambda s: common.nfd_strip(s).lower()
fold = lambda s: strip(s).replace('v', 'u').replace('j', 'i')
HW = {fold(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
M = common.manifest()
allw = S['prose_all'] + S['verse_all']
groups = {'plautus': ('phi0119',), 'terence': ('phi0134',), 'lucretius': ('phi0550',), 'varro_rr': ('phi0684',)}
RULES = [('os->us', lambda l: re.sub(r'os$', 'us', l)), ('om->um', lambda l: re.sub(r'om$', 'um', l)),
         ('uos/uom->us/um (after u/v)', lambda l: re.sub(r'uo([sm])$', r'u\1', l)), ('-umus->-imus', lambda l: l.replace('umus', 'imus')),
         ('medial u->i before b/m/p (lubet/libet, maxumus)', lambda l: re.sub(r'u([bmp])', r'i\1', l, count=1)), ('ei->i', lambda l: l.replace('ei', 'i')),
         ('quom/quum->cum', lambda l: re.sub(r'^qu[ou]m$', 'cum', l)), ('lemma ends -st/-t contraction (est/ut clitic: necessest)', lambda l: re.sub(r'(e)?st$', '', l)),
         ('x->cs / gs', lambda l: l.replace('x', 'cs')), ('uo->vo already folded; uu->u', lambda l: l.replace('uu', 'u')),
         ('ss->s / s->ss', lambda l: l.replace('ss', 's')), ('drop final -e (apocope: ante/ant, nunc)', lambda l: l[:-1] if l.endswith('e') else l)]
out = {}
for name, pref in groups.items():
    urns = [u for u in allw if u.startswith(pref)]
    toks = C()
    for u in urns:
        for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
            if p != 'u': toks[l] += 1
    n = sum(toks.values())
    um = {l: c for l, c in toks.items() if fold(l) not in HW}
    exact_diac = {l: c for l, c in toks.items() if common.nfd_strip(l) != l and fold(l) in HW and l.lower() not in {x.lower() for x in HW}}
    acc = {l: c for l, c in toks.items() if any(ord(ch) > 127 for ch in l)}
    res = {}
    for rn, fn in RULES:
        t = k = 0
        for l, c in um.items():
            if not l[:1].isupper() and l.isascii():
                x = fn(fold(l))
                if x != fold(l) and x in HW: t += 1; k += c
        res[rn] = dict(types=t, tokens=k)
    union_t = union_k = 0
    for l, c in um.items():
        if not l[:1].isupper() and l.isascii() and any(fn(fold(l)) != fold(l) and fn(fold(l)) in HW for rn, fn in RULES): union_t += 1; union_k += c
    out[name] = dict(works=len(urns), tokens=n, lemma_types=len(toks),
        lemma_types_with_non_ascii=len(acc), lemma_tokens_with_non_ascii=sum(acc.values()),
        unmatched_tokens_after_fold=sum(um.values()), unmatched_token_share=round(sum(um.values()) / n, 4),
        rescue_by_rule=res, rescue_union=dict(types=union_t, tokens=union_k), rescue_union_share_of_unmatched=round(union_k / max(1, sum(um.values())), 4))
json.dump(out, open(f'{outdir}/archaic.json', 'w'), indent=1)
for k, v in out.items(): print(k, v['works'], v['tokens'], v['lemma_tokens_with_non_ascii'], v['unmatched_token_share'], v['rescue_union'], {a: b['tokens'] for a, b in v['rescue_by_rule'].items() if b['tokens'] > 100})
