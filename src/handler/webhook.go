package handler

import (
	"getaced.io/src/config"
	"getaced.io/src/database"
	"getaced.io/src/structs"
	"getaced.io/src/worker"

	"github.com/gofiber/fiber/v3"
)

func DriveWebhook(c fiber.Ctx) error {
	channelID := c.Get("X-Goog-Channel-ID")
	state := c.Get("X-Goog-Resource-State")
	channelToken := c.Get("X-Goog-Channel-Token")

	if channelID == "" {
		return c.SendStatus(fiber.StatusBadRequest)
	}

	// Validate channel token matches our webhook secret
	expectedToken := config.Config("DRIVE_WEBHOOK_TOKEN")
	if expectedToken != "" && channelToken != expectedToken {
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// "sync" is the initial verification ping from Google
	if state == "sync" {
		return c.SendStatus(fiber.StatusOK)
	}

	// Look up user by their channel ID
	var user structs.User
	if err := database.DB.Where("drive_channel_id = ?", channelID).First(&user).Error; err != nil {
		return c.SendStatus(fiber.StatusOK) // unknown channel, ignore
	}

	// Enqueue for debounced processing
	worker.EnqueueUser(user.ID)

	return c.SendStatus(fiber.StatusOK)
}
