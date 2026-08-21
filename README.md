# mouseion

A self-hosted reading environment for learning foreign languages at an advanced reading level. It turns books from your own Calibre-Web/OPDS library into vocabulary you can actually study — analyzing texts, surfacing the words worth learning with example sentences, and exporting an Anki deck.

Built around a Go core with a Python NLP service for analysis.

See [the product specification](doc/product.md) and [documentation governance](doc/documentation-governance.md).

## Development

The workspace supports Go 1.24 and Python 3.11. It also requires Protobuf
29.x for regenerating the shared contract. Go dependencies are pinned in
`go.mod`/`go.sum`; Python development dependencies are pinned in
`nlp/requirements-dev.txt`.

```sh
export PATH="$PATH:/usr/local/go/bin:/root/go/bin"
make setup
make gen
make build
make test
make lint
```

`make dev` starts the web-only v1 server at `http://localhost:8080`. The
Python NLP producer is an ingest-time component, not a dependency of the
running web server. PostgreSQL schema changes live in `migrations/` as paired
golang-migrate SQL files.
