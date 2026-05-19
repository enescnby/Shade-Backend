package repositories

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"core-backend/internal/models"
	"core-backend/pkg/logger"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type localMediaRepository struct {
	db        *gorm.DB
	uploadDir string
}

// NewLocalMediaRepository creates a MediaRepository that stores files on local disk.
// Used when R2/S3 credentials are not configured (dev environment).
func NewLocalMediaRepository(db *gorm.DB, uploadDir string) MediaRepository {
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		logger.Log.Warn("failed to create upload directory", zap.String("dir", uploadDir), zap.Error(err))
	}
	logger.Log.Info("using local file storage for media", zap.String("dir", uploadDir))
	return &localMediaRepository{db: db, uploadDir: uploadDir}
}

func (r *localMediaRepository) SaveMetadata(ctx context.Context, metadata *models.ImageMetaData) error {
	if err := r.db.WithContext(ctx).Create(metadata).Error; err != nil {
		logger.Log.Error("failed to save image metadata", zap.Error(err))
		return err
	}
	return nil
}

func (r *localMediaRepository) GetMetadata(ctx context.Context, imageID uuid.UUID) (*models.ImageMetaData, error) {
	var metadata models.ImageMetaData
	if err := r.db.WithContext(ctx).Where("image_id = ?", imageID).First(&metadata).Error; err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (r *localMediaRepository) UploadToStorage(ctx context.Context, key string, body io.Reader, size int64) error {
	path := filepath.Join(r.uploadDir, key)
	f, err := os.Create(path)
	if err != nil {
		logger.Log.Error("failed to create local file", zap.String("path", path), zap.Error(err))
		return err
	}
	defer f.Close()

	written, err := io.Copy(f, body)
	if err != nil {
		logger.Log.Error("failed to write local file", zap.String("path", path), zap.Error(err))
		return err
	}
	logger.Log.Info("file saved locally", zap.String("key", key), zap.Int64("bytes", written))
	return nil
}

func (r *localMediaRepository) DownloadFromStorage(ctx context.Context, key string) (io.ReadCloser, error) {
	path := filepath.Join(r.uploadDir, key)
	f, err := os.Open(path)
	if err != nil {
		logger.Log.Error("failed to open local file", zap.String("path", path), zap.Error(err))
		return nil, fmt.Errorf("file not found: %s", key)
	}
	return f, nil
}
