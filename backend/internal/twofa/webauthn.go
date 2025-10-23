package twofa

import (
	"crypto/sha256"
	"fmt"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// WebAuthnService WebAuthn 服务
type WebAuthnService struct {
	webAuthn *webauthn.WebAuthn
}

// NewWebAuthnService 创建 WebAuthn 服务实例
func NewWebAuthnService(rpDisplayName, rpID, rpOrigin string) (*WebAuthnService, error) {
	wconfig := &webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     []string{rpOrigin},
	}

	web, err := webauthn.New(wconfig)
	if err != nil {
		return nil, fmt.Errorf("create webauthn: %w", err)
	}

	return &WebAuthnService{
		webAuthn: web,
	}, nil
}

// UsernameToID 将用户名转换为 ID
func UsernameToID(username string) []byte {
	hash := sha256.Sum256([]byte(username))
	return hash[:]
}

// BeginRegistration 开始注册流程
func (s *WebAuthnService) BeginRegistration(username string, credentials []webauthn.Credential) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	user := &WebAuthnUser{
		ID:          UsernameToID(username),
		Name:        username,
		DisplayName: username,
		Credentials: credentials,
	}

	options, session, err := s.webAuthn.BeginRegistration(user)
	if err != nil {
		return nil, nil, fmt.Errorf("begin registration: %w", err)
	}

	return options, session, nil
}

// FinishRegistration 完成注册
func (s *WebAuthnService) FinishRegistration(username string, credentials []webauthn.Credential, sessionData webauthn.SessionData, response *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	user := &WebAuthnUser{
		ID:          UsernameToID(username),
		Name:        username,
		DisplayName: username,
		Credentials: credentials,
	}

	credential, err := s.webAuthn.CreateCredential(user, sessionData, response)
	if err != nil {
		return nil, fmt.Errorf("create credential: %w", err)
	}

	return credential, nil
}

// BeginLogin 开始验证流程
func (s *WebAuthnService) BeginLogin(username string, credentials []webauthn.Credential) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	user := &WebAuthnUser{
		ID:          UsernameToID(username),
		Name:        username,
		DisplayName: username,
		Credentials: credentials,
	}

	options, session, err := s.webAuthn.BeginLogin(user)
	if err != nil {
		return nil, nil, fmt.Errorf("begin login: %w", err)
	}

	return options, session, nil
}

// FinishLogin 完成验证
func (s *WebAuthnService) FinishLogin(username string, credentials []webauthn.Credential, sessionData webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	user := &WebAuthnUser{
		ID:          UsernameToID(username),
		Name:        username,
		DisplayName: username,
		Credentials: credentials,
	}

	credential, err := s.webAuthn.ValidateLogin(user, sessionData, response)
	if err != nil {
		return nil, fmt.Errorf("validate login: %w", err)
	}

	return credential, nil
}
