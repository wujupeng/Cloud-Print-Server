package auth

import (
	"github.com/cloud-print/server/internal/errs"
)

var (
	ErrTokenExpired  = errs.New(errs.ErrJWTExpired, "token 已过期")
	ErrTokenInvalid  = errs.New(errs.ErrAuthInvalid, "token 无效")
	ErrTokenMissing  = errs.New(errs.ErrAuthInvalid, "缺少 Authorization 头")
	ErrAdminRequired = errs.New(errs.ErrAdminRequired, "需要管理员权限")
)