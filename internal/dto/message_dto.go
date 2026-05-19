package dto

// ReceiptRequest — Android'den gelen tek bir makbuz (REST fallback)
type ReceiptRequest struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"` // "READ" veya "DELIVERED"
}

// BatchReceiptRequest — POST /messages/receipts body
type BatchReceiptRequest struct {
	Receipts []ReceiptRequest `json:"receipts"`
}

// InboxItem is a queued WebSocketMessage envelope, base64-encoded protobuf.
// Clients should decode and unmarshal into pb.WebSocketMessage — same as WS binary frames.
type InboxItem struct {
	Data string `json:"data"`
}

type InboxResponse struct {
	Items   []InboxItem `json:"items"`
	Count   int         `json:"count"`
	HasMore bool        `json:"has_more"`
}
