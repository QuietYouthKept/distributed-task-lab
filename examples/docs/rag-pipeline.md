# RAG Pipeline

Retrieval-Augmented Generation (RAG) grounds a language model's answers in
retrieved documents instead of relying on the model's parametric memory alone.

The pipeline has three stages. First, documents are ingested and split into
chunks. Each chunk is embedded into a dense vector and stored in a vector index.

Second, at query time, the query is embedded with the same model and the index
is searched for the most similar chunks. These chunks become the retrieval context.

Third, the language model receives the query plus the retrieved chunks and
generates an answer grounded in that context, ideally with citations.
