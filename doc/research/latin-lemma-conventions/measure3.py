"""Homograph ambiguity, normalisation merges/splits, assimilation rescue, est/cum, Greek script. Aggregates and lemma strings only.
usage: LEXICA=... measure3.py <conllu-dir> <outdir>"""
import sys, os, re, json, collections, unicodedata
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import common, dicts
C = collections.Counter
cdir, outdir = sys.argv[1:3]
S = common.work_sets(); D = dicts.load()
strip = lambda s: common.nfd_strip(s).lower()
STEPS = [('casefold', lambda s: unicodedata.normalize('NFC', s).lower()),
         ('strip-diacritics', strip),
         ('u/v', lambda s: strip(s).replace('v', 'u')),
         ('i/j', lambda s: strip(s).replace('j', 'i')),
         ('u/v+i/j', lambda s: strip(s).replace('v', 'u').replace('j', 'i')),
         ('u/v+i/j+ae/oe', lambda s: strip(s).replace('v', 'u').replace('j', 'i').replace('ae', 'e').replace('oe', 'e'))]
nuv = STEPS[4][1]
out = {}
toks = C(); pos = collections.defaultdict(C); formlem = C()
for u in S['prose80']:
    for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'):
        if p == 'u': continue
        toks[l] += 1; pos[l][p] += 1
        if f.lower() in ('est', 'es', 'sunt', 'edit', 'ede', 'esse'): formlem[(f.lower(), l)] += 1
# ---- dictionary-side merges (distinct raw base keys -> one normalised key), L&S and Elementary
for dname in ('ls', 'el'):
    raw = {e[1] for e in D[dname]}     # digit-stripped base keys, raw case
    res = {}
    for name, fn in STEPS:
        g = collections.defaultdict(set)
        for k in raw: g[fn(k)].add(k)
        m = {k: v for k, v in g.items() if len(v) > 1}
        # only count groups that the step itself creates beyond casefold-less exact (all are distinct raw strings)
        res[name] = dict(distinct_raw=len(raw), normalised=len(g), groups_merged=len(m), keys_absorbed=sum(len(v) - 1 for v in m.values()),
                         examples=sorted(['/'.join(sorted(v)) for v in m.values()], key=lambda s: (len(s), s))[:12])
    out[f'dict_{dname}_merges'] = res
# entries per base key (homograph groups) ------------------------------------------------
hom = collections.defaultdict(list)
for e in D['ls']: hom[strip(e[1]).replace('v', 'u').replace('j', 'i')].append(e)
out['ls_homographs'] = dict(entries=len(D['ls']), base_keys=len({e[1] for e in D['ls']}),
    entries_with_digit_key=sum(bool(re.search(r'\d$', e[0])) for e in D['ls']),
    homograph_groups_exact=len({e[1] for e in D['ls'] if re.search(r'\d$', e[0])}),
    normalised_keys_with_gt1_entry=sum(len(v) > 1 for v in hom.values()),
    max_entries_per_key=max(len(v) for v in hom.values()))
# ---- ambiguity of matched lemmas
amb = C(); amb_tok = C(); ex = []
tot = sum(toks.values()); mtok = 0
for l, c in toks.items():
    k = nuv(l)
    n_ls = len(hom.get(k, []))
    if n_ls:
        mtok += c
        amb['types_ls_matched'] += 1; amb_tok['tokens_ls_matched'] += c
        if n_ls > 1: amb['types_ls_ambiguous'] += 1; amb_tok['tokens_ls_ambiguous'] += c; ex.append((c, l, n_ls))
ex.sort(reverse=True)
out['ambiguity_ls'] = dict(types=dict(amb), tokens=dict(amb_tok), prose80_tokens=tot,
    token_share_ambiguous_of_ls_matched=round(amb_tok['tokens_ls_ambiguous'] / amb_tok['tokens_ls_matched'], 4),
    top_ambiguous=[dict(lemma=l, tokens=c, ls_entries=n, pos=dict(pos[l])) for c, l, n in ex[:30]])
# ---- est / sum / edo and cum
out['forms_est_lemmas'] = {f'{f}|{l}': c for (f, l), c in formlem.most_common(14)}
out['lemma_cum_pos'] = dict(pos['cum']); out['lemma_ne_pos'] = dict(pos['ne']); out['lemma_sum_pos'] = dict(pos['sum']); out['lemma_edo_tokens'] = toks.get('edo', 0)
out['lemma_homograph_pairs_in_corpus'] = {l: dict(pos[l]) for l in ('cum', 'ne', 'ut', 'quam', 'qui', 'quis', 'sum', 'edo', 'fero', 'volo', 'malo', 'nolo', 'pario', 'pareo', 'liber', 'Liber', 'ab', 'a', 'ora', 'os') if l in toks}
# the dictionary's first-clause for cum1/cum2/ne1/ne2 (POS words) to see whether POS could split
def txt(e): return ''.join(e.itertext())
import xml.etree.ElementTree as ET
r = ET.parse(dicts.LS).getroot()
posw = {}
for e in r.iter('entryFree'):
    if e.get('key') in ('cum1', 'cum2', 'ne1', 'ne2', 'ne3', 'sum1', 'sum2', 'edo1', 'edo2', 'edo3', 'liber1', 'liber2', 'liber3', 'Liber1', 'Liber2', 'ut', 'uti', 'qui1', 'qui2', 'quam'):
        t = re.sub(r'\s+', ' ', txt(e))
        m = re.search(r'\b(praep|adv|conj|subst|adj|v\. a|v\. n|v\. dep|v\. freq|interj|pron|part|num)\b', t)
        posw[e.get('key')] = m.group(1) if m else None
out['ls_pos_word_for_selected_homographs'] = posw
# L&S entries carry <pos>? coverage
pe = sum(1 for e in r.iter('entryFree') if e.find('pos') is not None)
out['ls_entries_with_pos_element'] = pe
# ---- prefix assimilation rescue (lemma side), tested on lemmas absent from the headword set
HW = {nuv(x) for w in ('ls', 'el') for k, b, o, t, i in D[w] for x in [b] + o}
um = {l: c for l, c in toks.items() if nuv(l) not in HW and l.isascii() and not re.search(r'[\d.\-]', l)}
ASS = [(r'^adp', lambda m: 'app'), (r'^adc', lambda m: 'acc'), (r'^adg', lambda m: 'agg'), (r'^ads', lambda m: 'ass'), (r'^adt', lambda m: 'att'),
       (r'^adf', lambda m: 'aff'), (r'^adl', lambda m: 'all'), (r'^adr', lambda m: 'arr'), (r'^adn', lambda m: 'ann'), (r'^adq', lambda m: 'acq'),
       (r'^inp', lambda m: 'imp'), (r'^inb', lambda m: 'imb'), (r'^inm', lambda m: 'imm'), (r'^inl', lambda m: 'ill'), (r'^inr', lambda m: 'irr'),
       (r'^conp', lambda m: 'comp'), (r'^conb', lambda m: 'comb'), (r'^conm', lambda m: 'comm'), (r'^conl', lambda m: 'coll'), (r'^conr', lambda m: 'corr'),
       (r'^obc', lambda m: 'occ'), (r'^obp', lambda m: 'opp'), (r'^obf', lambda m: 'off'), (r'^obt', lambda m: 'ott'),
       (r'^subc', lambda m: 'succ'), (r'^subp', lambda m: 'supp'), (r'^subm', lambda m: 'summ'), (r'^subf', lambda m: 'suff'), (r'^subg', lambda m: 'sugg'),
       (r'^exf', lambda m: 'eff')]
RULES = {}
for pat, fn in ASS:
    name = pat.strip('^')
    RULES[name] = (re.compile(pat), fn)
first = {}
per = C(); pert = C(); tr = 0; tt = 0
resc_types = {}
for l, c in um.items():
    ln = nuv(l)
    for name, (rx, fn) in RULES.items():
        if rx.search(ln):
            new = rx.sub(fn, ln, count=1)
            if new != ln and new in HW:
                per[name] += 1; pert[name] += c
                resc_types.setdefault(l, new)
                break
out['prefix_assimilation_rescue'] = dict(types=len(resc_types), tokens=sum(um[l] for l in resc_types),
    by_rule_types=dict(per), by_rule_tokens=dict(pert), unmatched_ascii_types_tested=len(um), unmatched_ascii_tokens_tested=sum(um.values()),
    top=[(l, um[l], resc_types[l]) for l in sorted(resc_types, key=lambda x: -um[x])[:20]])
# ---- lemma-side splits: spelling variants of one lexeme under normalisation (corpus)
corp = {}
for name, fn in STEPS:
    g = collections.defaultdict(set)
    for l in toks: g[fn(l)].add(l)
    m = {k: v for k, v in g.items() if len(v) > 1}
    out_ex = sorted(m.values(), key=lambda v: -sum(toks[x] for x in v))[:12]
    corp[name] = dict(lemma_types=len(toks), normalised=len(g), groups_merged=len(m), types_absorbed=sum(len(v) - 1 for v in m.values()),
        tokens_in_merged_groups=sum(toks[x] for v in m.values() for x in v),
        examples=[{'/'.join(sorted(v)): sum(toks[x] for x in v)} for v in out_ex])
out['corpus_lemma_merges_prose80'] = corp
# Greek script and betacode-like lemmas
G = re.compile(r'[Ͱ-Ͽἀ-῿]')
out['greek'] = dict(types=sum(bool(G.search(l)) for l in toks), tokens=sum(c for l, c in toks.items() if G.search(l)),
    pos=dict(sum((C({p: n for p, n in pos[l].items()}) for l in toks if G.search(l)), C())),
    betacode_like_types=sum(bool(re.search(r'[=/\\]', l)) for l in toks), betacode_like_tokens=sum(c for l, c in toks.items() if re.search(r'[=/\\]', l)),
    works_with_greek_script=sum(1 for u in S['prose80'] if any(G.search(l) for f, l, p, ft, d, si in common.read_conllu(f'{cdir}/{u}.conllu'))))
# Greek loans in Latin script: share matching vs not (y/ph/th/ch/rh in lemma)
gl = {l: c for l, c in toks.items() if l.isascii() and re.search(r'y|ph|ch|th|rh|z|k', l.lower()) and not l[:1].isupper()}
out['greek_loan_like_latin_script_lowercase'] = dict(types=len(gl), tokens=sum(gl.values()), matched_types=sum(1 for l in gl if nuv(l) in HW), matched_tokens=sum(c for l, c in gl.items() if nuv(l) in HW))
# dictionary side Greek-derived orth
nongk = [e for e in D['ls'] if any('y' in x for x in e[2])]
out['dict_note'] = dict(ls_entries_with_y_in_orth=len(nongk))
json.dump(out, open(f'{outdir}/features3.json', 'w'), indent=1, ensure_ascii=False)
print(json.dumps(out, indent=1, ensure_ascii=False)[:14000])
