package calendar

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beekhof/calendar-sync/internal/config"

	"google.golang.org/api/calendar/v3"
)

// loadTestConfig loads the config from config.json for testing
func loadTestConfig(t *testing.T) *config.Config {
	configData, err := os.ReadFile("../../config.json")
	if err != nil {
		t.Fatalf("Failed to read config.json: %v", err)
	}

	var cfgData config.Config
	if err := json.Unmarshal(configData, &cfgData); err != nil {
		t.Fatalf("Failed to parse config.json: %v", err)
	}

	// Load the config with environment variable overrides
	loadedConfig, err := config.LoadConfig(
		"../../config.json",   // config file path
		cfgData.WorkTokenPath, // work token path override
		cfgData.WorkEmail,
		cfgData.GoogleCredentialsPath, // google credentials path override
	)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	return loadedConfig
}

// getAppleDestination finds the first Apple destination from the config
func getAppleDestination(t *testing.T, cfg *config.Config) *config.Destination {
	for i := range cfg.Destinations {
		if cfg.Destinations[i].Type == "apple" {
			return &cfg.Destinations[i]
		}
	}
	t.Fatal("No Apple destination found in config")
	return nil
}

// TestAppleCalendar_InsertAndGet tests inserting an event and then retrieving it
func TestAppleCalendar_InsertAndGet(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}
	t.Logf("Using calendar ID: %s", calendarID)

	// Create a test event
	now := time.Now()
	startTime := now.Add(1 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	testEvent := &calendar.Event{
		Id:          "test-event-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Insert and Get",
		Description: "This is a test event for insertion and retrieval",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "test-work-id-insert-get",
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}
	t.Logf("Successfully inserted event with ID: %s", testEvent.Id)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Retrieve events in the time range
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	// Find our test event
	var foundEvent *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == "test-work-id-insert-get" {
			foundEvent = event
			break
		}
	}

	if foundEvent == nil {
		t.Fatalf("Failed to find inserted event. Found %d events in range", len(events))
	}

	if foundEvent.Summary != testEvent.Summary {
		t.Errorf("Event summary mismatch: expected %q, got %q", testEvent.Summary, foundEvent.Summary)
	}

	t.Logf("Successfully retrieved event with ID: %s", foundEvent.Id)

	// Cleanup: delete the test event
	t.Cleanup(func() {
		if err := client.DeleteEvent(calendarID, foundEvent.Id); err != nil {
			t.Logf("Warning: failed to cleanup test event %s: %v", foundEvent.Id, err)
		}
	})
}

// TestAppleCalendar_FindByWorkID tests finding events by workEventId
func TestAppleCalendar_FindByWorkID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event with a specific workEventId
	now := time.Now()
	startTime := now.Add(2 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	workEventID := "test-work-id-find-" + now.Format("20060102T150405")
	testEvent := &calendar.Event{
		Id:          "test-event-find-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Find By Work ID",
		Description: "This is a test event for finding by workEventId",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": workEventID,
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}
	t.Logf("Inserted event with workEventId: %s", workEventID)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Find events by workEventId
	foundEvents, err := client.FindEventsByWorkID(calendarID, workEventID)
	if err != nil {
		t.Fatalf("Failed to find events by work ID: %v", err)
	}

	if len(foundEvents) == 0 {
		t.Fatalf("Failed to find event by workEventId %q", workEventID)
	}

	if len(foundEvents) > 1 {
		t.Logf("Warning: Found %d events with workEventId %q (expected 1)", len(foundEvents), workEventID)
	}

	foundEvent := foundEvents[0]
	if foundEvent.Summary != testEvent.Summary {
		t.Errorf("Event summary mismatch: expected %q, got %q", testEvent.Summary, foundEvent.Summary)
	}

	if foundEvent.ExtendedProperties == nil ||
		foundEvent.ExtendedProperties.Private == nil ||
		foundEvent.ExtendedProperties.Private["workEventId"] != workEventID {
		t.Errorf("workEventId mismatch: expected %q, got %v", workEventID, foundEvent.ExtendedProperties)
	}

	t.Logf("Successfully found event by workEventId: %s", workEventID)

	// Cleanup: delete all found test events
	t.Cleanup(func() {
		for _, event := range foundEvents {
			if err := client.DeleteEvent(calendarID, event.Id); err != nil {
				t.Logf("Warning: failed to cleanup test event %s: %v", event.Id, err)
			}
		}
	})
}

// TestAppleCalendar_Delete tests deleting an event
func TestAppleCalendar_Delete(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event
	now := time.Now()
	startTime := now.Add(3 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	testEvent := &calendar.Event{
		Id:          "test-event-delete-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Delete",
		Description: "This is a test event for deletion",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "test-work-id-delete-" + now.Format("20060102T150405"),
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}
	t.Logf("Inserted event with ID: %s", testEvent.Id)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Retrieve the event to get its actual ID (filename)
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	// Find our test event
	var eventToDelete *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == testEvent.ExtendedProperties.Private["workEventId"] {
			eventToDelete = event
			break
		}
	}

	if eventToDelete == nil {
		t.Fatalf("Failed to find event to delete. Found %d events in range", len(events))
	}

	t.Logf("Found event to delete with ID: %s", eventToDelete.Id)

	// Delete the event
	err = client.DeleteEvent(calendarID, eventToDelete.Id)
	if err != nil {
		t.Fatalf("Failed to delete event: %v", err)
	}
	t.Logf("Successfully deleted event with ID: %s", eventToDelete.Id)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Verify the event is deleted
	eventsAfter, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events after deletion: %v", err)
	}

	// Note: No cleanup needed - this test already deletes the event
	// Check if the event still exists
	for _, event := range eventsAfter {
		if event.Id == eventToDelete.Id {
			t.Errorf("Event still exists after deletion! ID: %s", event.Id)
		}
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == testEvent.ExtendedProperties.Private["workEventId"] {
			t.Errorf("Event with workEventId still exists after deletion!")
		}
	}

	t.Logf("Successfully verified event deletion. Events before: %d, after: %d", len(events), len(eventsAfter))
}

// TestAppleCalendar_Update tests updating an event
func TestAppleCalendar_Update(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event
	now := time.Now()
	startTime := now.Add(4 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	workEventID := "test-work-id-update-" + now.Format("20060102T150405")
	testEvent := &calendar.Event{
		Id:          "test-event-update-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Update Original",
		Description: "This is a test event for updating",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": workEventID,
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}
	t.Logf("Inserted event with ID: %s", testEvent.Id)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Retrieve the event to get its actual ID (filename)
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	// Find our test event
	var eventToUpdate *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == workEventID {
			eventToUpdate = event
			break
		}
	}

	if eventToUpdate == nil {
		t.Fatalf("Failed to find event to update. Found %d events in range", len(events))
	}

	t.Logf("Found event to update with ID: %s, Summary: %s", eventToUpdate.Id, eventToUpdate.Summary)

	// Create updated event
	updatedEvent := &calendar.Event{
		Id:          testEvent.Id, // Keep the same ID
		Summary:     "Test Event - Update Modified",
		Description: "This is an updated test event",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Add(30 * time.Minute).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Add(30 * time.Minute).Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": workEventID,
			},
		},
	}

	// Update the event using the event ID from GetEvents (the filename)
	err = client.UpdateEvent(calendarID, eventToUpdate.Id, updatedEvent)
	if err != nil {
		t.Fatalf("Failed to update event: %v", err)
	}
	t.Logf("Successfully updated event with ID: %s", eventToUpdate.Id)

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Verify the event was updated
	eventsAfter, err := client.GetEvents(calendarID, timeMin, timeMax.Add(1*time.Hour))
	if err != nil {
		t.Fatalf("Failed to get events after update: %v", err)
	}

	// Find the updated event
	var updatedEventFound *calendar.Event
	for _, event := range eventsAfter {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == workEventID {
			updatedEventFound = event
			break
		}
	}

	if updatedEventFound == nil {
		t.Fatalf("Failed to find updated event")
	}

	if updatedEventFound.Summary != updatedEvent.Summary {
		t.Errorf("Event summary not updated: expected %q, got %q", updatedEvent.Summary, updatedEventFound.Summary)
	}

	// Verify we still have only one event with this workEventId
	foundEvents, err := client.FindEventsByWorkID(calendarID, workEventID)
	if err != nil {
		t.Fatalf("Failed to find events by work ID after update: %v", err)
	}

	if len(foundEvents) != 1 {
		t.Errorf("Expected 1 event with workEventId %q after update, found %d", workEventID, len(foundEvents))
	}

	t.Logf("Successfully verified event update. Summary changed from %q to %q", testEvent.Summary, updatedEventFound.Summary)

	// Cleanup: delete the test event
	t.Cleanup(func() {
		if err := client.DeleteEvent(calendarID, eventToUpdate.Id); err != nil {
			t.Logf("Warning: failed to cleanup test event %s: %v", eventToUpdate.Id, err)
		}
	})
}

// TestAppleCalendar_InsertDeleteInsert tests the full cycle: insert, delete, insert again
// This ensures deletion works correctly and doesn't prevent re-insertion
func TestAppleCalendar_InsertDeleteInsert(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event
	now := time.Now()
	startTime := now.Add(5 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	workEventID := "test-work-id-cycle-" + now.Format("20060102T150405")
	testEvent := &calendar.Event{
		Id:          "test-event-cycle-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Insert Delete Insert Cycle",
		Description: "This event will be inserted, deleted, and re-inserted",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": workEventID,
			},
		},
	}

	// First insert
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event (first time): %v", err)
	}
	t.Logf("First insert successful")

	// Wait and verify it exists
	time.Sleep(2 * time.Second)
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events after first insert: %v", err)
	}

	var firstEvent *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == workEventID {
			firstEvent = event
			break
		}
	}

	if firstEvent == nil {
		t.Fatalf("Failed to find event after first insert")
	}
	t.Logf("Found event after first insert with ID: %s", firstEvent.Id)

	// Delete the event
	err = client.DeleteEvent(calendarID, firstEvent.Id)
	if err != nil {
		t.Fatalf("Failed to delete event: %v", err)
	}
	t.Logf("Delete successful")

	// Wait and verify it's deleted
	time.Sleep(2 * time.Second)
	eventsAfterDelete, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events after delete: %v", err)
	}

	for _, event := range eventsAfterDelete {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == workEventID {
			t.Errorf("Event still exists after deletion!")
		}
	}
	t.Logf("Verified event is deleted")

	// Re-insert the same event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event (second time): %v", err)
	}
	t.Logf("Second insert successful")

	// Wait and verify it exists again
	time.Sleep(2 * time.Second)
	eventsAfterReinsert, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events after re-insert: %v", err)
	}

	var secondEvent *calendar.Event
	for _, event := range eventsAfterReinsert {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == workEventID {
			secondEvent = event
			break
		}
	}

	if secondEvent == nil {
		t.Fatalf("Failed to find event after re-insert")
	}
	t.Logf("Found event after re-insert with ID: %s", secondEvent.Id)

	// Verify we can find it by workEventId
	foundEvents, err := client.FindEventsByWorkID(calendarID, workEventID)
	if err != nil {
		t.Fatalf("Failed to find events by work ID after re-insert: %v", err)
	}

	if len(foundEvents) != 1 {
		t.Errorf("Expected 1 event with workEventId %q after re-insert, found %d", workEventID, len(foundEvents))
	}

	// Cleanup: delete the re-inserted event
	t.Cleanup(func() {
		if secondEvent != nil {
			if err := client.DeleteEvent(calendarID, secondEvent.Id); err != nil {
				t.Logf("Warning: failed to cleanup test event %s: %v", secondEvent.Id, err)
			}
		}
	})

	t.Logf("Successfully completed insert-delete-insert cycle")
}

// TestAppleCalendar_AllDayEvent tests inserting and retrieving an all-day event
func TestAppleCalendar_AllDayEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create an all-day event (tomorrow)
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1)
	dateStr := tomorrow.Format("2006-01-02")
	nextDayStr := tomorrow.AddDate(0, 0, 1).Format("2006-01-02")

	testEvent := &calendar.Event{
		Id:          "test-event-allday-" + now.Format("20060102T150405"),
		Summary:     "Test All-Day Event",
		Description: "This is an all-day test event",
		Start: &calendar.EventDateTime{
			Date: dateStr,
		},
		End: &calendar.EventDateTime{
			Date: nextDayStr, // All-day events end on the next day (exclusive)
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "test-work-id-allday-" + now.Format("20060102T150405"),
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert all-day event: %v", err)
	}
	t.Logf("Successfully inserted all-day event")

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Retrieve events in the time range
	timeMin := tomorrow.Add(-1 * time.Hour)
	timeMax := tomorrow.Add(25 * time.Hour) // Next day + 1 hour
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	// Find our test event
	var foundEvent *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == testEvent.ExtendedProperties.Private["workEventId"] {
			foundEvent = event
			break
		}
	}

	if foundEvent == nil {
		t.Fatalf("Failed to find all-day event. Found %d events in range", len(events))
	}

	// Verify it's an all-day event
	if foundEvent.Start == nil || foundEvent.Start.Date == "" {
		t.Errorf("Expected all-day event (Date field), but got DateTime: %v", foundEvent.Start)
	}

	if foundEvent.Start.DateTime != "" {
		t.Errorf("All-day event should not have DateTime field, got: %s", foundEvent.Start.DateTime)
	}

	if foundEvent.Start.Date != dateStr {
		t.Errorf("All-day event date mismatch: expected %q, got %q", dateStr, foundEvent.Start.Date)
	}

	t.Logf("Successfully verified all-day event with date: %s", foundEvent.Start.Date)

	// Cleanup: delete the test event
	t.Cleanup(func() {
		if err := client.DeleteEvent(calendarID, foundEvent.Id); err != nil {
			t.Logf("Warning: failed to cleanup test event %s: %v", foundEvent.Id, err)
		}
	})
}

// TestAppleCalendar_GetEvent tests retrieving a single event by ID
func TestAppleCalendar_GetEvent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event
	now := time.Now()
	startTime := now.Add(6 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	testEvent := &calendar.Event{
		Id:          "test-event-get-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Get Single",
		Description: "This is a test event for GetEvent",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "test-work-id-get-" + now.Format("20060102T150405"),
			},
		},
	}

	// Insert the event
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event: %v", err)
	}

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Get the event ID (filename) from GetEvents first
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	var eventID string
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == testEvent.ExtendedProperties.Private["workEventId"] {
			eventID = event.Id
			break
		}
	}

	if eventID == "" {
		t.Fatalf("Failed to find event ID for GetEvent test")
	}

	// Now retrieve the single event using GetEvent
	retrievedEvent, err := client.GetEvent(calendarID, eventID)
	if err != nil {
		t.Fatalf("Failed to get single event: %v", err)
	}

	if retrievedEvent.Summary != testEvent.Summary {
		t.Errorf("Event summary mismatch: expected %q, got %q", testEvent.Summary, retrievedEvent.Summary)
	}

	if retrievedEvent.Description != testEvent.Description {
		t.Errorf("Event description mismatch: expected %q, got %q", testEvent.Description, retrievedEvent.Description)
	}

	if retrievedEvent.ExtendedProperties == nil ||
		retrievedEvent.ExtendedProperties.Private == nil ||
		retrievedEvent.ExtendedProperties.Private["workEventId"] != testEvent.ExtendedProperties.Private["workEventId"] {
		t.Errorf("workEventId mismatch in retrieved event")
	}

	t.Logf("Successfully retrieved single event using GetEvent with ID: %s", eventID)

	// Cleanup: delete the test event
	t.Cleanup(func() {
		if err := client.DeleteEvent(calendarID, eventID); err != nil {
			t.Logf("Warning: failed to cleanup test event %s: %v", eventID, err)
		}
	})
}

// TestAppleCalendar_SpecialCharacters tests that event IDs with special characters are handled correctly
func TestAppleCalendar_SpecialCharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	config := loadTestConfig(t)
	dest := getAppleDestination(t, config)

	ctx := context.Background()
	client, err := NewAppleCalendarClient(
		ctx,
		dest.ServerURL,
		dest.Username,
		dest.Password,
	)
	if err != nil {
		t.Fatalf("Failed to create Apple Calendar client: %v", err)
	}

	// Find or create test calendar (use the sync calendar from config)
	calendarID, err := client.FindOrCreateCalendarByName(dest.CalendarName, dest.CalendarColorID)
	if err != nil {
		t.Fatalf("Failed to find or create calendar: %v", err)
	}

	// Create a test event with special characters in the ID
	now := time.Now()
	startTime := now.Add(7 * time.Hour)
	endTime := startTime.Add(1 * time.Hour)

	// Event ID with special characters that need sanitization
	testEvent := &calendar.Event{
		Id:          "test/event:with\\special-chars-" + now.Format("20060102T150405"),
		Summary:     "Test Event - Special Characters",
		Description: "This event has special characters in its ID",
		Start: &calendar.EventDateTime{
			DateTime: startTime.Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: endTime.Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "test-work-id-special-" + now.Format("20060102T150405"),
			},
		},
	}

	// Insert the event (should sanitize the ID)
	err = client.InsertEvent(calendarID, testEvent)
	if err != nil {
		t.Fatalf("Failed to insert event with special characters: %v", err)
	}
	t.Logf("Successfully inserted event with special characters in ID")

	// Wait a bit for the server to process
	time.Sleep(2 * time.Second)

	// Retrieve the event
	timeMin := startTime.Add(-1 * time.Hour)
	timeMax := endTime.Add(1 * time.Hour)
	events, err := client.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		t.Fatalf("Failed to get events: %v", err)
	}

	// Find our test event
	var foundEvent *calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil &&
			event.ExtendedProperties.Private != nil &&
			event.ExtendedProperties.Private["workEventId"] == testEvent.ExtendedProperties.Private["workEventId"] {
			foundEvent = event
			break
		}
	}

	if foundEvent == nil {
		t.Fatalf("Failed to find event with special characters. Found %d events in range", len(events))
	}

	// Verify the event ID was sanitized (should have .ics extension and no special chars)
	if !strings.HasSuffix(foundEvent.Id, ".ics") {
		t.Errorf("Event ID should end with .ics, got: %s", foundEvent.Id)
	}

	// Verify special characters were replaced
	if strings.Contains(foundEvent.Id, "/") || strings.Contains(foundEvent.Id, "\\") || strings.Contains(foundEvent.Id, ":") {
		t.Errorf("Event ID should not contain special characters, got: %s", foundEvent.Id)
	}

	t.Logf("Successfully verified event with special characters. Sanitized ID: %s", foundEvent.Id)

	// Cleanup: delete the test event
	t.Cleanup(func() {
		if err := client.DeleteEvent(calendarID, foundEvent.Id); err != nil {
			t.Logf("Warning: failed to cleanup test event %s: %v", foundEvent.Id, err)
		}
	})
}
