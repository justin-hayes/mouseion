"""Per-Work incidence of the u/v, i/j spelling split: tokens matched only after the fold, and Works whose lemmas use u for consonantal v.
usage: LEXICA=... measure7.py <conllu-dir> <outdir>"""
import sys, os, json, collections, statistics
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
base = lambda s: common.nfd_strip(s).lower()
fold = lambda s: base(s).replace('v', 'u').replace('j', 'i')
HWb = {base(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
HWf = {fold(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
res = {}
for u in S['prose80']:
    n = r = 0; lv = lu = 0
    for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
        if p == 'u': continue
        n += 1
        if base(l) not in HWb and fold(l) in HWf: r += 1
        if l in ('voco', 'video', 'venio', 'vir', 'servus', 'vis', 'vivo', 'novus'): lv += 1
        if l in ('uoco', 'uideo', 'uenio', 'uir', 'seruus', 'uis', 'uiuo', 'nouus'): lu += 1
    res[u] = dict(tokens=n, rescued=r, share=round(r / n, 4), probe_v_spelling=lv, probe_u_spelling=lu)
sh = sorted(v['share'] for v in res.values())
out = dict(works=len(res), median_share=statistics.median(sh), max_share=sh[-1], works_gt_1pct=sum(s > 0.01 for s in sh), works_gt_3pct=sum(s > 0.03 for s in sh),
           works_u_style=[w for w, v in res.items() if v['probe_u_spelling'] > v['probe_v_spelling']], works_v_style=sum(v['probe_v_spelling'] > v['probe_u_spelling'] for v in res.values()),
           works_mixed=[w for w, v in res.items() if v['probe_u_spelling'] and v['probe_v_spelling']],
           top=[(w, v['share'], v['rescued']) for w, v in sorted(res.items(), key=lambda x: -x[1]['share'])[:10]])
json.dump(dict(summary=out, per_work=res), open(f'{outdir}/uv_ij_per_work.json', 'w'), indent=0)
print(json.dumps(out, indent=1))
