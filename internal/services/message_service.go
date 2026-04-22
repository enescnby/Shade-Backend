package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MessageService interface {
	GetUndeliveredMessages(ctx context.Context, userId string) (*dto.UndeliveredResponse, error)
	GetUserShadeID(ctx context.Context, userID string) (string, error)
	BuildReceiptEvents(ctx context.Context, userID string, messageIDs []string, status string) ([]dto.ReceiptEvent, error)
	SavePendingReceipts(ctx context.Context, events []dto.ReceiptEvent) error
	GetPendingReceipts(ctx context.Context, userID string) (*dto.PendingReceiptsResponse, error)
}

type messageService struct {
	msgRepo repositories.MessageRepository
	usrRepo repositories.UserRepository
}

func NewMessageService(msgRepo repositories.MessageRepository, usrRepo repositories.UserRepository) MessageService {
	return &messageService{msgRepo: msgRepo, usrRepo: usrRepo}
}

func (s *messageService) GetUndeliveredMessages(ctx context.Context, userID string) (*dto.UndeliveredResponse, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	messages, err := s.msgRepo.GetUndeliveredMessages(ctx, uid)
	if err != nil {
		logger.Log.Error("failed to get undelivered messages", zap.Error(err))
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(messages))
	items := make([]dto.EncryptedMessageItem, 0, len(messages))

	senderShadeIDCache := make(map[uuid.UUID]string)

	for _, msg := range messages {
		ids = append(ids, msg.MessageID)
		shadeID, ok := senderShadeIDCache[msg.SenderID]
		if !ok {
			sender, err := s.usrRepo.GetUserByID(msg.SenderID)
			if err != nil {
				continue
			}
			shadeID = sender.CoreGuardID
			senderShadeIDCache[msg.SenderID] = shadeID
		}
		items = append(items, dto.EncryptedMessageItem{
			MessageID:     msg.MessageID.String(),
			SenderID:      msg.SenderID.String(),
			SenderShadeID: shadeID,
			Ciphertext:    base64.StdEncoding.EncodeToString(msg.Ciphertext),
			Nonce:         base64.StdEncoding.EncodeToString(msg.Nonce),
			MessageType:   msg.MessageType,
			KeyVersion:    msg.KeyVersion,
			CreatedAt:     msg.CreatedAt.Format(time.RFC3339),
		})
	}

	if len(ids) > 0 {
		_ = s.msgRepo.MarkAsDelivered(ids)
	}

	return &dto.UndeliveredResponse{Messages: items}, nil
}

func (s *messageService) GetUserShadeID(ctx context.Context, userID string) (string, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return "", err
	}
	user, err := s.usrRepo.GetUserByID(uid)
	if err != nil {
		return "", err
	}
	return user.CoreGuardID, nil
}

func (s *messageService) BuildReceiptEvents(ctx context.Context, userID string, messageIDs []string, status string) ([]dto.ReceiptEvent, error) {
	if len(messageIDs) == 0 {
		return []dto.ReceiptEvent{}, nil
	}

	currentUserUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	ids := make([]uuid.UUID, 0, len(messageIDs))
	for _, rawID := range messageIDs {
		parsed, err := uuid.Parse(rawID)
		if err != nil {
			return nil, err
		}
		ids = append(ids, parsed)
	}

	messages, err := s.msgRepo.GetMessagesByIDsForReceiver(ctx, currentUserUUID, ids)
	if err != nil {
		logger.Log.Error("failed to build receipt events", zap.Error(err))
		return nil, err
	}

	user, err := s.usrRepo.GetUserByID(currentUserUUID)
	if err != nil {
		return nil, err
	}

	status = strings.ToUpper(strings.TrimSpace(status))
	now := time.Now().UnixMilli()

	events := make([]dto.ReceiptEvent, 0, len(messages))
	for _, msg := range messages {
		events = append(events, dto.ReceiptEvent{
			MessageID:     msg.MessageID.String(),
			SenderID:      userID,
			SenderShadeID: user.CoreGuardID,
			ReceiverID:    msg.SenderID.String(),
			Status:        status,
			Timestamp:     now,
		})
	}

	return events, nil
}

func (s *messageService) SavePendingReceipts(ctx context.Context, events []dto.ReceiptEvent) error {
	if len(events) == 0 {
		return nil
	}

	receipts := make([]models.PendingReceipt, 0, len(events))
	for _, event := range events {
		userID, err := uuid.Parse(event.ReceiverID)
		if err != nil {
			return err
		}
		fromUserID, err := uuid.Parse(event.SenderID)
		if err != nil {
			return err
		}
		messageID, err := uuid.Parse(event.MessageID)
		if err != nil {
			return err
		}
		receipts = append(receipts, models.PendingReceipt{
			UserID:     userID,
			FromUserID: fromUserID,
			MessageID:  messageID,
			Status:     strings.ToUpper(strings.TrimSpace(event.Status)),
			Timestamp:  time.UnixMilli(event.Timestamp),
		})
	}

	return s.msgRepo.SavePendingReceipts(receipts)
}

func (s *messageService) GetPendingReceipts(ctx context.Context, userID string) (*dto.PendingReceiptsResponse, error) {
	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}

	receipts, err := s.msgRepo.GetPendingReceipts(uid)
	if err != nil {
		return nil, err
	}

	items := make([]dto.PendingReceiptItem, 0, len(receipts))
	ids := make([]int, 0, len(receipts))

	for _, receipt := range receipts {
		ids = append(ids, receipt.ReceiptID)
		items = append(items, dto.PendingReceiptItem{
			MessageID: receipt.MessageID.String(),
			Status:    receipt.Status,
			Timestamp: receipt.Timestamp.UnixMilli(),
		})
	}

	if len(ids) > 0 {
		if err := s.msgRepo.DeletePendingReceipts(ids); err != nil {
			return nil, err
		}
	}

	return &dto.PendingReceiptsResponse{Receipts: items}, nil
}
