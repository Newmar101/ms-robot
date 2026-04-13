package listenutil

import (
	"context"
	"net"
	"strings"
	"unicode"
)

// ParseListenURL 解析监听 URL，返回 (network, addr)。
// 支持：tcp://:20605、tcp://0.0.0.0:20605、unix:///path/to/sock；
// 若不含 "://" 则视为 tcp，addr 原样；若为纯数字则补成 ":端口"。
func ParseListenURL(url string) (network, addr string) {
	parts := strings.SplitN(url, "://", 2)
	if len(parts) == 1 {
		addr = strings.TrimSpace(parts[0])
		if addr != "" && isDigitsOnly(addr) {
			addr = ":" + addr
		}
		return "tcp", addr
	}
	network = strings.ToLower(strings.TrimSpace(parts[0]))
	addr = strings.TrimSpace(parts[1])
	if network == "tcp" && addr != "" && isDigitsOnly(addr) {
		addr = ":" + addr
	}
	return network, addr
}

func isDigitsOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// Listen 按 URL 创建监听器，支持 tcp 与 unix。
func Listen(ctx context.Context, listenURL string) (net.Listener, error) {
	network, addr := ParseListenURL(listenURL)
	lc := &net.ListenConfig{}
	return lc.Listen(ctx, network, addr)
}
