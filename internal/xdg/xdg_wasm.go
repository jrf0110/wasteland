//go:build js

package xdg

const appName = "wasteland"

// ConfigHome returns an inert path in js builds.
func ConfigHome() string { return "/wasteland/config" }

// DataHome returns an inert path in js builds.
func DataHome() string { return "/wasteland/data" }

// ConfigDir returns the wasteland config directory.
func ConfigDir() string { return ConfigHome() + "/" + appName }

// DataDir returns the wasteland data directory.
func DataDir() string { return DataHome() + "/" + appName }
