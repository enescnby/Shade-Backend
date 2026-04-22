package storage

import (
	"core-backend/internal/config"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func NewR2Client(cfg *config.Config) *s3.Client {
	r2Endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.R2AccountID)

	client := s3.New(s3.Options{
		Region:       "auto",
		BaseEndpoint: aws.String(r2Endpoint),
		Credentials:  aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider(cfg.R2AccessKeyID, cfg.R2AccessSecret, "")),
	})

	return client
}
