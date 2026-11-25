"""Hit counts per OLA Work with the suffix-stripping reader versus exact-label matching; prevalence. Aggregates only."""
import sys, os, glob, json, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import lib
D = sys.argv[1]; out = {}
def exact_hits(tr):
    r = []
    for k, t in enumerate(tr.t):
        if t.pos != 'v' or t.mood != 'p' or t.case != 'b' or t.rel != 'ADV': continue
        subs = [c for c in tr.ch[k] if tr.t[c].rel == 'SBJ' and tr.t[c].case == 'b']
        if subs: r.append(k)
    return r
tot = collections.Counter()
for f in sorted(glob.glob(f'{D}/ola/*.conllu')):
    w = os.path.basename(f)[:-7]
    if w.startswith('phi0620') or w.startswith('phi0690') or w.startswith('phi0959') or w.startswith('phi0975') or w.startswith('phi1351.phi005.lasciva'): continue
    S = lib.read_ola(f); n = 0; h = 0; e = 0; viaco = 0
    for s in S:
        tr = lib.Tree(s, 'agdt'); n += sum(1 for t in s if t.pos != 'u')
        hh = lib.agdt_hits(tr); h += len(hh); e += len(exact_hits(tr))
        for k in hh:
            if '_' in s[k].rel or any('_' in s[c].rel for c in lib.sbj_agdt(tr, k)): viaco += 1
    out[w] = dict(words=n, hits=h, exact_only_hits=e, hits_needing_suffix_reader=viaco)
    for k, v in out[w].items(): tot[k] += v
out['TOTAL'] = dict(tot)
json.dump(out, open(sys.argv[2], 'w'), indent=1, sort_keys=True)
for k, v in out.items(): print(k, v)
