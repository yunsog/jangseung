# 장승 사용 설명서 (0.1.0)

English: [manual_en.md](manual_en.md)

장승은 RPKI 검증기 **해태 (Haetae)** 의 HTTP API 를 읽어 보여 주는 읽기 전용 웹 화면입니다. 실행 파일 하나가 포트 하나를 듣고, 보통 역프록시 뒤에서 돕니다.

## 1. 준비
- 장승이 있는 서버에서 닿을 수 있는 해태 HTTP API.
  일부 화면은 해태가 *내부* 접근 단계에만 열어 주는 경로를 씁니다 (6 절). 장승 서버를 해태의 내부 허용 목록에 넣어야 하고, 아니면 그 화면에 안내가 나옵니다.
- systemd 가 있는 Linux (제공하는 unit 사용 시). 소스로 빌드하려면 Go 1.26 이상.

## 2. 빌드와 설치
```sh
git clone https://github.com/yunsog/jangseung && cd jangseung
sudo ./deploy/install.sh
```
스크립트가 정적 실행 파일을 빌드하고 (`CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`), `jangseung` 계정을 만들고, `/etc/jangseung/jangseung.conf` 가 없으면 예제를 복사하고, **설정을 검사한 뒤** (`jangseung -check`) unit 을 설치해 시작합니다. 설정이 틀리면 아무것도 바꾸기 전에 멈춥니다.

**처음 시작 전에 `/etc/jangseung/jangseung.conf` 의 `[haetae] url` 을 고치고** `sudo systemctl restart jangseung` 하세요.
확인 : `curl -s http://127.0.0.1:8330/jangseung-healthz` → `ok`.

직접 빌드 : `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o jangseung .` 뒤 `./jangseung -c jangseung.conf`.
옵션 : `-c <파일>`, `-listen <주소>`, `-haetae <url>`, `-check` (검사만), `-version`. 옵션이 파일보다 우선합니다.

## 3. 설정 키
형식 : `[구역]`, `키 = 값`, `#` 주석. 모르는 키, 같은 키 두 번, 잘못된 값이 있으면 시작하지 않습니다. `systemctl reload jangseung` 으로 다시 읽고, `listen` 과 `timeout` 만 재시작이 필요합니다.

| 키 | 기본값 | 뜻 |
|---|---|---|
| `[haetae] url` | – (필수) | 해태 HTTP API, 예 `http://127.0.0.1:5002` |
| `[haetae] timeout` | `20s` | 해태 API 한 번의 제한 시간 (재시작) |
| `[listen] addr` | `127.0.0.1:8330` | `127.0.0.1:…` = 이 컴퓨터만, `0.0.0.0:…` = 인터넷 전체 (재시작) |
| `[internal] enabled` | `no` | 로그인 뒤 화면 (요약, SLURM, RTR 라우터) 켜기 |
| `[internal] key_file` | – | 비밀 값이 든 파일 (16 자 이상). `enabled` 나 `require_login` 이 yes 면 필요 |
| `[internal] header` | `X-Jangseung-Key` | 프록시가 붙이는 헤더 이름 |
| `[web] base_path` | `auto` | `auto` = `trust_proxy` 에서 온 `X-Forwarded-Prefix` 를 믿음, `/이름` = 고정, 비움 = 없음 |
| `[web] trust_proxy` | `127.0.0.1, ::1` | `X-Forwarded-*` 헤더를 믿을 주소 / 범위 |
| `[web] lang` | `auto` | `auto` = 브라우저 언어, 또는 `ko` / `en` |
| `[web] require_login` | `no` | 모든 화면이 비밀 헤더를 요구 (사내 전용) |
| `[cache] ttl` | `60s` | 해태 응답을 다시 쓰는 시간 |
| `[cache] refresh` | `5m` | 전체 목록을 다시 받는 최소 간격 |
| `[cache] max_rows` | `500` | 화면 하나의 표 줄 수 상한 (10 ~ 100000) |
| `[cache] upstream_rps` | `20` | 캐시에 없어 해태로 *새로* 가는 요청의 초당 상한 |

## 4. nginx 뒤에서 (권장)
`deploy/nginx.conf.example` 참고. 장승은 `127.0.0.1:8330` 을 듣고 nginx 가 TLS 와 접속 제한을 맡습니다.
- 아래 경로 (예 `/rpki/`) 로 줄 때 : `proxy_pass http://127.0.0.1:8330/;` 와 `proxy_set_header X-Forwarded-Prefix /rpki;`, 설정은 `base_path = auto`.
- 공개 location 에서는 항상 비밀 헤더를 지울 것 : `proxy_set_header X-Jangseung-Key "";`.

### 로그인 뒤 화면
장승은 인증을 하지 않습니다. 프록시가 로그인을 확인하고 (예 `auth_request`), 로그인한 요청에만 `X-Jangseung-Key: <key_file 의 값>` 을 붙입니다.
- 내부 화면만 : `[internal] enabled = yes`, `location ^~ /internal/` 에 헤더. "내부" 메뉴는 로그인한 브라우저에만 보입니다.
- 사이트 전체 : `[web] require_login = yes`, `location /` 에 헤더. 헤더가 없으면 `403`, `/jangseung-healthz` 만 워치독용으로 열려 있습니다.
키 만들기 : `openssl rand -hex 24 | sudo tee /etc/jangseung/internal.key; sudo chgrp jangseung /etc/jangseung/internal.key; sudo chmod 640 /etc/jangseung/internal.key`.

### 80 번 포트로 직접 열기
권하지 않습니다 (TLS 도 접속 제한도 없음). 꼭 필요하면 `addr = 0.0.0.0:80`, systemd unit 에 `CapabilityBoundingSet=CAP_NET_BIND_SERVICE` 와 `AmbientCapabilities=CAP_NET_BIND_SERVICE` (기본 unit 은 권한을 모두 뺐음), 방화벽 열기.

## 5. 운영
- 로그 : `journalctl -u jangseung`. 설정 다시 읽기 : `sudo systemctl reload jangseung`.
- systemd : `Type=notify`. 워치독 (3 분) 이 `/jangseung-healthz` 를 스스로 확인해 멈춘 프로세스를 다시 띄웁니다.
- 캐시 : 해태 응답은 `ttl` 동안 다시 씁니다. 전체 VRP / ASPA / 라우터 키 목록은 시작할 때 받아 두고 변경분으로 갱신합니다. 해태가 멈추면 마지막 캐시와 데이터 시각을 보여 줍니다.
- 해태가 받는 양은 거의 일정합니다. 캐시에 없는 새 요청은 `upstream_rps` 로 제한하고, 넘으면 마지막 캐시 또는 "바쁨" 안내를 보여 줍니다.
- 문제 해결 :
  - *"해태 응답 없음"* – `[haetae] url`, 방화벽, 해태의 허용 대역을 확인.
  - *운영 / 추이 화면이 비어 있음* – 이 서버가 해태의 내부 접근 단계가 아닐 가능성이 큼.
  - *"내부" 메뉴가 안 보임* – 로그인 안 됨, `enabled = no`, 또는 프록시가 `/internal/` 에 헤더를 안 붙임.
  - *아래 경로에서 링크가 틀림* – `X-Forwarded-Prefix` 와 프록시 주소가 `trust_proxy` 에 있는지 확인.

## 6. 쓰는 해태 API 경로
| 화면 | 경로 |
|---|---|
| 모든 화면 (바닥글) | `/healthz`, `/api/v1/status` |
| 탐색기 | `/api/v1/validity/AS<n>/<prefix>`, `/api/v1/prefix/<prefix>`, `/api/v1/as/<n>`, `/api/v1/aspa/verify`, `/api/v1/objects`, `/api/v1/ca` |
| 목록 | `/json-delta` (전체는 한 번, 그 뒤 변경분) |
| 추이 / 변경 | `/api/v1/history`, `/json-delta?session=…&serial=…&to=…` |
| 운영 | `/repos.json`, `/rejects.txt`, `/api/v1/expiring`, `/api/v1/objects`, `/api/v1/unsafe` |
| 내부 | `/api/v1/slurm`, `/api/v1/status` (RTR 라우터), 요약은 위 경로들 |

장승은 `GET` 만 보냅니다. 어느 경로가 어느 접근 단계에 열리는지는 해태 API 문서를 보세요.

## 7. 보안
[SECURITY.md](../SECURITY.md) 참고. 요약 : 프록시 뒤에서 `127.0.0.1` 로 돌리고, 키 파일을 지키고, `0.0.0.0` 은 인증도 접속 제한도 없는 HTTP 가 열린다는 점을 기억하세요.
