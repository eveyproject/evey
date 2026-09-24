ALTER TABLE syncplay_rooms DROP CONSTRAINT IF EXISTS fk_syncplay_rooms_host_id;

-- Back to a profile: the account's default one, else its oldest.
UPDATE syncplay_rooms r SET host_id = (
    SELECT p.id FROM profiles p WHERE p.user_id = r.host_id
    ORDER BY p.is_default DESC, p.created_at ASC LIMIT 1
) WHERE r.host_id IN (SELECT id FROM users);
DELETE FROM syncplay_rooms WHERE host_id IS NULL OR host_id NOT IN (SELECT id FROM profiles);

ALTER TABLE syncplay_rooms
    ADD CONSTRAINT fk_syncplay_rooms_host_id
    FOREIGN KEY (host_id) REFERENCES profiles(id) ON DELETE CASCADE;
