import sys,re,collections,unicodedata
import xml.etree.ElementTree as ET
r=ET.parse(sys.argv[1]).getroot()
es=list(r.iter('entryFree'))
C=collections.Counter
def txt(e): return ''.join(e.itertext())
keys=[e.get('key') for e in es]
print('entries',len(es),'unique keys',len(set(keys)),'dup-key entries',sum(v for v in C(keys).values() if v>1),'distinct dup keys',sum(1 for v in C(keys).values() if v>1))
print('keys with digit',sum(bool(re.search(r'\d',k)) for k in keys), [k for k in keys if re.search(r'\d',k)][:5])
print('keys ascii-only',sum(bool(re.fullmatch(r'[A-Za-z]+',k)) for k in keys))
print('keys w/ non-letter', C(re.sub(r'[A-Za-z]','',k) for k in keys).most_common(12))
print('capitalised keys',sum(k[:1].isupper() for k in keys))
print('keys with j',sum('j' in k.lower() for k in keys),'with v',sum('v' in k.lower() for k in keys),'with u',sum('u' in k.lower() for k in keys),'with i',sum('i' in k.lower() for k in keys))
# orth
o=[(e,e.find('orth')) for e in es]
print('no orth',sum(x is None for _,x in o))
first=[txt(x) for _,x in o if x is not None]
mac=sum(bool(re.search('[āēīōūȳĀĒĪŌŪ̄]',unicodedata.normalize('NFC',s))) for s in first)
brv=sum(bool(re.search('[ăĕĭŏŭ̆]',unicodedata.normalize('NFC',s))) for s in first)
print('first-orth with macron',mac,'with breve',brv)
print('first-orth with digit',sum(bool(re.search(r'\d',s)) for s in first))
print('first orth chars',C(c for s in first for c in unicodedata.normalize('NFD',s) if not c.isascii() ).most_common(15))
print('orth lang',C(x.get('lang') for _,x in o if x is not None), 'extent',C(x.get('extent') for _,x in o if x is not None))
# homograph: how are multiple same-key entries distinguished
dups=[k for k,v in C(keys).items() if v>1]
print('example dup counts',C(C(keys)[k] for k in dups))
# first orth normalised vs key
def norm(s): return ''.join(c for c in unicodedata.normalize('NFD',s) if not unicodedata.combining(c)).lower()
agree=sum(norm(txt(x)).strip(' ,.')==k.lower() for (e,x),k in zip(o,keys) if x is not None)
print('first orth (stripped) equals key',agree)
# enclitic / hyphen / paren
print('keys with -',sum('-' in k for k in keys))
print('orth text starting with -',sum(s.startswith('-') for s in first))
print('multi-orth entries',sum(len(e.findall('orth'))>1 for e in es))
print('itype entries',sum(e.find('itype') is not None for e in es),'pos',sum(e.find('pos') is not None for e in es))
for k in ['sum','fero','amo','mos','aut','que','ne','ve','sequor','Caesar','a','ab','is','pono']:
    print(k,[ (e.get('id'),e.get('type'),txt(e.find('orth'))[:20] if e.find('orth') is not None else None) for e in es if e.get('key')==k][:6])
