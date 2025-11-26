package sync

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/beekhof/calendar-sync/internal/auth"
	calclient "github.com/beekhof/calendar-sync/internal/calendar"
	"github.com/beekhof/calendar-sync/internal/config"
	"golang.org/x/term"

	"google.golang.org/api/calendar/v3"
)

// Syncer handles the synchronization logic between work and personal calendars.
type Syncer struct {
	workClient     calclient.CalendarClient
	personalClient calclient.CalendarClient
	config         *config.Config
	destination    *config.Destination // Destination-specific config (calendar name, color, etc.)
	verbose        bool                // Enable verbose DEBUG logging
}

// NewSyncer creates a new Syncer instance.
func NewSyncer(workClient, personalClient calclient.CalendarClient, cfg *config.Config, dest *config.Destination, verbose bool) *Syncer {
	return &Syncer{
		workClient:     workClient,
		personalClient: personalClient,
		config:         cfg,
		destination:    dest,
		verbose:        verbose,
	}
}

// debugLog logs a message only if verbose mode is enabled.
func (s *Syncer) debugLog(format string, v ...interface{}) {
	if s.verbose {
		log.Printf("DEBUG: "+format, v...)
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
// Returns (equal, fieldName) where fieldName is the name of the field that differs,
// or empty string if the events are equal.
// debugLog is an optional function for verbose logging (can be nil).
func eventsEqual(event1, event2 *calendar.Event, debugLog func(string, ...interface{})) (bool, string) {
	if event1.Summary != event2.Summary {
		if debugLog != nil {
			debugLog("summary mismatch: %v != %v", event1.Summary, event2.Summary)
		}
		return false, "summary"
	}

	if event1.Description != event2.Description {
		if debugLog != nil {
			debugLog("description mismatch: %v != %v", event1.Description, event2.Description)
		}
		return false, "description"
	}

	if event1.Location != event2.Location {
		if debugLog != nil {
			debugLog("location mismatch: %v != %v", event1.Location, event2.Location)
		}
		return false, "location"
	}

	// Compare start times (normalize timezones for DateTime comparisons)
	if equal, field := timesEqual(event1.Start, event2.Start, "start", debugLog); !equal {
		return false, field
	}

	// Compare end times (normalize timezones for DateTime comparisons)
	if equal, field := timesEqual(event1.End, event2.End, "end", debugLog); !equal {
		return false, field
	}

	// Compare conference data (Google Meet links)
	meetURL1 := getMeetURL(event1)
	meetURL2 := getMeetURL(event2)
	if meetURL1 != meetURL2 {
		if debugLog != nil {
			debugLog("conference data mismatch: %v != %v", meetURL1, meetURL2)
		}
		return false, "conference"
	}

	return true, ""
}

// checkAndCreateTokenReminder checks OAuth token expiration and creates/updates reminder events.
// This is only applicable for Google Calendar destinations that use OAuth tokens.
func (s *Syncer) checkAndCreateTokenReminder(ctx context.Context, destCalendarID string) error {
	// Load the token to check expiration
	tokenStore := auth.NewFileTokenStore(s.destination.TokenPath)
	token, err := tokenStore.LoadToken()
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if token == nil {
		// No token yet (first run), skip reminder
		return nil
	}

	now := time.Now()

	// Determine when the token was last refreshed by checking the token file's modification time
	// This gives us a better estimate than assuming 6 months
	tokenFileInfo, err := os.Stat(s.destination.TokenPath)
	if err != nil {
		// Can't determine file modification time, use conservative estimate
		return fmt.Errorf("failed to stat token file: %w", err)
	}
	tokenLastModified := tokenFileInfo.ModTime()

	// For Google OAuth refresh tokens:
	// - If app is in "Testing" mode: tokens expire after 7 days
	// - If app is "In production": tokens can last up to 6 months of inactivity
	// Since we can't determine the app status, we'll use the token file's modification time
	// and estimate conservatively. If the token was modified recently (within last 7 days),
	// assume it's a new token and estimate 7 days from modification time.
	// Otherwise, assume it's a production token and estimate 6 months from modification time.
	daysSinceModification := int(now.Sub(tokenLastModified).Hours() / 24)

	var estimatedRefreshTokenExpiry time.Time
	var expiryReason string

	if daysSinceModification < 7 {
		// Token was recently created/modified - likely in testing mode or newly issued
		// Estimate 7 days from when it was last modified
		estimatedRefreshTokenExpiry = tokenLastModified.AddDate(0, 0, 7)
		expiryReason = "7 days from last refresh (testing mode estimate)"
	} else {
		// Token is older - likely in production mode
		// Estimate 6 months from when it was last modified
		estimatedRefreshTokenExpiry = tokenLastModified.AddDate(0, 6, 0)
		expiryReason = "6 months from last refresh (production mode estimate)"
	}

	// Create reminder 2 days before estimated expiry (or 1 day if expiry is very soon)
	daysUntilExpiry := int(estimatedRefreshTokenExpiry.Sub(now).Hours() / 24)
	var reminderDate time.Time
	if daysUntilExpiry > 2 {
		reminderDate = estimatedRefreshTokenExpiry.AddDate(0, 0, -2) // 2 days before
	} else if daysUntilExpiry > 0 {
		reminderDate = now.AddDate(0, 0, 1) // Tomorrow if expiry is very soon
	} else {
		// Token has already expired or expires today - create reminder for today
		reminderDate = now
	}

	// Only create reminder if it's in the future and within reasonable timeframe
	if reminderDate.Before(now) {
		// Reminder date is in the past, skip
		return nil
	}
	if reminderDate.After(now.AddDate(0, 6, 0)) {
		// Reminder is too far in the future, skip
		return nil
	}

	// Log token expiration info
	log.Printf("[%s] OAuth grant estimated to expire: %s (reminder set for: %s) - %s",
		s.destination.Name,
		estimatedRefreshTokenExpiry.Format("2006-01-02"),
		reminderDate.Format("2006-01-02"),
		expiryReason)

	// Check if a reminder event already exists
	reminderWorkID := "TOKEN_REFRESH_REMINDER"
	existingReminders, err := s.personalClient.FindEventsByWorkID(destCalendarID, reminderWorkID)
	if err != nil {
		return fmt.Errorf("failed to find existing reminder events: %w", err)
	}

	// Create or update the reminder event
	reminderEvent := &calendar.Event{
		Summary: "⚠️ Refresh OAuth Token for Calendar Sync",
		Description: fmt.Sprintf(
			"Your OAuth token for '%s' is estimated to expire on %s (%s).\n\n"+
				"To refresh your token:\n"+
				"1. Run the calendar sync tool manually\n"+
				"2. You will be prompted to re-authenticate if needed\n"+
				"3. The token will be automatically refreshed\n\n"+
				"Note: If your OAuth app is in 'Testing' mode, tokens expire after 7 days.\n"+
				"Move your app to 'In production' in Google Cloud Console for longer-lived tokens.\n\n"+
				"This reminder will be updated on the next sync.",
			s.destination.Name,
			estimatedRefreshTokenExpiry.Format("January 2, 2006"),
			expiryReason,
		),
		Start: &calendar.EventDateTime{
			DateTime: reminderDate.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: reminderDate.Add(1 * time.Hour).Format(time.RFC3339),
		},
		Reminders: &calendar.EventReminders{
			UseDefault: true,
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": reminderWorkID,
			},
		},
	}

	if len(existingReminders) > 0 {
		// Update existing reminder
		existingReminder := existingReminders[0]
		if err := s.personalClient.UpdateEvent(destCalendarID, existingReminder.Id, reminderEvent); err != nil {
			return fmt.Errorf("failed to update reminder event: %w", err)
		}
		s.debugLog("Updated token refresh reminder event (ID: %s)", existingReminder.Id)
	} else {
		// Create new reminder
		if err := s.personalClient.InsertEvent(destCalendarID, reminderEvent); err != nil {
			return fmt.Errorf("failed to create reminder event: %w", err)
		}
		s.debugLog("Created token refresh reminder event")
	}

	return nil
}

// timesEqual compares two EventDateTime values, normalizing timezones for DateTime comparisons.
// For all-day events (Date field), it compares the date strings directly.
// For timed events (DateTime field), it parses and compares the times in UTC.
// Returns (equal, fieldName) where fieldName is the field name (e.g., "start" or "end") if different,
// or empty string if equal.
func timesEqual(dt1, dt2 *calendar.EventDateTime, fieldName string, debugLog func(string, ...interface{})) (bool, string) {
	// Handle nil cases
	if dt1 == nil && dt2 == nil {
		return true, ""
	}
	if dt1 == nil || dt2 == nil {
		if debugLog != nil {
			debugLog("%s time mismatch: one is nil, other is not", fieldName)
		}
		return false, fieldName
	}

	// Check if both are all-day events (Date field)
	if dt1.Date != "" && dt2.Date != "" {
		// Both are all-day events - compare date strings directly
		if dt1.Date != dt2.Date {
			if debugLog != nil {
				debugLog("%s date mismatch: %v != %v", fieldName, dt1.Date, dt2.Date)
			}
			return false, fieldName
		}
		return true, ""
	}

	// Check if both are timed events (DateTime field)
	if dt1.DateTime != "" && dt2.DateTime != "" {
		// Parse both times and compare in UTC
		t1, err1 := time.Parse(time.RFC3339, dt1.DateTime)
		t2, err2 := time.Parse(time.RFC3339, dt2.DateTime)
		if err1 != nil || err2 != nil {
			// If parsing fails, fall back to string comparison
			if dt1.DateTime != dt2.DateTime {
				if debugLog != nil {
					debugLog("%s time mismatch (parse failed): %v != %v", fieldName, dt1.DateTime, dt2.DateTime)
				}
				return false, fieldName
			}
			return true, ""
		}
		// Compare in UTC to normalize timezones
		if !t1.UTC().Equal(t2.UTC()) {
			if debugLog != nil {
				debugLog("%s time mismatch: %v (UTC: %v) != %v (UTC: %v)", fieldName,
					dt1.DateTime, t1.UTC(), dt2.DateTime, t2.UTC())
			}
			return false, fieldName
		}
		return true, ""
	}

	// One is Date, other is DateTime - they don't match
	if debugLog != nil {
		debugLog("%s time type mismatch: one is Date (%v), other is DateTime (%v)", fieldName,
			dt1.Date != "", dt2.Date != "")
	}
	return false, fieldName
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

// isInteractive checks if the program is running in an interactive terminal.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// promptForConfirmation prompts the user for confirmation and returns true if they confirm.
// Only prompts if running in an interactive terminal. In non-interactive mode, returns false.
func promptForConfirmation(message string) bool {
	if !isInteractive() {
		// Running headless (e.g., cron job) - don't prompt, just log and return false
		log.Printf("WARNING: Running in non-interactive mode. Skipping confirmation prompt.")
		log.Printf("WARNING: %s", message)
		return false
	}

	fmt.Fprintf(os.Stderr, "\n%s\n", message)
	fmt.Fprint(os.Stderr, "Do you want to continue? (yes/no): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}

	response := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return response == "yes" || response == "y"
}

// Sync performs the main synchronization logic.
func (s *Syncer) Sync(ctx context.Context) error {
	destName := s.destination.Name
	log.Printf("[%s] Starting sync...", destName)

	// Find or create the destination calendar
	destCalendarID, err := s.personalClient.FindOrCreateCalendarByName(s.destination.CalendarName, s.destination.CalendarColorID)
	if err != nil {
		return err
	}

	// Check token expiration and create reminder events for Google destinations
	if s.destination.Type == "google" {
		if err := s.checkAndCreateTokenReminder(ctx, destCalendarID); err != nil {
			// Log but don't fail the sync if reminder creation fails
			log.Printf("[%s] Warning: Failed to check/create token refresh reminder: %v", destName, err)
		}
	}

	// Check if calendar has manually created events (without workEventId) and prompt for confirmation
	// Only prompt if there are events that don't have workEventId - these will be deleted
	// Events with workEventId are expected (previously synced) and don't need confirmation
	checkNow := time.Now()
	wideTimeMin := checkNow.AddDate(-1, 0, 0) // 1 year ago
	wideTimeMax := checkNow.AddDate(1, 0, 0)  // 1 year from now
	existingEvents, err := s.personalClient.GetEvents(destCalendarID, wideTimeMin, wideTimeMax)
	if err != nil {
		// If we can't check for events, log a warning but continue
		log.Printf("[%s] Warning: Could not check for existing events: %v", destName, err)
	} else {
		// Count manually created events (those without workEventId)
		manuallyCreatedCount := 0
		for _, event := range existingEvents {
			workID := ""
			if event.ExtendedProperties != nil && event.ExtendedProperties.Private != nil {
				workID = event.ExtendedProperties.Private["workEventId"]
			}
			if workID == "" {
				manuallyCreatedCount++
			}
		}

		if manuallyCreatedCount > 0 {
			// Calendar has manually created events - prompt for confirmation
			message := fmt.Sprintf(
				"\n⚠️  WARNING: The calendar '%s' contains %d manually created event(s) (without workEventId).\n"+
					"This tool will DELETE these events as they are not present in your work calendar.\n\n"+
					"Are you sure you want to proceed?",
				s.destination.CalendarName, manuallyCreatedCount)

			if !promptForConfirmation(message) {
				return fmt.Errorf("sync cancelled by user")
			}
			log.Printf("[%s] User confirmed - proceeding with sync", destName)
		}
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
	wideTimeMinForSync := timeMin.AddDate(0, -6, 0)
	wideTimeMaxForSync := timeMax.AddDate(0, 6, 0)
	destEvents, err := s.personalClient.GetEvents(destCalendarID, wideTimeMinForSync, wideTimeMaxForSync)
	if err != nil {
		return err
	}

	log.Printf("Retrieved %d destination events (wide range: %s to %s) for duplicate detection",
		len(destEvents), wideTimeMinForSync.Format("2006-01-02"), wideTimeMaxForSync.Format("2006-01-02"))

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
			s.debugLog("Found event '%s' with normalized_start=%s, actual_start=%s, workEventId=%q, ID=%s",
				destEvent.Summary, destEvent.Start.DateTime, actualStart, workID, destEvent.Id)
		}

		if workID == "" {
			// This event doesn't have a workEventId - it was manually created
			// Per spec: Work calendar is the source of truth, so manually created events should be deleted
			eventsWithoutWorkID = append(eventsWithoutWorkID, destEvent)
			continue
		}

		if len(destEventsByWorkID[workID]) > 0 {
			s.debugLog("found duplicate event %s (summary: %v)", destEvent.Id, destEvent.Summary)
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
			s.debugLog("found matched event %s (summary: %v)", destEvent.Id, destEvent.Summary)

			// Check if the event has changed
			preparedEvent := s.prepareSyncEvent(sourceEvent)
			equal, diffField := eventsEqual(destEvent, preparedEvent, s.debugLog)
			if !equal {
				// Event has changed, update it
				if err := s.personalClient.UpdateEvent(destCalendarID, destEvent.Id, preparedEvent); err != nil {
					log.Printf("Warning: failed to update event %s (summary: %v, changed field: %s): %v", destEvent.Id, preparedEvent.Summary, diffField, err)
				} else {
					log.Printf("Updated event %s (workEventId: %s, summary: %v, changed field: %s)", destEvent.Id, workID, preparedEvent.Summary, diffField)
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

	log.Printf("[%s] Sync complete.", destName)
	return nil
}
