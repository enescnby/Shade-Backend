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

type InboxMessage struct {
	MessageID   string `json:"message_id"`
	SenderID    string `json:"sender_id"`
	ReceiverID  string `json:"receiver_id,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	Ciphertext  []byte `json:"ciphertext"`
	Nonce       []byte `json:"nonce"`
	MessageType int32  `json:"message_type"`
	Timestamp   int64  `json:"timestamp"`
}

type InboxReceipt struct {
	MessageID  string `json:"message_id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	GroupID    string `json:"group_id,omitempty"`
	Status     string `json:"status"`
	Timestamp  int64  `json:"timestamp"`
}

type InboxResponse struct {
	Messages []InboxMessage `json:"messages"`
	Receipts []InboxReceipt `json:"receipts"`
}
