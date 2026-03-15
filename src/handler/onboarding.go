package handler

import (
	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func ChromeSession(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep == structs.OnboardingStepComplete {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "onboarding complete"})
	}

	if user.OnboardingStep != structs.OnboardingStepChrome {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid onboarding step"})
	}

	// Clone the snapshot and configure, send back connect url
	// Make sure to also set timer

	return c.JSON(fiber.Map{
		"id":                  user.ID,
		"email":               user.Email,
		"onboarding_step":     user.OnboardingStep,
		"subscription_status": user.SubscriptionStatus,
		"subscription_id":     user.SubscriptionID,
	})
}
