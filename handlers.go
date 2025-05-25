package main

import (
	"log"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// SignalMessage defines the structure for signaling messages.
type SignalMessage struct {
	Type    string      `json:"type"`              // e.g., "offer", "answer", "candidate", "new-peer", "peer-left"
	Payload interface{} `json:"payload,omitempty"` // SDP data, ICE candidate, etc.
	Target  string      `json:"target,omitempty"`  // Specific peerID for targeted messages
	Sender  string      `json:"sender,omitempty"`  // peerID of the message originator
	RoomID  string      `json:"roomId,omitempty"`  // roomID message pertains to
}

// WebSocketHandler handles new WebSocket connections.
func WebSocketHandler() func(*fiber.Ctx) error {
	return websocket.New(func(conn *websocket.Conn) {
		// Extract roomID and peerID from path parameters
		roomID := conn.Params("roomID")
		peerID := conn.Params("peerID")

		if roomID == "" {
			log.Println("WebSocket: RoomID is missing, closing connection.")
			conn.WriteJSON(SignalMessage{Type: "error", Payload: "RoomID is required"})
			conn.Close()
			return
		}
		if peerID == "" {
			log.Println("WebSocket: PeerID is missing, closing connection.")
			conn.WriteJSON(SignalMessage{Type: "error", Payload: "PeerID is required"})
			conn.Close()
			return
		}

		currentRoom := GetOrCreateRoom(roomID)
		client := &Client{Conn: conn, PeerID: peerID, RoomID: roomID}
		currentRoom.AddClient(client)

		log.Printf("WebSocket: Peer '%s' (conn: %p) connected to room '%s'", peerID, conn, roomID)

		// Notify other peers and the new peer about existing connections
		currentRoom.NotifyPeers(client)

		defer func() {
			log.Printf("WebSocket: Peer '%s' (conn: %p) disconnecting from room '%s'", peerID, conn, roomID)
			currentRoom.RemoveClient(client)

			// Broadcast "peer-left" message to remaining clients in the room
			peerLeftMsg := SignalMessage{Type: "peer-left", Sender: peerID, RoomID: roomID}
			// Sender for BroadcastMessage is nil here because the "peer-left" event is originated by the server
			// due to this client's disconnection. If other clients need to react, they will.
			currentRoom.BroadcastMessage(nil, peerLeftMsg)
			conn.Close()
			log.Printf("WebSocket: Peer '%s' (conn: %p) connection closed and resources cleaned up.", peerID, conn)
		}()

		// Loop to read messages from the client
		for {
			var receivedSignal SignalMessage
			if err := conn.ReadJSON(&receivedSignal); err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure, websocket.CloseNoStatusReceived) {
					log.Printf("WebSocket: Read error from peer '%s' (conn: %p): %v. Terminating connection.", peerID, conn, err)
				} else if err.Error() == "websocket: close sent" || err.Error() == "EOF" {
					log.Printf("WebSocket: Connection closed by peer '%s' (conn: %p).", peerID, conn)
				} else {
					log.Printf("WebSocket: Unhandled read error from peer '%s' (conn: %p): %v", peerID, conn, err)
				}
				break // Exit loop on any read error or connection close
			}

			log.Printf("Room '%s': Received message type '%s' from '%s' (conn: %p)", roomID, receivedSignal.Type, peerID, conn)

			// Validate or supplement the sender information
			if receivedSignal.Sender == "" {
				receivedSignal.Sender = peerID // Ensure sender is set
			} else if receivedSignal.Sender != peerID {
				log.Printf("WebSocket: Sender spoofing attempt from PeerID '%s' claiming to be '%s'. Discarding message.", peerID, receivedSignal.Sender)
				// Optionally send an error back to the malicious client
				// conn.WriteJSON(SignalMessage{Type: "error", Payload: "Sender mismatch"})
				continue // Skip processing this message
			}

			// Ensure RoomID is also set in the message, if not already
			if receivedSignal.RoomID == "" {
				receivedSignal.RoomID = roomID
			}

			// Relay the message using the room's broadcast logic
			currentRoom.BroadcastMessage(client, receivedSignal)
		}
	})
}
