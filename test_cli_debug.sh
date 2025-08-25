#!/bin/bash

echo "Testing CLI debug toggle..."
echo "=============================="

# Start daemon in background
./bin/testdaemon -config config-clickhouse.yaml -cli-only &
DAEMON_PID=$!

# Wait for daemon to start and CLI server to be ready
sleep 4

# Check if daemon is running
if ps -p $DAEMON_PID > /dev/null; then
    echo "Daemon started with PID $DAEMON_PID"
    echo ""
    
    # Test the debug toggle via telnet
    echo "Testing debug toggle commands..."
    echo "================================"
    
    (
        echo "debug status"
        sleep 1
        echo "debug"
        sleep 1
        echo "debug status"
        sleep 1
        echo "debug"
        sleep 1
        echo "debug status"
        sleep 1
        echo "debug on"
        sleep 1
        echo "debug status"
        sleep 1
        echo "debug off"
        sleep 1
        echo "debug status"
        sleep 1
        echo "exit"
    ) | nc localhost 2323
    
    # Kill the daemon
    kill $DAEMON_PID 2>/dev/null
    wait $DAEMON_PID 2>/dev/null
    echo ""
    echo "Daemon stopped"
else
    echo "Daemon failed to start"
fi

echo "=============================="
echo "Test complete"