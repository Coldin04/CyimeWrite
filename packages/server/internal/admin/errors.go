package admin

import "errors"

var (
	ErrDocumentQuotaModeInvalid = errors.New("invalid document quota mode")
	ErrDocumentQuotaRequired    = errors.New("document quota is required when mode is custom")
	ErrDocumentQuotaInvalid     = errors.New("document quota must be a non-negative integer")
	ErrDocumentQuotaTooLarge    = errors.New("document quota exceeds the maximum allowed value")
	ErrDocumentQuotaMustBeEmpty = errors.New("document quota must be empty unless mode is custom")
	ErrEmailRequired            = errors.New("email is required")
	ErrEmailInvalid             = errors.New("invalid email address")
	ErrEmailAlreadyInUse        = errors.New("email is already in use")
	ErrEmailNotSet              = errors.New("user does not have an email address")
	ErrCannotRevokeOwnSession   = errors.New("cannot revoke your own sessions from admin panel")
	ErrCannotUnregisterSelf     = errors.New("cannot unregister yourself")
)
