"""Profile of Scaife/Perseus6 LatinCy tagging-shard TSV files (UD scheme):
chunking, sentence boundaries, roots, tag and relation inventories.
usage: measure_scaife.py <out.json> <file.tagged.tsv> ...   (counts only)"""
import sys, json, collections, re
out, files = sys.argv[1], sys.argv[2:]
res = {}
for f in files:
    chunks = collections.OrderedDict(); a = collections.Counter(); rel = collections.Counter(); upos = collections.Counter()
    for line in open(f, encoding='utf-8'):
        c = line.rstrip('\n').split('\t')
        if len(c) != 9: a['bad_cols'] += 1; continue
        ref, idx, form, u, x, morph, lemma, dep, head = c
        chunks.setdefault(ref, []).append((int(idx), form, u, dep, int(head)))
        rel[dep] += 1; upos[u] += 1; a['tokens'] += 1
        if form == '': a['blank_form'] += 1
        if form.lower() in ('que', 'ne', 've') : a['split_enclitic_form'] += 1
    a['chunks'] = len(chunks)
    sizes = sorted(len(v) for v in chunks.values())
    a['chunk_median_tokens'] = sizes[len(sizes) // 2]
    roots = [sum(1 for t in v if t[3] == 'ROOT') for v in chunks.values()]
    a['chunks_roots_0'] = roots.count(0); a['chunks_roots_1'] = roots.count(1); a['chunks_roots_2plus'] = sum(1 for r in roots if r >= 2)
    a['roots_total'] = sum(roots)
    # sentence reconstruction: a sentence is a ROOT's tree; check contiguity
    noncontig = 0
    for v in chunks.values():
        for t in v:
            pass
    a['ref_shape'] = re.sub(r'\d+', 'N', next(iter(chunks)))
    res[f.split('/')[-1]] = dict(counts=dict(a), upos=dict(upos), rel_base=dict(collections.Counter({k.split(':')[0]: 0 for k in rel}) ), rel=dict(rel))
json.dump(res, open(out, 'w'), indent=1, sort_keys=True)
for k, v in res.items(): print(k, v['counts'], '\n  upos', v['upos'], '\n  rels', len(v['rel']), sorted(v['rel'].items(), key=lambda x: -x[1])[:14])
