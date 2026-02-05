package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/enterprise/risk-engine/internal/auth"
	"github.com/enterprise/risk-engine/internal/models"
	"github.com/enterprise/risk-engine/internal/repositories"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrWeakPassword       = errors.New("password does not meet requirements")
)

// AuthService handles authentication operations
type AuthService struct {
	userRepo   *repositories.UserRepository
	jwtManager *auth.JWTManager
}

// NewAuthService creates a new auth service
func NewAuthService(userRepo *repositories.UserRepository, jwtManager *auth.JWTManager) *AuthService {
	return &AuthService{
		userRepo:   userRepo,
		jwtManager: jwtManager,
	}
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
	Role     string `json:"role"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	Token     string       `json:"token"`
	ExpiresIn int64        `json:"expires_in"`
	User      UserResponse `json:"user"`
}

// UserResponse represents a user in responses
type UserResponse struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt string    `json:"created_at"`
}

// Register registers a new user
func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, error) {
	// Validate password strength
	if !auth.ValidatePasswordStrength(req.Password) {
		return nil, ErrWeakPassword
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	// Set default role
	role := req.Role
	if role == "" {
		role = "user"
	}

	// Create user
	user := &models.User{
		Email:        req.Email,
		PasswordHash: hashedPassword,
		Role:         role,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		if errors.Is(err, repositories.ErrUserAlreadyExists) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Generate token
	token, err := s.jwtManager.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		Token:     token,
		ExpiresIn: 86400, // 24 hours in seconds
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

// Login authenticates a user
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Find user by email
	user, err := s.userRepo.GetByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, repositories.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("failed to find user: %w", err)
	}

	// Check password
	if !auth.CheckPassword(req.Password, user.PasswordHash) {
		return nil, ErrInvalidCredentials
	}

	// Generate token
	token, err := s.jwtManager.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		Token:     token,
		ExpiresIn: 86400,
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

// RefreshToken refreshes an authentication token
func (s *AuthService) RefreshToken(ctx context.Context, currentToken string) (*AuthResponse, error) {
	// Validate current token and get claims
	claims, err := s.jwtManager.ValidateToken(currentToken)
	if err != nil {
		return nil, err
	}

	// Get user to ensure they still exist
	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Generate new token
	newToken, err := s.jwtManager.GenerateToken(user.ID, user.Email, user.Role)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	return &AuthResponse{
		Token:     newToken,
		ExpiresIn: 86400,
		User: UserResponse{
			ID:        user.ID,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
		},
	}, nil
}

// GetUser retrieves a user by ID
func (s *AuthService) GetUser(ctx context.Context, userID uuid.UUID) (*UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &UserResponse{
		ID:        user.ID,
		Email:     user.Email,
		Role:      user.Role,
		CreatedAt: user.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}
