# Frontend vendor assets

Mouseion serves its foundational CSS and HTMX runtime from the embedded static
filesystem. The application does not require a browser-time connection to a CDN.

## Pinned assets

| Asset                | Version | Upstream                                        | SHA-256                                                            |
| -------------------- | ------: | ----------------------------------------------- | ------------------------------------------------------------------ |
| `pico-2.1.1.min.css` |   2.1.1 | `@picocss/pico` npm package, `css/pico.min.css` | `fbc9a63fc9fc9f72d12fd7fc9806e11fa9f77ae4f9cad146b27003a1119ba3db` |
| `htmx-2.0.7.min.js`  |   2.0.7 | `htmx.org` npm package, `dist/htmx.min.js`      | `60231ae6ba9db3825eb15a261122d5f55921c4d53b66bf637dc18b4ee27c79f9` |

The corresponding upstream licenses are under [`licenses/`](licenses/).

## Update procedure

1. Choose an exact upstream version and review its release notes.
2. Download the published npm tarball with `npm pack <package>@<version>`.
3. Copy only the minified distribution asset and license into this directory.
4. Include the version in the filename; do not overwrite an old version under an
   ambiguous name.
5. Update the URLs in `internal/webapp/views.templ`.
6. Regenerate `views_templ.go` with `templ generate`.
7. Update the version, origin, and SHA-256 in this file.
8. Run `go test ./internal/webapp` and the complete repository verification.

Do not edit vendored distribution files. Mouseion-specific rules belong in
`../app.css`.
