"""Fetch the PAULA cts01 (Citation) member of every classical Work file with
BY-SA text, by range request. usage: fetch_cts.py <manifest.tsv> <outdir>"""
import sys, os
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rangezip
manifest, out = sys.argv[1:3]
os.makedirs(out, exist_ok=True)
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
urns = [r[0] for r in rows if r[1] == 'core' and 'CC-BY-SA' in r[3]]
z = rangezip.outer()
oz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/ola.zip')
idx = {}
for i in oz.infolist():
    p = i.filename.split('/')
    if len(p) == 4 and p[1] in ('ola_v0.2.0_1', 'ola_v0.2.0_2') and p[3]: idx[(p[2], p[3])] = i
for n, u in enumerate(urns):
    path = f'{out}/{u}.cts01.xml'
    if os.path.exists(path): continue
    open(path, 'wb').write(oz.read(idx[(u, f'ola.{u}.tok01_cts01.xml')]))
print('done', len(urns), oz.fp.requests, oz.fp.bytes)
