package main

import (
	"sync"

	"github.com/gin-gonic/gin"
	chatws "mangahub/internal/websocket"
)

var (
	chatHub  = chatws.NewChatHub()
	chatOnce sync.Once
)

func (s *APIServer) setupChatRoutes() {
	chatOnce.Do(func() {
		go chatHub.Run()
	})

	s.Router.GET("/chat/ws", func(c *gin.Context) {
		userID := c.Query("user_id")
		if userID == "" {
			userID = "guest"
		}

		username := c.Query("username")
		if username == "" {
			username = "guest"
		}

		chatHub.ServeWS(c.Writer, c.Request, userID, username)
	})
}
