# Calendar Sync Tool

A one-way synchronization tool that syncs events from a work Google Calendar to a personal calendar (Google Calendar or Apple Calendar/iCloud). This tool is designed to work around admin restrictions by using API access instead of the standard calendar sharing UI.

## License

This project is licensed under the Apache License 2.0. See the [LICENSE](../LICENSE) file for details.

## Overview

The Calendar Sync Tool creates a read-only "Work Sync" calendar in your personal calendar account (Google Calendar or Apple Calendar/iCloud), populated with filtered events from your work Google Calendar. The work calendar is the "source of truth" - any manual changes made to the synced calendar will be overwritten on the next sync.

### ⚠️ Important Warning

**Before your first sync, please be aware:**

- **The work calendar is the source of truth** - the tool will DELETE any events in the destination calendar that are not present in the work calendar
- **Manually created events will be deleted** - any events you manually add to the destination calendar will be removed on the next sync
- **Use a dedicated calendar** - only use this tool with a calendar that you don't manually edit. The tool will automatically create a "Work Sync" calendar for you, or you can specify a different name in the config
- **Stale events are removed** - events that were previously synced but no longer exist in the work calendar will be deleted

**This tool is designed for one-way synchronization only. Do not manually edit the destination calendar.**

### Key Features

- **One-way sync**: Work calendar → Personal calendar (Google or Apple)
- **Multiple destination support**: Sync to Google Calendar or Apple Calendar/iCloud
- **Automatic filtering**: Only syncs relevant events (6 AM - midnight, excludes timed OOF events)
- **Recurring event expansion**: Expands recurring events into individual instances
- **Configurable sync window**: Customize how many weeks forward and backward to sync (default: 2 weeks forward, 0 weeks past)
- **Automatic cleanup**: Removes stale events outside the sync window
- **Flexible configuration**: Config file, environment variables, or command-line flags

## Installation

### Prerequisites

- Go 1.21 or later
- Google OAuth 2.0 credentials (Client ID and Client Secret)

### Install using go install

```bash
# Install from GitHub
go install github.com/beekhof/calendar-sync/cmd/calsync@latest

# Or install from a local directory
cd /path/to/calendar-sync
go install ./cmd/calsync

# The binary will be installed to $GOPATH/bin or $GOBIN (default: ~/go/bin)
# Make sure this directory is in your PATH
```

### Build from source

```bash
# Clone the repository
git clone <repository-url>
cd calendar-sync

# Build the binary
make build
# or
go build -o calsync ./cmd/calsync
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

#### Option A: JSON Config File (Required)

Create a `config.json` file. The `destinations` array is required and must contain at least one destination:

```json
{
  "work_token_path": "/path/to/work_token.json",
  "google_credentials_path": "/path/to/credentials.json",
  "sync_window_weeks": 2,
  "sync_window_weeks_past": 0,
  "destinations": [
    {
      "name": "Personal Google",
      "type": "google",
      "token_path": "/path/to/personal_token.json",
      "calendar_name": "Work Sync",
      "calendar_color_id": "7"
    }
  ]
}
```

**Example with multiple destinations:**
```json
{
  "work_token_path": "/path/to/work_token.json",
  "google_credentials_path": "/path/to/credentials.json",
  "sync_window_weeks": 2,
  "sync_window_weeks_past": 0,
  "destinations": [
    {
      "name": "Personal Google",
      "type": "google",
      "token_path": "/path/to/personal_token.json",
      "calendar_name": "Work Sync",
      "calendar_color_id": "7"
    },
    {
      "name": "iCloud",
      "type": "apple",
      "server_url": "https://caldav.icloud.com",
      "username": "your-email@icloud.com",
      "password": "app-specific-password",
      "calendar_name": "Work",
      "calendar_color_id": "1"
    }
  ]
}
```

**Notes**:
- The `google_credentials_path` should point to the JSON file downloaded from Google Cloud Console. The file should contain either an "installed" or "web" section with "client_id" and "client_secret" fields.
- For Apple Calendar destinations, you need to generate an app-specific password from iCloud:
  1. Go to https://appleid.apple.com/account/manage
  2. Sign in with your Apple ID
  3. Under "Security" → "App-Specific Passwords", click "Generate Password"
  4. Use this password for the `password` field in the Apple destination

#### Option B: Environment Variables

Some settings can be provided via environment variables, but destination configuration must be in the config file:

```bash
export WORK_TOKEN_PATH="/path/to/work_token.json"
export GOOGLE_CREDENTIALS_PATH="/path/to/credentials.json"
export SYNC_WINDOW_WEEKS=2
export SYNC_WINDOW_WEEKS_PAST=0
```

**Note**: Destination configuration (type, token_path, server_url, etc.) must be specified in the config file's `destinations` array. Environment variables cannot override destination settings.

#### Option C: Command-Line Flags

Only a few settings can be overridden via command-line flags:

```bash
./calsync \
  --config /path/to/config.json \
  --work-token-path /path/to/work_token.json \
  --google-credentials-path /path/to/credentials.json
```

**Note**: Destination configuration must be specified in the config file. Command-line flags cannot override destination settings.

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
# Using a config file (required)
./calsync --config config.json

# Override some settings via environment variables
GOOGLE_CREDENTIALS_PATH="/path/to/creds.json" ./calsync --config config.json

# Override some settings via command-line flags
./calsync --config config.json --work-token-path /path/to/work_token.json
```

### Scheduled Execution

The tool automatically detects when running in non-interactive mode (e.g., from launchd or cron). In this mode:
- **Confirmation prompts are skipped** - the tool will not wait for user input
- **If manually created events are found**, the sync will fail with a clear error message
- This prevents the sync from hanging in automated environments

**Important**: Before setting up automated syncs, ensure your destination calendar only contains synced events (events with `workEventId`). Manually created events should be removed or moved to a different calendar.

#### macOS: Using launchd (Recommended)

`launchd` is the native macOS scheduler and is recommended over cron. See [SETUP_LAUNCHD.md](SETUP_LAUNCHD.md) for detailed instructions.

**Easiest setup** - use the automated script:
```bash
./setup-launchd.sh
```

The script will handle everything automatically, including copying your config to the standard location (`~/.config/calsync/config.json`).

**Manual setup**:
```bash
# 1. Edit com.beekhof.calsync.plist to match your paths
# 2. Install the service
cp com.beekhof.calsync.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.beekhof.calsync.plist

# 3. Verify it's running
launchctl list | grep calsync
tail -f ~/Library/Logs/calsync/stderr.log
```

#### Alternative: Using Cron

For hourly syncs, add to your crontab:

```bash
# Edit crontab
crontab -e

# Add this line (runs every hour at minute 0)
0 * * * * /path/to/calsync --config /path/to/config.json >> ~/Library/Logs/calsync/cron.log 2>&1
```

**Note**: Cron on macOS may not run when the computer is asleep. launchd handles this better.

## Configuration Options

### Required Settings

- **`work_token_path`**: Path where the work account OAuth token will be stored (always required)
- **`google_credentials_path`**: Path to the Google OAuth credentials JSON file (downloaded from Google Cloud Console) (always required)
- **`destinations`**: Array of destination configurations (required, must contain at least one destination)

### Destination Configuration

Each destination in the `destinations` array can have the following fields:

**Common fields (all destinations)**:
- **`name`**: Optional name for logging (defaults to "Destination N")
- **`type`**: Required - `"google"` or `"apple"`
- **`calendar_name`**: Optional - Name of the calendar to create/use (default: `"Work Sync"`)
- **`calendar_color_id`**: Optional - Color ID for the calendar (default: `"7"`)

**Google Calendar destination fields**:
- **`token_path`**: Required - Path where the personal account OAuth token will be stored

**Apple Calendar destination fields**:
- **`server_url`**: Required - CalDAV server URL (e.g., `"https://caldav.icloud.com"` for iCloud)
- **`username`**: Required - Your iCloud email address
- **`password`**: Required - App-specific password from iCloud (generate at https://appleid.apple.com/account/manage)

### Optional Settings

- **`sync_window_weeks`**: Number of weeks to sync forward from start of current week (default: `2`)
- **`sync_window_weeks_past`**: Number of weeks to sync backward from start of current week (default: `0`)

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

The tool syncs events within a configurable rolling window starting from the current week (Monday). By default, it syncs:
- **Forward**: 2 weeks (current week + next week)
- **Backward**: 0 weeks (no past events)

You can customize the sync window using:
- **Config file**: `"sync_window_weeks": 3, "sync_window_weeks_past": 1`
- **Environment variables**: `SYNC_WINDOW_WEEKS=3 SYNC_WINDOW_WEEKS_PAST=1`

**Examples**:
- `sync_window_weeks: 2, sync_window_weeks_past: 0` (default): Syncs current week + next week
- `sync_window_weeks: 4, sync_window_weeks_past: 1`: Syncs last week + current week + next 3 weeks (5 weeks total)
- `sync_window_weeks: 1, sync_window_weeks_past: 2`: Syncs 2 weeks ago + last week + current week (3 weeks total)

Events outside the configured window are automatically cleaned up.

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

- Check that events fall within the configured sync window (default: current week + next week)
- Verify events are not timed OOF events (these are filtered out)
- Ensure events are between 6 AM and midnight
- If you need past events, set `sync_window_weeks_past` to a value greater than 0

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

