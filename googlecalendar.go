package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
)

// Client is a wrapper around the Google Calendar API service.
type Client struct {
	service *calendar.Service
}

// NewClient creates a new Google Calendar API client using the provided HTTP client.
func NewClient(ctx context.Context, httpClient *http.Client) (*Client, error) {
	service, err := calendar.NewService(ctx, option.WithHTTPClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("failed to create calendar service: %w", err)
	}

	return &Client{service: service}, nil
}

// FindOrCreateCalendarByName finds an existing calendar by name or creates a new one.
// Returns the calendar ID.
func (c *Client) FindOrCreateCalendarByName(name string, colorID string) (string, error) {
	// List the user's calendars
	calendarList, err := c.service.CalendarList.List().Do()
	if err != nil {
		return "", fmt.Errorf("Google: failed to list calendars: %w", err)
	}

	// Check if a calendar with the given name exists
	for _, cal := range calendarList.Items {
		if cal.Summary == name {
			return cal.Id, nil
		}
	}

	// Calendar doesn't exist, create it
	newCalendar := &calendar.Calendar{
		Summary:     name,
		Description: "Synced calendar from work account",
	}

	created, err := c.service.Calendars.Insert(newCalendar).Do()
	if err != nil {
		return "", fmt.Errorf("failed to create calendar: %w", err)
	}

	// Set the color if provided
	if colorID != "" {
		_, err = c.service.CalendarList.Patch(created.Id, &calendar.CalendarListEntry{
			ColorId: colorID,
		}).Do()
		if err != nil {
			// Log but don't fail if color setting fails
			fmt.Printf("Warning: failed to set calendar color: %v\n", err)
		}
	}

	return created.Id, nil
}

// GetEvent retrieves a single event by ID.
func (c *Client) GetEvent(calendarID, eventID string) (*calendar.Event, error) {
	event, err := c.service.Events.Get(calendarID, eventID).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}
	return event, nil
}

// GetEvents retrieves events from a calendar within the specified time window.
// Important: Sets SingleEvents = true to expand recurring events.
func (c *Client) GetEvents(calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error) {
	eventsList, err := c.service.Events.List(calendarID).
		TimeMin(timeMin.Format(time.RFC3339)).
		TimeMax(timeMax.Format(time.RFC3339)).
		SingleEvents(true). // Expand recurring events
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to list events: %w", err)
	}

	return eventsList.Items, nil
}

// FindEventsByWorkID finds events in a calendar that have a specific workEventId
// in their private extended properties.
func (c *Client) FindEventsByWorkID(calendarID, workEventID string) ([]*calendar.Event, error) {
	// Use privateExtendedProperty to search for events with the workEventId
	query := fmt.Sprintf("workEventId=%s", workEventID)

	eventsList, err := c.service.Events.List(calendarID).
		PrivateExtendedProperty(query).
		SingleEvents(true).
		Do()
	if err != nil {
		return nil, fmt.Errorf("failed to find events by work ID: %w", err)
	}

	return eventsList.Items, nil
}

// InsertEvent inserts a new event into a calendar.
// Important: Sets sendUpdates="none" to prevent notifications.
func (c *Client) InsertEvent(calendarID string, event *calendar.Event) error {
	_, err := c.service.Events.Insert(calendarID, event).
		SendUpdates("none"). // Disable notifications
		Do()
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}

	return nil
}

// UpdateEvent updates an existing event in a calendar.
func (c *Client) UpdateEvent(calendarID, eventID string, event *calendar.Event) error {
	_, err := c.service.Events.Update(calendarID, eventID, event).
		SendUpdates("none"). // Disable notifications
		Do()
	if err != nil {
		return fmt.Errorf("failed to update event: %w", err)
	}

	return nil
}

// DeleteEvent deletes an event from a calendar.
func (c *Client) DeleteEvent(calendarID, eventID string) error {
	err := c.service.Events.Delete(calendarID, eventID).
		SendUpdates("none"). // Disable notifications
		Do()
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	return nil
}
