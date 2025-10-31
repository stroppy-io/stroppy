package token

import (
	"context"
	"errors"

	"github.com/mitchellh/mapstructure"
)

func AccountFromClaims[T any](claims *Claims) (*T, error) {
	user := new(T)
	err := mapstructure.Decode(claims.Additional, user)
	return user, err
}

func AccountToTokenClaims[T any](a *T) (map[string]interface{}, error) {
	if a == nil {
		return make(map[string]interface{}), nil
	}
	mp := make(map[string]interface{})
	err := mapstructure.WeakDecode(a, &mp)
	return mp, err
}

var (
	ErrNoAccountInContext = errors.New("no user in context")
	ErrNoTokenInContext   = errors.New("no token in context")
)

type AccountCtxKeyType string

const AccountCtxKey = AccountCtxKeyType("account")

// CtxWithAccount sets the provided user in the given context.
//
// It takes a context.Context and a *User as parameters.
// It returns a context.Context.
func CtxWithAccount[T any](ctx context.Context, user *T) context.Context {
	return context.WithValue(ctx, AccountCtxKey, user)
}

// AccountFromCtx returns the user from the context.
//
// It takes a context.Context as a parameter.
// It returns a *User and an error.
func AccountFromCtx[T any](ctx context.Context) (*T, error) {
	account, ok := ctx.Value(AccountCtxKey).(*T)
	if !ok {
		return nil, ErrNoAccountInContext
	}

	return account, nil
}

type AccountTokenKeyType string

const AccountTokenKey = AccountTokenKeyType("account_token")

func CtxWithAccountToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, AccountTokenKey, token)
}

func AccountTokenFromCtx(ctx context.Context) (string, error) {
	token, ok := ctx.Value(AccountTokenKey).(string)
	if !ok {
		return "", ErrNoTokenInContext
	}

	return token, nil
}
