package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"L3_5/internal/server"
	"L3_5/internal/storage"
	"L3_5/models"
)

func main() {
	log.Printf("=== Starting Event Booking Service ===")
	log.Printf("Loading configuration from config.yaml")

	cfg := models.MustLoadConfig("config.yaml")
	log.Printf("Configuration loaded successfully")

	log.Printf("Initializing database connection...")
	pool, err := storage.InitDB(cfg)
	if err != nil {
		log.Fatal("Failed to init DB:", err)
	}
	defer func() {
		log.Printf("Closing database connection pool")
		pool.Close()
	}()

	log.Printf("Creating storage and server instances")
	store := storage.New(pool)
	srv := server.New(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("Starting background worker for expired booking cleanup")
	go srv.StartBackgroundWorker(ctx)

	log.Printf("Starting HTTP server on port %s", cfg.Server.Port)
	go func() {
		if err := srv.Start(cfg.Server.Port); err != nil {
			log.Fatal("Server error:", err)
		}
	}()

	log.Printf("=== Event Booking Service Started Successfully ===")
	log.Printf("Server is running on port %s", cfg.Server.Port)
	log.Printf("Press Ctrl+C to stop the server")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit

	log.Printf("Received interrupt signal, shutting down gracefully...")
	cancel()
	log.Printf("=== Event Booking Service Stopped ===")
}
