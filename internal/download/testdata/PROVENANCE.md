# Provenance

`tall.pdf` and `leaves.epub` are KOReader's own partial-MD5 test files. KOReader
references them as `spec/front/unit/data/` in `spec/unit/util_spec.lua`
(`describe("partialMD5()")`), where they give `41cce710f34e5ec21315e19c99821415`
and `59d481d168cca6267322f150c5f6a2a3`.

They live in KOReader's `test` submodule, https://github.com/koreader/test-data,
at commit `c3b5d06e1ae8ea088bd898b0e7f9da3753d0d86c`, the commit koreader/koreader
master `b32d91a7c1c2395a15b6672e0ef63e719fcddf2a` pins. The test-data repository
declares no license of its own; KOReader itself is AGPL-3.0, as Mouseion is.
