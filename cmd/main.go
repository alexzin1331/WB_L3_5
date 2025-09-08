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
	cfg := models.MustLoadConfig("config.yaml")

	pool, err := storage.InitDB(cfg)
	if err != nil {
		log.Fatal("Failed to init DB:", err)
	}
	defer pool.Close()

	store := storage.New(pool)
	srv := server.New(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv.StartBackgroundWorker(ctx)

	go func() {
		if err := srv.Start(cfg.Server.Port); err != nil {
			log.Fatal("Server error:", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	cancel()
}
