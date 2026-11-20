# Jangseung (장승) 0.1.0

A small, dependency-free web front end for the HTTP API of **[Haetae](https://github.com/yunsog/haetae)**, an RPKI relying party.
Jangseung shows what the RP knows — VRPs, ASPAs, router keys, route origin validation, ASPA path checks,
certification paths, history and repository health — in a public, read-only web UI (Korean / English, day / night).

장승은 RPKI 검증기 **해태 (Haetae)** 의 HTTP API 를 읽어 보여 주는 읽기 전용 웹 화면입니다. Go 표준 라이브러리만 쓰고, 정적 실행 파일 하나로 동작합니다.

```
browser ──► nginx (TLS, rate limit, optional login) ──► jangseung 127.0.0.1:8330 ──► Haetae HTTP API
```

Jangseung is deliberately a **separate process** from the RP: web traffic (bots, reload storms) never reaches the
validator, a bug in the UI cannot affect RTR, and Jangseung caches answers per Haetae serial, so Haetae sees
roughly constant load however many visitors there are.

## Screens

| Public (no login) | |
|---|---|
| Status | health, serial, VRP / ASPA / router key counts, last run |
| Explorer | route validity (`AS` + prefix), AS lookup, prefix lookup (covering / more specific), ASPA path check, certification path (TA → CA → ROA) |
| Lists | all VRPs, ASPAs, router keys with search and paging |
| History / Changes | VRP count over time, held runs and reasons, what changed in one run (ASPA as `customer → providers`) |
| Operations | repository state, rejected objects, expiring objects, object search, unsafe VRPs |

| Internal (optional, behind your own login) | |
|---|---|
| Summary, SLURM, RTR clients | operator-only details (router addresses, local policy) |

Documentation: [manual (English)](docs/manual_en.md) · [사용 설명서 (한국어)](docs/manual_ko.md) · [CHANGELOG](CHANGELOG.md)
Slides: [English](docs/jangseung_v0.1_en.pdf) · [한국어](docs/jangseung_v0.1_ko.pdf)

## Requirements

* A running Haetae with its HTTP API reachable from the Jangseung host. Use an address Haetae allows as *internal*
  if you want the Operations / Internal screens (Haetae hides those APIs from its public access class).
* Go 1.26 or newer to build. No other dependencies.

## Build and run

```sh
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o jangseung .   # static, about 9 MB
cp deploy/jangseung.conf.example jangseung.conf                    # edit [haetae] url
./jangseung -check -c jangseung.conf                               # validate the file only
./jangseung -c jangseung.conf                                      # listens on 127.0.0.1:8330 by default
```

Command line: `-c <file>`, `-listen <addr>`, `-haetae <url>`, `-check`, `-version`. Options override the file.
Without `-c`, `/etc/jangseung/jangseung.conf` is read if it exists.

## Configuration

`deploy/jangseung.conf.example` documents every key. Same style as Haetae: `[section]`, `key = value`, `#` comments.
Unknown keys, duplicate keys and bad values stop the start-up (no silent typos).

| Section | Key | Meaning |
|---|---|---|
| `[haetae]` | `url` (required), `timeout` | Haetae HTTP API |
| `[listen]` | `addr` | `127.0.0.1:8330` = this machine only (default). `0.0.0.0:8330` = everyone (no TLS, no rate limit: firewall it) |
| `[internal]` | `enabled`, `key_file`, `header` | login-only screens, off by default |
| `[web]` | `base_path`, `trust_proxy`, `lang` | serve under a sub-path, which proxies to trust, default language |
| `[cache]` | `ttl`, `refresh`, `max_rows`, `upstream_rps` | caching of Haetae answers, table size limit, cap on new requests to Haetae per second |

`systemctl reload jangseung` re-reads the file (`listen` and `timeout` need a restart).

## systemd

```sh
sudo ./deploy/install.sh        # builds, creates the user, installs the unit, starts it
```

`deploy/jangseung.service` is `Type=notify` with a watchdog: Jangseung asks its own `/jangseung-healthz` and only then
tells systemd it is alive, so a hung (not crashed) server is restarted. The unit is hardened
(`systemd-analyze security` ≈ 1.4) because Jangseung only needs sockets.

## Internal screens and login

Jangseung does **no authentication itself**. Put a reverse proxy in front (`deploy/nginx.conf.example`):
after the proxy has checked the visitor's login (e.g. `auth_request`), it adds a secret header
(`X-Jangseung-Key: <contents of key_file>`) on `/internal/` requests only. Jangseung answers `/internal/` only when
the header equals the key, so reaching the port directly never opens it. On public requests the proxy clears the header.
The "Internal" menu entry stays hidden until the browser can fetch `/internal/ping`.

`X-Forwarded-Prefix` (sub-path hosting) is trusted only from addresses in `trust_proxy`.

## Private (login-only) installs

Want to run it only for your own staff? Set `[web] require_login = yes` (and a `key_file`): every page then needs the secret header that
your reverse proxy adds after checking the login (nginx: put `auth_request` and the `proxy_set_header` in `location /`, see the comments in
`deploy/nginx.conf.example`). Requests without the header get `403`. Only `/jangseung-healthz` stays open for the systemd watchdog.
Jangseung never checks passwords itself — keep it on `127.0.0.1` behind the proxy.

## Security notes

* Read-only: Jangseung never writes to Haetae. All input is validated (ASN, prefix, URI); output is escaped by `html/template`.
* Everything Jangseung publishes is derived from public RPKI data, except the Internal screens.
* Do not expose it directly without a proxy unless you accept no TLS and no rate limiting.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
