package repositories

import (
	"context"
	"core-backend/internal/models"
	"core-backend/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type MessageRepository interface {
	SaveMessage(message *models.EncryptedMessages) error
	GetUndeliveredMessages(ctx context.Context, receiverID uuid.UUID) ([]models.EncryptedMessages, error)
	MarkAsDelivered(messageIDs []uuid.UUID) error

	GetMessagesByIDsForReceiver(ctx context.Context, receiverID uuid.UUID, messageIDs []uuid.UUID) ([]models.EncryptedMessages, error)

	SavePendingReceipts(receipts []models.PendingReceipt) error
	GetPendingReceipts(userID uuid.UUID) ([]models.PendingReceipt, error)
	DeletePendingReceipts(receiptsID []int) error
}

type messageRepository struct {
	db *gorm.DB
}

func NewMessageRepository(db *gorm.DB) MessageRepository {
	return &messageRepository{db: db}
}

func (r *messageRepository) SaveMessage(message *models.EncryptedMessages) error {
	if err := r.db.Create(message).Error; err != nil {
		logger.Log.Error("failed to save encrypted message", zap.Error(err))
		return err
	}
	return nil
}

func (r *messageRepository) GetUndeliveredMessages(ctx context.Context, receiverID uuid.UUID) ([]models.EncryptedMessages, error) {
	var messages []models.EncryptedMessages

	err := r.db.WithContext(ctx).Preload("Status").
		Joins("JOIN delivery_statuses ON delivery_statuses.message_id = encrypted_messages.message_id").
		Where("encrypted_messages.receiver_id = ? AND delivery_statuses.is_delivered = ?", receiverID, false).
		Find(&messages).Error

	if err != nil {
		logger.Log.Error("failed to fetch undelivered messages", zap.Error(err))
		return nil, err
	}
	return messages, nil
}

func (r *messageRepository) MarkAsDelivered(messageIDs []uuid.UUID) error {
	err := r.db.Model(&models.DeliveryStatus{}).
		Where("message_id IN ?", messageIDs).
		Updates(map[string]interface{}{
			"is_delivered": true,
			"delivered_at": gorm.Expr("NOW()"),
		}).Error

	if err != nil {
		logger.Log.Error("failed to update message delivery status", zap.Error(err))
		return err
	}
	return nil
}

func (r *messageRepository) GetMessagesByIDsForReceiver(
	ctx context.Context,
	receiverID uuid.UUID,
	messageIDs []uuid.UUID) ([]models.EncryptedMessages, error) {
	var messages []models.EncryptedMessages
	if len(messageIDs) == 0 {
		return messages, nil
	}

	err := r.db.WithContext(ctx).
		Where("message_id IN ? AND receiver_id = ?", messageIDs, receiverID).
		Find(&messages).Error

	if err != nil {
		logger.Log.Error("failed to fetch messages by ids for receiver", zap.Error(err))
		return nil, err
	}
	return messages, nil
}

func (r *messageRepository) SavePendingReceipts(receipts []models.PendingReceipt) error {
	if len(receipts) == 0 {
		return nil
	}

	if err := r.db.Create(&receipts).Error; err != nil {
		logger.Log.Error("failed to save pending receipts", zap.Error(err))
		return err
	}
	return nil
}

func (r *messageRepository) GetPendingReceipts(userID uuid.UUID) ([]models.PendingReceipt, error) {
	var receipts []models.PendingReceipt
	err := r.db.
		Where("user_id = ?", userID).
		Order("timestamp ASC").
		Find(&receipts).Error

	if err != nil {
		logger.Log.Error("failed to fetch pending receipts", zap.Error(err))
		return nil, err
	}
	return receipts, nil
}

func (r *messageRepository) DeletePendingReceipts(receiptIDs []int) error {
	if len(receiptIDs) == 0 {
		return nil
	}
	if err := r.db.Where("receipt_id IN ?", receiptIDs).Delete(&models.PendingReceipt{}).Error; err != nil {
		logger.Log.Error("failed to delete pending receipts", zap.Error(err))
		return err
	}
	return nil
}
