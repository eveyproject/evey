-- A room's host is the ACCOUNT, like its members.
--
-- host_id referenced profiles while everything that reads it compares an
-- account id: the room websocket admits room.IsMember(claims.UserID), the hub
-- gives host authority to the sender's user id, members are stored by user id,
-- and a host election writes the oldest member's user id into host_id. So a
-- creator signed in on a profile was refused on their own room (seen on the
-- dev server, 2026-09-24), and an election failed on this constraint.
ALTER TABLE syncplay_rooms DROP CONSTRAINT IF EXISTS fk_syncplay_rooms_host_id;

UPDATE syncplay_rooms r SET host_id = p.user_id FROM profiles p WHERE p.id = r.host_id;
DELETE FROM syncplay_rooms WHERE host_id NOT IN (SELECT id FROM users);

ALTER TABLE syncplay_rooms
    ADD CONSTRAINT fk_syncplay_rooms_host_id
    FOREIGN KEY (host_id) REFERENCES users(id) ON DELETE CASCADE;
