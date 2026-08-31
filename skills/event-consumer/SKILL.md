---
name: event-consumer
description: Implement or review exchange-lab NATS JetStream publishers and consumers, including Transactional Outbox, event schemas, durable consumers, acknowledgement, redelivery, idempotency, replay, parking, correlation IDs, and bounded failure handling.
---

# Event Consumer

Assume at-least-once delivery. Duplicates are normal and must be harmless.

## Workflow

1. Read `docs/adr/ADR-003-transactional-outbox.md`, the event migration, and `references/checklist.md`.
2. Write a failing test for duplicate delivery, crash-before-ack, or replay before changing the consumer.
3. Define an immutable versioned envelope with stable event ID, type, aggregate ID, correlation ID, timestamp, and bounded object payload. Preserve decimal strings.
4. Publish only committed Outbox rows. Set a stable JetStream dedupe ID, require publish acknowledgement, and update Outbox afterward. A publish timeout remains retryable.
5. Use a named durable pull consumer, explicit ACK, bounded AckWait/MaxDeliver/MaxAckPending, and finite backoff. Empty fetches are normal.
6. In one database transaction, insert `(consumer,event_id)` and apply the side effect. On duplicate, perform no side effect and ACK.
7. ACK only after commit. NAK retryable failures with delay. Park invalid or exhausted events with a sanitized reason and terminate that delivery.
8. Test NATS restart, consumer crash, ack timeout, duplicates, out-of-order input, incompatible versions, parking, and deterministic replay.

Never publish an irreversible business message before its database transaction commits. Never add an AI-controlled event that places/cancels orders or changes leverage, margin, balances, liquidation, or settlement.
