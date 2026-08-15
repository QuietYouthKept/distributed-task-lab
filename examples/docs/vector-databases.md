# Vector Databases

A vector database stores embeddings and supports approximate nearest-neighbor
search over them. Unlike a traditional index, it optimizes for similarity
rather than exact key lookup.

Common choices include pgvector for PostgreSQL, Milvus, Qdrant, and Weaviate.
Each offers different tradeoffs around latency, recall, and operational cost.

For a task-queue demo, a deterministic hash embedding is a faithful stand-in
for a real embedding model: it is stable across runs, which makes idempotent
re-ingestion easy to reason about.
