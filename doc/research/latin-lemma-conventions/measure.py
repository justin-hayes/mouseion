"""Lemma conventions of the pinned OLA v0.2.0 CoNLL-U against L&S / Elementary headwords.
usage: LEXICA=<perseus lexica checkout @56061ca1> measure.py <conllu-dir> <outdir>
Writes aggregate JSON/TSV only (lemma strings and counts, no running text)."""
import sys, os, re, json, collections, unicodedata
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
os.makedirs(outdir, exist_ok=True)
SETS = common.work_sets()
_all = SETS['prose_all'] + SETS['verse_all'] + SETS['borderline']
for _n, _p in [('plautus', 'phi0119'), ('terence', 'phi0134'), ('lucretius', 'phi0550'), ('varro_rr', 'phi0684'), ('caesar', 'phi0448'), ('cicero', 'phi0474'), ('livy', 'phi0914'), ('virgil', 'phi0690'), ('apuleius', 'phi1212'), ('gellius', 'phi1254')]:
    SETS['author:' + _n] = [u for u in _all if u.startswith(_p)]
D = dicts.load()

# ---------- normalisations (applied to both sides) ----------
import unicodedata
def n_exact(s): return s
def n_case(s): return unicodedata.normalize('NFC', s).lower()
def n_strip(s): return common.nfd_strip(s).lower()
def n_ij(s): return n_strip(s).replace('j', 'i')
def n_uv(s): return n_strip(s).replace('v', 'u')
def n_uvij(s): return n_strip(s).replace('j', 'i').replace('v', 'u')
def n_uvij_diph(s): return n_uvij(s).replace('ae', 'e').replace('oe', 'e')
NORMS = [('exact', n_exact), ('NFC+lowercase', n_case), ('+strip-diacritics', n_strip), ('+i/j', n_ij), ('+u/v', n_uv),
         ('+u/v+i/j', n_uvij), ('+u/v+i/j+ae/oe', n_uvij_diph)]

def headword_sets(which, use_orth):
    """raw headword strings (digit-stripped key; optionally all single-word orth forms)."""
    s = set()
    for k, base, orths, typ, i in D[which]:
        s.add(base)
        if use_orth: s.update(orths)
    return s
HW = {
    'ls_key': headword_sets('ls', False),
    'ls_key+orth': headword_sets('ls', True),
    'el_key+orth': headword_sets('el', True),
}
HW['ls+el_key+orth'] = HW['ls_key+orth'] | HW['el_key+orth']
def normset(raw, fn): return {fn(x) for x in raw}
IDX = {(h, n): normset(raw, fn) for h, raw in HW.items() for n, fn in NORMS}

# ---------- read the corpus ----------
def load_set(urns):
    toks = C()        # lemma -> tokens (non-punct)
    works = collections.defaultdict(set)
    pos = collections.defaultdict(C)
    forms = collections.defaultdict(C)
    feats = collections.defaultdict(C)
    punct = 0
    perwork = {}
    for u in urns:
        wc = C()
        for form, lem, p, ft, dep, si in common.read_conllu(f'{cdir}/{u}.conllu'):
            if p == 'u': punct += 1; continue
            toks[lem] += 1; works[lem].add(u); pos[lem][p] += 1; wc[lem] += 1
            forms[lem][form] += 1
            if 'Voice=d' in ft: feats[lem]['dep'] += 1
        perwork[u] = wc
    return toks, works, pos, forms, feats, perwork

def rate(toks, idx, fn):
    mt = mk = 0; nt = sum(toks.values()); nk = len(toks)
    for l, c in toks.items():
        if fn(l) in idx: mt += c; mk += 1
    return mk, nk, mt, nt

res = {}
cache = {}
for sname, urns in SETS.items():
    cache[sname] = load_set(urns)
    toks, works, pos, forms, feats, perwork = cache[sname]
    r = {'works': len(urns), 'tokens': sum(toks.values()), 'types': len(toks), 'match': {}}
    for h in HW:
        for n, fn in NORMS:
            mk, nk, mt, nt = rate(toks, IDX[(h, n)], fn)
            r['match'][f'{h}|{n}'] = dict(types_matched=mk, types=nk, tokens_matched=mt, tokens=nt,
                                          type_rate=round(mk / nk, 4), token_rate=round(mt / nt, 4))
    res[sname] = r
json.dump(res, open(f'{outdir}/match_rates.json', 'w'), indent=1)

# ---------- per-Work spread under the headline normalisation ----------
H, N, FN = 'ls+el_key+orth', '+u/v+i/j', n_uvij
pw = {}
for u in SETS['prose80']:
    wc = cache['prose80'][5][u]
    mk, nk, mt, nt = rate(wc, IDX[(H, N)], FN)
    pw[u] = round(mt / nt, 4)
vals = sorted(pw.values())
res_pw = dict(min=vals[0], p10=vals[len(vals)//10], median=vals[len(vals)//2], max=vals[-1],
              lowest=sorted(pw.items(), key=lambda x: x[1])[:8], highest=sorted(pw.items(), key=lambda x: -x[1])[:3])
json.dump(res_pw, open(f'{outdir}/per_work_prose80.json', 'w'), indent=1)
json.dump(pw, open(f'{outdir}/per_work_prose80_all.json', 'w'), indent=0, sort_keys=True)
print('per-work', json.dumps(res_pw))
for s in res: print(s, res[s]['works'], res[s]['tokens'], res[s]['types'])
for k, v in res['prose80']['match'].items(): print(k, v['type_rate'], v['token_rate'])
