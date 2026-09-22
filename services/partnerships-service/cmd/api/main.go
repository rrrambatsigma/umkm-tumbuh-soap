package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/config"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/database"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/partnerships"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/rest"
	"github.com/savitar393/umkm-tumbuh/services/partnerships-service/internal/router"
)

func main() {
	cfg := config.Load()

	ctx := context.Background()

	db, err := database.NewPostgresPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	partnershipRepo := partnerships.NewRepository(db)
	partnershipService := partnerships.NewService(partnershipRepo)
	partnershipHandler := rest.NewHandler(partnershipService)

	appRouter := router.NewRouter(
		partnershipHandler,
		cfg.FrontendURL,
		cfg.JWTSecret,
	)

	address := fmt.Sprintf("%s:%s", cfg.ServerHost, cfg.ServerPort)

	log.Printf("Server running on http://%s", address)

	if err := http.ListenAndServe(address, appRouter); err != nil {
		log.Fatal(err)
	}
}
