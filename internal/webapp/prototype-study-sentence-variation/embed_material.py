"""PROTOTYPE helper: inline words.json into index.html's study-material block.

Run from anywhere: python3 internal/webapp/prototype-study-sentence-variation/embed_material.py
"""
import json
import re
from pathlib import Path

here = Path(__file__).resolve().parent
words = json.loads((here / "words.json").read_text(encoding="utf-8"))
payload = json.dumps(words, ensure_ascii=False, indent=1).replace("</", "<\\/")
page = here / "index.html"
html = page.read_text(encoding="utf-8")
pattern = re.compile(r'(<script type="application/json" id="study-material">\n).*?(\n</script>)', re.S)
html, n = pattern.subn(lambda m: m.group(1) + payload + m.group(2), html)
if n != 1:
    raise SystemExit("study-material block not found")
page.write_text(html, encoding="utf-8")
print(f"embedded {len(words)} words")
