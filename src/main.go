package main

import (
	"encoding/hex"
	"log"
	"os"
	"os/signal"
	"syscall"

	"getaced.io/src/analytics"
	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/database"
	"getaced.io/src/handler"
	"getaced.io/src/openai"
	"getaced.io/src/router"
	"getaced.io/src/worker"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/joho/godotenv"
)

func validateConfig() {
	required := []string{
		"JWT_SECRET",
		"GOOGLE_CLIENT_ID",
		"GOOGLE_CLIENT_SECRET",
		"GOOGLE_REDIRECT_URL",
		"GOOGLE_DRIVE_REDIRECT_URL",
		"OPENAI_API_KEY",
		"DRIVE_TOKEN_ENCRYPT_KEY",
		"DRIVE_WEBHOOK_TOKEN",
		"WEBHOOK_BASE_URL",
		"ALLOWED_EMAILS",
	}
	for _, key := range required {
		if config.Config(key) == "" {
			log.Fatalf("required environment variable %s is not set", key)
		}
	}

	// Validate encryption key format (must be 64 hex chars = 32 bytes for AES-256)
	encKey := config.Config("DRIVE_TOKEN_ENCRYPT_KEY")
	if len(encKey) != 64 {
		log.Fatalf("DRIVE_TOKEN_ENCRYPT_KEY must be 64 hex characters (32 bytes), got %d", len(encKey))
	}
	if _, err := hex.DecodeString(encKey); err != nil {
		log.Fatalf("DRIVE_TOKEN_ENCRYPT_KEY is not valid hex: %v", err)
	}
}

func main() {
	godotenv.Load()
	validateConfig()

	if err := database.Init("getaced.db"); err != nil {
		log.Fatal("failed to init database:", err)
	}

	analytics.Start()

	startScript, err := os.ReadFile("scripts/start.sh")
	if err != nil {
		log.Fatal("failed to read scripts/start.sh:", err)
	}
	containers.StartScript = startScript

	if err := containers.Init(); err != nil {
		log.Fatal("failed to init containers:", err)
	}

	openaiClient := openai.NewClient(
		config.Config("OPENAI_API_KEY"),
		config.Config("OPENAI_MODEL"),
	)
	worker.RegisterCleanupHook(handler.CleanupExpiredOAuthStates)
	worker.RegisterCleanupHook(handler.CleanupExpiredGoogleStates)
	worker.Start(openaiClient)

	app := fiber.New()

	// CORS — allow frontend origin
	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://getaced.io"
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{frontendURL, "http://localhost:3000"},
		AllowHeaders: []string{"Authorization", "Content-Type"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}))

	router.Routes(app)

	port := config.Config("APP_PORT")
	if port == "" {
		port = "80"
	}

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("shutting down...")
		analytics.Stop()
		worker.Stop()
		app.Shutdown()
		database.Close()
	}()

	log.Fatal(app.Listen(":" + port))
}
