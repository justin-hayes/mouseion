"""Seekable file object over HTTP range requests, so zipfile can read a
member of the 3.86 GB OLA archive (and the zips nested inside it) without
downloading the whole archive."""
import io, time, urllib.error, urllib.request, zipfile

URL = 'https://zenodo.org/api/records/15183688/files/opera_latina_adnotata_v0.2.0.zip/content'

class RangeFile(io.RawIOBase):
    def __init__(self, url=URL, size=None, base=0, length=None, cache=1 << 20):
        self.url, self.base, self.pos = url, base, 0
        self.length = length
        if self.length is None:
            req = urllib.request.Request(url, headers={'Range': 'bytes=0-0'})
            with urllib.request.urlopen(req) as r:
                self.length = int(r.headers['Content-Range'].split('/')[1])
        self.requests = 0
        self.bytes = 0
        self._blocks = {}
    def seekable(self): return True
    def readable(self): return True
    def tell(self): return self.pos
    def seek(self, off, whence=0):
        self.pos = {0: off, 1: self.pos + off, 2: self.length + off}[whence]
        return self.pos
    def _fetch(self, a, b):
        req = urllib.request.Request(self.url, headers={'Range': f'bytes={self.base+a}-{self.base+b-1}'})
        for attempt in range(12):
            try:
                with urllib.request.urlopen(req, timeout=120) as r:
                    data = r.read()
                self.requests += 1; self.bytes += len(data)
                return data
            except urllib.error.HTTPError as e:
                # Zenodo rate-limits anonymous clients (429, Retry-After 60).
                if e.code == 429: time.sleep(20 + 10 * attempt); continue
                raise
            except Exception:
                if attempt == 11: raise
                time.sleep(2)
    BLOCK = 1 << 19
    def read(self, n=-1):
        if n < 0 or self.pos + n > self.length: n = self.length - self.pos
        if n <= 0: return b''
        if n >= self.BLOCK:  # large read: fetch exactly what is asked
            data = self._fetch(self.pos, self.pos + n); self.pos += n; return data
        out, end = [], self.pos + n
        while self.pos < end:
            b = self.pos // self.BLOCK
            if b not in self._blocks:
                a = b * self.BLOCK
                self._blocks[b] = self._fetch(a, min(a + self.BLOCK, self.length))
            blk = self._blocks[b]; off = self.pos - b * self.BLOCK
            take = blk[off:off + (end - self.pos)]
            out.append(take); self.pos += len(take)
        return b''.join(out)
    def readinto(self, b):
        d = self.read(len(b)); b[:len(d)] = d; return len(d)

def outer():
    return zipfile.ZipFile(RangeFile())

def nested(zf, name, base_url=URL):
    """Open a stored (uncompressed) member that is itself a zip, by offset."""
    info = zf.getinfo(name)
    assert info.compress_type == 0, name
    # local header: 30 bytes + name + extra
    raw = zf.fp
    raw.seek(info.header_offset)
    h = raw.read(30)
    nlen = int.from_bytes(h[26:28], 'little'); elen = int.from_bytes(h[28:30], 'little')
    start = info.header_offset + 30 + nlen + elen
    return zipfile.ZipFile(RangeFile(base_url, base=start, length=info.file_size))
