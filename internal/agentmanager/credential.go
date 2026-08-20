package agentmanager

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type CredentialManager struct {
	masterKey []byte
}

func NewCredentialManager(masterKey string) *CredentialManager {
	return &CredentialManager{masterKey: []byte(masterKey)}
}

func (cm *CredentialManager) Generate(agentID string) (token string, enc []byte, err error) {
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		return "", nil, fmt.Errorf("rand: %w", e)
	}
	token = hex.EncodeToString(raw)
	enc = cm.seal(agentID, token)
	return token, enc, nil
}

func (cm *CredentialManager) Validate(agentID, token string, enc []byte) bool {
	if len(enc) == 0 || token == "" {
		return false
	}
	expected := cm.seal(agentID, token)
	return hmac.Equal(expected, enc)
}

func (cm *CredentialManager) seal(agentID, token string) []byte {
	h := hmac.New(sha256.New, cm.masterKey)
	h.Write([]byte(agentID))
	h.Write([]byte{0x3a})
	h.Write([]byte(token))
	return h.Sum(nil)
}

func (m *Manager) ValidateAgentToken(ctx context.Context, agentID, token string) bool {
	if m.repo == nil || m.cred == nil {
		return false
	}
	agent, err := m.repo.GetAgent(ctx, agentID)
	if err != nil || agent == nil {
		return false
	}
	return m.cred.Validate(agentID, token, agent.DeviceTokenEnc)
}

func (m *Manager) GenerateAgentToken(agentID string) (token, encToken string, err error) {
	if m.cred == nil {
		return "", "", fmt.Errorf("credential manager not configured")
	}
	tok, enc, e := m.cred.Generate(agentID)
	if e != nil {
		return "", "", e
	}
	return tok, hex.EncodeToString(enc), nil
}