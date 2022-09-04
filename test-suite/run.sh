#!/usr/bin/env bash
# Script to run performance test case for operator types multiple times
# To run as background process in ZSH - nohup ./run.sh >> script.log 2>&1 &!

# Optional variable:
# RESULTS_DIR
# - Description: Directory to save test suite data to
# - Default: results
# TYPE
# - Description: Operator project type to run test suite against
# - Default: go/v3
# - Options: go/v3 | ansible | helm
# OSDKVersion
# - Description: Operator SDK version to clone
# - Default: v1.20.0
# MAX_CONCURRENT_RECONCILE
# - Description: Set maximum number of concurrent reconciles via --max-concurrent-reconciles flag (only available for GO & Helm )
# - Default: as configured by cloned Operator-SDK project
# CPU_LIMIT
# - Description: Set CPU limit resource on Operator container in Deployment
# - Default: as configured by cloned Operator-SDK project
# MEMORY_LIMIT
# - Description: Set Memory limit resource on Operator container in Deployment
# - Default: as configured by cloned Operator-SDK project
# SCRAPE_METRICS
# - Description: Set to true to deploy instance of prometheus and kube state metrics to scape cluster and operator metrics
# - Default: false
# - Options: true
# DESTROY_CLUSTER
# - Description: Set to true to destroy KIND cluster at the end of a single run
# - Default: false
# - Options: true

RUNS=${RUNS:-10}

for ((c = 1; c <= $RUNS; c++)); do
  echo "Attempt Go: $c"
  TYPE=go/v3 ginkgo -v -progress
done

for ((c = 1; c <= $RUNS; c++)); do
  echo "Attempt Helm: $c"
  TYPE=helm ginkgo -v -progress
done

for ((c = 1; c <= $RUNS; c++)); do
  echo "Attempt Ansible $c"
  TYPE=ansible ginkgo -v -progress
done
