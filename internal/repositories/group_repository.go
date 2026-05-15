package repositories

import (
	"context"
	"core-backend/internal/models"
	"errors"
	"math/rand"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupRepository interface {
	// Groups
	CreateGroup(ctx context.Context, g *models.Group) error
	GetGroupByID(ctx context.Context, groupID uuid.UUID) (*models.Group, error)
	ListGroupsForUser(ctx context.Context, userID uuid.UUID) ([]models.Group, error)
	DeleteGroup(ctx context.Context, groupID uuid.UUID) error

	// Members
	AddMember(ctx context.Context, m *models.GroupMember) error
	RemoveMember(ctx context.Context, groupID, userID uuid.UUID) error
	GetMember(ctx context.Context, groupID, userID uuid.UUID) (*models.GroupMember, error)

	// Invites
	CreateInvite(ctx context.Context, inv *models.ContactInvite) error
	GetInviteByCode(ctx context.Context, code string) (*models.ContactInvite, error)
	IncrementInviteUse(ctx context.Context, code string) error
}

type groupRepository struct {
	db *gorm.DB
}

func NewGroupRepository(db *gorm.DB) GroupRepository {
	return &groupRepository{db: db}
}

// ── Groups ────────────────────────────────────────────────────────────────────

func (r *groupRepository) CreateGroup(ctx context.Context, g *models.Group) error {
	return r.db.WithContext(ctx).Create(g).Error
}

func (r *groupRepository) GetGroupByID(ctx context.Context, groupID uuid.UUID) (*models.Group, error) {
	var g models.Group
	err := r.db.WithContext(ctx).
		Preload("Members").
		Where("group_id = ?", groupID).
		First(&g).Error
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *groupRepository) ListGroupsForUser(ctx context.Context, userID uuid.UUID) ([]models.Group, error) {
	var groups []models.Group
	err := r.db.WithContext(ctx).
		Preload("Members").
		Joins("JOIN group_members gm ON gm.group_id = groups.group_id").
		Where("gm.user_id = ?", userID).
		Find(&groups).Error
	return groups, err
}

func (r *groupRepository) DeleteGroup(ctx context.Context, groupID uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("group_id = ?", groupID).Delete(&models.GroupMember{}).Error; err != nil {
			return err
		}
		return tx.Where("group_id = ?", groupID).Delete(&models.Group{}).Error
	})
}

// ── Members ───────────────────────────────────────────────────────────────────

func (r *groupRepository) AddMember(ctx context.Context, m *models.GroupMember) error {
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *groupRepository) RemoveMember(ctx context.Context, groupID, userID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&models.GroupMember{}).Error
}

func (r *groupRepository) GetMember(ctx context.Context, groupID, userID uuid.UUID) (*models.GroupMember, error) {
	var m models.GroupMember
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		First(&m).Error
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ── Invites ───────────────────────────────────────────────────────────────────

const inviteChars = "abcdefghijkmnpqrstuvwxyz23456789"

func generateCode(n int) string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := strings.Builder{}
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteByte(inviteChars[rng.Intn(len(inviteChars))])
	}
	return b.String()
}

func (r *groupRepository) CreateInvite(ctx context.Context, inv *models.ContactInvite) error {
	if inv.Code == "" {
		inv.Code = generateCode(12)
	}
	return r.db.WithContext(ctx).Create(inv).Error
}

func (r *groupRepository) GetInviteByCode(ctx context.Context, code string) (*models.ContactInvite, error) {
	var inv models.ContactInvite
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&inv).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}
	return &inv, nil
}

func (r *groupRepository) IncrementInviteUse(ctx context.Context, code string) error {
	return r.db.WithContext(ctx).
		Model(&models.ContactInvite{}).
		Where("code = ?", code).
		UpdateColumn("use_count", gorm.Expr("use_count + 1")).
		Error
}
