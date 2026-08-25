package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

const (
	VirtualRoot  = "/data/workspace"
	rootFolderID = "00000000-0000-0000-0000-000000000000"
	maxNameBytes = 4096
)

var (
	ErrNotFound    = errors.New("workspace node not found")
	ErrConflict    = errors.New("workspace node conflict")
	ErrInvalidName = errors.New("invalid workspace node name")
	ErrInvalidNode = errors.New("invalid workspace node")
	ErrVersion     = errors.New("workspace version conflict")
)

type Node struct {
	ID           string  `json:"id"`
	ParentID     string  `json:"parent_id"`
	Name         string  `json:"name"`
	Type         string  `json:"type"`
	MIMEType     string  `json:"mime_type"`
	SizeBytes    int64   `json:"size_bytes"`
	ContentHash  string  `json:"content_hash"`
	RelativePath string  `json:"-"`
	Version      int64   `json:"version"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	DeletedAt    *string `json:"deleted_at,omitempty"`
	VirtualPath  string  `json:"virtual_path"`
	ExecPath     string  `json:"exec_path,omitempty"`
}

type WriteRequest struct {
	ID              string
	ParentID        string
	Name            string
	Content         []byte
	MIMEType        string
	ExpectedVersion int64
	ActorID         string
}

type SearchResult struct {
	Node
	Snippet string `json:"snippet,omitempty"`
}

type Stats struct {
	TotalSize        int64            `json:"total_size"`
	TotalFiles       int64            `json:"total_files"`
	TotalDirectories int64            `json:"total_directories"`
	Breakdown        map[string]int64 `json:"breakdown"`
}

type Store struct {
	db          *sql.DB
	root        string
	namedRoot   string
	now         func() time.Time
	namedMu     sync.RWMutex
	namedPaused bool
	execByID    map[string]string
	idByExec    map[string]string
}

func NewStore(ctx context.Context, db *sql.DB, root string) (*Store, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolving workspace filesystem root: %w", err)
	}
	if err := Migrate(ctx, db, absRoot); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(absRoot, rootFolderID), 0750); err != nil {
		return nil, fmt.Errorf("creating workspace root folder: %w", err)
	}
	namedRoot := filepath.Join(absRoot, NamedDirName)
	if err := os.MkdirAll(namedRoot, 0750); err != nil {
		return nil, fmt.Errorf("creating named workspace projection: %w", err)
	}
	store := &Store{
		db:        db,
		root:      absRoot,
		namedRoot: namedRoot,
		now:       time.Now,
		execByID:  map[string]string{},
		idByExec:  map[string]string{},
	}
	store.refreshNamed(ctx)
	return store, nil
}

func (s *Store) DB() *sql.DB { return s.db }

func ValidateName(name string) error {
	if name == "" || !utf8.ValidString(name) || len(name) > maxNameBytes || strings.ContainsRune(name, '\x00') {
		return ErrInvalidName
	}
	return nil
}

func hashContent(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func detectMIME(name, requested string, content []byte) string {
	if requested != "" && requested != "application/octet-stream" {
		return strings.ToLower(strings.TrimSpace(strings.Split(requested, ";")[0]))
	}
	detected := http.DetectContentType(content)
	if detected != "application/octet-stream" {
		return strings.ToLower(strings.TrimSpace(strings.Split(detected, ";")[0]))
	}
	if extType := mime.TypeByExtension(filepath.Ext(name)); extType != "" {
		return strings.ToLower(strings.TrimSpace(strings.Split(extType, ";")[0]))
	}
	return "application/octet-stream"
}

func storageRelativePath(parentID, fileID string) (string, error) {
	if parentID == "" {
		parentID = rootFolderID
	}
	parentUUID, err := uuid.Parse(parentID)
	if err != nil || parentUUID.String() != parentID {
		return "", fmt.Errorf("%w: invalid storage folder ID", ErrInvalidNode)
	}
	fileUUID, err := uuid.Parse(fileID)
	if err != nil || fileUUID.String() != fileID {
		return "", fmt.Errorf("%w: invalid storage file ID", ErrInvalidNode)
	}
	return filepath.ToSlash(filepath.Join(parentID, fileID)), nil
}

func (s *Store) storagePath(relativePath string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(relativePath))
	if filepath.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: invalid workspace storage path", ErrInvalidNode)
	}
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("%w: workspace storage path must contain folder and file IDs", ErrInvalidNode)
	}
	if _, err := storageRelativePath(parts[0], parts[1]); err != nil {
		return "", err
	}
	target := filepath.Join(s.root, clean)
	rel, err := filepath.Rel(s.root, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: workspace storage path escaped root", ErrInvalidNode)
	}
	return target, nil
}

func (s *Store) ensureParent(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, parentID string) error {
	if parentID == "" {
		return nil
	}
	var nodeType string
	var deletedAt sql.NullString
	err := q.QueryRowContext(ctx, `SELECT node_type, deleted_at FROM workspace_nodes WHERE id = ?`, parentID).Scan(&nodeType, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) || deletedAt.Valid {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("checking workspace parent: %w", err)
	}
	if nodeType != "directory" {
		return fmt.Errorf("%w: parent is not a directory", ErrInvalidNode)
	}
	return nil
}

func scanNode(scanner interface{ Scan(...any) error }) (Node, error) {
	var node Node
	var deleted sql.NullString
	err := scanner.Scan(&node.ID, &node.ParentID, &node.Name, &node.Type, &node.MIMEType,
		&node.SizeBytes, &node.ContentHash, &node.RelativePath, &node.Version, &node.CreatedAt, &node.UpdatedAt, &deleted)
	if deleted.Valid {
		node.DeletedAt = &deleted.String
	}
	return node, err
}

const nodeColumns = `id, parent_id, name, node_type, mime_type, size_bytes, content_hash, relative_path,
	version, created_at, updated_at, deleted_at`

const qualifiedNodeColumns = `n.id, n.parent_id, n.name, n.node_type, n.mime_type, n.size_bytes, n.content_hash, n.relative_path,
	n.version, n.created_at, n.updated_at, n.deleted_at`

func (s *Store) Get(ctx context.Context, id string) (Node, error) {
	node, err := scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes WHERE id = ? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, fmt.Errorf("getting workspace node: %w", err)
	}
	node.VirtualPath, err = s.VirtualPath(ctx, node.ID)
	if err != nil {
		return Node{}, err
	}
	node.ExecPath = s.lookupExecPath(node.ID)
	return node, nil
}

func (s *Store) VirtualPath(ctx context.Context, id string) (string, error) {
	if id == "" {
		return VirtualRoot, nil
	}
	segments := make([]string, 0, 8)
	seen := map[string]struct{}{}
	current := id
	for current != "" {
		if _, ok := seen[current]; ok {
			return "", fmt.Errorf("%w: workspace hierarchy cycle", ErrInvalidNode)
		}
		seen[current] = struct{}{}
		var parent, name string
		var deleted sql.NullString
		err := s.db.QueryRowContext(ctx, `SELECT parent_id, name, deleted_at FROM workspace_nodes WHERE id = ?`, current).Scan(&parent, &name, &deleted)
		if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
			return "", ErrNotFound
		}
		if err != nil {
			return "", fmt.Errorf("resolving workspace virtual path: %w", err)
		}
		segments = append(segments, url.PathEscape(name))
		current = parent
		if len(segments) > 1024 {
			return "", fmt.Errorf("%w: workspace hierarchy is too deep", ErrInvalidNode)
		}
	}
	for left, right := 0, len(segments)-1; left < right; left, right = left+1, right-1 {
		segments[left], segments[right] = segments[right], segments[left]
	}
	return VirtualRoot + "/" + strings.Join(segments, "/"), nil
}

func (s *Store) List(ctx context.Context, parentID string) ([]Node, error) {
	if err := s.ensureParent(ctx, s.db, parentID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes
		WHERE parent_id = ? AND deleted_at IS NULL
		ORDER BY CASE node_type WHEN 'directory' THEN 0 ELSE 1 END, name COLLATE NOCASE, name`, parentID)
	if err != nil {
		return nil, fmt.Errorf("listing workspace nodes: %w", err)
	}
	nodes := make([]Node, 0)
	for rows.Next() {
		node, scanErr := scanNode(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning workspace node: %w", scanErr)
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating workspace nodes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing workspace node rows: %w", err)
	}
	for index := range nodes {
		s.attachDerivedPaths(ctx, &nodes[index])
	}
	return nodes, nil
}

// Breadcrumbs returns the active directory ancestry from the workspace root
// to directoryID. The virtual root itself is represented by the empty ID and
// is intentionally omitted.
func (s *Store) Breadcrumbs(ctx context.Context, directoryID string) ([]Node, error) {
	if directoryID == "" {
		return []Node{}, nil
	}
	seen := make(map[string]struct{})
	reversed := make([]Node, 0, 8)
	current := directoryID
	for current != "" {
		if _, exists := seen[current]; exists {
			return nil, fmt.Errorf("%w: workspace parent cycle", ErrInvalidNode)
		}
		seen[current] = struct{}{}
		node, err := s.Get(ctx, current)
		if err != nil {
			return nil, err
		}
		if node.Type != "directory" {
			return nil, fmt.Errorf("%w: breadcrumb node is not a directory", ErrInvalidNode)
		}
		reversed = append(reversed, node)
		current = node.ParentID
		if len(reversed) > 1024 {
			return nil, fmt.Errorf("%w: workspace ancestry is too deep", ErrInvalidNode)
		}
	}
	result := make([]Node, len(reversed))
	for index := range reversed {
		result[len(reversed)-1-index] = reversed[index]
	}
	return result, nil
}

// ResolveLegacyPath supports one release of path-based API clients. New code
// must retain node IDs because names may themselves contain path separators.
func (s *Store) ResolveLegacyPath(ctx context.Context, legacyPath string) (Node, error) {
	legacyPath = strings.TrimSpace(strings.ReplaceAll(legacyPath, `\`, "/"))
	legacyPath = strings.TrimPrefix(legacyPath, VirtualRoot)
	legacyPath = strings.Trim(legacyPath, "/")
	if legacyPath == "" {
		return Node{}, fmt.Errorf("%w: workspace root has no node ID", ErrInvalidNode)
	}
	parentID := ""
	var node Node
	for _, rawSegment := range strings.Split(legacyPath, "/") {
		segment, err := url.PathUnescape(rawSegment)
		if err != nil {
			return Node{}, fmt.Errorf("decoding legacy workspace path: %w", err)
		}
		node, err = scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes
			WHERE parent_id = ? AND name = ? AND deleted_at IS NULL`, parentID, segment))
		if errors.Is(err, sql.ErrNoRows) {
			return Node{}, ErrNotFound
		}
		if err != nil {
			return Node{}, fmt.Errorf("resolving legacy workspace path: %w", err)
		}
		parentID = node.ID
	}
	s.attachDerivedPaths(ctx, &node)
	return node, nil
}

// Walk returns roots and descendants in deterministic pre-order without ever
// materializing database content.
func (s *Store) Walk(ctx context.Context, rootIDs []string) ([]Node, error) {
	if len(rootIDs) == 0 {
		roots, err := s.List(ctx, "")
		if err != nil {
			return nil, err
		}
		for _, root := range roots {
			rootIDs = append(rootIDs, root.ID)
		}
	}
	result := make([]Node, 0)
	var visit func(string) error
	visit = func(id string) error {
		node, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		result = append(result, node)
		if node.Type != "directory" {
			return nil
		}
		children, err := s.List(ctx, node.ID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := visit(child.ID); err != nil {
				return err
			}
		}
		return nil
	}
	for _, id := range rootIDs {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Store) CreateDirectory(ctx context.Context, parentID, name string) (Node, error) {
	if err := ValidateName(name); err != nil {
		return Node{}, err
	}
	if err := s.ensureParent(ctx, s.db, parentID); err != nil {
		return Node{}, err
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	id := uuid.NewString()
	physicalDir := filepath.Join(s.root, id)
	if err := os.Mkdir(physicalDir, 0750); err != nil {
		return Node{}, fmt.Errorf("creating workspace directory storage: %w", err)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO workspace_nodes
		(id, parent_id, name, node_type, mime_type, size_bytes, content_hash, relative_path, version, created_at, updated_at)
		VALUES (?, ?, ?, 'directory', 'inode/directory', 0, '', '', 1, ?, ?)`, id, parentID, name, now, now)
	if err != nil {
		_ = os.Remove(physicalDir)
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Node{}, ErrConflict
		}
		return Node{}, fmt.Errorf("creating workspace directory: %w", err)
	}
	s.refreshNamed(ctx)
	return s.Get(ctx, id)
}

func (s *Store) Write(ctx context.Context, req WriteRequest) (Node, error) {
	if req.ID == "" && ValidateName(req.Name) != nil {
		return Node{}, ErrInvalidName
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, fmt.Errorf("beginning workspace write: %w", err)
	}
	defer tx.Rollback()

	now := s.now().UTC().Format(time.RFC3339Nano)
	hash := hashContent(req.Content)
	nodeID := req.ID
	version := int64(1)
	name := req.Name
	mimeType := req.MIMEType
	parentID := req.ParentID
	var relativePath string
	if nodeID == "" {
		if err := s.ensureParent(ctx, tx, req.ParentID); err != nil {
			return Node{}, err
		}
		nodeID = uuid.NewString()
		mimeType = detectMIME(name, mimeType, req.Content)
		relativePath, err = storageRelativePath(parentID, nodeID)
		if err != nil {
			return Node{}, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO workspace_nodes
			(id, parent_id, name, node_type, mime_type, size_bytes, content_hash, relative_path, version, created_at, updated_at)
			VALUES (?, ?, ?, 'file', ?, ?, ?, ?, 1, ?, ?)`, nodeID, req.ParentID, name, mimeType, len(req.Content), hash, relativePath, now, now)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return Node{}, ErrConflict
			}
			return Node{}, fmt.Errorf("creating workspace file: %w", err)
		}
	} else {
		var nodeType string
		var deleted sql.NullString
		err = tx.QueryRowContext(ctx, `SELECT name, parent_id, node_type, mime_type, version,
			deleted_at, relative_path FROM workspace_nodes WHERE id = ?`, nodeID).
			Scan(&name, &parentID, &nodeType, &mimeType, &version, &deleted, &relativePath)
		if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
			return Node{}, ErrNotFound
		}
		if err != nil {
			return Node{}, fmt.Errorf("loading workspace file for update: %w", err)
		}
		if nodeType != "file" {
			return Node{}, fmt.Errorf("%w: node is not a file", ErrInvalidNode)
		}
		if req.ExpectedVersion > 0 && req.ExpectedVersion != version {
			return Node{}, ErrVersion
		}
		version++
		mimeType = detectMIME(name, req.MIMEType, req.Content)
		result, updateErr := tx.ExecContext(ctx, `UPDATE workspace_nodes SET mime_type = ?, size_bytes = ?,
			content_hash = ?, version = ?, updated_at = ? WHERE id = ? AND deleted_at IS NULL`,
			mimeType, len(req.Content), hash, version, now, nodeID)
		if updateErr != nil {
			return Node{}, fmt.Errorf("updating workspace file: %w", updateErr)
		}
		if rows, _ := result.RowsAffected(); rows != 1 {
			return Node{}, ErrNotFound
		}
	}

	target, err := s.storagePath(relativePath)
	if err != nil {
		return Node{}, err
	}
	staged, err := stageWorkspaceFile(target, req.Content)
	if err != nil {
		return Node{}, fmt.Errorf("staging workspace file: %w", err)
	}
	defer os.Remove(staged)

	if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id = ?`, nodeID); err != nil {
		return Node{}, fmt.Errorf("clearing workspace search document: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspace_fts(node_id, name) VALUES (?, ?)`, nodeID, name); err != nil {
		return Node{}, fmt.Errorf("indexing workspace file: %w", err)
	}
	rollbackFile, finalizeFile, err := installWorkspaceFile(staged, target)
	if err != nil {
		return Node{}, fmt.Errorf("installing workspace file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		rollbackFile()
		return Node{}, fmt.Errorf("committing workspace write: %w", err)
	}
	finalizeFile()
	s.refreshNamed(ctx)
	return s.Get(ctx, nodeID)
}

func (s *Store) Read(ctx context.Context, id string, offset, limit int64) (Node, []byte, error) {
	if offset < 0 || limit < 0 {
		return Node{}, nil, fmt.Errorf("%w: negative read range", ErrInvalidNode)
	}
	node, file, err := s.Open(ctx, id)
	if err != nil {
		return Node{}, nil, err
	}
	defer file.Close()
	if offset >= node.SizeBytes {
		return node, []byte{}, nil
	}
	if limit == 0 || offset+limit > node.SizeBytes {
		limit = node.SizeBytes - offset
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return Node{}, nil, fmt.Errorf("seeking workspace file: %w", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return Node{}, nil, fmt.Errorf("reading workspace file: %w", err)
	}
	return node, content, nil
}

// Open returns the current immutable file handle for streaming callers.
func (s *Store) Open(ctx context.Context, id string) (Node, *os.File, error) {
	node, err := s.Get(ctx, id)
	if err != nil {
		return Node{}, nil, err
	}
	if node.Type != "file" {
		return Node{}, nil, fmt.Errorf("%w: node is not a file", ErrInvalidNode)
	}
	target, err := s.storagePath(node.RelativePath)
	if err != nil {
		return Node{}, nil, err
	}
	file, err := os.Open(target)
	if errors.Is(err, os.ErrNotExist) {
		return Node{}, nil, ErrNotFound
	}
	if err != nil {
		return Node{}, nil, fmt.Errorf("opening workspace file: %w", err)
	}
	return node, file, nil
}
func stageWorkspaceFile(target string, content []byte) (string, error) {
	if err := os.MkdirAll(filepath.Dir(target), 0750); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(target), ".workspace-*")
	if err != nil {
		return "", err
	}
	name := temporary.Name()
	if err := temporary.Chmod(0640); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

func installWorkspaceFile(staged, target string) (rollback func(), finalize func(), err error) {
	backup := target + ".backup-" + uuid.NewString()
	hadOriginal := false
	if err := os.Rename(target, backup); err == nil {
		hadOriginal = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, nil, err
	}
	if err := os.Rename(staged, target); err != nil {
		if hadOriginal {
			_ = os.Rename(backup, target)
		}
		return nil, nil, err
	}
	rollback = func() {
		_ = os.Remove(target)
		if hadOriginal {
			_ = os.Rename(backup, target)
		}
	}
	finalize = func() {
		if hadOriginal {
			_ = os.Remove(backup)
		}
	}
	return rollback, finalize, nil
}

func (s *Store) Rename(ctx context.Context, id, newParentID, newName string, expectedVersion int64) (Node, error) {
	if err := ValidateName(newName); err != nil {
		return Node{}, err
	}
	if newParentID == id {
		return Node{}, fmt.Errorf("%w: node cannot be its own parent", ErrInvalidNode)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Node{}, fmt.Errorf("beginning workspace rename: %w", err)
	}
	defer tx.Rollback()
	if err := s.ensureParent(ctx, tx, newParentID); err != nil {
		return Node{}, err
	}
	if newParentID != "" {
		var descendant int
		err := tx.QueryRowContext(ctx, `WITH RECURSIVE descendants(id) AS (
			SELECT id FROM workspace_nodes WHERE parent_id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN descendants d ON n.parent_id = d.id WHERE n.deleted_at IS NULL
		) SELECT COUNT(*) FROM descendants WHERE id = ?`, id, newParentID).Scan(&descendant)
		if err != nil {
			return Node{}, fmt.Errorf("checking workspace move: %w", err)
		}
		if descendant > 0 {
			return Node{}, fmt.Errorf("%w: cannot move a directory into its descendant", ErrInvalidNode)
		}
	}
	var nodeType string
	var version int64
	var deleted sql.NullString
	var currentRelativePath string
	err = tx.QueryRowContext(ctx, `SELECT node_type, version, deleted_at, relative_path
		FROM workspace_nodes WHERE id = ?`, id).Scan(&nodeType, &version, &deleted, &currentRelativePath)
	if errors.Is(err, sql.ErrNoRows) || deleted.Valid {
		return Node{}, ErrNotFound
	}
	if err != nil {
		return Node{}, fmt.Errorf("loading workspace node for rename: %w", err)
	}
	if expectedVersion > 0 && version != expectedVersion {
		return Node{}, ErrVersion
	}
	now := s.now().UTC().Format(time.RFC3339Nano)
	newRelativePath := currentRelativePath
	if nodeType == "file" {
		newRelativePath, err = storageRelativePath(newParentID, id)
		if err != nil {
			return Node{}, err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE workspace_nodes SET parent_id = ?, name = ?, relative_path = ?, version = version + 1,
		updated_at = ? WHERE id = ? AND deleted_at IS NULL`, newParentID, newName, newRelativePath, now, id)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Node{}, ErrConflict
		}
		return Node{}, fmt.Errorf("renaming workspace node: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return Node{}, ErrNotFound
	}
	var rollbackMove = func() {}
	if nodeType == "file" {
		if newRelativePath != currentRelativePath {
			currentTarget, err := s.storagePath(currentRelativePath)
			if err != nil {
				return Node{}, err
			}
			newTarget, err := s.storagePath(newRelativePath)
			if err != nil {
				return Node{}, err
			}
			if err := os.MkdirAll(filepath.Dir(newTarget), 0750); err != nil {
				return Node{}, fmt.Errorf("creating workspace move target: %w", err)
			}
			if err := os.Rename(currentTarget, newTarget); err != nil {
				return Node{}, fmt.Errorf("moving workspace file: %w", err)
			}
			rollbackMove = func() { _ = os.Rename(newTarget, currentTarget) }
		}
		if _, err := tx.ExecContext(ctx, `UPDATE workspace_fts SET name = ? WHERE node_id = ?`, newName, id); err != nil {
			rollbackMove()
			return Node{}, fmt.Errorf("updating workspace search name: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		rollbackMove()
		return Node{}, fmt.Errorf("committing workspace rename: %w", err)
	}
	s.refreshNamed(ctx)
	return s.Get(ctx, id)
}

func (s *Store) Delete(ctx context.Context, id string, expectedVersion int64, recursive bool) error {
	node, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	if expectedVersion > 0 && node.Version != expectedVersion {
		return ErrVersion
	}
	if node.Type == "directory" && !recursive {
		var count int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_nodes WHERE parent_id = ? AND deleted_at IS NULL`, id).Scan(&count); err != nil {
			return fmt.Errorf("checking workspace directory contents: %w", err)
		}
		if count > 0 {
			return fmt.Errorf("%w: directory is not empty", ErrConflict)
		}
	}
	storagePaths, directoryIDs, err := s.deletionStorageTargets(ctx, id, recursive)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning workspace delete: %w", err)
	}
	defer tx.Rollback()
	now := s.now().UTC().Format(time.RFC3339Nano)
	if recursive {
		if _, err := tx.ExecContext(ctx, `WITH RECURSIVE targets(id) AS (
			SELECT id FROM workspace_nodes WHERE id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id WHERE n.deleted_at IS NULL
		) UPDATE workspace_nodes SET deleted_at = ?, updated_at = ?, version = version + 1 WHERE id IN (SELECT id FROM targets)`, id, now, now); err != nil {
			return fmt.Errorf("deleting workspace tree: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id IN (
			WITH RECURSIVE targets(id) AS (
				SELECT id FROM workspace_nodes WHERE id = ?
				UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id
			) SELECT id FROM targets
		)`, id); err != nil {
			return fmt.Errorf("removing workspace search tree: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE workspace_nodes SET deleted_at = ?, updated_at = ?,
			version = version + 1 WHERE id = ? AND deleted_at IS NULL`, now, now, id); err != nil {
			return fmt.Errorf("deleting workspace node: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_fts WHERE node_id = ?`, id); err != nil {
			return fmt.Errorf("removing workspace search document: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing workspace delete: %w", err)
	}
	for _, relativePath := range storagePaths {
		target, pathErr := s.storagePath(relativePath)
		if pathErr != nil {
			return pathErr
		}
		if removeErr := os.Remove(target); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("removing workspace file: %w", removeErr)
		}
	}
	for _, directoryID := range directoryIDs {
		if removeErr := os.RemoveAll(filepath.Join(s.root, directoryID)); removeErr != nil {
			return fmt.Errorf("removing workspace directory storage: %w", removeErr)
		}
	}
	s.refreshNamed(ctx)
	return nil
}

func (s *Store) deletionStorageTargets(ctx context.Context, id string, recursive bool) ([]string, []string, error) {
	query := `SELECT n.id, n.node_type, n.relative_path
		FROM workspace_nodes n WHERE n.id = ? AND n.deleted_at IS NULL`
	if recursive {
		query = `WITH RECURSIVE targets(id) AS (
			SELECT id FROM workspace_nodes WHERE id = ? AND deleted_at IS NULL
			UNION ALL SELECT n.id FROM workspace_nodes n JOIN targets t ON n.parent_id = t.id WHERE n.deleted_at IS NULL
		) SELECT n.id, n.node_type, n.relative_path
			FROM workspace_nodes n JOIN targets t ON t.id = n.id`
	}
	rows, err := s.db.QueryContext(ctx, query, id)
	if err != nil {
		return nil, nil, fmt.Errorf("loading workspace deletion targets: %w", err)
	}
	defer rows.Close()
	paths := make([]string, 0)
	directories := make([]string, 0)
	for rows.Next() {
		var nodeID, nodeType, relativePath string
		if err := rows.Scan(&nodeID, &nodeType, &relativePath); err != nil {
			return nil, nil, fmt.Errorf("scanning workspace deletion target: %w", err)
		}
		if nodeType == "directory" {
			directories = append(directories, nodeID)
		} else if relativePath != "" {
			paths = append(paths, relativePath)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating workspace deletion targets: %w", err)
	}
	return paths, directories, nil
}

func ftsQuery(query string) string {
	parts := strings.Fields(query)
	quoted := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.ReplaceAll(part, `"`, `""`)
		if part != "" {
			quoted = append(quoted, `"`+part+`"*`)
		}
	}
	return strings.Join(quoted, " AND ")
}

func (s *Store) Search(ctx context.Context, query, parentID string, limit int) ([]SearchResult, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		nodes, err := s.List(ctx, parentID)
		if len(nodes) > limit {
			nodes = nodes[:limit]
		}
		results := make([]SearchResult, 0, len(nodes))
		for _, node := range nodes {
			results = append(results, SearchResult{Node: node})
		}
		return results, err
	}
	match := ftsQuery(query)
	like := "%" + strings.ToLower(query) + "%"
	rows, err := s.db.QueryContext(ctx, `SELECT `+qualifiedNodeColumns+`, ''
		FROM workspace_nodes n
		WHERE n.deleted_at IS NULL AND (? = '' OR n.parent_id = ?)
		AND (lower(n.name) LIKE ? OR n.id IN (SELECT node_id FROM workspace_fts WHERE workspace_fts MATCH ?))
		ORDER BY CASE WHEN lower(n.name) LIKE ? THEN 0 ELSE 1 END, n.updated_at DESC LIMIT ?`,
		parentID, parentID, like, match, like, limit)
	if err != nil {
		return nil, fmt.Errorf("searching workspace: %w", err)
	}
	results := make([]SearchResult, 0)
	for rows.Next() {
		var result SearchResult
		var deleted sql.NullString
		if err := rows.Scan(&result.ID, &result.ParentID, &result.Name, &result.Type, &result.MIMEType,
			&result.SizeBytes, &result.ContentHash, &result.RelativePath, &result.Version, &result.CreatedAt, &result.UpdatedAt,
			&deleted, &result.Snippet); err != nil {
			return nil, fmt.Errorf("scanning workspace search result: %w", err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterating workspace search results: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("closing workspace search rows: %w", err)
	}
	for index := range results {
		s.attachDerivedPaths(ctx, &results[index].Node)
	}
	return results, nil
}

func (s *Store) Duplicate(ctx context.Context, id, parentID, name, actorID string) (Node, error) {
	node, content, err := s.Read(ctx, id, 0, 0)
	if err != nil {
		return Node{}, err
	}
	if name == "" {
		name = node.Name + " copy"
	}
	return s.Write(ctx, WriteRequest{ParentID: parentID, Name: name, Content: content, MIMEType: node.MIMEType, ActorID: actorID})
}

func MediaKind(mimeType string) string {
	switch {
	case mimeType == "inode/directory":
		return "dir"
	case strings.HasPrefix(mimeType, "image/"):
		return "image"
	case mimeType == "application/pdf":
		return "pdf"
	case strings.Contains(mimeType, "json"):
		return "json"
	case strings.Contains(mimeType, "csv") || strings.Contains(mimeType, "tab-separated"):
		return "csv"
	case strings.HasPrefix(mimeType, "text/") || strings.Contains(mimeType, "xml") || strings.Contains(mimeType, "javascript"):
		return "text"
	case strings.HasPrefix(mimeType, "audio/"):
		return "audio"
	case strings.HasPrefix(mimeType, "video/"):
		return "video"
	case strings.Contains(mimeType, "zip") || strings.Contains(mimeType, "archive") || strings.Contains(mimeType, "compressed"):
		return "archive"
	default:
		return "binary"
	}
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	stats := Stats{Breakdown: map[string]int64{"documents": 0, "code": 0, "data": 0, "media": 0, "other": 0}}
	if err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN node_type = 'file' THEN size_bytes ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN node_type = 'file' THEN 1 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN node_type = 'directory' THEN 1 ELSE 0 END), 0)
		FROM workspace_nodes WHERE deleted_at IS NULL`).Scan(&stats.TotalSize, &stats.TotalFiles, &stats.TotalDirectories); err != nil {
		return Stats{}, fmt.Errorf("computing workspace stats: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT mime_type, COALESCE(SUM(size_bytes), 0)
		FROM workspace_nodes WHERE deleted_at IS NULL AND node_type = 'file' GROUP BY mime_type`)
	if err != nil {
		return Stats{}, fmt.Errorf("computing workspace breakdown: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var mimeType string
		var size int64
		if err := rows.Scan(&mimeType, &size); err != nil {
			return Stats{}, fmt.Errorf("scanning workspace breakdown: %w", err)
		}
		category := "other"
		switch {
		case strings.HasPrefix(mimeType, "image/"), strings.HasPrefix(mimeType, "audio/"), strings.HasPrefix(mimeType, "video/"):
			category = "media"
		case strings.Contains(mimeType, "json"), strings.Contains(mimeType, "csv"), strings.Contains(mimeType, "xml"), strings.Contains(mimeType, "sql"):
			category = "data"
		case strings.HasPrefix(mimeType, "text/"):
			category = "documents"
		case strings.Contains(mimeType, "pdf"), strings.Contains(mimeType, "officedocument"):
			category = "documents"
		}
		stats.Breakdown[category] += size
	}
	return stats, rows.Err()
}
