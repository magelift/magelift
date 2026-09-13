-- Scrubbed recovery fixture: deterministic Magento-shaped records only.
CREATE TABLE magelift_recovery_probe (id INTEGER PRIMARY KEY, value TEXT NOT NULL);
INSERT INTO magelift_recovery_probe (id, value) VALUES (1, 'known-database-record');
INSERT INTO magelift_recovery_probe (id, value) VALUES (2, 'known-database-record-2');
