-- Preserve the compatibility column while making every existing account a learner.
UPDATE users SET is_admin = false WHERE is_admin;
