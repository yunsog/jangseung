#!/bin/bash
# 장승 설치 (systemd 가 있는 Linux, root). Go 1.26 이상 필요. 각 단계는 다시 실행해도 안전.
#   sudo ./deploy/install.sh
# Install Jangseung as a systemd service (Go >= 1.26, run as root). Safe to re-run.
set -e
cd "$(dirname "$0")/.."
# 정적 빌드 : libc 를 쓰지 않음 (static binary)
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /usr/local/bin/jangseung.new .
id jangseung >/dev/null 2>&1 || useradd --system --no-create-home --shell /usr/sbin/nologin jangseung
mkdir -p /etc/jangseung
# 설정 : 없을 때만 예제를 복사. 반드시 [haetae] url 을 고칠 것 (아니면 아래 -check 에서 멈춤)
[ -s /etc/jangseung/jangseung.conf ] || install -m 644 deploy/jangseung.conf.example /etc/jangseung/jangseung.conf
/usr/local/bin/jangseung.new -check -c /etc/jangseung/jangseung.conf
mv -f /usr/local/bin/jangseung.new /usr/local/bin/jangseung
cp deploy/jangseung.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable jangseung && systemctl restart jangseung
sleep 2; systemctl is-active jangseung || { journalctl -u jangseung -n 15 --no-pager; exit 1; }
echo "시작함. 확인 : curl -s http://127.0.0.1:8330/jangseung-healthz"
echo "내부 화면을 쓰려면 : openssl rand -hex 24 > /etc/jangseung/internal.key; chgrp jangseung /etc/jangseung/internal.key; chmod 640 /etc/jangseung/internal.key"
echo "  jangseung.conf 의 [internal] enabled = yes, nginx 에 같은 값의 비밀 헤더 (deploy/nginx.conf.example)"
