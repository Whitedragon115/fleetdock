-- Sample data for the public demo's "analytics" database (MySQL).
SET SESSION cte_max_recursion_depth = 100000;

CREATE TABLE events (
  id          bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  occurred_at datetime NOT NULL,
  session_id  char(32) NOT NULL,
  name        varchar(50) NOT NULL,
  path        varchar(200) NOT NULL,
  country     char(2),
  duration_ms int,
  KEY events_occurred_idx (occurred_at)
);
CREATE TABLE daily_active_users (
  day   date PRIMARY KEY,
  users int NOT NULL
);

INSERT INTO events (occurred_at, session_id, name, path, country, duration_ms)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 50000)
SELECT NOW() - INTERVAL ((g * 7) % 20160) MINUTE,
       LEFT(SHA2(FLOOR(g / 8), 256), 32),
       ELT(1 + g % 8, 'page_view', 'page_view', 'page_view', 'click', 'signup', 'checkout_started', 'checkout_completed', 'search'),
       ELT(1 + FLOOR(g / 3) % 8, '/', '/pricing', '/docs', '/docs/getting-started', '/blog', '/features', '/login', '/signup'),
       ELT(1 + g % 10, 'US', 'DE', 'GB', 'FR', 'NL', 'SE', 'CA', 'AU', 'JP', 'BR'),
       100 + (g * 53) % 9000
FROM seq;

INSERT INTO daily_active_users (day, users)
WITH RECURSIVE days (d) AS (SELECT CURDATE() - INTERVAL 90 DAY UNION ALL SELECT d + INTERVAL 1 DAY FROM days WHERE d < CURDATE())
SELECT d, 1200 + (DAYOFYEAR(d) * 37) % 800 FROM days;
