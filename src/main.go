package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/creem"
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
		"CREEM_API_KEY",
		"CREEM_WEBHOOK_SECRET",
		"CREEM_BASE_URL",
		"OPENAI_API_KEY",
		"DRIVE_TOKEN_ENCRYPT_KEY",
		"WEBHOOK_BASE_URL",
	}
	for _, key := range required {
		if config.Config(key) == "" {
			log.Fatalf("required environment variable %s is not set", key)
		}
	}
}

func main() {
	godotenv.Load()
	validateConfig()

	if err := database.Init("getaced.db"); err != nil {
		log.Fatal("failed to init database:", err)
	}

	if err := containers.Init(); err != nil {
		log.Fatal("failed to init containers:", err)
	}

	handler.CreemClient = creem.NewClient(
		config.Config("CREEM_BASE_URL"),
		config.Config("CREEM_API_KEY"),
	)

	openaiClient := openai.NewClient(
		config.Config("OPENAI_API_KEY"),
		config.Config("OPENAI_MODEL"),
	)
	worker.RegisterCleanupHook(handler.CleanupExpiredOAuthStates)
	worker.Start(openaiClient)

	app := fiber.New()

	// CORS — allow frontend origin
	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://getaced.io"
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{frontendURL},
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
		worker.Stop()
		app.Shutdown()
		database.Close()
	}()

	log.Fatal(app.Listen(":" + port))
}
