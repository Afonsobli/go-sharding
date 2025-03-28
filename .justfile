setup:
    go mod tidy
    mkdir peer1
    mkdir peer2
    mkdir peer3
    mkdir get
    go build -o peer ./cmd/shard
    cp peer peer1/
    cp peer peer2/
    cp peer peer3/
    rm peer

clean:
    rm -rf peer1/
    rm -rf peer2/
    rm -rf peer3/
    rm -rf get/

e2e:
    docker system prune -af
    ./docker_test_runner.sh
    