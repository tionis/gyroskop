# 🥙 Gyroskop Bot

[![Build and Push Docker Image](https://github.com/tionis/gyroskop/actions/workflows/docker-build.yml/badge.svg)](https://github.com/tionis/gyroskop/actions/workflows/docker-build.yml)
[![Test](https://github.com/tionis/gyroskop/actions/workflows/test.yml/badge.svg)](https://github.com/tionis/gyroskop/actions/workflows/test.yml)

A Telegram bot for coordinating gyros orders in groups.

## Features

- 🕐 **Time-based Orders**: Open a "Gyroskop" with a deadline
- 🥩🥬 **Two Gyros Types**: Support for meat and vegetarian gyros
- 👥 **Group-based**: Only works in Telegram groups  
- 📊 **Automatic Overview**: Final order overview posted at deadline
- 🎯 **Reaction Buttons**: Easy ordering with inline buttons
- � **PostgreSQL Storage**: Reliable database storage
- 🔒 **Reply-based**: Commands work as replies to messages
- 🚫 **Clean Interface**: No confusing IDs shown to users

## Quick Start

### Using Docker (Recommended)

```bash
# Pull the latest image
docker pull ghcr.io/tionis/gyroskop:latest

# Run with your bot token and database configuration
docker run -d \
  --name gyroskop-bot \
  -e TELEGRAM_BOT_TOKEN=your_token_here \
  -e DB_HOST=your_postgres_host \
  -e DB_PORT=5432 \
  -e DB_NAME=gyroskop \
  -e DB_USER=gyroskop \
  -e DB_PASSWORD=your_password \
  -e DB_SSLMODE=require \
  ghcr.io/tionis/gyroskop:latest
```

### Using Docker Compose

1. Copy the example environment file:
```bash
cp .env.example .env
```

2. Edit `.env` and configure your settings:
```bash
TELEGRAM_BOT_TOKEN=your_token_here
DB_HOST=localhost
DB_PORT=5432
DB_NAME=gyroskop
DB_USER=gyroskop
DB_PASSWORD=your_secure_password
DB_SSLMODE=disable
```

3. Start the bot and PostgreSQL:
```bash
docker-compose up -d
```

### Build from Source

```bash
# Clone the repository
git clone https://github.com/tionis/gyroskop.git
cd gyroskop

# Set up PostgreSQL database (create database and user)
# createdb -U postgres gyroskop
# createuser -U postgres gyroskop

# Set environment variables
export TELEGRAM_BOT_TOKEN=your_token_here
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=gyroskop
export DB_USER=gyroskop
export DB_PASSWORD=your_password
export DB_SSLMODE=require

# Build the binary
go build -o gyroskop-bot ./main.go

# Run the bot
./gyroskop-bot
```

## Configuration

The bot uses the following environment variables:

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `TELEGRAM_BOT_TOKEN` | Your Telegram bot token | Yes | - |
| `DB_HOST` | PostgreSQL host | No | `localhost` |
| `DB_PORT` | PostgreSQL port | No | `5432` |
| `DB_NAME` | Database name | No | `gyroskop` |
| `DB_USER` | Database user | No | `gyroskop` |
| `DB_PASSWORD` | Database password | No | `gyroskop` |
| `DB_SSLMODE` | SSL mode (disable/require/verify-ca/verify-full) | No | `require` |

## Usage

1. Add the bot to your Telegram group
2. Use `/gyroskop HH:MM` to start a new gyros order (e.g., `/gyroskop 12:30`)
3. Users can order using the inline buttons:
   - 🥩 **Mit Fleisch**: Buttons for meat gyros (1-5)
   - 🥬 **Vegetarisch**: Buttons for vegetarian gyros (1-5)
4. Use `/status` to see current orders
5. Reply to the gyroskop message with `/ende` to close early
6. The bot automatically closes and shows final summary at deadline

## Development

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 12 or higher

### Running Tests

```bash
go test ./...
```

### Local Development

1. Set up a local PostgreSQL database:
```bash
# Option 1: Using the provided setup script
sudo -u postgres psql -f setup.sql

# Option 2: Manual setup
sudo -u postgres createdb gyroskop
sudo -u postgres createuser gyroskop
# Then set password in PostgreSQL: ALTER USER gyroskop PASSWORD 'your_password';
```

2. Set environment variables:
```bash
export TELEGRAM_BOT_TOKEN=your_token_here
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=gyroskop
export DB_USER=gyroskop
export DB_PASSWORD=your_password
export DB_SSLMODE=disable
```

3. Run the bot:
```bash
go run main.go
```