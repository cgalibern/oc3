package cdb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// IdentityUser is the collector account an OpenID Connect identity resolves to.
type IdentityUser struct {
	ID    int64
	Email string
	// Locked is true when web2py would refuse the account a sign-in: a
	// registration key set ("pending", "blocked", "disabled").
	Locked bool
}

func (oDb *DB) identityUser(ctx context.Context, where string, arg any) (*IdentityUser, error) {
	var (
		u     IdentityUser
		email sql.NullString
		key   sql.NullString
	)
	err := oDb.DB.QueryRowContext(ctx,
		"SELECT auth_user.id, auth_user.email, auth_user.registration_key FROM auth_user "+where, arg).
		Scan(&u.ID, &email, &key)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	u.Email = email.String
	u.Locked = strings.TrimSpace(key.String) != ""
	return &u, nil
}

// UserByIdentity returns the account linked to the identity (issuer, subject), or
// nil when the identity is not linked to any.
func (oDb *DB) UserByIdentity(ctx context.Context, issuer, subject string) (*IdentityUser, error) {
	var userID int64
	err := oDb.DB.QueryRowContext(ctx,
		"SELECT user_id FROM auth_user_identities WHERE issuer = ? AND subject = ?", issuer, subject).
		Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("UserByIdentity: %w", err)
	}
	u, err := oDb.identityUser(ctx, "WHERE auth_user.id = ?", userID)
	if err != nil {
		return nil, fmt.Errorf("UserByIdentity: %w", err)
	}
	return u, nil
}

// UserByEmailForIdentity returns the account with this email, or nil. More than one
// account with the same email is refused: linking would pick one arbitrarily.
func (oDb *DB) UserByEmailForIdentity(ctx context.Context, email string) (*IdentityUser, error) {
	var n int
	if err := oDb.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM auth_user WHERE email = ?", email).Scan(&n); err != nil {
		return nil, fmt.Errorf("UserByEmailForIdentity: %w", err)
	}
	switch {
	case n == 0:
		return nil, nil
	case n > 1:
		return nil, fmt.Errorf("UserByEmailForIdentity: %d accounts share the email %s", n, email)
	}
	u, err := oDb.identityUser(ctx, "WHERE auth_user.email = ?", email)
	if err != nil {
		return nil, fmt.Errorf("UserByEmailForIdentity: %w", err)
	}
	return u, nil
}

// LinkIdentity links the identity (issuer, subject) to the account userID.
func (oDb *DB) LinkIdentity(ctx context.Context, userID int64, issuer, subject string) error {
	if _, err := oDb.ExecContext(ctx,
		"INSERT INTO auth_user_identities (user_id, issuer, subject, created, last_login) VALUES (?, ?, ?, NOW(), NOW())",
		userID, issuer, subject); err != nil {
		return fmt.Errorf("LinkIdentity: %w", err)
	}
	oDb.SetChange("auth_user_identities")
	return nil
}

// TouchIdentity records a sign-in through the identity (issuer, subject).
func (oDb *DB) TouchIdentity(ctx context.Context, issuer, subject string) error {
	if _, err := oDb.ExecContext(ctx,
		"UPDATE auth_user_identities SET last_login = NOW() WHERE issuer = ? AND subject = ?",
		issuer, subject); err != nil {
		return fmt.Errorf("TouchIdentity: %w", err)
	}
	return nil
}

// UpdateUserNames sets the first and last names of an account, as the provider
// gives them; an empty value leaves the column as it is.
func (oDb *DB) UpdateUserNames(ctx context.Context, userID int64, firstName, lastName string) error {
	sets := []string{}
	args := []any{}
	if firstName != "" {
		sets = append(sets, "first_name = ?")
		args = append(args, firstName)
	}
	if lastName != "" {
		sets = append(sets, "last_name = ?")
		args = append(args, lastName)
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, userID)
	if _, err := oDb.ExecContext(ctx, "UPDATE auth_user SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...); err != nil {
		return fmt.Errorf("UpdateUserNames: %w", err)
	}
	oDb.SetChange("auth_user")
	return nil
}

// UserRoles returns the roles of the groups the account userID is a member of.
func (oDb *DB) UserRoles(ctx context.Context, userID int64) ([]string, error) {
	rows, err := oDb.DB.QueryContext(ctx,
		`SELECT auth_group.role FROM auth_membership
		JOIN auth_group ON auth_group.id = auth_membership.group_id
		WHERE auth_membership.user_id = ?`, userID)
	if err != nil {
		return nil, fmt.Errorf("UserRoles: %w", err)
	}
	defer func() { _ = rows.Close() }()
	roles := []string{}
	for rows.Next() {
		var role sql.NullString
		if err := rows.Scan(&role); err != nil {
			return nil, fmt.Errorf("UserRoles scan: %w", err)
		}
		if role.Valid {
			roles = append(roles, role.String)
		}
	}
	return roles, rows.Err()
}

// UserHasIdentity tells whether an OpenID Connect identity is linked to the
// user: their password is then the provider's.
func (oDb *DB) UserHasIdentity(ctx context.Context, userID int64) (bool, error) {
	var linked bool
	err := oDb.DB.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM auth_user_identities WHERE user_id = ?)", userID).Scan(&linked)
	return linked, err
}
