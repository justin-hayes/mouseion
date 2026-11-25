"""Refinements: homograph ambiguity by dictionary base key, deponent rescue, fold-only merges, Elementary conventions, capitalised-lemma collisions, fold gains with examples.
usage: LEXICA=... measure4.py <conllu-dir> <outdir>"""
import sys, os, re, json, collections, math, unicodedata
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
strip = lambda s: common.nfd_strip(s).lower()
fold = lambda s: strip(s).replace('v', 'u').replace('j', 'i')
toks = C(); pos = collections.defaultdict(C)
for u in S['prose80']:
    for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
        if p == 'u': continue
        toks[l] += 1; pos[l][p] += 1
out = {}
# (a) homograph ambiguity by *base key* (digit-homograph groups), exact raw key, and by folded key
for dname in ('ls', 'el'):
    byraw = collections.defaultdict(set); byfold = collections.defaultdict(set)
    for k, b, o, t, i in D[dname]:
        byraw[b].add(i); byfold[fold(b)].add(i)
        for x in o: byfold[fold(x)].add(i)
    m = {l: c for l, c in toks.items() if fold(l) in byfold}
    amb_fold = {l: c for l, c in m.items() if len(byfold[fold(l)]) > 1}
    exact = {l: c for l, c in toks.items() if l in byraw}
    amb_exact = {l: c for l, c in exact.items() if len(byraw[l]) > 1}
    out[f'homographs_{dname}'] = dict(matched_tokens_folded=sum(m.values()), ambiguous_tokens_folded=sum(amb_fold.values()), ambiguous_types_folded=len(amb_fold), matched_types_folded=len(m),
        share_tokens=round(sum(amb_fold.values()) / sum(m.values()), 4), share_types=round(len(amb_fold) / len(m), 4),
        exact_key_ambiguous_tokens=sum(amb_exact.values()), exact_key_matched_tokens=sum(exact.values()),
        exact_key_share=round(sum(amb_exact.values()) / sum(exact.values()), 4),
        entries_per_ambiguous_type_hist=dict(C(min(len(byfold[fold(l)]), 6) for l in amb_fold)))
# (b) deponents: lemma ends -o but a -or / -ior deponent headword exists, among unmatched
HW = {fold(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
um = {l: c for l, c in toks.items() if fold(l) not in HW and l.isascii()}
dep = {}
for l, c in um.items():
    if pos[l].get('v', 0) * 2 >= c and l.endswith('o'):
        for suf in ('r', 'ior'):
            cand = fold(l[:-1] + suf) if suf == 'ior' else fold(l + 'r')
            if cand in HW: dep[l] = (c, cand); break
out['deponent_o_to_or_rescue'] = dict(types=len(dep), tokens=sum(c for c, _ in dep.values()),
    top=[(l, dep[l][0], dep[l][1]) for l in sorted(dep, key=lambda x: -dep[x][0])[:15]])
# verb lemma shapes, matched shares
vt = {l: c for l, c in toks.items() if pos[l].get('v', 0) * 2 >= c}
def share(sel): 
    t = sum(c for l, c in sel.items()); m = sum(c for l, c in sel.items() if fold(l) in HW)
    return dict(types=len(sel), tokens=t, matched_types=sum(1 for l in sel if fold(l) in HW), matched_tokens=m, token_rate=round(m / t, 4) if t else None)
out['verb_lemma_shapes'] = {
    'ends_-o': share({l: c for l, c in vt.items() if l.endswith('o')}),
    'ends_-or': share({l: c for l, c in vt.items() if l.endswith('or')}),
    'ends_-m (sum, inquam)': share({l: c for l, c in vt.items() if l.endswith('m')}),
    'ends_-re_or_-ri (infinitive-shaped)': share({l: c for l, c in vt.items() if re.search(r'(re|ri)$', l)}),
    'other_endings': share({l: c for l, c in vt.items() if not re.search(r'(o|or|m|re|ri)$', l)}),
    'all_verb_lemmas': share(vt)}
# (c) dictionary-side merges created by u/v and i/j beyond case
raw = {e[1] for e in D['ls']}
g = collections.defaultdict(set); g2 = collections.defaultdict(set)
for k in raw: g[k.lower()].add(k); g2[fold(k)].add(k)
extra = [sorted(v) for kk, v in g2.items() if len({x.lower() for x in v}) > 1]
out['ls_uvij_merges_beyond_case'] = dict(groups=len(extra), all=['/'.join(v) for v in extra])
rawel = {e[1] for e in D['el']}
g2e = collections.defaultdict(set)
for k in rawel: g2e[fold(k)].add(k)
extra_el = [sorted(v) for kk, v in g2e.items() if len({x.lower() for x in v}) > 1]
out['el_uvij_merges_beyond_case'] = dict(groups=len(extra_el), all=['/'.join(v) for v in extra_el][:30])
# (d) conventions: j / v / u / i use in each dictionary's base keys and in corpus lemmas
def conv(keys):
    return dict(keys=len(keys), with_j=sum('j' in k.lower() for k in keys), with_v=sum('v' in k.lower() for k in keys))
out['key_conventions'] = dict(ls=conv(raw), elementary=conv(rawel), corpus_lemma_types=dict(types=len(toks), with_j=sum('j' in l.lower() for l in toks), with_v=sum('v' in l.lower() for l in toks)),
    corpus_lemma_tokens=dict(with_j=sum(c for l, c in toks.items() if 'j' in l.lower()), with_v=sum(c for l, c in toks.items() if 'v' in l.lower())))
# corpus lemma types that match ONLY after a fold, by fold
def gain(fn_list):
    raw_ls = {x for k, b, o, t, i in D['ls'] for x in [b] + o}
    raw_el = {x for k, b, o, t, i in D['el'] for x in [b] + o}
    both = raw_ls | raw_el
    res = {}
    prev = {x for x in both}
    ladder = [('exact', lambda s: s), ('NFC+lowercase', lambda s: unicodedata.normalize('NFC', s).lower()), ('+strip-diacritics', strip), ('+i/j', lambda s: strip(s).replace('j', 'i')),
              ('+u/v', lambda s: strip(s).replace('v', 'u')), ('+u/v+i/j', fold)]
    for name, fn in ladder:
        idx = {fn(x) for x in both}
        mt = sum(c for l, c in toks.items() if fn(l) in idx); mk = sum(1 for l in toks if fn(l) in idx)
        res[name] = dict(tokens=mt, types=mk)
    return res
out['ladder_union_prose80'] = gain(None)
# which lemma types are rescued by u/v+i/j only (vs exact+case+strip)
base = lambda s: strip(s)
HWb = {base(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
resc = {l: c for l, c in toks.items() if base(l) not in HWb and fold(l) in HW}
out['rescued_only_by_uv_ij'] = dict(types=len(resc), tokens=sum(resc.values()), top=[(l, c) for l, c in sorted(resc.items(), key=lambda x: -x[1])[:25]])
# (e) capitalised-lemma collisions
cap = {l: c for l, c in toks.items() if l[:1].isupper() and l.isascii()}
col = {l: c for l, c in cap.items() if l.lower() in toks and l.lower() != l}
out['capitalised_lemmas'] = dict(types=len(cap), tokens=sum(cap.values()), also_present_lowercase_types=len(col), also_present_lowercase_tokens=sum(col.values()),
    lowercase_is_dictionary_headword_types=sum(1 for l in col if fold(l) in HW),
    top=[(l, c, toks[l.lower()]) for l, c in sorted(col.items(), key=lambda x: -x[1])[:15]])
allcaps = {l: c for l, c in toks.items() if len(l) > 1 and l.isupper() and l.isascii()}
out['allcaps_lemmas'] = dict(types=len(allcaps), tokens=sum(allcaps.values()), top=[(l, c) for l, c in sorted(allcaps.items(), key=lambda x: -x[1])[:12]])
# L&S capitalised keys vs proper-noun matching
lscap = {e[1] for e in D['ls'] if e[1][:1].isupper()}
out['ls_capitalised_base_keys'] = len(lscap)
# Wilson intervals
def wilson(k, n, z=1.96):
    p = k / n; d = 1 + z * z / n; c = p + z * z / (2 * n); a = z * math.sqrt(p * (1 - p) / n + z * z / (4 * n * n))
    return round((c - a) / d, 3), round((c + a) / d, 3)
out['sample_wilson'] = {k: wilson(*v) for k, v in dict(unmatched_lemmatiser_error_83_of_120=(83, 120), unmatched_convention_variant_9_of_120=(9, 120), unmatched_nonlexical_19_of_120=(19, 120), unmatched_dictionary_gap_5_of_120=(5, 120), unmatched_deponent_o_4_of_120=(4, 120), matched_wrong_1_of_100=(1, 100)).items()}
json.dump(out, open(f'{outdir}/features4.json', 'w'), indent=1, ensure_ascii=False)
print(json.dumps(out, indent=1, ensure_ascii=False)[:9000])
