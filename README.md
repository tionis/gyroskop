# Gyroskop Bot

[![Build and Push Docker Image](https://github.com/tionis/gyroskop/actions/workflows/docker-build.yml/badge.svg)](https://github.com/tionis/gyroskop/actions/workflows/docker-build.yml)
[![Test](https://github.com/tionis/gyroskop/actions/workflows/test.yml/badge.svg)](https://github.com/tionis/gyroskop/actions/workflows/test.yml)

A Telegram bot for coordinating food orders in groups with time-based ordering.

## Features

- Time-based ordering with configurable deadlines
- Custom food types with arbitrary options (Pizza, Gyros, Burger, etc.)
- Fuzzy matching for natural language orders
- Inline button support for quick ordering
- PostgreSQL storage
- Group-only operation
- Automatic summary at deadline

## Usage

### Command Format

Commands use comma-separated syntax:
```
/gyroskop [time], [name], option1, option2, ...
```

All parameters are optional. Time and name must be separated by commas.

### Creating Orders

```
/gyroskop                                    # Default: Gyros for 15min
/gyroskop 30min                              # Custom duration
/gyroskop 17:00                              # Until specific time (Berlin timezone)
/gyroskop Pizza, Margherita, Salami          # Custom food type
/gyroskop 17:00, Burger, Beef, Chicken       # Full customization
/gyroskop 10min, Döner, Fleisch, Vegetarisch # 10 minutes with custom options
```

Time formats supported:
- Duration: `30min`, `1h`, `2h`, `45min`
- Absolute time: `17:00`, `12:30` (HH:MM in Berlin timezone)
- Default: 15 minutes if not specified

### Placing Orders

Place orders via text with fuzzy matching:
```
2 fleisch              # Orders 2x Fleisch
3 veggie               # Orders 3x Vegetarisch
2 meat, 3 veggie       # Multiple items in one line
2 fl                   # Prefix matching works
```

Or use inline buttons for quantities 1-5.

### Other Commands

```
/status                           # Show current orders
/ende                             # Close order early (creator only)
/stornieren                       # Cancel your order
/gyroskop (as reply)              # Reopen or modify existing order
```

### Reply-Based Actions

- `/ende` as reply to gyroskop message: Close that specific order
- `/gyroskop [args]` as reply: Reopen closed order or change options

## Installation

### Prerequisites

1. **Telegram Bot Token**: Create a bot via [@BotFather](https://t.me/botfather) on Telegram
2. **PostgreSQL Database**: Version 12 or higher

### Docker

```bash
docker pull ghcr.io/tionis/gyroskop:latest

docker run -d \
  --name gyroskop-bot \
  -e TELEGRAM_BOT_TOKEN=your_token \
  -e POSTGRES_HOST=your_postgres_host \
  -e POSTGRES_PORT=5432 \
  -e POSTGRES_DB=gyroskop \
  -e POSTGRES_USER=gyroskop \
  -e POSTGRES_PASSWORD=your_password \
  -e POSTGRES_SSLMODE=require \
  ghcr.io/tionis/gyroskop:latest
```

### Docker Compose

```bash
# Copy example environment file
cp .env.example .env

# Edit .env with your configuration
# - Get bot token from @BotFather
# - Configure PostgreSQL settings

# Start bot and database
docker-compose up -d
```

### From Source

```bash
git clone https://github.com/tionis/gyroskop.git
cd gyroskop

# Set environment variables
export TELEGRAM_BOT_TOKEN=your_token
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=gyroskop
export POSTGRES_USER=gyroskop
export POSTGRES_PASSWORD=your_password
export POSTGRES_SSLMODE=disable

# Build and run
go build -o gyroskop-bot ./main.go
./gyroskop-bot
```

## Configuration

Environment variables:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `TELEGRAM_BOT_TOKEN` | Yes | - | Telegram bot token |
| `POSTGRES_HOST` | No | `localhost` | PostgreSQL host |
| `POSTGRES_PORT` | No | `5432` | PostgreSQL port |
| `POSTGRES_DB` | No | `gyroskop` | Database name |
| `POSTGRES_USER` | No | `gyroskop` | Database user |
| `POSTGRES_PASSWORD` | No | `gyroskop` | Database password |
| `POSTGRES_SSLMODE` | No | `disable` | SSL mode (disable/require/verify-ca/verify-full) |

## Development

Requirements:
- Go 1.21+
- PostgreSQL 12+

### Database Setup

Option 1 - Using provided script:
```bash
sudo -u postgres psql -f setup.sql
```

Option 2 - Manual setup:
```bash
sudo -u postgres createdb gyroskop
sudo -u postgres createuser gyroskop
sudo -u postgres psql -c "ALTER USER gyroskop PASSWORD 'your_password';"
sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE gyroskop TO gyroskop;"
```

Tables are created automatically by the bot on first run.

### Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test ./... -v

# Run specific package tests
go test ./internal/bot/... -v
```

### Local Development

```bash
# Set environment variables
export TELEGRAM_BOT_TOKEN=your_token
export POSTGRES_HOST=localhost
export POSTGRES_PORT=5432
export POSTGRES_DB=gyroskop
export POSTGRES_USER=gyroskop
export POSTGRES_PASSWORD=your_password
export POSTGRES_SSLMODE=disable

# Run the bot
go run main.go
```

## License

See LICENSE file.