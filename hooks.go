package cargo

import (
	"context"
)

// Hook function types for type-safe hook implementation
type BeforeCreateFunc[T any] func(ctx *Context, req *T) error
type BeforeReadFunc[T any] func(ctx *Context, req *T) error
type BeforeUpdateFunc[T any] func(ctx *Context, req *T) error
type BeforeDeleteFunc[T any] func(ctx *Context, req *T) error
type BeforeListFunc[T any] func(ctx *Context, query *Query) error
type BeforeLoginFunc[T any] func(ctx *Context, req *T) (*AuthIdentity, error)

type AfterCreateFunc[T any] func(ctx *Context, resp *T) error
type AfterReadFunc[T any] func(ctx *Context, resp *T) error
type AfterUpdateFunc[T any] func(ctx *Context, resp *T) error
type AfterDeleteFunc[T any] func(ctx *Context, resp *T) error

// TypedHooks contains type-safe hooks for a specific message type
type TypedHooks[T any] struct {
	BeforeCreate BeforeCreateFunc[T]
	BeforeRead   BeforeReadFunc[T]
	BeforeUpdate BeforeUpdateFunc[T]
	BeforeDelete BeforeDeleteFunc[T]
	BeforeList   BeforeListFunc[T]
	BeforeLogin  BeforeLoginFunc[T]

	AfterCreate AfterCreateFunc[T]
	AfterRead   AfterReadFunc[T]
	AfterUpdate AfterUpdateFunc[T]
	AfterDelete AfterDeleteFunc[T]
}

// HookRegistry manages typed hooks for different message types
type HookRegistry struct {
	hooks map[string]interface{} // serviceName -> TypedHooks[T]
}

func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		hooks: make(map[string]interface{}),
	}
}

func RegisterHooks[T any](r *HookRegistry, serviceName string, hooks TypedHooks[T]) {
	r.hooks[serviceName] = hooks
}

func GetHooks[T any](r *HookRegistry, serviceName string) (TypedHooks[T], bool) {
	if hooks, exists := r.hooks[serviceName]; exists {
		if typedHooks, ok := hooks.(TypedHooks[T]); ok {
			return typedHooks, true
		}
	}
	return TypedHooks[T]{}, false
}

// ServiceHandler provides type-safe service handling
type ServiceHandler[T any] struct {
	app         *App
	serviceName string
	hooks       TypedHooks[T]
	validator   *Validator
}

func NewServiceHandler[T any](app *App, serviceName string, hooks TypedHooks[T]) *ServiceHandler[T] {
	return &ServiceHandler[T]{
		app:         app,
		serviceName: serviceName,
		hooks:       hooks,
		validator:   NewValidator(),
	}
}

// WithValidator sets a custom validator for the service handler
func (s *ServiceHandler[T]) WithValidator(validator *Validator) *ServiceHandler[T] {
	s.validator = validator
	return s
}

func (s *ServiceHandler[T]) Create(ctx context.Context, req *T) (*T, error) {
	cargoCtx := s.app.newContext(ctx)

	// Validate request
	if err := s.validator.Validate(req); err != nil {
		return nil, ToGRPCError(err)
	}

	// Before create hook
	if s.hooks.BeforeCreate != nil {
		if err := s.hooks.BeforeCreate(cargoCtx, req); err != nil {
			return nil, ToGRPCError(err)
		}
	}

	// Persist to database
	if err := s.app.createInMongoDB(cargoCtx, req); err != nil {
		return nil, err
	}

	// After create hook
	if s.hooks.AfterCreate != nil {
		if err := s.hooks.AfterCreate(cargoCtx, req); err != nil {
			return nil, ToGRPCError(err)
		}
	}

	return req, nil
}

func (s *ServiceHandler[T]) Read(ctx context.Context, req *T) (*T, error) {
	cargoCtx := s.app.newContext(ctx)

	// Before read hook
	if s.hooks.BeforeRead != nil {
		if err := s.hooks.BeforeRead(cargoCtx, req); err != nil {
			return nil, err
		}
	}

	// Read from database
	result, err := s.app.readFromMongoDB(cargoCtx, req)
	if err != nil {
		return nil, err
	}

	typedResult, ok := result.(*T)
	if !ok {
		return nil, NewInternalError("type assertion failed")
	}

	// After read hook
	if s.hooks.AfterRead != nil {
		if err := s.hooks.AfterRead(cargoCtx, typedResult); err != nil {
			return nil, err
		}
	}

	return typedResult, nil
}

func (s *ServiceHandler[T]) Update(ctx context.Context, req *T) (*T, error) {
	cargoCtx := s.app.newContext(ctx)

	// Validate request
	if err := s.validator.Validate(req); err != nil {
		return nil, ToGRPCError(err)
	}

	// Before update hook
	if s.hooks.BeforeUpdate != nil {
		if err := s.hooks.BeforeUpdate(cargoCtx, req); err != nil {
			return nil, ToGRPCError(err)
		}
	}

	// Update in database
	if err := s.app.updateInMongoDB(cargoCtx, req); err != nil {
		return nil, err
	}

	// After update hook
	if s.hooks.AfterUpdate != nil {
		if err := s.hooks.AfterUpdate(cargoCtx, req); err != nil {
			return nil, ToGRPCError(err)
		}
	}

	return req, nil
}

func (s *ServiceHandler[T]) Delete(ctx context.Context, req *T) (*T, error) {
	cargoCtx := s.app.newContext(ctx)

	// Before delete hook (validation happens in hook if needed)
	if s.hooks.BeforeDelete != nil {
		if err := s.hooks.BeforeDelete(cargoCtx, req); err != nil {
			return nil, ToGRPCError(err)
		}
	}

	// Delete from database
	if err := s.app.deleteFromMongoDB(cargoCtx, req); err != nil {
		return nil, err
	}

	return req, nil
}

func (s *ServiceHandler[T]) List(ctx context.Context, req *T) ([]T, error) {
	cargoCtx := s.app.newContext(ctx)
	query := NewQuery()

	// Before list hook
	if s.hooks.BeforeList != nil {
		if err := s.hooks.BeforeList(cargoCtx, query); err != nil {
			return nil, err
		}
	}

	// Query database
	results, err := s.app.listFromMongoDB(cargoCtx, req, query)
	if err != nil {
		return nil, err
	}

	typedResults, ok := results.([]T)
	if !ok {
		return nil, NewInternalError("type assertion failed for list results")
	}

	return typedResults, nil
}

func (s *ServiceHandler[T]) Login(ctx context.Context, req *T) (string, error) {
	cargoCtx := s.app.newContext(ctx)

	// Before login hook
	if s.hooks.BeforeLogin == nil {
		return "", NewUnimplementedError("login hook not implemented")
	}

	identity, err := s.hooks.BeforeLogin(cargoCtx, req)
	if err != nil {
		return "", err
	}

	// Generate JWT token
	token, err := s.app.generateJWT(identity)
	if err != nil {
		return "", NewInternalError("failed to generate token")
	}

	return token, nil
}
