package handler

import (
	"fmt"
	"log"
	"time"

	"getaced.io/src/config"
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

	// Extract customer ID — can be nested under "customer" object or flat "customer_id"
	var customerID string
	if customer, ok := obj["customer"].(map[string]interface{}); ok {
		customerID, _ = customer["id"].(string)
	}
	if customerID == "" {
		customerID, _ = obj["customer_id"].(string)
	}
	if customerID == "" {
		customerID, _ = obj["customer"].(string)
	}

	// Extract subscription ID — can be nested under "subscription" object or flat "id"
	var subscriptionID string
	if sub, ok := obj["subscription"].(map[string]interface{}); ok {
		subscriptionID, _ = sub["id"].(string)
	}
	if subscriptionID == "" {
		subscriptionID, _ = obj["id"].(string)
	}

	log.Printf("creem webhook: event=%s customer=%s subscription=%s request_id=%s", event.EventType, customerID, subscriptionID, requestID)

	if customerID == "" && requestID == "" {
		return c.SendStatus(fiber.StatusOK)
	}

	// Idempotency check: skip if already processed
	eventKey := event.EventType + ":" + subscriptionID
	var existing structs.ProcessedWebhookEvent
	if err := database.DB.Where("event_id = ?", eventKey).First(&existing).Error; err == nil {
		return c.SendStatus(fiber.StatusOK) // already processed
	}

	// Try to find user by creem_customer_id first, then by request_id
	var user structs.User
	var err error
	if customerID != "" {
		err = database.DB.Where("creem_customer_id = ?", customerID).First(&user).Error
	}
	if err != nil && requestID != "" {
		err = database.DB.Where("id = ?", requestID).First(&user).Error
	}

	if err != nil {
		return c.SendStatus(fiber.StatusOK)
	}

	// Process in a transaction for atomicity
	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		switch event.EventType {
		case "checkout.completed":
			now := time.Now()
			tx.Model(&user).Updates(map[string]interface{}{
				"creem_customer_id":    customerID,
				"subscription_id":      subscriptionID,
				"subscription_status":  "active",
				"subscription_paid_at": &now,
			})
		case "subscription.active", "subscription.paid":
			now := time.Now()
			tx.Model(&user).Updates(map[string]interface{}{
				"subscription_status":  "active",
				"subscription_paid_at": &now,
			})
		case "subscription.trialing":
			tx.Model(&user).Update("subscription_status", "trialing")
		case "subscription.canceled":
			tx.Model(&user).Update("subscription_status", "canceled")
			go worker.StopUserWatch(user)
		case "subscription.past_due":
			tx.Model(&user).Update("subscription_status", "past_due")
		case "subscription.expired":
			tx.Model(&user).Update("subscription_status", "expired")
			go worker.StopUserWatch(user)
		}

		// Record event as processed
		tx.Create(&structs.ProcessedWebhookEvent{EventID: eventKey})
		return nil
	})

	if txErr != nil {
		log.Printf("creem webhook transaction error: %v", txErr)
	}

	return c.SendStatus(fiber.StatusOK)
}
