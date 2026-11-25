import sys,re,random,xml.etree.ElementTree as ET
r=ET.parse(sys.argv[1]).getroot()
es=[e for e in r.iter('entryFree') if e.get('type')=='main']
txt=lambda e:''.join(e.itertext())
def defs(s):
    o=[]
    for c in s:
        if c.tag in('cit','bibl'):break
        if c.tag=='hi' and c.get('rend')=='ital':
            t=txt(c).strip(' ,;:.')
            if t:o.append(t)
    return o
random.seed(int(sys.argv[2])); n=int(sys.argv[3])
for e in random.sample(es,n):
    ss=e.findall('sense')
    print('##',e.get('key'),e.get('id'),'|',txt(e.find('orth')),'|',txt(e.find('itype')) if e.find('itype') is not None else '','|',(txt(e.find('pos')) if e.find('pos') is not None else ''), '| len',len(txt(e)))
    if not ss: print('   NOSENSE',txt(e)[:110]);continue
    for s in ss[:7]:
        print('  ',s.get('level'),s.get('n'),defs(s)[:4], '' if defs(s) else 'TXT:'+txt(s)[:50].replace('\n',' '))
