"""Spot checks cited in the findings: Roman-numeral/period lemmas matched, digit-suffixed lemma types, unsplit -ne cases, entries behind `in`.
usage: LEXICA=... measure8.py <conllu-dir> <outdir>"""
import sys, os, re, json, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
fold = lambda s: common.nfd_strip(s).lower().replace('v', 'u').replace('j', 'i')
HW = {fold(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
toks = C(); ne = C(); dsuf = C()
for u in S['prose80']:
    for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
        if p == 'u': continue
        toks[l] += 1
        fl = f.lower()
        if fl.endswith('ne') and len(fl) > 3 and l.lower() == fl[:-2] and not l.endswith('io'): ne[(fl, l)] += 1
        if re.search(r'[A-Za-z]\d$', l): dsuf[l] += 1
ROMAN = re.compile(r'^[IVXLCDM]+$')
def share(sel):
    t = sum(sel.values()); m = sum(c for l, c in sel.items() if fold(l) in HW)
    return dict(types=len(sel), tokens=t, matched_types=sum(1 for l in sel if fold(l) in HW), matched_tokens=m)
out = dict(roman_lemmas=share({l: c for l, c in toks.items() if ROMAN.match(l)}),
           lemmas_with_period=share({l: c for l, c in toks.items() if '.' in l}),
           lemmas_with_digit=share({l: c for l, c in toks.items() if re.search(r'\d', l)}),
           greek_script=share({l: c for l, c in toks.items() if re.search(r'[Ͱ-Ͽἀ-῿]', l)}),
           digit_suffixed_lemma_types=dict(dsuf.most_common(30)),
           unsplit_ne_not_io_nouns=dict(tokens=sum(ne.values()), types=len(ne), top=[(f"{a}->{b}", c) for (a, b), c in ne.most_common(12)]),
           ls_entries_for_key_in=sorted(e[0] for e in D['ls'] if fold(e[1]) == 'in'))
json.dump(out, open(f'{outdir}/spot_checks.json', 'w'), indent=1, ensure_ascii=False)
print(json.dumps(out, indent=1, ensure_ascii=False))
