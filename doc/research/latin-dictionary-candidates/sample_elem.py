import sys,random,xml.etree.ElementTree as ET
r=ET.parse(sys.argv[1]).getroot()
txt=lambda e:''.join(e.itertext())
es=list(r.iter('entry'))
random.seed(int(sys.argv[2]))
for e in random.sample(es,int(sys.argv[3])):
    ss=e.findall('sense')
    allt=[txt(t).strip(' ,;:.') for t in e.iter('tr')]
    print('##',e.get('key'),'|',txt(e.find('form/orth')),'| senses',len(ss),'levels',[s.get('level') for s in ss][:6],'| tr',len(allt),allt[:6],'| len',len(txt(e)))
    if not allt: print('    RAW',txt(e)[:100])
