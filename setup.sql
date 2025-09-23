-- PostgreSQL setup script for Gyroskop Bot
-- Run this script to set up the database and user for the Gyroskop bot

-- Connect as postgres superuser and run these commands:

-- Create database
CREATE DATABASE gyroskop;

-- Create user
CREATE USER gyroskop WITH PASSWORD 'gyroskop123';

-- Grant privileges
GRANT ALL PRIVILEGES ON DATABASE gyroskop TO gyroskop;

-- Connect to the gyroskop database and grant schema privileges
\c gyroskop;
GRANT ALL ON SCHEMA public TO gyroskop;
GRANT CREATE ON SCHEMA public TO gyroskop;

-- The tables will be created automatically by the bot on first run