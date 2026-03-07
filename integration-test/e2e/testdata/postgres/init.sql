-- Simple test data for hugr e2e tests
CREATE TABLE IF NOT EXISTS products (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    price DECIMAL(10,2) NOT NULL,
    active BOOLEAN DEFAULT true
);

INSERT INTO products (name, price, active) VALUES
    ('Widget A', 10.99, true),
    ('Widget B', 24.50, true),
    ('Widget C', 5.00, false);
