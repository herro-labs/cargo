package cargo

import (
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// handleServiceCall routes the call to the appropriate service method with hooks
func (a *App) handleServiceCall(ctx *Context, req interface{}, fullMethod string) (interface{}, error) {
	// Extract service and method names
	parts := strings.Split(fullMethod, "/")
	if len(parts) < 3 {
		return nil, status.Error(codes.Internal, "invalid method name")
	}

	serviceName := strings.Split(parts[1], ".")[1] // Remove package prefix
	methodName := parts[2]

	// Get hooks for this service
	hooks, exists := a.hooks[serviceName]
	if !exists {
		hooks = Hooks{} // Empty hooks if none registered
	}

	// Handle different method types
	switch methodName {
	case "Login":
		return a.handleLogin(ctx, req, hooks)
	case "Create":
		return a.handleCreate(ctx, req, hooks)
	case "Get", "Read":
		return a.handleRead(ctx, req, hooks)
	case "List":
		return a.handleList(ctx, req, hooks)
	case "Delete":
		return a.handleDelete(ctx, req, hooks)
	case "Update":
		return a.handleUpdate(ctx, req, hooks)
	default:
		return nil, status.Errorf(codes.Unimplemented, "method %s not implemented", methodName)
	}
}

// callHook uses reflection to call a hook function with proper types
func callHook(hook interface{}, ctx *Context, req interface{}) ([]reflect.Value, error) {
	if hook == nil {
		return nil, nil
	}

	hookValue := reflect.ValueOf(hook)
	if hookValue.Kind() != reflect.Func {
		return nil, fmt.Errorf("hook is not a function")
	}

	// Prepare arguments
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(req),
	}

	// Call the function
	results := hookValue.Call(args)
	return results, nil
}

// callQueryHook calls a hook that takes a Query parameter
func callQueryHook(hook interface{}, ctx *Context, query *Query) error {
	if hook == nil {
		return nil
	}

	hookValue := reflect.ValueOf(hook)
	if hookValue.Kind() != reflect.Func {
		return fmt.Errorf("hook is not a function")
	}

	// Prepare arguments
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(query),
	}

	// Call the function
	results := hookValue.Call(args)
	if len(results) > 0 && !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}

// handleLogin processes login requests with authentication hooks
func (a *App) handleLogin(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	if hooks.BeforeLogin == nil {
		return nil, status.Error(codes.Unimplemented, "login hook not implemented")
	}

	// Call before login hook
	results, err := callHook(hooks.BeforeLogin, ctx, req)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if len(results) < 2 {
		return nil, status.Error(codes.Internal, "invalid hook return values")
	}

	// Check for error
	if !results[1].IsNil() {
		return nil, status.Error(codes.PermissionDenied, results[1].Interface().(error).Error())
	}

	// Get auth identity
	identity := results[0].Interface().(*AuthIdentity)

	// Generate JWT token
	token, err := generateJWT(identity)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to generate token")
	}

	// Create response with token
	return map[string]interface{}{
		"token": token,
	}, nil
}

// handleCreate processes create requests with hooks
func (a *App) handleCreate(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	// Call before create hook
	if hooks.BeforeCreate != nil {
		results, err := callHook(hooks.BeforeCreate, ctx, req)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.InvalidArgument, results[0].Interface().(error).Error())
		}
	}

	// Persist to MongoDB
	if err := a.createInMongoDB(ctx, req); err != nil {
		return nil, err
	}

	// Call after create hook
	if hooks.AfterCreate != nil {
		results, err := callHook(hooks.AfterCreate, ctx, req)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.Internal, results[0].Interface().(error).Error())
		}
	}

	return req, nil
}

// handleRead processes read requests with hooks
func (a *App) handleRead(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	// Read from MongoDB
	result, err := a.readFromMongoDB(ctx, req)
	if err != nil {
		return nil, err
	}

	// Call after read hook
	if hooks.AfterRead != nil {
		results, err := callHook(hooks.AfterRead, ctx, result)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.Internal, results[0].Interface().(error).Error())
		}
	}

	return result, nil
}

// handleList processes list requests with hooks
func (a *App) handleList(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	query := NewQuery()

	// Call before list hook
	if hooks.BeforeList != nil {
		if err := callQueryHook(hooks.BeforeList, ctx, query); err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
	}

	// Query MongoDB
	results, err := a.listFromMongoDB(ctx, req, query)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// handleDelete processes delete requests with hooks
func (a *App) handleDelete(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	// Call before delete hook
	if hooks.BeforeDelete != nil {
		results, err := callHook(hooks.BeforeDelete, ctx, req)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.PermissionDenied, results[0].Interface().(error).Error())
		}
	}

	// Delete from MongoDB
	if err := a.deleteFromMongoDB(ctx, req); err != nil {
		return nil, err
	}

	return req, nil
}

// handleUpdate processes update requests with hooks
func (a *App) handleUpdate(ctx *Context, req interface{}, hooks Hooks) (interface{}, error) {
	// Call before update hook
	if hooks.BeforeUpdate != nil {
		results, err := callHook(hooks.BeforeUpdate, ctx, req)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.InvalidArgument, results[0].Interface().(error).Error())
		}
	}

	// Update in MongoDB
	if err := a.updateInMongoDB(ctx, req); err != nil {
		return nil, err
	}

	// Call after update hook
	if hooks.AfterUpdate != nil {
		results, err := callHook(hooks.AfterUpdate, ctx, req)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}

		if len(results) > 0 && !results[0].IsNil() {
			return nil, status.Error(codes.Internal, results[0].Interface().(error).Error())
		}
	}

	return req, nil
}
