"""Citation scheme carried by the OLA cts01 layer, per Work (aggregates only).

usage: python3 -I cts_scheme.py <ola.zip> <out.json>
For each Work: modal citation depth (number of '_'-separated components of the
cts value), share of tokens with an empty or missing value, number of distinct
citation values.
"""
import collections
import json
import re
import sys
import zipfile

VAL = re.compile(rb'<feat id="u_\d+" xlink:href="#t_\d+" value="([^"]*)"')


def main(zpath, out):
    z = zipfile.ZipFile(zpath)
    res = {}
    for n in z.namelist():
        if "/ola_v0.2.0_" in n and n.endswith("tok01_cts01.xml"):
            urn = re.search(r"ola_v0\.2\.0_\d/([^/]+)/", n).group(1)
            vals = VAL.findall(z.read(n))
            depth = collections.Counter(len(v.split(b"_")) if v else 0 for v in vals)
            res[urn] = dict(
                tokens=len(vals),
                empty=sum(1 for v in vals if not v),
                distinct=len(set(vals)),
                modal_depth=depth.most_common(1)[0][0] if depth else 0,
                depth_hist=dict(sorted(depth.items())),
            )
    json.dump(res, open(out, "w"), indent=1, sort_keys=True)
    print(len(res))


if __name__ == "__main__":
    main(*sys.argv[1:3])
