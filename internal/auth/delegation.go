package auth

// RiskLevel defines the approval requirement for an automated SaaS action.
type RiskLevel string

const (
	RiskLow    RiskLevel = "Low"    // Read-only (e.g., Read email, Search Notion) -> Auto-execute 100%
	RiskMedium RiskLevel = "Medium" // Scoped write (e.g., Draft email, Create Notion page) -> Auto-execute within workspace
	RiskHigh   RiskLevel = "High"   // Destructive / Finance / Push code -> Human-in-the-loop approval required
)

// ScopeDefinition describes an OAuth permission scope and its risk tier.
type ScopeDefinition struct {
	Scope       string    `json:"scope"`
	Service     string    `json:"service"`
	Description string    `json:"description"`
	Risk        RiskLevel `json:"risk_level"`
}

// Predefined OAuth scopes for supported SaaS connectors.
var PredefinedScopes = map[string][]ScopeDefinition{
	"google": {
		{Scope: "https://www.googleapis.com/auth/gmail.readonly", Service: "Gmail", Description: "Read email messages", Risk: RiskLow},
		{Scope: "https://www.googleapis.com/auth/gmail.compose", Service: "Gmail", Description: "Create and edit email drafts", Risk: RiskMedium},
		{Scope: "https://www.googleapis.com/auth/gmail.send", Service: "Gmail", Description: "Send email messages", Risk: RiskHigh},
		{Scope: "https://www.googleapis.com/auth/calendar.readonly", Service: "Calendar", Description: "View calendar events", Risk: RiskLow},
		{Scope: "https://www.googleapis.com/auth/calendar.events", Service: "Calendar", Description: "Create and edit calendar events", Risk: RiskMedium},
		{Scope: "https://www.googleapis.com/auth/drive.readonly", Service: "Drive", Description: "View Google Drive files", Risk: RiskLow},
		{Scope: "https://www.googleapis.com/auth/drive.file", Service: "Drive", Description: "Create and edit selected Drive files", Risk: RiskMedium},
	},
	"github": {
		{Scope: "repo:read", Service: "GitHub", Description: "Read public and private repositories", Risk: RiskLow},
		{Scope: "repo", Service: "GitHub", Description: "Full control of private repositories", Risk: RiskHigh},
		{Scope: "issues:write", Service: "GitHub", Description: "Create and edit repository issues", Risk: RiskMedium},
	},
	"notion": {
		{Scope: "read_content", Service: "Notion", Description: "Read pages and databases", Risk: RiskLow},
		{Scope: "insert_content", Service: "Notion", Description: "Insert and update pages", Risk: RiskMedium},
	},
	"slack": {
		{Scope: "channels:history", Service: "Slack", Description: "Read messages in public channels", Risk: RiskLow},
		{Scope: "chat:write", Service: "Slack", Description: "Send messages as agent", Risk: RiskMedium},
	},
}

// GetDefaultScopes returns default scopes for a given provider.
func GetDefaultScopes(provider string) []string {
	defs, ok := PredefinedScopes[provider]
	if !ok {
		return nil
	}
	var scopes []string
	for _, d := range defs {
		scopes = append(scopes, d.Scope)
	}
	return scopes
}
