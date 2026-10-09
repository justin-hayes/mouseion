# Contributing

Mouseion is a personal project, but issues and pull requests are welcome.

- **Bugs and ideas:** open an issue describing the behavior you saw or want.
  For anything security-related, follow [SECURITY.md](SECURITY.md) instead.
- **Before a large change:** open an issue first. Consequential product or
  architecture changes are settled in a feature document or an
  [ADR](doc/adr/) before implementation
  ([documentation governance](doc/documentation-governance.md)).
- **Setup and tests:** see the [development guide](doc/development.md). Run
  `make lint` and `make test` before opening a pull request, and
  `make test-integration` and `make browser-smoke` when your change touches
  persistence or the interface.
- **Style:** use [Conventional Commits](https://www.conventionalcommits.org/)
  (`type(scope): imperative summary`) and the domain vocabulary in
  [`CONTEXT.md`](CONTEXT.md). Regenerate generated code rather than editing it.
- **Review:** paths listed in [`.github/CODEOWNERS`](.github/CODEOWNERS) need
  maintainer review.

By contributing, you agree that your contributions are licensed under the
project's [AGPL-3.0-only license](LICENSE).
