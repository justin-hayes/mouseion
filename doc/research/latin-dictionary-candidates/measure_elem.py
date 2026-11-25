"""Measure Lewis Elementary Latin Dictionary TEI structure (entities pre-resolved). Aggregates only.
Usage: python3 -I measure_elem.py <elem.xml>"""
import sys,re,json,collections,random,unicodedata,xml.etree.ElementTree as ET
r=ET.parse(sys.argv[1]).getroot()
C=collections.Counter
es=list(r.iter('entry'))
txt=lambda e:''.join(e.itertext())
keys=[e.get('key') for e in es]
print('entries',len(es),'types',dict(C(e.get('type') for e in es)),'unique keys',len(set(keys)))
print('keys with trailing digit',sum(bool(re.search(r'\d$',k)) for k in keys),'ascii-only',sum(bool(re.fullmatch(r'[A-Za-z]+\d*',k)) for k in keys),'capitalised',sum(k[:1].isupper() for k in keys))
print('keys with j',sum('j' in k.lower() for k in keys),'v',sum('v' in k.lower() for k in keys))
o=[txt(e.find('form/orth')) for e in es]
nf=lambda s:unicodedata.normalize('NFD',s)
print('orth with macron',sum('̄' in nf(s) for s in o),'breve',sum('̆' in nf(s) for s in o))
def defs(s):
    out=[]
    for c in s.iter():
        if c.tag=='foreign' or c.tag=='usg': break
        if c.tag=='tr':
            t=txt(c).strip(' ,;:.')
            if t: out.append(t)
    return out
# first-citation window: iterate in document order over the sense subtree
def defs_doc(s):
    out=[];seen=False
    def walk(n):
        nonlocal seen
        if seen: return
        for c in n:
            if seen: return
            if c.tag=='foreign': seen=True; return
            if c.tag=='tr':
                t=txt(c).strip(' ,;:.')
                if t: out.append(t)
            else: walk(c)
    walk(s); return out
st=C()
for e in es:
    ss=e.findall('sense'); st['senses']+=len(ss)
    if not ss: st['no_sense']+=1
    d=[defs_doc(s) for s in ss]
    st['senses_with_defs']+=sum(bool(x) for x in d)
    if any(d): st['entries_with_def']+=1
    if ss and any(c.tag in('trans',) for c in e) : pass
    if ss and not any(d): st['senses_no_def_entries']+=1
print(dict(st))
print('sense level',sorted(C(s.get('level') for s in r.iter('sense')).items()))
print('sense n labels sample',C((s.get('level'),s.get('n')) for s in r.iter('sense')).most_common(8))
print('entries with tr',sum(e.find('.//tr') is not None for e in es))
print('entries with a sense containing tr',sum(any(s.find('.//tr') is not None for s in e.findall('sense')) for e in es))
print('entries with only cross-ref text<=120',sum(len(txt(e))<=120 for e in es))
print('empty-def tr (strings)',sum(len(txt(t).strip(' ,;:.'))<=3 for t in r.iter('tr')),'of',sum(1 for _ in r.iter('tr')))
trs=[txt(t).strip(' ,;:.') for t in r.iter('tr')]
print('tr starting P. of/from',sum(bool(re.match(r'^P\. ?(a\. )?(of|from)\b',t)) for t in trs),'tr starting see',sum(t.lower().startswith('see') for t in trs))
nx=[e for e in es if not any(re.match(r'^(P\. ?(a\. )?(of|from)|see\b)',txt(t).strip()) for t in e.iter('tr')) and e.find('.//tr') is not None]
print('entries with a real (non-xref) tr',len(nx))
print('entries with >=2 tr glosses before first citation-free window? (any 2+ tr)',sum(len(list(e.iter('tr')))>=2 for e in es))
