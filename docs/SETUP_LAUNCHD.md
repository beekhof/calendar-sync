# Setting Up Hourly Sync with launchd on macOS

`launchd` is the recommended way to run scheduled tasks on macOS. It's more reliable than cron and better integrated with the macOS system.

## Automated Setup (Recommended)

Use the provided setup script for the easiest installation:

```bash
# Run the setup script
./setup-launchd.sh
```

The script will:
- ✅ Check if `calsync` is installed
- ✅ Copy your `config.json` to `~/.config/calsync/config.json`
- ✅ Create log directories
- ✅ Generate and install the launchd plist with correct paths
- ✅ Load the service
- ✅ Run a test sync to verify everything works

## Manual Setup

If you prefer to set up manually:

1. **Edit the plist file** (`com.beekhof.calsync.plist`) to match your paths:
   - Update the path to `calsync` binary (or use the full path where you installed it)
   - Update the path to your `config.json` file
   - Update log file paths if desired

2. **Install the launchd service**:
   ```bash
   # Copy the plist to LaunchAgents directory
   cp com.beekhof.calsync.plist ~/Library/LaunchAgents/
   
   # Load the service
   launchctl load ~/Library/LaunchAgents/com.beekhof.calsync.plist
   ```

3. **Verify it's running**:
   ```bash
   # Check if the service is loaded
   launchctl list | grep calsync
   
   # View recent logs
   tail -f ~/Library/Logs/calsync/stderr.log
   ```

## Managing the Service

### Start/Stop
```bash
# Start the service
launchctl start com.beekhof.calsync

# Stop the service
launchctl stop com.beekhof.calsync
```

### Unload/Reload
```bash
# Unload (stop and remove from launchd)
launchctl unload ~/Library/LaunchAgents/com.beekhof.calsync.plist

# Reload after making changes to the plist
launchctl unload ~/Library/LaunchAgents/com.beekhof.calsync.plist
launchctl load ~/Library/LaunchAgents/com.beekhof.calsync.plist
```

### View Logs
```bash
# View error log (most useful)
tail -f ~/Library/Logs/calsync/stderr.log

# View output log
tail -f ~/Library/Logs/calsync/stdout.log

# View last 50 lines
tail -50 ~/Library/Logs/calsync/stderr.log
```

## Plist Configuration Options

- **`StartInterval`**: Run every N seconds (3600 = 1 hour)
- **`RunAtLoad`**: Run immediately when loaded (useful for testing)
- **`StandardOutPath`**: Where to write stdout
- **`StandardErrorPath`**: Where to write stderr
- **`EnvironmentVariables`**: Set PATH and other environment variables

## Alternative: Using Cron

If you prefer cron (though launchd is recommended on macOS):

```bash
# Edit crontab
crontab -e

# Add this line (runs every hour at minute 0)
0 * * * * /Users/beekhof/go/bin/calsync --config /Users/beekhof/repos/calendar-sync/config.json >> /Users/beekhof/Library/Logs/calsync/cron.log 2>&1
```

**Note**: Cron on macOS may not run when the computer is asleep. launchd handles this better.

## Troubleshooting

### Service not running
```bash
# Check if it's loaded
launchctl list | grep calsync

# Check for errors
launchctl error com.beekhof.calsync

# View system logs
log show --predicate 'process == "calsync"' --last 1h
```

### Permission issues
- Make sure the binary is executable: `chmod +x /path/to/calsync`
- Make sure log directories exist: `mkdir -p ~/Library/Logs/calsync`

### PATH issues
- The plist includes a PATH in `EnvironmentVariables`
- Adjust it to include the directory where `calsync` is installed
- Or use the full path to the binary in `ProgramArguments`

