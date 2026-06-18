-- create events table
CREATE TABLE IF NOT EXISTS pulse.pulse_events (
    event_id UUID,
    event_name String,
    user_id String,
    timestamp DateTime64(3, 'UTC'),
    request_id String,
    properties Map(String, String)
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (event_name, timestamp, user_id);
