package repository

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"videoview/internal/model"
)

func OpenMySQL(dsn string, log *zap.Logger, isDev bool) (*gorm.DB, error) {
	level := gormlogger.Warn
	if isDev {
		level = gormlogger.Info
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: NewZapGormLogger(log, level),
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(10 * time.Minute)
	return db, nil
}

func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(
		&model.User{},
		&model.Role{},
		&model.Permission{},
		&model.PasswordReset{},
		&model.UploadSession{},
		&model.Video{},
		&model.VideoRendition{},
	)
}

func Seed(ctx context.Context, db *gorm.DB) error {
	perms := []model.Permission{
		{Code: model.PermVideoUpload, Name: "上传视频"},
		{Code: model.PermVideoRead, Name: "观看视频"},
		{Code: model.PermVideoDownload, Name: "下载视频"},
		{Code: model.PermVideoManage, Name: "管理全部视频"},
		{Code: model.PermRBACManage, Name: "管理角色权限"},
	}
	for i := range perms {
		if err := db.WithContext(ctx).Where("code = ?", perms[i].Code).FirstOrCreate(&perms[i], perms[i]).Error; err != nil {
			return err
		}
	}
	var all []model.Permission
	if err := db.WithContext(ctx).Find(&all).Error; err != nil {
		return err
	}
	adminRole := model.Role{Name: "管理员", Code: model.RoleAdmin}
	if err := db.WithContext(ctx).Where("code = ?", model.RoleAdmin).FirstOrCreate(&adminRole, adminRole).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Model(&adminRole).Association("Permissions").Replace(all); err != nil {
		return err
	}

	var userPerms []model.Permission
	if err := db.WithContext(ctx).Where("code IN ?", []string{
		model.PermVideoUpload, model.PermVideoRead, model.PermVideoDownload,
	}).Find(&userPerms).Error; err != nil {
		return err
	}
	userRole := model.Role{Name: "普通用户", Code: model.RoleUser}
	if err := db.WithContext(ctx).Where("code = ?", model.RoleUser).FirstOrCreate(&userRole, userRole).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Model(&userRole).Association("Permissions").Replace(userPerms); err != nil {
		return err
	}

	var admin model.User
	err := db.WithContext(ctx).Where("username = ?", "admin").First(&admin).Error
	if err == gorm.ErrRecordNotFound {
		hash, herr := hashPassword("Admin@123")
		if herr != nil {
			return herr
		}
		admin = model.User{Username: "admin", PasswordHash: hash, Status: model.UserStatusActive}
		if err := db.WithContext(ctx).Create(&admin).Error; err != nil {
			return err
		}
		if err := db.WithContext(ctx).Model(&admin).Association("Roles").Append(&adminRole); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}
