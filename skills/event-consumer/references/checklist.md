# Event Delivery Checklist

- Stable event ID is distinct from transport sequence and correlation ID.
- Envelope version/type/size/time/decimal fields are validated before use.
- Outbox and business facts share one commit; publisher claims use a lease and `SKIP LOCKED`.
- Stream subjects, retention, max messages/bytes/age/message size, replicas, and duplicate window are bounded.
- Durable consumer has explicit ACK, finite AckWait, MaxDeliver, MaxAckPending, and backoff.
- Consumer dedupe record and business side effect share one database transaction.
- ACK happens after commit; crash-before-ACK redelivery produces no duplicate side effect.
- Invalid/exhausted messages are parked with sanitized reason; replay procedure is documented.
- Logs never include full payloads, credentials, raw DSNs, tokens, or provider errors.
- Runtime check covers restart, duplicate delivery, backlog recovery, and receipt count.
