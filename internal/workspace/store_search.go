package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

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
