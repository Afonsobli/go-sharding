#!/bin/bash

# Build and run the tests
echo "Building and starting Docker containers..."
docker-compose up --build -d

# Run the test container
echo "Running E2E tests..."
docker-compose run test

# Get the test container exit code
TEST_EXIT_CODE=$?

# Tear down the containers
echo "Stopping Docker containers..."
docker-compose down

# Exit with the same exit code as the test
echo "E2E tests completed with exit code: $TEST_EXIT_CODE"
exit $TEST_EXIT_CODE
