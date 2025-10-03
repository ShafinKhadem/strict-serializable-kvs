#!/bin/bash
# Master script to run all experiments and generate graphs

set -e  # Exit on error

echo "=========================================="
echo "  Performance Experiment Suite"
echo "=========================================="
echo ""

# Step 1: Check dependencies
echo "Checking dependencies..."
if ! command -v python3 &> /dev/null; then
    echo "Error: python3 not found. Please install Python 3."
    exit 1
fi

if ! python3 -c "import matplotlib" 2>/dev/null; then
    echo "Error: matplotlib not found."
    echo "Install with: pip3 install matplotlib"
    exit 1
fi

# Auto-detect environment (CloudLab vs local)
if command -v /usr/local/etc/emulab/tmcc &> /dev/null; then
    echo "✓ CloudLab environment detected"
    RUN_SCRIPT="./run-cluster.sh"
    chmod +x "$RUN_SCRIPT" 2>/dev/null || true
else
    echo "✓ Local environment detected"
    RUN_SCRIPT="./run-local.sh"
fi

if [ ! -f "$RUN_SCRIPT" ]; then
    echo "Error: $RUN_SCRIPT not found."
    echo "Make sure you're running this from the project root."
    exit 1
fi

echo "Using: $RUN_SCRIPT"

echo "✓ All dependencies found"
echo ""

# Step 2: Collect experiment data
echo "=========================================="
echo "  Running Experiments"
echo "=========================================="

# Use appropriate collection script
if [ "$RUN_SCRIPT" = "./run-cluster.sh" ]; then
    ./collect-results.sh
else
    ./collect-results-local.sh
fi

# Step 3: Generate graphs
echo ""
echo "=========================================="
echo "  Generating Graphs"
echo "=========================================="
./generate-graphs.py

echo ""
echo "=========================================="
echo "  Complete!"
echo "=========================================="
echo "Graphs are available in ./graphs/"
echo ""
echo "Next steps:"
echo "  1. Review graphs in ./graphs/"
echo "  2. Update README.md Performance Graphs section"
echo "  3. Commit graphs to repository: git add graphs/"
