# Calendar Sync Tool

A one-way synchronization tool that syncs events from a work Google Calendar to a personal Google Calendar. This tool is designed to work around admin restrictions by using API access instead of the standard calendar sharing UI.

## Overview

The Calendar Sync Tool creates a read-only "Work Sync" calendar in your personal Google account, populated with filtered events from your work calendar. The work calendar is the "source of truth" - any manual changes made to the synced calendar will be overwritten on the next sync.

### Key Features

- **One-way sync**: Work calendar → Personal calendar
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
4. Go to "Credentials" → "Create Credentials" → "OAuth 2.0 Client ID"
5. Choose "Desktop app" as the application type
6. Save your **Client ID** and **Client Secret**

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
  "google_client_id": "your-client-id-here",
  "google_client_secret": "your-client-secret-here"
}
```

**Note**: For security, you can omit `google_client_id` and `google_client_secret` from the config file and provide them via environment variables instead.

#### Option B: Environment Variables

```bash
export WORK_TOKEN_PATH="/path/to/work_token.json"
export PERSONAL_TOKEN_PATH="/path/to/personal_token.json"
export SYNC_CALENDAR_NAME="Work Sync"
export SYNC_CALENDAR_COLOR_ID="7"
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-client-secret"
```

#### Option C: Command-Line Flags

```bash
./calsync \
  --work-token-path /path/to/work_token.json \
  --personal-token-path /path/to/personal_token.json \
  --sync-calendar-name "Work Sync" \
  --sync-calendar-color-id "7" \
  --google-client-id "your-client-id" \
  --google-client-secret "your-client-secret"
```

### Configuration Precedence

Settings are loaded in the following order (highest to lowest priority):

1. **Command-line flags** (highest priority)
2. **Environment variables**
3. **Config file** (`--config`)
4. **Defaults** (lowest priority)

**Security Note**: Secrets (`google_client_id`, `google_client_secret`) can be overridden by environment variables even if specified in the config file. This allows you to keep secrets out of version-controlled config files.

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

# Mix config file and environment variables (secrets from env)
GOOGLE_CLIENT_ID="id" GOOGLE_CLIENT_SECRET="secret" ./calsync --config config.json
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

- **`work_token_path`**: Path where the work account OAuth token will be stored
- **`personal_token_path`**: Path where the personal account OAuth token will be stored
- **`google_client_id`**: Google OAuth 2.0 Client ID
- **`google_client_secret`**: Google OAuth 2.0 Client Secret

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

