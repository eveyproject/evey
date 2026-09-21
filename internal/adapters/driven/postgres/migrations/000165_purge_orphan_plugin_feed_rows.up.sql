-- Home rows that outlived their plugin.
--
-- A home widget is placed by a "plugin:<id>" row in feed_admin_config and in
-- each profile's feed_profile_config. Deleting a library removed its
-- "library:<id>" rows; uninstalling a plugin removed nothing, so every
-- reinstall of a widget (a new id each time) left the previous rows behind.
-- The homes then rendered those ghosts: on a phone, five of them put 80 dp of
-- empty space above the first real widget. The server now deletes the rows
-- on uninstall; this sweeps what the years left.
DELETE FROM feed_profile_config
 WHERE feed_type LIKE 'plugin:%'
   AND substr(feed_type, length('plugin:') + 1) NOT IN (SELECT id FROM plugins);
DELETE FROM feed_admin_config
 WHERE feed_type LIKE 'plugin:%'
   AND substr(feed_type, length('plugin:') + 1) NOT IN (SELECT id FROM plugins);
