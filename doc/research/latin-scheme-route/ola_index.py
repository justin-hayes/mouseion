"""Size and per-Work member index of the pinned OLA v0.2.0 archive, read from
the central directories only (no member bodies). Prints/writes, per classical
BY-SA Work file: compressed and uncompressed size of the CoNLL-U member and of
the cts01 (Citation) and tok01 members in workspace/ola.zip.
usage: ola_index.py <manifest.tsv> <out.json>"""
import sys, os, json
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rangezip
manifest, out = sys.argv[1:3]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
z = rangezip.outer()
cz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/conllu.zip')
oz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/ola.zip')
cinfo = {i.filename: i for i in cz.infolist()}
pinfo = {}
for i in oz.infolist():
    p = i.filename.split('/')
    if len(p) == 4 and p[1] in ('ola_v0.2.0_1', 'ola_v0.2.0_2') and p[3]: pinfo[(p[2], p[3])] = i
res = {}
for r in rows:
    urn = r[0]
    c = cinfo.get(f'conllu/sentence-seg.ola.{urn}.tok01_sentence-seg01.predicted.conllu')
    t = pinfo.get((urn, f'ola.{urn}.tok01_cts01.xml'))
    k = pinfo.get((urn, f'ola.{urn}.tok01.xml'))
    if not (c and t and k): res[urn] = dict(missing=True); continue
    res[urn] = dict(cls=r[1], licence=r[3], conllu_z=c.compress_size, conllu=c.file_size, conllu_crc=f'{c.CRC:08x}',
                    cts_z=t.compress_size, cts=t.file_size, cts_crc=f'{t.CRC:08x}',
                    tok_z=k.compress_size, tok=k.file_size)
json.dump(res, open(out, 'w'), indent=0, sort_keys=True)
print(len(res), sum(1 for v in res.values() if v.get('missing')), 'requests', z.fp.requests + cz.fp.requests + oz.fp.requests, 'bytes', z.fp.bytes + cz.fp.bytes + oz.fp.bytes)
print('central directories: outer', z.fp.bytes, 'conllu.zip', cz.fp.bytes, 'ola.zip', oz.fp.bytes)
