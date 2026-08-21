package server

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type workspaceEnvelope struct {
	Data json.RawMessage `json:"data"`
}

func decodeWorkspaceData(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	var envelope workspaceEnvelope
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, recorder.Body.String())
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		t.Fatalf("decode response data: %v (data=%s)", err, envelope.Data)
	}
}

func workspaceJSONRequest(t *testing.T, srv *Server, method, endpoint, body string, expected int) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, endpoint, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	srv.Router().ServeHTTP(recorder, req)
	if recorder.Code != expected {
		t.Fatalf("%s %s: expected %d, got %d: %s", method, endpoint, expected, recorder.Code, recorder.Body.String())
	}
	return recorder
}

func approveWorkspaceMutation(t *testing.T, srv *Server, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var pending struct {
		Approval struct {
			ID string `json:"id"`
		} `json:"approval"`
	}
	decodeWorkspaceData(t, recorder, &pending)
	if pending.Approval.ID == "" {
		t.Fatalf("approval id missing: %s", recorder.Body.String())
	}
	approved := workspaceJSONRequest(t, srv, http.MethodPost, "/api/approvals/"+pending.Approval.ID+"/approve", "", http.StatusOK)
	var response struct {
		Result map[string]any `json:"result"`
	}
	decodeWorkspaceData(t, approved, &response)
	return response.Result
}

func createWorkspaceDirectory(t *testing.T, srv *Server, parentID, name string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"parent_id": parentID, "name": name})
	return approveWorkspaceMutation(t, srv, workspaceJSONRequest(t, srv, http.MethodPost, "/api/workspace/mkdir", string(body), http.StatusAccepted))
}

func createWorkspaceFile(t *testing.T, srv *Server, parentID, name, content string) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"parent_id": parentID, "name": name, "content": content})
	return approveWorkspaceMutation(t, srv, workspaceJSONRequest(t, srv, http.MethodPost, "/api/workspace/file", string(body), http.StatusAccepted))
}

func workspaceString(value any) string {
	result, _ := value.(string)
	return result
}

func assertNoWorkspaceHostPath(t *testing.T, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	if strings.Contains(lower, `\\data\\workspace`) || strings.Contains(lower, `d:\\`) || strings.Contains(lower, `absolute_path`) {
		t.Fatalf("workspace API leaked a host path: %s", body)
	}
}

func TestWorkspaceDatabaseLifecycleUsesOpaqueIDs(t *testing.T) {
	srv := newTestServer(t)
	directoryName := `Kế hoạch / quý:1?*\\`
	directory := createWorkspaceDirectory(t, srv, "", directoryName)
	directoryID := workspaceString(directory["id"])
	if directoryID == "" || workspaceString(directory["name"]) != directoryName {
		t.Fatalf("directory identity/name mismatch: %#v", directory)
	}

	fileName := `bản nháp / 100%:?.txt`
	file := createWorkspaceFile(t, srv, directoryID, fileName, "nội dung chính xác")
	fileID := workspaceString(file["id"])
	if fileID == "" || workspaceString(file["name"]) != fileName {
		t.Fatalf("file identity/name mismatch: %#v", file)
	}

	listed := workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/files?parent_id="+url.QueryEscape(directoryID), "", http.StatusOK)
	assertNoWorkspaceHostPath(t, listed.Body.String())
	var listing struct {
		Files []databaseWorkspaceItem `json:"files"`
	}
	decodeWorkspaceData(t, listed, &listing)
	if len(listing.Files) != 1 || listing.Files[0].ID != fileID || listing.Files[0].Name != fileName {
		t.Fatalf("unexpected listing: %#v", listing.Files)
	}

	read := workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/file?id="+url.QueryEscape(fileID), "", http.StatusOK)
	assertNoWorkspaceHostPath(t, read.Body.String())
	var contents struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Content string `json:"content"`
		Version int64  `json:"version"`
	}
	decodeWorkspaceData(t, read, &contents)
	if contents.ID != fileID || contents.Content != "nội dung chính xác" {
		t.Fatalf("unexpected content: %#v", contents)
	}

	renamedName := `không cần đuôi / vẫn hợp lệ<>|`
	renameBody, _ := json.Marshal(map[string]any{"file_id": fileID, "parent_id": directoryID, "name": renamedName, "expected_version": contents.Version})
	renamed := approveWorkspaceMutation(t, srv, workspaceJSONRequest(t, srv, http.MethodPost, "/api/workspace/rename", string(renameBody), http.StatusAccepted))
	if workspaceString(renamed["id"]) != fileID || workspaceString(renamed["name"]) != renamedName {
		t.Fatalf("rename changed identity or name: %#v", renamed)
	}

	duplicateBody, _ := json.Marshal(map[string]string{"file_id": fileID, "parent_id": directoryID, "name": `bản sao / tùy ý`})
	duplicate := approveWorkspaceMutation(t, srv, workspaceJSONRequest(t, srv, http.MethodPost, "/api/workspace/duplicate", string(duplicateBody), http.StatusAccepted))
	if workspaceString(duplicate["id"]) == "" || workspaceString(duplicate["id"]) == fileID {
		t.Fatalf("duplicate must receive a new opaque id: %#v", duplicate)
	}

	deleteURL := "/api/workspace/file?id=" + url.QueryEscape(fileID) + "&expected_version=" + url.QueryEscape(workspaceString(renamed["version"]))
	deleted := approveWorkspaceMutation(t, srv, workspaceJSONRequest(t, srv, http.MethodDelete, deleteURL, "", http.StatusAccepted))
	if workspaceString(deleted["status"]) != "deleted" {
		t.Fatalf("unexpected delete result: %#v", deleted)
	}
	workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/file?id="+url.QueryEscape(fileID), "", http.StatusNotFound)
}

func TestWorkspaceBinaryUploadRawStatsAndZip(t *testing.T) {
	srv := newTestServer(t)
	directory := createWorkspaceDirectory(t, srv, "", `media / tùy ý`)
	directoryID := workspaceString(directory["id"])
	payload := []byte{0x00, 0xff, 0x10, 0x80, 'A'}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "ignored.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(payload)
	_ = writer.WriteField("parent_id", directoryID)
	_ = writer.WriteField("name", `ảnh / không-format.bin`)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/workspace/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	srv.Router().ServeHTTP(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("upload: %d %s", recorder.Code, recorder.Body.String())
	}
	uploaded := approveWorkspaceMutation(t, srv, recorder)
	fileID := workspaceString(uploaded["id"])

	raw := workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/raw?id="+url.QueryEscape(fileID), "", http.StatusOK)
	if !bytes.Equal(raw.Body.Bytes(), payload) {
		t.Fatalf("raw binary mismatch: got %s want %s", base64.StdEncoding.EncodeToString(raw.Body.Bytes()), base64.StdEncoding.EncodeToString(payload))
	}

	stats := workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/stats", "", http.StatusOK)
	var summary struct {
		TotalFiles       int   `json:"total_files"`
		TotalDirectories int   `json:"total_directories"`
		TotalSize        int64 `json:"total_size"`
	}
	decodeWorkspaceData(t, stats, &summary)
	if summary.TotalFiles != 1 || summary.TotalDirectories != 1 || summary.TotalSize != int64(len(payload)) {
		t.Fatalf("database stats mismatch: %#v", summary)
	}

	archive := workspaceJSONRequest(t, srv, http.MethodGet, "/api/workspace/zip?id="+url.QueryEscape(directoryID), "", http.StatusOK)
	zipReader, err := zip.NewReader(bytes.NewReader(archive.Body.Bytes()), int64(archive.Body.Len()))
	if err != nil {
		t.Fatalf("read zip: %v", err)
	}
	var found bool
	for _, entry := range zipReader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if bytes.Equal(content, payload) {
			found = true
		}
	}
	if !found {
		t.Fatalf("uploaded binary missing from archive: %#v", zipReader.File)
	}
}
