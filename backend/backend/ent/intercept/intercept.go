package intercept

import "context"

type Query interface {
	WhereP(...func(s interface {
		SetP(func(context.Context, any) error)
	}))
}

type TraverseFunc func(context.Context, Query) error
