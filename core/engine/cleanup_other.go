//go:build !windows

package engine

// platformTraces are only those of the core itself until Linux and Android have their own.
func (e *Engine) platformTraces() []trace {
	return []trace{e.systemProxyTrace()}
}
