package dto

type EncryptedMessageItem struct {
	MessageID     string `json:"message_id"`
	SenderID      string `json:"sender_id"`
	SenderShadeID string `json:"sender_shade_id"`
	Ciphertext    string `json:"ciphertext"`
	Nonce         string `json:"nonce"`
	MessageType   int    `json:"message_type"`
	KeyVersion    int    `json:"key_version"`
	CreatedAt     string `json:"created_at"`
}

type UndeliveredResponse struct {
	Messages []EncryptedMessageItem `json:"messages"`
}

type ReceiptRequest struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
}

type BatchReceiptRequest struct {
	Receipts []ReceiptRequest `json:"receipts"`
}

type PendingReceiptItem struct {
	MessageID string `json:"message_id"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"`
}

type PendingReceiptsResponse struct {
	Receipts []PendingReceiptItem `json:"receipts"`
}

type ReceiptEvent struct {
	MessageID     string
	SenderID      string
	SenderShadeID string
	ReceiverID    string
	Status        string
	Timestamp     int64
}
