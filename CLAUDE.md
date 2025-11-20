# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A Go application that monitors Telegram groups for member leave events and sends notifications to designated groups. Built using the [gotd/td](https://github.com/gotd/td) MTProto API client.

## Development Commands

### Running the Application

```bash
# Run locally (requires .env file)
go run main.go

# Run with Docker Compose (development)
docker compose -f compose.dev.yml up --build

# Run with Docker Compose (production)
docker compose up -d
```

### Building

```bash
# Build binary
go build -o go-export-telegram-group-members .

# Build with CGO disabled (for Alpine/Docker)
CGO_ENABLED=0 go build -o server .

# Build Docker image
docker build -t go-export-telegram-group-members .
```

### Dependency Management

```bash
# Download dependencies
go mod download

# Tidy up go.mod and go.sum
go mod tidy

# Verify dependencies
go mod verify
```

## Configuration

The application requires a `.env` file (see `.env.example`):

- `TG_PHONE`: Phone number in international format (e.g., +4123456789)
- `APP_ID`: Telegram API ID from https://my.telegram.org/
- `APP_HASH`: Telegram API hash from https://my.telegram.org/
- `REDIS_URL`: Redis connection URL for session storage
- `MONITOR_GROUPS`: Comma-separated list of channel IDs to monitor for leave events
- `NOTIFICATION_GROUPS`: Comma-separated list of channel IDs where notifications are sent

### Group ID Formats

The application handles Telegram group IDs in multiple formats:
- Plain channel ID (e.g., 1234567890)
- Bot API format for supergroups/channels (e.g., -1001234567890)

Internally, channel IDs are extracted and the full ID format is calculated as `-1000000000000 - channelID`.

## Architecture

### Core Components

**Session Management**: Sessions are stored in Redis using the `gotd/contrib/redis` adapter. Local peer database and update state are stored in the `session/phone-{digits}` directory using Pebble and BoltDB.

**Update Handling**: The application uses a three-layer update handling system:
1. `updates.Recovery` - Persistent storage for qts/pts state recovery
2. `storage.UpdateHook` - Fills peer storage before dispatching
3. `tg.UpdateDispatcher` - Routes updates to registered handlers

**Peer Storage**: Uses Pebble database for caching peer information and access hashes. This enables short update handling and reduces API calls. The application fills the peer storage from dialogs on startup.

**Rate Limiting**: Two-layer rate limiting to avoid FLOOD_WAIT errors:
1. `floodwait.Waiter` - Automatically retries on FLOOD_WAIT with logging
2. `ratelimit.New` - General rate limit (5 requests per 100ms)

### Message Notification Flow

When a user leaves a monitored group:
1. `OnNewChannelMessage` handler receives `UpdateNewChannelMessage`
2. Filter for `MessageService` with `MessageActionChatDeleteUser` action
3. Extract channel ID and check against `monitorGroups` map
4. Call `sendLeaveNotification` with user and group entities
5. Send formatted notification to all `notificationGroups` using `message.Sender` with resolver

The resolver automatically handles access hash lookups from peer storage.

### Logging

Structured logging using `zap`:
- JSON logs written to `session/phone-{digits}/log.jsonl`
- Rotation: max 1MB, 3 backups, 7 days retention
- Debug level logging for observability

### Authentication

First-run authentication flow:
1. Checks for existing session in Redis
2. If no session, prompts for phone code via terminal
3. May prompt for 2FA password if enabled
4. Session persisted to Redis for subsequent runs

## Key Implementation Details

### Channel ID Conversion

The codebase currently has commented-out conversion logic (main.go:104-112) for handling bot API format IDs. The current implementation attempts to send notifications directly using the channel ID from the update.

### Random Access Hash

The `randomID()` function (main.go:53-58) generates a random int64 for access hashes, though this is not the correct approach for Telegram API. Access hashes should be retrieved from peer storage via the resolver, which the `message.Sender` with resolver handles automatically.

### Service Message Types

The dispatcher handles various `MessageAction` types:
- `MessageActionChatDeleteUser` - User left or was removed
- `MessageActionChatAddUser` - User(s) added to group
- Other actions logged for debugging

## Docker Deployment

The application runs as a non-root user (`appuser`, UID 10001) in an Alpine container. The `/app/session` directory is exposed as a volume for persistent storage of peer data and update state.

Session data in Redis and the session volume must be preserved for the application to maintain authentication and avoid re-login.
