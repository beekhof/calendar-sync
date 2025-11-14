package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"google.golang.org/api/calendar/v3"
)

// AppleCalendarClient is a client for Apple Calendar/iCloud using CalDAV.
type AppleCalendarClient struct {
	httpClient *http.Client
	username   string
	password   string
	serverURL  string
	basePath   string
}

// NewAppleCalendarClient creates a new Apple Calendar client using CalDAV.
// serverURL should be the CalDAV server URL (e.g., "https://caldav.icloud.com" for iCloud)
// username and password are the iCloud credentials (password should be an app-specific password)
func NewAppleCalendarClient(ctx context.Context, serverURL, username, password string) (*AppleCalendarClient, error) {
	// Create HTTP client with basic auth
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// For iCloud, the base path is typically /{userID}/calendars/
	// We'll discover this during calendar listing
	basePath := fmt.Sprintf("/%s/calendars/", username)

	return &AppleCalendarClient{
		httpClient: httpClient,
		username:   username,
		password:   password,
		serverURL:  serverURL,
		basePath:   basePath,
	}, nil
}

// makeRequest makes an authenticated HTTP request to the CalDAV server.
func (c *AppleCalendarClient) makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	url := strings.TrimSuffix(c.serverURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.username, c.password)
	if body != nil {
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	}
	req.Header.Set("Depth", "1")

	return c.httpClient.Do(req)
}

// FindOrCreateCalendarByName finds an existing calendar by name or creates a new one.
// Returns the calendar path.
func (c *AppleCalendarClient) FindOrCreateCalendarByName(name string, colorID string) (string, error) {
	// List calendars using PROPFIND
	propfindBody := `<?xml version="1.0" encoding="utf-8" ?>
<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">
  <d:prop>
    <d:displayname/>
    <c:calendar-description/>
  </d:prop>
</d:propfind>`

	resp, err := c.makeRequest("PROPFIND", c.basePath, strings.NewReader(propfindBody))
	if err != nil {
		return "", fmt.Errorf("failed to list calendars: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to list calendars: HTTP %d", resp.StatusCode)
	}

	// Parse XML response to find calendar by name
	// For now, return a simple path - full implementation would parse the XML
	calendarPath := c.basePath + strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "/"

	// Verify calendar exists by trying to access it
	verifyResp, err := c.makeRequest("PROPFIND", calendarPath, strings.NewReader(propfindBody))
	if err == nil {
		verifyResp.Body.Close()
		if verifyResp.StatusCode == http.StatusOK || verifyResp.StatusCode == http.StatusMultiStatus {
			return calendarPath, nil
		}
	}

	// Calendar doesn't exist - for iCloud, calendars must be created manually
	return "", fmt.Errorf("calendar '%s' not found. Please create it manually in Apple Calendar/iCloud", name)
}

// GetEvents retrieves events from a calendar within the specified time window.
func (c *AppleCalendarClient) GetEvents(calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error) {
	// Build CalDAV REPORT query
	queryBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8" ?>
<C:calendar-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:getetag/>
    <C:calendar-data/>
  </D:prop>
  <C:filter>
    <C:comp-filter name="VCALENDAR">
      <C:comp-filter name="VEVENT">
        <C:time-range start="%s" end="%s"/>
      </C:comp-filter>
    </C:comp-filter>
  </C:filter>
</C:calendar-query>`, timeMin.Format("20060102T150405Z"), timeMax.Format("20060102T150405Z"))

	resp, err := c.makeRequest("REPORT", calendarID, strings.NewReader(queryBody))
	if err != nil {
		return nil, fmt.Errorf("failed to query calendar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMultiStatus {
		return nil, fmt.Errorf("failed to query calendar: HTTP %d", resp.StatusCode)
	}

	// Parse the response to extract iCalendar data
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse XML to extract calendar-data elements
	events, err := parseCalDAVResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CalDAV response: %w", err)
	}

	// Convert iCalendar events to Google Calendar Event format
	var googleEvents []*calendar.Event
	for _, icalData := range events {
		icalCal, err := ical.NewDecoder(strings.NewReader(icalData)).Decode()
		if err != nil {
			fmt.Printf("Warning: failed to parse iCalendar data: %v\n", err)
			continue
		}

		googleEvent, err := icalToGoogleEvent(icalCal)
		if err != nil {
			fmt.Printf("Warning: failed to convert event: %v\n", err)
			continue
		}
		googleEvents = append(googleEvents, googleEvent)
	}

	return googleEvents, nil
}

// GetEvent retrieves a single event by ID.
func (c *AppleCalendarClient) GetEvent(calendarID, eventID string) (*calendar.Event, error) {
	// Fetch the event using GET
	resp, err := c.makeRequest("GET", calendarID+eventID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get event: HTTP %d", resp.StatusCode)
	}

	// Parse iCalendar data
	icalCal, err := ical.NewDecoder(resp.Body).Decode()
	if err != nil {
		return nil, fmt.Errorf("failed to parse iCalendar: %w", err)
	}

	return icalToGoogleEvent(icalCal)
}

// InsertEvent inserts a new event into a calendar.
func (c *AppleCalendarClient) InsertEvent(calendarID string, event *calendar.Event) error {
	// Convert Google Calendar Event to iCalendar format
	icalCal, err := googleEventToICal(event)
	if err != nil {
		return fmt.Errorf("failed to convert event: %w", err)
	}

	// Serialize to iCalendar format
	var buf bytes.Buffer
	enc := ical.NewEncoder(&buf)
	if err := enc.Encode(icalCal); err != nil {
		return fmt.Errorf("failed to encode iCalendar: %w", err)
	}

	// Generate a unique event ID
	eventID := fmt.Sprintf("%s.ics", event.Id)

	// Put the event using PUT
	resp, err := c.makeRequest("PUT", calendarID+eventID, &buf)
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to insert event: HTTP %d", resp.StatusCode)
	}

	return nil
}

// UpdateEvent updates an existing event in a calendar.
func (c *AppleCalendarClient) UpdateEvent(calendarID, eventID string, event *calendar.Event) error {
	// Same as InsertEvent for CalDAV
	return c.InsertEvent(calendarID, event)
}

// DeleteEvent deletes an event from a calendar.
func (c *AppleCalendarClient) DeleteEvent(calendarID, eventID string) error {
	resp, err := c.makeRequest("DELETE", calendarID+eventID, nil)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete event: HTTP %d", resp.StatusCode)
	}

	return nil
}

// FindEventsByWorkID finds events in a calendar that have a specific workEventId
// in their private extended properties.
func (c *AppleCalendarClient) FindEventsByWorkID(calendarID, workEventID string) ([]*calendar.Event, error) {
	// Get all events in a wide time range
	now := time.Now()
	timeMin := now.AddDate(-1, 0, 0) // 1 year ago
	timeMax := now.AddDate(1, 0, 0)  // 1 year from now

	events, err := c.GetEvents(calendarID, timeMin, timeMax)
	if err != nil {
		return nil, err
	}

	// Filter events by workEventId
	var results []*calendar.Event
	for _, event := range events {
		if event.ExtendedProperties != nil && event.ExtendedProperties.Private != nil {
			if event.ExtendedProperties.Private["workEventId"] == workEventID {
				results = append(results, event)
			}
		}
	}

	return results, nil
}

// parseCalDAVResponse parses a CalDAV REPORT response to extract iCalendar data.
func parseCalDAVResponse(body []byte) ([]string, error) {
	type CalendarData struct {
		XMLName xml.Name `xml:"calendar-data"`
		Data    string   `xml:",chardata"`
	}

	type Prop struct {
		CalendarData CalendarData `xml:"calendar-data"`
	}

	type Response struct {
		XMLName xml.Name `xml:"response"`
		Prop    Prop     `xml:"propstat>prop"`
	}

	type Multistatus struct {
		XMLName   xml.Name   `xml:"multistatus"`
		Responses []Response `xml:"response"`
	}

	var multistatus Multistatus
	if err := xml.Unmarshal(body, &multistatus); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	var events []string
	for _, resp := range multistatus.Responses {
		if resp.Prop.CalendarData.Data != "" {
			events = append(events, resp.Prop.CalendarData.Data)
		}
	}

	return events, nil
}

// icalToGoogleEvent converts an iCalendar event to Google Calendar Event format.
func icalToGoogleEvent(icalCal *ical.Calendar) (*calendar.Event, error) {
	// Find the VEVENT component
	var vevent *ical.Component
	for _, comp := range icalCal.Children {
		if comp.Name == "VEVENT" {
			vevent = comp
			break
		}
	}

	if vevent == nil {
		return nil, fmt.Errorf("no VEVENT found in calendar")
	}

	event := &calendar.Event{}

	// Extract UID (event ID)
	if uid := vevent.Props.Get(ical.PropUID); uid != nil {
		event.Id = uid.Value
	}

	// Extract summary
	if summary := vevent.Props.Get(ical.PropSummary); summary != nil {
		event.Summary = summary.Value
	}

	// Extract description
	if desc := vevent.Props.Get(ical.PropDescription); desc != nil {
		event.Description = desc.Value
	}

	// Extract location
	if loc := vevent.Props.Get(ical.PropLocation); loc != nil {
		event.Location = loc.Value
	}

	// Extract start time
	if dtstart := vevent.Props.Get(ical.PropDateTimeStart); dtstart != nil {
		startTime, err := parseICalDateTime(dtstart)
		if err == nil {
			// Check if it's a DATE value type (all-day)
			// Check the VALUE parameter
			valueParam := dtstart.Params.Get("VALUE")
			if valueParam != "" && valueParam == "DATE" {
				// All-day event
				event.Start = &calendar.EventDateTime{
					Date: startTime.Format("2006-01-02"),
				}
				event.End = &calendar.EventDateTime{
					Date: startTime.AddDate(0, 0, 1).Format("2006-01-02"),
				}
			} else {
				// Timed event
				event.Start = &calendar.EventDateTime{
					DateTime: startTime.Format(time.RFC3339),
				}
			}
		}
	}

	// Extract end time
	if dtend := vevent.Props.Get(ical.PropDateTimeEnd); dtend != nil {
		endTime, err := parseICalDateTime(dtend)
		if err == nil {
			valueParam := dtend.Params.Get("VALUE")
			if valueParam != "" && valueParam == "DATE" {
				// All-day event end
				if event.End == nil {
					event.End = &calendar.EventDateTime{
						Date: endTime.Format("2006-01-02"),
					}
				}
			} else {
				// Timed event end
				if event.End == nil {
					event.End = &calendar.EventDateTime{
						DateTime: endTime.Format(time.RFC3339),
					}
				}
			}
		}
	}

	// Extract transparency (for OOF detection)
	if transp := vevent.Props.Get("TRANSP"); transp != nil {
		if text, err := transp.Text(); err == nil && text == "TRANSPARENT" {
			event.Transparency = "transparent"
		}
	}

	// Extract extended properties (for workEventId tracking)
	// Store in X- properties
	if xWorkID := vevent.Props.Get("X-WORK-EVENT-ID"); xWorkID != nil {
		if event.ExtendedProperties == nil {
			event.ExtendedProperties = &calendar.EventExtendedProperties{
				Private: make(map[string]string),
			}
		}
		event.ExtendedProperties.Private["workEventId"] = xWorkID.Value
	}

	return event, nil
}

// googleEventToICal converts a Google Calendar Event to iCalendar format.
func googleEventToICal(event *calendar.Event) (*ical.Calendar, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropVersion, "2.0")
	cal.Props.SetText(ical.PropProductID, "-//Calendar Sync//EN")

	vevent := ical.NewComponent(ical.CompEvent)
	cal.Children = append(cal.Children, vevent)

	// Set UID
	if event.Id != "" {
		vevent.Props.SetText(ical.PropUID, event.Id)
	} else {
		// Generate a UID if not present
		vevent.Props.SetText(ical.PropUID, fmt.Sprintf("%s@calendar-sync", time.Now().Format(time.RFC3339Nano)))
	}

	// Set summary
	if event.Summary != "" {
		vevent.Props.SetText(ical.PropSummary, event.Summary)
	}

	// Set description
	if event.Description != "" {
		vevent.Props.SetText(ical.PropDescription, event.Description)
	}

	// Set location
	if event.Location != "" {
		vevent.Props.SetText(ical.PropLocation, event.Location)
	}

	// Set start time
	if event.Start != nil {
		if event.Start.Date != "" {
			// All-day event
			startDate, err := time.Parse("2006-01-02", event.Start.Date)
			if err == nil {
				dtstart := ical.NewProp("DTSTART")
				dtstart.SetDate(startDate)
				vevent.Props.Set(dtstart)
			}
		} else if event.Start.DateTime != "" {
			// Timed event
			startTime, err := time.Parse(time.RFC3339, event.Start.DateTime)
			if err == nil {
				vevent.Props.SetDateTime("DTSTART", startTime)
			}
		}
	}

	// Set end time
	if event.End != nil {
		if event.End.Date != "" {
			// All-day event
			endDate, err := time.Parse("2006-01-02", event.End.Date)
			if err == nil {
				dtend := ical.NewProp("DTEND")
				dtend.SetDate(endDate)
				vevent.Props.Set(dtend)
			}
		} else if event.End.DateTime != "" {
			// Timed event
			endTime, err := time.Parse(time.RFC3339, event.End.DateTime)
			if err == nil {
				vevent.Props.SetDateTime("DTEND", endTime)
			}
		}
	}

	// Set transparency
	if event.Transparency == "transparent" {
		vevent.Props.SetText("TRANSP", "TRANSPARENT")
	}

	// Store workEventId in extended properties
	if event.ExtendedProperties != nil && event.ExtendedProperties.Private != nil {
		if workID := event.ExtendedProperties.Private["workEventId"]; workID != "" {
			vevent.Props.SetText("X-WORK-EVENT-ID", workID)
		}
	}

	// Set created and last modified timestamps
	now := time.Now()
	vevent.Props.SetDateTime(ical.PropDateTimeStamp, now)
	vevent.Props.SetDateTime(ical.PropCreated, now)
	vevent.Props.SetDateTime(ical.PropLastModified, now)

	return cal, nil
}

// parseICalDateTime parses an iCalendar date-time property.
func parseICalDateTime(prop *ical.Prop) (time.Time, error) {
	// Use the library's DateTime method which handles parsing
	// Pass nil for location to use UTC
	return prop.DateTime(nil)
}
