-- Synthetic Magento-shaped seed fixture (no PII). Clean-room for MIGRATE-01 offline proof.
CREATE TABLE IF NOT EXISTS magelift_seed_probe (
  id INT NOT NULL PRIMARY KEY,
  label VARCHAR(64) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO magelift_seed_probe (id, label) VALUES (1, 'tiny-fixture')
  ON DUPLICATE KEY UPDATE label = VALUES(label);

CREATE TABLE IF NOT EXISTS store (
  store_id SMALLINT UNSIGNED NOT NULL PRIMARY KEY,
  code VARCHAR(32) NOT NULL,
  name VARCHAR(64) NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

INSERT INTO store (store_id, code, name) VALUES (1, 'default', 'Default Store')
  ON DUPLICATE KEY UPDATE name = VALUES(name);
