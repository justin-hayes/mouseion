"""Fetch a sample of Works from the pinned OLA v0.2.0 archive by range request.
usage: fetch_sample.py <outdir> <urn> [<urn> ...]
Writes <outdir>/conllu/<urn>.conllu and <outdir>/paula/<urn>/<member> for the
token, cts, text and sentence layers, and prints per-member sizes (the
per-Work fetch cost)."""
import sys, os, json
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rangezip
out, urns = sys.argv[1], sys.argv[2:]
z = rangezip.outer()
cz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/conllu.zip')
oz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/ola.zip')
index = {}
for i in oz.infolist():
    parts = i.filename.split('/')
    if len(parts) == 4 and parts[1] == 'ola_v0.2.0_1' and parts[3]:
        index.setdefault(parts[2], []).append(i)
os.makedirs(out + '/conllu', exist_ok=True)
cost = {}
for urn in urns:
    ci = [i for i in cz.infolist() if i.filename == f'conllu/sentence-seg.ola.{urn}.tok01_sentence-seg01.predicted.conllu']
    assert len(ci) == 1, (urn, [i.filename for i in ci][:3])
    data = cz.read(ci[0])
    open(f'{out}/conllu/{urn}.conllu', 'wb').write(data)
    cost[urn] = {'conllu_compressed': ci[0].compress_size, 'conllu_bytes': len(data), 'paula': {}}
    os.makedirs(f'{out}/paula/{urn}', exist_ok=True)
    for i in index.get(urn, []):
        name = i.filename.split('/')[-1]
        if any(k in name for k in ('_cts01', '.tok01.xml', '.text.xml', '_postag01', '_lemma01', 'dep01')):
            open(f'{out}/paula/{urn}/{name}', 'wb').write(oz.read(i))
            cost[urn]['paula'][name] = i.compress_size
print(json.dumps(cost, indent=1)[:3000])
print('requests', z.fp.requests + cz.fp.requests + oz.fp.requests, 'bytes', z.fp.bytes + cz.fp.bytes + oz.fp.bytes)
