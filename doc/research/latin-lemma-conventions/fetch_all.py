"""usage: fetch_all.py <outdir> <urn-list-file>: CoNLL-U members of the pinned OLA archive by range request."""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rangezip
out, lst = sys.argv[1:3]
urns = [l.strip() for l in open(lst) if l.strip()]
z = rangezip.outer()
cz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/conllu.zip')
os.makedirs(out, exist_ok=True)
for u in urns:
    p = f'{out}/{u}.conllu'
    if os.path.exists(p): continue
    open(p, 'wb').write(cz.read(f'conllu/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu'))
print('done', len(urns))
