-- +goose Up
-- Adds durable consumer deduplication, receipts and terminal parking evidence.
CREATE TABLE processed_events (
    consumer_name TEXT NOT NULL CHECK (char_length(consumer_name) BETWEEN 1 AND 128),
    event_id TEXT NOT NULL CHECK (char_length(event_id) BETWEEN 1 AND 128),
    subject TEXT NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 128),
    processed_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, event_id)
);

CREATE TABLE event_receipts (
    consumer_name TEXT NOT NULL,
    event_id TEXT NOT NULL,
    aggregate_id TEXT NOT NULL CHECK (char_length(aggregate_id) BETWEEN 1 AND 128),
    correlation_id TEXT NOT NULL CHECK (char_length(correlation_id) BETWEEN 1 AND 128),
    received_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, event_id),
    FOREIGN KEY (consumer_name, event_id) REFERENCES processed_events(consumer_name, event_id) ON DELETE RESTRICT
);

CREATE TABLE parked_events (
    consumer_name TEXT NOT NULL CHECK (char_length(consumer_name) BETWEEN 1 AND 128),
    event_id TEXT NOT NULL CHECK (char_length(event_id) BETWEEN 1 AND 128),
    subject TEXT NOT NULL CHECK (char_length(subject) BETWEEN 1 AND 128),
    reason_code TEXT NOT NULL CHECK (char_length(reason_code) BETWEEN 1 AND 64),
    delivery_count BIGINT NOT NULL CHECK (delivery_count > 0),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    parked_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (consumer_name, event_id)
);

-- +goose Down
DROP TABLE parked_events;
DROP TABLE event_receipts;
DROP TABLE processed_events;
