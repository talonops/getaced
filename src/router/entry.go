package router

import (
	"getaced.io/src/config"
	"getaced.io/src/handler"
	"getaced.io/src/middleware"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/static"
)

func Routes(app *fiber.App) {
	v1 := app.Group("/v1", logger.New())

	novncPath := config.Config("NOVNC_STATIC_PATH")
	if novncPath == "" {
		novncPath = config.DefaultNoVNCPath
	}
	app.Get("/novnc/*", static.New(novncPath))
	app.Get("/docs/*", static.New("./docs"))
	app.Get("/", func(c fiber.Ctx) error {
		return c.Redirect().To("/docs/")
	})

	// Public auth routes
	v1.Get("/auth/google", handler.GoogleAuth)
	v1.Get("/auth/google/callback", handler.GoogleCallback)
	v1.Post("/auth/exchange", handler.ExchangeAuthCode)

	// Drive OAuth (payment required to initiate, callback is public)
	v1.Get("/auth/google/drive", middleware.AuthRequired, middleware.PaymentRequired, handler.DriveAuth)
	v1.Get("/auth/google/drive/callback", handler.DriveCallback)

	// Public webhooks
	v1.Post("/webhooks/creem", handler.CreemWebhook)
	v1.Post("/webhooks/drive", handler.DriveWebhook)

	// noVNC session page (public but token-protected)
	v1.Get("/s/:token", handler.BrowserSession)

	// Internal: nginx auth_request to resolve WS token → container IP
	v1.Get("/internal/resolve-ws", handler.ResolveWS)

	// Protected routes
	v1.Get("/me", middleware.AuthRequired, handler.GetMe)
	v1.Post("/checkout", middleware.AuthRequired, handler.CreateCheckout)
	v1.Post("/billing", middleware.AuthRequired, handler.CustomerPortal)

	// Onboarding (payment required)
	v1.Post("/onboarding/session", middleware.AuthRequired, middleware.PaymentRequired, handler.CreateSetupSession)
	v1.Post("/onboarding/session/complete", middleware.AuthRequired, middleware.PaymentRequired, handler.CompleteSession)
	v1.Post("/onboarding/watch-folder", middleware.AuthRequired, middleware.PaymentRequired, handler.SetOnboardingWatchFolder)

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
