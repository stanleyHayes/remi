package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"remi-api/internal/config"
	"remi-api/internal/db"
	"remi-api/internal/handlers"
	"remi-api/internal/server"
)

func main() {
	cfg := config.Load()
	if err := cfg.ValidateProduction(); err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, database, err := db.Connect(ctx, cfg.MongoURI)
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
	defer client.Disconnect(context.Background())
	if cfg.SeedAdminConfigured {
		created, err := db.EnsureInitialAdmin(ctx, database, cfg.SeedAdminEmail, cfg.SeedAdminPassword)
		if err != nil {
			log.Fatalf("initial admin: %v", err)
		}
		if created {
			log.Printf("created initial administrator %s", cfg.SeedAdminEmail)
		}
	}

	h := handlers.New(database, cfg)
	r := server.NewRouter(h, cfg.CORSOrigins)

	addr := ":" + cfg.Port
	log.Printf("REMI API listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, r))
}
