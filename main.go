package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
)

var globalNode Node

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Limit the size of the uploaded file (optional)
	r.ParseMultipartForm(10 << 20) // 10 MB

	// Get the file from the request
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Unable to read the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Create a new SHA-256 hash object
	hash := sha256.New()

	// Copy the file content into the hash object
	_, err = io.Copy(hash, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Compute the hash and return it as an hexadecimal string
	hashInBytes := hash.Sum(nil)
	finalFilename := hex.EncodeToString(hashInBytes)

	// Create a new file on the server to save the uploaded file
	dst, err := os.Create(finalFilename)
	if err != nil {
		http.Error(w, "Unable to save the file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	file.Seek(0, io.SeekStart)
	// Copy the contents of the uploaded file to the new file on the server
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, "File uploaded successfully!")

    globalNode.DistributeFile(finalFilename)

    // Respond to the client indicating success
    fmt.Fprintln(w, "File uploaded successfully and distributed to peers!")
}

func main() {
	var err error
	globalNode, err = StartP2PNode()
	if err != nil {
		fmt.Printf("Failed to start P2P node: %s\n", err)
		return
	}
	defer globalNode.host.Close()

	http.HandleFunc("/upload", uploadHandler)

	// Start the HTTP server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server started at http://localhost:", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), nil); err != nil {
		fmt.Println("Error starting server:", err)
	}
}
