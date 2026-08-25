package channels

import (
	"path/filepath"
	"strings"
)

const (
	metaFileName  = "file_name"
	metaMIMEType  = "mime_type"
	metaMediaKind = "media_kind"
)

// MediaKindFromName classifies an attachment as photo, voice, or document for
// platform send APIs.
func MediaKindFromName(name, mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	ext := strings.ToLower(filepath.Ext(name))
	switch {
	case strings.HasPrefix(mimeType, "image/") && ext != ".svg" && ext != ".svgz":
		return "photo"
	case mimeType == "image/svg+xml":
		return "document"
	case strings.HasPrefix(mimeType, "audio/") || ext == ".ogg" || ext == ".mp3" || ext == ".wav" || ext == ".m4a":
		return "voice"
	case ext == ".png" || ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp":
		return "photo"
	default:
		return "document"
	}
}

// AttachMediaMetadata records original filename and MIME on an outbound envelope.
func AttachMediaMetadata(meta map[string]string, name, mimeType string) map[string]string {
	if meta == nil {
		meta = make(map[string]string)
	}
	if name != "" {
		meta[metaFileName] = name
		meta[metaMediaKind] = MediaKindFromName(name, mimeType)
	}
	if mimeType != "" {
		meta[metaMIMEType] = mimeType
	}
	return meta
}
