package main

import (
    "fmt"
    "net/http"
    "os"

    "shard/internal/handlers"
    "shard/internal/node"
)
func main() {
    n, err := node.New("out")
    if err != nil {
        fmt.Printf("Failed to start P2P node: %s\n", err)
        return
    }
    defer n.Close()

    h := handlers.New(n)
    http.HandleFunc("/upload", h.Upload)
    http.HandleFunc("/file", h.GetFile)
    http.HandleFunc("/health", h.HealthHandler)

    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }

    fmt.Printf("Server started at http://localhost:%s\n", port)
    if err := http.ListenAndServe(":"+port, nil); err != nil {
        fmt.Println("Error starting server:", err)
    }
}