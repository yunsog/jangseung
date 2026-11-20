# Contributing to Jangseung / 기여 안내

Thanks for your interest! 관심 가져 주셔서 감사합니다.

## Ground rules
- **Go standard library only.** No third-party modules, please (the static, dependency-free binary is a feature).
- **Read-only.** Jangseung never writes to Haetae.
- Keep functions short with a comment saying *why*. Comments may be in Korean or English.
- Public screens must show only data that is public in RPKI. Anything operator-only belongs behind the `[internal]` gate.

## Development
```sh
go vet ./... && go test ./...
gofmt -l .                       # must print nothing
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o jangseung .
./jangseung -c deploy/jangseung.conf.example   # edit [haetae] url first
```
You need a reachable Haetae HTTP API to see real data. The tests do not need one.

## Pull requests
1. Open an issue first for anything bigger than a small fix.
2. One topic per PR; add or update tests when behavior changes.
3. Update `CHANGELOG.md` (Unreleased section) and the manuals if you change user-visible behavior or config keys.
4. New text in the UI needs both Korean and English entries (`i18n.go`).
5. Do not add secrets, internal host names or real credentials to examples (use `example.org`, `192.0.2.0/24`, `AS64496`).

## Reporting security problems
Do **not** open a public issue. See [SECURITY.md](SECURITY.md).

## License
By contributing you agree that your contribution is licensed under the Apache License 2.0 (see [LICENSE](LICENSE)).
