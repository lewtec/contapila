package web

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/plugin"
)

func fillPlugins(pc PageContext, data *PageData) {
	if pc.Project == nil || pc.Project.Config == nil {
		data.Error = "no project config"
		return
	}
	cfg := pc.Project.Config.Value
	flags, _ := config.PluginFlags(cfg)

	seen := map[string]bool{}
	for _, info := range plugin.Infos() {
		seen[info.ID] = true
		on := plugin.IsEnabled(cfg, info.ID)
		if pc.Ledger != nil {
			on = pc.Ledger.PluginEnabled(info.ID, info.DefaultOn)
		}
		_, inCfg := flags[info.ID]
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:        info.ID,
			Enabled:   on,
			InConfig:  inCfg,
			HasStream: info.HasStream,
			EntryCUE:  pluginEntryCUE(cfg, info.ID, inCfg, info.DefaultOn),
		})
	}
	// Stream expanders are first-party engine steps, not RegisterTyped modules.
	for _, id := range config.KnownPluginIDs() {
		if seen[id] {
			continue
		}
		seen[id] = true
		defaultOn := streamModuleDefaultOn(id)
		on := defaultOn
		if pc.Ledger != nil {
			on = pc.Ledger.PluginEnabled(id, defaultOn)
		} else if b, ok := flags[id]; ok {
			on = b
		}
		_, inCfg := flags[id]
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:        id,
			Enabled:   on,
			InConfig:  inCfg,
			HasStream: true,
			EntryCUE:  pluginEntryCUE(cfg, id, inCfg, defaultOn),
		})
	}
	for id, on := range flags {
		if seen[id] {
			continue
		}
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:       id,
			Enabled:  on,
			InConfig: true,
			EntryCUE: formatCUE(plugin.PluginValue(cfg, id)),
		})
	}

	pluginsNode := cfg.LookupPath(cue.ParsePath("plugins"))
	data.PluginsCUE = formatCUE(pluginsNode)
}

func pluginEntryCUE(cfg cue.Value, id string, inCfg, defaultOn bool) string {
	if !inCfg {
		if defaultOn {
			return "(not in unified plugins map — DefaultOn)"
		}
		return "(not in unified plugins map — default off)"
	}
	return formatCUE(plugin.PluginValue(cfg, id))
}

func fillConfig(pc PageContext, data *PageData) {
	if pc.Project == nil || pc.Project.Config == nil {
		data.Error = "no project config"
		return
	}
	data.ConfigCUE = formatCUE(pc.Project.Config.Value)
}

func streamModuleDefaultOn(id string) bool {
	switch id {
	case "check_closing", "web_accounts":
		return false
	default:
		return true
	}
}

// formatCUE pretty-prints a unified CUE value as plain source text.
func formatCUE(v cue.Value) string {
	if !v.Exists() {
		return "(absent)"
	}
	s := fmt.Sprintf("%v", v)
	s = strings.TrimSpace(s)
	if s == "" {
		return "(empty)"
	}
	return s
}
