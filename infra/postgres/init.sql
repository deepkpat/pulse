-- create api_keys table
CREATE TABLE IF NOT EXISTS api_keys (
    key VARCHAR(64) PRIMARY KEY,
    description VARCHAR(256) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- seed with the development key (SHA-256 hash of 'your-secret-api-key-here')
INSERT INTO api_keys (key, description)
VALUES ('48202b6757f08ed91077eb1c2d4ae38f6db0c09a58bd27767e3da6e80d666632', 'development API key')
ON CONFLICT (key) DO NOTHING;
