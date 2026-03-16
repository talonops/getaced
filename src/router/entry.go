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
	app.Use("/docs", static.New("./docs"))

	// Public auth routes
	v1.Get("/auth/google", handler.GoogleAuth)
	v1.Get("/auth/google/callback", handler.GoogleCallback)

	// Drive OAuth (auth required to initiate, callback is public)
	v1.Get("/auth/google/drive", middleware.AuthRequired, handler.DriveAuth)
	v1.Get("/auth/google/drive/callback", handler.DriveCallback)

	// Public webhooks
	v1.Post("/webhooks/creem", handler.CreemWebhook)
	v1.Post("/webhooks/drive", handler.DriveWebhook)

	// noVNC session page (public but token-protected)
	v1.Get("/s/:token", handler.ChromeSession)

	// Internal: nginx auth_request to resolve WS token → container IP
	v1.Get("/internal/resolve-ws", handler.ResolveWS)

	// Protected routes
	v1.Get("/me", middleware.AuthRequired, handler.GetMe)
	v1.Post("/onboarding/chrome", middleware.AuthRequired, handler.CreateSetupSession)
	v1.Post("/onboarding/complete", middleware.AuthRequired, handler.CompleteOnboarding)
	v1.Post("/checkout", middleware.AuthRequired, handler.CreateCheckout)

	// Settings (payment required)
	v1.Get("/settings", middleware.AuthRequired, middleware.PaymentRequired, handler.GetSettings)
	v1.Put("/settings/watch-folder", middleware.AuthRequired, middleware.PaymentRequired, handler.UpdateWatchFolder)
	v1.Put("/settings/bookmark-folder", middleware.AuthRequired, middleware.PaymentRequired, handler.UpdateBookmarkFolder)
	v1.Put("/settings/prompt", middleware.AuthRequired, middleware.PaymentRequired, handler.UpdatePrompt)

	// Usage & activity (payment required)
	v1.Get("/usage", middleware.AuthRequired, middleware.PaymentRequired, handler.GetUsage)
	v1.Get("/activity", middleware.AuthRequired, middleware.PaymentRequired, handler.GetActivity)

	// Account management
	v1.Delete("/account", middleware.AuthRequired, handler.DeleteAccount)

	// Health check
	v1.Get("/health", handler.Health)
}
