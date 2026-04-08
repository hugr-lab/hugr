-- Add subscription_root column to _schema_modules for GraphQL Subscription support.
ALTER TABLE _schema_modules ADD COLUMN IF NOT EXISTS subscription_root VARCHAR;

-- Set subscription_root on the root module (empty name) to the Subscription system type.
UPDATE _schema_modules SET subscription_root = 'Subscription' WHERE name = '';

UPDATE "version" SET "version" = '0.0.17';
