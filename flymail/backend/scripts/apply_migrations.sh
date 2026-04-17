#!/bin/bash

# Script to apply SQL migrations to the database

DB_PATH="${DB_PATH:-./data/flymail.db}"
MIGRATIONS_DIR="${MIGRATIONS_DIR:-./migrations}"

echo "Applying migrations from $MIGRATIONS_DIR to $DB_PATH"

# Check if database exists
if [ ! -f "$DB_PATH" ]; then
    echo "Error: Database file not found at $DB_PATH"
    exit 1
fi

# Check if migrations directory exists
if [ ! -d "$MIGRATIONS_DIR" ]; then
    echo "Error: Migrations directory not found at $MIGRATIONS_DIR"
    exit 1
fi

# Apply all SQL migrations in order
for migration in "$MIGRATIONS_DIR"/*.sql; do
    if [ -f "$migration" ]; then
        echo "Applying migration: $(basename "$migration")"
        sqlite3 "$DB_PATH" < "$migration"
        if [ $? -eq 0 ]; then
            echo "  ✓ Success"
        else
            echo "  ✗ Failed"
            exit 1
        fi
    fi
done

echo "All migrations applied successfully!"