package handlers

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/services"
	wsmanager "core-backend/internal/websocket"
	"core-backend/pb"
	"core-backend/pkg/logger"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
)

type MessageHandler struct {
	messageService    services.MessageService
	connectionManager wsmanager.ConnectionManager
}

func NewMessageHandler(s services.MessageService, cm wsmanager.ConnectionManager) *MessageHandler {
	return &MessageHandler{messageService: s, connectionManager: cm}
}

func (h *MessageHandler) GetUndelivered(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)

	res, err := h.messageService.GetUndeliveredMessages(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to fetch messages",
		})
	}

	events, err := h.buildDeliveredReceiptEvents(c.Context(), userID, res.Messages)
	if err == nil {
		if relayErr := h.relayReceiptEvents(c.Context(), events); relayErr != nil {
			logger.Log.Warn("failed to relay delivered receipts", zap.Error(relayErr))
		}
	} else {
		logger.Log.Warn("failed to build delivered receipt events", zap.Error(err))
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *MessageHandler) PostReceipts(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)

	var req dto.BatchReceiptRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON format",
		})
	}

	if len(req.Receipts) == 0 {
		return c.SendStatus(fiber.StatusOK)
	}

	messageIDs := make([]string, 0, len(req.Receipts))
	for _, receipt := range req.Receipts {
		status := strings.ToUpper(strings.TrimSpace(receipt.Status))
		if status != "READ" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "only READ status is supported",
			})
		}
		if strings.TrimSpace(receipt.MessageID) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "message_id is required",
			})
		}
		messageIDs = append(messageIDs, receipt.MessageID)
	}

	events, err := h.messageService.BuildReceiptEvents(c.Context(), userID, messageIDs, "READ")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to build receipt events",
		})
	}

	if err := h.relayReceiptEvents(c.Context(), events); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to relay receipts",
		})
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *MessageHandler) GetPendingReceipts(c *fiber.Ctx) error {
	userID, _ := c.Locals("user_id").(string)

	res, err := h.messageService.GetPendingReceipts(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to fetch pending receipts",
		})
	}

	return c.Status(fiber.StatusOK).JSON(res)
}

func (h *MessageHandler) buildDeliveredReceiptEvents(ctx context.Context, userID string, messages []dto.EncryptedMessageItem) ([]dto.ReceiptEvent, error) {
	shadeID, err := h.messageService.GetUserShadeID(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UnixMilli()
	events := make([]dto.ReceiptEvent, 0, len(messages))
	for _, msg := range messages {
		events = append(events, dto.ReceiptEvent{
			MessageID:     msg.MessageID,
			SenderID:      userID, // receipt sender (current user)
			SenderShadeID: shadeID,
			ReceiverID:    msg.SenderID, // receipt receiver (original sender)
			Status:        "DELIVERED",
			Timestamp:     now,
		})
	}

	return events, nil
}

func (h *MessageHandler) relayReceiptEvents(ctx context.Context, events []dto.ReceiptEvent) error {
	if len(events) == 0 {
		return nil
	}

	pending := make([]dto.ReceiptEvent, 0)

	for _, event := range events {
		rawPayload, err := buildReceiptPayload(event)
		if err != nil {
			pending = append(pending, event)
			continue
		}

		if err := h.connectionManager.SendToUser(event.ReceiverID, rawPayload); err != nil {
			pending = append(pending, event)
		}
	}

	if len(pending) > 0 {
		return h.messageService.SavePendingReceipts(ctx, pending)
	}

	return nil
}

func buildReceiptPayload(event dto.ReceiptEvent) ([]byte, error) {
	status, ok := mapReceiptStatus(event.Status)
	if !ok {
		return nil, fmt.Errorf("invalid receipt status: %s", event.Status)
	}

	receipt := &pb.DeliveryReceipt{
		MessageId:     event.MessageID,
		SenderId:      event.SenderID,
		SenderShadeId: event.SenderShadeID,
		ReceiverId:    event.ReceiverID,
		Status:        status,
		Timestamp:     event.Timestamp,
	}

	wrapper := &pb.WebSocketMessage{
		Content: &pb.WebSocketMessage_Receipt{
			Receipt: receipt,
		},
	}

	return proto.Marshal(wrapper)
}

func mapReceiptStatus(status string) (pb.ReceiptStatus, bool) {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "DELIVERED":
		return pb.ReceiptStatus_DELIVERED, true
	case "READ":
		return pb.ReceiptStatus_READ, true
	default:
		return pb.ReceiptStatus_DELIVERED, false
	}
}
