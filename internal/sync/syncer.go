package sync

import (
	"context"
	"log"
	"strings"
	"time"

	calclient "calendar-sync/internal/calendar"
	"calendar-sync/internal/config"

	"google.golang.org/api/calendar/v3"
)

// Syncer handles the synchronization logic between work and personal calendars.
type Syncer struct {
	workClient     calclient.CalendarClient
	personalClient calclient.CalendarClient
	config         *config.Config
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(workClient, personalClient calclient.CalendarClient, cfg *config.Config) *Syncer {
	return &Syncer{
		workClient:     workClient,
		personalClient: personalClient,
		config:         cfg,
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
		// Rule 1: Handle all-day events
		if event.Start.Date != "" {
			// Skip all-day events that indicate work location
			if isWorkLocationEvent(event) {
				continue
			}
			// Keep all other all-day events (including OOF)
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

// isWorkLocationEvent checks if an all-day event indicates work location.
// Google Calendar uses all-day events to indicate work location (e.g., "Remote", "Office").
// This excludes OOF events which may also contain "office" in the summary.
func isWorkLocationEvent(event *calendar.Event) bool {
	if event.Start == nil || event.Start.Date == "" {
		// Not an all-day event
		return false
	}

	summary := strings.ToLower(event.Summary)
	if summary == "" {
		return false
	}

	// Exclude OOF events - they should not be filtered as work location
	if strings.Contains(summary, "out of office") || strings.Contains(summary, "oof") {
		return false
	}

	// Common patterns for work location events
	// These are more specific patterns that indicate work location, not OOF
	locationPatterns := []string{
		"remote",
		"working from",
		"work from home",
		"work from office",
		"wfh", // work from home
		"wfo", // work from office
		"work location",
	}

	// Check for "office" only if it's not part of "out of office"
	if strings.Contains(summary, "office") && !strings.Contains(summary, "out of") {
		return true
	}

	// Check other location patterns
	for _, pattern := range locationPatterns {
		if strings.Contains(summary, pattern) {
			return true
		}
	}

	return false
}

// isOutOfOffice checks if an event is marked as "Out of Office".
// Uses multiple methods in order of reliability:
// 1. EventType field (most reliable - explicitly set by Google Calendar)
// 2. Transparency field (fallback - indicates free/busy status)
// 3. Parent event check (for recurring event instances)
// 4. Keyword matching in summary (last resort)
func isOutOfOffice(event *calendar.Event, client calclient.CalendarClient) bool {
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
		log.Printf("DEBUG: summary mismatch: %v != %v", event1.Summary, event2.Summary)
		return false
	}

	if event1.Description != event2.Description {
		// skip this check for now
		// It causes problems with escaping characters in the description
		//log.Printf("DEBUG: description mismatch: %v != %v", event1.Description, event2.Description)
		//return false
	}

	if event1.Location != event2.Location {
		log.Printf("DEBUG: location mismatch: %v != %v", event1.Location, event2.Location)
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
		log.Printf("DEBUG: start time mismatch: %v != %v", start1, start2)
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
		log.Printf("DEBUG: end time mismatch: %v != %v", end1, end2)
		return false
	}

	// Compare conference data (Google Meet links)
	meetURL1 := getMeetURL(event1)
	meetURL2 := getMeetURL(event2)
	if meetURL1 != meetURL2 {
		log.Printf("DEBUG: conference data mismatch: %v != %v", meetURL1, meetURL2)
		return false
	}

	return true
}

// getMeetURL extracts the Google Meet URL from an event's conferenceData.
func getMeetURL(event *calendar.Event) string {
	if event.ConferenceData == nil || event.ConferenceData.EntryPoints == nil {
		return ""
	}
	for _, entryPoint := range event.ConferenceData.EntryPoints {
		if entryPoint.EntryPointType == "video" && entryPoint.Uri != "" {
			return entryPoint.Uri
		}
	}
	return ""
}

// Sync performs the main synchronization logic.
func (s *Syncer) Sync(ctx context.Context) error {
	log.Println("Starting sync...")

	// Find or create the destination calendar
	destCalendarID, err := s.personalClient.FindOrCreateCalendarByName(s.config.SyncCalendarName, s.config.SyncCalendarColorID)
	if err != nil {
		return err
	}

	// Calculate time window: from past weeks to future weeks from start of current week
	now := time.Now()

	// Find the start of the current week (Monday)
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday = 7
	}
	daysFromMonday := weekday - 1
	startOfCurrentWeek := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	startOfCurrentWeek = startOfCurrentWeek.AddDate(0, 0, -daysFromMonday)

	// Start of sync window (Monday at 00:00:00 of the first week in the past)
	// If SyncWindowWeeksPast is 0, start from current week
	// If SyncWindowWeeksPast is 1, go back 1 week (so include last week)
	// The start is 7 * SyncWindowWeeksPast days before the current week's Monday
	timeMin := startOfCurrentWeek.AddDate(0, 0, -7*s.config.SyncWindowWeeksPast)

	// End of sync window (Sunday at 23:59:59 of the last week in the future)
	// SyncWindowWeeks weeks means: current week + (SyncWindowWeeks - 1) additional weeks
	// For example, 2 weeks = current week (7 days) + next week (7 days) = 14 days total
	// The last day is Sunday of the last week, which is 7 * SyncWindowWeeks - 1 days from Monday
	timeMax := startOfCurrentWeek.AddDate(0, 0, 7*s.config.SyncWindowWeeks-1)
	timeMax = time.Date(timeMax.Year(), timeMax.Month(), timeMax.Day(), 23, 59, 59, 0, timeMax.Location())

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
	// Use a wider time range to catch duplicates that might have been created in previous runs
	// Search 6 months before and 6 months after the sync window
	wideTimeMin := timeMin.AddDate(0, -6, 0)
	wideTimeMax := timeMax.AddDate(0, 6, 0)
	destEvents, err := s.personalClient.GetEvents(destCalendarID, wideTimeMin, wideTimeMax)
	if err != nil {
		return err
	}

	log.Printf("Retrieved %d destination events (wide range: %s to %s) for duplicate detection",
		len(destEvents), wideTimeMin.Format("2006-01-02"), wideTimeMax.Format("2006-01-02"))

	// Group destination events by workEventId to handle duplicates
	// Use ALL destEvents (wide range) for duplicate detection, not just those in the sync window
	destEventsByWorkID := make(map[string][]*calendar.Event)
	eventsWithoutWorkID := []*calendar.Event{}

	// Use ALL destEvents for duplicate detection (wide range)
	for _, destEvent := range destEvents {
		// Get the work event ID from extended properties
		workID := ""
		if destEvent.ExtendedProperties != nil && destEvent.ExtendedProperties.Private != nil {
			workID = destEvent.ExtendedProperties.Private["workEventId"]
		}

		// Log events with "DR for Virtualization" in the summary for debugging
		if destEvent.Summary != "" && strings.Contains(destEvent.Summary, "DR for Virtualization") {
			workID := ""
			if destEvent.ExtendedProperties != nil && destEvent.ExtendedProperties.Private != nil {
				workID = destEvent.ExtendedProperties.Private["workEventId"]
			}
			actualStart := ""
			if destEvent.Start != nil {
				if destEvent.Start.DateTime != "" {
					actualStart = destEvent.Start.DateTime
				} else if destEvent.Start.Date != "" {
					actualStart = destEvent.Start.Date
				}
			}
			log.Printf("DEBUG: Found event '%s' with normalized_start=%s, actual_start=%s, workEventId=%q, ID=%s",
				destEvent.Summary, destEvent.Start.DateTime, actualStart, workID, destEvent.Id)
		}

		if workID == "" {
			// This event doesn't have a workEventId - it was manually created
			// Per spec: Work calendar is the source of truth, so manually created events should be deleted
			eventsWithoutWorkID = append(eventsWithoutWorkID, destEvent)
			continue
		}

		if len(destEventsByWorkID[workID]) > 0 {
			log.Printf("DEBUG: found duplicate event %s (summary: %v)", destEvent.Id, destEvent.Summary)
		}
		destEventsByWorkID[workID] = append(destEventsByWorkID[workID], destEvent)
	}

	// Delete manually created events (events without workEventId)
	// Per spec: "The Work calendar is the single source of truth"
	if len(eventsWithoutWorkID) > 0 {
		log.Printf("Found %d manually created events (without workEventId), deleting them", len(eventsWithoutWorkID))
		for _, destEvent := range eventsWithoutWorkID {
			if err := s.personalClient.DeleteEvent(destCalendarID, destEvent.Id); err != nil {
				log.Printf("Warning: failed to delete manually created event %s (Summary: %s): %v", destEvent.Id, destEvent.Summary, err)
			} else {
				log.Printf("Deleted manually created event %s (Summary: %s)", destEvent.Id, destEvent.Summary)
			}
		}
	}

	// Process destination events grouped by workEventId
	for workID, allDestEventsWithSameWorkID := range destEventsByWorkID {
		sourceEvent, exists := sourceEventsMap[workID]

		// Filter to only events in the sync window for normal processing
		destEventsWithSameWorkID := []*calendar.Event{}
		for _, event := range allDestEventsWithSameWorkID {
			eventTime := time.Time{}
			if event.Start != nil {
				if event.Start.DateTime != "" {
					if t, err := time.Parse(time.RFC3339, event.Start.DateTime); err == nil {
						eventTime = t
					}
				} else if event.Start.Date != "" {
					if t, err := time.Parse("2006-01-02", event.Start.Date); err == nil {
						eventTime = t
					}
				}
			}
			if !eventTime.IsZero() && (eventTime.After(timeMin) || eventTime.Equal(timeMin)) && eventTime.Before(timeMax) {
				destEventsWithSameWorkID = append(destEventsWithSameWorkID, event)
			}
		}

		if exists {
			// Event exists in source (Update/Check)

			// Use the first (or only) destination event in sync window for update/check
			// If no events in sync window, use the first from wide range
			var destEvent *calendar.Event
			if len(destEventsWithSameWorkID) > 0 {
				destEvent = destEventsWithSameWorkID[0]
			} else {
				destEvent = allDestEventsWithSameWorkID[0]
			}
			log.Printf("DEBUG: found matched event %s (summary: %v)", destEvent.Id, destEvent.Summary)

			// Check if the event has changed
			preparedEvent := s.prepareSyncEvent(sourceEvent)
			if !eventsEqual(destEvent, preparedEvent) {
				// Event has changed, update it
				if err := s.personalClient.UpdateEvent(destCalendarID, destEvent.Id, preparedEvent); err != nil {
					log.Printf("Warning: failed to update event %s (summary: %v): %v", destEvent.Id, preparedEvent.Summary, err)
				} else {
					log.Printf("Updated event %s (workEventId: %s, summary: %v)", destEvent.Id, workID, preparedEvent.Summary)
				}
			}
			// Remove from map to mark as processed
			delete(sourceEventsMap, workID)
		} else {
			// Event doesn't exist in source (Delete Stale)
			// Delete all events with this workEventId since they're no longer in the source (wide range)
			for _, destEvent := range allDestEventsWithSameWorkID {
				if err := s.personalClient.DeleteEvent(destCalendarID, destEvent.Id); err != nil {
					log.Printf("Warning: failed to delete stale event %s (Summary: %s, workEventId: %s): %v", destEvent.Id, destEvent.Summary, workID, err)
				} else {
					log.Printf("Deleted stale event %s (Summary: %s, workEventId: %s)", destEvent.Id, destEvent.Summary, workID)
				}
			}
		}
	}

	// Process remaining events in sourceEventsMap (these are new)
	// Before inserting, check if there's already an event with the same summary+start time
	// This prevents creating duplicates when workEventId matching fails
	for _, newEvent := range sourceEventsMap {
		preparedEvent := s.prepareSyncEvent(newEvent)

		// Check if there's already an event with the same summary and start time
		var existingEvent *calendar.Event

		// Search through all destination events to find a match by summary+start
		destEventsForWorkID := destEventsByWorkID[preparedEvent.ExtendedProperties.Private["workEventId"]]
		if len(destEventsForWorkID) > 1 {
			existingEvent = destEventsForWorkID[0]
			log.Printf("Found %d duplicate events with workEventId %s, deleting them", len(destEventsForWorkID), preparedEvent.ExtendedProperties.Private["workEventId"])
			for _, destEvent := range destEventsForWorkID {
				if err := s.personalClient.DeleteEvent(destCalendarID, destEvent.Id); err != nil {
					log.Printf("Warning: failed to delete duplicate event %s (Summary: %s, workEventId: %s): %v", destEvent.Id, destEvent.Summary, preparedEvent.ExtendedProperties.Private["workEventId"], err)
				} else {
					log.Printf("Deleted duplicate event %s (Summary: %s, workEventId: %s)", destEvent.Id, destEvent.Summary, preparedEvent.ExtendedProperties.Private["workEventId"])
				}
			}

		} else if len(destEventsForWorkID) == 1 {
			existingEvent = destEventsForWorkID[0]
			log.Printf("Found existing event with same workEventId, updating instead of inserting: %s (existing ID: %s, workEventId: %s)",
				preparedEvent.Summary, existingEvent.Id, newEvent.Id)
		} else {
			log.Printf("No existing event found with same workEventId, inserting new event: %s (workEventId: %s)",
				preparedEvent.Summary, newEvent.Id)
		}

		if existingEvent != nil {
			// Update the existing event
			if err := s.personalClient.UpdateEvent(destCalendarID, existingEvent.Id, preparedEvent); err != nil {
				log.Printf("Warning: failed to update existing event %s (preventing duplicate to %v): %v", existingEvent.Id, preparedEvent.Description, err)
				// If update fails, try inserting anyway
				//if err := s.personalClient.InsertEvent(destCalendarID, preparedEvent); err != nil {
				//	log.Printf("Warning: failed to insert event %s: %v", newEvent.Id, err)
				//}
			} else {
				log.Printf("Updated existing event %s to prevent duplicate (workEventId: %s, summary: %v)", existingEvent.Id, newEvent.Id, preparedEvent.Summary)
			}
		} else {
			// No existing event found, safe to insert
			if err := s.personalClient.InsertEvent(destCalendarID, preparedEvent); err != nil {
				log.Printf("Warning: failed to insert event %s (summary: %v): %v", newEvent.Id, preparedEvent.Summary, err)
			} else {
				log.Printf("Inserted new event %s (workEventId: %s, summary: %v)", newEvent.Id, newEvent.Id, preparedEvent.Summary)
			}
		}
	}

	log.Println("Sync complete.")
	return nil
}
