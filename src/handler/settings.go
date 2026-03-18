package handler

import (
	"log"
	"net/url"
	"strings"

	"getaced.io/src/config"
	"getaced.io/src/crypto"
	"getaced.io/src/database"
	"getaced.io/src/drive"
	"getaced.io/src/structs"
	"getaced.io/src/worker"

	"github.com/gofiber/fiber/v3"
)

func GetSettings(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	return c.JSON(fiber.Map{
		"watch_folder_id":      user.WatchFolderID,
		"bookmark_folder_name": user.BookmarkFolderName,
		"custom_prompt":        user.CustomPrompt,
		"has_drive_connected":  user.DriveRefreshToken != "",
	})
}

// extractFolderID extracts a Google Drive folder ID from a full URL or raw ID.
// Supports:
//   - https://drive.google.com/drive/folders/FOLDER_ID
//   - https://drive.google.com/drive/u/0/folders/FOLDER_ID
//   - https://drive.google.com/drive/folders/FOLDER_ID?resourcekey=...
//   - Raw folder ID
func extractFolderID(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}

	parsed, err := url.Parse(input)
	if err != nil || parsed.Host == "" {
		// Not a URL — treat as raw folder ID
		return input
	}

	// Find "folders" in path and grab the next segment
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i, seg := range segments {
		if seg == "folders" && i+1 < len(segments) {
			return segments[i+1]
		}
	}

	return ""
}

func UpdateWatchFolder(c fiber.Ctx) error {
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

	database.DB.Model(&user).Update("watch_folder_id", folderID)

	// Register Drive push notifications (best effort — fallback poll covers failures)
	if err := worker.RegisterUserWatch(user, srv); err != nil {
		log.Printf("failed to register drive watch for user %d: %v", userID, err)
	}

	return c.JSON(fiber.Map{"watch_folder_id": folderID})
}

func UpdateBookmarkFolder(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var body struct {
		FolderName string `json:"folder_name"`
	}
	if err := c.Bind().JSON(&body); err != nil || body.FolderName == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_name required"})
	}
	if len(body.FolderName) > config.MaxBookmarkFolderNameLen {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "folder_name too long"})
	}

	database.DB.Model(&structs.User{}).Where("id = ?", userID).Update("bookmark_folder_name", body.FolderName)

	return c.JSON(fiber.Map{"bookmark_folder_name": body.FolderName})
}

func UpdatePrompt(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var body struct {
		CustomPrompt string `json:"custom_prompt"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}
	if len(body.CustomPrompt) > config.MaxCustomPromptLen {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "custom_prompt too long"})
	}

	database.DB.Model(&structs.User{}).Where("id = ?", userID).Update("custom_prompt", body.CustomPrompt)

	return c.JSON(fiber.Map{"custom_prompt": body.CustomPrompt})
}
