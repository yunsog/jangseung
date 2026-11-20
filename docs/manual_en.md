# Jangseung manual (0.1.0)

Korean version: [manual_ko.md](manual_ko.md)

Jangseung is a read-only web UI for the HTTP API of **Haetae**, an RPKI relying party. It is one static executable that listens on one port and normally runs behind a reverse proxy.

## 1. Requirements
- A running Haetae whose HTTP API the Jangseung host can reach.
  Some screens use API paths that Haetae serves only to its *internal* access class (see §6). Put the Jangseung host in Haetae's internal allow list, or those screens show a notice.
- Linux with systemd (for the provided unit). Go 1.26 or newer to build.

## 2. Build and install
```sh
git clone https://github.com/yunsog/jangseung && cd jangseung
sudo ./deploy/install.sh
```
The script builds a static binary (`CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`), creates the `jangseung` user, installs `/etc/jangseung/jangseung.conf` from the example if missing, **validates it** (`jangseung -check`), installs the unit and starts it. It stops before replacing anything if the config is wrong.

**Before the first start edit `[haetae] url`** in `/etc/jangseung/jangseung.conf`, then `sudo systemctl restart jangseung`.
Check: `curl -s http://127.0.0.1:8330/jangseung-healthz` → `ok`.

Manual build: `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o jangseung .` then `./jangseung -c jangseung.conf`.
Options: `-c <file>`, `-listen <addr>`, `-haetae <url>`, `-check` (validate and exit), `-version`. Options override the file.

## 3. Configuration reference
Format: `[section]`, `key = value`, `#` comments. Unknown keys, duplicate keys and bad values stop start-up. `systemctl reload jangseung` re-reads the file; only `listen` and `timeout` need a restart.

| Key | Default | Meaning |
|---|---|---|
| `[haetae] url` | – (required) | Haetae HTTP API, e.g. `http://127.0.0.1:5002` |
| `[haetae] timeout` | `20s` | timeout of one Haetae API call (restart) |
| `[listen] addr` | `127.0.0.1:8330` | `127.0.0.1:…` = this machine only; `0.0.0.0:…` = everyone (restart) |
| `[internal] enabled` | `no` | enable login-only screens (summary, SLURM, RTR clients) |
| `[internal] key_file` | – | file with the secret (16+ chars). Needed if `enabled` or `require_login` is yes |
| `[internal] header` | `X-Jangseung-Key` | header name your proxy sets |
| `[web] base_path` | `auto` | `auto` = trust `X-Forwarded-Prefix` from `trust_proxy`; `/name` = fixed; empty = none |
| `[web] trust_proxy` | `127.0.0.1, ::1` | addresses/ranges whose `X-Forwarded-*` headers are trusted |
| `[web] lang` | `auto` | `auto` = browser language, or `ko` / `en` |
| `[web] require_login` | `no` | every page needs the secret header (staff-only installs) |
| `[cache] ttl` | `60s` | how long a Haetae answer is reused |
| `[cache] refresh` | `5m` | minimum interval between full list downloads |
| `[cache] max_rows` | `500` | table row limit per screen (10–100000) |
| `[cache] upstream_rps` | `20` | cap on *new* (uncached) requests to Haetae per second |

## 4. Behind nginx (recommended)
See `deploy/nginx.conf.example`. Jangseung listens on `127.0.0.1:8330`; nginx adds TLS and rate limits.
- Serve under a sub-path (e.g. `/rpki/`): `proxy_pass http://127.0.0.1:8330/;` plus `proxy_set_header X-Forwarded-Prefix /rpki;` and `base_path = auto`.
- Always clear the secret header on public locations: `proxy_set_header X-Jangseung-Key "";`.

### Login-only screens
Jangseung does no authentication. Your proxy checks the login (e.g. `auth_request`) and, only for logged-in requests, adds `X-Jangseung-Key: <contents of key_file>`.
- Internal screens only: set `[internal] enabled = yes` and add the header on `location ^~ /internal/`. The "Internal" menu entry appears in logged-in browsers only.
- Whole site: set `[web] require_login = yes` and add the header on `location /`. Requests without it get `403`; `/jangseung-healthz` stays open for the watchdog.
Create the key: `openssl rand -hex 24 | sudo tee /etc/jangseung/internal.key; sudo chgrp jangseung /etc/jangseung/internal.key; sudo chmod 640 /etc/jangseung/internal.key`.

### Listening on port 80 directly
Not recommended (no TLS, no rate limits). If you must: `addr = 0.0.0.0:80`, and in the systemd unit set `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` and `AmbientCapabilities=CAP_NET_BIND_SERVICE` (the unit drops all capabilities by default), and open the firewall.

## 5. Operation
- Logs: `journalctl -u jangseung`. Reload config: `sudo systemctl reload jangseung`.
- systemd: `Type=notify`; the watchdog (3 min) pings `/jangseung-healthz` itself and restarts a hung process.
- Caching: answers are reused for `ttl`; the full VRP/ASPA/router key list is downloaded at start-up and updated from deltas. If Haetae is down, the last cache is shown with its data time.
- Load on Haetae stays roughly constant; new uncached requests are capped by `upstream_rps` (excess: last cache or a "busy" notice).
- Troubleshooting:
  - *"Haetae is not responding"* – check `[haetae] url`, firewall, and that the host is allowed by Haetae.
  - *Operations / History screens empty or unavailable* – the host is probably not in Haetae's internal access class.
  - *Menu shows no "Internal"* – not logged in, `enabled = no`, or the proxy does not add the header on `/internal/`.
  - *Wrong links under a sub-path* – check `X-Forwarded-Prefix` and that the proxy address is in `trust_proxy`.

## 6. Haetae API paths used
| Screen | Paths |
|---|---|
| all (footer) | `/healthz`, `/api/v1/status` |
| Explorer | `/api/v1/validity/AS<n>/<prefix>`, `/api/v1/prefix/<prefix>`, `/api/v1/as/<n>`, `/api/v1/aspa/verify`, `/api/v1/objects`, `/api/v1/ca` |
| Lists | `/json-delta` (full list once, then deltas) |
| History / Changes | `/api/v1/history`, `/json-delta?session=…&serial=…&to=…` |
| Operations | `/repos.json`, `/rejects.txt`, `/api/v1/expiring`, `/api/v1/objects`, `/api/v1/unsafe` |
| Internal | `/api/v1/slurm`, `/api/v1/status` (RTR clients), the above for the summary |

Jangseung only sends `GET` requests. See Haetae's API documentation for which paths are available to which access class.

## 7. Security notes
See [SECURITY.md](../SECURITY.md). In short: run behind a proxy on `127.0.0.1`; protect the key file; remember that listening on `0.0.0.0` exposes unauthenticated, unthrottled HTTP.
