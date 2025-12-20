package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/handler"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

func main() {
	// Configuration
	port := getEnv("PORT", "8080")
	jwtSecret := getEnv("JWT_SECRET", "development-secret-change-in-production")
	jwtExpiration := 24 * time.Hour

	// Initialize stores
	userStore := store.NewUserStore()

	// Initialize services
	jwtService := handler.NewJWTService(jwtSecret, jwtExpiration)

	// Initialize handlers
	authHandler := handler.NewAuthHandler(userStore, jwtService)

	// Create router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(handler.AuthMiddleware(jwtService))

	// CORS for development
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// Serve OpenAPI spec
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "../docs/v2/api-spec.yaml")
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Mount API routes
	strictHandler := api.NewStrictHandler(authHandler, nil)
	api.HandlerFromMux(strictHandler, r)

	// Start server
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting server on %s", addr)
	log.Printf("OpenAPI spec: http://localhost:%s/api/openapi.yaml", port)
	log.Printf("Swagger UI: https://petstore.swagger.io/?url=http://localhost:%s/api/openapi.yaml", port)

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
