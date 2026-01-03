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
	"github.com/michaelw/timesheet-app/service/internal/classification"
	"github.com/michaelw/timesheet-app/service/internal/crypto"
	"github.com/michaelw/timesheet-app/service/internal/database"
	"github.com/michaelw/timesheet-app/service/internal/google"
	"github.com/michaelw/timesheet-app/service/internal/handler"
	"github.com/michaelw/timesheet-app/service/internal/store"
)

func main() {
	// Configuration
	port := getEnv("PORT", "8080")
	jwtSecret := getEnv("JWT_SECRET", "development-secret-change-in-production")
	jwtExpiration := 24 * time.Hour
	databaseURL := getEnv("DATABASE_URL", "postgresql://timesheet:changeMe123!@localhost:5432/timesheet_v2")

	// Calendar integration config
	encryptionKey := getEnv("ENCRYPTION_KEY", "")
	googleClientID := getEnv("GOOGLE_CLIENT_ID", "")
	googleClientSecret := getEnv("GOOGLE_CLIENT_SECRET", "")
	googleRedirectURL := getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/auth/google/callback")

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

	// Initialize encryption service (optional, required for calendar integration)
	var cryptoService *crypto.EncryptionService
	if encryptionKey != "" {
		var err error
		cryptoService, err = crypto.NewEncryptionService(encryptionKey)
		if err != nil {
			log.Fatalf("Failed to initialize encryption: %v", err)
		}
		log.Printf("Encryption service initialized")
	} else {
		log.Printf("Warning: ENCRYPTION_KEY not set, calendar integration disabled")
	}

	// Initialize Google Calendar service (optional)
	var googleService *google.CalendarService
	if googleClientID != "" && googleClientSecret != "" {
		googleService = google.NewCalendarService(googleClientID, googleClientSecret, googleRedirectURL)
		log.Printf("Google Calendar integration enabled")
	} else {
		log.Printf("Google Calendar integration not configured (missing GOOGLE_CLIENT_ID/GOOGLE_CLIENT_SECRET)")
	}

	// Initialize stores
	userStore := store.NewUserStore(db.Pool)
	projectStore := store.NewProjectStore(db.Pool)
	timeEntryStore := store.NewTimeEntryStore(db.Pool)
	calendarConnectionStore := store.NewCalendarConnectionStore(db.Pool, cryptoService)
	calendarStore := store.NewCalendarStore(db.Pool)
	calendarEventStore := store.NewCalendarEventStore(db.Pool)
	classificationRuleStore := store.NewClassificationRuleStore(db.Pool)

	// Initialize services
	jwtService := handler.NewJWTService(jwtSecret, jwtExpiration)
	classificationService := classification.NewService(db.Pool, classificationRuleStore, calendarEventStore)

	// Initialize handlers
	serverHandler := handler.NewServer(
		userStore, projectStore, timeEntryStore,
		calendarConnectionStore, calendarStore, calendarEventStore,
		classificationRuleStore,
		jwtService, googleService,
		classificationService,
	)

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

	// Intercept OAuth callback (needs browser redirect, not JSON response)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if req.URL.Path == "/api/auth/google/callback" && req.Method == "GET" {
				code := req.URL.Query().Get("code")
				state := req.URL.Query().Get("state")

				if code == "" || state == "" {
					http.Redirect(w, req, "/settings?error=missing_params", http.StatusFound)
					return
				}

				err := serverHandler.CalendarHandler.HandleOAuthCallback(req.Context(), code, state)
				if err != nil {
					log.Printf("OAuth callback error: %v", err)
					http.Redirect(w, req, "/settings?error=oauth_failed", http.StatusFound)
					return
				}

				http.Redirect(w, req, "/settings?connected=google", http.StatusFound)
				return
			}
			next.ServeHTTP(w, req)
		})
	})

	// Serve OpenAPI spec
	apiSpecPath := getEnv("API_SPEC_PATH", "../docs/v2/api-spec.yaml")
	r.Get("/api/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, apiSpecPath)
	})

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Mount API routes
	strictHandler := api.NewStrictHandler(serverHandler, nil)
	api.HandlerFromMux(strictHandler, r)

	// Serve static files for SPA (must be after API routes)
	staticDir := getEnv("STATIC_DIR", "")
	if staticDir != "" {
		log.Printf("Serving static files from %s", staticDir)
		fileServer := http.FileServer(http.Dir(staticDir))
		r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the file directly
			path := staticDir + r.URL.Path
			if _, err := os.Stat(path); os.IsNotExist(err) {
				// File doesn't exist, serve index.html for SPA routing
				http.ServeFile(w, r, staticDir+"/index.html")
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}

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
