package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"execution-engine/internal/engine"
)

var Upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func RegisterSessionWS(r *gin.Engine, eng engine.Engine) {
	r.GET("/ws/session/:id", func(c *gin.Context) {
		id := c.Param("id")

		sess, ok := eng.GetSession(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
			return
		}

		conn, err := Upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		conn.SetCloseHandler(func(code int, text string) error {
			log.Printf("WebSocket closed with code %d and text: %s", code, text)
			sess.DetachWS()
			log.Printf("WebSocket detached from session %s. Active connections: %d", sess.ID, sess.ActiveWSCount())
			return nil
		})

		sess.AttachWS()
		log.Printf("WS attached to %s (active=%d)", sess.ID, sess.ActiveWSCount())

		// stdin
		go func() {
			for {
				var msg struct {
					Type string `json:"type"`
					Data string `json:"data"`
				}

				if err := conn.ReadJSON(&msg); err != nil {
					return
				}

				if msg.Type == "input" {
					_ = sess.WriteInput(msg.Data)
				}
			}
		}()

		lastStdout, lastStderr := 0, 0
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-sess.Done():
				sendDiff(conn, "stdout", sess.Stdout.String(), &lastStdout)
				sendDiff(conn, "stderr", sess.Stderr.String(), &lastStderr)

				conn.WriteJSON(gin.H{
					"type":  "state",
					"state": sess.State,
				})
				log.Printf("Session %s finished", sess.ID)
				return

			case <-ticker.C:
				if err := sendDiff(conn, "stdout", sess.Stdout.String(), &lastStdout); err != nil {
					return
				}
				if err := sendDiff(conn, "stderr", sess.Stderr.String(), &lastStderr); err != nil {
					return
				}
			}
		}
	})
}

func sendDiff(conn *websocket.Conn, t string, data string, last *int) error {
	if len(data) > *last {
		chunk := data[*last:]
		*last = len(data)
		return conn.WriteJSON(gin.H{"type": t, "data": chunk})
	}
	return nil
}
