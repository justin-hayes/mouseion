# Frontend vendor assets

Mouseion serves HTMX and one compiled application stylesheet from the embedded
static filesystem. CSS source lives under [`../../styles/`](../../styles/), with
`../../styles/app.css` as the build entry point and `../app.css` as its committed
output. The application does not require a browser-time connection to a CDN.

## Pinned assets

| Asset               | Version | Upstream                                   | SHA-256                                                            |
| ------------------- | ------: | ------------------------------------------ | ------------------------------------------------------------------ |
| `htmx-4.0.0.min.js` |   4.0.0 | `htmx.org` npm package, `dist/htmx.min.js` | `e484d9171a9db30a39c8f16e3d709d4137f3211c659f8e6125816635033d593f` |

## Shipped typefaces

The approved #1480 Margin notes direction uses Commissioner for application UI
and Literata for Book identity and source passages. Both are OFL-licensed and
served from this embedded directory; fonts do not load from a third-party host.
The source CSS and original assets are preserved in the prototype at commit
`3fd446144af25013a19ab99d0a83e4e778fe7737`.

| Asset | Version / upstream source | License | SHA-256 |
| --- | --- | --- | --- |
| `fonts/commissioner-latin-wght-normal.woff2` | `@fontsource-variable/commissioner@5.3.0/files/commissioner-latin-wght-normal.woff2` | OFL (`fonts/commissioner-OFL.txt`) | `e3d539b926881fbb3592788297eee24d6d179c1c06dbad36805fc722775247ce` |
| `fonts/commissioner-latin-ext-wght-normal.woff2` | `@fontsource-variable/commissioner@5.3.0/files/commissioner-latin-ext-wght-normal.woff2` | OFL (`fonts/commissioner-OFL.txt`) | `bd6c9bcd8fddfa16facc4398d075d0dfdf1286fc08334b94e8ea0c94beb1e0ff` |
| `fonts/commissioner-greek-wght-normal.woff2` | `@fontsource-variable/commissioner@5.3.0/files/commissioner-greek-wght-normal.woff2` | OFL (`fonts/commissioner-OFL.txt`) | `75d4734b1d207892feee610186e34fe02912d604025906fdeca5c7b8c295a622` |
| `fonts/literata-latin-opsz-normal.woff2` | `@fontsource-variable/literata@5.3.0/files/literata-latin-opsz-normal.woff2` | OFL (`fonts/literata-OFL.txt`) | `29de894c768689feef6ab4ef274a9a16d19bfc5b0c3cbfcdac80ef220816210c` |
| `fonts/literata-latin-ext-opsz-normal.woff2` | `@fontsource-variable/literata@5.3.0/files/literata-latin-ext-opsz-normal.woff2` | OFL (`fonts/literata-OFL.txt`) | `1160835f4cdae6a86572ed501c13b62b341480cc7c13fc88ba0c21efdf1fcd73` |
| `fonts/literata-greek-opsz-normal.woff2` | `@fontsource-variable/literata@5.3.0/files/literata-greek-opsz-normal.woff2` | OFL (`fonts/literata-OFL.txt`) | `1a7f2e3f1b37ae8c92b57eb8f164da44ac369783bd58ad6295368bbc7b9f7d08` |

HTMX and build-tool upstream licenses are under [`licenses/`](licenses/);
the two font OFL texts are alongside the font assets in [`fonts/`](fonts/).

## CSS build tools

These exact build-time bundles are downloaded by `tools/build-frontend-css.sh` into
ignored `.tmp/` storage and verified before the compiler or plugin is used. They
are not served to browsers or needed at runtime.

| Build input | Version | Upstream release asset | SHA-256 |
| --- | ---: | --- | --- |
| Tailwind standalone CLI (Linux x64) | 4.1.11 | [tailwindcss-linux-x64](https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.11/tailwindcss-linux-x64) | `64805b84af4292e043ea6f86d242f191c0ac75359c1a498455dfe6c642afdbab` |
| Tailwind standalone CLI (Linux arm64) | 4.1.11 | [tailwindcss-linux-arm64](https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.11/tailwindcss-linux-arm64) | `0409aa4222969f47fa6f4160fe5387e79bf7269e7afe0e8b22f7532c98e1d314` |
| Tailwind standalone CLI (macOS x64) | 4.1.11 | [tailwindcss-macos-x64](https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.11/tailwindcss-macos-x64) | `76e27326506d10d50e65b751795f0537f9304ecb100abe835ec138c41774f38c` |
| Tailwind standalone CLI (macOS arm64) | 4.1.11 | [tailwindcss-macos-arm64](https://github.com/tailwindlabs/tailwindcss/releases/download/v4.1.11/tailwindcss-macos-arm64) | `f5984b9c005c3e67841c33906c7a7c92e85e405f61e029e9bb62e880dd662e79` |
| daisyUI standalone plugin | 5.0.50 | [daisyui.mjs](https://github.com/saadeghi/daisyui/releases/download/v5.0.50/daisyui.mjs) | `dddf3a71d5d3bf8ddc63ba207e09e9aacc1e66e4396ec828b5366f064d7033cc` |

The compiler and plugin are MIT-licensed; their notices are under
[`licenses/`](licenses/). Build-tool versions and checksums must agree with the
script before either bundle is used.

## Update procedure

1. Choose an exact upstream version and review its release notes.
2. Download the published npm tarball with `npm pack <package>@<version>`.
3. Copy only the minified distribution asset and license into this directory.
4. Include the version in the filename; do not overwrite an old version under an
   ambiguous name.
5. Update the build entry point or owned stylesheet source, then run
   `make frontend-css`.
6. Update the version, origin, and SHA-256 in this file.
7. Run `make check-frontend-css`, `go test ./internal/webapp`, and the complete
   repository verification.

Do not edit vendored distribution files. Mouseion-specific rules belong in the
owned source stylesheets under [`../../styles/`](../../styles/). Never edit the
generated `../app.css` directly.
