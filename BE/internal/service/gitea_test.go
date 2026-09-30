package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/kuayle/kuayle-backend/internal/domain"
	"github.com/kuayle/kuayle-backend/internal/repository"
	"github.com/kuayle/kuayle-backend/pkg/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// stubGiteaRepo implements only the method under test; every other method
// panics if called, which keeps these tests honest.
type stubGiteaRepo struct {
	repository.GiteaRepo
	cfg *domain.GiteaOAuthConfig
}

func (s *stubGiteaRepo) GetOAuthConfigByWorkspace(_ context.Context, _ uuid.UUID) (*domain.GiteaOAuthConfig, error) {
	return s.cfg, nil
}

func signPayload(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func newWebhookService(t *testing.T, webhookSecret string) *GiteaService {
	t.Helper()
	key := crypto.DeriveKey("gitea-webhook-test")
	svc := &GiteaService{
		gtRepo:        &stubGiteaRepo{},
		encryptionKey: key,
	}
	if webhookSecret != "" {
		encrypted, err := crypto.Encrypt(webhookSecret, key)
		require.NoError(t, err)
		svc.gtRepo = &stubGiteaRepo{cfg: &domain.GiteaOAuthConfig{
			ID:            uuid.New(),
			WorkspaceID:   uuid.New(),
			InstanceURL:   "https://gitea.example.lan",
			WebhookSecret: encrypted,
		}}
	}
	return svc
}

// The workspace must be resolved *before* verification, otherwise the lookup
// always misses and every signed webhook gets rejected.
func TestVerifyWebhookSignature_UsesWorkspaceSecret(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"created","issue":{"number":1}}`)
	secret := "s3cr3t"
	svc := newWebhookService(t, secret)

	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload(secret, payload)),
		"a valid signature must be accepted")
	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, "deadbeef"),
		"a wrong signature must be rejected")
	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, ""),
		"an unsigned webhook must be rejected when a secret is configured")
}

// When no webhook secret is configured on our side we cannot verify anything,
// so the webhook is accepted (both signed and unsigned).
func TestVerifyWebhookSignature_SkipsWhenNoSecretConfigured(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"created","issue":{"number":1}}`)
	svc := newWebhookService(t, "")

	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, ""))
	require.True(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload("whatever", payload)))
}

// A signature produced with a different secret must not be accepted.
func TestVerifyWebhookSignature_RejectsForeignSecret(t *testing.T) {
	ctx := context.Background()
	payload := []byte(`{"action":"opened"}`)
	svc := newWebhookService(t, "my-secret")

	require.False(t, svc.VerifyWebhookSignature(ctx, uuid.New(), payload, signPayload("other-secret", payload)))
}
