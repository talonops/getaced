package handler

import (
	"crypto/rand"
	"fmt"
	"strings"
	"sync"
	"time"

	"getaced.io/src/auth"
	"getaced.io/src/config"
	"getaced.io/src/database"
	"getaced.io/src/middleware"
	"getaced.io/src/structs"

	"github.com/gofiber/fiber/v3"
)

func isEmailAllowed(email string) bool {
	raw := config.Config("ALLOWED_EMAILS")
	target := strings.ToLower(strings.TrimSpace(email))
	for _, entry := range strings.Split(raw, ",") {
		if strings.ToLower(strings.TrimSpace(entry)) == target {
			return true
		}
	}
	return false
}

// OAuth state tokens — prevents CSRF on Google login
var googleOAuthStates = struct {
	sync.Mutex
	m map[string]time.Time // state → expiresAt
}{m: make(map[string]time.Time)}

// Auth codes — short-lived codes exchanged for JWTs (avoids JWT in URL)
type authCodeEntry struct {
	JWT       string
	ExpiresAt time.Time
}

var authCodes = struct {
	sync.Mutex
	m map[string]authCodeEntry
}{m: make(map[string]authCodeEntry)}

func generateRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

func GoogleAuth(c fiber.Ctx) error {
	state, err := generateRandomHex(32)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate state"})
	}

	googleOAuthStates.Lock()
	googleOAuthStates.m[state] = time.Now().Add(config.OAuthStateExpiry)
	googleOAuthStates.Unlock()

	url := auth.ConfigGoogle().AuthCodeURL(state)
	return c.JSON(fiber.Map{"url": url})
}

func GoogleCallback(c fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing code parameter"})
	}

	// Validate and consume state token
	googleOAuthStates.Lock()
	expiresAt, ok := googleOAuthStates.m[state]
	if ok {
		delete(googleOAuthStates.m, state)
	}
	googleOAuthStates.Unlock()

	if !ok || time.Now().After(expiresAt) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid or expired state"})
	}

	token, err := auth.ConfigGoogle().Exchange(c.RequestCtx(), code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "failed to exchange token"})
	}

	userInfo, err := auth.GetUserInfo(token.AccessToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to get user info"})
	}

	if !isEmailAllowed(userInfo.Email) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": "access denied"})
	}

	// Upsert user
	var user structs.User
	result := database.DB.Where("google_id = ?", userInfo.ID).First(&user)
	if result.Error != nil {
		now := time.Now()
		usageReset := now.Add(30 * 24 * time.Hour)

		user = structs.User{
			GoogleID:           userInfo.ID,
			Email:              userInfo.Email,
			Name:               userInfo.Name,
			AvatarURL:          userInfo.Picture,
			BookmarkFolderName: "school work",
			UsageResetAt:       &usageReset,
		}
		if err := database.DB.Create(&user).Error; err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create user"})
		}
	} else {
		database.DB.Model(&user).Updates(map[string]interface{}{
			"email":      userInfo.Email,
			"name":       userInfo.Name,
			"avatar_url": userInfo.Picture,
		})
	}

	// Generate JWT
	jwtToken, err := middleware.GenerateJWT(user.ID, user.Email)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate token"})
	}

	frontendURL := config.Config("FRONTEND_URL")
	if frontendURL == "" {
		return c.JSON(fiber.Map{"token": jwtToken})
	}

	// Generate short-lived auth code instead of putting JWT in URL
	authCode, err := generateRandomHex(32)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate auth code"})
	}

	authCodes.Lock()
	authCodes.m[authCode] = authCodeEntry{
		JWT:       jwtToken,
		ExpiresAt: time.Now().Add(config.AuthCodeExpiry),
	}
	authCodes.Unlock()

	return c.Redirect().To(frontendURL + "/auth/callback?code=" + authCode)
}

// ExchangeAuthCode exchanges a short-lived auth code for a JWT
func ExchangeAuthCode(c fiber.Ctx) error {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.Code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "code required"})
	}

	authCodes.Lock()
	entry, ok := authCodes.m[body.Code]
	if ok {
		delete(authCodes.m, body.Code)
	}
	authCodes.Unlock()

	if !ok || time.Now().After(entry.ExpiresAt) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid or expired code"})
	}

	return c.JSON(fiber.Map{"token": entry.JWT})
}

// CleanupExpiredGoogleStates removes expired OAuth states and auth codes
func CleanupExpiredGoogleStates() {
	now := time.Now()

	googleOAuthStates.Lock()
	for state, expiresAt := range googleOAuthStates.m {
		if now.After(expiresAt) {
			delete(googleOAuthStates.m, state)
		}
	}
	googleOAuthStates.Unlock()

	authCodes.Lock()
	for code, entry := range authCodes.m {
		if now.After(entry.ExpiresAt) {
			delete(authCodes.m, code)
		}
	}
	authCodes.Unlock()
}
