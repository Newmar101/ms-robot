package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HandleGetEndpoints 获取所有端点，与 GET /api/devices 一致返回 { endpoints, count }
// GET /api/endpoints
func (s *Server) HandleGetEndpoints(c *gin.Context) {
	endpoints := s.endpointManager.GetEndpointsWithInfo()
	for _, ep := range endpoints {
		if key, ok := ep["id"].(string); ok {
			ep["status"] = s.unifiedDeviceManager.GetEndpointStatus(key)
		}
	}
	c.JSON(http.StatusOK, gin.H{"endpoints": endpoints, "count": len(endpoints), "endpointsMutable": s.endpointsMutable})
}

// HandleAddEndpoint 添加端点
// POST /api/endpoints
func (s *Server) HandleAddEndpoint(c *gin.Context) {
	if !s.endpointsMutable {
		c.JSON(http.StatusForbidden, gin.H{"error": "未开放此功能"})
		return
	}
	var req struct {
		Endpoint string `json:"endpoint" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	addedKey, err := s.endpointManager.AddEndpoint(req.Endpoint, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s.unifiedDeviceManager.OnEndpointAdded(addedKey)
	if mgr, ok := s.unifiedDeviceManager.GetManagerForEndpoint(addedKey); ok && s.wsHub != nil {
		mgr.SetWSHub(s.wsHub)
	}
	if s.wsHub != nil {
		s.wsHub.BroadcastEndpointsChanged()
	}
	c.JSON(http.StatusOK, gin.H{"message": "端点添加成功", "id": addedKey})
}

// HandleRemoveEndpoint 删除端点
// DELETE /api/endpoints/:endpoint
func (s *Server) HandleRemoveEndpoint(c *gin.Context) {
	if !s.endpointsMutable {
		c.JSON(http.StatusForbidden, gin.H{"error": "未开放此功能"})
		return
	}
	endpointParam := c.Param("endpoint")

	removedKey, err := s.endpointManager.RemoveEndpoint(endpointParam)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	s.unifiedDeviceManager.OnEndpointRemoved(removedKey)
	if s.wsHub != nil {
		s.wsHub.BroadcastEndpointsChanged()
	}
	c.JSON(http.StatusOK, gin.H{"message": "端点删除成功"})
}
