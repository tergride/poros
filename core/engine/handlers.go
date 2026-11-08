package engine

import (
	"context"
	"encoding/json"

	C "github.com/sagernet/sing-box/constant"
	"github.com/xtls/xray-core/core"

	"PorosCore/internal/jsonrpc"
)

func (e *Engine) Info() AppInfo {
	return AppInfo{SingBox: C.Version, Xray: core.Version(), DataDir: e.dataDir, Elevated: elevated()}
}

// Handlers maps the methods of api.go onto the engine.
func (e *Engine) Handlers() map[string]jsonrpc.Handler {
	return map[string]jsonrpc.Handler{
		MethodAppInfo:             noParams(func() (AppInfo, error) { return e.Info(), nil }),
		MethodAppHandOver:         noParams(func() (any, error) { e.HandOver(); return nil, nil }),
		MethodSettingsGet:         noParams(e.Settings),
		MethodSettingsSet:         withParams(e.SetSettings),
		MethodGroupsList:          noParams(e.Groups),
		MethodGroupsAdd:           withParams(e.AddGroup),
		MethodGroupsRename:        withParams(e.RenameGroup),
		MethodGroupsRemove:        withParams(func(p ByID) (any, error) { return nil, e.RemoveGroup(p.ID) }),
		MethodGroupsConfigure:     withParams(e.ConfigureGroup),
		MethodSubscriptionsAdd:    withContext(e.AddSubscription),
		MethodSubscriptionsUpdate: withContext(func(ctx context.Context, p ByID) (SubscriptionUpdate, error) { return e.UpdateSubscription(ctx, p.ID) }),
		MethodServersList:         withParams(e.Servers),
		MethodServersAdd:          withParams(e.AddServers),
		MethodServersGet:          withParams(e.Server),
		MethodServersSave:         withParams(e.SaveServer),
		MethodServersRemove:       withParams(func(p ServerIDs) (any, error) { return nil, e.RemoveServers(p) }),
		MethodServersClone:        withParams(e.CloneServers),
		MethodServersMove:         withParams(e.MoveServers),
		MethodServersLinks:        withParams(e.ShareLinks),
		MethodServersDeepLinks:    withParams(e.DeepLinks),
		MethodServersExport:       withParams(e.ExportServer),
		MethodPresetsList:         noParams(e.Presets),
		MethodPresetsGet:          withParams(func(p ByID) (PresetCopy, error) { return e.PresetCopy(p.ID) }),
		MethodPresetsSave:         withParams(e.SavePreset),
		MethodPresetsRemove:       withParams(func(p ByID) (any, error) { return nil, e.RemovePreset(p.ID) }),
		MethodPresetsExport:       withParams(func(p ByID) (PresetLink, error) { return e.ExportPreset(p.ID) }),
		MethodPresetsImport:       withParams(e.ImportPreset),
		MethodPresetsWriteLists:   withParams(WritePresetLists),
		MethodPresetsReadLists:    withParams(ReadPresetLists),
		MethodLinksParse:          withParams(e.ParseLink),
		MethodServersTest:         withParams(e.TestServers),
		MethodLogsGet:             noParams(e.Logs),
		MethodConnectionStart:     noParams(e.StartConnection),
		MethodConnectionStop:      noParams(e.StopConnection),
		MethodConnectionState:     noParams(func() (ConnectionState, error) { return e.ConnectionState(), nil }),
		MethodNetworkCheck:        noParams(e.CheckNetwork),
		MethodNetworkState:        noParams(func() (NetworkState, error) { return e.NetworkState(), nil }),
		MethodKillSwitchState:     noParams(func() (KillSwitchState, error) { return e.KillSwitchState(), nil }),
		MethodUpdatesState:        noParams(func() (UpdateState, error) { return e.UpdatesState(), nil }),
		MethodUpdatesCheck:        noParams(e.CheckUpdates),
		MethodUpdatesInstall:      noParams(e.InstallUpdate),
	}
}

func noParams[R any](f func() (R, error)) jsonrpc.Handler {
	return func(context.Context, json.RawMessage) (any, error) { return f() }
}

func withParams[P, R any](f func(P) (R, error)) jsonrpc.Handler {
	return withContext(func(_ context.Context, params P) (R, error) { return f(params) })
}

// withContext is for methods that wait on the network: the request's context ends with the host's connection.
func withContext[P, R any](f func(context.Context, P) (R, error)) jsonrpc.Handler {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var params P
		if err := jsonrpc.DecodeParams(raw, &params); err != nil {
			return nil, err
		}
		return f(ctx, params)
	}
}
