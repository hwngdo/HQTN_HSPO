package accounts

import (
	"errors"
	"net/http"
	"strings"
)

// ErrInactive is deliberately indistinguishable from a failed login on the wire.
// Never disclose whether the username exists or is locked to unauthenticated callers.
var ErrInactive = errors.New("invalid credentials or inactive account")
var ErrNoSession = errors.New("session is not authorized")

type SessionIdentity struct {
	Username string
}

// ResolveSession MUST be implemented using the real Core server's validated
// server-side session. This code must never take username/role from client JSON,
// headers, query strings, or localStorage as proof of identity.
type ResolveSession func(*http.Request) (SessionIdentity, error)

// CurrentUser checks current persisted account state on EACH request, not the
// privileges cached at the time the session was issued. Thus locking an account
// or revoking admin role takes effect on the next request.
func (s *Store) CurrentUser(username string) (PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, _, err := s.readLocked()
	if err != nil {
		return PublicUser{}, err
	}
	i, err := find(users, username)
	if err != nil {
		return PublicUser{}, ErrNoSession
	}
	u := users[i]
	if !getBool(u, "active") {
		return PublicUser{}, ErrNoSession
	}
	return PublicUser{Username: getString(u, "username"), DisplayName: canonicalName(u), Role: getString(u, "role"), Active: true}, nil
}

// AdminFromSession is a safe authorization adapter for the future Core route.
// It is not a replacement for Core's authentication/session implementation.
func (s *Store) AdminFromSession(resolve ResolveSession) Authorize {
	return func(r *http.Request) bool {
		if resolve == nil {
			return false
		}
		id, err := resolve(r)
		if err != nil || strings.TrimSpace(id.Username) == "" {
			return false
		}
		current, err := s.CurrentUser(id.Username)
		return err == nil && current.Active && strings.EqualFold(current.Role, "admin")
	}
}

// ValidateLoginUser is the server-side account-state check to call AFTER
// verifying credentials, and BEFORE creating any new session.
func (s *Store) ValidateLoginUser(username string) (PublicUser, error) {
	x, err := s.CurrentUser(username)
	if err != nil {
		return PublicUser{}, ErrInactive
	}
	return x, nil
}
