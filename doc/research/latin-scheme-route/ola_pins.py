"""Per-Work pin table for the OLA v0.2.0 BY-SA classical Work files: for the
CoNLL-U member (workspace/conllu.zip) and the cts01 member (workspace/ola.zip),
the absolute byte offset of the member's local header in the Zenodo file, its
compressed size, CRC-32 and the SHA-256 of the inflated bytes. Then re-fetches
two members by one range request each (no zip directory read) and checks them.
usage: ola_pins.py <manifest.tsv> <conllu-dir> <cts-dir> <out.tsv>"""
import sys, os, zlib, hashlib, struct, urllib.request, time, urllib.error
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import rangezip
manifest, cdir, ctsdir, out = sys.argv[1:5]
rows = [l.rstrip('\n').split('\t') for l in open(manifest)][1:]
urns = [r[0] for r in rows if r[1] == 'core' and 'CC-BY-SA' in r[3]]
z = rangezip.outer()
def start_of(name):
    i = z.getinfo(name); z.fp.seek(i.header_offset); h = z.fp.read(30)
    return i.header_offset + 30 + int.from_bytes(h[26:28], 'little') + int.from_bytes(h[28:30], 'little')
cz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/conllu.zip'); cbase = start_of('opera_latina_adnotata_v0.2.0/workspace/conllu.zip')
oz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/ola.zip'); obase = start_of('opera_latina_adnotata_v0.2.0/workspace/ola.zip')
pidx = {}
for i in oz.infolist():
    p = i.filename.split('/')
    if len(p) == 4 and p[1] in ('ola_v0.2.0_1', 'ola_v0.2.0_2') and p[3]: pidx[(p[2], p[3])] = i
def sha(path): return hashlib.sha256(open(path, 'rb').read()).hexdigest()
lines = ['urn\tconllu_abs_offset\tconllu_comp_size\tconllu_size\tconllu_crc32\tconllu_sha256\tcts_abs_offset\tcts_comp_size\tcts_size\tcts_crc32\tcts_sha256']
rec = {}
for u in urns:
    ci = cz.getinfo(f'conllu/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu')
    pi = pidx[(u, f'ola.{u}.tok01_cts01.xml')]
    rec[u] = (cbase + ci.header_offset, ci, obase + pi.header_offset, pi)
    lines.append('\t'.join(map(str, [u, cbase + ci.header_offset, ci.compress_size, ci.file_size, f'{ci.CRC:08x}',
        sha(f'{cdir}/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu'), obase + pi.header_offset, pi.compress_size, pi.file_size, f'{pi.CRC:08x}', sha(f'{ctsdir}/{u}.cts01.xml')])))
open(out, 'w').write('\n'.join(lines) + '\n')
# verify two members with one range request each, straight from the manifest numbers
def member(abs_off, info):
    # 30-byte local header + name + extra, then the deflate stream
    need = 30 + len(info.filename.encode()) + 64 + info.compress_size
    r = rangezip.RangeFile(length=3857914226)
    r.seek(abs_off); raw = r.read(need)
    nlen, elen = struct.unpack('<HH', raw[26:30]); body = raw[30 + nlen + elen: 30 + nlen + elen + info.compress_size]
    return zlib.decompress(body, -15), r.requests
for u in ('phi0631.phi001.perseus-lat4', 'phi1351.phi002.perseus-lat1'):
    co, ci, po, pi = rec[u]
    a, ra = member(co, ci); b, rb = member(po, pi)
    print(u, 'conllu ok', hashlib.sha256(a).hexdigest() == sha(f'{cdir}/sentence-seg.ola.{u}.tok01_sentence-seg01.predicted.conllu'), 'requests', ra,
          '| cts ok', hashlib.sha256(b).hexdigest() == sha(f'{ctsdir}/{u}.cts01.xml'), 'requests', rb)
