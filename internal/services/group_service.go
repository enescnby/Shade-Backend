package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupService interface {
	// Groups
	CreateGroup(ctx context.Context, ownerID uuid.UUID, ownerShadeID string, req dto.CreateGroupRequest) (*dto.GroupResponse, error)
	GetGroup(ctx context.Context, groupID uuid.UUID, callerID uuid.UUID) (*dto.GroupResponse, error)
	ListMyGroups(ctx context.Context, userID uuid.UUID) ([]dto.GroupResponse, error)
	DeleteGroup(ctx context.Context, groupID, callerID uuid.UUID) error

	// Members
	AddMember(ctx context.Context, groupID, callerID uuid.UUID, req dto.AddMemberRequest) error
	RemoveMember(ctx context.Context, groupID, callerID, targetID uuid.UUID) error

	// Invites
	CreateInvite(ctx context.Context, callerID uuid.UUID, req dto.CreateInviteRequest) (*dto.InviteResponse, error)
	RedeemInvite(ctx context.Context, code string, callerID uuid.UUID, callerShadeID string) (*dto.RedeemInviteResponse, error)
}

type groupService struct {
	groupRepo repositories.GroupRepository
	userRepo  repositories.UserRepository
}

func NewGroupService(groupRepo repositories.GroupRepository, userRepo repositories.UserRepository) GroupService {
	return &groupService{groupRepo: groupRepo, userRepo: userRepo}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func groupToDTO(g *models.Group) *dto.GroupResponse {
	members := make([]dto.GroupMemberResponse, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, dto.GroupMemberResponse{
			UserID:  m.UserID.String(),
			ShadeID: m.ShadeID,
			Role:    m.Role,
		})
	}
	return &dto.GroupResponse{
		GroupID:   g.GroupID.String(),
		Name:      g.Name,
		OwnerID:   g.OwnerID.String(),
		AvatarURL: g.AvatarURL,
		Members:   members,
		CreatedAt: g.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ── Groups ────────────────────────────────────────────────────────────────────

func (s *groupService) CreateGroup(ctx context.Context, ownerID uuid.UUID, ownerShadeID string, req dto.CreateGroupRequest) (*dto.GroupResponse, error) {
	if req.Name == "" {
		return nil, errors.New("group name is required")
	}

	g := &models.Group{
		Name:    req.Name,
		OwnerID: ownerID,
	}
	if err := s.groupRepo.CreateGroup(ctx, g); err != nil {
		return nil, err
	}

	// Add owner as first member
	if err := s.groupRepo.AddMember(ctx, &models.GroupMember{
		GroupID: g.GroupID,
		UserID:  ownerID,
		ShadeID: ownerShadeID,
		Role:    "owner",
	}); err != nil {
		return nil, err
	}

	// Add additional members provided at creation time
	for _, idStr := range req.MemberIDs {
		uid, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		user, err := s.userRepo.GetUserByID(uid)
		if err != nil {
			continue // skip unknown users
		}
		_ = s.groupRepo.AddMember(ctx, &models.GroupMember{
			GroupID: g.GroupID,
			UserID:  uid,
			ShadeID: user.CoreGuardID,
			Role:    "member",
		})
	}

	// Reload to pick up all members
	full, err := s.groupRepo.GetGroupByID(ctx, g.GroupID)
	if err != nil {
		return nil, err
	}
	return groupToDTO(full), nil
}

func (s *groupService) GetGroup(ctx context.Context, groupID uuid.UUID, callerID uuid.UUID) (*dto.GroupResponse, error) {
	g, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return nil, err
	}
	// Only members can fetch group details
	if _, err := s.groupRepo.GetMember(ctx, groupID, callerID); err != nil {
		return nil, errors.New("not a member")
	}
	return groupToDTO(g), nil
}

func (s *groupService) ListMyGroups(ctx context.Context, userID uuid.UUID) ([]dto.GroupResponse, error) {
	groups, err := s.groupRepo.ListGroupsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.GroupResponse, 0, len(groups))
	for i := range groups {
		out = append(out, *groupToDTO(&groups[i]))
	}
	return out, nil
}

func (s *groupService) DeleteGroup(ctx context.Context, groupID, callerID uuid.UUID) error {
	g, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	if g.OwnerID != callerID {
		return errors.New("only the owner can delete the group")
	}
	return s.groupRepo.DeleteGroup(ctx, groupID)
}

// ── Members ───────────────────────────────────────────────────────────────────

func (s *groupService) AddMember(ctx context.Context, groupID, callerID uuid.UUID, req dto.AddMemberRequest) error {
	// Only existing members (or owner) can add new members
	if _, err := s.groupRepo.GetMember(ctx, groupID, callerID); err != nil {
		return errors.New("not a member of the group")
	}
	uid, err := uuid.Parse(req.UserID)
	if err != nil {
		return errors.New("invalid user_id")
	}
	user, err := s.userRepo.GetUserByID(uid)
	if err != nil {
		return errors.New("user not found")
	}
	return s.groupRepo.AddMember(ctx, &models.GroupMember{
		GroupID: groupID,
		UserID:  uid,
		ShadeID: user.CoreGuardID,
		Role:    "member",
	})
}

func (s *groupService) RemoveMember(ctx context.Context, groupID, callerID, targetID uuid.UUID) error {
	g, err := s.groupRepo.GetGroupByID(ctx, groupID)
	if err != nil {
		return err
	}
	// Owner can remove anyone; members can only remove themselves
	if g.OwnerID != callerID && callerID != targetID {
		return errors.New("not authorised to remove this member")
	}
	return s.groupRepo.RemoveMember(ctx, groupID, targetID)
}

// ── Invites ───────────────────────────────────────────────────────────────────

func (s *groupService) CreateInvite(ctx context.Context, callerID uuid.UUID, req dto.CreateInviteRequest) (*dto.InviteResponse, error) {
	maxUses := req.MaxUses
	if maxUses <= 0 {
		maxUses = 1
	}

	inv := &models.ContactInvite{
		OwnerID: callerID,
		MaxUses: maxUses,
	}

	if req.GroupID != "" {
		gid, err := uuid.Parse(req.GroupID)
		if err != nil {
			return nil, errors.New("invalid group_id")
		}
		// Make sure caller is a member
		if _, err := s.groupRepo.GetMember(ctx, gid, callerID); err != nil {
			return nil, errors.New("not a member of the group")
		}
		inv.GroupID = &gid
	}

	if err := s.groupRepo.CreateInvite(ctx, inv); err != nil {
		return nil, err
	}

	resp := &dto.InviteResponse{
		Code:     inv.Code,
		MaxUses:  inv.MaxUses,
		UseCount: inv.UseCount,
	}
	if inv.GroupID != nil {
		gidStr := inv.GroupID.String()
		resp.GroupID = &gidStr
	}
	if inv.ExpiresAt != nil {
		exp := inv.ExpiresAt.UTC().Format(time.RFC3339)
		resp.ExpiresAt = &exp
	}
	return resp, nil
}

func (s *groupService) RedeemInvite(ctx context.Context, code string, callerID uuid.UUID, callerShadeID string) (*dto.RedeemInviteResponse, error) {
	inv, err := s.groupRepo.GetInviteByCode(ctx, code)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invite not found")
		}
		return nil, err
	}

	// Expiry check
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		return nil, errors.New("invite has expired")
	}
	// Usage limit check
	if inv.UseCount >= inv.MaxUses {
		return nil, errors.New("invite has reached its usage limit")
	}

	if err := s.groupRepo.IncrementInviteUse(ctx, code); err != nil {
		return nil, err
	}

	if inv.GroupID != nil {
		// Group invite — add caller to the group
		_ = s.groupRepo.AddMember(ctx, &models.GroupMember{
			GroupID: *inv.GroupID,
			UserID:  callerID,
			ShadeID: callerShadeID,
			Role:    "member",
		})
		g, err := s.groupRepo.GetGroupByID(ctx, *inv.GroupID)
		if err != nil {
			return nil, err
		}
		gResp := groupToDTO(g)
		return &dto.RedeemInviteResponse{Type: "group", Group: gResp}, nil
	}

	// Personal contact invite — return owner's contact info
	ownerUser, err := s.userRepo.GetUserByID(inv.OwnerID)
	if err != nil {
		return nil, err
	}
	// Reload with Key preloaded via GetUserByCoreGuardID
	ownerFull, err := s.userRepo.GetUserForLookup(ctx, ownerUser.CoreGuardID)
	if err != nil {
		return nil, err
	}
	contactResp := &dto.LookupResponse{
		UserID:              ownerFull.UserID.String(),
		ShadeID:             ownerFull.CoreGuardID,
		EncryptionPublicKey: ownerFull.Key.EncryptionPublicKey,
	}
	return &dto.RedeemInviteResponse{Type: "contact", Contact: contactResp}, nil
}
