# Timesheet App Dockerfile
# Multi-stage build for minimal image size and security

# ============================================================================
# Stage 1: Builder - Install dependencies
# ============================================================================
FROM python:3.11-slim as builder

WORKDIR /app

# Install build dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    gcc \
    && rm -rf /var/lib/apt/lists/*

# Copy only requirements first (layer caching optimization)
COPY requirements.txt .

# Install Python packages to user directory
RUN pip install --user --no-cache-dir -r requirements.txt

# ============================================================================
# Stage 2: Runtime - Minimal production image
# ============================================================================
FROM python:3.11-slim

WORKDIR /app

# Create non-root user for security
RUN useradd -m -u 1000 appuser && \
    mkdir -p /data && \
    chown -R appuser:appuser /app /data

# Copy Python packages from builder stage to a location accessible by appuser
COPY --from=builder --chown=appuser:appuser /root/.local /home/appuser/.local

# Copy application code
COPY --chown=appuser:appuser src/ ./src/
COPY --chown=appuser:appuser migrations/ ./migrations/

# Switch to non-root user
USER appuser

# Ensure Python packages are in PATH
ENV PATH=/home/appuser/.local/bin:$PATH

# Disable Python output buffering (important for Docker logs)
ENV PYTHONUNBUFFERED=1

# Volume mount point for persistent data
VOLUME /data

# Default environment variables (can be overridden)
ENV DATABASE_PATH=/data/timesheet.db

# Expose application port
EXPOSE 8000

# Set working directory to src for proper imports
WORKDIR /app/src

# Health check - verifies app is running and database is accessible
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
  CMD python -c "import urllib.request; urllib.request.urlopen('http://localhost:8000/health').read()"

# Start the application
# Note: Migrations run automatically in main.py on startup
CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
