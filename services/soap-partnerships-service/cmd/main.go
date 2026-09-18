package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/config"
	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/database"
	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/partnerships"
	"github.com/savitar393/umkm-tumbuh/services/soap-partnerships-service/internal/soap"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer pool.Close()
	log.Println("Database connected.")

	// Load WSDL file content (embedded at startup)
	wsdlContent, err := os.ReadFile("wsdl/partnership.wsdl")
	if err != nil {
		log.Fatalf("Failed to read WSDL file: %v", err)
	}

	// Wire up layers
	repo := partnerships.NewRepository(pool)
	svc := partnerships.NewService(repo)
	handler := soap.NewHandler(svc, string(wsdlContent))

	// Register route
	mux := http.NewServeMux()
	mux.Handle("/partnership", handler)

	addr := ":" + cfg.Port
	log.Printf("SOAP Partnership Service starting on http://localhost%s", addr)
	log.Printf("WSDL available at:  http://localhost%s/partnership?wsdl", addr)
	log.Printf("SOAP endpoint:      http://localhost%s/partnership", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
