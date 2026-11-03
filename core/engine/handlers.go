package engine

import (
	"context"
	"encoding/json"

	C "github.com/sagernet/sing-box/constant"
	"github.com/xtls/xray-core/core"

	"PorosCore/internal/jsonrpc"
)

func (e *Engine) Info() AppInfo {
	return AppInfo{SingBox: C.Version, Xray: core.Version(), DataDir: e.dataDir}
}

// Handlers maps the methods of api.go onto the engine.
func (e *Engine) Handlers() map[string]jsonrpc.Handler {
	return map[string]jsonrpc.Handler{
		MethodAppInfo:             noParams(func() (AppInfo, error) { return e.Info(), nil }),
		MethodSettingsGet:         noParams(e.Settings),
		MethodSettingsSet:         withParams(e.SetSettings),
		MethodSubscriptionsList:   noParams(e.Subscriptions),
		MethodSubscriptionsAdd:    withParams(e.AddSubscription),
		MethodSubscriptionsRemove: withParams(func(p ByID) (any, error) { return nil, e.RemoveSubscription(p.ID) }),
		MethodPresetsList:         noParams(func() ([]Preset, error) { return e.Presets(), nil }),
	}
}

func noParams[R any](f func() (R, error)) jsonrpc.Handler {
	return func(context.Context, json.RawMessage) (any, error) { return f() }
}

func withParams[P, R any](f func(P) (R, error)) jsonrpc.Handler {
	return func(_ context.Context, raw json.RawMessage) (any, error) {
		var params P
		if err := jsonrpc.DecodeParams(raw, &params); err != nil {
			return nil, err
		}
		return f(params)
	}
}
