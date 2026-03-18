package handler

import (
	"crypto/rand"
	"fmt"
	"log"
	"sync"
	"time"

	"getaced.io/src/config"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/oauth2"
)

type oauthState struct {
	UserID    uint
	ExpiresAt time.Time
}

var driveOAuthStates = struct {
	sync.Mutex
	m map[string]oauthState
}{m: make(map[string]oauthState)}

func generateStateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func DriveAuth(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	if user.OnboardingStep != structs.OnboardingStepDrive {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid onboarding step"})
	}

	stateToken, err := generateStateToken()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate state"})
	}

	driveOAuthStates.Lock()
	driveOAuthStates.m[stateToken] = oauthState{
		UserID:    userID,
		ExpiresAt: time.Now().Add(10 * time.Minute),
	}
	driveOAuthStates.Unlock()

	cfg := drive.OAuthConfig()
	url := cfg.AuthCodeURL(
		stateToken,
		oauth2.AccessTypeOffline,
		oauth2.SetAuthURLParam("prompt", "consent"),
	)

	return c.JSON(fiber.Map{"url": url})
}

func DriveCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code"})
	}

	// Validate state token
	driveOAuthStates.Lock()
	s, ok := driveOAuthStates.m[state]
	if ok {
		delete(driveOAuthStates.m, state) // single-use
	}
	driveOAuthStates.Unlock()

	if !ok || time.Now().After(s.ExpiresAt) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or expired state"})
	}

	userID := s.UserID

	cfg := drive.OAuthConfig()
	token, err := cfg.Exchange(c.Context(), code)
	if err != nil {
		log.Printf("drive oauth exchange error: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "failed to exchange token"})
	}

	if token.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "no refresh token received"})
	}

	// Check that this Drive account isn't already linked to another user
	srv, err := drive.NewServiceFromToken(token)
	if err != nil {
		log.Printf("drive service from token error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to verify drive account"})
	}
	about, err := srv.About.Get().Fields("user(emailAddress)").Do()
	if err != nil {
		log.Printf("drive about error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to verify drive account"})
	}
	driveEmail := about.User.EmailAddress

	var existingUser structs.User
	if err := database.DB.Where("id != ? AND drive_email = ?", userID, driveEmail).First(&existingUser).Error; err == nil {
		frontendURL := config.Config("FRONTEND_URL")
		if frontendURL == "" {
			frontendURL = "https://getaced.io"
		}
		return c.Redirect().To(frontendURL + "/onboarding?step=drive&error=drive_already_linked")
	}

	encryptedToken, err := crypto.Encrypt(token.RefreshToken, config.Config("DRIVE_TOKEN_ENCRYPT_KEY"))
	if err != nil {
		log.Printf("encrypt refresh token error: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to store token"})
	}

	database.DB.Model(&structs.User{}).Where("id = ?", userID).Updates(map[string]interface{}{
		"drive_refresh_token": encryptedToken,
		"drive_email":         driveEmail,
		"onboarding_step":     structs.OnboardingStepSession,
	})

	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "https://getaced.io"
	}

	return c.Redirect().To(frontendURL + "/onboarding?step=session")
}

// CleanupExpiredOAuthStates removes expired state tokens from memory
func CleanupExpiredOAuthStates() {
	driveOAuthStates.Lock()
	defer driveOAuthStates.Unlock()
	now := time.Now()
	for token, state := range driveOAuthStates.m {
		if now.After(state.ExpiresAt) {
			delete(driveOAuthStates.m, token)
		}
	}
}
