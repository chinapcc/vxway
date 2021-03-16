package utils

import (
	"fmt"
	"strings"

	"github.com/valyala/fasthttp"
)

const (
	// MinAddrFormat min addr format
	MinAddrFormat = "000000000000000000000"
	// MaxAddrFormat max addr format
	MaxAddrFormat = "255.255.255.255:99999"
)

// GetAddrFormat 返回用于排序的addr格式，左填充为0
func GetAddrFormat(addr string) string {
	return fmt.Sprintf("%021s", addr)
}

// GetAddrNextFormat 返回排序的下一个addr格式，左填充为0
func GetAddrNextFormat(addr string) string {
	return fmt.Sprintf("%s%c", addr[:len(addr)-1], addr[len(addr)-1]+1)
}

// ClientIP 返回真实的客户端IP
func ClientIP(ctx *fasthttp.RequestCtx) string {
	clientIP := string(ctx.Request.Header.Peek("X-Forwarded-For"))
	if index := strings.IndexByte(clientIP, ','); index >= 0 {
		clientIP = clientIP[0:index]
	}
	clientIP = strings.TrimSpace(clientIP)
	if len(clientIP) > 0 {
		return clientIP
	}
	clientIP = strings.TrimSpace(string(ctx.Request.Header.Peek("X-Real-Ip")))
	if len(clientIP) > 0 {
		return clientIP
	}
	return ctx.RemoteIP().String()
}

