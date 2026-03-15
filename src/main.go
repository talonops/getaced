package main

import (
	"log"

	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/creem"
	"getaced.io/src/database"
	"getaced.io/src/handler"
	"getaced.io/src/router"
	"github.com/gofiber/fiber/v3"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	if err := database.Init("getaced.db"); err != nil {
		log.Fatal("failed to init database:", err)
	}

	if err := containers.Init(); err != nil {
		log.Fatal("failed to lxd client:", err)
	}

	if err := containers.Init(); err != nil {
		log.Fatal("failed to lxd client:", err)
	}

	handler.CreemClient = creem.NewClient(
		config.Config("CREEM_BASE_URL"),
		config.Config("CREEM_API_KEY"),
	)

	app := fiber.New()
	router.Routes(app)

	port := config.Config("APP_PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatal(app.Listen(":" + port))

}
