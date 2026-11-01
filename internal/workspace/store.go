package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
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
