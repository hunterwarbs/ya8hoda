# Ya8hoda - Telegram RAG Bot

Ya8hoda is a Telegram bot with Retrieval-Augmented Generation (RAG) capabilities using Milvus as a vector database.

## Features

- Interact with users via Telegram
- Store and retrieve documents using vector similarity search
- Generate images using OpenRouter API
- Role-based access control for different user types

## Requirements

- Go 1.21 or higher
- Docker and Docker Compose for running Milvus

## Tech Stack

- [go-telegram/bot](https://github.com/go-telegram/bot) - Telegram Bot API Go framework
- [Milvus](https://milvus.io/) - Vector database for RAG capabilities
- [OpenRouter](https://openrouter.ai/) - API for LLM and image generation

## Configuration

Create a `.env` file in the project root with the following environment variables:

```
# Required settings
TG_BOT_TOKEN=your_telegram_bot_token
OPENROUTER_API_KEY=your_openrouter_api_key
OPENROUTER_MODEL=qwen/qwq-32b


# Optional settings
MILVUS_HOST=milvus
MILVUS_PORT=19530
LOG_LEVEL=info
ADMIN_USER_IDS=123456789,987654321
ALLOWED_USER_IDS=123456789,987654321
```

## Running the Bot

### Using Docker Compose (recommended)

```bash
docker-compose up -d
```

This will start the bot and Milvus server in containers.

### Running Locally

First, start the Milvus server:

```bash
docker-compose up -d milvus
```

Then run the bot:

```bash
go run cmd/main.go
```

## Development

The main bot code is in `cmd/bot/main.go`. The root `cmd/main.go` serves as a wrapper that can either run the bot directly or execute the compiled binary.

## License

MIT 