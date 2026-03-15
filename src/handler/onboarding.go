package handler

import (
	"log"

	"getaced.io/src/containers"
	"getaced.io/src/database"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func CreateSetupSession(c fiber.Ctx) error {
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

	token, err := containers.NewSession(userID)

	if err != nil {
		log.Printf("failed to create Chrome session for user %d: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"session_url": "https://api.getaced.io/v1/s/" + token,
	})
}
