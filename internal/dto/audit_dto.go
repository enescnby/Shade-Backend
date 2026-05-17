package dto

type AuditLogEntry struct {
	ActionType string `json:"action_type"`
	IPAddress  string `json:"ip_address"`
	Timestamp  string `json:"timestamp"`
}

type AuditLogsResponse struct {
	Logs []AuditLogEntry `json:"logs"`
}
