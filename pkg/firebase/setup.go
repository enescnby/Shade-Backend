package firebase

import (
	"context"
	"core-backend/pkg/logger"
	"os"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

func InitFirebase() *firebase.App {
	credPath := os.Getenv("FIREBASE_CREDENTIALS_PATH")
	if credPath == "" {
		credPath = "firebase-adminsdk.json"
	}

	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		logger.Log.Warn("Firebase credentials file not found — FCM push notifications disabled. Set FIREBASE_CREDENTIALS_PATH to enable.")
		return nil
	}

	opt := option.WithCredentialsFile(credPath)
	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		logger.Log.Warn("Firebase failed to initialize — FCM push notifications disabled.")
		return nil
	}

	logger.Log.Info("Firebase Admin SDK initialized successfully.")
	return app
}
