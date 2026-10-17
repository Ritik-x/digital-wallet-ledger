ALTER TABLE users
ADD COLUMN role TEXT NOT NULL DEFAULT 'customer'
CHECK (role IN ('customer', 'support', 'admin'));