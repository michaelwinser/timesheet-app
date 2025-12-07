# ABANDONED: Multi-User Support via Database-Per-User

**Status**: Abandoned
**Date**: 2025-12-07
**Reason**: Technical debt and operational complexity outweigh benefits

## Approach

Each user gets their own SQLite database file, isolated by email hash:
- Database path: `timesheet-data/{sha256(email)[:16]}.db`
- Middleware creates/attaches user-specific DB per request
- Session stores user email after OAuth login
- All routes use `request.state.db` for database access

## Why We Tried This

- **Simple user isolation** - Separate files = complete data separation
- **No schema changes** - Existing single-user schema works as-is
- **Easy to backup individual users** - Just copy their .db file
- **SQLite-friendly** - No need for user_id columns everywhere

## What Went Wrong

### 1. Migration System Became a Performance Problem

```python
# This runs on EVERY HTTP REQUEST
for migration_file in sorted(migrations_dir.glob("*.sql")):
    try:
        request.state.db.execute_script(f.read())
    except Exception:
        pass  # Silent failure - could hide real errors
```

**Problems**:
- Disk I/O on every request to read migration files
- Execute DDL statements on every request (even if idempotent)
- Silent error swallowing could mask real migration failures
- No tracking of which migrations have been applied
- Scales poorly as migrations accumulate

**Better approach**: Track migrations per database, only run once per DB

### 2. OAuth Implementation Gotchas

**Lesson Learned**: Google OAuth requires specific scopes for ID tokens

```python
# WRONG - Missing openid scope, credentials.id_token is None
GOOGLE_SCOPES = [
    "https://www.googleapis.com/auth/calendar.readonly",
]

# CORRECT - Need OpenID Connect scopes for user info
GOOGLE_SCOPES = [
    "openid",  # Required for ID token
    "https://www.googleapis.com/auth/userinfo.email",
    "https://www.googleapis.com/auth/userinfo.profile",
    "https://www.googleapis.com/auth/calendar.readonly",
]
```

**Also learned**: Extract user info from ID token, not userinfo API
```python
# Better: Use ID token (already have it)
id_info = id_token.verify_oauth2_token(
    credentials.id_token,
    google.auth.transport.requests.Request(),
    config.GOOGLE_CLIENT_ID
)
email = id_info["email"]
```

### 3. Middleware Ordering Is Critical

```python
# WRONG - SessionMiddleware not available when DatabaseMiddleware runs
app.add_middleware(SessionMiddleware, ...)
app.add_middleware(DatabaseMiddleware)

# CORRECT - Middleware runs in reverse order of addition
app.add_middleware(DatabaseMiddleware)  # Add first, runs last
app.add_middleware(SessionMiddleware, ...)  # Add last, runs first
```

**Lesson**: Middleware with dependencies (DatabaseMiddleware needs session) must be added in reverse order.

### 4. Operational Challenges

**Debugging**:
- Hashed filenames (`8a7f3b2c1d.db`) hard to map to users
- No logging of which user's DB is being used
- Can't easily inspect user data without email->hash lookup

**Backup/Recovery**:
- Need to backup potentially thousands of small files
- Restoring single user requires knowing their hash
- No easy way to list all users

**Monitoring**:
- Can't easily count total users (need to scan directory)
- Disk space used per user not tracked
- No way to identify inactive user databases

**Migrations**:
- Every user DB needs migrations applied
- New migrations run on first request after deployment (slow)
- No way to verify all users are on latest schema
- Rolling back a migration requires touching every DB file

## Alternative Approaches to Consider

### Option 1: Single Database with user_id Column

**Pros**:
- Standard approach, well-understood patterns
- Single migration path for all users
- Easy to query across users (analytics, admin)
- Simple backup/restore
- Better performance (one connection pool)

**Cons**:
- Requires schema changes (add user_id everywhere)
- Need to be careful with query filters (don't leak data)
- Foreign key constraints need user_id

**Migration path**: Add `user_id` column to all tables, update all queries

### Option 2: PostgreSQL with Row-Level Security

**Pros**:
- Database enforces isolation automatically
- Single database, single schema
- Can still query across users as admin
- Industry standard for multi-tenant apps

**Cons**:
- Requires PostgreSQL (more infrastructure)
- More complex than SQLite
- Overkill for small deployments

### Option 3: Hybrid - Separate DBs, Better Tooling

**Pros**:
- Keep data isolation benefits
- Address operational issues with tooling

**Cons**:
- Still have migration complexity
- Still hard to query across users
- More moving parts

## Key Takeaways

1. **Don't sacrifice operational simplicity for implementation simplicity**
   - Easy to code ≠ easy to operate
   - Running migrations on every request is unacceptable

2. **Test the full OAuth flow early**
   - Don't assume scopes, verify them
   - ID token verification is simpler than userinfo API

3. **Document middleware ordering requirements**
   - Reverse order is confusing
   - Add comments explaining dependencies

4. **Consider the operational lifecycle**
   - How do you backup?
   - How do you monitor?
   - How do you debug in production?
   - How do you handle schema migrations?

5. **When using SQLite for multi-user apps**
   - Database-per-user works for very small scale (< 100 users)
   - Consider single DB with user_id for anything larger
   - Or just use PostgreSQL from the start

## Recommendation for Next Attempt

Use **Option 1: Single Database with user_id**:

1. Add `user_id TEXT NOT NULL` to all tables
2. Add foreign key to users table
3. Update all queries to filter by user_id
4. Use middleware to inject user_id from session
5. Keep using SQLite (works fine with proper schema)

This is the simplest path forward that addresses all the operational concerns while maintaining reasonable performance and simplicity.

## Files Modified (To Be Reverted)

- `src/db.py` - Added `get_user_db_path()`
- `src/main.py` - Added `DatabaseMiddleware`
- `src/config.py` - Added OAuth scopes
- `src/routes/*.py` - Changed to use `request.state.db`
- `src/services/*.py` - Accept `db` parameter
- `requirements.txt` - Added `itsdangerous`
- `src/templates/base.html` - Added logout UI
- `src/static/css/style.css` - Added logout styles

Note: The OAuth fixes and logout UI should be kept.
