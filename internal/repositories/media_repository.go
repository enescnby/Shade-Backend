package repositories

import (
	"context"
	"core-backend/internal/models"
	"core-backend/pkg/logger"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type MediaRepository interface {
	SaveMetadata(ctx context.Context, metadata *models.ImageMetaData) error
	GetMetadata(ctx context.Context, imageID uuid.UUID) (*models.ImageMetaData, error)
	UploadToStorage(ctx context.Context, key string, body io.Reader, size int64) error
	DownloadFromStorage(ctx context.Context, key string) (io.ReadCloser, error)
}

type mediaRepository struct {
	db     *gorm.DB
	s3     *s3.Client
	bucket string
}

func NewMediaRepository(db *gorm.DB, s3Client *s3.Client, bucket string) MediaRepository {
	return &mediaRepository{db: db, s3: s3Client, bucket: bucket}
}

func (r *mediaRepository) SaveMetadata(ctx context.Context, metadata *models.ImageMetaData) error {
	if err := r.db.WithContext(ctx).Create(metadata).Error; err != nil {
		logger.Log.Error("failed to save image metadata", zap.Error(err))
		return err
	}
	return nil
}

func (r *mediaRepository) GetMetadata(ctx context.Context, imageID uuid.UUID) (*models.ImageMetaData, error) {
	var metadata models.ImageMetaData
	if err := r.db.WithContext(ctx).Where("image_id = ?", imageID).First(&metadata).Error; err != nil {
		return nil, err
	}
	return &metadata, nil
}

func (r *mediaRepository) UploadToStorage(ctx context.Context, key string, body io.Reader, size int64) error {
	_, err := r.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(r.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String("application/octet-stream"),
	})
	if err != nil {
		logger.Log.Error("failed to upload R2", zap.String("key", key), zap.Error(err))
		return err
	}
	return nil
}

func (r *mediaRepository) DownloadFromStorage(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := r.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		logger.Log.Error("failed to download from R2", zap.String("key", key), zap.Error(err))
		return nil, err
	}
	return output.Body, nil
}
