package main

import (
	"time"

	"google.golang.org/api/calendar/v3"
)

// CalendarClient is a generic interface for calendar operations.
// Both Google Calendar and Apple Calendar clients implement this interface.
type CalendarClient interface {
	FindOrCreateCalendarByName(name string, colorID string) (string, error)
	GetEvents(calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error)
	GetEvent(calendarID, eventID string) (*calendar.Event, error)
	InsertEvent(calendarID string, event *calendar.Event) error
	UpdateEvent(calendarID, eventID string, event *calendar.Event) error
	DeleteEvent(calendarID, eventID string) error
	FindEventsByWorkID(calendarID, workEventID string) ([]*calendar.Event, error)
}
