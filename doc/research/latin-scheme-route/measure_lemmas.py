"""Lemma-spelling variation in the OLA classical (BY-SA) CoNLL-U layer, to size
the normalisation a Latin profile would need (u/v, i/j, case, diacritics).
usage: measure_lemmas.py <conllu-dir> <manifest.tsv> <out.json>   (counts only)"""
import sys, json, collections, unicodedata
d, manifest, out = sys.argv[1:4]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
urns = [r[0] for r in rows if r[1] == 'core' and 'CC-BY-SA' in r[3]]
def fold(s):
    s = unicodedata.normalize('NFD', s)
    s = ''.join(c for c in s if not unicodedata.combining(c))
    return s.lower().replace('j', 'i').replace('v', 'u')
lem = collections.Counter(); posof = {}
tokens = 0; cap_noun = 0; works_with_j = collections.Counter()
for u in urns:
    for line in open(f'{d}/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu', encoding='utf-8'):
        c = line.rstrip('\n').split('\t')
        if len(c) != 10 or c[3] in ('u',): continue
        lem[c[2]] += 1; tokens += 1
        if 'j' in c[2].lower(): works_with_j[u] += 1
groups = collections.defaultdict(list)
for l, n in lem.items(): groups[fold(l)].append((l, n))
multi = {k: v for k, v in groups.items() if len(v) > 1}
tok_in_multi = sum(n for v in multi.values() for _, n in v)
only_variant = sum(sorted(n for _, n in v)[0] for v in multi.values())  # tokens on the minority spellings, loose lower bound
kinds = collections.Counter()
for k, v in multi.items():
    forms = [l for l, _ in v]
    if len({f.lower() for f in forms}) < len(forms): kinds['case_only'] += 1
    if any('j' in f.lower() for f in forms) and any('i' in f.lower() for f in forms): kinds['i_j'] += 1
    if any('v' in f.lower() for f in forms): kinds['v_present'] += 1
    if any(ord(ch) > 127 for f in forms for ch in f): kinds['non_ascii'] += 1
res = dict(works=len(urns), non_punct_tokens=tokens, lemma_types=len(lem), folded_types=len(groups),
           folded_groups_with_variants=len(multi), tokens_in_variant_groups=tok_in_multi,
           tokens_on_minority_spellings_lower_bound=only_variant, kinds=dict(kinds),
           works_with_any_j_lemma=len(works_with_j), tokens_with_j_lemma=sum(works_with_j.values()))
json.dump(res, open(out, 'w'), indent=1); print(json.dumps(res, indent=1))
