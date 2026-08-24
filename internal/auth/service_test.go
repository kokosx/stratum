package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSetupCreatesHashedSessionAndAllowsLogin(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	service, err := NewService(database.DB, db.New(database.DB), false)
	if err != nil {
		t.Fatal(err)
	}
	if service.SetupCode() == "" {
		t.Fatal("fresh installation must have a setup code")
	}
	if _, err := service.Setup(ctx, "wrong", "Example", "admin@example.com", "a sufficiently long password"); err != ErrInvalidSetupCode {
		t.Fatalf("Setup with wrong code error = %v, want %v", err, ErrInvalidSetupCode)
	}

	token, err := service.Setup(ctx, service.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if service.SetupCode() != "" {
		t.Fatal("setup code must be discarded after successful setup")
	}
	var storedHash string
	if err := database.DB.QueryRowContext(ctx, "SELECT token_hash FROM sessions").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash == token {
		t.Fatal("session token was stored without hashing")
	}

	user, err := service.UserForToken(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "admin@example.com" || user.Role != "admin" {
		t.Fatalf("session user = %#v", user)
	}
	loginToken, err := service.Login(ctx, "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(ctx, loginToken); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UserForToken(ctx, loginToken); err == nil {
		t.Fatal("logged out session remains valid")
	}
}

func TestUpdateUserRevokesSessionsAndDisablesLogin(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(database.DB, db.New(database.DB), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Setup(ctx, service.SetupCode(), "Example", "admin@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateUser(ctx, "author@example.com", "a sufficiently long password", "author"); err != nil {
		t.Fatal(err)
	}
	token, err := service.Login(ctx, "author@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := db.New(database.DB).GetUserByEmail(ctx, "author@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateUser(ctx, user.ID, "author", "disabled"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UserForToken(ctx, token); err == nil {
		t.Fatal("disabled account session remains valid")
	}
	if _, err := service.Login(ctx, "author@example.com", "a sufficiently long password"); err != ErrInvalidCredentials {
		t.Fatalf("disabled login error = %v", err)
	}
}

func TestUpdateUserKeepsOneActiveAdministrator(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(database.DB, db.New(database.DB), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Setup(ctx, service.SetupCode(), "Example", "admin@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	admin, err := db.New(database.DB).GetUserByEmail(ctx, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateUser(ctx, admin.ID, "author", "active"); err == nil {
		t.Fatal("last active administrator was demoted")
	}
}

func TestUpdateUserKeepsOneActiveAdministratorWhenDisabled(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(database.DB, db.New(database.DB), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Setup(ctx, service.SetupCode(), "Example", "admin@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	admin, _ := db.New(database.DB).GetUserByEmail(ctx, "admin@example.com")
	if err := service.UpdateUser(ctx, admin.ID, "admin", "disabled"); err == nil {
		t.Fatal("last active administrator was disabled")
	}
}

func TestResetPasswordRejectsDisplayAddressAndRevokesSessions(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	service, err := NewService(database.DB, db.New(database.DB), false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Setup(ctx, service.SetupCode(), "Example", "admin@example.com", "a sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	if err := service.CreateUser(ctx, "Name <author@example.com>", "a sufficiently long password", "author"); err == nil {
		t.Fatal("display-name email was accepted")
	}
	if err := service.CreateUser(ctx, "AUTHOR@example.com", "a sufficiently long password", "author"); err != nil {
		t.Fatal(err)
	}
	token, err := service.Login(ctx, "author@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := db.New(database.DB).GetUserByEmail(ctx, "author@example.com")
	if err := service.ResetPassword(ctx, user.ID, "another sufficiently long password"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UserForToken(ctx, token); err == nil {
		t.Fatal("password reset did not revoke sessions")
	}
}
