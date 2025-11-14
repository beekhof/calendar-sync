package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/api/calendar/v3"
)

// mockGoogleCalendarClient is a mock implementation of CalendarClient for testing.
type mockGoogleCalendarClient struct {
	calendars        map[string]string // name -> id
	events          map[string][]*calendar.Event // calendarID -> events
	insertedEvents  []*calendar.Event
	updatedEvents   []*calendar.Event
	deletedEventIDs []string
}

func newMockGoogleCalendarClient() *mockGoogleCalendarClient {
	return &mockGoogleCalendarClient{
		calendars:       make(map[string]string),
		events:         make(map[string][]*calendar.Event),
		insertedEvents: []*calendar.Event{},
		updatedEvents:  []*calendar.Event{},
		deletedEventIDs: []string{},
	}
}

func (m *mockGoogleCalendarClient) FindOrCreateCalendarByName(name string, colorID string) (string, error) {
	if id, exists := m.calendars[name]; exists {
		return id, nil
	}
	// Create new calendar
	newID := "cal_" + name
	m.calendars[name] = newID
	m.events[newID] = []*calendar.Event{}
	return newID, nil
}

func (m *mockGoogleCalendarClient) GetEvents(calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error) {
	return m.events[calendarID], nil
}

func (m *mockGoogleCalendarClient) GetEvent(calendarID, eventID string) (*calendar.Event, error) {
	// Search through all events in the calendar to find the event
	if events, exists := m.events[calendarID]; exists {
		for _, event := range events {
			if event.Id == eventID {
				return event, nil
			}
		}
	}
	// Also check if it's a recurring event parent by searching all calendars
	for _, events := range m.events {
		for _, event := range events {
			if event.Id == eventID {
				return event, nil
			}
		}
	}
	return nil, fmt.Errorf("event not found: %s", eventID)
}

func (m *mockGoogleCalendarClient) InsertEvent(calendarID string, event *calendar.Event) error {
	m.insertedEvents = append(m.insertedEvents, event)
	if m.events[calendarID] == nil {
		m.events[calendarID] = []*calendar.Event{}
	}
	m.events[calendarID] = append(m.events[calendarID], event)
	return nil
}

func (m *mockGoogleCalendarClient) UpdateEvent(calendarID, eventID string, event *calendar.Event) error {
	m.updatedEvents = append(m.updatedEvents, event)
	// Update in mock storage
	if events, exists := m.events[calendarID]; exists {
		for i, e := range events {
			if e.Id == eventID {
				events[i] = event
				break
			}
		}
	}
	return nil
}

func (m *mockGoogleCalendarClient) DeleteEvent(calendarID, eventID string) error {
	m.deletedEventIDs = append(m.deletedEventIDs, eventID)
	if events, exists := m.events[calendarID]; exists {
		for i, e := range events {
			if e.Id == eventID {
				events = append(events[:i], events[i+1:]...)
				m.events[calendarID] = events
				break
			}
		}
	}
	return nil
}

func (m *mockGoogleCalendarClient) FindEventsByWorkID(calendarID, workEventID string) ([]*calendar.Event, error) {
	var results []*calendar.Event
	if events, exists := m.events[calendarID]; exists {
		for _, e := range events {
			if e.ExtendedProperties != nil && e.ExtendedProperties.Private != nil {
				if e.ExtendedProperties.Private["workEventId"] == workEventID {
					results = append(results, e)
				}
			}
		}
	}
	return results, nil
}

func TestFilterEvents_TimedOOF(t *testing.T) {
	mockClient := newMockGoogleCalendarClient()
	syncer := &Syncer{
		workClient: mockClient,
	}
	
	// Create a timed OOF event using EventType (most reliable method)
	oofEvent := &calendar.Event{
		Id:        "oof-1",
		Summary:   "Out of Office",
		EventType: "outOfOffice",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}

	events := []*calendar.Event{oofEvent}
	filtered := syncer.filterEvents(events)

	if len(filtered) != 0 {
		t.Errorf("Expected timed OOF event to be filtered out, but got %d events", len(filtered))
	}
}

func TestFilterEvents_TimedOOF_TransparencyFallback(t *testing.T) {
	mockClient := newMockGoogleCalendarClient()
	syncer := &Syncer{
		workClient: mockClient,
	}
	
	// Create a timed OOF event using Transparency (fallback method)
	// This tests backward compatibility with older events
	oofEvent := &calendar.Event{
		Id:      "oof-1",
		Summary: "Out of Office",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 14, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 15, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		Transparency: "transparent",
	}

	events := []*calendar.Event{oofEvent}
	filtered := syncer.filterEvents(events)

	if len(filtered) != 0 {
		t.Errorf("Expected timed OOF event (via transparency) to be filtered out, but got %d events", len(filtered))
	}
}

func TestFilterEvents_AllDayOOF(t *testing.T) {
	mockClient := newMockGoogleCalendarClient()
	syncer := &Syncer{
		workClient: mockClient,
	}
	
	// Create an all-day OOF event
	allDayOOF := &calendar.Event{
		Id:      "oof-2",
		Summary: "Out of Office",
		Start: &calendar.EventDateTime{
			Date: "2024-01-15",
		},
		End: &calendar.EventDateTime{
			Date: "2024-01-16",
		},
		Transparency: "transparent",
	}

	events := []*calendar.Event{allDayOOF}
	filtered := syncer.filterEvents(events)

	if len(filtered) != 1 {
		t.Errorf("Expected all-day OOF event to be kept, but got %d events", len(filtered))
	}
}

func TestFilterEvents_OutsideWindow(t *testing.T) {
	mockClient := newMockGoogleCalendarClient()
	syncer := &Syncer{
		workClient: mockClient,
	}
	
	// Create an event at 5:00 AM - 5:30 AM (entirely outside window)
	earlyEvent := &calendar.Event{
		Id:      "early-1",
		Summary: "Early Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 5, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 5, 30, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}

	events := []*calendar.Event{earlyEvent}
	filtered := syncer.filterEvents(events)

	if len(filtered) != 0 {
		t.Errorf("Expected event entirely before 6 AM to be filtered out, but got %d events", len(filtered))
	}
}

func TestFilterEvents_PartialOverlap(t *testing.T) {
	mockClient := newMockGoogleCalendarClient()
	syncer := &Syncer{
		workClient: mockClient,
	}
	
	// Create an event at 5:30 AM - 6:30 AM (partially overlaps window)
	overlapEvent := &calendar.Event{
		Id:      "overlap-1",
		Summary: "Overlap Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 5, 30, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 6, 30, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}

	events := []*calendar.Event{overlapEvent}
	filtered := syncer.filterEvents(events)

	if len(filtered) != 1 {
		t.Errorf("Expected event partially overlapping window to be kept, but got %d events", len(filtered))
	}
}

func TestSync_NewEvent(t *testing.T) {
	workClient := newMockGoogleCalendarClient()
	personalClient := newMockGoogleCalendarClient()
	
	config := &Config{
		SyncCalendarName:    "Work Sync",
		SyncCalendarColorID: "7",
	}

	syncer := NewSyncer(workClient, personalClient, config)

	// Add a new event to work calendar
	workEvent := &calendar.Event{
		Id:      "work-1",
		Summary: "Work Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}
	workClient.events["primary"] = []*calendar.Event{workEvent}

	ctx := context.Background()
	err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync() returned an error: %v", err)
	}

	// Verify InsertEvent was called
	if len(personalClient.insertedEvents) != 1 {
		t.Errorf("Expected InsertEvent to be called once, but got %d calls", len(personalClient.insertedEvents))
	}

	// Verify the inserted event has the workEventId
	inserted := personalClient.insertedEvents[0]
	if inserted.ExtendedProperties == nil || inserted.ExtendedProperties.Private == nil {
		t.Error("Inserted event should have extended properties")
	} else if inserted.ExtendedProperties.Private["workEventId"] != "work-1" {
		t.Errorf("Expected workEventId to be 'work-1', got '%s'", inserted.ExtendedProperties.Private["workEventId"])
	}
}

func TestSync_DeletedEvent(t *testing.T) {
	workClient := newMockGoogleCalendarClient()
	personalClient := newMockGoogleCalendarClient()
	
	config := &Config{
		SyncCalendarName:    "Work Sync",
		SyncCalendarColorID: "7",
	}

	syncer := NewSyncer(workClient, personalClient, config)

	// Add an event to personal calendar that no longer exists in work
	destCalendarID := "cal_Work Sync"
	personalClient.calendars["Work Sync"] = destCalendarID
	staleEvent := &calendar.Event{
		Id:      "stale-1",
		Summary: "Old Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "work-deleted",
			},
		},
	}
	personalClient.events[destCalendarID] = []*calendar.Event{staleEvent}
	
	// Work calendar has no events
	workClient.events["primary"] = []*calendar.Event{}

	ctx := context.Background()
	err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync() returned an error: %v", err)
	}

	// Verify DeleteEvent was called
	if len(personalClient.deletedEventIDs) != 1 {
		t.Errorf("Expected DeleteEvent to be called once, but got %d calls", len(personalClient.deletedEventIDs))
	}

	if personalClient.deletedEventIDs[0] != "stale-1" {
		t.Errorf("Expected DeleteEvent to be called with 'stale-1', got '%s'", personalClient.deletedEventIDs[0])
	}
}

func TestSync_UnchangedEvent(t *testing.T) {
	workClient := newMockGoogleCalendarClient()
	personalClient := newMockGoogleCalendarClient()
	
	config := &Config{
		SyncCalendarName:    "Work Sync",
		SyncCalendarColorID: "7",
	}

	syncer := NewSyncer(workClient, personalClient, config)

	// Add the same event to both calendars
	workEvent := &calendar.Event{
		Id:      "work-1",
		Summary: "Work Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}
	workClient.events["primary"] = []*calendar.Event{workEvent}

	destCalendarID := "cal_Work Sync"
	personalClient.calendars["Work Sync"] = destCalendarID
	destEvent := &calendar.Event{
		Id:      "dest-1",
		Summary: "Work Meeting",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "work-1",
			},
		},
	}
	personalClient.events[destCalendarID] = []*calendar.Event{destEvent}

	ctx := context.Background()
	err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync() returned an error: %v", err)
	}

	// Verify no UpdateEvent was called
	if len(personalClient.updatedEvents) != 0 {
		t.Errorf("Expected no UpdateEvent calls for unchanged event, but got %d calls", len(personalClient.updatedEvents))
	}
}

func TestSync_ChangedEvent(t *testing.T) {
	workClient := newMockGoogleCalendarClient()
	personalClient := newMockGoogleCalendarClient()
	
	config := &Config{
		SyncCalendarName:    "Work Sync",
		SyncCalendarColorID: "7",
	}

	syncer := NewSyncer(workClient, personalClient, config)

	// Work event has been updated
	workEvent := &calendar.Event{
		Id:      "work-1",
		Summary: "Work Meeting Updated",
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339), // Changed time
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
	}
	workClient.events["primary"] = []*calendar.Event{workEvent}

	destCalendarID := "cal_Work Sync"
	personalClient.calendars["Work Sync"] = destCalendarID
	destEvent := &calendar.Event{
		Id:      "dest-1",
		Summary: "Work Meeting", // Old summary
		Start: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC).Format(time.RFC3339), // Old time
		},
		End: &calendar.EventDateTime{
			DateTime: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC).Format(time.RFC3339),
		},
		ExtendedProperties: &calendar.EventExtendedProperties{
			Private: map[string]string{
				"workEventId": "work-1",
			},
		},
	}
	personalClient.events[destCalendarID] = []*calendar.Event{destEvent}

	ctx := context.Background()
	err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Sync() returned an error: %v", err)
	}

	// Verify UpdateEvent was called
	if len(personalClient.updatedEvents) != 1 {
		t.Errorf("Expected UpdateEvent to be called once, but got %d calls", len(personalClient.updatedEvents))
	}

	updated := personalClient.updatedEvents[0]
	if updated.Summary != "Work Meeting Updated" {
		t.Errorf("Expected updated event summary to be 'Work Meeting Updated', got '%s'", updated.Summary)
	}
}

