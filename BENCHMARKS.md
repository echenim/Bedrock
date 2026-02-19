# BENCHMARKS.md

Bedrock Benchmarking & Reproducibility Guide

This document defines reproducible benchmarking procedures for
validating:

-   Finality latency
-   Fork rate
-   Throughput (tx/sec)
-   Bandwidth per node
-   Snapshot bootstrap time
-   Partition recovery time
-   Byzantine safety under ≥1/3 adversarial validators

All benchmarks are deterministic and script-driven.

------------------------------------------------------------------------

# 1. Test Environment

Minimum Setup:

-   4--7 validator nodes
-   2 cloud regions (e.g., us-east-1 + us-west-2)
-   4 vCPU / 8 GB RAM per node
-   QUIC transport enabled
-   RocksDB default configuration

Recommended:

-   Kubernetes cluster (multi-zone)
-   Prometheus + Grafana
-   OpenTelemetry enabled

------------------------------------------------------------------------

# 2. Local Cluster Benchmark (Docker Compose)

## docker-compose.yaml (minimal example)

``` yaml
version: '3.8'

services:
  node1:
    image: bedrock:latest
    command: ["--validator-id=1"]
    ports:
      - "26657:26657"

  node2:
    image: bedrock:latest
    command: ["--validator-id=2"]

  node3:
    image: bedrock:latest
    command: ["--validator-id=3"]

  node4:
    image: bedrock:latest
    command: ["--validator-id=4"]
```

Run:

``` bash
docker compose up -d
```

------------------------------------------------------------------------

# 3. Synthetic Load Generator

## txgen.sh

``` bash
#!/usr/bin/env bash

NODE=${1:-http://localhost:26657}
TPS=${2:-100}
DURATION=${3:-60}

echo "Target: $NODE"
echo "TPS: $TPS"
echo "Duration: $DURATION seconds"

END=$((SECONDS + DURATION))

while [ $SECONDS -lt $END ]; do
  for i in $(seq 1 $TPS); do
    curl -s -X POST "$NODE/broadcast_tx"       -H "Content-Type: application/json"       -d '{"payload":"benchmark_tx"}' &
  done
  wait
done

echo "Load test complete"
```

Run:

``` bash
chmod +x txgen.sh
./txgen.sh http://localhost:26657 200 120
```

------------------------------------------------------------------------

# 4. Finality Latency Measurement

Assumes metric exposed:

    bedrock_finality_latency_seconds

Prometheus query:

    histogram_quantile(0.95, sum(rate(bedrock_finality_latency_seconds_bucket[5m])) by (le))

Record:

-   p50
-   p95
-   p99

------------------------------------------------------------------------

# 5. Fork Rate Detection

Metric:

    bedrock_fork_events_total

Query:

    increase(bedrock_fork_events_total[10m])

Expected Result:

-   Zero under normal operation
-   Zero under ≤1/3 Byzantine simulation

------------------------------------------------------------------------

# 6. Byzantine Validator Simulation

Start one malicious validator:

``` bash
./bedrock   --validator-id=3   --malicious-mode=equivocate   --double-sign=true
```

Expected:

-   Equivocation detected
-   Block rejected
-   No conflicting commit
-   Safety preserved

Log assertion:

    Detected equivocation from validator 3

------------------------------------------------------------------------

# 7. Network Partition Test

Simulate partition:

``` bash
iptables -A INPUT -s <node_ip> -j DROP
```

Wait 60 seconds.

Heal partition:

``` bash
iptables -D INPUT -s <node_ip> -j DROP
```

Expected:

-   View change triggered
-   Liveness restored
-   No safety violation

Measure:

-   Recovery time (seconds)
-   Height difference during partition

------------------------------------------------------------------------

# 8. Snapshot Bootstrap Benchmark

On fresh node:

``` bash
./bedrock --snapshot-sync=true
```

Measure:

    time ./bedrock --join-network

Compare:

-   Full sync time
-   Snapshot sync time

Expected:

\~60% faster bootstrap using snapshot.

------------------------------------------------------------------------

# 9. Bandwidth Measurement

Using ifstat:

``` bash
sudo apt install ifstat
ifstat -t 1
```

Or Prometheus metric:

    bedrock_network_bytes_total

Compute:

-   Bytes per block
-   Bytes per second
-   Gossip amplification factor

------------------------------------------------------------------------

# 10. Deterministic Replay Verification

Replay last 100 blocks:

``` bash
./bedrock --replay --blocks=100
```

Expected:

-   Identical state root
-   Zero divergence
-   Deterministic execution confirmed

------------------------------------------------------------------------

# 11. Multi-Region Benchmark (Kubernetes)

Deploy:

``` bash
terraform apply
kubectl apply -f k8s/validators.yaml
```

Inject load:

``` bash
./txgen.sh http://<lb-endpoint> 500 180
```

Collect:

-   Cross-region latency
-   Finality delay delta
-   Bandwidth per region

------------------------------------------------------------------------

# 12. Benchmark Report Template

Record:

Environment: Validator Count: Region Layout: TPS Target:

Results:

-   p50 finality:
-   p95 finality:
-   Fork events:
-   Recovery time:
-   Bootstrap time:
-   Bandwidth per node:

Safety Violations: Liveness Failures:

------------------------------------------------------------------------

# Reproducibility Guarantees

To ensure reproducibility:

-   Fix Docker image tag
-   Fix validator set
-   Disable nondeterministic clock usage
-   Use deterministic seed

Example:

``` bash
./bedrock --seed=42
```

------------------------------------------------------------------------

# Philosophy

Benchmarks are meaningless without adversarial simulation.

Bedrock benchmarks are valid only if:

-   Byzantine behavior is injected
-   Partitions are simulated
-   Replay tests pass
-   State roots match across independent nodes

Correctness precedes performance.
