-- Migration: Add password reset fields to users table
-- Created: 2025-09-09
-- Description: Adds password reset token, expiration, and admin reset flag to users table

ALTER TABLE users 
ADD COLUMN password_reset_token VARCHAR(255) NULL,
ADD COLUMN password_reset_expires TIMESTAMP NULL,
ADD COLUMN password_reset_by_admin BOOLEAN DEFAULT FALSE;

-- Add index for faster token lookups
CREATE INDEX idx_users_password_reset_token ON users(password_reset_token);
CREATE INDEX idx_users_password_reset_expires ON users(password_reset_expires);
