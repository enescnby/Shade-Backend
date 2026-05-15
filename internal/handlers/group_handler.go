package handlers

import (
	"core-backend/internal/dto"
	"core-backend/internal/services"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GroupHandler struct {
	groupService services.GroupService
}

func NewGroupHandler(service services.GroupService) *GroupHandler {
	return &GroupHandler{groupService: service}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func callerFromCtx(c *fiber.Ctx) (uuid.UUID, string, bool) {
	userIDStr, ok := c.Locals("user_id").(string)
	if !ok {
		return uuid.Nil, "", false
	}
	shadeID, _ := c.Locals("core_guard_id").(string)
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, "", false
	}
	return uid, shadeID, true
}

// ── Groups ────────────────────────────────────────────────────────────────────

// POST /groups
func (h *GroupHandler) CreateGroup(c *fiber.Ctx) error {
	callerID, shadeID, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	var req dto.CreateGroupRequest
	if err := c.BodyParser(&req); err != nil || req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}

	resp, err := h.groupService.CreateGroup(c.UserContext(), callerID, shadeID, req)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GET /groups
func (h *GroupHandler) ListGroups(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	groups, err := h.groupService.ListMyGroups(c.UserContext(), callerID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(groups)
}

// GET /groups/:id
func (h *GroupHandler) GetGroup(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid group id"})
	}
	resp, err := h.groupService.GetGroup(c.UserContext(), groupID, callerID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "group not found"})
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}

// DELETE /groups/:id
func (h *GroupHandler) DeleteGroup(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid group id"})
	}
	if err := h.groupService.DeleteGroup(c.UserContext(), groupID, callerID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "group not found"})
		}
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Members ───────────────────────────────────────────────────────────────────

// POST /groups/:id/members
func (h *GroupHandler) AddMember(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid group id"})
	}
	var req dto.AddMemberRequest
	if err := c.BodyParser(&req); err != nil || req.UserID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "user_id is required"})
	}
	if err := h.groupService.AddMember(c.UserContext(), groupID, callerID, req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// DELETE /groups/:id/members/:userId
func (h *GroupHandler) RemoveMember(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	groupID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid group id"})
	}
	targetID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid user id"})
	}
	if err := h.groupService.RemoveMember(c.UserContext(), groupID, callerID, targetID); err != nil {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ── Invites ───────────────────────────────────────────────────────────────────

// POST /invites
func (h *GroupHandler) CreateInvite(c *fiber.Ctx) error {
	callerID, _, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	var req dto.CreateInviteRequest
	_ = c.BodyParser(&req) // body is optional

	resp, err := h.groupService.CreateInvite(c.UserContext(), callerID, req)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusCreated).JSON(resp)
}

// GET /invites/:code
func (h *GroupHandler) RedeemInvite(c *fiber.Ctx) error {
	callerID, shadeID, ok := callerFromCtx(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	code := c.Params("code")
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invite code required"})
	}
	resp, err := h.groupService.RedeemInvite(c.UserContext(), code, callerID, shadeID)
	if err != nil {
		if err.Error() == "invite not found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.Status(fiber.StatusOK).JSON(resp)
}
