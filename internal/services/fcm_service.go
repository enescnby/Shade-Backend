package services

import (
	"context"
	"core-backend/pkg/logger"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"go.uber.org/zap"
)

type FCMService interface {
	SendWakeUpSignal(fcmToken string) error
}

type fcmService struct {
	app *firebase.App
}

func NewFCMService(app *firebase.App) FCMService {
	return &fcmService{app: app}
}

func (s *fcmService) SendWakeUpSignal(fcmToken string) error {
	// Firebase credentials yoksa development ortamında sessizce geç
	if s.app == nil {
		logger.Log.Debug("FCM skipped — Firebase not initialized (dev mode)")
		return nil
	}

	if fcmToken == "" {
		logger.Log.Debug("FCM skipped — empty FCM token")
		return nil
	}

	client, err := s.app.Messaging(context.Background())
	if err != nil {
		logger.Log.Error("FCM client could not be created", zap.Error(err))
		return err
	}

	message := &messaging.Message{
		Token: fcmToken,
		Data: map[string]string{
			"type":   "NEW_ENCRYPTED_MESSAGE",
			"action": "SYNC_REQUIRED",
		},
	}

	response, err := client.Send(context.Background(), message)
	if err != nil {
		logger.Log.Error("FCM notification could not be sent",
			zap.Error(err),
			zap.String("token", fcmToken),
		)
		return err
	}

	logger.Log.Info("FCM wake-up signal sent successfully", zap.String("response", response))
	return nil
}
