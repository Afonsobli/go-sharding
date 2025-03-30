#!/bin/bash

# Build and run the containers
echo "Building and starting Docker containers..."
docker-compose up --build -d

# Wait for services to be healthy
echo "Waiting for all services to be healthy..."
MAX_RETRIES=30
RETRY_COUNT=0
while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
  UNHEALTHY=0
  for service in peer1 peer2 peer3; do
    STATUS=$(docker inspect --format "{{.State.Health.Status}}" $(docker-compose ps -q $service 2>/dev/null) 2>/dev/null)
    if [ "$STATUS" != "healthy" ]; then
      UNHEALTHY=1
      echo "Service $service is $STATUS"
    fi
  done
  
  if [ $UNHEALTHY -eq 0 ]; then
    echo "All services are healthy!"
    break
  fi
  
  RETRY_COUNT=$((RETRY_COUNT+1))
  echo "Waiting for services to be healthy (attempt $RETRY_COUNT/$MAX_RETRIES)..."
  sleep 5
done

if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
  echo "❌ Services failed to become healthy after $MAX_RETRIES attempts"
  docker-compose logs
  docker-compose down
  exit 1
fi

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