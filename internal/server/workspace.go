// Package server implements the HTTP server for TF-AI.
// This file contains all workspace-related HTTP handlers and helpers.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// resolveAbsDir cleans and validates that the given path is absolute.
// Returns the cleaned path or an error suitable for returning to the client.
func resolveAbsDir(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("dir is required")
	}
	dir := filepath.Clean(raw)
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("dir must be an absolute path")
	}
	return dir, nil
}

// writeJSONError writes a JSON-formatted error response with the given status code.
func writeJSONError(w http.ResponseWriter, msg string, status int) {
	http.Error(w, `{"error":"`+msg+`"}`, status)
}

// handleWorkspace handles GET /api/workspace?dir=<path>.
// It returns the directory contents, TF file list, subdirs, and workspace status flags.
func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	dir, err := resolveAbsDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, "directory not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "failed to read directory", http.StatusInternalServerError)
		return
	}

	resp := workspaceResponse{
		Dir:   dir,
		Files: []string{},
		Dirs:  []string{},
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			if name == ".terraform" {
				resp.Initialized = true
			}
			// Exclude hidden directories from the visible Dirs list.
			if !strings.HasPrefix(name, ".") {
				resp.Dirs = append(resp.Dirs, name)
			}
			continue
		}
		switch name {
		case "terraform.tfstate":
			resp.HasState = true
		case ".terraform.lock.hcl":
			resp.HasLockfile = true
		}
		ext := filepath.Ext(name)
		if ext == ".tf" || ext == ".tfvars" {
			resp.Files = append(resp.Files, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("server: workspace encode error: %v", err)
	}
}

// handleWorkspaceCreate handles POST /api/workspace/create.
// It creates the directory and writes a minimal Terraform scaffold into it.
func (s *Server) handleWorkspaceCreate(w http.ResponseWriter, r *http.Request) {
	var body createWorkspaceRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Printf("server: workspace create decode error: %v", err)
		writeJSONError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	dir, err := resolveAbsDir(body.Dir)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp := createWorkspaceResponse{Dir: dir}
	if body.Description != "" {
		resp.Prompt = "Create a Terraform workspace for: " + body.Description
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("server: workspace create mkdir error: %v", err)
		writeJSONError(w, "failed to create directory: "+err.Error(), http.StatusInternalServerError)
		return
	}

	for _, f := range scaffoldFiles() {
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, []byte(f.content), 0o644); err != nil {
			log.Printf("server: workspace create write %s error: %v", f.name, err)
			writeJSONError(w, "failed to create "+f.name+": "+err.Error(), http.StatusInternalServerError)
			return
		}
		resp.Files = append(resp.Files, f.name)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("server: workspace create encode error: %v", err)
	}
}

// handleFileRead handles GET /api/file?path=<absolute-path>.
// Returns the raw content of the requested file. The path must be absolute.
func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("path")
	if raw == "" {
		writeJSONError(w, "path is required", http.StatusBadRequest)
		return
	}
	path := filepath.Clean(raw)
	if !filepath.IsAbs(path) {
		writeJSONError(w, "path must be absolute", http.StatusBadRequest)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, "file not found", http.StatusNotFound)
			return
		}
		writeJSONError(w, "failed to read file", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(fileResponse{Path: path, Content: string(content)}); err != nil {
		log.Printf("server: file read encode error: %v", err)
	}
}

// handleFileSave handles PUT /api/file.
// Writes the provided content to the given absolute path. The path must
// already exist within a directory that is accessible on the filesystem.
func (s *Server) handleFileSave(w http.ResponseWriter, r *http.Request) {
	var body fileSaveRequest
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSONError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Path == "" {
		writeJSONError(w, "path is required", http.StatusBadRequest)
		return
	}
	path := filepath.Clean(body.Path)
	if !filepath.IsAbs(path) {
		writeJSONError(w, "path must be absolute", http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(path, []byte(body.Content), 0o644); err != nil {
		log.Printf("server: file save error path=%s: %v", path, err)
		writeJSONError(w, "failed to save file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("server: file saved path=%s", path)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"ok":true}`)
}

// scaffoldFile is a name/content pair for a file to write during workspace creation.
type scaffoldFile struct {
	// name is the filename to write inside the workspace directory.
	name string
	// content is the initial file content.
	content string
}

// scaffoldFiles returns the minimal set of Terraform files for a new workspace.
func scaffoldFiles() []scaffoldFile {
	return []scaffoldFile{
		{"main.tf", "# Add your resources here\n"},
		{"variables.tf", "# Define input variables here\n"},
		{"outputs.tf", "# Define outputs here\n"},
		{"versions.tf", "terraform {\n  required_version = \">= 1.5\"\n}\n"},
	}
}
