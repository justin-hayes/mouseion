"""Seeded token samples from prose80 for hand-judging: unmatched and matched.
usage: LEXICA=... sample.py <conllu-dir> <seed> <n_unmatched> <n_matched> <out.tsv>"""
import sys, os, random, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
cdir, seed, nu, nm, out = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), int(sys.argv[4]), sys.argv[5]
S = common.work_sets(); D = dicts.load()
nuv = lambda s: common.nfd_strip(s).lower().replace('j', 'i').replace('v', 'u')
IDX = {nuv(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
um, ma = [], []
for u in S['prose80']:
    for n, (f, l, p, ft, d, si) in enumerate(common.read_conllu(f'{cdir}/{u}.conllu')):
        if p == 'u': continue
        (ma if nuv(l) in IDX else um).append((u, n, f, l, p))
rnd = random.Random(seed)
rows = [('unmatched',) + t for t in rnd.sample(um, nu)] + [('matched',) + t for t in rnd.sample(ma, nm)]
open(out, 'w').write('set\tworkurn\ttokidx\tform\tlemma\tpos\n' + '\n'.join('\t'.join(map(str, r)) for r in rows) + '\n')
print(len(um), len(ma))
