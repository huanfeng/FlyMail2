#!/bin/bash

# Configuration
# You can override these by setting them before running the script
# e.g. PORT=9090 ./start_dev.sh

export PORT=${PORT:-8080}
export FRONTEND_HOST=${FRONTEND_HOST:-::}
export FRONTEND_PORT=${FRONTEND_PORT:-8008}
export API_HOST=${API_HOST:-http://localhost:$PORT}
export CORS_ORIGINS=${CORS_ORIGINS:-"http://localhost:$FRONTEND_PORT"}

# Frontend VITE config
# Vite picks up VITE_ prefixed variables automatically
export VITE_API_BASE_URL="/api"

echo "========================================"
echo "Starting Mail2IM Development Environment"
echo "========================================"
echo "Backend Port:  $PORT"
echo "Frontend Port: $FRONTEND_PORT"
echo "API URL:       $VITE_API_BASE_URL"
echo "CORS Origins:  $CORS_ORIGINS"
echo "========================================"

# Start Backend
echo "[Backend] Starting..."
cd backend

# Get Version Info
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE=$(date "+%Y-%m-%d %H:%M:%S")
LDFLAGS="-X 'main.AppVersion=$VERSION' -X 'main.GitCommit=$COMMIT' -X 'main.BuildTime=$DATE'"

go run -ldflags "$LDFLAGS" cmd/server/main.go &
BACKEND_PID=$!

# Wait for Backend to be ready
echo "[Backend] Waiting for port $PORT..."
MAX_RETRIES=30
COUNT=0
while ! timeout 1 bash -c "echo > /dev/tcp/127.0.0.1/$PORT" 2>/dev/null; do
    sleep 1
    COUNT=$((COUNT+1))
    if [ $COUNT -ge $MAX_RETRIES ]; then
        echo "[Error] Backend failed to start within $MAX_RETRIES seconds."
        kill $BACKEND_PID 2>/dev/null
        exit 1
    fi
    # Check if process is still running
    if ! ps -p $BACKEND_PID > /dev/null; then
        echo "[Error] Backend process died unexpectedly."
        exit 1
    fi
done

echo "[Backend] is ready!"

# Start Frontend
echo "[Frontend] Starting..."
cd ../frontend
# Force the port for vite
pnpm dev --port $FRONTEND_PORT --host $FRONTEND_HOST &
FRONTEND_PID=$!

# Cleanup function to kill both processes on Ctrl+C
cleanup() {
    echo ""
    echo "Stopping services..."
    kill $BACKEND_PID 2>/dev/null
    kill $FRONTEND_PID 2>/dev/null
    exit
}

trap cleanup SIGINT SIGTERM

# Wait for processes
wait
