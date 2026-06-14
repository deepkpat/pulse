-- create api_keys table
CREATE TABLE IF NOT EXISTS api_keys (
    key VARCHAR(32) PRIMARY KEY,
    description VARCHAR(256) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- seed with the development key
INSERT INTO api_keys (key, description)
VALUES ('your-secret-api-key-here', 'development API key')
ON CONFLICT (key) DO NOTHING;
