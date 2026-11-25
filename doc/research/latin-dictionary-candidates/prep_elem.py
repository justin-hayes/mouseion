import re,sys,html.entities,collections
s=open(sys.argv[1],encoding='utf8').read()
c=collections.Counter(re.findall(r'&([A-Za-z0-9]+);',s))
builtin={'amp','lt','gt','quot','apos'}
unk=[k for k in c if k not in builtin and k not in html.entities.html5 and k+';' not in html.entities.html5]
print('entity kinds',len(c),'refs',sum(c.values()),'unknown to HTML5',unk[:20],sum(c[k] for k in unk))
def rep(m):
    n=m.group(1)
    if n in builtin: return m.group(0)
    v={'ymacr':'\u0233','acutemacr':'\u0301\u0304','cj':'\u2060'}.get(n) or html.entities.html5.get(n+';')
    return v if v else m.group(0)
s=re.sub(r'&([A-Za-z0-9]+);',rep,s)
s=re.sub(r'<!DOCTYPE.*?\]>','',s,flags=re.S)
open(sys.argv[2],'w',encoding='utf8').write(s)
