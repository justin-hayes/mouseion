# Frontend vendor assets

Mouseion serves HTMX from the embedded static filesystem; its shared application
CSS is maintained in `../app.css`. The application does not require a
browser-time connection to a CDN.

## Pinned assets

| Asset               | Version | Upstream                                   | SHA-256                                                            |
| ------------------- | ------: | ------------------------------------------ | ------------------------------------------------------------------ |
| `htmx-4.0.0.min.js` |   4.0.0 | `htmx.org` npm package, `dist/htmx.min.js` | `e484d9171a9db30a39c8f16e3d709d4137f3211c659f8e6125816635033d593f` |

The corresponding upstream licenses are under [`licenses/`](licenses/).

## CSS build tools

These exact build-time bundles are downloaded by `tools/build-login-css.sh` into
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
5. Update the URL in `internal/webapp/views.templ` and regenerate with
   `templ generate`.
6. Update the version, origin, and SHA-256 in this file.
7. Run `go test ./internal/webapp` and the complete repository verification.

Do not edit vendored distribution files. Mouseion-specific rules belong in
`../app.css`.
