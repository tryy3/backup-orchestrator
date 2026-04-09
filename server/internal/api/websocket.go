package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"github.com/tryy3/backup-orchestrator/server/internal/events"
)

// websocketHandler upgrades an HTTP connection to WebSocket and streams events from the Hub.
func websocketHandler(hub *events.Hub) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			// Allow connections from the Vite dev server and any origin for now.
			// Auth can be layered on later.
			InsecureSkipVerify: true,
		})
		if err != nil {
			log.Printf("WebSocket accept error: %v", err)
			return
		}
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "server closing") }()

		clientID, eventCh := hub.Register()
		defer hub.Unregister(clientID)

		ctx := r.Context()

		// Read pump: handles client messages (keepalive). We discard them but must
		// read to process control frames (ping/pong/close).
		go func() {
			for {
				_, _, err := conn.Read(ctx)
				if err != nil {
					return
				}
			}
		}()

		// Write pump: send events from the hub to the WebSocket client.
		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-eventCh:
				if !ok {
					// Hub closed the channel (unregistered).
					return
				}
				writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err := conn.Write(writeCtx, websocket.MessageText, data)
				cancel()
				if err != nil {
					log.Printf("WebSocket write error for client %s: %v", clientID, err)
					return
				}
			}
		}
	}
}
