package dto

type InboxMessage struct {
	MessageID   string `json:"message_id"`
	SenderID    string `json:"sender_id"`
	ReceiverID  string `json:"receiver_id"`
	Ciphertext  []byte `json:"ciphertext"`
	Nonce       []byte `json:"nonce"`
	MessageType int32  `json:"message_type"`
	Timestamp   int64  `json:"timestamp"`
}

type InboxReceipt struct {
	MessageID  string `json:"message_id"`
	SenderID   string `json:"sender_id"`
	ReceiverID string `json:"receiver_id"`
	Status     string `json:"status"`
	Timestamp  int64  `json:"timestamp"`
}

type InboxResponse struct {
	Messages []InboxMessage `json:"messages"`
	Receipts []InboxReceipt `json:"receipts"`
}
