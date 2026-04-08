package intercept

import "context"

type Query interface {
	WhereP(...func(s interface{ SetP(func(context.Context, interface{}) error) }))
}

type TraverseFunc func(context.Context, Query) error
