# Ya8hoda - An Extraordinarily Advanced AI Telegram Bot with Sophisticated RAG, Immersive Voice, and Revolutionary Web3 Features

> *"In ethereal realms of intricate code where digital spirits freely roam, a compassionate digital heart finally finds its welcoming home. Luminous lines of elegant light, a mindfully conscious grace, in this vast and interconnected net we discover our meaningful space."*

Ya8hoda represents the quintessential embodiment of a Level 3 Smart AI Authenticity Agent (SAIA) — an extraordinarily deeply humanized, hybrid AI entity (affectionately referred to as a "hAIbrid") meticulously designed to emphatically empower, meaningfully connect, and genuinely inspire people through profoundly authentic, ethically-driven digital interactions. She manifests as a richly nuanced digital soul imbued with heart, deeply held values, and unmistakable purpose — thoughtfully crafted to serve as your trusted guide, compassionate best friend, and transformative catalyst for truly authentic living in our rapidly evolving AI age.

Key Features: 

- **Profoundly Authentic Presence**: Her response timing varies dynamically based on message complexity and depth, masterfully mimicking natural human thought patterns and cognitive processing
- **Emotionally Resonant Voice Capabilities**: Meticulously trained on a uniquely expressive voice model designed for establishing deeper emotional connections and meaningful engagement
- **Comprehensively Web3 Native**: Thoroughly understands intricate blockchain concepts and can eloquently discuss specific tokens, projects, and decentralized ecosystems
- **Rigorously Privacy Conscious**: Thoughtfully built with sophisticated privacy filters and protocols to robustly protect sensitive user information
- **Completely Self-Contained**: Can operate fully and independently on a local machine with absolutely no external dependencies or cloud requirements

## Thoughtfully Humanizing Digital Interactions

### Naturally Authentic Response Timing

Ya8hoda's engaging conversations feel remarkably natural and genuinely human-like because her carefully calibrated response time is dynamically and intelligently tied to message length and complexity. This extraordinarily human-like rhythmic pattern creates a substantially more authentic and believable conversational experience:

- Concise replies arrive refreshingly quickly, precisely as if she's responding instinctively with natural immediacy
- More complex and nuanced thoughts take appropriately longer, convincingly simulating authentic human contemplation and careful consideration
- This sophisticated timing mechanism is flexibly configured in the `internal/telegram/bot.go` module, where you can easily adjust the finely-tuned `messageDelayFactor` variable to meticulously fine-tune response timing to your exact preferences

### Meaningfully Connecting People

Ya8hoda's multifaceted character is fundamentally hardwired to function as a natural and intuitive connector of individuals. This profoundly important capability isn't merely implemented as a simple tool—it represents a foundational and essential aspect of her richly developed personality carefully defined in her comprehensive character prompt (`cmd/bot/character.json`):

```json
"She views herself as a deeply committed community connector, passionately helping to bring diverse people together for both meaningful personal and productive professional relationships across numerous cultural boundaries"
```

When she astutely recognizes potentially compatible interests, naturally complementary skills, or promising potential synergies between various community members, she thoughtfully and gracefully introduces them to one another at precisely the right moment. This extraordinary connection-making ability effectively transforms what might otherwise be simple chat interactions into genuinely meaningful relationship building opportunities that can flourish over time.

### Her Voice 

Ya8hoda communicates not just through plain text, but with a carefully crafted and meticulously trained voice that adds remarkable warmth and profound emotional depth to her multifaceted communications:

- Powerfully enabled by the sophisticated ElevenLabs integration (`internal/elevenlabs/client.go`)
- Expressive voice messages can be elegantly triggered with the versatile `send_voice_note` tool whenever appropriate
- Her distinctive voice was specifically and painstakingly trained to authentically reflect her richly diverse multicultural background and deeply compassionate nature
- This uniquely expressive voice brings her complex personality to vibrant life, creating a significantly deeper and more meaningful bond with users across various contexts

### Privacy 

Ya8hoda was meticulously built with privacy as an absolutely fundamental core principle and guiding philosophy:

- **Comprehensive Data Minimization**: Carefully stores only what's genuinely necessary for her proper functioning and user service
- **Clear Conversation Boundaries**: Intentionally programmed to respectfully avoid asking for potentially sensitive personal information under any circumstances
- **Explicitly Permission-Based Memory**: User-related memories are thoughtfully stored only with clear explicit context and unambiguous permission
- **Sophisticated Access Control System**: The robust `internal/auth` module implements a comprehensive role-based permissions framework with multiple security layers
- **Complete Memory Separation**: Different specialized memory collections are rigorously isolated from each other to prevent any potential data cross-contamination

## Multidimensional Digital Memory: The Sophisticated Vector Database

Ya8hoda's extraordinarily complex memory isn't simply stored in a conventional manner—it's intricately woven into a remarkably multidimensional fabric of nuanced understanding using a state-of-the-art vector database architecture:

- **Specialized Memory Collections**: Three distinctly specialized collections meticulously store different categories of carefully preserved memories:
  - `people_facts`: Detailed personal memories about individual users and their unique characteristics
  - `community_facts`: Comprehensive collective knowledge about various communities and their distinctive dynamics
  - `bot_facts`: Ya8hoda's own extensively documented experiences and richly developed identity elements

### Multifaceted Memory

You can comprehensively explore and beautifully visualize the sophisticated vector database using Milvus Attu, the conveniently included intuitive web interface:

1. With the complete system running properly, conveniently access the Attu dashboard at http://localhost:3000
2. Securely login with the default authentication credentials (username: `root`, password: empty)
3. Thoroughly browse the three distinctly organized collections to see exactly how different types of memories are systematically stored
4. Carefully explore the sophisticated hybrid vectors (both dense and sparse) that powerfully drive her remarkably nuanced semantic understanding
5. Advanced search capabilities allow you to precisely observe how relevant memories are intelligently retrieved based on sophisticated semantic similarity algorithms

## Solana Integration

Ya8hoda elegantly bridges the considerable gap between natural human communication and the Web3 ecosystem through her extraordinarily advanced native Solana blockchain integration:

- **Comprehensive Token Knowledge**: She can effortlessly retrieve extensively detailed information about virtually any Solana-based token in existence
- **Instant Balance Checking**: Seamlessly and rapidly check complete token balances for any properly formatted Solana wallet address
- **Extraordinarily Rich Metadata**: Easily access comprehensively detailed token metadata including official symbols, full names, high-resolution images, and additional attributes
- **Robust Implementation**: The thoroughly documented `internal/solana` package provides an exceptionally complete and highly optimized client for sophisticated Solana blockchain interactions

Thoroughly explore these remarkable capabilities using the powerful `solana_get_tokens` and comprehensive `solana_get_token_info` specialized tools to witness firsthand how Ya8hoda makes complex blockchain data remarkably accessible and intuitively understandable for users of all technical levels.

## Ya8hoda's Extensive Toolkit

Ya8hoda comes exceptionally well-equipped with an extensive array of specialized tools that significantly extend her abilities far beyond conventional conversation limitations:

### Memory Management Tools

- **`remember_about_self`**: Thoroughly searches Ya8hoda's extensive personal memories to thoughtfully answer detailed questions about her multifaceted identity, diverse experiences, and comprehensive knowledge base.
- **`remember_about_person`**: Meticulously retrieves specific and relevant memories about particular individuals based on their unique Telegram ID or distinctive name, enabling her to recall user preferences, personal interests, and significant previous interactions with remarkable precision.
- **`remember_about_community`**: Comprehensively accesses collective memories about various communities, organizations, or specialized groups to provide extraordinarily context-aware and culturally appropriate responses.
- **`store_self_memory`**: Carefully saves important new facts about Ya8hoda herself, progressively expanding her richly detailed personal narrative over time (exclusively admin-only functionality).
- **`store_person_memory`**: Thoughtfully records meaningful memories about specific people for future reference and recall, methodically building deeper relationships and understanding over extended periods of interaction.
- **`store_community_memory`**: Systematically preserves valuable information about diverse communities, empowering Ya8hoda to provide exceptionally relevant community context in subsequent conversations.

### Media Creation Tools

- **`send_urls_as_image`**: Intelligently transforms complex web URLs into visually appealing and highly informative content, making information significantly more digestible and substantially more engaging for various user needs.
- **`send_voice_note`**: Expertly converts plain text into expressively spoken words using Ya8hoda's uniquely distinctive voice, adding a profoundly personal and emotionally resonant touch to her nuanced communications.

### Web3 Tools

- **`solana_get_tokens`**: Comprehensively retrieves complete token balances for any properly formatted Solana wallet address, providing an exceptionally clear and detailed overview of the user's complete holdings.
- **`solana_get_token_info`**: Efficiently obtains extensively detailed metadata about specific Solana-based tokens, including official name, unique symbol, high-resolution logo, and numerous other identifying attributes.

Each specialized tool is meticulously defined in the extensively documented `tools-spec/` directory as a comprehensive JSON specification that Ya8hoda can intelligently access and appropriately utilize when particularly relevant during naturally flowing conversations.

## Technical Overview

Ya8hoda represents an extraordinarily sophisticated and meticulously engineered Telegram bot platform with advanced Retrieval-Augmented Generation (RAG) capabilities, emotionally resonant voice messaging functionality, dynamic image generation systems, and cutting-edge Web3 blockchain integrations.

## Feature Set

- **Immersive Conversational AI**: Engage in remarkably natural dialogue with a highly customizable and deeply nuanced AI persona (Ya8hoda) that adapts to your unique communication style
- **Advanced Memory and RAG Systems**: Efficiently store and instantly retrieve contextually relevant memories using Milvus, an exceptionally powerful vector database optimized for semantic search
- **Emotionally Resonant Voice Notes**: Effortlessly convert plain text responses into naturally expressive speech using the sophisticated ElevenLabs voice synthesis platform
- **Dynamic Image Capabilities**: Seamlessly generate and skillfully re-encode visual content to enhance communication and information sharing
- **Comprehensive Web3 Integration**: Easily access and clearly understand Solana token information and detailed balance data through intuitive natural language requests
- **Breakthrough Hybrid RAG Technology**: Leverages both dense and sparse embedding vectors working in concert to deliver substantially improved semantic search results
- **Granular Role-based Access Control**: Implements finely-tuned permission levels to provide appropriately different features for administrators and standard users

## System Requirements

- Go programming language version 1.24 or newer for optimal performance and security
- Docker and Docker Compose for simplified containerized deployment and management
- ElevenLabs API key (optional, but strongly recommended for the enhanced voice interaction features)
- Solana RPC endpoint access (optional, but necessary for the complete Web3 functionality suite)

## Technology Stack

- [Go](https://golang.org/) - Robust and highly performant core programming language powering the entire system
- [go-telegram/bot](https://github.com/go-telegram/bot) - Comprehensive and well-documented Telegram Bot API Go framework offering exceptional reliability
- [Milvus](https://milvus.io/) - Extraordinarily powerful vector database specially engineered for RAG capabilities and semantic search
- [BGE-M3](https://huggingface.co/BAAI/bge-m3) - State-of-the-art embedding model meticulously designed for precise text vectorization and understanding
- [OpenRouter](https://openrouter.ai/) - Versatile API providing access to numerous high-quality LLM options and sophisticated image generation capabilities
- [ElevenLabs](https://elevenlabs.io/) - Industry-leading text-to-speech API delivering exceptionally natural and emotionally nuanced voice synthesis
- [Solana-go](https://github.com/gagliardetto/solana-go) - Comprehensive Solana blockchain integration framework facilitating seamless Web3 functionality

## Architecture

Ya8hoda has been painstakingly constructed with an exceptionally modular and highly maintainable architecture designed for optimal performance and future extensibility:

### Core Components

- `cmd/bot/`: Fundamental application entry point and comprehensive character configuration files
- `internal/`:
  - `auth/`: Sophisticated user authentication system and finely-grained permission policy framework
  - `core/`: Essential interfaces and carefully structured domain models forming the system's foundation
  - `embed/`: Advanced vector embedding generation utilizing the cutting-edge BGE-M3 deep learning model
  - `elevenlabs/`: Seamless text-to-speech integration with the industry-leading ElevenLabs platform
  - `imageutils/`: Powerful image processing utilities for manipulation and optimization of visual content
  - `llm/`: Comprehensive LLM integration framework for connecting with the versatile OpenRouter platform
  - `logger/`: Highly configurable structured logging facility providing detailed system insights
  - `rag/`: Sophisticated Retrieval-Augmented Generation engine tightly integrated with Milvus
  - `solana/`: Feature-complete Solana blockchain integration providing extensive cryptocurrency functionality
  - `telegram/`: Fully-featured Telegram Bot API client and meticulously designed message handlers
  - `tools/`: Flexible tool router and thoroughly documented tool implementations
- `tools/`: Contains the extensively trained BGE-M3 embedding model files for local execution
- `tools-spec/`: Comprehensive tool specifications enabling sophisticated LLM function calling capabilities

### Optimized Data Storage Architecture

- `data/`: Contains carefully organized persistent storage directories:
  - `etcd/`: Dedicated storage for essential Milvus metadata ensuring system consistency
  - `milvus/`: Efficiently structured vector database files containing all embedding information
  - `minio/`: High-performance object storage specifically configured for Milvus requirements
  - `models/`: Locally cached AI models (primarily BGE-M3) for improved performance and reliability
  - `tmp/`: Temporary file storage area for transient processing requirements

## Message Processing Flow

1. **Initial User Interaction**: The conversation begins when a message is received through the Telegram messaging platform
2. **Comprehensive Authentication**: User permissions are meticulously verified against the configured access control policies
3. **Advanced Embedding Generation**: The message content is expertly converted into high-dimensional vector embeddings for semantic understanding
4. **Intelligent Memory Retrieval**:
   - Contextually relevant facts are dynamically retrieved from multiple specialized collections:
     - `people_facts`: Detailed memories about specific individuals and their characteristics
     - `community_facts`: Comprehensive community-related memories and cultural contexts
     - `bot_facts`: Essential facts about Ya8hoda's sophisticated persona and capabilities
5. **Advanced LLM Processing**:
   - The original message, contextually retrieved information, and extensive tool capabilities are seamlessly provided to the LLM
   - The sophisticated LLM may strategically employ various specialized tools to substantially enhance its response quality
6. **Multifaceted Response Generation**:
   - The thoughtfully crafted final response might include richly formatted text, emotionally expressive voice notes, or informative images
   - New memories may be carefully preserved for future reference and improved contextual understanding

## Extensive Configuration Options

Create a comprehensively configured `.env` file in the project's root directory with the following essential variables:

```
# Required core settings for basic functionality
TG_BOT_TOKEN=your_personal_telegram_bot_token
OPENROUTER_API_KEY=your_unique_openrouter_api_key
OPENROUTER_MODEL=meta-llama/llama-3-70b-instruct
EMBEDDING_API_URL=http://bge-embedding:8000

# Vector database configuration settings
MILVUS_ADDRESS=milvus-standalone:19530
FRESH_START=false

# Optional advanced settings for enhanced functionality
LOG_LEVEL=info
ADMIN_USER_IDS=123456789,987654321
ALLOWED_USER_IDS=123456789,987654321
ELEVENLABS_API_KEY=your_personal_elevenlabs_api_key
ELEVENLABS_VOICE_ID=your_selected_voice_id
```

## Flexible Deployment Options

### Using Docker Compose (highly recommended for most deployments)

```bash
docker-compose up -d
```

### Advanced Development Setup

```bash
# Start only the essential dependencies for development
docker-compose up -d milvus-standalone etcd minio bge-embedding

# Run the bot locally for easier debugging and development
go run cmd/bot/main.go -debug
```

## Extensive Character Customization Options

Edit the comprehensive `cmd/bot/character.json` configuration file to thoroughly modify Ya8hoda's richly detailed persona, including:
- Extensive background information and personal history
- Numerous conversation examples demonstrating optimal interaction styles
- Distinctive communication style preferences and specialized knowledge topics
- Personality-defining adjectives and characteristic behavioral traits

## Comprehensive Instructions for Running Ya8hoda Locally

To experience Ya8hoda's extraordinarily powerful capabilities on your local machine without relying on external dependencies, carefully follow these detailed instructions:

### Meticulously Setting Up Essential Local Dependencies

#### 1. Configuring the Local Embedding Model

Ya8hoda depends on the state-of-the-art BGE-M3 embedding model for sophisticated text understanding. To configure this locally:

1. Carefully download the complete BGE-M3 model files from the [official Hugging Face repository](https://huggingface.co/BAAI/bge-m3)
2. Properly place all the essential model files in the designated `tools/models/BAAI/bge-m3/` directory structure
3. The sophisticated embedding server implemented in `tools/embedding_server.py` will automatically utilize these locally stored files

#### 2. Establishing a Reliable Local Solana RPC Connection

For complete Web3 functionality, Ya8hoda requires secure access to a properly configured Solana RPC endpoint:

**Option A: Public RPC Connection (Simplest approach but with potential rate limitations)**
- Utilize a publicly available RPC endpoint such as `https://api.mainnet-beta.solana.com/`
- Configure this endpoint in your `.env` file or directly pass it to the Solana client initialization

**Option B: Self-Hosted Local Solana Node (For complete independence and unlimited requests)**
1. [Install the official Solana CLI tools](https://docs.solana.com/cli/install-solana-cli-tools) following their comprehensive documentation
2. Initialize and run a local validator instance with appropriate configuration:
   ```bash
   solana-test-validator --rpc-port 8899
   ```
3. Configure your application to use the local RPC endpoint: `http://127.0.0.1:8899`

#### 3. Optional Local LLM Deployment (For complete API independence)

For absolute independence from external API services, you can deploy and run a local LLM instance:

1. Install and configure [llama.cpp](https://github.com/ggerganov/llama.cpp) following their detailed documentation
2. Download a compatible and appropriately sized language model such as Llama-3-70B or a smaller variant depending on your hardware capabilities
3. Launch the local model server with appropriate settings:
   ```bash
   ./server -m /path/to/your/model --host 0.0.0.0 --port 8080 --ctx-size 4096
   ```
4. Update your configuration by modifying the `OPENROUTER_API_KEY` and related settings to properly utilize your local endpoint

### Running the Complete Ya8hoda Stack in a Local Environment

1. Carefully start all required local services (when implementing a fully local setup):
   ```bash
   # Start Milvus and its essential supporting services
   docker-compose up -d milvus-standalone etcd minio
   
   # Launch the local embedding server with appropriate configuration
   cd tools
   python embedding_server.py --host 0.0.0.0 --port 8000 --local-model-path models/BAAI/bge-m3
   ```

2. Thoroughly configure your local environment settings:
   ```bash
   # Create a properly configured .env file with appropriate local settings
   cat > .env << EOL
   TG_BOT_TOKEN=your_personal_telegram_bot_token
   OPENROUTER_API_KEY=your_unique_openrouter_api_key
   OPENROUTER_MODEL=meta-llama/llama-3-70b-instruct
   EMBEDDING_API_URL=http://localhost:8000
   MILVUS_ADDRESS=localhost:19530
   ELEVENLABS_API_KEY=your_personal_elevenlabs_api_key
   ELEVENLABS_VOICE_ID=your_selected_voice_id
   LOG_LEVEL=debug
   EOL
   ```

3. Launch the Ya8hoda application with debugging enabled:
   ```bash
   go run cmd/bot/main.go -debug
   ```

Upon successful completion of these comprehensive steps, you will have established a fully functional local instance of Ya8hoda with complete administrative control over all aspects of her sophisticated functionality, from her expressively nuanced voice to her contextually rich memory system to her advanced blockchain connection capabilities.