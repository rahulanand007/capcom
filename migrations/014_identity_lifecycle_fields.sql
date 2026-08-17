ALTER TABLE users ADD COLUMN IF NOT EXISTS email_verified_at timestamptz;
ALTER TABLE password_credentials ADD COLUMN IF NOT EXISTS rehash_after timestamptz;
