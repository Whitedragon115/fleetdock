-- Sample data for the public demo's "shop" database (MySQL). Loaded on the
-- sandbox's first start and again on every reset.
SET SESSION cte_max_recursion_depth = 100000;

CREATE TABLE customers (
  id          bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  email       varchar(255) NOT NULL UNIQUE,
  full_name   varchar(200) NOT NULL,
  country     char(2) NOT NULL,
  marketing   boolean NOT NULL DEFAULT false,
  created_at  datetime NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE TABLE products (
  id          bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  sku         varchar(32) NOT NULL UNIQUE,
  name        varchar(200) NOT NULL,
  category    varchar(50) NOT NULL,
  price_cents int NOT NULL CHECK (price_cents >= 0),
  stock       int NOT NULL DEFAULT 0,
  active      boolean NOT NULL DEFAULT true,
  attributes  json
);
CREATE TABLE orders (
  id           bigint unsigned AUTO_INCREMENT PRIMARY KEY,
  customer_id  bigint unsigned NOT NULL,
  status       varchar(20) NOT NULL DEFAULT 'pending',
  total_cents  int NOT NULL,
  placed_at    datetime NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY orders_placed_idx (placed_at),
  CONSTRAINT orders_customer_fk FOREIGN KEY (customer_id) REFERENCES customers (id)
);
CREATE TABLE order_items (
  order_id    bigint unsigned NOT NULL,
  product_id  bigint unsigned NOT NULL,
  quantity    int NOT NULL,
  price_cents int NOT NULL,
  PRIMARY KEY (order_id, product_id),
  CONSTRAINT order_items_order_fk FOREIGN KEY (order_id) REFERENCES orders (id) ON DELETE CASCADE,
  CONSTRAINT order_items_product_fk FOREIGN KEY (product_id) REFERENCES products (id)
);
CREATE VIEW revenue_by_day AS
  SELECT DATE(placed_at) AS day, count(*) AS orders, sum(total_cents) / 100.0 AS revenue
  FROM orders WHERE status <> 'cancelled' GROUP BY DATE(placed_at);

INSERT INTO customers (email, full_name, country, marketing, created_at)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 2400)
SELECT LOWER(CONCAT(f, '.', l, g, '@', ELT(1 + g % 5, 'gmail.com', 'outlook.com', 'proton.me', 'fastmail.com', 'icloud.com'))),
       CONCAT(f, ' ', l),
       ELT(1 + g % 10, 'US', 'DE', 'GB', 'FR', 'NL', 'SE', 'CA', 'AU', 'JP', 'BR'),
       g % 3 = 0,
       NOW() - INTERVAL g HOUR
FROM (SELECT g,
             ELT(1 + g % 15, 'Ada', 'Grace', 'Linus', 'Margaret', 'Ken', 'Barbara', 'Dennis', 'Frances', 'Alan', 'Radia', 'Guido', 'Sophie', 'Yukihiro', 'Anders', 'Hedy') AS f,
             ELT(1 + FLOOR(g / 15) % 15, 'Lovelace', 'Hopper', 'Torvalds', 'Hamilton', 'Thompson', 'Liskov', 'Ritchie', 'Allen', 'Turing', 'Perlman', 'Rossum', 'Wilson', 'Matsumoto', 'Hejlsberg', 'Lamarr') AS l
      FROM seq) n;

INSERT INTO products (sku, name, category, price_cents, stock, active, attributes)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 180)
SELECT CONCAT('SKU-', LPAD(g, 5, '0')),
       CONCAT(ELT(1 + g % 10, 'Mechanical keyboard', 'USB-C hub', '27" monitor', 'Noise-cancelling headphones', 'Laptop stand', 'Webcam 4K', 'Desk lamp', 'Ergonomic mouse', 'Standing desk', 'Cable kit'), ' ',
              ELT(1 + g % 5, 'Pro', 'Lite', 'Max', 'Mini', 'Plus')),
       ELT(1 + g % 5, 'peripherals', 'displays', 'audio', 'furniture', 'accessories'),
       1500 + (g * 937) % 48000,
       (g * 13) % 240,
       g % 17 <> 0,
       -- A JSON column, for the data browser's JSON viewer and editor.
       JSON_OBJECT(
         'color', ELT(1 + g % 5, 'black', 'white', 'silver', 'graphite', 'blue'),
         'weight_g', 150 + (g * 37) % 4000,
         'warranty_years', 1 + g % 3,
         'tags', CAST(ELT(1 + g % 5, '["wireless", "usb-c"]', '["ergonomic"]', '["4k", "hdr"]', '["bestseller", "new"]', '[]') AS JSON),
         'dimensions_mm', JSON_OBJECT('w', 100 + g % 500, 'h', 20 + g % 300, 'd', 10 + g % 200))
FROM seq;

INSERT INTO orders (customer_id, status, total_cents, placed_at)
WITH RECURSIVE seq (g) AS (SELECT 1 UNION ALL SELECT g + 1 FROM seq WHERE g < 9000)
SELECT 1 + (g * 7) % 2400,
       ELT(1 + g % 10, 'paid', 'paid', 'paid', 'shipped', 'shipped', 'delivered', 'delivered', 'pending', 'refunded', 'cancelled'),
       2500 + (g * 331) % 90000,
       NOW() - INTERVAL ((g * 17) % 43200) MINUTE
FROM seq;

INSERT IGNORE INTO order_items (order_id, product_id, quantity, price_cents)
SELECT o.id, 1 + (o.id * k.k) % 180, 1 + (o.id + k.k) % 3, 1500 + (o.id * k.k * 97) % 48000
FROM orders o CROSS JOIN (SELECT 1 AS k UNION ALL SELECT 2) k;
