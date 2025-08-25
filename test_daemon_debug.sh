#!/bin/bash

echo "Testing testdaemon with -debug flag..."
echo "========================================="

# Start daemon with debug flag in background
./bin/testdaemon -config config-clickhouse.yaml -debug -cli-only &
DAEMON_PID=$!

# Wait for daemon to start
sleep 3

# Check if daemon is running
if ps -p $DAEMON_PID > /dev/null; then
    echo "Daemon started with PID $DAEMON_PID"
    
    # Try to connect and run a test command
    echo -e "test 1:1/19 24.62.212.226 ifcico\nexit" | timeout 10 nc localhost 2323 || echo "Note: CLI interface may not be enabled"
    
    # Kill the daemon
    kill $DAEMON_PID 2>/dev/null
    wait $DAEMON_PID 2>/dev/null
    echo "Daemon stopped"
else
    echo "Daemon failed to start"
fi

echo "========================================="
echo "Test complete"