package main

import (
	"log"
	// "errors" // Uncomment if using CustomErrorHandler example

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
)

func main() {
	app := fiber.New(fiber.Config{
		// ErrorHandler: CustomErrorHandler, // Anda bisa menambahkan error handler kustom jika diperlukan
	})

	// Middleware
	app.Use(logger.New(logger.Config{
		Format: "[${ip}]:${port} ${status} - ${method} ${path} ${latency}\n",
	}))
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*", // PERHATIAN: Gunakan domain spesifik di production!
		AllowHeaders: "Origin, Content-Type, Accept, Authorization",
		AllowMethods: "GET, POST, HEAD, PUT, DELETE, PATCH",
	}))

	// Rute statis untuk menyajikan file client (opsional)
	// Contoh: jika folder client/public Anda satu level di atas server/
	// app.Static("/", "../client/public")
	// Atau jika di dalam struktur yang sama (misal, server/public)
	// app.Static("/", "./public")

	// Middleware untuk memeriksa apakah ini permintaan upgrade WebSocket
	app.Use("/ws", func(c *fiber.Ctx) error {
		if websocket.IsWebSocketUpgrade(c) {
			c.Locals("allowed", true)
			return c.Next()
		}
		return fiber.ErrUpgradeRequired
	})

	// Rute WebSocket utama
	// :roomID dan :peerID akan menjadi parameter path
	app.Get("/ws/:roomID/:peerID", WebSocketHandler())

	// Rute HTTP dasar untuk pengecekan server
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Selamat Datang di Signaling Server WebRTC Go + Fiber!",
			"status":  "ok",
		})
	})

	// Rute untuk melihat room aktif (hanya untuk debugging, amankan jika di production)
	app.Get("/debug/rooms", func(c *fiber.Ctx) error {
		roomsMu.Lock()
		defer roomsMu.Unlock()

		activeRooms := make(map[string][]string)
		for roomID, room := range rooms {
			room.mu.Lock()
			var clientIDs []string
			for _, client := range room.Clients {
				clientIDs = append(clientIDs, client.PeerID)
			}
			activeRooms[roomID] = clientIDs
			room.mu.Unlock()
		}
		return c.JSON(activeRooms)
	})

	port := "8080"
	log.Printf("Server Go Fiber WebRTC berjalan di port %s", port)
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Gagal menjalankan server: %v", err)
	}
}
