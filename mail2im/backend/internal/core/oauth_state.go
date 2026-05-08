package core

import (
	"sync"
	"time"
)

type OAuthPendingState struct {
	Status    string // "pending", "done", "error"
	AccountID uint
	Error     string
	CreatedAt time.Time
}

var oauthStateStore sync.Map

func SetOAuthPending(state string) {
	oauthStateStore.Store(state, &OAuthPendingState{
		Status:    "pending",
		CreatedAt: time.Now(),
	})
}

func SetOAuthDone(state string, accountID uint) {
	if v, ok := oauthStateStore.Load(state); ok {
		s := v.(*OAuthPendingState)
		s.Status = "done"
		s.AccountID = accountID
	}
}

func SetOAuthError(state string, errMsg string) {
	if v, ok := oauthStateStore.Load(state); ok {
		s := v.(*OAuthPendingState)
		s.Status = "error"
		s.Error = errMsg
	}
}

func GetOAuthState(state string) (*OAuthPendingState, bool) {
	v, ok := oauthStateStore.Load(state)
	if !ok {
		return nil, false
	}
	return v.(*OAuthPendingState), true
}

func CleanupOAuthStates() {
	cutoff := time.Now().Add(-15 * time.Minute)
	oauthStateStore.Range(func(k, v interface{}) bool {
		s := v.(*OAuthPendingState)
		if s.CreatedAt.Before(cutoff) {
			oauthStateStore.Delete(k)
		}
		return true
	})
}
