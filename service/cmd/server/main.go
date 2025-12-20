package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/michaelw/timesheet-app/service/internal/api"
	"github.com/michaelw/timesheet-app/service/internal/database"
	"github.com/michaelw/timesheet-app/service/internal/handler"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

func main() {
	// Configuration
	port := getEnv("PORT", "8080")
	jwtSecret := getEnv("JWT_SECRET", "development-secret-change-in-production")
	jwtExpiration := 24 * time.Hour
	databaseURL := getEnv("DATABASE_URL", "postgresql://timesheet:changeMe123!@localhost:5432/timesheet_v2")

	ctx := context.Background()

	// Initialize database
	log.Printf("Connecting to database...")
	db, err := database.New(ctx, databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run migrations
	log.Printf("Running migrations...")
	if err := db.Migrate(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)

	// Initialize services
	jwtService := handler.NewJWTService(jwtSecret, jwtExpiration)

	// Initialize handlers
	serverHandler := handler.NewServer(userStore, projectStore, timeEntryStore, jwtService)

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
	strictHandler := api.NewStrictHandler(serverHandler, nil)
	api.HandlerFromMux(strictHandler, r)

	// Start server
	addr := fmt.Sprintf(":%s", port)
	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Printf("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("Starting server on %s", addr)
	log.Printf("OpenAPI spec: http://localhost:%s/api/openapi.yaml", port)
	log.Printf("Swagger UI: https://petstore.swagger.io/?url=http://localhost:%s/api/openapi.yaml", port)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server failed: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
