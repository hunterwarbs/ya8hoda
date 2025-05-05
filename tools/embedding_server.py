#!/usr/bin/env python3
"""
Embedding server for BGE-M3 model.
This server exposes an API endpoint that the Go code can call to get embeddings.
"""

import argparse
import json
import logging
import os
from typing import List, Dict, Any, Optional

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
import uvicorn

# Import BGE-M3 embedding function
try:
    from pymilvus.model.hybrid import BGEM3EmbeddingFunction
except ImportError:
    raise ImportError(
        "Please install pymilvus with the model extra: "
        "pip install 'pymilvus[model]'"
    )

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
)
logger = logging.getLogger("embedding_server")

# Initialize FastAPI app
app = FastAPI(title="BGE-M3 Embedding Server")

# Load the embedding model
bge_m3_ef = None


class EmbeddingRequest(BaseModel):
    texts: List[str]
    model_name: str = "BAAI/bge-m3"


class EmbeddingResponse(BaseModel):
    embeddings: Dict[str, Any]
    error: Optional[str] = None


@app.on_event("startup")
async def startup_event():
    global bge_m3_ef
    try:
        # Initialize the BGE-M3 embedding function
        model_name = "BAAI/bge-m3"
        device = "cpu"  # Can be changed to "cuda:0" for GPU
        use_fp16 = False  # Set to False when using CPU

        logger.info(f"Loading BGE-M3 model: {model_name} on {device}")
        bge_m3_ef = BGEM3EmbeddingFunction(
            model_name=model_name,
            device=device,
            use_fp16=use_fp16
        )
        logger.info("BGE-M3 model loaded successfully")
    except Exception as e:
        logger.error(f"Failed to load BGE-M3 model: {str(e)}")
        raise


@app.post("/embed", response_model=EmbeddingResponse)
async def embed(request: EmbeddingRequest) -> EmbeddingResponse:
    """
    Generate embeddings for the provided texts.
    By default, treats texts as queries.
    """
    global bge_m3_ef
    
    if bge_m3_ef is None:
        raise HTTPException(
            status_code=500,
            detail="Embedding model not initialized"
        )
    
    try:
        # Check if the requested model matches the loaded model
        if request.model_name != bge_m3_ef.model_name:
            logger.warning(
                f"Requested model {request.model_name} doesn't match loaded model "
                f"{bge_m3_ef.model_name}. Using loaded model."
            )
        
        logger.info(f"Generating query embeddings for {len(request.texts)} texts")
        
        # Generate query embeddings
        embeddings = bge_m3_ef.encode_queries(request.texts)
        
        # Convert numpy arrays to lists for JSON serialization
        serializable_embeddings = {
            "dense": [emb.tolist() for emb in embeddings["dense"]],
            "sparse": [
                {
                    "indices": sparse_emb.indices.tolist(),
                    "values": sparse_emb.data.tolist(),
                    "shape": sparse_emb.shape,
                }
                for sparse_emb in embeddings["sparse"]
            ]
        }
        
        logger.info(f"Generated embeddings: {serializable_embeddings}")
        return EmbeddingResponse(embeddings=serializable_embeddings)
    
    except Exception as e:
        logger.error(f"Error generating embeddings: {str(e)}")
        return EmbeddingResponse(
            embeddings={},
            error=f"Failed to generate embeddings: {str(e)}"
        )


@app.post("/embed/document", response_model=EmbeddingResponse)
async def embed_document(request: EmbeddingRequest) -> EmbeddingResponse:
    """
    Generate document embeddings for the provided texts.
    This is different from query embeddings as they are asymmetric.
    """
    global bge_m3_ef
    
    if bge_m3_ef is None:
        raise HTTPException(
            status_code=500,
            detail="Embedding model not initialized"
        )
    
    try:
        # Check if the requested model matches the loaded model
        if request.model_name != bge_m3_ef.model_name:
            logger.warning(
                f"Requested model {request.model_name} doesn't match loaded model "
                f"{bge_m3_ef.model_name}. Using loaded model."
            )
        
        logger.info(f"Generating document embeddings for {len(request.texts)} texts")
        
        # Generate document embeddings
        embeddings = bge_m3_ef.encode_documents(request.texts)
        
        # Convert numpy arrays to lists for JSON serialization
        serializable_embeddings = {
            "dense": [emb.tolist() for emb in embeddings["dense"]],
            "sparse": [
                {
                    "indices": sparse_emb.indices.tolist(),
                    "values": sparse_emb.data.tolist(),
                    "shape": sparse_emb.shape,
                }
                for sparse_emb in embeddings["sparse"]
            ]
        }
        
        return EmbeddingResponse(embeddings=serializable_embeddings)
    
    except Exception as e:
        logger.error(f"Error generating document embeddings: {str(e)}")
        return EmbeddingResponse(
            embeddings={},
            error=f"Failed to generate document embeddings: {str(e)}"
        )


@app.get("/health")
async def health_check():
    """
    Health check endpoint that verifies the model is loaded and functioning.
    """
    global bge_m3_ef
    
    if bge_m3_ef is None:
        raise HTTPException(
            status_code=503,
            detail="Embedding model not initialized"
        )
    
    try:
        # Simple test to verify the model is actually working
        test_text = "This is a test sentence for health check."
        _ = bge_m3_ef.encode_queries([test_text])
        return {
            "status": "healthy", 
            "model": bge_m3_ef.model_name,
            "dimensions": {
                "dense": bge_m3_ef.dim.get("dense", 0),
                "sparse": bge_m3_ef.dim.get("sparse", 0)
            }
        }
    except Exception as e:
        logger.error(f"Health check failed: {str(e)}")
        raise HTTPException(
            status_code=503,
            detail=f"Model check failed: {str(e)}"
        )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="BGE-M3 Embedding Server")
    parser.add_argument(
        "--host", type=str, default="127.0.0.1",
        help="Host to bind the server to"
    )
    parser.add_argument(
        "--port", type=int, default=8000,
        help="Port to bind the server to"
    )
    parser.add_argument(
        "--model", type=str, default="BAAI/bge-m3",
        help="BGE-M3 model name to use"
    )
    parser.add_argument(
        "--local-model-path", type=str, default=None,
        help="Local path to the BGE-M3 model (to prevent downloading)"
    )
    parser.add_argument(
        "--device", type=str, default="cpu",
        choices=["cpu", "cuda:0", "cuda:1", "cuda:2", "cuda:3"],
        help="Device to run the model on"
    )
    parser.add_argument(
        "--fp16", action="store_true",
        help="Use FP16 precision (not compatible with CPU)"
    )
    
    args = parser.parse_args()
    
    # Override global model settings based on command line arguments
    @app.on_event("startup")
    async def override_model_settings():
        global bge_m3_ef
        try:
            model_name = args.model
            
            # If a local model path is provided, set the transformer models cache directory
            if args.local_model_path:
                local_model_path = os.path.abspath(args.local_model_path)
                logger.info(f"Using local model path: {local_model_path}")
                os.environ["TRANSFORMERS_CACHE"] = os.path.dirname(local_model_path)
                if os.path.exists(local_model_path):
                    model_name = local_model_path
                else:
                    logger.warning(f"Local model path {local_model_path} not found. Falling back to download.")
            
            logger.info(f"Loading BGE-M3 model: {model_name} on {args.device}")
            bge_m3_ef = BGEM3EmbeddingFunction(
                model_name=model_name,
                device=args.device,
                use_fp16=args.fp16
            )
            logger.info("BGE-M3 model loaded successfully")
            logger.info(f"Dense embedding dimension: {bge_m3_ef.dim['dense']}")
            logger.info(f"Sparse embedding dimension: {bge_m3_ef.dim['sparse']}")
        except Exception as e:
            logger.error(f"Failed to load BGE-M3 model: {str(e)}")
            raise
    
    # Run the server
    uvicorn.run(
        app, host=args.host, port=args.port,
        log_level="info"
    ) 