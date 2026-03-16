package router

import (
	"getaced.io/src/handler"
	"getaced.io/src/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/static"
)

func Routes(app *fiber.App) {
	v1 := app.Group("/v1", logger.New())
	app.Get("/novnc/*", static.New("/home/talon/novnc-static"))

	// Public auth routes
	v1.Get("/auth/google", handler.GoogleAuth)
	v1.Get("/auth/google/callback", handler.GoogleCallback)

	// Public webhook
	v1.Post("/webhooks/creem", handler.CreemWebhook)

	// noVNC routes (public but token-protected)
	v1.Get("/s/:token", handler.ChromeSession)
	v1.Get("/ws/:token/*", handler.WSProxy)

	// Protected routes
	v1.Get("/me", middleware.AuthRequired, handler.GetMe)
	v1.Post("/onboarding/chrome", middleware.AuthRequired, handler.CreateSetupSession)
	v1.Post("/checkout", middleware.AuthRequired, handler.CreateCheckout)

}
