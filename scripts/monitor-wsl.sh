#!/bin/bash
# Lightweight WSL2 memory monitor — logs to ~/wsl-monitor.log
# Usage: ./scripts/monitor-wsl.sh &
# After a crash, check ~/wsl-monitor.log to see what ate the RAM

LOG=~/wsl-monitor.log
INTERVAL=30

echo "=== WSL monitor started $(date) ===" >> "$LOG"

while true; do
  {
    echo "--- $(date '+%H:%M:%S') ---"
    free -h | head -2
    # top 5 processes by RSS
    ps aux --sort=-%mem | awk 'NR<=6{printf "%-6s %-8s %s\n", $4"%", $6"K", $11}'
    # OOM killer events since last check
    dmesg -T 2>/dev/null | tail -5 | grep -i "oom\|killed" || true
  } >> "$LOG" 2>&1
  sleep "$INTERVAL"
done
