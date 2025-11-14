package main

import (
	"context"
	"log"
	"strings"
	"time"

	"google.golang.org/api/calendar/v3"
)

// Syncer handles the synchronization logic between work and personal calendars.
type Syncer struct {
	workClient     CalendarClient
	personalClient CalendarClient
	config         *Config
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(workClient, personalClient CalendarClient, config *Config) *Syncer {
	return &Syncer{
		workClient:     workClient,
		personalClient: personalClient,
		config:         config,
	}
}

// filterEvents applies the filtering rules from the spec:
// - Keep all-day events (even OOF)
// - Skip timed OOF events
// - Skip events entirely outside 6:00 AM - 12:00 AM (midnight)
// - Keep any event that partially overlaps the window
func (s *Syncer) filterEvents(events []*calendar.Event) []*calendar.Event {
	var filtered []*calendar.Event

	for _, event := range events {
		// Rule 1: Keep all all-day events (even OOF)
		if event.Start.Date != "" {
			filtered = append(filtered, event)
			continue
		}

		// Rule 2: Skip timed OOF events
		// For recurring event instances, check the parent event's transparency
		if isOutOfOffice(event, s.workClient) {
			continue
		}

		// Rule 3: Check time window (6:00 AM - 12:00 AM)
		// Parse the start and end times
		startTime, err := time.Parse(time.RFC3339, event.Start.DateTime)
		if err != nil {
			log.Printf("Warning: failed to parse event start time: %v", err)
			continue
		}

		endTime, err := time.Parse(time.RFC3339, event.End.DateTime)
		if err != nil {
			log.Printf("Warning: failed to parse event end time: %v", err)
			continue
		}

		// Get the local time components
		startHour := startTime.Hour()
		startMinute := startTime.Minute()
		endHour := endTime.Hour()
		endMinute := endTime.Minute()

		// Convert to minutes since midnight for easier comparison
		startMinutes := startHour*60 + startMinute
		endMinutes := endHour*60 + endMinute

		// Window: 6:00 AM (360 minutes) to 12:00 AM (1440 minutes, which is midnight of next day)
		windowStart := 6 * 60 // 6:00 AM
		windowEnd := 24 * 60  // 12:00 AM (midnight)

		// Check if event overlaps with the window
		// Event overlaps if:
		// - Start is within window, OR
		// - End is within window, OR
		// - Event spans the entire window
		overlaps := (startMinutes >= windowStart && startMinutes < windowEnd) ||
			(endMinutes > windowStart && endMinutes <= windowEnd) ||
			(startMinutes < windowStart && endMinutes > windowEnd)

		if overlaps {
			filtered = append(filtered, event)
		}
	}

	return filtered
}

// isOutOfOffice checks if an event is marked as "Out of Office".
// Uses multiple methods in order of reliability:
// 1. EventType field (most reliable - explicitly set by Google Calendar)
// 2. Transparency field (fallback - indicates free/busy status)
// 3. Parent event check (for recurring event instances)
// 4. Keyword matching in summary (last resort)
func isOutOfOffice(event *calendar.Event, client CalendarClient) bool {
	// Primary check: EventType field is the most reliable indicator
	// Google Calendar sets this to "outOfOffice" for OOF events
	if event.EventType == "outOfOffice" {
		return true
	}

	// For recurring event instances, check the parent event's EventType first
	if event.RecurringEventId != "" {
		parentEvent, err := client.GetEvent("primary", event.RecurringEventId)
		if err == nil && parentEvent != nil {
			// Check parent's EventType first (most reliable)
			if parentEvent.EventType == "outOfOffice" {
				return true
			}
			// Fallback to parent's transparency
			if parentEvent.Transparency == "transparent" {
				return true
			}
		}
		// If we can't fetch the parent, fall through to other checks
	}

	// Secondary check: Transparency field (indicates free/busy, not specifically OOF)
	// This is less reliable but useful for older events or events created differently
	if event.Transparency == "transparent" {
		return true
	}

	// Tertiary check: Keyword matching in summary (last resort)
	// Useful for events that might be OOF but not properly marked
	summary := strings.ToLower(event.Summary)
	oofKeywords := []string{"out of office", "oof", "pto", "vacation", "away"}
	for _, keyword := range oofKeywords {
		if strings.Contains(summary, keyword) {
			return true
		}
	}

	return false
}

// prepareSyncEvent creates a new calendar.Event for the personal calendar
// based on the source work event.
func (s *Syncer) prepareSyncEvent(sourceEvent *calendar.Event) *calendar.Event {
	destEvent := &calendar.Event{
		Summary:        sourceEvent.Summary,
		Description:    sourceEvent.Description,
		Location:       sourceEvent.Location,
		Start:          sourceEvent.Start,
		End:            sourceEvent.End,
		ConferenceData: sourceEvent.ConferenceData,
		// Omit attendees (guest list)
		// Set reminders to use default
		Reminders: &calendar.EventReminders{
			UseDefault: true,
		},
		// Set extended properties to track the work event ID
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": sourceEvent.Id,
			},
		},
	}

	return destEvent
}

// eventsEqual checks if two events have the same key properties.
func eventsEqual(event1, event2 *calendar.Event) bool {
	if event1.Summary != event2.Summary {
		return false
	}

	if event1.Description != event2.Description {
		return false
	}

	if event1.Location != event2.Location {
		return false
	}

	// Compare start times
	start1 := event1.Start.DateTime
	if start1 == "" {
		start1 = event1.Start.Date
	}
	start2 := event2.Start.DateTime
	if start2 == "" {
		start2 = event2.Start.Date
	}
	if start1 != start2 {
		return false
	}

	// Compare end times
	end1 := event1.End.DateTime
	if end1 == "" {
		end1 = event1.End.Date
	}
	end2 := event2.End.DateTime
	if end2 == "" {
		end2 = event2.End.Date
	}
	if end1 != end2 {
		return false
	}

	return true
}

// Sync performs the main synchronization logic.
func (s *Syncer) Sync(ctx context.Context) error {
	log.Println("Starting sync...")

	// Find or create the destination calendar
	destCalendarID, err := s.personalClient.FindOrCreateCalendarByName(s.config.SyncCalendarName, s.config.SyncCalendarColorID)
	if err != nil {
		return err
	}

	// Calculate time window: start of current week (Monday) to end of next week (Sunday)
	now := time.Now()

	// Find the start of the current week (Monday)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	daysFromMonday := weekday - 1
	startOfCurrentWeek := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfCurrentWeek = startOfCurrentWeek.AddDate(0, 0, -daysFromMonday)

	// End of next week (Sunday at 23:59:59)
	endOfNextWeek := startOfCurrentWeek.AddDate(0, 0, 13) // 7 days (current week) + 7 days (next week) - 1 day
	endOfNextWeek = time.Date(endOfNextWeek.Year(), endOfNextWeek.Month(), endOfNextWeek.Day(), 23, 59, 59, 0, endOfNextWeek.Location())

	timeMin := startOfCurrentWeek
	timeMax := endOfNextWeek

	// Get source events from work calendar
	sourceEvents, err := s.workClient.GetEvents("primary", timeMin, timeMax)
	if err != nil {
		return err
	}

	// Filter events according to spec
	filteredEvents := s.filterEvents(sourceEvents)

	// Create a map of filtered events by ID for easy lookup
	sourceEventsMap := make(map[string]*calendar.Event)
	for _, event := range filteredEvents {
		sourceEventsMap[event.Id] = event
	}

	// Get destination events from personal calendar
	destEvents, err := s.personalClient.GetEvents(destCalendarID, timeMin, timeMax)
	if err != nil {
		return err
	}

	// Process existing destination events
	for _, destEvent := range destEvents {
		// Get the work event ID from extended properties
		workID := ""
		if destEvent.ExtendedProperties != nil && destEvent.ExtendedProperties.Private != nil {
			workID = destEvent.ExtendedProperties.Private["workEventId"]
		}

		if workID == "" {
			// This event doesn't have a workEventId, skip it (might be manually created)
			continue
		}

		sourceEvent, exists := sourceEventsMap[workID]

		if exists {
			// Event exists in source (Update/Check)
			// Check if the event has changed
			preparedEvent := s.prepareSyncEvent(sourceEvent)
			if !eventsEqual(destEvent, preparedEvent) {
				// Event has changed, update it
				if err := s.personalClient.UpdateEvent(destCalendarID, destEvent.Id, preparedEvent); err != nil {
					log.Printf("Warning: failed to update event %s: %v", destEvent.Id, err)
				}
			}
			// Remove from map to mark as processed
			delete(sourceEventsMap, workID)
		} else {
			// Event doesn't exist in source (Delete Stale)
			// This event is on the personal calendar but not in the source
			if err := s.personalClient.DeleteEvent(destCalendarID, destEvent.Id); err != nil {
				log.Printf("Warning: failed to delete event %s: %v", destEvent.Id, err)
			}
		}
	}

	// Process remaining events in sourceEventsMap (these are new)
	for _, newEvent := range sourceEventsMap {
		preparedEvent := s.prepareSyncEvent(newEvent)
		if err := s.personalClient.InsertEvent(destCalendarID, preparedEvent); err != nil {
			log.Printf("Warning: failed to insert event %s: %v", newEvent.Id, err)
		}
	}

	log.Println("Sync complete.")
	return nil
}
