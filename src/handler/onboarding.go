package handler

import (
	"log"

	"getaced.io/src/config"
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create session"})
	}

	return c.JSON(fiber.Map{
		"session_url": config.Config("WEBHOOK_BASE_URL") + "/v1/s/" + token,
	})
}

func CompleteOnboarding(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep == structs.OnboardingStepComplete {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "onboarding already complete"})
	}

	if user.OnboardingStep != structs.OnboardingStepChrome {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "complete previous steps first"})
	}

	database.DB.Model(&user).Update("onboarding_step", structs.OnboardingStepComplete)

	return c.JSON(fiber.Map{"onboarding_step": "complete"})
}
