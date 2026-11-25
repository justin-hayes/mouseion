"""Check, on the sampled Works, that the CoNLL-U token (column 10, t_N) joins the
PAULA tok01 mark and the cts01 feature of the same id, and that the surface in
the CoNLL-U equals the surface recorded for that id in tok01.
usage: measure_join.py <ola-sample-dir> <out.json> (counts only)"""
import sys, os, re, json, html
base, out = sys.argv[1:3]
res = {}
for urn in sorted(os.listdir(base + '/conllu')):
    urn = urn[:-len('.conllu')]
    pd = f'{base}/paula/{urn}'
    tok = open(f'{pd}/ola.{urn}.tok01.xml', encoding='utf-8').read()
    forms = {int(m.group(1)): html.unescape(m.group(2)) for m in re.finditer(r'<mark id="t_(\d+)"[^>]*/>\s*<!--(.*?)-->', tok, re.S)}
    cts = {int(m.group(1)): m.group(2) for m in re.finditer(r'xlink:href="#t_(\d+)" value="([^"]*)"', open(f'{pd}/ola.{urn}.tok01_cts01.xml', encoding='utf-8').read())}
    n = miss_tok = miss_cts = diff = 0; ids = []
    for line in open(f'{base}/conllu/{urn}.conllu', encoding='utf-8'):
        c = line.rstrip('\n').split('\t')
        if len(c) != 10: continue
        t = int(c[9][2:]); n += 1; ids.append(t)
        if t not in forms: miss_tok += 1
        elif forms[t] != c[1]: diff += 1
        if t not in cts: miss_cts += 1
    res[urn] = dict(conllu_tokens=n, paula_tokens=len(forms), cts_values=len(cts), conllu_not_in_tok=miss_tok, conllu_not_in_cts=miss_cts,
                    surface_differs=diff, ids_increasing=all(a < b for a, b in zip(ids, ids[1:])), paula_only_tokens=len(set(forms) - set(ids)))
json.dump(res, open(out, 'w'), indent=1, sort_keys=True)
for k, v in res.items(): print(k, v)
