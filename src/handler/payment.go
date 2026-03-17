package handler

import (
	"fmt"
	"log"
	"time"

	"getaced.io/src/config"
	"getaced.io/src/containers"
	"getaced.io/src/creem"
	"getaced.io/src/database"
	"getaced.io/src/structs"
	"getaced.io/src/worker"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"
)

var CreemClient *creem.Client

func CreateCheckout(c fiber.Ctx) error {
	userID := c.Locals("user_id").(uint)

	var body struct {
		ProductID  string `json:"product_id"`
		SuccessURL string `json:"success_url"`
	}
	if err := c.Bind().JSON(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid request body"})
	}

	checkout, err := CreemClient.CreateCheckout(structs.CheckoutRequest{
		ProductID:  body.ProductID,
		SuccessURL: body.SuccessURL,
		RequestID:  fmt.Sprintf("%d", userID),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to create checkout"})
	}

	return c.JSON(fiber.Map{"checkout_url": checkout.CheckoutURL})
}

func CreemWebhook(c fiber.Ctx) error {
	signature := c.Get("creem-signature")
	body := c.Body()

	if !creem.VerifySignature(body, signature, config.Config("CREEM_WEBHOOK_SECRET")) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "invalid signature"})
	}

	var event structs.WebhookEvent
	if err := c.Bind().JSON(&event); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid event payload"})
	}

	obj := event.Object
	requestID, _ := obj["request_id"].(string)

	// Extract customer ID from nested customer object
	var customerID string
	if customer, ok := obj["customer"].(map[string]interface{}); ok {
		customerID, _ = customer["id"].(string)
	}

	// Extract subscription ID — nested object on checkout, or the object itself on subscription events
	var subscriptionID string
	if sub, ok := obj["subscription"].(map[string]interface{}); ok {
		subscriptionID, _ = sub["id"].(string)
	} else if objType, _ := obj["object"].(string); objType == "subscription" {
		subscriptionID, _ = obj["id"].(string)
	}

	log.Printf("creem webhook: event=%s customer=%s subscription=%s request_id=%s event_id=%s", event.EventType, customerID, subscriptionID, requestID, event.ID)

	if customerID == "" && requestID == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	// Idempotency check using Creem's unique event ID
	if event.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "missing event id"})
	}
	var existing structs.ProcessedWebhookEvent
	if err := database.DB.Where("event_id = ?", event.ID).First(&existing).Error; err == nil {
		return c.SendStatus(fiber.StatusOK) // already processed
	}

	// Try to find user by creem_customer_id first, then by request_id
	var user structs.User
	found := false
	if customerID != "" {
		if err := database.DB.Where("creem_customer_id = ?", customerID).First(&user).Error; err == nil {
			found = true
		}
	}
	if !found && requestID != "" {
		if err := database.DB.Where("id = ?", requestID).First(&user).Error; err == nil {
			found = true
		}
	}

	if !found {
		log.Printf("creem webhook: user not found for customer=%s request_id=%s", customerID, requestID)
		return c.SendStatus(fiber.StatusOK)
	}

	// Track whether we need to clean up user resources after the transaction
	var shouldCleanup bool

	// Process in a transaction for atomicity
	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		switch event.EventType {
		case "checkout.completed":
			now := time.Now()
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"creem_customer_id":    customerID,
				"subscription_id":      subscriptionID,
				"subscription_status":  "active",
				"subscription_paid_at": &now,
			}).Error; err != nil {
				return err
			}
		case "subscription.active", "subscription.paid":
			now := time.Now()
			if err := tx.Model(&user).Updates(map[string]interface{}{
				"subscription_status":  "active",
				"subscription_paid_at": &now,
			}).Error; err != nil {
				return err
			}
		case "subscription.trialing":
			if err := tx.Model(&user).Update("subscription_status", "trialing").Error; err != nil {
				return err
			}
		case "subscription.canceled":
			if err := tx.Model(&user).Update("subscription_status", "canceled").Error; err != nil {
				return err
			}
			shouldCleanup = true
		case "subscription.past_due":
			if err := tx.Model(&user).Update("subscription_status", "past_due").Error; err != nil {
				return err
			}
		case "subscription.expired":
			if err := tx.Model(&user).Update("subscription_status", "expired").Error; err != nil {
				return err
			}
			shouldCleanup = true
		}

		// Record event as processed
		if err := tx.Create(&structs.ProcessedWebhookEvent{EventID: event.ID}).Error; err != nil {
			return err
		}
		return nil
	})

	if txErr != nil {
		log.Printf("creem webhook transaction error: %v", txErr)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to process webhook"})
	}

	// Only clean up resources after the transaction has committed successfully
	if shouldCleanup {
		go func() {
			worker.StopUserWatch(user)
			containers.DeleteUserContainer(user.ID)
		}()
	}

	return c.SendStatus(fiber.StatusOK)
}
