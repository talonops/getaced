package handler

import (
	"fmt"
	"log"

	"getaced.io/src/containers"
	"getaced.io/src/database"
	"getaced.io/src/structs"
	"getaced.io/src/worker"

	"github.com/canonical/lxd/shared/api"
	"github.com/gofiber/fiber/v3"
)

func DeleteAccount(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	// Stop Drive watch
	worker.StopUserWatch(user)

	// Delete LXD container
	containerName := fmt.Sprintf("getaced-%d", userID)
	deleteContainer(containerName)

	// Delete related records
	database.DB.Where("user_id = ?", userID).Delete(&structs.ProcessedFile{})
	database.DB.Where("user_id = ?", userID).Delete(&structs.FailedFile{})
	database.DB.Delete(&user)

	return c.JSON(fiber.Map{"message": "account deleted"})
}

func DisconnectDrive(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	// Stop Drive watch
	worker.StopUserWatch(user)

	database.DB.Model(&user).Updates(map[string]interface{}{
		"drive_refresh_token":  "",
		"watch_folder_id":      "",
		"drive_page_token":     "",
		"drive_channel_id":     "",
		"drive_channel_expiry": nil,
	})

	return c.JSON(fiber.Map{"message": "drive disconnected"})
}

func ResetChrome(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var user structs.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "user not found"})
	}

	// Delete old container
	containerName := fmt.Sprintf("getaced-%d", userID)
	deleteContainer(containerName)

	// Create new session
	token, err := containers.NewSession(userID)
	if err != nil {
		log.Printf("failed to create Chrome session for user %d: %v", userID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{
		"session_url": "https://api.getaced.io/v1/s/" + token,
	})
}

func deleteContainer(name string) {
	// Read IP config before stopping
	var suffix int
	content, _, err := containers.Client.GetInstanceFile(name, "/etc/getaced.conf")
	if err == nil {
		buf := make([]byte, 256)
		n, _ := content.Read(buf)
		fmt.Sscanf(string(buf[:n]), "IP_SUFFIX=%d", &suffix)
	}

	// Stop (best effort)
	stopOp, err := containers.Client.UpdateInstanceState(name, api.InstanceStatePut{Action: "stop", Force: true}, "")
	if err == nil {
		stopOp.Wait()
	}

	// Delete
	delOp, err := containers.Client.DeleteInstance(name, true)
	if err != nil {
		log.Printf("failed to delete container %s: %v", name, err)
		return
	}
	if err := delOp.Wait(); err != nil {
		log.Printf("error waiting for container %s deletion: %v", name, err)
		return
	}

	// Release IP
	if suffix >= 101 && suffix <= 254 {
		containers.IPPool.Release(suffix)
	}
}
