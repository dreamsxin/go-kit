package generator

import (
	"fmt"
	"strings"
)

// Normalize returns options with derived defaults filled in.
func (opt Options) Normalize() Options {
	if opt.OutputDir == "" {
		opt.OutputDir = "."
	}
	opt.ConfigMode = strings.TrimSpace(opt.ConfigMode)
	opt.RemoteProvider = strings.TrimSpace(opt.RemoteProvider)
	opt.DBDriver = strings.TrimSpace(opt.DBDriver)

	if opt.WithConfig && opt.ConfigMode == "" {
		opt.ConfigMode = "file"
	}
	for _, p := range opt.Protocols {
		if strings.EqualFold(strings.TrimSpace(p), "grpc") {
			opt.WithGRPC = true
			break
		}
	}
	return opt
}

// Validate checks generation options shared by CLI and direct generator callers.
func (opt Options) Validate() error {
	if opt.DBDriver != "" {
		if _, ok := supportedDrivers[opt.DBDriver]; !ok {
			return fmt.Errorf("unsupported db driver: %s", opt.DBDriver)
		}
	}

	if !opt.WithConfig {
		if opt.ConfigMode != "" {
			return fmt.Errorf("-config-mode requires -config=true")
		}
		if opt.RemoteProvider != "" {
			return fmt.Errorf("-remote-provider requires -config=true")
		}
		return nil
	}

	switch opt.ConfigMode {
	case "file", "hybrid", "remote":
	default:
		return fmt.Errorf("unsupported -config-mode %q (want file, hybrid, or remote)", opt.ConfigMode)
	}
	switch opt.RemoteProvider {
	case "", "consul":
	default:
		return fmt.Errorf("unsupported -remote-provider %q", opt.RemoteProvider)
	}
	if opt.ConfigMode == "file" && opt.RemoteProvider != "" {
		return fmt.Errorf("-remote-provider requires -config-mode=hybrid or -config-mode=remote")
	}
	if (opt.ConfigMode == "hybrid" || opt.ConfigMode == "remote") && opt.RemoteProvider == "" {
		return fmt.Errorf("-config-mode=%s requires -remote-provider", opt.ConfigMode)
	}
	return nil
}
