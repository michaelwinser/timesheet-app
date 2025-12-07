# Docker Deployment Guide

This guide covers deploying the Timesheet App using Docker, both for local development and production deployment on TrueNAS Scale.

## Quick Start (Local Development)

1. **Copy the environment template:**
   ```bash
   cp .env.example .env
   ```

2. **Edit `.env` and fill in your credentials:**
   - Get Google OAuth credentials from [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
   - Generate a secret key: `python -c "import secrets; print(secrets.token_hex(32))"`
   - Optionally add your Anthropic API key for LLM features

3. **Start the application:**
   ```bash
   docker-compose up -d
   ```

4. **Access the app:**
   - Open http://localhost:8000
   - Login with your Google account

5. **View logs:**
   ```bash
   docker-compose logs -f
   ```

6. **Stop the application:**
   ```bash
   docker-compose down
   ```

## Google OAuth Setup

Before deploying, you need to configure Google OAuth:

1. Go to [Google Cloud Console](https://console.cloud.google.com/apis/credentials)
2. Create a new project or select an existing one
3. Enable the Google Calendar API
4. Create OAuth 2.0 credentials:
   - Application type: Web application
   - Authorized redirect URIs:
     - For local dev: `http://localhost:8000/auth/callback`
     - For TrueNAS: `https://timesheet.yourdomain.com/auth/callback`
5. Copy the Client ID and Client Secret to your `.env` file

## TrueNAS Scale Deployment

### Prerequisites

- TrueNAS Scale 22.x or later
- Domain name with reverse proxy configured (recommended)
- Google OAuth credentials with production redirect URI

### Method 1: Docker Compose (Recommended)

1. **SSH to TrueNAS:**
   ```bash
   ssh admin@truenas-ip
   ```

2. **Create application directory:**
   ```bash
   mkdir -p /mnt/pool/apps/timesheet
   cd /mnt/pool/apps/timesheet
   ```

3. **Copy files to TrueNAS:**
   - `docker-compose.prod.yaml`
   - `.env` (with your production credentials)

4. **Edit `docker-compose.prod.yaml`:**
   - Update the host path volume: `/mnt/pool/apps/timesheet/data`
   - If using a pre-built image, update `image:` line with your Docker Hub username

5. **Create data directory:**
   ```bash
   mkdir -p /mnt/pool/apps/timesheet/data
   ```

6. **Deploy:**
   ```bash
   docker-compose -f docker-compose.prod.yaml up -d
   ```

7. **Configure reverse proxy** (nginx, Traefik, or Cloudflare Tunnel) to route:
   - `https://timesheet.yourdomain.com` → `http://truenas-ip:8000`

8. **Verify deployment:**
   ```bash
   docker-compose -f docker-compose.prod.yaml logs -f
   curl http://localhost:8000/health
   ```

### Method 2: TrueNAS Custom App UI

1. **Build and push Docker image:**
   ```bash
   # On your development machine
   docker build -t yourusername/timesheet-app:latest .
   docker push yourusername/timesheet-app:latest
   ```

2. **In TrueNAS UI:**
   - Navigate to **Apps → Discover Apps → Custom App**
   - Fill in:
     - **Application Name:** `timesheet-app`
     - **Image Repository:** `yourusername/timesheet-app`
     - **Image Tag:** `latest`
     - **Container Port:** `8000`
     - **Node Port:** Choose available port (e.g., `30000`)

3. **Add environment variables** (in TrueNAS UI):
   - `GOOGLE_CLIENT_ID`
   - `GOOGLE_CLIENT_SECRET`
   - `OAUTH_REDIRECT_URI`
   - `SECRET_KEY`
   - `ENVIRONMENT=production`
   - `DEBUG=false`

4. **Configure storage:**
   - Add Host Path Volume:
     - **Host Path:** `/mnt/pool/apps/timesheet/data`
     - **Mount Path:** `/data`

5. **Deploy** and access via `http://truenas-ip:30000`

## Environment Variables Reference

### Required

| Variable | Description | Example |
|----------|-------------|---------|
| `GOOGLE_CLIENT_ID` | OAuth client ID from Google Cloud Console | `abc123.apps.googleusercontent.com` |
| `GOOGLE_CLIENT_SECRET` | OAuth client secret | `GOCSPX-xyz789` |
| `OAUTH_REDIRECT_URI` | OAuth callback URL (must match Google Cloud Console) | `https://timesheet.example.com/auth/callback` |
| `SECRET_KEY` | Random secret for session encryption | Generate with `secrets.token_hex(32)` |

### Optional

| Variable | Description | Default |
|----------|-------------|---------|
| `ANTHROPIC_API_KEY` | Anthropic API key for LLM classification | (empty - feature disabled) |
| `ENVIRONMENT` | `development` or `production` | `development` |
| `DEBUG` | Enable debug mode (`true`/`false`) | `true` |
| `LOG_LEVEL` | Logging verbosity | `INFO` |
| `DATABASE_PATH` | SQLite database file path | `/data/timesheet.db` |

## Data Persistence

The SQLite database is stored in a Docker volume:

- **Development:** Named volume `timesheet-data`
  - View: `docker volume inspect timesheet-data`
  - Backup: `docker run --rm -v timesheet-data:/data -v $(pwd):/backup busybox tar czf /backup/timesheet-backup.tar.gz /data`

- **Production (TrueNAS):** Host path `/mnt/pool/apps/timesheet/data`
  - Direct access to database file for backups
  - Use TrueNAS snapshots for automatic backups
  - Or use `sqlite3 timesheet.db .backup backup.db`

## Monitoring and Maintenance

### View Logs

```bash
# Development
docker-compose logs -f

# Production (TrueNAS)
docker-compose -f docker-compose.prod.yaml logs -f

# Or via Docker
docker logs -f timesheet-app
```

### Check Health Status

```bash
# Via Docker inspect
docker inspect --format='{{.State.Health.Status}}' timesheet-app

# Via HTTP
curl http://localhost:8000/health
```

### Update to Latest Version

```bash
# Pull latest image
docker-compose pull

# Restart with new image
docker-compose up -d

# Or for production
docker-compose -f docker-compose.prod.yaml pull
docker-compose -f docker-compose.prod.yaml up -d
```

### Backup Database

```bash
# Copy database file (if using host path)
cp /mnt/pool/apps/timesheet/data/timesheet.db ~/backups/timesheet-$(date +%Y%m%d).db

# Or use SQLite backup command
sqlite3 /mnt/pool/apps/timesheet/data/timesheet.db ".backup /path/to/backup.db"

# Export time entries to CSV (via app UI)
# Navigate to week view → Export button
```

## Troubleshooting

### Container won't start

```bash
# Check logs for errors
docker logs timesheet-app

# Verify environment variables are set
docker exec timesheet-app env | grep GOOGLE

# Check database file permissions
docker exec timesheet-app ls -la /data
```

### OAuth redirect mismatch

- Ensure `OAUTH_REDIRECT_URI` in `.env` EXACTLY matches Google Cloud Console
- Check for trailing slashes, http vs https, port numbers
- Verify reverse proxy is correctly forwarding requests

### Database locked errors

- Ensure only one container instance is running
- Check file permissions: `ls -la /mnt/pool/apps/timesheet/data/`
- SQLite has limited concurrent write support

### Port already in use

```bash
# Change host port in docker-compose.yaml
ports:
  - "8001:8000"  # Changed from 8000:8000
```

### Health check failing

```bash
# Test health endpoint manually
docker exec timesheet-app curl http://localhost:8000/health

# Check database connectivity
docker exec timesheet-app python -c "from src.db import get_db; get_db().execute('SELECT 1')"
```

## Development Tips

### Hot Reload

Uncomment the source volume mount in `docker-compose.yaml`:

```yaml
volumes:
  - timesheet-data:/data
  - ./src:/app/src:ro  # Enable live code reloading
```

Then restart with `--reload` flag:

```bash
docker-compose run --rm -p 8000:8000 timesheet-app uvicorn src.main:app --host 0.0.0.0 --port 8000 --reload
```

### Database Access

```bash
# Open SQLite shell
docker exec -it timesheet-app sqlite3 /data/timesheet.db

# Run SQL query
docker exec timesheet-app sqlite3 /data/timesheet.db "SELECT * FROM projects;"

# Export schema
docker exec timesheet-app sqlite3 /data/timesheet.db .schema > schema.sql
```

### Shell Access

```bash
# Open shell in running container
docker exec -it timesheet-app /bin/bash

# Or start a new container with shell
docker run --rm -it --entrypoint /bin/bash yourusername/timesheet-app
```

## Security Best Practices

1. **Never commit `.env` to version control** - It's in `.gitignore`
2. **Use HTTPS in production** - Configure reverse proxy with Let's Encrypt
3. **Rotate secrets regularly** - Especially `SECRET_KEY` and OAuth credentials
4. **Limit network access** - Use firewall rules to restrict who can access the app
5. **Keep Docker images updated** - Rebuild regularly for security patches
6. **Enable TrueNAS backups** - Snapshot the data volume daily

## Performance Tuning

### Resource Limits

Adjust in `docker-compose.prod.yaml`:

```yaml
deploy:
  resources:
    limits:
      cpus: '2.0'      # Increase for better performance
      memory: 2G       # Increase if handling many events
```

### SQLite Optimization

For better performance with large datasets, consider:

1. Adding indexes (via migrations)
2. Using WAL mode (Write-Ahead Logging)
3. Migrating to PostgreSQL for multi-user scenarios

## Next Steps

- [ ] Set up automated backups (TrueNAS snapshots or cron job)
- [ ] Configure monitoring (UptimeRobot, Grafana, etc.)
- [ ] Set up CI/CD for automatic deployments
- [ ] Enable HTTPS via reverse proxy
- [ ] Configure email notifications (future feature)

## Support

For issues or questions:
- Check the main README.md
- Review design.md for architecture details
- Check BUGS.md for known issues
- See the PRD in docs/prd.md for feature roadmap
