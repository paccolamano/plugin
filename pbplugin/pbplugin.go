package pbplugin

import "github.com/pocketbase/pocketbase/core"

// PBPlugin is the interface that all PocketBase plugins must implement.
//
// Plugin repositories must:
//  1. Export a variable named "Plugin" of type PBPlugin
//  2. Publish releases (source code is downloaded and compiled locally)
//  3. Have a valid go.mod and expose the plugin package at the repository root
//
// Example plugin implementation:
//
//	var Plugin pbplugin.PBPlugin = &myPlugin{}
//
//	type myPlugin struct{}
//	func (p *myPlugin) Name() string               { return "myplugin" }
//	func (p *myPlugin) Version() string            { return "v1.0.0" }
//	func (p *myPlugin) Register(app core.App) error { ... }
type PBPlugin interface {
	Name() string
	Version() string
	Register(app core.App) error
}
