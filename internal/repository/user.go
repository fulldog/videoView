package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"videoview/internal/model"
)

type UserRepo struct {
	db *gorm.DB
}

func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	return r.db.WithContext(ctx).Create(user).Error
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Preload("Roles.Permissions").Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var user model.User
	err := r.db.WithContext(ctx).Preload("Roles.Permissions").First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepo) List(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := r.db.WithContext(ctx).Preload("Roles").Find(&users).Error
	return users, err
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id uint, hash string) error {
	return r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", id).Update("password_hash", hash).Error
}

func (r *UserRepo) ReplaceRoles(ctx context.Context, user *model.User, roles []model.Role) error {
	return r.db.WithContext(ctx).Model(user).Association("Roles").Replace(roles)
}

func (r *UserRepo) AttachRoles(ctx context.Context, user *model.User, roles []model.Role) error {
	return r.db.WithContext(ctx).Model(user).Association("Roles").Append(roles)
}

type RoleRepo struct {
	db *gorm.DB
}

func NewRoleRepo(db *gorm.DB) *RoleRepo {
	return &RoleRepo{db: db}
}

func (r *RoleRepo) List(ctx context.Context) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).Preload("Permissions").Find(&roles).Error
	return roles, err
}

func (r *RoleRepo) FindByID(ctx context.Context, id uint) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).Preload("Permissions").First(&role, id).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepo) FindByIDs(ctx context.Context, ids []uint) ([]model.Role, error) {
	var roles []model.Role
	err := r.db.WithContext(ctx).Preload("Permissions").Where("id IN ?", ids).Find(&roles).Error
	return roles, err
}

func (r *RoleRepo) FindByCode(ctx context.Context, code string) (*model.Role, error) {
	var role model.Role
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&role).Error
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RoleRepo) Create(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Create(role).Error
}

func (r *RoleRepo) Update(ctx context.Context, role *model.Role) error {
	return r.db.WithContext(ctx).Save(role).Error
}

func (r *RoleRepo) ReplacePermissions(ctx context.Context, role *model.Role, perms []model.Permission) error {
	return r.db.WithContext(ctx).Model(role).Association("Permissions").Replace(perms)
}

type PermRepo struct {
	db *gorm.DB
}

func NewPermRepo(db *gorm.DB) *PermRepo {
	return &PermRepo{db: db}
}

func (r *PermRepo) List(ctx context.Context) ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.WithContext(ctx).Find(&perms).Error
	return perms, err
}

func (r *PermRepo) FindByIDs(ctx context.Context, ids []uint) ([]model.Permission, error) {
	var perms []model.Permission
	err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&perms).Error
	return perms, err
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
