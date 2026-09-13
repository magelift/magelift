-- Known Magento application-integrity fixture. Applied after every live dump
-- import; tiny.sql already includes the same table for offline dumpimport tests.
-- HA cells read magelift_seed_probe.label through Magento's resource layer.
CREATE TABLE IF NOT EXISTS magelift_seed_probe (
  id INT NOT NULL PRIMARY KEY,
  label VARCHAR(64) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO magelift_seed_probe (id, label) VALUES (1, 'tiny-fixture')
  ON DUPLICATE KEY UPDATE label = VALUES(label);
