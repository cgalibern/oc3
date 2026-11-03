-- 2026-11-03: an index on the snooze deadline of the nodes and of the
-- services, and on the last update of a service. The alerting and the
-- dashboards select the objects whose snooze is over or still running, and the
-- objects updated since a date: without these keys each such query scanned the
-- table whole. Created by hand on some databases before this migration, under
-- the same names: IF NOT EXISTS leaves them as they are. Built in place, the
-- tables kept writable meanwhile.
ALTER TABLE nodes ADD INDEX IF NOT EXISTS k_snooze_till (snooze_till), ALGORITHM=INPLACE, LOCK=NONE;
ALTER TABLE services ADD INDEX IF NOT EXISTS k_svc_snooze_till (svc_snooze_till), ALGORITHM=INPLACE, LOCK=NONE;
ALTER TABLE services ADD INDEX IF NOT EXISTS k_updated (updated), ALGORITHM=INPLACE, LOCK=NONE;
