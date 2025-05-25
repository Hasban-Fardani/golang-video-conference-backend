package main

import (
	"log"
	"sync"

	"github.com/gofiber/contrib/websocket"
)

// Client struct (Tidak berubah)
type Client struct {
	Conn   *websocket.Conn
	PeerID string
	RoomID string
}

// Room struct (Tidak berubah)
type Room struct {
	ID      string
	Clients map[*websocket.Conn]*Client
	mu      sync.Mutex
}

// Global rooms map (Tidak berubah)
var rooms = make(map[string]*Room)
var roomsMu sync.Mutex

// GetOrCreateRoom (Tidak berubah)
func GetOrCreateRoom(roomID string) *Room {
	roomsMu.Lock()
	defer roomsMu.Unlock()

	if r, ok := rooms[roomID]; ok {
		return r
	}

	r := &Room{
		ID:      roomID,
		Clients: make(map[*websocket.Conn]*Client),
	}
	rooms[roomID] = r
	log.Printf("Room '%s' created", roomID)
	return r
}

// AddClient (Tidak berubah)
func (r *Room) AddClient(client *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Clients[client.Conn] = client
	log.Printf("Client '%s' (conn: %p) added to room '%s'", client.PeerID, client.Conn, r.ID)
}

// RemoveClient (Tidak berubah, tapi pastikan lognya jelas)
func (r *Room) RemoveClient(client *Client) {
	r.mu.Lock()
	log.Printf("Attempting to remove client '%s' (conn: %p) from room '%s'", client.PeerID, client.Conn, r.ID)
	_, exists := r.Clients[client.Conn]
	if !exists {
		log.Printf("Client '%s' (conn: %p) not found in room '%s' for removal.", client.PeerID, client.Conn, r.ID)
		r.mu.Unlock()
		return
	}
	delete(r.Clients, client.Conn)
	clientCount := len(r.Clients)
	r.mu.Unlock()

	log.Printf("Client '%s' successfully removed. Room '%s' now has %d clients.", client.PeerID, r.ID, clientCount)

	if clientCount == 0 {
		roomsMu.Lock()
		delete(rooms, r.ID)
		log.Printf("Room '%s' deleted because it's empty", r.ID)
		roomsMu.Unlock()
	}
}

// BroadcastMessage (Sedikit penyesuaian pada logging dan penanganan sender nil)
func (r *Room) BroadcastMessage(sender *Client, message SignalMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.Clients) == 0 {
		log.Printf("Room '%s': No clients to broadcast message type '%s' to.", r.ID, message.Type)
		return
	}

	sourcePeerID := "server" // Default jika sender adalah nil
	if sender != nil {
		sourcePeerID = sender.PeerID
	}
	if message.Sender != "" && message.Sender != sourcePeerID && sender != nil { // Jika message punya sender sendiri & itu beda dari actual sender
		log.Printf("Room '%s': Message sender field ('%s') differs from actual sender ('%s'). Using actual sender for logic.", r.ID, message.Sender, sourcePeerID)
		// Anda mungkin ingin message.Sender diisi oleh server saja untuk konsistensi
	}

	for targetConn, targetClient := range r.Clients {
		// Jika pesan memiliki target spesifik
		if message.Target != "" {
			if targetClient.PeerID == message.Target {
				log.Printf("Room '%s': Relaying targeted message type '%s' from '%s' to target '%s' (conn: %p)", r.ID, message.Type, sourcePeerID, targetClient.PeerID, targetConn)
				if err := targetConn.WriteJSON(message); err != nil {
					log.Printf("Room '%s': Write error for targeted message to '%s': %v", r.ID, targetClient.PeerID, err)
				}
				// Setelah mengirim ke target, kita bisa return jika hanya satu target yang dimaksud.
				// Jika `Target` bisa berupa list, logikanya akan berbeda. Asumsi `Target` adalah ID tunggal.
				return
			}
			continue // Bukan target, lanjut ke client berikutnya
		}

		// Jika broadcast umum (tidak ada target spesifik)
		// Jangan kirim ke pengirim asli, kecuali jika sender adalah nil (pesan dari server)
		// atau tipe pesan tertentu yang memang harus diterima pengirim.
		if sender != nil && targetConn == sender.Conn {
			// Contoh: jika ada pesan konfirmasi yang perlu diterima pengirim juga
			// if message.Type == "some-confirmation-type" { ... kirim ... }
			continue // Skip mengirim ke diri sendiri untuk broadcast umum
		}

		log.Printf("Room '%s': Broadcasting message type '%s' from '%s' to '%s' (conn: %p)", r.ID, message.Type, sourcePeerID, targetClient.PeerID, targetConn)
		if err := targetConn.WriteJSON(message); err != nil {
			log.Printf("Room '%s': Broadcast write error to '%s': %v", r.ID, targetClient.PeerID, err)
		}
	}
}

// NotifyPeers (Tidak berubah)
func (r *Room) NotifyPeers(newClient *Client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	newPeerMessage := SignalMessage{Type: "new-peer", Sender: newClient.PeerID, RoomID: r.ID}
	for _, existingClient := range r.Clients {
		if existingClient.Conn == newClient.Conn {
			continue
		}
		log.Printf("Room '%s': Notifying existing peer '%s' about new peer '%s'", r.ID, existingClient.PeerID, newClient.PeerID)
		if err := existingClient.Conn.WriteJSON(newPeerMessage); err != nil {
			log.Printf("Room '%s': Error notifying existing peer '%s': %v", r.ID, existingClient.PeerID, err)
		}
	}

	for _, existingClient := range r.Clients {
		if existingClient.Conn == newClient.Conn {
			continue
		}
		existingPeerMessage := SignalMessage{Type: "existing-peer", Sender: existingClient.PeerID, RoomID: r.ID}
		log.Printf("Room '%s': Notifying new peer '%s' about existing peer '%s'", r.ID, newClient.PeerID, existingClient.PeerID)
		if err := newClient.Conn.WriteJSON(existingPeerMessage); err != nil {
			log.Printf("Room '%s': Error notifying new peer '%s' about existing peer '%s': %v", r.ID, newClient.PeerID, existingClient.PeerID, err)
		}
	}
}
