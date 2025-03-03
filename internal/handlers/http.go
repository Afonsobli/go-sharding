package handlers

import (
    "crypto/sha256"
    "encoding/hex"
    "fmt"
    "io"
    "net/http"
    "os"

    "shard/internal/types"
)

type Handler struct {
    node types.Node
}

func New(node types.Node) *Handler {
    return &Handler{node: node}
}

func (h *Handler) Upload(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10 << 20) // 10 MB

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Unable to read the file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	hashInBytes := hash.Sum(nil)
	finalFilename := hex.EncodeToString(hashInBytes)

	dst, err := os.Create(finalFilename)
	if err != nil {
		http.Error(w, "Unable to save the file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	file.Seek(0, io.SeekStart)
	_, err = io.Copy(dst, file)
	if err != nil {
		http.Error(w, "Failed to write file", http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, "File uploaded successfully!")
    h.node.DistributeFile(finalFilename)

    // Respond to the client indicating success
    fmt.Fprintln(w, "File uploaded successfully and distributed to peers!")
}

func (h *Handler) GetFile(w http.ResponseWriter, r *http.Request) {
    hash := r.URL.Query().Get("hash")
    if hash == "" {
        http.Error(w, "Hash parameter is required", http.StatusBadRequest)
        return
    }

    // Try to open the file locally first
    file, err := os.Open(hash)
    if err != nil {
        if os.IsNotExist(err) {
            // File not found locally, try to get it from peers
            err = h.node.RequestFileFromPeers(hash)
            if err != nil {
                http.Error(w, "File not found in network", http.StatusNotFound)
                return
            }
            // Now try to open the file again
            file, err = os.Open(hash)
            if err != nil {
                http.Error(w, "Error opening file after retrieval", http.StatusInternalServerError)
                return
            }
        } else {
            http.Error(w, "Error opening file", http.StatusInternalServerError)
            return
        }
    }
    defer file.Close()

    // Set content type header to application/octet-stream for binary file download
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", hash))

    // Copy the file to the response writer
    _, err = io.Copy(w, file)
    if err != nil {
        http.Error(w, "Error sending file", http.StatusInternalServerError)
        return
    }
}
