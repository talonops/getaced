package handler

import (
	"log"

	"getaced.io/src/analytics"
	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	"getaced.io/src/structs"
	"getaced.io/src/worker"

	"github.com/gofiber/fiber/v3"
)

func CreateSetupSession(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep == structs.OnboardingComplete {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "onboarding complete"})
	}

	if user.OnboardingStep != structs.OnboardingStepSession {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid onboarding step"})
	}

	token, err := containers.NewSession(userID)

	if err != nil {
		log.Printf("failed to create browser session for user %d: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create session"})
	}

	return c.JSON(fiber.Map{
		"session_url": config.Config("WEBHOOK_BASE_URL") + "/v1/s/" + token,
	})
}

func CompleteSession(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep == structs.OnboardingComplete {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "onboarding already complete"})
	}

	if user.OnboardingStep != structs.OnboardingStepSession {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "complete previous steps first"})
	}

	database.DB.Model(&user).Update("onboarding_step", structs.OnboardingComplete)
	analytics.Track("onboarding_step_completed", userID, analytics.WithStep("session"))
	analytics.Track("onboarding_step_completed", userID, analytics.WithStep("complete"))

	return c.JSON(fiber.Map{"onboarding_step": structs.OnboardingComplete})
}

func SetOnboardingWatchFolder(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var body struct {
		FolderURL string `json:"folder_url"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.FolderURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_url required"})
	}

	folderID := extractFolderID(body.FolderURL)
	if folderID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "could not extract folder ID from URL"})
	}

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep != structs.OnboardingStepWatchFolder {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid onboarding step"})
	}

	if user.DriveRefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "google drive not connected"})
	}

	refreshToken, err := crypto.Decrypt(user.DriveRefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
	if err != nil {
		log.Printf("decrypt refresh token error for user %d: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to read drive credentials"})
	}

	srv, err := drive.NewServiceFromRefreshToken(refreshToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to connect to drive"})
	}

	if err := drive.ValidateFolder(srv, folderID); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid folder: " + err.Error()})
	}

	database.DB.Model(&user).Updates(map[string]interface{}{
		"watch_folder_id": folderID,
		"onboarding_step": structs.OnboardingStepSession,
	})
	analytics.Track("onboarding_step_completed", userID, analytics.WithStep("watch_folder"))

	if err := worker.RegisterUserWatch(user, srv); err != nil {
		log.Printf("failed to register drive watch for user %d: %v", userID, err)
	}

	return c.JSON(fiber.Map{
		"onboarding_step": structs.OnboardingStepSession,
		"watch_folder_id": folderID,
	})
}
