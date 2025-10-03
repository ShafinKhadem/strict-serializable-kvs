#!/bin/bash
# Script to run experiments and collect results for graphing (CloudLab)

OUTPUT_DIR="experiment-results"
mkdir -p "$OUTPUT_DIR"

# Make run-cluster.sh executable
chmod +x ./run-cluster.sh 2>/dev/null || true

echo "Running CLOUDLAB experiments and collecting data..."
echo ""

# Experiment 1: Throughput vs. Number of Servers (scaling)
echo "=== Experiment 1: Throughput Scaling ==="
echo "servers,clients,commits_per_sec" > "$OUTPUT_DIR/throughput-scaling.csv"

for servers in 1 2 3 4; do
    clients=$servers
    echo "Running: $servers servers, $clients clients..."

    # Run experiment and save full output for debugging
    output=$(./run-cluster.sh $servers $clients "" "-workload YCSB-B -secs 30" 2>&1)

    # Show last 10 lines for debugging
    echo "--- Last 10 lines of output ---"
    echo "$output" | tail -10
    echo "-------------------------------"

    # Extract total commits/s from output (format: "total XXX commit/s")
    commits=$(echo "$output" | grep -E "^total" | awk '{print $2}')

    if [ -z "$commits" ]; then
        echo "  Warning: No 'total' line found, trying alternate patterns..."
        # Try case-insensitive
        commits=$(echo "$output" | grep -iE "total.*commit" | awk '{print $2}')
    fi

    if [ -z "$commits" ]; then
        echo "  ERROR: Could not extract commits/s"
        echo "  Full output saved to: /tmp/experiment-debug-${servers}s.txt"
        echo "$output" > "/tmp/experiment-debug-${servers}s.txt"
        continue
    fi

    echo "$servers,$clients,$commits" >> "$OUTPUT_DIR/throughput-scaling.csv"
    echo "  ✓ Result: $commits commits/s"
    sleep 5
done

# Experiment 2: Contention Analysis (theta variation)
echo ""
echo "=== Experiment 2: Contention Analysis ==="
echo "theta,commits_per_sec" > "$OUTPUT_DIR/contention-analysis.csv"

for theta in 0.0 0.25 0.5 0.75 0.99; do
    echo "Running: theta=$theta..."
    output=$(./run-cluster.sh 2 1 "" "-workload YCSB-B -theta $theta -secs 30" 2>&1)

    echo "--- Last 10 lines of output ---"
    echo "$output" | tail -10
    echo "-------------------------------"

    commits=$(echo "$output" | grep -E "^total" | awk '{print $2}')

    if [ -z "$commits" ]; then
        echo "  Warning: No 'total' line found, trying alternate patterns..."
        commits=$(echo "$output" | grep -iE "total.*commit" | awk '{print $2}')
    fi

    if [ -z "$commits" ]; then
        echo "  ERROR: Could not extract commits/s"
        echo "  Full output saved to: /tmp/experiment-debug-theta${theta}.txt"
        echo "$output" > "/tmp/experiment-debug-theta${theta}.txt"
        continue
    fi

    echo "$theta,$commits" >> "$OUTPUT_DIR/contention-analysis.csv"
    echo "  ✓ Result: $commits commits/s"
    sleep 5
done

# Experiment 3: Commit vs Abort Rates
echo ""
echo "=== Experiment 3: Commit vs Abort Rates ==="
echo "config,commits_per_sec,aborts_per_sec" > "$OUTPUT_DIR/commit-abort-rates.csv"

# Different configurations
configs=(
    "1,1,Single-Server"
    "2,2,2-Servers"
    "3,3,3-Servers"
    "4,4,4-Servers"
)

for config in "${configs[@]}"; do
    IFS=',' read -r servers clients label <<< "$config"
    echo "Running: $label..."
    output=$(./run-cluster.sh $servers $clients "" "-workload YCSB-B -secs 30" 2>&1)

    echo "--- Last 10 lines of output ---"
    echo "$output" | tail -10
    echo "-------------------------------"

    # Extract commits (format: "total XXX commit/s")
    commits=$(echo "$output" | grep -E "^total" | awk '{print $2}')

    if [ -z "$commits" ]; then
        echo "  Warning: No 'total' line found, trying alternate patterns..."
        commits=$(echo "$output" | grep -iE "total.*commit" | awk '{print $2}')
    fi

    # Extract aborts from server logs after the run
    # Server logs are in logs/latest/kvsserver-*.log
    # Format: "abort/s X.XX"
    aborts=$(grep "abort/s" logs/latest/kvsserver-*.log 2>/dev/null | tail -${servers} | awk '{sum += $2} END {printf "%.1f", sum/'${servers}'}')

    if [ -z "$aborts" ]; then
        aborts="0"
    fi

    if [ -z "$commits" ]; then
        echo "  ERROR: Could not extract commits/s"
        echo "  Full output saved to: /tmp/experiment-debug-${label}.txt"
        echo "$output" > "/tmp/experiment-debug-${label}.txt"
        continue
    fi

    echo "$label,$commits,$aborts" >> "$OUTPUT_DIR/commit-abort-rates.csv"
    echo "  ✓ Result: $commits commits/s, $aborts aborts/s"
    sleep 5
done

echo ""
echo "Data collection complete! Results saved to $OUTPUT_DIR/"
echo ""
echo "CSV files created:"
ls -lh "$OUTPUT_DIR"/*.csv
echo ""
echo "Preview of data:"
for csv in "$OUTPUT_DIR"/*.csv; do
    echo "=== $(basename $csv) ==="
    cat "$csv"
    echo ""
done
echo ""
echo "Run './generate-graphs.py' to create visualizations"
echo ""
echo "If experiments failed, check debug files in /tmp/experiment-debug-*.txt"
