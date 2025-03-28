#!/bin/bash
set -e

echo "Running all E2E tests..."
FAILED=0

for test in /app/e2e/*.sh; do
  echo "⚙️ Running test: $test ------------------------------"
  if ! $test; then
    echo "❌ Test failed: $test"
    FAILED=1
  fi
done

if [ $FAILED -eq 0 ]; then
  echo "✅ All tests passed!"
  exit 0
else
  echo "❌ Some tests failed!"
  exit 1
fi