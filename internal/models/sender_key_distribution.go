package models

import (
	"time"

	"github.com/google/uuid"
)

// GroupSenderKeyDistribution, bir göndericinin (user, device) belirli bir
// alıcıya yolladığı **son** SKDM'i kalıcı tutar.
//
// RabbitMQ kuyruğu yalnızca alıcı cihaz queue'su BAĞLI iken yayınlanan
// mesajları biriktirir; web cihazı QR-auth'tan önce yapılan SKDM'leri
// kaybeder. Bu tablo o boşluğu doldurur: inbox drain'de `recipient_user_id`
// için kaydedilmiş tüm satırlar tekrar yayınlanır, içerik pairwise-şifreli
// olduğundan sunucu plaintext'i okuyamaz.
type GroupSenderKeyDistribution struct {
	SenderUserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	SenderDeviceID  uuid.UUID `gorm:"type:uuid;primaryKey"`
	RecipientUserID uuid.UUID `gorm:"type:uuid;primaryKey;index"`
	GroupID         uuid.UUID `gorm:"type:uuid;primaryKey;index"`

	EncryptedSKDM []byte `gorm:"not null"`
	Nonce         []byte `gorm:"not null"`

	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

func (GroupSenderKeyDistribution) TableName() string {
	return "group_sender_key_distributions"
}
