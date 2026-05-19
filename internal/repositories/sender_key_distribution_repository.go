package repositories

import (
	"context"
	"core-backend/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SenderKeyDistributionRepository, alıcı bazında saklanan SKDM kayıtlarını
// yönetir. Sunucu içeriğe (encrypted_skdm) körüdür; sadece envelope
// alanlarını rota bilgisi olarak kullanır.
type SenderKeyDistributionRepository interface {
	// Upsert, aynı (sender_user, sender_device, recipient_user, group) çiftine
	// gelen yeni SKDM'i kaydeder veya günceller.
	Upsert(ctx context.Context, row *models.GroupSenderKeyDistribution) error

	// ListForRecipient, bir alıcıya yönelik tüm kayıtlı SKDM'leri döner —
	// inbox drain'de tekrar yayınlamak için.
	ListForRecipient(ctx context.Context, recipientUserID uuid.UUID) ([]models.GroupSenderKeyDistribution, error)

	// DeleteForRecipientGroup, recipient bir gruptan ayrıldığında ilgili
	// SKDM kayıtlarını siler.
	DeleteForRecipientGroup(ctx context.Context, recipientUserID, groupID uuid.UUID) error
}

type senderKeyDistributionRepository struct {
	db *gorm.DB
}

func NewSenderKeyDistributionRepository(db *gorm.DB) SenderKeyDistributionRepository {
	return &senderKeyDistributionRepository{db: db}
}

func (r *senderKeyDistributionRepository) Upsert(ctx context.Context, row *models.GroupSenderKeyDistribution) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "sender_user_id"},
				{Name: "sender_device_id"},
				{Name: "recipient_user_id"},
				{Name: "group_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"encrypted_skdm", "nonce", "updated_at",
			}),
		}).
		Create(row).Error
}

func (r *senderKeyDistributionRepository) ListForRecipient(ctx context.Context, recipientUserID uuid.UUID) ([]models.GroupSenderKeyDistribution, error) {
	var rows []models.GroupSenderKeyDistribution
	err := r.db.WithContext(ctx).
		Where("recipient_user_id = ?", recipientUserID).
		Find(&rows).Error
	return rows, err
}

func (r *senderKeyDistributionRepository) DeleteForRecipientGroup(ctx context.Context, recipientUserID, groupID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("recipient_user_id = ? AND group_id = ?", recipientUserID, groupID).
		Delete(&models.GroupSenderKeyDistribution{}).Error
}
