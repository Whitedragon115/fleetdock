-- Sample data for the public demo's "billing" database (MySQL).
SET SESSION cte_max_recursion_depth = 100000;

CREATE TABLE plans (
  id               varchar(20) PRIMARY KEY,
  name             varchar(50) NOT NULL,
  price_cents      int NOT NULL,
  billing_interval varchar(10) NOT NULL DEFAULT 'month'
);
CREATE TABLE subscriptions (
  id          bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  account     varchar(20) NOT NULL,
  plan_id     varchar(20) NOT NULL,
  status      varchar(20) NOT NULL,
  started_at  datetime NOT NULL,
  renews_at   datetime,
  CONSTRAINT subscriptions_plan_fk FOREIGN KEY (plan_id) REFERENCES plans (id)
);
CREATE TABLE invoices (
  id              bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  subscription_id bigint unsigned NOT NULL,
  amount_cents    int NOT NULL,
  status          varchar(20) NOT NULL,
  issued_at       datetime NOT NULL,
  CONSTRAINT invoices_subscription_fk FOREIGN KEY (subscription_id) REFERENCES subscriptions (id)
);

INSERT INTO plans VALUES ('starter', 'Starter', 900, 'month'), ('team', 'Team', 4900, 'month'),
  ('business', 'Business', 19900, 'month'), ('enterprise', 'Enterprise', 99000, 'year');

INSERT INTO subscriptions (account, plan_id, status, started_at, renews_at)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 600)
SELECT CONCAT('acct_', LPAD(g, 5, '0')),
       ELT(1 + g % 5, 'starter', 'team', 'team', 'business', 'enterprise'),
       ELT(1 + g % 5, 'active', 'active', 'active', 'past_due', 'canceled'),
       NOW() - INTERVAL g DAY,
       NOW() + INTERVAL (g % 30) DAY
FROM seq;

INSERT INTO invoices (subscription_id, amount_cents, status, issued_at)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 4000)
SELECT 1 + g % 600,
       ELT(1 + g % 4, 900, 4900, 19900, 99000),
       ELT(1 + g % 5, 'paid', 'paid', 'paid', 'open', 'void'),
       NOW() - INTERVAL (g % 365) DAY
FROM seq;
