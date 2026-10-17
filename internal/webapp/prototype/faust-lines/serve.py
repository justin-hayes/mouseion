"""PROTOTYPE server for #2149. Serves the repo with /static/ mapped like the app.

    python3 internal/webapp/prototype/faust-lines/serve.py [bind-address] [port]

Then open http://<address>:8149/ (redirects to the prototype).
"""

import functools
import http.server
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[4]
PAGE = "/internal/webapp/prototype/faust-lines/"


class Handler(http.server.SimpleHTTPRequestHandler):
    def do_GET(self):
        if self.path in ("/", ""):
            self.send_response(302)
            self.send_header("Location", PAGE)
            self.end_headers()
            return
        if self.path.startswith("/static/"):
            self.path = "/internal/webapp" + self.path
        super().do_GET()


bind = sys.argv[1] if len(sys.argv) > 1 else "127.0.0.1"
port = int(sys.argv[2]) if len(sys.argv) > 2 else 8149
handler = functools.partial(Handler, directory=str(ROOT))
http.server.ThreadingHTTPServer((bind, port), handler).serve_forever()
