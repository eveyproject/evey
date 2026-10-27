-- The device that starts a pairing may declare it will send the secret back
-- when it collects the tokens; the check then demands it for that code only.
ALTER TABLE quick_connect_requests
    ADD COLUMN IF NOT EXISTS secret_required BOOLEAN NOT NULL DEFAULT FALSE;
