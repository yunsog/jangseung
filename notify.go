package main

// systemd 연동 (Type=notify + WatchdogSec). Go 표준만 : 알림은 $NOTIFY_SOCKET (유닉스 소켓) 에 문자열 한 줄.
// systemd 밖에서 (환경 변수 없음) 돌리면 아무 일도 하지 않음.

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// sdNotify : "READY=1" / "WATCHDOG=1" / "STOPPING=1" / "RELOADING=1" 등을 systemd 에 보냄
func sdNotify(msg string) {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return
	}
	if strings.HasPrefix(sock, "@") { // 추상 소켓
		sock = "\x00" + sock[1:]
	}
	conn, err := net.Dial("unixgram", sock)
	if err != nil {
		log.Println("systemd 알림 실패 :", err)
		return
	}
	defer conn.Close()
	if _, err := conn.Write([]byte(msg)); err != nil {
		log.Println("systemd 알림 실패 :", err)
	}
}

// watchdog : WatchdogSec 의 1/3 마다 "내 화면이 실제로 답하는지" 를 자기 자신에게 물어보고, 답할 때만 WATCHDOG=1.
// (프로세스만 살아 있고 서버가 막힌 경우를 잡음). 워치독이 꺼져 있으면 (WATCHDOG_USEC 없음) 아무 일도 안 함
func watchdog(listen string) {
	us, err := strconv.ParseInt(os.Getenv("WATCHDOG_USEC"), 10, 64)
	if err != nil || us <= 0 {
		return
	}
	every := time.Duration(us) * time.Microsecond / 3
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return
	}
	if host == "" || host == "0.0.0.0" || host == "::" { // 모든 주소를 듣는 중 -> 내 쪽 loopback 으로 물음
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port) + "/jangseung-healthz"
	hc := &http.Client{Timeout: every / 2}
	for range time.Tick(every) {
		resp, err := hc.Get(url)
		if err != nil {
			log.Println("워치독 : 자기 확인 실패, 알림 건너뜀 :", err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == 200 {
			sdNotify("WATCHDOG=1")
		} else {
			log.Println("워치독 : 자기 확인 응답", resp.Status)
		}
	}
}

// notifyStatus : systemctl status 에 보이는 한 줄
func notifyStatus(format string, a ...any) { sdNotify("STATUS=" + fmt.Sprintf(format, a...)) }
