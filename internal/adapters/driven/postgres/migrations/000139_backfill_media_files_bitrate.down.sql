-- Irreversible by design: the pre-migration state was "0 everywhere", and
-- zeroing the column back would also wipe any real value written since.
SELECT 1;
