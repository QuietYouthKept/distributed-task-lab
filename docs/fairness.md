# Priority fairness and aging

Default queues retain strict priority: `priority DESC`, then FIFO. This can
starve lower-priority work under a permanent high-priority stream.

Set both fields on a queue to opt into bounded aging:

```json
{"aging_after_seconds":60,"aging_priority_boost":100}
```

After the threshold, a pending task's effective priority is its stored priority
plus the boost. The stored priority is never mutated. A `priority_promoted`
event records the original and effective priorities, and the subsequent claim
event records `priority_promoted=true`. Lease reclaims remain ahead of pending
work and do not receive aging boosts.

For each policy, run 80% continuous high priority and 20% low priority traffic
and compare high/low p95 and maximum queue wait. The boost must be large enough
for the queue's priority range; aging improves liveness but does not itself
prove a maximum-wait SLO.
