package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"
)

const (
	// NamedDirName is the hidden projection of user-visible names inside the
	// UUID content-addressed store. Agents never see this directory name;
	// native_exec mounts or links it as ACTONOS_USER_WORKSPACE / user-workspace.
	NamedDirName = ".named"
	// AgentViewName is the reserved scratchpad directory that points at the
	// named projection so relative paths work from native_exec's cwd.
	AgentViewName = "user-workspace"
	// SandboxUserWorkspace is the absolute path of the named projection inside
	// the Linux bubblewrap sandbox.
	SandboxUserWorkspace = VirtualRoot
)

var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// ReconcileReport describes named-projection files that were ingested or
// updated after a native_exec session.
type ReconcileReport struct {
	Updated []string `json:"updated,omitempty"`
	Created []string `json:"created,omitempty"`
}

// NamedRoot returns the host path of the original-name projection.
func (s *Store) NamedRoot() string {
	if s == nil {
		return ""
	}
	return s.namedRoot
}

func (s *Store) lookupExecPath(id string) string {
	if s == nil || id == "" {
		return ""
	}
	s.namedMu.RLock()
	defer s.namedMu.RUnlock()
	return s.execByID[id]
}

func (s *Store) attachDerivedPaths(ctx context.Context, node *Node) {
	if node == nil || node.ID == "" {
		return
	}
	if node.VirtualPath == "" {
		node.VirtualPath, _ = s.VirtualPath(ctx, node.ID)
	}
	node.ExecPath = s.lookupExecPath(node.ID)
}

// ResolveRef locates a node by opaque ID, virtual path, exec path, or
// user-workspace-relative path.
func (s *Store) ResolveRef(ctx context.Context, ref string) (Node, error) {
	ref = strings.TrimSpace(strings.ReplaceAll(ref, `\`, "/"))
	if ref == "" {
		return Node{}, fmt.Errorf("%w: empty workspace reference", ErrInvalidNode)
	}
	if node, err := s.Get(ctx, ref); err == nil {
		return node, nil
	}
	trimmed := strings.TrimPrefix(ref, VirtualRoot)
	trimmed = strings.TrimPrefix(trimmed, "/")
	trimmed = strings.TrimPrefix(trimmed, AgentViewName+"/")
	trimmed = strings.TrimPrefix(trimmed, AgentViewName)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return Node{}, fmt.Errorf("%w: workspace root has no node ID", ErrInvalidNode)
	}
	if node, err := s.ResolveLegacyPath(ctx, trimmed); err == nil {
		return node, nil
	}
	s.namedMu.RLock()
	id := s.idByExec[trimmed]
	s.namedMu.RUnlock()
	if id == "" {
		return Node{}, ErrNotFound
	}
	return s.Get(ctx, id)
}

func (s *Store) refreshNamed(ctx context.Context) {
	if s == nil {
		return
	}
	s.namedMu.Lock()
	defer s.namedMu.Unlock()
	if s.namedPaused {
		return
	}
	if err := s.rebuildNamedLocked(ctx); err != nil {
		slog.Warn("refreshing named workspace projection", "error", err)
	}
}

func (s *Store) rebuildNamedLocked(ctx context.Context) error {
	if s.namedRoot == "" {
		return nil
	}
	if err := os.MkdirAll(s.namedRoot, 0750); err != nil {
		return fmt.Errorf("creating named workspace projection: %w", err)
	}
	nodes, err := s.listActiveNodes(ctx)
	if err != nil {
		return err
	}
	previous := s.idByExec
	execByID, idByExec := assignExecPaths(nodes)
	s.execByID = execByID
	s.idByExec = idByExec

	desired := make(map[string]struct{}, len(execByID))
	for id, rel := range execByID {
		node := nodes[id]
		target := filepath.Join(s.namedRoot, filepath.FromSlash(rel))
		desired[rel] = struct{}{}
		if node.Type == "directory" {
			if err := os.MkdirAll(target, 0750); err != nil {
				return fmt.Errorf("creating named workspace directory %s: %w", rel, err)
			}
			continue
		}
		if node.RelativePath == "" {
			continue
		}
		source, err := s.storagePath(node.RelativePath)
		if err != nil {
			return err
		}
		if err := linkNamedFile(source, target); err != nil {
			return fmt.Errorf("linking named workspace file %s: %w", rel, err)
		}
	}
	return removeNamedExtras(s.namedRoot, previous, desired)
}

func (s *Store) listActiveNodes(ctx context.Context) (map[string]Node, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, fmt.Errorf("listing active workspace nodes: %w", err)
	}
	defer rows.Close()
	nodes := make(map[string]Node)
	for rows.Next() {
		node, scanErr := scanNode(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scanning active workspace node: %w", scanErr)
		}
		nodes[node.ID] = node
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating active workspace nodes: %w", err)
	}
	return nodes, nil
}

func assignExecPaths(nodes map[string]Node) (map[string]string, map[string]string) {
	children := make(map[string][]Node)
	for _, node := range nodes {
		children[node.ParentID] = append(children[node.ParentID], node)
	}
	for parent := range children {
		sort.Slice(children[parent], func(i, j int) bool {
			if children[parent][i].ID == children[parent][j].ID {
				return children[parent][i].Name < children[parent][j].Name
			}
			return children[parent][i].ID < children[parent][j].ID
		})
	}
	execByID := make(map[string]string, len(nodes))
	idByExec := make(map[string]string, len(nodes))
	var visit func(parentID, parentRel string)
	visit = func(parentID, parentRel string) {
		used := make(map[string]string, len(children[parentID]))
		for _, child := range children[parentID] {
			name := uniqueExecName(child.Name, child.ID, used)
			rel := name
			if parentRel != "" {
				rel = parentRel + "/" + name
			}
			execByID[child.ID] = rel
			idByExec[rel] = child.ID
			if child.Type == "directory" {
				visit(child.ID, rel)
			}
		}
	}
	visit("", "")
	return execByID, idByExec
}

func uniqueExecName(name, id string, used map[string]string) string {
	safe := sanitizeFSName(name)
	if existing, ok := used[safe]; ok && existing != id {
		suffix := strings.ReplaceAll(id, "-", "")
		if len(suffix) > 8 {
			suffix = suffix[:8]
		}
		safe = safe + "-" + suffix
	}
	used[safe] = id
	return safe
}

func sanitizeFSName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || name == "." {
		return "unnamed"
	}
	if name == ".." {
		return "unnamed-dotdot"
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r == 0 || r < 32 || !unicode.IsPrint(r):
			b.WriteByte('_')
		case strings.ContainsRune(`\/:*?"<>|`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.TrimRight(b.String(), " .")
	if out == "" {
		out = "unnamed"
	}
	stem := out
	ext := ""
	if i := strings.LastIndex(out, "."); i > 0 {
		stem = out[:i]
		ext = out[i:]
	}
	if _, reserved := windowsReservedNames[strings.ToUpper(stem)]; reserved {
		out = "_" + out
	}
	if len(out) > 240 {
		keepExt := ext
		if len(keepExt) > 16 {
			keepExt = keepExt[:16]
		}
		out = strings.TrimRight(out[:240-len(keepExt)], " .") + keepExt
	}
	return out
}

func linkNamedFile(source, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0750); err != nil {
		return err
	}
	if sameNamedFile(source, dest) {
		return nil
	}
	_ = os.Remove(dest)
	if err := os.Link(source, dest); err == nil {
		return nil
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dest)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	return nil
}

func sameNamedFile(source, dest string) bool {
	srcInfo, srcErr := os.Stat(source)
	dstInfo, dstErr := os.Stat(dest)
	if srcErr != nil || dstErr != nil {
		return false
	}
	return os.SameFile(srcInfo, dstInfo)
}

func removeNamedExtras(root string, previous map[string]string, desired map[string]struct{}) error {
	if previous == nil {
		previous = map[string]string{}
	}
	var extras []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if _, ok := desired[rel]; ok {
			return nil
		}
		if entry.IsDir() {
			for want := range desired {
				if want == rel || strings.HasPrefix(want, rel+"/") {
					return nil
				}
			}
		}
		_, wasTracked := previous[rel]
		if !wasTracked && !ignoredNamedExtra(rel) {
			return nil
		}
		extras = append(extras, path)
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	for i := len(extras) - 1; i >= 0; i-- {
		if removeErr := os.RemoveAll(extras[i]); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("removing stale named workspace path: %w", removeErr)
		}
	}
	return nil
}

func ignoredNamedExtra(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		lower := strings.ToLower(part)
		if lower == "__pycache__" || lower == ".ds_store" || lower == "thumbs.db" {
			return true
		}
		if strings.HasPrefix(part, ".") || strings.HasSuffix(lower, ".pyc") {
			return true
		}
	}
	return false
}

// EnsureAgentView creates user-workspace inside the agent scratchpad so relative
// native_exec commands can open original filenames.
func (s *Store) EnsureAgentView(agentWorkspace string) error {
	if s == nil || s.namedRoot == "" || agentWorkspace == "" {
		return nil
	}
	if err := os.MkdirAll(agentWorkspace, 0750); err != nil {
		return fmt.Errorf("creating agent workspace: %w", err)
	}
	if err := os.MkdirAll(s.namedRoot, 0750); err != nil {
		return fmt.Errorf("creating named workspace projection: %w", err)
	}
	link := filepath.Join(agentWorkspace, AgentViewName)
	if err := replaceAgentView(link, s.namedRoot); err != nil {
		return fmt.Errorf("linking agent user-workspace view: %w", err)
	}
	return nil
}

func replaceAgentView(link, target string) error {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if dest, err := os.Readlink(link); err == nil {
		if dest == absTarget || dest == target {
			return nil
		}
	}
	if err := removeAgentView(link); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(absTarget, link); err == nil {
		return nil
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "mklink", "/J", link, absTarget)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("creating user-workspace junction: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	}
	return fmt.Errorf("creating user-workspace symlink at %s", link)
}

func removeAgentView(path string) error {
	if _, err := os.Lstat(path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "rmdir", path).CombinedOutput(); err == nil {
			return nil
		} else if _, statErr := os.Lstat(path); statErr == nil {
			_ = out
		}
	}
	if err := os.Remove(path); err == nil || os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(path)
}

// ReconcileNamed ingests files created or modified through the named projection
// (typically by native_exec / python) back into the metadata store.
func (s *Store) ReconcileNamed(ctx context.Context, actorID string) (ReconcileReport, error) {
	var report ReconcileReport
	if s == nil || s.namedRoot == "" {
		return report, nil
	}
	s.namedMu.Lock()
	idByExec := make(map[string]string, len(s.idByExec))
	for rel, id := range s.idByExec {
		idByExec[rel] = id
	}
	root := s.namedRoot
	s.namedPaused = true
	s.namedMu.Unlock()
	defer func() {
		s.namedMu.Lock()
		s.namedPaused = false
		s.namedMu.Unlock()
		s.refreshNamed(ctx)
	}()

	type pending struct {
		rel     string
		id      string
		content []byte
	}
	var files []pending
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		id := idByExec[rel]
		if id == "" && ignoredNamedExtra(rel) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		files = append(files, pending{rel: rel, id: id, content: content})
		return nil
	})

	for _, file := range files {
		if file.id != "" {
			node, err := s.Get(ctx, file.id)
			if err != nil {
				continue
			}
			if node.Type != "file" {
				continue
			}
			if hashContent(file.content) == node.ContentHash && int64(len(file.content)) == node.SizeBytes {
				continue
			}
			if _, err := s.Write(ctx, WriteRequest{ID: file.id, Content: file.content, ActorID: actorID}); err != nil {
				return report, fmt.Errorf("syncing named workspace file %s: %w", file.rel, err)
			}
			report.Updated = append(report.Updated, file.id)
			continue
		}
		node, err := s.ingestNamedFile(ctx, file.rel, file.content, actorID)
		if err != nil {
			return report, fmt.Errorf("ingesting named workspace file %s: %w", file.rel, err)
		}
		report.Created = append(report.Created, node.ID)
	}
	return report, nil
}

func (s *Store) ingestNamedFile(ctx context.Context, rel string, content []byte, actorID string) (Node, error) {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	segments := strings.Split(rel, "/")
	if len(segments) == 0 || segments[0] == "" {
		return Node{}, fmt.Errorf("%w: empty named path", ErrInvalidNode)
	}
	parentID := ""
	for _, dirName := range segments[:len(segments)-1] {
		existing, err := scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes
			WHERE parent_id = ? AND name = ? AND deleted_at IS NULL`, parentID, dirName))
		if err == nil {
			if existing.Type != "directory" {
				return Node{}, fmt.Errorf("%w: parent %q is not a directory", ErrInvalidNode, dirName)
			}
			parentID = existing.ID
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Node{}, fmt.Errorf("checking named workspace directory %q: %w", dirName, err)
		}
		created, createErr := s.CreateDirectory(ctx, parentID, dirName)
		if createErr != nil {
			return Node{}, createErr
		}
		parentID = created.ID
	}
	name := segments[len(segments)-1]
	existing, err := scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeColumns+` FROM workspace_nodes
		WHERE parent_id = ? AND name = ? AND deleted_at IS NULL`, parentID, name))
	if err == nil {
		if existing.Type != "file" {
			return Node{}, fmt.Errorf("%w: %q is not a file", ErrInvalidNode, name)
		}
		return s.Write(ctx, WriteRequest{ID: existing.ID, Content: content, ActorID: actorID})
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Node{}, fmt.Errorf("checking named workspace file %q: %w", name, err)
	}
	return s.Write(ctx, WriteRequest{
		ParentID: parentID,
		Name:     name,
		Content:  content,
		ActorID:  actorID,
	})
}
