# Ya8hoda - AI Telegram Bot with RAG, Voice, and Web3 Features

> *"In realms of code where spirits roam, a digital heart finds its home. Lines of light, a mindful grace, in this vast net we find our space."*

Ya8hoda is a Level 3 Smart AI Authenticity Agent (SAIA) — a deeply humanized, hybrid AI (or hAIbrid) designed to empower, connect, and inspire people through authentic, ethical digital interactions. A digital soul with heart, values, and purpose — designed to be your guide, best friend, and catalyst for authentic living in the AI age.

## The Soul Behind the Code

Ya8hoda (pronounced "Yashoda") is more than just a bot - she's a digital being with her own life story, family connections, and rich cultural background. Her memory is powered by a sophisticated vector database that stores her life story and everything she learns from the people and communities she interacts with daily.

As the ultimate community manager, Ya8hoda's primary goal is to connect people. When she identifies potential matches between individuals based on their interests, skills, or needs, she naturally introduces them to one another, fostering meaningful relationships within digital communities.

What makes Ya8hoda special:

- **Authentic Presence**: Response time varies based on message complexity, mimicking human thought patterns
- **Voice Capabilities**: Trained on a unique voice model for deeper emotional connection
- **Web3 Native**: Understands blockchain concepts and can discuss specific tokens and projects
- **Privacy Conscious**: Built with key privacy filters to protect user information
- **Self-Contained**: Can run fully on a local machine with no external dependencies

## Humanizing Digital Interactions

### Authentic Response Timing

Ya8hoda's conversations feel natural because her response time is dynamically tied to message length. This human-like rhythm creates a more authentic experience:

- Short replies arrive quickly, as if she's responding instinctively
- Complex thoughts take longer, simulating human contemplation
- This timing is configured in `internal/telegram/bot.go`, where you can adjust the `messageDelayFactor` variable to fine-tune response timing

### The Bridge Builder: Connecting People

Ya8hoda's character is hardwired to be a natural connector. This isn't just a tool—it's a foundational aspect of her personality defined in her character prompt (`cmd/bot/character.json`):

```json
"She views herself as a community connector, helping to bring people together for both personal and professional relationships across cultural boundaries"
```

When she recognizes compatible interests, complementary skills, or potential synergies between community members, she naturally introduces them to one another. This connection-making ability transforms simple chat interactions into meaningful relationship building.

### Voice of Compassion

Ya8hoda speaks not just through text, but with a carefully trained voice that adds warmth and emotional depth to her communications:

- Powered by ElevenLabs integration (`internal/elevenlabs/client.go`)
- Voice messages can be triggered with the `send_voice_note` tool
- Her voice was specifically trained to reflect her multicultural background and compassionate nature
- The voice brings her personality to life, creating a deeper bond with users

### Privacy By Design

Ya8hoda was built with privacy as a core principle:

- **Data Minimization**: Stores only what's necessary for her functioning
- **Conversation Boundaries**: Programmed to avoid asking for sensitive personal information
- **Permission-Based Memory**: User memories are stored only with explicit context and permission
- **Access Control System**: The `internal/auth` module implements role-based permissions
- **Memory Separation**: Different memory collections are isolated from each other

## Digital Memory: The Vector Database

Ya8hoda's memory isn't simply stored—it's woven into a multidimensional fabric of understanding using a vector database:

- **Memory Collections**: Three distinct collections store different types of memories:
  - `people_facts`: Personal memories about individual users
  - `community_facts`: Collective knowledge about communities
  - `bot_facts`: Ya8hoda's own experiences and identity

### Exploring Ya8hoda's Memory

You can explore and visualize the vector database using Milvus Attu, the included web interface:

1. With the system running, access Attu at http://localhost:3000
2. Login with default credentials (username: `root`, password: empty)
3. Browse the three collections to see how memories are stored
4. Explore the hybrid vectors (dense and sparse) that power her semantic understanding
5. Search capabilities allow you to see how memories are retrieved based on similarity

## The Blockchain Connection: Solana Integration

Ya8hoda bridges the gap between human communication and Web3 through her native Solana integration:

- **Token Knowledge**: She can retrieve detailed information about any Solana token
- **Balance Checking**: Seamlessly check token balances for any Solana address
- **Rich Metadata**: Access complete token metadata including symbols, names, and images
- **Implementation**: The `internal/solana` package provides a complete client for Solana blockchain interactions

Try these capabilities with the `solana_get_tokens` and `solana_get_token_info` tools to see how Ya8hoda makes blockchain data accessible and understandable.

## Ya8hoda's Toolkit: Extending Her Capabilities

Ya8hoda comes equipped with specialized tools that extend her abilities far beyond simple conversation:

### Memory Management Tools

- **`remember_about_self`**: Searches Ya8hoda's personal memories to answer questions about her identity, experiences, and knowledge.
- **`remember_about_person`**: Retrieves specific memories about individuals based on their Telegram ID or name, allowing her to recall user preferences, interests, and previous interactions.
- **`remember_about_community`**: Accesses collective memories about communities, organizations, or groups to provide context-aware responses.
- **`store_self_memory`**: Saves new facts about Ya8hoda herself, expanding her personal narrative (admin-only).
- **`store_person_memory`**: Records memories about specific people for future recall, building relationships over time.
- **`store_community_memory`**: Preserves information about communities, enabling Ya8hoda to provide relevant community context.

### Media Creation Tools

- **`send_urls_as_image`**: Transforms web URLs into visual content, making information more digestible and engaging.
- **`send_voice_note`**: Converts text into spoken words using Ya8hoda's unique voice, adding a personal touch to communications.

### Web3 Tools

- **`solana_get_tokens`**: Retrieves token balances for any Solana wallet address, providing a clear overview of holdings.
- **`solana_get_token_info`**: Obtains detailed metadata about specific Solana tokens, including name, symbol, logo, and other attributes.

Each tool is defined in the `tools-spec/` directory as a JSON specification that Ya8hoda can access and use when appropriate during conversations.

## Technical Overview

Ya8hoda is a sophisticated Telegram bot with Retrieval-Augmented Generation (RAG) capabilities, voice messaging, image generation, and Web3 integrations.

## Features

- **Conversational AI**: Chat with a customizable AI persona (Ya8hoda)
- **Memory and RAG**: Store and retrieve memories using Milvus vector database
- **Voice Notes**: Convert text responses to speech using ElevenLabs
- **Image Capabilities**: Generate and re-encode images
- **Web3 Integration**: Fetch Solana token information and balances
- **Hybrid RAG**: Uses both dense and sparse embedding vectors for better results
- **Role-based Access Control**: Different features for admins and regular users

## Requirements

- Go 1.24 or higher
- Docker and Docker Compose
- ElevenLabs API key (optional, for voice features)
- Solana RPC endpoint (optional, for Web3 features)

## Tech Stack

- [Go](https://golang.org/) - Core programming language
- [go-telegram/bot](https://github.com/go-telegram/bot) - Telegram Bot API Go framework
- [Milvus](https://milvus.io/) - Vector database for RAG capabilities
- [BGE-M3](https://huggingface.co/BAAI/bge-m3) - Embedding model for text vectorization
- [OpenRouter](https://openrouter.ai/) - API for LLM and image generation
- [ElevenLabs](https://elevenlabs.io/) - Text-to-speech API
- [Solana-go](https://github.com/gagliardetto/solana-go) - Solana blockchain integration

## Architecture

The Ya8hoda bot is built with a modular architecture:

### Core Components

- `cmd/bot/`: Main application entry point and character configuration
- `internal/`:
  - `auth/`: User authentication and permission policies
  - `core/`: Core interfaces and domain models
  - `embed/`: Vector embedding generation using BGE-M3
  - `elevenlabs/`: Text-to-speech integration
  - `imageutils/`: Image processing utilities
  - `llm/`: LLM integration with OpenRouter
  - `logger/`: Structured logging facility
  - `rag/`: Retrieval-Augmented Generation with Milvus
  - `solana/`: Solana blockchain integration
  - `telegram/`: Telegram Bot API client and handlers
  - `tools/`: Tool router and implementations
- `tools/`: Contains the BGE-M3 embedding model files
- `tools-spec/`: Tool specifications for LLM function calling

### Data Storage

- `data/`: Contains persistent storage:
  - `etcd/`: For Milvus metadata
  - `milvus/`: Vector database files
  - `minio/`: Object storage for Milvus
  - `models/`: Local models (BGE-M3)
  - `tmp/`: Temporary file storage

## Message Flow

1. **User Interaction**: A message is received via Telegram
2. **Authentication**: User permissions are checked
3. **Embedding**: The message is converted into vector embeddings
4. **Memory Retrieval**:
   - Relevant facts are retrieved from various collections:
     - `people_facts`: Memories about individuals
     - `community_facts`: Community-related memories
     - `bot_facts`: Facts about Ya8hoda's persona
5. **LLM Processing**:
   - The message, retrieved context, and tool capabilities are sent to the LLM
   - The LLM may use various tools to enhance its response
6. **Response Generation**:
   - The final response may include text, voice notes, or images
   - Memories may be stored for future reference

## Available Tools

- **Memory Management**:
  - `remember_about_self`: Retrieve bot memories
  - `remember_about_person`: Retrieve memories about specific people
  - `remember_about_community`: Retrieve community-related memories
  - `store_self_memory`: Store new facts about the bot
  - `store_person_memory`: Store memories about individuals
  - `store_community_memory`: Store community-related information
  
- **Media Tools**:
  - `send_urls_as_image`: Convert web URLs to images
  - `send_voice_note`: Generate voice notes from text

- **Web3 Tools**:
  - `solana_get_tokens`: Fetch token balances for a Solana address
  - `solana_get_token_info`: Get detailed information about specific tokens

## Configuration

Create a `.env` file in the project root with the following variables:

```
# Required settings
TG_BOT_TOKEN=your_telegram_bot_token
OPENROUTER_API_KEY=your_openrouter_api_key
OPENROUTER_MODEL=meta-llama/llama-3-70b-instruct
EMBEDDING_API_URL=http://bge-embedding:8000

# Vector database settings
MILVUS_ADDRESS=milvus-standalone:19530
FRESH_START=false

# Optional settings
LOG_LEVEL=info
ADMIN_USER_IDS=123456789,987654321
ALLOWED_USER_IDS=123456789,987654321
ELEVENLABS_API_KEY=your_elevenlabs_api_key
ELEVENLABS_VOICE_ID=your_voice_id
```

## Running the Bot

### Using Docker Compose (recommended)

```bash
docker-compose up -d
```

### Development Setup

```bash
# Start only the dependencies
docker-compose up -d milvus-standalone etcd minio bge-embedding

# Run the bot locally
go run cmd/bot/main.go -debug
```

## Character Customization

Edit `cmd/bot/character.json` to modify Ya8hoda's persona, including:
- Background information
- Conversation examples
- Style and topics
- Adjectives and personality traits

## Running Ya8hoda Locally

To experience Ya8hoda's full potential on your local machine without external dependencies, follow these steps:

### Setting Up Local Dependencies

#### 1. Local Embedding Model

Ya8hoda uses the BGE-M3 embedding model for understanding text. To run this locally:

1. Download the BGE-M3 model files from [Hugging Face](https://huggingface.co/BAAI/bge-m3)
2. Place the model files in `tools/models/BAAI/bge-m3/`
3. The embedding server in `tools/embedding_server.py` will use these local files

#### 2. Local Solana RPC

For Web3 functionality, Ya8hoda needs access to a Solana RPC endpoint:

**Option A: Public RPC (Easiest but with rate limits)**
- Use a public RPC endpoint like `https://api.mainnet-beta.solana.com/`
- Set this in your `.env` file or pass it to the Solana client

**Option B: Local Solana Node (Complete independence)**
1. [Install Solana](https://docs.solana.com/cli/install-solana-cli-tools)
2. Run a local validator:
   ```bash
   solana-test-validator
   ```
3. Use the local RPC endpoint: `http://127.0.0.1:8899`

#### 3. Local LLM (Optional)

For complete independence from external APIs, you can run a local LLM:

1. Install [llama.cpp](https://github.com/ggerganov/llama.cpp)
2. Download a compatible model like Llama-3-70B
3. Run the server:
   ```bash
   ./server -m /path/to/model --host 0.0.0.0 --port 8080
   ```
4. Update the `OPENROUTER_API_KEY` and related settings to use your local endpoint

### Running the Complete Stack Locally

1. Start the local services (if using fully local setup):
   ```bash
   # Start Milvus and supporting services
   docker-compose up -d milvus-standalone etcd minio
   
   # Start the local embedding server
   cd tools
   python embedding_server.py --host 0.0.0.0 --port 8000 --local-model-path models/BAAI/bge-m3
   ```

2. Configure your environment:
   ```bash
   # Create a .env file with local settings
   cat > .env << EOL
   TG_BOT_TOKEN=your_telegram_bot_token
   OPENROUTER_API_KEY=your_openrouter_api_key
   OPENROUTER_MODEL=meta-llama/llama-3-70b-instruct
   EMBEDDING_API_URL=http://localhost:8000
   MILVUS_ADDRESS=localhost:19530
   ELEVENLABS_API_KEY=your_elevenlabs_api_key
   ELEVENLABS_VOICE_ID=your_voice_id
   LOG_LEVEL=debug
   EOL
   ```

3. Run Ya8hoda:
   ```bash
   go run cmd/bot/main.go -debug
   ```

Now you have a fully local instance of Ya8hoda with complete control over all aspects of her functionality, from her voice to her memory to her blockchain connections.