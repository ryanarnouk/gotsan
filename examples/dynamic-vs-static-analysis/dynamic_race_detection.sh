#!/bin/bash

count=0
start=$(date +%s.%N)

while true; do
    count=$((count+1))

    run_start=$(date +%s.%N)

    output=$(go run -race dynamic_example.go 2>&1)

    run_end=$(date +%s.%N)
    run_time=$(echo "$run_end - $run_start" | bc)

    echo "Run $count took ${run_time}s"

    if echo "$output" | grep -q "DATA RACE"; then
        total_end=$(date +%s.%N)
        total_time=$(echo "$total_end - $start" | bc)

        echo ""
        echo "DATA RACE FOUND"
        echo "Executions: $count"
        echo "Total time: ${total_time}s"
        break
    fi
done
