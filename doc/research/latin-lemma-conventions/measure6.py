"""Head-of-vocabulary match rates and hapax share of the unmatched types (prose80). usage: LEXICA=... measure6.py <conllu-dir> <outdir>"""
import sys, os, json, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
fold = lambda s: common.nfd_strip(s).lower().replace('v', 'u').replace('j', 'i')
HW = {fold(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
HWLS = {fold(x) for k, b, o, t, i in D['ls'] for x in [b] + o}
toks = C(); works = collections.defaultdict(set)
for u in S['prose80']:
    for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
        if p != 'u': toks[l] += 1; works[l].add(u)
rank = sorted(toks, key=lambda l: -toks[l])
out = {}
for n in (100, 500, 1000, 2000, 5000, 10000, 20000):
    top = rank[:n]
    out[f'top{n}'] = dict(types_matched=sum(fold(l) in HW for l in top), rate=round(sum(fold(l) in HW for l in top) / n, 4),
                          tokens=sum(toks[l] for l in top), token_rate=round(sum(toks[l] for l in top if fold(l) in HW) / sum(toks[l] for l in top), 4),
                          min_count=toks[top[-1]], ls_only_types_matched=sum(fold(l) in HWLS for l in top))
um = [l for l in toks if fold(l) not in HW]
out['unmatched_types'] = dict(n=len(um), hapax=sum(toks[l] == 1 for l in um), in_one_work=sum(len(works[l]) == 1 for l in um), freq_ge_10=sum(toks[l] >= 10 for l in um), freq_ge_50=sum(toks[l] >= 50 for l in um), freq_ge_100=sum(toks[l] >= 100 for l in um))
out['all_types'] = dict(n=len(toks), hapax=sum(c == 1 for c in toks.values()), freq_ge_10=sum(c >= 10 for c in toks.values()))
# token coverage: share of tokens covered by matched types with freq>=10 etc
tot = sum(toks.values())
out['unmatched_tokens_in_types_freq_ge_10'] = sum(toks[l] for l in um if toks[l] >= 10)
out['unmatched_tokens_total'] = sum(toks[l] for l in um)
# ls-only token rate (pinned candidate)
out['ls_only_token_rate_folded'] = round(sum(c for l, c in toks.items() if fold(l) in HWLS) / tot, 4)
out['union_token_rate_folded'] = round(sum(c for l, c in toks.items() if fold(l) in HW) / tot, 4)
json.dump(out, open(f'{outdir}/head_vocab.json', 'w'), indent=1)
print(json.dumps(out, indent=1))
