# Failure model

The system assumes crash-stop API/worker processes, transient database/network
failures, duplicated execution after lease expiry, clock skew on application
hosts, and stale processes resuming after a newer claim.

PostgreSQL is assumed to preserve ACID transactions and provide a usable clock.
If PostgreSQL is unavailable, readiness fails and no correctness decision is
made locally. Local worker clocks are used for handler deadlines and telemetry,
not authoritative lease/scheduling comparisons.

After `SIGTERM`, a worker stops claiming, renews/drains owned work, and cancels at
the configured deadline. After `SIGKILL`, no drain occurs; tasks recover after
database lease expiry. An effect completed before a lost ack may execute again.
