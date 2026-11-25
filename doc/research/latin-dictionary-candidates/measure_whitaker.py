"""Aggregate Whitaker's Words DICTLINE.GEN. Usage: python3 -I measure_whitaker.py DICTLINE.GEN"""
import sys,re,collections
C=collections.Counter
rx=re.compile(r'^(.{19})(.{19})(.{19})(.{19})(\S+)\s+(.*?)\s+([A-Z]) ([A-Z]) ([A-Z]) ([A-Z]) ([A-Z]) (.*)$')
n=0;bad=0;pos=C();age=C();freq=C();src=C();segs=C();gl=0
stems=C();noascii=0
for line in open(sys.argv[1],encoding='latin-1'):
    line=line.rstrip('\r\n'); n+=1
    m=rx.match(line)
    if not m: bad+=1; continue
    s=[m.group(i).strip() for i in range(1,5)]
    p=m.group(5); pos[p]+=1; age[m.group(7)]+=1; freq[m.group(10)]+=1; src[m.group(11)]+=1
    g=m.group(12).strip(); gl+=bool(g)
    k=len([x for x in g.split(';') if x.strip()]); segs[min(k,6)]+=1
    if not all(ord(c)<128 for c in line): noascii+=1
    if any(re.search(r'[AEIOUaeiou][^ ]*',x) for x in s[:1]) and re.search(r'[ŭūāă]',line): pass
print('lines',n,'unparsed',bad,'with gloss',gl,'non-ascii lines',noascii)
print('pos',pos.most_common(12));print('age',sorted(age.items()));print('freq',sorted(freq.items()));print('gloss segment counts',sorted(segs.items()))
