package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"execution-engine/internal/engine"
	"execution-engine/internal/executor"
	"execution-engine/internal/modules"

	"github.com/gin-contrib/cors"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // handle auth later
	},
}

func main() {
	// ---- bootstrap docker executor ----
	dockerExec, err := executor.NewDockerExecutor()
	if err != nil {
		panic(err)
	}

	// ---- engine ----
	eng := engine.New(dockerExec)

	r := gin.Default()

	// 🔥 CORS: allow from anywhere (DEV MODE)
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: false,
		MaxAge:           12 * time.Hour,
	}))

	// ------------------------------------------------
	// Create Session (HTTP)
	// ------------------------------------------------
	r.POST("/session", func(c *gin.Context) {
		var req modules.ExecuteRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid json",
			})
			return
		}

		sess, err := eng.StartSession(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}

		log.Printf("Session %s created", sess.ID)

		c.JSON(http.StatusOK, gin.H{
			"sessionId": sess.ID,
		})
	})

	// ------------------------------------------------
	// WebSocket: Interactive Execution
	// ------------------------------------------------
	r.GET("/ws/session/:id", func(c *gin.Context) {
		id := c.Param("id")

		sess, ok := eng.GetSession(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "session not found",
			})
			return
		}

		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
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

		// attach WS
		sess.AttachWS()
		log.Printf("WebSocket attached to session %s. Active connections: %d", sess.ID, sess.ActiveWSCount())

		// -------------------------------
		// Client → stdin
		// -------------------------------
		go func() {
			for {
				var msg struct {
					Type string `json:"type"`
					Data string `json:"data"`
				}

				if err := conn.ReadJSON(&msg); err != nil {
					return // handled by defer
				}

				if msg.Type == "input" {
					_ = sess.WriteInput(msg.Data)
				}
			}
		}()

		// ---------------- stdout/stderr (server → client) ----------------
		lastStdout := 0
		lastStderr := 0

		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-sess.Done():
				// flush remaining output
				_ = sendDiff(conn, "stdout", sess.Stdout.String(), &lastStdout)
				_ = sendDiff(conn, "stderr", sess.Stderr.String(), &lastStderr)

				_ = conn.WriteJSON(gin.H{
					"type":  "state",
					"state": sess.State,
				})
				log.Printf("Session %s finished with state %s", sess.ID, sess.State)
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

	// ------------------------------------------------
	// Start server
	// ------------------------------------------------
	log.Println("Server started on :8080")
	if err := r.Run(":8080"); err != nil {
		panic(err)
	}
}

func sendDiff(conn *websocket.Conn, t string, data string, last *int) error {
	if len(data) > *last {
		chunk := data[*last:]
		*last = len(data)

		return conn.WriteJSON(gin.H{
			"type": t,
			"data": chunk,
		})
	}
	return nil
}
