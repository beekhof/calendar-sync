Here is a detailed, step-by-step blueprint for building the Go-based calendar sync tool, followed by a series of prompts for a code-generation LLM.

### Project Blueprint & Phased Rollout

The project will be a command-line Go tool. The core design is to have a stateless `Syncer` service that is given API clients and configuration, performs the sync, and then exits. This makes it perfect for execution via a `cron` job.

**Project Structure:**

  * `/main.go`: Entrypoint, environment loading, and wiring.
  * `/config.go`: Structs and loading logic for configuration (from env vars).
  * `/auth.go`: Handles the complex OAuth 2.0 flow, token storage, and client creation.
  * `/store.go`: A simple file-based token store to persist refresh tokens.
  * `/googlecalendar.go`: A wrapper for the Google Calendar API, providing high-level methods (e.g., `GetFilteredEvents`, `FindOrCreateCalendar`).
  * `/syncer.go`: The core business logic, including event comparison, filtering, and the main `Sync()` method.

**Iterative Chunks (Phases):**

1.  **Phase 1: Config & Token Storage:** Lay the foundation. Create the `Config` struct and the file-based `TokenStore` for saving and loading OAuth tokens.
2.  **Phase 2: Authentication:** Build the OAuth 2.0 flow. This is the most complex *interactive* part. We'll create a function that can guide a user through authentication if a token file is missing and create an authenticated `http.Client`.
3.  **Phase 3: Google Calendar API Client:** Create a wrapper around the Google Calendar API. This client will use the authenticated `http.Client` from Phase 2 and provide clean, testable methods like `FindOrCreateCalendarByName`, `GetEventsInWindow`, etc.
4.  **Phase 4: Core Syncer Logic:** Create the `Syncer` struct. This is the "brains" of the operation. It will contain the filtering logic (6am-midnight, OOF, etc.) and the comparison logic (create, update, delete).
5.  **Phase 5: Main Wiring:** Connect all the pieces in `main.go`. The main function will load config, get the two (work/personal) authenticated clients, pass them to the `Syncer`, and call `Sync()`.

-----

### Prompts for Code Generation

Here is the series of prompts to build the entire project, step-by-step.

-----

```text
Prompt 1: Project Setup and Configuration

We'll start by setting up the project's configuration.

Please generate a Go package with two files: `config.go` and `config_test.go`.

In `config.go`:
1.  Define a `Config` struct.
2.  This struct should contain fields for:
    * `WorkTokenPath` (string)
    * `PersonalTokenPath` (string)
    * `SyncCalendarName` (string)
    * `SyncCalendarColorID` (string, e.g., "7" for Grape)
3.  Create a function `LoadConfig() (*Config, error)` that loads these values from environment variables: `WORK_TOKEN_PATH`, `PERSONAL_TOKEN_PATH`, `SYNC_CALENDAR_NAME`, and `SYNC_CALENDAR_COLOR_ID`.
4.  Return an error if any of these variables are missing.

In `config_test.go`:
1.  Write a unit test `TestLoadConfig` that uses `t.Setenv` to set all the required environment variables.
2.  Call `LoadConfig()` and assert that the returned `Config` struct contains the correct values.
3.  Write a second test `TestLoadConfigMissing` that leaves one variable unset and asserts that an error is returned.
```

-----

```text
Prompt 2: Token Storage

We need a way to save and load the OAuth tokens from the file paths defined in our config.

Please generate a Go package with two files: `store.go` and `store_test.go`.

In `store.go`:
1.  Define a `FileTokenStore` struct that has one field: `Path` (string).
2.  Create a `NewFileTokenStore(path string) *FileTokenStore` constructor.
3.  Implement a method `SaveToken(token *oauth2.Token) error`. This method should:
    * Marshal the token into JSON.
    * Write the JSON to the file at `store.Path`.
4.  Implement a method `LoadToken() (*oauth2.Token, error)`. This method should:
    * Check if the file at `store.Path` exists. If not, return `nil, nil` (no error).
    * If it exists, read the file, unmarshal the JSON into an `*oauth2.Token`, and return it.

In `store_test.go`:
1.  Write a unit test `TestFileTokenStore_SaveLoad`.
2.  Use `t.TempDir()` to create a temporary directory for the token file.
3.  Create a new token store pointing to a file in that directory.
4.  Create a sample `oauth2.Token` (with `AccessToken`, `RefreshToken`, and `Expiry`).
5.  Call `SaveToken()`.
6.  Call `LoadToken()` and assert that the loaded token's fields match the saved token.
7.  Write a second test `TestFileTokenStore_LoadEmpty` that points to a non-existent file and asserts that `LoadToken()` returns `nil, nil`.
```

-----

```text
Prompt 3: OAuth 2.0 Authentication Helper

This is a critical piece. We need a helper to handle the OAuth 2.0 flow, including the interactive part for the first run.

Please generate a Go package with two files: `auth.go` and `auth_test.go`.

In `auth.go`:
1.  Define a new struct `TokenStore` as an interface with `SaveToken` and `LoadToken` methods (matching our `FileTokenStore` from Prompt 2).
2.  Create a function `GetAuthenticatedClient(ctx context.Context, oauthConfig *oauth2.Config, tokenStore TokenStore) (*http.Client, error)`.
3.  This function's logic should be:
    * Attempt to load a token using `tokenStore.LoadToken()`.
    * **If token is `nil` (first run):**
        * Generate an auth URL: `authURL := oauthConfig.AuthCodeURL("state-token", oauth2.AccessTypeOffline)`.
        * Print the URL to the console, asking the user to visit it.
        * Read the auth code from `os.Stdin`.
        * Exchange the code for a token: `token, err := oauthConfig.Exchange(ctx, code)`.
        * If successful, save the new token using `tokenStore.SaveToken(token)`.
    * **If token is not `nil` (subsequent runs):**
        * The loaded token is used.
    * Create a new `oauth2.Config` token source: `tokenSource := oauthConfig.TokenSource(ctx, token)`.
    * Create a *new* token source that wraps the original one, which will auto-save any refreshed tokens: `autoSaveTokenSource := oauth2.ReuseTokenSource(token, tokenSource)`.
    * Implement `oauth2.TokenSource` for `autoSaveTokenSource` and in the `Token()` method, if the token is refreshed, call `tokenStore.SaveToken()`.
    * Return a new `http.Client` using this `autoSaveTokenSource`: `return oauth2.NewClient(ctx, autoSaveTokenSource), nil`.

In `auth_test.go`:
1.  This function is hard to unit test due to the interactive prompt.
2.  Instead, write a test `TestGetAuthenticatedClient_TokenExists` that mocks the `TokenStore`.
3.  The mock `LoadToken()` should return a valid, non-expired token.
4.  Assert that the function returns a valid `*http.Client` without trying to read from `os.Stdin`.
```

-----

```text
Prompt 4: Google Calendar API Client

Now we'll build a high-level wrapper around the Google Calendar API, using the `http.Client` from the previous step.

Please generate a Go package with two files: `googlecalendar.go` and `googlecalendar_test.go`.

In `googlecalendar.go`:
1.  Define a `Client` struct that holds a `*calendar.Service` from the Google API client library.
2.  Create a `NewClient(ctx context.Context, httpClient *http.Client) (*Client, error)` constructor. It should initialize the `calendar.Service` using the provided `httpClient`.
3.  Implement `FindOrCreateCalendarByName(name string, colorID string) (string, error)`:
    * It should list the user's calendars.
    * If a calendar with `summary == name` exists, return its `id`.
    * If not, create a new calendar with the given `name` and `colorID` and return the new `id`.
4.  Implement `GetEvents(calendarID string, timeMin, timeMax time.Time) ([]*calendar.Event, error)`:
    * Calls the `Events.List` API.
    * **Important:** It must set `SingleEvents = true` to expand recurring events, as per our spec.
    * It should set `TimeMin` and `TimeMax` based on the function parameters.
    .   Return the list of events.
5.  Implement `FindEventsByWorkID(calendarID, workEventID string) ([]*calendar.Event, error)`:
    * This calls `Events.List` and uses the `privateExtendedProperty` query to find events with `"workEventId=" + workEventID`.
6.  Implement `InsertEvent(calendarID string, event *calendar.Event) error`:
    * This calls `Events.Insert` and **must** set `sendUpdates="none"`, as per our spec.
7.  Implement `UpdateEvent(calendarID, eventID string, event *calendar.Event) error`:
    * This calls `Events.Update`.
8.  Implement `DeleteEvent(calendarID, eventID string) error`:
    * This calls `Events.Delete`.

In `googlecalendar_test.go`:
1.  Write unit tests for these methods using a mock `*calendar.Service` or `httptest.NewServer`.
2.  `TestGetEvents`: Assert that `SingleEvents` is set to `true` on the API call.
3.  `TestInsertEvent`: Assert that `sendUpdates` is set to `"none"` on the API call.
4.  `TestFindEventsByWorkID`: Assert that the `privateExtendedProperty` field is correctly formatted.
```

-----

```text
Prompt 5: Core Syncer Logic

This is the "brains" of the tool. It implements all the business rules from our spec.

Please generate a Go package with two files: `syncer.go` and `syncer_test.go`.

In `syncer.go`:
1.  Define a `googleCalendarClient` interface. This interface should list all the methods we created in `googlecalendar.go` (e.g., `FindOrCreateCalendarByName`, `GetEvents`, `InsertEvent`, `UpdateEvent`, `DeleteEvent`, `FindEventsByWorkID`).
2.  Define a `Syncer` struct that holds:
    * `workClient` (googleCalendarClient)
    * `personalClient` (googleCalendarClient)
    * `config` (*Config)
3.  Create a `NewSyncer(workClient, personalClient googleCalendarClient, config *Config) *Syncer` constructor.
4.  Implement a private method `filterEvents(events []*calendar.Event) []*calendar.Event`:
    * This method implements the spec logic:
    * Rule 1: Keep all-day events (even OOF).
    * Rule 2: Skip timed OOF events.
    * Rule 3: Skip events *entirely* outside 6:00 AM - 12:00 AM (midnight). Keep any event that *partially* overlaps this window.
    * It should return a new slice containing only the events that pass the filters.
5.  Implement a private method `prepareSyncEvent(sourceEvent *calendar.Event) *calendar.Event`:
    * This creates a new `calendar.Event` for the personal calendar.
    * It copies `summary`, `description`, `location`, `start`, `end`, and `conferenceData`.
    * It **omits** `attendees` (guest list).
    * It **must** set `reminders.useDefault = true`.
    * It **must** set `extendedProperties.private["workEventId"] = sourceEvent.Id` for tracking.
6.  Implement the main public method `Sync(ctx context.Context) error`:
    * `log.Println("Starting sync...")`
    * `destCalendarID, err := personalClient.FindOrCreateCalendarByName(config.SyncCalendarName, config.SyncCalendarColorID)`
    * Calculate `timeMin` (start of current week) and `timeMax` (end of next week).
    * `sourceEvents, err := workClient.GetEvents("primary", timeMin, timeMax)`
    * `filteredEvents := s.filterEvents(sourceEvents)`
    * Create a map of `filteredEvents` by ID for easy lookup: `sourceEventsMap`.
    * `destEvents, err := personalClient.GetEvents(destCalendarID, timeMin, timeMax)`
    * Loop through `destEvents`:
        * `workID := destEvent.ExtendedProperties.Private["workEventId"]`
        * `sourceEvent, exists := sourceEventsMap[workID]`
        * **If `exists` (Update/Check):**
            * Compare `destEvent` with `sourceEvent`. If they differ (check `summary`, `start`, `end`, `description`, etc.), call `personalClient.UpdateEvent(...)` with the `prepareSyncEvent(sourceEvent)`.
            * Remove `workID` from `sourceEventsMap` to mark it as processed.
        * **If `!exists` (Delete Stale):**
            * This event is on the personal calendar but not in the source. Delete it: `personalClient.DeleteEvent(...)`.
    * Loop through any remaining events in `sourceEventsMap` (these are new):
        * `personalClient.InsertEvent(destCalendarID, s.prepareSyncEvent(newEvent))`
    * `log.Println("Sync complete.")`
    * Return `nil`.

In `syncer_test.go`:
1.  Write extensive unit tests for `filterEvents` using sample `calendar.Event` objects.
    * Test: Timed OOF is skipped.
    * Test: All-day OOF is kept.
    * Test: 5:00 AM - 5:30 AM event is skipped.
    * Test: 5:30 AM - 6:30 AM event is kept.
2.  Write unit tests for the main `Sync` method using mocks for the `googleCalendarClient` interface.
    * Test: A new event on the work calendar causes `InsertEvent` to be called.
    * Test: A deleted event on the work calendar causes `DeleteEvent` to be called.
    * Test: An unchanged event causes no API calls.
    * Test: A changed event (e.g., new time) causes `UpdateEvent` to be called.
```

-----

```text
Prompt 6: Final Main Wiring

This is the final prompt. We will wire everything together in `main.go`.

Please generate a `main.go` file. This file should replace any previous `main.go` file.

The `main` function should:
1.  Set up logging: `log.SetFlags(log.LstdFlags | log.Lshortfile)`.
2.  `ctx := context.Background()`.
3.  Call `LoadConfig()` (from Prompt 1). Handle any errors with `log.Fatalf`.
4.  Define the OAuth 2.0 configuration:
    * `googleOAuthConfig := &oauth2.Config{...}`
    * Use `os.Getenv("GOOGLE_CLIENT_ID")` and `os.Getenv("GOOGLE_CLIENT_SECRET")`.
    * Set `RedirectURL` to `"urn:ietf:wg:oauth:2.0:oob"`.
    * Set `Scopes` to `[]string{calendar.CalendarScope, calendar.CalendarEventsScope}`.
5.  Create the two token stores:
    * `workTokenStore := NewFileTokenStore(config.WorkTokenPath)`
    * `personalTokenStore := NewFileTokenStore(config.PersonalTokenPath)`
6.  Get the two authenticated clients using our `auth.go` helper (from Prompt 3):
    * `workHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, workTokenStore)`
    * `personalHTTPClient, err := GetAuthenticatedClient(ctx, googleOAuthConfig, personalTokenStore)`
7.  Create the two high-level Google Calendar clients (from Prompt 4):
    * `workClient, err := NewClient(ctx, workHTTPClient)`
    * `personalClient, err := NewClient(ctx, personalHTTPClient)`
8.  Create the `Syncer` (from Prompt 5):
    * `syncer := NewSyncer(workClient, personalClient, config)`
9.  Run the sync:
    * `if err := syncer.Sync(ctx); err != nil { log.Fatalf("Sync failed: %v", err) }`
10. If successful:
    * `log.Println("Sync completed successfully.")`

This `main.go` file should import all the packages we've built: `config`, `store`, `auth`, `googlecalendar`, and `syncer`.
```