#!/bin/bash
set -e

echo "Running all E2E tests..."
FAILED=0
TOTAL_START_TIME=$(date +%s)
declare -A TEST_TIMES
declare -a FAILED_TESTS

# Check if Docker is accessible
echo "Checking Docker connectivity..."
if ! docker ps > /dev/null; then
  echo "❌ Cannot connect to Docker daemon. Make sure the Docker socket is mounted correctly."
  exit 1
fi

echo "✅ Docker connectivity verified. Container management is available."

# List container names for reference
PEER_CONTAINERS=$(docker ps --format '{{.Names}}' | grep peer)
echo "Available peer containers: $PEER_CONTAINERS"

# Terminal width for separator
TERM_WIDTH=$(tput cols 2>/dev/null || echo 80)
SEPARATOR=$(printf '%*s' "$TERM_WIDTH" | tr ' ' '=')

echo "$SEPARATOR"
echo "STARTING TEST EXECUTION"
echo "$SEPARATOR"
echo ""

for test in /app/e2e/tests/*.sh; do
  TEST_NAME=$(basename "$test")
  echo "$SEPARATOR"
  echo "⚙️  TEST: $TEST_NAME"
  echo "$SEPARATOR"
  START_TIME=$(date +%s)
  
  if ! $test; then
    echo ""
    echo "❌ Test failed: $test"
    FAILED=1
    FAILED_TESTS+=("$TEST_NAME")
  else
    echo ""
    echo "✅ Test passed: $test"
  fi
  
  END_TIME=$(date +%s)
  DURATION=$((END_TIME - START_TIME))
  TEST_TIMES["$TEST_NAME"]=$DURATION
  echo "⏱️  Time: ${DURATION}s"
  echo ""
done

TOTAL_END_TIME=$(date +%s)
TOTAL_DURATION=$((TOTAL_END_TIME - TOTAL_START_TIME))

# Generate report
echo "$SEPARATOR"
echo "📊 TEST EXECUTION REPORT 📊"
echo "$SEPARATOR"
echo "Test Name | Duration | Status"
echo "----------------------------------------"

for test_name in "${!TEST_TIMES[@]}"; do
  duration=${TEST_TIMES["$test_name"]}
  
  # Check if test is in failed tests array
  status="✅ PASS"
  for failed in "${FAILED_TESTS[@]}"; do
    if [ "$failed" == "$test_name" ]; then
      status="❌ FAIL"
      break
    fi
  done
  
  printf "%-40s | %5ds | %s\n" "$test_name" "$duration" "$status"
done

echo "----------------------------------------"
echo "Total execution time: ${TOTAL_DURATION}s"
echo "$SEPARATOR"

if [ $FAILED -eq 0 ]; then
  echo "✅ All tests passed!"
  exit 0
else
  echo "❌ ${#FAILED_TESTS[@]} tests failed!"
  exit 1
fi