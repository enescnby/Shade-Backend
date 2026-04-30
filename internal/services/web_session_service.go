package services

import (
	"core-backend/internal/dto"
	"core-backend/internal/models"
	"core-backend/internal/repositories"
	"core-backend/pkg/logger"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	SessionTTL  = 120 * time.Second
	LongPollMax = 25 * time.Second
)

var (
	ErrSessionNotFound   = errors.New("session not found")
	ErrSessionExpired    = errors.New("session expired")
	ErrAlreadyAuthorized = errors.New("session already authorized")
	ErrMissingFields     = errors.New("missing required fields")
)

type WebSessionService interface {
	CreateSession() (*dto.CreateSessionResponse, error)
	PollSession(sessionID string) (*dto.SessionPollResponse, error)
	AuthorizeSession(sessionID string, req *dto.AuthorizeRequest) error
	GetSession(sessionID string) (*models.WebSession, error)
}

type webSessionService struct {
	repo     repositories.WebSessionRepository
	mu       sync.Mutex
	notifyCh map[string]chan struct{}
}

func NewSessionService(repo repositories.WebSessionRepository) WebSessionService {
	return &webSessionService{
		repo:     repo,
		notifyCh: make(map[string]chan struct{}),
	}
}

func (s *webSessionService) CreateSession() (*dto.CreateSessionResponse, error) {
	id := uuid.New()
	session := &models.WebSession{
		SessionID: id,
		Status:    "pending",
		ExpiresAt: time.Now().Add(SessionTTL),
	}

	if err := s.repo.Create(session); err != nil {
		return nil, err
	}

	ch := make(chan struct{})
	s.mu.Lock()
	s.notifyCh[id.String()] = ch
	s.mu.Unlock()

	go func() {
		time.Sleep(SessionTTL + time.Second)
		s.mu.Lock()
		delete(s.notifyCh, id.String())
		s.mu.Unlock()
	}()

	logger.Log.Info("web session created", zap.String("session_id", id.String()))

	return &dto.CreateSessionResponse{
		SessionID: id.String(),
		ExpiresAt: session.ExpiresAt,
	}, nil
}

func (s *webSessionService) PollSession(sessionID string) (*dto.SessionPollResponse, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	session, err := s.repo.GetByID(id)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	if session.Status == "authorized" {
		return buildPollResponse(session), nil
	}

	s.mu.Lock()
	ch, ok := s.notifyCh[sessionID]
	if !ok {
		ch = make(chan struct{})
		s.notifyCh[sessionID] = ch
	}
	s.mu.Unlock()

	timer := time.NewTimer(LongPollMax)
	defer timer.Stop()

	select {
	case <-ch:
		session, err = s.repo.GetByID(id)
		if err != nil || session == nil {
			return nil, ErrSessionNotFound
		}
		return buildPollResponse(session), nil

	case <-timer.C:
		return nil, nil
	}
}

func (s *webSessionService) AuthorizeSession(sessionID string, req *dto.AuthorizeRequest) error {
	if req.Ciphertext == "" || req.Nonce == "" || req.AndroidX25519Pub == "" {
		return ErrMissingFields
	}

	id, err := uuid.Parse(sessionID)
	if err != nil {
		return ErrSessionNotFound
	}

	session, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if session == nil {
		return ErrSessionNotFound
	}
	if time.Now().After(session.ExpiresAt) {
		return ErrSessionExpired
	}
	if session.Status == "authorized" {
		return ErrAlreadyAuthorized
	}

	if err := s.repo.Authorize(id, req.Ciphertext, req.Nonce, req.AndroidX25519Pub); err != nil {
		return err
	}

	s.mu.Lock()
	if ch, ok := s.notifyCh[sessionID]; ok {
		close(ch)
		delete(s.notifyCh, sessionID)
	}
	s.mu.Unlock()

	logger.Log.Info("web session authorized by android", zap.String("session_id", sessionID))
	return nil
}

func (s *webSessionService) GetSession(sessionID string) (*models.WebSession, error) {
	id, err := uuid.Parse(sessionID)
	if err != nil {
		return nil, ErrSessionNotFound
	}
	session, err := s.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrSessionNotFound
	}
	return session, nil
}

func buildPollResponse(s *models.WebSession) *dto.SessionPollResponse {
	return &dto.SessionPollResponse{
		Ciphertext:       s.Ciphertext,
		Nonce:            s.Nonce,
		AndroidX25519Pub: s.AndroidX25519Pub,
	}
}
