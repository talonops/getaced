package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"

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
		log.Fatal("failed to init containers:", err)
	}

	handler.CreemClient = creem.NewClient(
		config.Config("CREEM_BASE_URL"),
		config.Config("CREEM_API_KEY"),
	)

	appPort := config.Config("APP_PORT")
	if appPort == "" {
		appPort = "80"
	}

	wsPort := config.Config("WS_PORT")
	if wsPort == "" {
		wsPort = "81"
	}

	app := fiber.New()
	router.Routes(app)

	// WS proxy on a secondary port
	fiberTarget, _ := url.Parse("http://127.0.0.1:" + appPort)
	proxy := httputil.NewSingleHostReverseProxy(fiberTarget)
	http.HandleFunc("/v1/ws/", handler.WSProxyHTTP)
	http.Handle("/", proxy)
	go func() {
		log.Printf("ws proxy listening on :%s (proxying rest to fiber :%s)", wsPort, appPort)
		if err := http.ListenAndServe(":"+wsPort, nil); err != nil {
			log.Fatal("ws proxy failed:", err)
		}
	}()

	log.Printf("fiber listening on :%s", appPort)
	log.Fatal(app.Listen(":" + appPort))
}
