#!/usr/bin/env python3
"""
Generate performance graphs from experiment results.
Requires: matplotlib
Install: pip3 install matplotlib
"""

import csv
import os
import sys
import matplotlib.pyplot as plt
import matplotlib
matplotlib.use('Agg')  # Use non-interactive backend for server environments

# Configuration
DATA_DIR = "experiment-results"
OUTPUT_DIR = "graphs"
os.makedirs(OUTPUT_DIR, exist_ok=True)

def read_csv(filename):
    """Read CSV file and return data."""
    filepath = os.path.join(DATA_DIR, filename)
    if not os.path.exists(filepath):
        print(f"Warning: {filepath} not found")
        return None

    with open(filepath, 'r') as f:
        reader = csv.DictReader(f)
        return list(reader)

def plot_throughput_scaling():
    """Graph 1: Throughput vs. Number of Servers"""
    data = read_csv("throughput-scaling.csv")
    if not data:
        return

    # Filter out rows with empty values
    filtered_data = [row for row in data if row.get('commits_per_sec', '').strip()]
    if not filtered_data:
        print("Warning: No valid data in throughput-scaling.csv")
        return

    servers = [int(row['servers']) for row in filtered_data]
    commits = [float(row['commits_per_sec']) for row in filtered_data]

    plt.figure(figsize=(10, 6))
    plt.plot(servers, commits, marker='o', linewidth=2, markersize=8, color='#4285f4')
    plt.xlabel('Number of Servers', fontsize=12)
    plt.ylabel('Throughput (commits/s)', fontsize=12)
    plt.title('Throughput Scaling with Cluster Size', fontsize=14, fontweight='bold')
    plt.grid(True, alpha=0.3)
    plt.xticks(servers)

    # Add value labels on points
    for i, (s, c) in enumerate(zip(servers, commits)):
        plt.annotate(f'{c:.0f}', (s, c), textcoords="offset points",
                    xytext=(0,10), ha='center', fontsize=10)

    plt.tight_layout()
    plt.savefig(os.path.join(OUTPUT_DIR, 'throughput-scaling.png'), dpi=300)
    print(f"✓ Generated: {OUTPUT_DIR}/throughput-scaling.png")
    plt.close()

def plot_contention_analysis():
    """Graph 2: Throughput vs. Contention (Theta)"""
    data = read_csv("contention-analysis.csv")
    if not data:
        return

    # Filter out rows with empty values
    filtered_data = [row for row in data if row.get('commits_per_sec', '').strip()]
    if not filtered_data:
        print("Warning: No valid data in contention-analysis.csv")
        return

    theta = [float(row['theta']) for row in filtered_data]
    commits = [float(row['commits_per_sec']) for row in filtered_data]

    plt.figure(figsize=(10, 6))
    plt.plot(theta, commits, marker='s', linewidth=2, markersize=8, color='#ea4335')
    plt.xlabel('Theta (Zipfian Skew Parameter)', fontsize=12)
    plt.ylabel('Throughput (commits/s)', fontsize=12)
    plt.title('Impact of Contention on Throughput', fontsize=14, fontweight='bold')
    plt.grid(True, alpha=0.3)

    # Add value labels
    for i, (t, c) in enumerate(zip(theta, commits)):
        plt.annotate(f'{c:.0f}', (t, c), textcoords="offset points",
                    xytext=(0,10), ha='center', fontsize=10)

    # Add annotation for contention levels
    plt.text(0.1, max(commits) * 0.95, 'Low contention', fontsize=10, style='italic')
    plt.text(0.8, min(commits) * 1.1, 'High contention', fontsize=10, style='italic')

    plt.tight_layout()
    plt.savefig(os.path.join(OUTPUT_DIR, 'contention-analysis.png'), dpi=300)
    print(f"✓ Generated: {OUTPUT_DIR}/contention-analysis.png")
    plt.close()

def plot_commit_abort_rates():
    """Graph 3: Commit Rate vs. Abort Rate"""
    data = read_csv("commit-abort-rates.csv")
    if not data:
        return

    # Filter out rows with empty values
    filtered_data = [row for row in data if row.get('commits_per_sec', '').strip()]
    if not filtered_data:
        print("Warning: No valid data in commit-abort-rates.csv")
        return

    configs = [row['config'] for row in filtered_data]
    commits = [float(row['commits_per_sec']) for row in filtered_data]
    aborts = [float(row.get('aborts_per_sec', '0') or '0') for row in filtered_data]

    fig, ax = plt.subplots(figsize=(10, 6))
    x = range(len(configs))
    width = 0.35

    bars1 = ax.bar([i - width/2 for i in x], commits, width,
                    label='Commits/s', color='#34a853')
    bars2 = ax.bar([i + width/2 for i in x], aborts, width,
                    label='Aborts/s', color='#fbbc04')

    ax.set_xlabel('Configuration', fontsize=12)
    ax.set_ylabel('Rate (operations/s)', fontsize=12)
    ax.set_title('Commit vs. Abort Rates', fontsize=14, fontweight='bold')
    ax.set_xticks(x)
    ax.set_xticklabels(configs)
    ax.legend()
    ax.grid(True, alpha=0.3, axis='y')

    # Add value labels on bars
    for bars in [bars1, bars2]:
        for bar in bars:
            height = bar.get_height()
            ax.annotate(f'{height:.0f}',
                       xy=(bar.get_x() + bar.get_width() / 2, height),
                       xytext=(0, 3),
                       textcoords="offset points",
                       ha='center', va='bottom', fontsize=9)

    plt.tight_layout()
    plt.savefig(os.path.join(OUTPUT_DIR, 'commit-abort-rates.png'), dpi=300)
    print(f"✓ Generated: {OUTPUT_DIR}/commit-abort-rates.png")
    plt.close()

def main():
    print("Generating performance graphs...")
    print()

    # Check if data directory exists
    if not os.path.exists(DATA_DIR):
        print(f"Error: {DATA_DIR}/ not found")
        print("Run ./collect-results.sh first to gather experiment data")
        sys.exit(1)

    # Generate all graphs
    plot_throughput_scaling()
    plot_contention_analysis()
    plot_commit_abort_rates()

    print()
    print(f"All graphs saved to {OUTPUT_DIR}/")
    print("Add these to your README with:")
    print(f"  ![Throughput Scaling](./{OUTPUT_DIR}/throughput-scaling.png)")
    print(f"  ![Contention Analysis](./{OUTPUT_DIR}/contention-analysis.png)")
    print(f"  ![Commit vs Abort Rates](./{OUTPUT_DIR}/commit-abort-rates.png)")

if __name__ == "__main__":
    main()
