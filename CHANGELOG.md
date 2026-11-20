# Changelog

All notable changes are listed here. Format: [Keep a Changelog](https://keepachangelog.com/), versioning: [SemVer](https://semver.org/).

## [0.1.0] - 2026-11

First public release.

### Added
- Public web UI for the HTTP API of Haetae (RPKI relying party), read-only, Korean / English, day / night, mobile friendly:
  - **Status**: health, serial, VRP / ASPA / router key counts, last run.
  - **Explorer**: route origin validation (AS + prefix), AS lookup, prefix lookup (covering / more-specific VRPs, a "no match" tag for VRPs whose maxLength is too short), ASPA path check, certification path (TA → CA → ROA), one ROA card per ROA.
  - **Lists**: all VRPs, ASPAs and router keys with search and numbered paging (with a jump-to-page box).
  - **History / Changes**: VRP count per run, held runs with reasons, per-serial delta (ASPA shown as `customer → providers`).
  - **Operations**: repository state, rejected objects, expiring objects, object search, unsafe VRPs.
  - **Internal** (optional, off by default): summary, SLURM, RTR clients, shown only to requests carrying a secret header added by your reverse proxy after login.
- Config file (`/etc/jangseung/jangseung.conf`, same format as Haetae): Haetae URL, listen address, internal screens, `require_login` (staff-only installs), trusted proxies, default language, cache and request limits. `-check`, `-c`, `-listen`, `-haetae`, `-version`; `systemctl reload` re-reads it.
- Static binary (`CGO_ENABLED=0`, about 9 MB), Go standard library only.
- systemd unit: `Type=notify`, watchdog that asks Jangseung's own page, reload, sandboxing (`systemd-analyze security` about 1.4).
- Security: CSP and other security headers, input validation, cap on new requests to Haetae per second (`upstream_rps`), proxy headers trusted only from `trust_proxy`, constant-time key comparison.
- Documentation: manuals (Korean / English), slides (Korean / English), nginx example.

[0.1.0]: https://github.com/yunsog/jangseung/releases/tag/v0.1.0
