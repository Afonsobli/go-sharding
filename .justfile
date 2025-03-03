setup:
    mkdir peer1
    mkdir peer2
    mkdir peer3
    go build -o peer .
    cp peer peer1/
    cp peer peer2/
    cp peer peer3/

clean:
    rm -rf peer1/
    rm -rf peer2/
    rm -rf peer3/
    rm peer