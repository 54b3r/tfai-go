// Package server implements the HTTP server for TF-AI.
// This file contains all workspace-related HTTP handlers and helpers.
package server

import (
	"encoding/json"
	"fmt"
	"io/fs"
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

// confineToDir validates that target resolves to a path inside root after
// cleaning both. This prevents path traversal attacks (e.g. "../../etc/passwd").
// Returns the cleaned absolute target path or an error.
func confineToDir(root, target string) (string, error) {
	root = filepath.Clean(root)
	target = filepath.Clean(target)
	if !strings.HasPrefix(target+string(filepath.Separator), root+string(filepath.Separator)) {
		return "", fmt.Errorf("path is outside the workspace directory")
	}
	return target, nil
}

// handleWorkspace handles GET /api/workspace?dir=<path>.
// It recursively walks the directory and returns all .tf/.tfvars files as
// relative paths (e.g. "modules/vpc/main.tf"), plus workspace status flags.
func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	dir, err := resolveAbsDir(r.URL.Query().Get("dir"))
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		writeJSONError(w, "directory not found", http.StatusNotFound)
		return
	}

	resp := workspaceResponse{
		Dir:   dir,
		Files: []string{},
		Dirs:  []string{},
	}

	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		name := d.Name()
		if d.IsDir() {
			// Skip hidden dirs entirely (don't descend into .terraform)
			if strings.HasPrefix(name, ".") {
				if name == ".terraform" {
					resp.Initialized = true
				}
				return filepath.SkipDir
			}
			return nil
		}
		switch name {
		case "terraform.tfstate":
			resp.HasState = true
		case ".terraform.lock.hcl":
			resp.HasLockfile = true
		}
		ext := filepath.Ext(name)
		if ext == ".tf" || ext == ".tfvars" {
			rel, relErr := filepath.Rel(dir, path)
			if relErr == nil {
				resp.Files = append(resp.Files, rel)
			}
		}
		return nil
	})
	if err != nil {
		log.Printf("server: workspace walk error: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("server: workspace encode error: %v", err)
	}
}

// handleWorkspaceCreate handles POST /api/workspace/create.
// It writes a minimal Terraform scaffold into an existing directory.
// The directory must already exist — this handler will not create it.
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

	// Reject if the directory does not already exist — we do not create directories.
	if info, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, "directory does not exist — create it first, then scaffold", http.StatusBadRequest)
			return
		}
		writeJSONError(w, "failed to access directory: "+err.Error(), http.StatusInternalServerError)
		return
	} else if !info.IsDir() {
		writeJSONError(w, "path exists but is not a directory", http.StatusBadRequest)
		return
	}

	resp := createWorkspaceResponse{Dir: dir}
	if body.Description != "" {
		resp.Prompt = "Create a Terraform workspace for: " + body.Description
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

// handleFileRead handles GET /api/file?path=<absolute-path>&workspaceDir=<root>.
// Returns the raw content of the requested file. The path must resolve within
// the declared workspaceDir to prevent path traversal.
func (s *Server) handleFileRead(w http.ResponseWriter, r *http.Request) {
	rawPath := r.URL.Query().Get("path")
	rawRoot := r.URL.Query().Get("workspaceDir")
	if rawPath == "" {
		writeJSONError(w, "path is required", http.StatusBadRequest)
		return
	}
	if rawRoot == "" {
		writeJSONError(w, "workspaceDir is required", http.StatusBadRequest)
		return
	}
	path, err := confineToDir(rawRoot, rawPath)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusForbidden)
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
// Writes content to the given path. The path must resolve within the declared
// workspaceDir to prevent writes outside the user's workspace.
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
	if body.WorkspaceDir == "" {
		writeJSONError(w, "workspaceDir is required", http.StatusBadRequest)
		return
	}
	path, err := confineToDir(body.WorkspaceDir, body.Path)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusForbidden)
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
