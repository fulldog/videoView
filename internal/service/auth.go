package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"videoview/internal/config"
	"videoview/internal/middleware"
	"videoview/internal/model"
	"videoview/internal/repository"
)

var (
	ErrInvalidCredential = errors.New("invalid username or password")
	ErrUserExists        = errors.New("username already exists")
	ErrWeakPassword      = errors.New("password must be at least 8 characters")
	ErrBadUsername       = errors.New("username must be 3-32 letters, digits or underscore")
	ErrOldPassword       = errors.New("old password mismatch")
	ErrResetInvalid      = errors.New("reset token invalid or expired")
	ErrRoleNotFound      = errors.New("role not found")
)

var usernameRE = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

type AuthService struct {
	users  *repository.UserRepo
	roles  *repository.RoleRepo
	resets *repository.ResetRepo
	cfg    *config.Config
}

func NewAuthService(users *repository.UserRepo, roles *repository.RoleRepo, resets *repository.ResetRepo, cfg *config.Config) *AuthService {
	return &AuthService{users: users, roles: roles, resets: resets, cfg: cfg}
}

func (s *AuthService) Register(ctx context.Context, username, password string) (*model.User, string, error) {
	if !usernameRE.MatchString(username) {
		return nil, "", ErrBadUsername
	}
	if len(password) < 8 {
		return nil, "", ErrWeakPassword
	}
	if _, err := s.users.FindByUsername(ctx, username); err == nil {
		return nil, "", ErrUserExists
	} else if !repository.IsNotFound(err) {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", err
	}
	role, err := s.roles.FindByCode(ctx, model.RoleUser)
	if err != nil {
		return nil, "", err
	}
	user := &model.User{
		Username:     username,
		PasswordHash: string(hash),
		Status:       model.UserStatusActive,
	}
	if err := s.users.Create(ctx, user); err != nil {
		return nil, "", err
	}
	if err := s.users.AttachRoles(ctx, user, []model.Role{*role}); err != nil {
		return nil, "", err
	}
	user, err = s.users.FindByID(ctx, user.ID)
	if err != nil {
		return nil, "", err
	}
	token, err := s.signToken(user)
	return user, token, err
}

func (s *AuthService) Login(ctx context.Context, username, password string) (*model.User, string, error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if repository.IsNotFound(err) {
			return nil, "", ErrInvalidCredential
		}
		return nil, "", err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredential
	}
	if user.Status != model.UserStatusActive {
		return nil, "", ErrInvalidCredential
	}
	token, err := s.signToken(user)
	return user, token, err
}

func (s *AuthService) ChangePassword(ctx context.Context, userID uint, oldPwd, newPwd string) error {
	if len(newPwd) < 8 {
		return ErrWeakPassword
	}
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPwd)); err != nil {
		return ErrOldPassword
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.users.UpdatePassword(ctx, userID, string(hash))
}

func (s *AuthService) RequestReset(ctx context.Context, username string) (token string, returned bool, err error) {
	user, err := s.users.FindByUsername(ctx, username)
	if err != nil {
		if repository.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", false, err
	}
	plain := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plain))
	rec := &model.PasswordReset{
		UserID:    user.ID,
		TokenHash: hex.EncodeToString(sum[:]),
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	if err := s.resets.Create(ctx, rec); err != nil {
		return "", false, err
	}
	if s.cfg.IsDev() {
		return plain, true, nil
	}
	return "", false, nil
}

func (s *AuthService) ResetPassword(ctx context.Context, token, newPwd string) error {
	if len(newPwd) < 8 {
		return ErrWeakPassword
	}
	sum := sha256.Sum256([]byte(token))
	rec, err := s.resets.FindValid(ctx, hex.EncodeToString(sum[:]), time.Now())
	if err != nil {
		if repository.IsNotFound(err) {
			return ErrResetInvalid
		}
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, rec.UserID, string(hash)); err != nil {
		return err
	}
	return s.resets.MarkUsed(ctx, rec.ID)
}

func (s *AuthService) signToken(user *model.User) (string, error) {
	claims := middleware.Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.JWT.Expire)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(s.cfg.JWT.Secret))
}

type RBACService struct {
	users *repository.UserRepo
	roles *repository.RoleRepo
	perms *repository.PermRepo
}

func NewRBACService(users *repository.UserRepo, roles *repository.RoleRepo, perms *repository.PermRepo) *RBACService {
	return &RBACService{users: users, roles: roles, perms: perms}
}

func (s *RBACService) ListUsers(ctx context.Context) ([]model.User, error) {
	return s.users.List(ctx)
}

func (s *RBACService) ListRoles(ctx context.Context) ([]model.Role, error) {
	return s.roles.List(ctx)
}

func (s *RBACService) ListPerms(ctx context.Context) ([]model.Permission, error) {
	return s.perms.List(ctx)
}

func (s *RBACService) CreateRole(ctx context.Context, name, code string, permIDs []uint) (*model.Role, error) {
	perms, err := s.perms.FindByIDs(ctx, permIDs)
	if err != nil {
		return nil, err
	}
	role := &model.Role{Name: name, Code: code}
	if err := s.roles.Create(ctx, role); err != nil {
		return nil, err
	}
	if err := s.roles.ReplacePermissions(ctx, role, perms); err != nil {
		return nil, err
	}
	role.Permissions = perms
	return role, nil
}

func (s *RBACService) UpdateRole(ctx context.Context, id uint, name string, permIDs []uint) (*model.Role, error) {
	role, err := s.roles.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if name != "" {
		role.Name = name
	}
	if err := s.roles.Update(ctx, role); err != nil {
		return nil, err
	}
	if permIDs != nil {
		perms, err := s.perms.FindByIDs(ctx, permIDs)
		if err != nil {
			return nil, err
		}
		if err := s.roles.ReplacePermissions(ctx, role, perms); err != nil {
			return nil, err
		}
		role.Permissions = perms
	}
	return role, nil
}

func (s *RBACService) SetUserRoles(ctx context.Context, userID uint, roleIDs []uint) error {
	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	roles, err := s.roles.FindByIDs(ctx, roleIDs)
	if err != nil {
		return err
	}
	if len(roles) != len(roleIDs) {
		return ErrRoleNotFound
	}
	return s.users.ReplaceRoles(ctx, user, roles)
}

func PublicUser(u *model.User) map[string]any {
	codes := []string{}
	roleCodes := []string{}
	for _, r := range u.Roles {
		roleCodes = append(roleCodes, r.Code)
		for _, p := range r.Permissions {
			codes = append(codes, p.Code)
		}
	}
	return map[string]any{
		"id":          u.ID,
		"username":    u.Username,
		"status":      u.Status,
		"roles":       roleCodes,
		"permissions": unique(codes),
	}
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func MapAuthError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrInvalidCredential), errors.Is(err, ErrOldPassword), errors.Is(err, ErrResetInvalid):
		return 40100, err.Error()
	case errors.Is(err, ErrUserExists), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrBadUsername), errors.Is(err, ErrRoleNotFound):
		return 40000, err.Error()
	default:
		return 50000, "internal error"
	}
}
