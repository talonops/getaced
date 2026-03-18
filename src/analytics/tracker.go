package analytics

import (
	"log"
	"time"

	"getaced.io/src/database"
	"getaced.io/src/structs"
)

const bufferSize = 256

var eventCh chan structs.AnalyticsEvent

// Start initializes the analytics event channel and starts the consumer goroutine.
func Start() {
	eventCh = make(chan structs.AnalyticsEvent, bufferSize)
	go consumer()
	log.Println("analytics started")
}

// Stop closes the event channel, allowing the consumer to drain and exit.
func Stop() {
	if eventCh != nil {
		close(eventCh)
	}
}

// Option configures optional fields on an AnalyticsEvent.
type Option func(*structs.AnalyticsEvent)

func WithStep(step string) Option {
	return func(e *structs.AnalyticsEvent) { e.StepName = step }
}

func WithPlan(plan string) Option {
	return func(e *structs.AnalyticsEvent) { e.PlanType = plan }
}

func WithAmount(cents int) Option {
	return func(e *structs.AnalyticsEvent) { e.Amount = cents }
}

func WithTokens(input, output int) Option {
	return func(e *structs.AnalyticsEvent) {
		e.TokensInput = input
		e.TokensOutput = output
	}
}

// Track sends an analytics event to the buffered channel.
// Never blocks the caller — if the channel is full the event is dropped and logged.
func Track(event string, userID uint, opts ...Option) {
	e := structs.AnalyticsEvent{
		CreatedAt: time.Now().UTC(),
		UserID:    userID,
		Event:     event,
	}
	for _, o := range opts {
		o(&e)
	}
	select {
	case eventCh <- e:
	default:
		log.Printf("analytics: channel full, dropping event %s for user %d", event, userID)
	}
}

// TrackUserActive emits a user_active event deduplicated to once per hour per user.
// Safe to call from a goroutine — never returns errors to the caller.
func TrackUserActive(userID uint) {
	var user structs.User
	if err := database.DB.Select("last_active_at").First(&user, userID).Error; err != nil {
		return
	}
	now := time.Now().UTC()
	if user.LastActiveAt != nil && now.Sub(*user.LastActiveAt) < time.Hour {
		return
	}
	database.DB.Model(&structs.User{}).Where("id = ?", userID).Update("last_active_at", &now)
	Track("user_active", userID)
}

func consumer() {
	for e := range eventCh {
		if err := database.DB.Create(&e).Error; err != nil {
			log.Printf("analytics: failed to write event %s: %v", e.Event, err)
		}
	}
}
