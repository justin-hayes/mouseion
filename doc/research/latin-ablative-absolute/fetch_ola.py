import sys, os, re
sys.path.insert(0, os.environ['RANGEZIP_DIR'])  # doc/research/latin-scheme-route on branch research/latin-scheme-route
import rangezip
out = sys.argv[1]
pat = re.compile(sys.argv[2])
z = rangezip.outer()
cz = rangezip.nested(z, 'opera_latina_adnotata_v0.2.0/workspace/conllu.zip')
for i in cz.infolist():
    m = re.match(r'conllu/sentence-seg\.ola\.(.+)\.tok01_sentence-seg01\.predicted\.conllu$', i.filename)
    if m and pat.search(m.group(1)):
        open(f'{out}/{m.group(1)}.conllu','wb').write(cz.read(i)); print(m.group(1), i.file_size)
