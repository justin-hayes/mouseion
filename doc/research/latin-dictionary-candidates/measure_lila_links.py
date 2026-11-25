"""Aggregate the CIRCSE/LewisShort RDF -> LiLa lemma links against Perseus L&S entry ids.
Usage: python3 -I measure_lila_links.py <lewisShort09052023.ttl> <lat.ls.perseus-eng2.xml>"""
import re,sys,collections,xml.etree.ElementTree as ET
s=open(sys.argv[1],encoding='utf8').read()
link={}
for b in s.split('\n\n'):
    m=re.match(r'ls_lexical_entry:(n\d+)\s',b)
    if m and 'ontolex:LexicalEntry' in b:
        l=re.findall(r'<http://lila-erc\.eu/data/id/lemma/(\d+)>',b)
        link[m.group(1)]=l
r=ET.parse(sys.argv[2]).getroot()
ents={e.get('id'):e for e in r.iter('entryFree')}
print('RDF lexical entries',len(link),'with >=1 lemma link',sum(bool(v) for v in link.values()),'with >1',sum(len(v)>1 for v in link.values()),'distinct lemma ids',len({x for v in link.values() for x in v}))
print('RDF entry ids present in Perseus file',len(set(link)&set(ents)),'absent',len(set(link)-set(ents)))
print('Perseus entries without RDF entry',len(set(ents)-set(link)),'; by type',dict(collections.Counter(ents[i].get('type') for i in set(ents)-set(link))))
print('Perseus main entries linked',sum(1 for i,e in ents.items() if e.get('type')=='main' and link.get(i)),'of',sum(e.get('type')=='main' for e in ents.values()))
