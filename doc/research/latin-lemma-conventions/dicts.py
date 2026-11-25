"""Headword sets for Lewis & Short and Lewis's Elementary (Perseus lexica @56061ca1)."""
import re, os, sys, html.entities, xml.etree.ElementTree as ET, collections
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import nfd_strip
LEX = os.environ['LEXICA']
LS = f'{LEX}/CTS_XML_TEI/perseus/pdllex/lat/ls/lat.ls.perseus-eng2.xml'
EL = f'{LEX}/CTS_XML_TEI/perseus/pdllex/lat/viaf2845558/viaf001/viaf2845558.viaf001.xml'
txt = lambda e: ''.join(e.itertext())
def clean_orth(s):
    s = nfd_strip(s)
    s = re.sub(r'[\-‐‑\'’]', '', s)
    s = re.sub(r'[,;.:?!()\[\]*†]', ' ', s)
    w = s.split()
    return w[0] if len(w) == 1 else None
def load_elem():
    import html
    s = open(EL, encoding='utf8').read()
    builtin = {'amp','lt','gt','quot','apos'}
    def rep(m):
        n = m.group(1)
        if n in builtin: return m.group(0)
        v = {'ymacr':'ȳ','acutemacr':'́̄','cj':'⁠'}.get(n) or html.entities.html5.get(n+';')
        return v if v else m.group(0)
    s = re.sub(r'&([A-Za-z0-9]+);', rep, s)
    s = re.sub(r'<!DOCTYPE.*?\]>', '', s, flags=re.S)
    return ET.fromstring(s)
def load():
    """returns {'ls': {'key': set, 'orth': set, 'entries': [(key_raw, base_key, [orths])]}, 'el': ...}"""
    out = {}
    r = ET.parse(LS).getroot()
    ents = []
    for e in r.iter('entryFree'):
        k = e.get('key')
        o = [clean_orth(txt(x)) for x in e.findall('orth')]
        ents.append((k, re.sub(r'\d+$', '', k), [x for x in o if x], e.get('type'), e.get('id')))
    out['ls'] = ents
    r = load_elem()
    ents = []
    for e in r.iter('entry'):
        k = e.get('key')
        o = [clean_orth(txt(x)) for x in e.findall('form/orth')]
        ents.append((k, re.sub(r'\d+$', '', k), [x for x in o if x], e.get('type'), e.get('id')))
    out['el'] = ents
    return out
