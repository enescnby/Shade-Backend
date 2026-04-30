package services

import (
	"context"
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"
	"errors"
	"io"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type MediaService interface {
	Upload(ctx context.Context, uploaderID uuid.UUID, fileData io.Reader, fileSize int64) (*dto.UploadImageResponse, error)
	Download(ctx context.Context, imageID uuid.UUID) (io.ReadCloser, error)
}

type mediaService struct {
	repo        repositories.MediaRepository
	maxFileSize int64
}

func NewMediaService(repo repositories.MediaRepository, maxFileSize int64) MediaService {
	return &mediaService{repo: repo, maxFileSize: maxFileSize}
}

func (s *mediaService) Upload(ctx context.Context, uploaderID uuid.UUID, fileData io.Reader, fileSize int64) (*dto.UploadImageResponse, error) {
	if fileSize > s.maxFileSize {
		return nil, errors.New("file size exceeds maximum allowed size")
	}

	imageID := uuid.New()

	if err := s.repo.UploadToStorage(ctx, imageID.String(), fileData, fileSize); err != nil {
		logger.Log.Error("failed to upload image to storage", zap.Error(err))
		return nil, err
	}

	metadata := &models.ImageMetaData{
		ImageID:    imageID,
		UploaderID: uploaderID,
		Size:       fileSize,
	}

	if err := s.repo.SaveMetadata(ctx, metadata); err != nil {
		logger.Log.Error("failed to save image metadata", zap.Error(err))
		return nil, err
	}

	logger.Log.Info("image uploaded successfully",
		zap.String("image_id", imageID.String()),
		zap.String("uploader_id", uploaderID.String()),
	)

	return &dto.UploadImageResponse{
		ImageID: imageID.String(),
	}, nil
}

func (s *mediaService) Download(ctx context.Context, imageID uuid.UUID) (io.ReadCloser, error) {
	_, err := s.repo.GetMetadata(ctx, imageID)
	if err != nil {
		return nil, err
	}

	body, err := s.repo.DownloadFromStorage(ctx, imageID.String())
	if err != nil {
		return nil, err
	}

	return body, nil
}
