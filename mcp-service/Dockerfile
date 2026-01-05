FROM python:3.12-slim

WORKDIR /app

# Install dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy application
COPY api_client.py server.py ./

# Default to stdio transport (override with MCP_TRANSPORT=http for HTTP mode)
ENV MCP_TRANSPORT=stdio
ENV PYTHONUNBUFFERED=1

CMD ["python", "server.py"]
