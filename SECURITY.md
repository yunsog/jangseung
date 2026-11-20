# Security policy / 보안 정책

## Supported versions
| Version | Supported |
|---|---|
| 0.1.x | yes |

## Reporting a vulnerability
Please **do not** open a public issue for security problems.
Use GitHub's private reporting: the repository's **Security** tab → **Report a vulnerability**.
Include the version (`jangseung -version`), your config (without secrets), and steps to reproduce.
보안 문제는 공개 이슈로 올리지 말고, 저장소의 **Security** 탭 → **Report a vulnerability** 로 비공개 신고해 주세요.

We aim to acknowledge a report within a few days and to publish a fix and a note in `CHANGELOG.md` once users can update.

## Threat model in short
- Jangseung is **read-only** toward Haetae and meant to run **behind a reverse proxy** (TLS, rate limits, login), listening on `127.0.0.1`.
- It does **no authentication itself**. Login-only screens (`[internal]`, `[web] require_login`) rely on a secret header that only your proxy can add. Treat the key file like a password (mode 640, dedicated group), and make the proxy strip that header from outside requests.
- Listening on `0.0.0.0` exposes plain HTTP with no rate limiting. Do that only on a network you control.
- Everything shown on public screens is derived from public RPKI data; router addresses and local policy (SLURM) are internal-only.
