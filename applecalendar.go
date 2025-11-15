package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
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
// Note: For iCloud, the username should be your full iCloud email address
func NewAppleCalendarClient(ctx context.Context, serverURL, username, password string) (*AppleCalendarClient, error) {
	// Create HTTP client with basic auth
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	client := &AppleCalendarClient{
		httpClient: httpClient,
		username:   username,
		password:   password,
		serverURL:  serverURL,
	}

	// Discover the principal and calendar home path
	basePath, err := client.discoverPrincipal()
	if err != nil {
		return nil, fmt.Errorf("failed to discover CalDAV principal: %w", err)
	}
	client.basePath = basePath

	return client, nil
}

// makeRequest makes an authenticated HTTP request to the CalDAV server.
func (c *AppleCalendarClient) makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	// Ensure path starts with / and doesn't contain the server URL
	path = strings.TrimPrefix(path, c.serverURL)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	url := strings.TrimSuffix(c.serverURL, "/") + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.username, c.password)
	// Set User-Agent header (required by some CalDAV servers)
	req.Header.Set("User-Agent", "calendar-sync/1.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	}
	if method == "PROPFIND" || method == "REPORT" {
		req.Header.Set("Depth", "1")
	}

	return c.httpClient.Do(req)
}

// discoverPrincipal discovers the CalDAV principal and calendar home path.
// This is required for iCloud CalDAV which uses a specific path structure.
func (c *AppleCalendarClient) discoverPrincipal() (string, error) {
	// First, try to discover the principal using PROPFIND on the root
	// Use Depth: 0 and simpler XML format that works with iCloud
	propfindBody := `<propfind xmlns='DAV:'><prop><current-user-principal/><calendar-home-set xmlns='urn:ietf:params:xml:ns:caldav'/></prop></propfind>`

	// Create a request with Depth: 0 for principal discovery
	url := strings.TrimSuffix(c.serverURL, "/") + "/"
	req, err := http.NewRequest("PROPFIND", url, strings.NewReader(propfindBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("User-Agent", "calendar-sync/1.0")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "0") // Use Depth: 0 for principal discovery

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to discover principal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusMultiStatus {
		// If root discovery fails, try common iCloud paths
		// iCloud uses different path structures depending on the server
		// Extract username part (before @) for some path formats
		usernamePart := c.username
		if idx := strings.Index(c.username, "@"); idx > 0 {
			usernamePart = c.username[:idx]
		}

		commonPaths := []string{
			fmt.Sprintf("/%s/principal/", usernamePart),
			fmt.Sprintf("/%s/calendars/", usernamePart),
			fmt.Sprintf("/%s/principal/", c.username),
			fmt.Sprintf("/%s/calendars/", c.username),
			"/calendars/",
			"/",
		}

		for _, path := range commonPaths {
			// Try with Depth: 0 for principal discovery
			testURL := strings.TrimSuffix(c.serverURL, "/") + path
			testReq, err := http.NewRequest("PROPFIND", testURL, strings.NewReader(propfindBody))
			if err == nil {
				testReq.SetBasicAuth(c.username, c.password)
				testReq.Header.Set("User-Agent", "calendar-sync/1.0")
				testReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
				testReq.Header.Set("Depth", "0")

				testResp, err := c.httpClient.Do(testReq)
				if err == nil {
					testResp.Body.Close()
					if testResp.StatusCode == http.StatusOK || testResp.StatusCode == http.StatusMultiStatus {
						return path, nil
					}
				}
			}
		}

		// Read response body for better error diagnostics
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("failed to discover principal: HTTP %d - %s (tried paths: %v)", resp.StatusCode, string(body), commonPaths)
	}

	// Parse the response to extract calendar-home-set and current-user-principal
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Extract calendar-home-set from XML response
	calendarHome := c.extractCalendarHomeFromXML(body)
	if calendarHome != "" {
		// Validate the extracted path
		if !strings.HasPrefix(calendarHome, "/") {
			calendarHome = "/" + calendarHome
		}
		return calendarHome, nil
	}

	// If calendar-home-set not found, try to extract current-user-principal
	// and then query the principal path for calendar-home-set
	principal := c.extractPrincipalFromXML(body)
	if principal != "" {
		// Query the principal path directly for calendar-home-set
		principalPropfindBody := `<propfind xmlns='DAV:'><prop><calendar-home-set xmlns='urn:ietf:params:xml:ns:caldav'/></prop></propfind>`
		principalURL := strings.TrimSuffix(c.serverURL, "/") + principal
		principalReq, err := http.NewRequest("PROPFIND", principalURL, strings.NewReader(principalPropfindBody))
		if err == nil {
			principalReq.SetBasicAuth(c.username, c.password)
			principalReq.Header.Set("User-Agent", "calendar-sync/1.0")
			principalReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
			principalReq.Header.Set("Depth", "0")
			principalResp, err := c.httpClient.Do(principalReq)
			if err == nil {
				defer principalResp.Body.Close()
				if principalResp.StatusCode == http.StatusOK || principalResp.StatusCode == http.StatusMultiStatus {
					principalBody, err := io.ReadAll(principalResp.Body)
					if err == nil {
						calendarHome = c.extractCalendarHomeFromXML(principalBody)
						if calendarHome != "" {
							if !strings.HasPrefix(calendarHome, "/") {
								calendarHome = "/" + calendarHome
							}
							return calendarHome, nil
						}
					}
				}
			}
		}
	}

	if principal == "" {
		// If we still can't find it, the XML might be in a different format
		// Try a more flexible extraction - look for any href in the response
		// that looks like a principal or calendar path
		bodyStr := string(body)
		// Look for href patterns that might be the principal
		hrefPatterns := []string{"<d:href>", "<href>", "href=\""}
		for _, pattern := range hrefPatterns {
			idx := strings.Index(bodyStr, pattern)
			if idx != -1 {
				// Extract the href value
				start := idx + len(pattern)
				if pattern == "href=\"" {
					start = idx + 6 // "href=\""
				}
				end := strings.Index(bodyStr[start:], "<")
				if end == -1 {
					end = strings.Index(bodyStr[start:], "\"")
				}
				if end == -1 {
					end = strings.Index(bodyStr[start:], ">")
				}
				if end > 0 {
					potentialPath := strings.TrimSpace(bodyStr[start : start+end])
					// Check if it looks like a valid path (contains / and doesn't have XML attributes)
					if strings.Contains(potentialPath, "/") && !strings.Contains(potentialPath, "xmlns") && !strings.Contains(potentialPath, "=") {
						if !strings.HasPrefix(potentialPath, "/") {
							potentialPath = "/" + potentialPath
						}
						// Test if this path works for calendar listing
						testURL := strings.TrimSuffix(c.serverURL, "/") + potentialPath
						testReq, _ := http.NewRequest("PROPFIND", testURL, strings.NewReader(propfindBody))
						testReq.SetBasicAuth(c.username, c.password)
						testReq.Header.Set("User-Agent", "calendar-sync/1.0")
						testReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
						testReq.Header.Set("Depth", "0")
						testResp, err := c.httpClient.Do(testReq)
						if err == nil {
							testResp.Body.Close()
							if testResp.StatusCode == http.StatusOK || testResp.StatusCode == http.StatusMultiStatus {
								principal = potentialPath
								break
							}
						}
					}
				}
			}
		}
	}

	if principal != "" {
		// Ensure principal is a relative path (starts with /)
		if !strings.HasPrefix(principal, "/") {
			principal = "/" + principal
		}

		// For iCloud, try different possible calendar home paths
		// The calendar home might be at:
		// 1. {principal}/calendars/ (most common)
		// 2. {principal} (if principal is already the calendar home)
		// 3. Extract numeric ID and try /{id}/calendars/ (some iCloud setups)
		testPaths := []string{}

		// Extract numeric ID from principal (e.g., /88940651/principal/ -> 88940651)
		parts := strings.Split(strings.Trim(principal, "/"), "/")
		var numericID string
		if len(parts) > 0 {
			numericID = parts[0]
		}

		if strings.HasSuffix(principal, "/") {
			testPaths = []string{
				principal + "calendars/",
				principal,
			}
		} else {
			testPaths = []string{
				principal + "/calendars/",
				principal + "/",
			}
		}

		// Also try paths based on numeric ID
		if numericID != "" {
			testPaths = append(testPaths, fmt.Sprintf("/%s/calendars/", numericID))
		}

		// Test each path to see which one works
		// Use a simple propfind for testing (not allprop which might not be supported)
		testPropfindBody := `<propfind xmlns='DAV:'><prop><resourcetype xmlns='DAV:'/></prop></propfind>`
		for _, testPath := range testPaths {
			testURL := strings.TrimSuffix(c.serverURL, "/") + testPath
			// Try with Depth: 1 to see if there are children (calendars)
			testReq, _ := http.NewRequest("PROPFIND", testURL, strings.NewReader(testPropfindBody))
			testReq.SetBasicAuth(c.username, c.password)
			testReq.Header.Set("User-Agent", "calendar-sync/1.0")
			testReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
			testReq.Header.Set("Depth", "1")
			testResp, err := c.httpClient.Do(testReq)
			if err == nil {
				testResp.Body.Close()
				if testResp.StatusCode == http.StatusOK || testResp.StatusCode == http.StatusMultiStatus {
					return testPath, nil
				}
			}
		}

		// If none of the tested paths work, return the most likely one
		// For iCloud, it's typically principal/calendars/
		if strings.HasSuffix(principal, "/") {
			return principal + "calendars/", nil
		}
		return principal + "/calendars/", nil
	}

	// Fallback to common iCloud path structure
	// Extract username part (before @) for path
	usernamePart := c.username
	if idx := strings.Index(c.username, "@"); idx > 0 {
		usernamePart = c.username[:idx]
	}
	return fmt.Sprintf("/%s/calendars/", usernamePart), nil
}

// extractPrincipalFromXML extracts the current-user-principal href from XML response.
func (c *AppleCalendarClient) extractPrincipalFromXML(body []byte) string {
	bodyStr := string(body)

	// Look for current-user-principal
	startIdx := strings.Index(bodyStr, "current-user-principal")
	if startIdx == -1 {
		return ""
	}

	// The href can be nested inside current-user-principal with namespace
	// Example: <current-user-principal xmlns="DAV:"><href xmlns="DAV:">/88940651/principal/</href></current-user-principal>
	// Find the <href> tag that comes after current-user-principal (may have namespace)
	searchStart := startIdx

	// Look for <href> or <href xmlns="DAV:"> pattern within the current-user-principal element
	// First, find where current-user-principal ends
	principalEnd := strings.Index(bodyStr[searchStart:], "</current-user-principal>")
	if principalEnd == -1 {
		principalEnd = strings.Index(bodyStr[searchStart:], "</d:current-user-principal>")
	}
	if principalEnd == -1 {
		// Fallback: search within next 500 chars
		principalEnd = 500
	}
	searchEnd := searchStart + principalEnd

	// Look for <href> within this range
	hrefTagStart := strings.Index(bodyStr[searchStart:searchEnd], "<href")
	if hrefTagStart == -1 {
		hrefTagStart = strings.Index(bodyStr[searchStart:searchEnd], "<d:href")
	}
	if hrefTagStart == -1 {
		return ""
	}

	hrefTagStart += searchStart

	// Find the start of the href value (skip past the tag and any attributes)
	// Look for the > that closes the opening tag
	valueStart := strings.Index(bodyStr[hrefTagStart:], ">")
	if valueStart == -1 {
		return ""
	}
	valueStart += hrefTagStart + 1 // Skip the >

	// Find the closing tag
	hrefEnd := strings.Index(bodyStr[valueStart:], "</href>")
	if hrefEnd == -1 {
		hrefEnd = strings.Index(bodyStr[valueStart:], "</d:href>")
	}
	if hrefEnd == -1 {
		return ""
	}

	href := strings.TrimSpace(bodyStr[valueStart : valueStart+hrefEnd])

	// Validate href - reject if it contains XML attributes
	if strings.Contains(href, "xmlns") || strings.Contains(href, "=") || strings.Contains(href, "<") || strings.Contains(href, ">") {
		return ""
	}

	// Ensure it's a relative path starting with /
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}

	// Make sure it ends with /
	if !strings.HasSuffix(href, "/") {
		href += "/"
	}

	return href
}

// extractCalendarHomeFromXML extracts the calendar-home-set href from XML response.
func (c *AppleCalendarClient) extractCalendarHomeFromXML(body []byte) string {
	// Simple extraction - look for calendar-home-set href
	// This is a simplified parser; a full implementation would use proper XML parsing
	bodyStr := string(body)

	// Look for calendar-home-set
	startIdx := strings.Index(bodyStr, "calendar-home-set")
	if startIdx == -1 {
		return ""
	}

	// Find the <href> tag that comes after calendar-home-set
	// Look for <href> or <d:href> or href=" pattern
	searchStart := startIdx
	// First try to find <href> tag
	hrefTagStart := strings.Index(bodyStr[searchStart:], "<href>")
	if hrefTagStart == -1 {
		hrefTagStart = strings.Index(bodyStr[searchStart:], "<d:href>")
	}
	if hrefTagStart == -1 {
		// Try href=" pattern
		hrefTagStart = strings.Index(bodyStr[searchStart:], "href=\"")
		if hrefTagStart == -1 {
			hrefTagStart = strings.Index(bodyStr[searchStart:], "href='")
		}
		if hrefTagStart != -1 {
			// For href=" pattern, skip href="
			hrefStart := searchStart + hrefTagStart + 6 // "href=\""
			hrefEnd := strings.Index(bodyStr[hrefStart:], "\"")
			if hrefEnd == -1 {
				hrefEnd = strings.Index(bodyStr[hrefStart:], "'")
			}
			if hrefEnd > 0 {
				href := bodyStr[hrefStart : hrefStart+hrefEnd]
				// Validate href - reject if it contains XML attributes
				if strings.Contains(href, "xmlns") || strings.Contains(href, "=") || strings.Contains(href, "<") || strings.Contains(href, ">") {
					return ""
				}
				// Ensure it's a relative path starting with /
				if !strings.HasPrefix(href, "/") {
					href = "/" + href
				}
				if !strings.HasSuffix(href, "/") {
					href += "/"
				}
				return href
			}
		}
		return ""
	}

	// Found <href> tag, extract content
	hrefStart := searchStart + hrefTagStart
	// Skip past <href> or <d:href>
	if strings.HasPrefix(bodyStr[hrefStart:], "<d:href>") {
		hrefStart += 8 // "<d:href>"
	} else {
		hrefStart += 6 // "<href>"
	}

	// Find closing tag
	hrefEnd := strings.Index(bodyStr[hrefStart:], "</href>")
	if hrefEnd == -1 {
		hrefEnd = strings.Index(bodyStr[hrefStart:], "</d:href>")
	}
	if hrefEnd == -1 {
		return ""
	}

	href := strings.TrimSpace(bodyStr[hrefStart : hrefStart+hrefEnd])

	// Validate href - reject if it contains XML attributes
	if strings.Contains(href, "xmlns") || strings.Contains(href, "=") || strings.Contains(href, "<") || strings.Contains(href, ">") {
		return ""
	}

	// Ensure it's a relative path starting with /
	if !strings.HasPrefix(href, "/") {
		href = "/" + href
	}

	// Make sure it ends with /
	if !strings.HasSuffix(href, "/") {
		href += "/"
	}

	return href
}

// CalendarInfo represents a calendar found in the CalDAV response.
type CalendarInfo struct {
	Name string
	Path string
}

// parseCalendarListFromXML parses the PROPFIND response to extract calendar list.
func (c *AppleCalendarClient) parseCalendarListFromXML(body []byte) []CalendarInfo {
	var calendars []CalendarInfo
	bodyStr := string(body)

	// Look for all <response> blocks
	responseIdx := 0
	for {
		responseStart := strings.Index(bodyStr[responseIdx:], "<response")
		if responseStart == -1 {
			break
		}
		responseStart += responseIdx

		// Find the end of this response block
		responseEnd := strings.Index(bodyStr[responseStart:], "</response>")
		if responseEnd == -1 {
			break
		}
		responseEnd += responseStart + len("</response>")

		responseBlock := bodyStr[responseStart:responseEnd]

		// Extract href (the path)
		hrefStart := strings.Index(responseBlock, "href")
		if hrefStart != -1 {
			// Find href value
			hrefValueStart := strings.Index(responseBlock[hrefStart:], ">")
			if hrefValueStart == -1 {
				hrefValueStart = strings.Index(responseBlock[hrefStart:], "=\"")
				if hrefValueStart != -1 {
					hrefValueStart += 2
					hrefValueEnd := strings.Index(responseBlock[hrefStart+hrefValueStart:], "\"")
					if hrefValueEnd > 0 {
						path := responseBlock[hrefStart+hrefValueStart : hrefStart+hrefValueStart+hrefValueEnd]

						// Extract displayname
						displayNameStart := strings.Index(responseBlock, "displayname")
						var name string
						if displayNameStart != -1 {
							nameStart := strings.Index(responseBlock[displayNameStart:], ">")
							if nameStart != -1 {
								nameStart += displayNameStart + 1
								nameEnd := strings.Index(responseBlock[nameStart:], "<")
								if nameEnd > 0 {
									name = strings.TrimSpace(responseBlock[nameStart : nameStart+nameEnd])
								}
							}
						}

						if path != "" {
							calendars = append(calendars, CalendarInfo{
								Name: name,
								Path: path,
							})
						}
					}
				}
			} else {
				hrefValueStart += hrefStart + 1
				hrefValueEnd := strings.Index(responseBlock[hrefValueStart:], "<")
				if hrefValueEnd > 0 {
					path := strings.TrimSpace(responseBlock[hrefValueStart : hrefValueStart+hrefValueEnd])

					// Extract displayname
					displayNameStart := strings.Index(responseBlock, "displayname")
					var name string
					if displayNameStart != -1 {
						nameStart := strings.Index(responseBlock[displayNameStart:], ">")
						if nameStart != -1 {
							nameStart += displayNameStart + 1
							nameEnd := strings.Index(responseBlock[nameStart:], "<")
							if nameEnd > 0 {
								name = strings.TrimSpace(responseBlock[nameStart : nameStart+nameEnd])
							}
						}
					}

					if path != "" {
						calendars = append(calendars, CalendarInfo{
							Name: name,
							Path: path,
						})
					}
				}
			}
		}

		responseIdx = responseEnd
	}

	return calendars
}

// createCalendar creates a new calendar using CalDAV MKCALENDAR method (RFC 4791).
// Falls back to MKCOL if MKCALENDAR is not supported.
func (c *AppleCalendarClient) createCalendar(path, name string) error {
	url := strings.TrimSuffix(c.serverURL, "/") + path

	// First, try MKCALENDAR (RFC 4791) - the proper CalDAV method for creating calendars
	// According to https://www.onecal.io/blog/how-to-integrate-icloud-calendar-api-into-your-app
	// iCloud supports MKCALENDAR with write rights (see Section 5.3.1 of RFC 4791)
	mkcalendarBody := `<?xml version="1.0" encoding="utf-8"?>
<mkcalendar xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <set>
    <prop>
      <displayname xmlns="DAV:">` + name + `</displayname>
      <C:calendar-description xmlns:C="urn:ietf:params:xml:ns:caldav">Synced calendar from work account</C:calendar-description>
    </prop>
  </set>
</mkcalendar>`

	req, err := http.NewRequest("MKCALENDAR", url, strings.NewReader(mkcalendarBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("User-Agent", "calendar-sync/1.0")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create calendar: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for detailed error logging
	respBody, _ := io.ReadAll(resp.Body)
	respBodyStr := string(respBody)

	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}

	// If MKCALENDAR returns 400, try with resourcetype included
	if resp.StatusCode == http.StatusBadRequest {
		// Try MKCALENDAR with resourcetype explicitly set
		mkcalendarBody2 := `<?xml version="1.0" encoding="utf-8"?>
<mkcalendar xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <set>
    <prop>
      <resourcetype>
        <collection/>
        <C:calendar/>
      </resourcetype>
      <displayname xmlns="DAV:">` + name + `</displayname>
    </prop>
  </set>
</mkcalendar>`

		req1b, err := http.NewRequest("MKCALENDAR", url, strings.NewReader(mkcalendarBody2))
		if err == nil {
			req1b.SetBasicAuth(c.username, c.password)
			req1b.Header.Set("User-Agent", "calendar-sync/1.0")
			req1b.Header.Set("Content-Type", "application/xml; charset=utf-8")
			resp1b, err := c.httpClient.Do(req1b)
			if err == nil {
				resp1bBody, _ := io.ReadAll(resp1b.Body)
				resp1b.Body.Close()
				if resp1b.StatusCode == http.StatusCreated || resp1b.StatusCode == http.StatusOK {
					return nil
				}
				// If still 400, include the response in error
				if resp1b.StatusCode == http.StatusBadRequest {
					respBodyStr = string(resp1bBody)
				}
			}
		}
	}

	// If MKCALENDAR is not supported (405 Method Not Allowed), try extended MKCOL
	if resp.StatusCode == http.StatusMethodNotAllowed {
		// Try extended MKCOL with calendar properties
		mkcolBody := `<?xml version="1.0" encoding="utf-8"?>
<mkcol xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <set>
    <prop>
      <resourcetype>
        <collection/>
        <C:calendar/>
      </resourcetype>
      <displayname xmlns="DAV:">` + name + `</displayname>
      <C:calendar-description xmlns:C="urn:ietf:params:xml:ns:caldav">Synced calendar from work account</C:calendar-description>
    </prop>
  </set>
</mkcol>`

		req2, err := http.NewRequest("MKCOL", url, strings.NewReader(mkcolBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req2.SetBasicAuth(c.username, c.password)
		req2.Header.Set("User-Agent", "calendar-sync/1.0")
		req2.Header.Set("Content-Type", "application/xml; charset=utf-8")

		resp2, err := c.httpClient.Do(req2)
		if err != nil {
			return fmt.Errorf("failed to create calendar: %w", err)
		}
		defer resp2.Body.Close()

		resp2Body, _ := io.ReadAll(resp2.Body)

		if resp2.StatusCode == http.StatusCreated || resp2.StatusCode == http.StatusOK {
			return nil
		}

		return fmt.Errorf("failed to create calendar with MKCOL: HTTP %d\nRequest URL: %s\nResponse: %s", resp2.StatusCode, url, string(resp2Body))
	}

	// MKCALENDAR was attempted but failed - include detailed error information
	// Include response headers for debugging
	headers := ""
	for k, v := range resp.Header {
		headers += fmt.Sprintf("  %s: %s\n", k, strings.Join(v, ", "))
	}

	errorMsg := fmt.Sprintf("failed to create calendar with MKCALENDAR: HTTP %d\nRequest URL: %s\nRequest Body: %s\nResponse Body: %s\nResponse Headers:\n%s\n\nNote: According to https://www.onecal.io/blog/how-to-integrate-icloud-calendar-api-into-your-app, iCloud supports MKCALENDAR, but there may be specific requirements or permissions needed. Please check:\n1. Your app-specific password has write permissions\n2. The calendar path format is correct\n3. If issues persist, create the calendar manually in Apple Calendar/iCloud",
		resp.StatusCode, url, mkcalendarBody, respBodyStr, headers)

	return fmt.Errorf(errorMsg)
}

// FindOrCreateCalendarByName finds an existing calendar by name or creates a new one.
// Returns the calendar path.
func (c *AppleCalendarClient) FindOrCreateCalendarByName(name string, colorID string) (string, error) {
	// List calendars using PROPFIND - request displayname to identify calendars
	propfindBody := `<propfind xmlns='DAV:'><prop><displayname xmlns='DAV:'/></prop></propfind>`

	// Use Depth: 1 to get immediate children (calendars)
	url := strings.TrimSuffix(c.serverURL, "/") + c.basePath
	req, err := http.NewRequest("PROPFIND", url, strings.NewReader(propfindBody))
	if err != nil {
		return "", fmt.Errorf("apple: failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("User-Agent", "calendar-sync/1.0")
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", "1") // Depth: 1 for listing calendars

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("apple: failed to list calendars: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		// Read response body for better error message
		body, err := io.ReadAll(resp.Body)
		bodyStr := ""
		if err == nil {
			bodyStr = string(body)
		}

		// If we get 400, the XML format might be wrong - try different formats
		if resp.StatusCode == http.StatusBadRequest {
			// Try with propname (just property names, no values)
			propnameBody := `<propfind xmlns='DAV:'><propname/></propfind>`
			propnameReq, _ := http.NewRequest("PROPFIND", url, strings.NewReader(propnameBody))
			propnameReq.SetBasicAuth(c.username, c.password)
			propnameReq.Header.Set("User-Agent", "calendar-sync/1.0")
			propnameReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
			propnameReq.Header.Set("Depth", "1")
			propnameResp, err := c.httpClient.Do(propnameReq)
			if err == nil {
				defer propnameResp.Body.Close()
				if propnameResp.StatusCode == http.StatusOK || propnameResp.StatusCode == http.StatusMultiStatus {
					// The propname format worked, read the response
					propnameBody, err := io.ReadAll(propnameResp.Body)
					if err == nil {
						calendars := c.parseCalendarListFromXML(propnameBody)
						for _, cal := range calendars {
							if cal.Name == name {
								return cal.Path, nil
							}
						}
						return "", fmt.Errorf("calendar '%s' not found. Found calendars: %v", name, calendars)
					}
				}
			}

			// Try with specific properties but without redundant namespace declarations
			specificBody := `<propfind xmlns='DAV:'><prop><displayname/></prop></propfind>`
			specificReq, _ := http.NewRequest("PROPFIND", url, strings.NewReader(specificBody))
			specificReq.SetBasicAuth(c.username, c.password)
			specificReq.Header.Set("User-Agent", "calendar-sync/1.0")
			specificReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
			specificReq.Header.Set("Depth", "1")
			specificResp, err := c.httpClient.Do(specificReq)
			if err == nil {
				defer specificResp.Body.Close()
				if specificResp.StatusCode == http.StatusOK || specificResp.StatusCode == http.StatusMultiStatus {
					// The specific format worked, read the response
					specificBody, err := io.ReadAll(specificResp.Body)
					if err == nil {
						calendars := c.parseCalendarListFromXML(specificBody)
						for _, cal := range calendars {
							if cal.Name == name {
								return cal.Path, nil
							}
						}
						return "", fmt.Errorf("calendar '%s' not found. Found calendars: %v", name, calendars)
					}
				}
			}
		}

		// If we get 403, the path might be wrong - try alternative paths
		if resp.StatusCode == http.StatusForbidden {
			// Try using just the principal path (without /calendars/)
			if strings.HasSuffix(c.basePath, "/calendars/") {
				altPath := strings.TrimSuffix(c.basePath, "/calendars/")
				if !strings.HasSuffix(altPath, "/") {
					altPath += "/"
				}
				altURL := strings.TrimSuffix(c.serverURL, "/") + altPath
				altReq, _ := http.NewRequest("PROPFIND", altURL, strings.NewReader(propfindBody))
				altReq.SetBasicAuth(c.username, c.password)
				altReq.Header.Set("User-Agent", "calendar-sync/1.0")
				altReq.Header.Set("Content-Type", "application/xml; charset=utf-8")
				altReq.Header.Set("Depth", "1")
				altResp, err := c.httpClient.Do(altReq)
				if err == nil {
					altResp.Body.Close()
					if altResp.StatusCode == http.StatusOK || altResp.StatusCode == http.StatusMultiStatus {
						// Update basePath and retry
						c.basePath = altPath
						return c.FindOrCreateCalendarByName(name, colorID)
					}
				}
			}
		}

		return "", fmt.Errorf("apple: failed to list calendars: HTTP %d - %s (path: %s, url: %s)", resp.StatusCode, bodyStr, c.basePath, url)
	}

	// Parse XML response to find calendar by name
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("apple: failed to read calendar list response: %w", err)
	}

	// Parse the XML to find calendars
	calendars := c.parseCalendarListFromXML(body)

	// Check if a calendar with the given name exists
	for _, cal := range calendars {
		if cal.Name == name {
			return cal.Path, nil
		}
	}

	// Calendar doesn't exist - try to create it
	// According to RFC 4791 and iCloud documentation, MKCALENDAR is supported
	// iCloud typically uses UUID-based paths for calendars (as seen in existing calendars)
	// Generate a UUID v4 for the calendar path
	uuidBytes := make([]byte, 16)
	rand.Read(uuidBytes)
	uuidBytes[6] = (uuidBytes[6] & 0x0f) | 0x40 // Version 4
	uuidBytes[8] = (uuidBytes[8] & 0x3f) | 0x80 // Variant 10
	uuid := fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(uuidBytes[0:4]),
		hex.EncodeToString(uuidBytes[4:6]),
		hex.EncodeToString(uuidBytes[6:8]),
		hex.EncodeToString(uuidBytes[8:10]),
		hex.EncodeToString(uuidBytes[10:16]))

	calendarPath := c.basePath + strings.ToUpper(uuid) + "/"
	// Make sure the path doesn't have double slashes
	calendarPath = strings.ReplaceAll(calendarPath, "//", "/")

	// Create calendar using MKCOL
	err = c.createCalendar(calendarPath, name)
	if err != nil {
		// If creation fails, provide helpful error message
		return "", fmt.Errorf("apple: calendar '%s' not found and automatic creation failed: %w\n\nPlease create the calendar '%s' manually in Apple Calendar/iCloud, then run the sync again.", name, err, name)
	}

	// After creation, re-list to get the actual path (iCloud may assign a different path)
	req2, err := http.NewRequest("PROPFIND", url, strings.NewReader(propfindBody))
	if err == nil {
		req2.SetBasicAuth(c.username, c.password)
		req2.Header.Set("User-Agent", "calendar-sync/1.0")
		req2.Header.Set("Content-Type", "application/xml; charset=utf-8")
		req2.Header.Set("Depth", "1")
		resp2, err := c.httpClient.Do(req2)
		if err == nil {
			defer resp2.Body.Close()
			if resp2.StatusCode == http.StatusOK || resp2.StatusCode == http.StatusMultiStatus {
				body2, err := io.ReadAll(resp2.Body)
				if err == nil {
					calendars2 := c.parseCalendarListFromXML(body2)
					for _, cal := range calendars2 {
						if cal.Name == name {
							return cal.Path, nil
						}
					}
				}
			}
		}
	}

	// If we can't find it in the list, return the path we created
	return calendarPath, nil
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

	// Generate a unique event ID - ensure it ends with .ics
	// The event.Id from Google Calendar might contain special characters that need to be sanitized
	eventID := event.Id
	if eventID == "" {
		// Generate a UID if not present
		eventID = fmt.Sprintf("%s@calendar-sync", time.Now().Format(time.RFC3339Nano))
	}

	// Sanitize the event ID for use in URL (remove special characters that might cause issues)
	eventID = strings.ReplaceAll(eventID, "/", "-")
	eventID = strings.ReplaceAll(eventID, "\\", "-")
	eventID = strings.ReplaceAll(eventID, ":", "-")

	if !strings.HasSuffix(eventID, ".ics") {
		eventID = eventID + ".ics"
	}

	// Build the full URL - ensure calendarID ends with / and we don't have double slashes
	calendarPath := strings.TrimSuffix(calendarID, "/") + "/"
	url := strings.TrimSuffix(c.serverURL, "/") + calendarPath + eventID

	// Create PUT request with proper headers for iCalendar
	req, err := http.NewRequest("PUT", url, &buf)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("User-Agent", "calendar-sync/1.0")
	req.Header.Set("Content-Type", "text/calendar; charset=utf-8")

	// Get iCalendar content for error reporting
	icalContent := buf.String()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	respBody, _ := io.ReadAll(resp.Body)
	respBodyStr := string(respBody)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		// Include detailed error information
		headers := ""
		for k, v := range resp.Header {
			headers += fmt.Sprintf("  %s: %s\n", k, strings.Join(v, ", "))
		}
		// Include first 1000 chars of iCalendar content in error for debugging
		icalPreview := icalContent
		if len(icalPreview) > 1000 {
			icalPreview = icalPreview[:1000] + "..."
		}
		return fmt.Errorf("failed to insert event: HTTP %d\nRequest URL: %s\niCalendar Content (first 1000 chars):\n%s\nResponse Body: %s\nResponse Headers:\n%s",
			resp.StatusCode, url, icalPreview, respBodyStr, headers)
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
				// Set VALUE=DATE parameter for all-day events
				dtstart.Params.Set("VALUE", "DATE")
				vevent.Props.Set(dtstart)
			}
		} else if event.Start.DateTime != "" {
			// Timed event
			startTime, err := time.Parse(time.RFC3339, event.Start.DateTime)
			if err == nil {
				dtstart := ical.NewProp("DTSTART")
				dtstart.SetDateTime(startTime)
				// Ensure timezone is UTC if not specified
				if startTime.Location() == time.UTC {
					dtstart.Params.Set("TZID", "UTC")
				}
				vevent.Props.Set(dtstart)
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
				// Set VALUE=DATE parameter for all-day events
				dtend.Params.Set("VALUE", "DATE")
				vevent.Props.Set(dtend)
			}
		} else if event.End.DateTime != "" {
			// Timed event
			endTime, err := time.Parse(time.RFC3339, event.End.DateTime)
			if err == nil {
				dtend := ical.NewProp("DTEND")
				dtend.SetDateTime(endTime)
				// Ensure timezone is UTC if not specified
				if endTime.Location() == time.UTC {
					dtend.Params.Set("TZID", "UTC")
				}
				vevent.Props.Set(dtend)
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
	now := time.Now().UTC()
	vevent.Props.SetDateTime(ical.PropDateTimeStamp, now)
	vevent.Props.SetDateTime(ical.PropCreated, now)
	vevent.Props.SetDateTime(ical.PropLastModified, now)

	// Set DTSTAMP (required by RFC 5545) - must be in UTC
	dtstamp := ical.NewProp("DTSTAMP")
	dtstamp.SetDateTime(now)
	vevent.Props.Set(dtstamp)

	return cal, nil
}

// parseICalDateTime parses an iCalendar date-time property.
func parseICalDateTime(prop *ical.Prop) (time.Time, error) {
	// Use the library's DateTime method which handles parsing
	// Pass nil for location to use UTC
	return prop.DateTime(nil)
}
