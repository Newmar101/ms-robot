package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	gorillaWS "github.com/gorilla/websocket"
)

var upgrader = gorillaWS.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许所有来源（生产环境应限制）
	},
}

// handleWebSocket WebSocket连接。device_udid 做 trim 并尽量解析为与其它接口一致的 apiUdid（serial 或 serial@endpointId），解析失败则用 trim 后的原值。
func (s *Server) handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	raw := strings.TrimSpace(c.Query("device_udid"))
	deviceUDID := raw
	if raw != "" {
		if resolved, err := s.resolveOne(raw); err == nil {
			deviceUDID = apiUdidFromResolved(resolved)
		}
	}
	s.wsHub.RegisterClient(conn, deviceUDID)
}
