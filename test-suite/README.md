# Test Suite
The test suite is developed using Ginkgo to conform with current Operator SDK testing standards.

The tests are based on the Operator SDK E2E tests. Some test util functions were copied to this repository because they cannot be imported as a module as it exists in an internal directory.

A KIND local cluster will be created, and one of the Memcached Operators will be deployed onto the cluster. A load will
be applied to the cluster by creating 15 Memcached CRs, and resources/pod metrics will be saved to the `results`
directory for analysis.

## Pre-requisites
* [go1.17](https://go.dev/doc/install)
* [Ginkgo v1](https://pkg.go.dev/github.com/onsi/ginkgo/ginkgo)
```shell
go install github.com/onsi/ginkgo/ginkgo@latest
```

## Run Test Suite
### Single Run
```shell
TYPE=go ginkgo -v -progress
```
### Script for running 10 tests for each project type 
```shell
nohup ./run.sh >> script.log 2>&1 &!
```
### Configuration Options
See [run.sh](run.sh) for additional configuration options that can be passed to the test suite