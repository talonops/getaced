package router

import (
	"getaced.io/src/handler"
	"getaced.io/src/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

func Routes(app *fiber.App) {
	v1 := app.Group("/v1", logger.New())

	// Public auth routes
	v1.Get("/auth/google", handler.GoogleAuth)
	v1.Get("/auth/google/callback", handler.GoogleCallback)

	// Public webhook
	v1.Post("/webhooks/creem", handler.CreemWebhook)

	// Protected routes
	protected := v1.Group("", middleware.AuthRequired)
	protected.Get("/me", handler.GetMe)

	protected.Post("/onboarding/chrome", handler.CreateSetupSession)

	protected.Post("/checkout", handler.CreateCheckout)

	// noVNC routes (public but token-protected)
	v1.Get("/s/:token", handler.ChromeSession)
	v1.Get("/ws/:token/*", handler.WSProxy)

}
