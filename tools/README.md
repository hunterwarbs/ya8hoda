# BGE-M3 Embedding Server

This directory contains a FastAPI server that exposes the BGE-M3 embedding model for use with the Milvus vector database.

## Features

- **Multi-Lingual Support**: BGE-M3 can handle over 100 languages for multilingual and cross-lingual retrieval tasks
- **Multi-Functionality**: Provides both dense and sparse embeddings in a single model
- **Multi-Granularity**: Can perform different levels of retrieval granularity
- **Docker Integration**: Ready to use in Docker Compose environment
- **Local Model Support**: Can use a local copy of the model to prevent downloads

## Prerequisites

- Python 3.8 or higher
- PyMilvus with model support

## Installation

### Local Installation

1. Install the required dependencies:

```bash
pip install fastapi uvicorn
pip install "pymilvus[model]"
```

2. You can either:
   - Let the server download the BGE-M3 model (approximately 2.3GB) on first run, or
   - Download the model in advance and specify the local path (recommended for production)

### Pre-downloading the model

To download the model in advance, you can use the Hugging Face CLI:

```bash
# Install huggingface_hub if not already installed
pip install huggingface_hub

# Download the model to tools/models/BAAI/bge-m3
python -c "from huggingface_hub import snapshot_download; snapshot_download(repo_id='BAAI/bge-m3', local_dir='tools/models/BAAI/bge-m3')"
```

### Docker Installation

A Dockerfile and docker-compose configuration are provided for easy deployment:

1. Build the Docker image:
```bash
docker build -f tools/Dockerfile.bge -t bge-embedding .
```

2. Or use the provided docker-compose.yaml:
```bash
docker-compose up -d
```

This will start the BGE embedding server alongside the Milvus database.

## Usage

### Starting the server

#### Locally (downloading the model on first run)
```bash
python embedding_server.py --host 127.0.0.1 --port 8000
```

#### Locally (using pre-downloaded model)
```bash
python embedding_server.py --host 127.0.0.1 --port 8000 --local-model-path ./models/BAAI/bge-m3
```

#### With Docker Compose
```bash
docker-compose up -d bge-embedding
```

### Command-line options

- `--host`: Host to bind the server to (default: 127.0.0.1)
- `--port`: Port to bind the server to (default: 8000)
- `--model`: BGE-M3 model name to use (default: BAAI/bge-m3)
- `--local-model-path`: Local path to the BGE-M3 model (prevents downloading)
- `--device`: Device to run the model on. Options: cpu, cuda:0, cuda:1, etc. (default: cpu)
- `--fp16`: Use FP16 precision (not compatible with CPU)

### API Endpoints

#### `/embed` (POST)

Generate query embeddings for the provided texts.

**Request Body**:
```json
{
  "texts": ["Your text here", "Another text"],
  "model_name": "BAAI/bge-m3"
}
```

**Response**:
```json
{
  "embeddings": {
    "dense": [
      [0.1, 0.2, ...],
      [0.3, 0.4, ...]
    ],
    "sparse": [
      {
        "indices": [1, 5, 10],
        "values": [0.5, 0.3, 0.1],
        "shape": [1, 250002]
      },
      {
        "indices": [2, 6, 15],
        "values": [0.4, 0.2, 0.1],
        "shape": [1, 250002]
      }
    ]
  }
}
```

#### `/embed/document` (POST)

Generate document embeddings for the provided texts. This uses a different embedding method optimized for documents rather than queries.

**Request and response format are the same as for `/embed`.**

#### `/health` (GET)

Check if the server is healthy and the model is loaded.

**Response**:
```json
{
  "status": "healthy",
  "model": "BAAI/bge-m3"
}
```

## Integration with Go Client

The Go client in `internal/embed/bge.go` is configured to work with this embedding server. When using docker-compose, the client automatically connects to the BGE embedding service using the service name.

## Example Usage

### In Docker Compose Environment

1. Start all services:
```bash
docker-compose up -d
```

2. The Go application will automatically connect to the BGE embedding service using the service name.

### Local Development

1. Start the embedding server:
```bash
python tools/embedding_server.py --host 127.0.0.1 --port 8000 --local-model-path ./tools/models/BAAI/bge-m3
```

2. In your Go application, configure the BGE Embedder:
```go
config := embed.BGEEmbedderConfig{
    ApiURL: "http://localhost:8000",
    ModelName: "BAAI/bge-m3",
}
bgeEmbedder := embed.NewBGEEmbedder(milvusService, config)

// Generate embeddings
embedding, err := bgeEmbedder.EmbedQuery(context.Background(), "Your query text here")
if err != nil {
    log.Fatalf("Failed to create embedding: %v", err)
}

// Use the dense and/or sparse embeddings with Milvus
denseVector := embedding.Dense
sparseVector := embedding.Sparse
```

## References

- [BGE-M3 Documentation](https://milvus.io/docs/embed-with-bgm-m3.md)
- [Exploring BGE-M3: The Future of Information Retrieval with Milvus](https://zilliz.com/learn/Exploring-BGE-M3-the-future-of-information-retrieval-with-milvus) 