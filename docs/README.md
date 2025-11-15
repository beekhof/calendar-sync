# Calendar Sync Tool

A one-way synchronization tool that syncs events from a work Google Calendar to a personal calendar (Google Calendar or Apple Calendar/iCloud). This tool is designed to work around admin restrictions by using API access instead of the standard calendar sharing UI.

## Overview

The Calendar Sync Tool creates a read-only "Work Sync" calendar in your personal calendar account (Google Calendar or Apple Calendar/iCloud), populated with filtered events from your work Google Calendar. The work calendar is the "source of truth" - any manual changes made to the synced calendar will be overwritten on the next sync.

### Key Features

- **One-way sync**: Work calendar → Personal calendar (Google or Apple)
- **Multiple destination support**: Sync to Google Calendar or Apple Calendar/iCloud
- **Automatic filtering**: Only syncs relevant events (6 AM - midnight, excludes timed OOF events)
- **Recurring event expansion**: Expands recurring events into individual instances
- **Two-week window**: Syncs current week + next week
- **Automatic cleanup**: Removes stale events outside the sync window
- **Flexible configuration**: Config file, environment variables, or command-line flags

## Installation

### Prerequisites

- Go 1.21 or later
- Google OAuth 2.0 credentials (Client ID and Client Secret)

### Build

```bash
# Clone the repository
git clone <repository-url>
cd calendar-sync

# Build the binary
make build
# or
go build -o calsync .
```

## Setup

### 1. Get Google OAuth Credentials

1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project or select an existing one
3. Enable the Google Calendar API
4. **Configure OAuth Consent Screen**:
   - Go to "APIs & Services" → "OAuth consent screen"
   - Choose "External" user type (unless you're using a Google Workspace account and want to restrict to your organization)
   - Fill in the required app information (App name, User support email, Developer contact email)
   - Add scopes: `https://www.googleapis.com/auth/calendar.readonly` and `https://www.googleapis.com/auth/calendar.events`
   - Add test users (if app is in Testing mode): Add your personal and work email addresses
   - Save and continue through the steps
5. **Create OAuth Credentials**:
   - Go to "Credentials" → "Create Credentials" → "OAuth 2.0 Client ID"
   - Choose "Desktop app" as the application type
   - **Important**: Add `http://127.0.0.1:8080` (or `http://localhost:8080`) to the "Authorized redirect URIs" list
   - Click "Download" to download the credentials JSON file
6. Save the JSON file in a secure location (e.g., `~/credentials.json`)

**Note**: The tool uses a local HTTP server on port 8080 (or a random port if 8080 is unavailable) to receive the OAuth callback. Make sure this redirect URI is added to your OAuth client configuration in Google Cloud Console.

**Troubleshooting**:
- If you see "Access blocked: [app] can only be used within its organization":
  - Go to "OAuth consent screen" in Google Cloud Console
  - Change "User type" from "Internal" to "External"
  - If the app is in "Testing" mode, add your email addresses as test users
  - If you want to use it with any Google account, you'll need to publish the app (requires verification for sensitive scopes)

### 2. Apple Calendar/iCloud Setup

For Apple Calendar destination, you need:

1. **App-Specific Password**:
   - Go to https://appleid.apple.com/account/manage
   - Sign in with your Apple ID
   - Under "Security" → "App-Specific Passwords", click "Generate Password"
   - Use this password for `apple_caldav_password` (not your regular Apple ID password)

2. **CalDAV Server URL**:
   - For iCloud, use: `https://caldav.icloud.com`
   - Some iCloud accounts may use a server-specific URL like `https://pXX-caldav.icloud.com` (where XX is a number)
   - If you get a 403 error, try checking your iCloud calendar settings to find the correct server URL

3. **Username**:
   - Use your full iCloud email address (e.g., `yourname@icloud.com`)

**Troubleshooting Apple Calendar**:
- **HTTP 403 Forbidden**: 
  - Verify you're using an app-specific password (not your regular Apple ID password)
  - Check that the CalDAV server URL is correct
  - Ensure your iCloud account has calendar access enabled
  - Try using just the username part (before @) if the full email doesn't work
- **Failed to discover principal**:
  - The tool will try multiple common paths automatically
  - Check the error message for which paths were tried
  - Verify your iCloud account is active and calendar is enabled

### 2. Configure the Tool

You can configure the tool using one of three methods (or a combination):

#### Option A: JSON Config File (Recommended)

Create a `config.json` file:

```json
{
  "work_token_path": "/path/to/work_token.json",
  "personal_token_path": "/path/to/personal_token.json",
  "sync_calendar_name": "Work Sync",
  "sync_calendar_color_id": "7",
  "google_credentials_path": "/path/to/credentials.json"
}
```

**Note**: The `google_credentials_path` should point to the JSON file downloaded from Google Cloud Console. The file should contain either an "installed" or "web" section with "client_id" and "client_secret" fields.

**Example for Apple Calendar destination:**
```json
{
  "work_token_path": "/path/to/work_token.json",
  "sync_calendar_name": "Work Sync",
  "sync_calendar_color_id": "7",
  "google_credentials_path": "/path/to/credentials.json",
  "destination_type": "apple",
  "apple_caldav_server_url": "https://caldav.icloud.com",
  "apple_caldav_username": "your-email@icloud.com",
  "apple_caldav_password": "app-specific-password"
}
```

**Note**: For Apple Calendar, you need to generate an app-specific password from iCloud:
1. Go to https://appleid.apple.com/account/manage
2. Sign in with your Apple ID
3. Under "Security" → "App-Specific Passwords", click "Generate Password"
4. Use this password for `apple_caldav_password`

#### Option B: Environment Variables

```bash
# For Google Calendar destination
export WORK_TOKEN_PATH="/path/to/work_token.json"
export PERSONAL_TOKEN_PATH="/path/to/personal_token.json"
export SYNC_CALENDAR_NAME="Work Sync"
export SYNC_CALENDAR_COLOR_ID="7"
export GOOGLE_CREDENTIALS_PATH="/path/to/credentials.json"
export DESTINATION_TYPE="google"

# For Apple Calendar destination
export WORK_TOKEN_PATH="/path/to/work_token.json"
export SYNC_CALENDAR_NAME="Work Sync"
export GOOGLE_CREDENTIALS_PATH="/path/to/credentials.json"
export DESTINATION_TYPE="apple"
export APPLE_CALDAV_SERVER_URL="https://caldav.icloud.com"
export APPLE_CALDAV_USERNAME="your-email@icloud.com"
export APPLE_CALDAV_PASSWORD="app-specific-password"
```

#### Option C: Command-Line Flags

```bash
# For Google Calendar destination
./calsync \
  --work-token-path /path/to/work_token.json \
  --personal-token-path /path/to/personal_token.json \
  --sync-calendar-name "Work Sync" \
  --sync-calendar-color-id "7" \
  --google-credentials-path /path/to/credentials.json \
  --destination-type google

# For Apple Calendar destination
./calsync \
  --work-token-path /path/to/work_token.json \
  --sync-calendar-name "Work Sync" \
  --google-credentials-path /path/to/credentials.json \
  --destination-type apple \
  --apple-caldav-server-url "https://caldav.icloud.com" \
  --apple-caldav-username "your-email@icloud.com" \
  --apple-caldav-password "app-specific-password"
```

### Configuration Precedence

Settings are loaded in the following order (highest to lowest priority):

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Config file** (`--config`)
4. **Defaults** (lowest priority)

**Security Note**: The `google_credentials_path` can be overridden by the `GOOGLE_CREDENTIALS_PATH` environment variable. This allows you to keep the credentials file path out of version-controlled config files, or use different credentials files for different environments.

## Usage

### First Run

On the first run, the tool will:

1. Prompt you to authorize your **work account** (read-only access)
2. Prompt you to authorize your **personal account** (full calendar access)
3. Create the "Work Sync" calendar in your personal account
4. Perform the initial sync

You'll need to visit the authorization URLs in your browser and paste the authorization codes back into the terminal.

### Subsequent Runs

After the first run, the tool uses stored refresh tokens and runs automatically without user interaction.

### Basic Usage

```bash
# Using a config file
./calsync --config config.json

# Using environment variables
./calsync

# Using command-line flags
./calsync --work-token-path /path/to/work.json --personal-token-path /path/to/personal.json ...

# Mix config file and environment variables (override credentials path)
GOOGLE_CREDENTIALS_PATH="/path/to/creds.json" ./calsync --config config.json
```

### Scheduled Execution (Cron)

For hourly syncs, add to your crontab:

```bash
# Edit crontab
crontab -e

# Add this line (runs every hour)
0 * * * * /path/to/calsync --config /path/to/config.json >> /var/log/calsync.log 2>&1
```

## Configuration Options

### Required Settings

- **`work_token_path`**: Path where the work account OAuth token will be stored (always required)
- **`google_credentials_path`**: Path to the Google OAuth credentials JSON file (downloaded from Google Cloud Console) (always required)
- **`personal_token_path`**: Path where the personal account OAuth token will be stored (required for Google Calendar destination only)
- **`destination_type`**: Destination calendar type - `"google"` or `"apple"` (default: `"google"`)

### Apple Calendar Settings (required when `destination_type` is `"apple"`)

- **`apple_caldav_server_url`**: CalDAV server URL (e.g., `"https://caldav.icloud.com"` for iCloud)
- **`apple_caldav_username`**: Your iCloud email address
- **`apple_caldav_password`**: App-specific password from iCloud (generate at https://appleid.apple.com/account/manage)

### Optional Settings

- **`sync_calendar_name`**: Name of the calendar to create/use (default: `"Work Sync"`)
- **`sync_calendar_color_id`**: Color ID for the calendar (default: `"7"` for Grape)

### Calendar Color IDs

Common color IDs:
- `1` - Lavender
- `2` - Sage
- `3` - Grape
- `4` - Flamingo
- `5` - Banana
- `6` - Tangerine
- `7` - Grape (default)
- `8` - Graphite
- `9` - Blueberry
- `10` - Basil
- `11` - Tomato

## Event Filtering Rules

The tool applies the following filters when syncing events:

1. **All-day events**: All all-day events are synced (including Out of Office)
2. **Timed events**: Only events between **6:00 AM** and **12:00 AM (midnight)** are synced
3. **Out of Office**: Timed OOF events are **skipped** (all-day OOF events are kept)
4. **Recurring events**: Recurring events are expanded into individual instances within the sync window
5. **RSVP status**: All events are synced regardless of RSVP status

### Sync Window

The tool syncs events within a **two-week rolling window**:
- Current week (Monday - Sunday)
- Next week (Monday - Sunday)

Events outside this window are automatically cleaned up.

## Event Data

When syncing events, the following data is copied:

- ✅ Summary (title)
- ✅ Description
- ✅ Location
- ✅ Start and end times
- ✅ Conference data (meeting links)

The following data is **not** copied:

- ❌ Attendees (guest list)
- ❌ Attachments
- ❌ Other metadata

## Troubleshooting

### Authentication Issues

If you get authentication errors:

1. Delete the token files (`work_token.json` and `personal_token.json`)
2. Re-run the tool to go through the OAuth flow again

### Missing Events

- Check that events fall within the sync window (current week + next week)
- Verify events are not timed OOF events (these are filtered out)
- Ensure events are between 6 AM and midnight

### Permission Errors

- Verify your OAuth credentials have the correct scopes:
  - Work account: `calendar.events.readonly`
  - Personal account: `calendar` (full access)

## Development

### Running Tests

```bash
make test
# or
go test ./...
```

### Building

```bash
make build
# or
go build -o calsync .
```

### Code Formatting

```bash
make fmt
# or
go fmt ./...
```

## License

[Add your license here]

## Contributing

[Add contribution guidelines here]

