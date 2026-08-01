package web

import (
	"fmt"
	"strings"

	"cuelang.org/go/cue"

	"github.com/lucasew/contapila-go/internal/config"
	"github.com/lucasew/contapila-go/internal/plugin"
)

func fillDebug(pc PageContext, data *PageData) {
	if pc.Project == nil || pc.Project.Config == nil {
		data.Error = "no project config"
		return
	}
	cfg := pc.Project.Config.Value
	flags, _ := config.PluginFlags(cfg)

	infos := plugin.Infos()
	seen := map[string]bool{}
	for _, info := range infos {
		seen[info.ID] = true
		on := plugin.IsEnabled(cfg, info.ID)
		inCfg := flags != nil && containsPluginFlag(flags, info.ID)
		entry := plugin.PluginValue(cfg, info.ID)
		entryStr := formatCUE(entry)
		if !inCfg {
			if info.DefaultOn {
				entryStr = "(not in unified plugins map — DefaultOn)"
			} else {
				entryStr = "(not in unified plugins map — default off)"
			}
		}
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:        info.ID,
			Enabled:   on,
			InConfig:  inCfg,
			HasStream: info.HasStream,
			EntryCUE:  entryStr, // plain; highlighted in templ via codeHighlight
		})
	}
	for id, on := range flags {
		if seen[id] {
			continue
		}
		entry := plugin.PluginValue(cfg, id)
		data.PluginRows = append(data.PluginRows, PluginStatusRow{
			ID:       id,
			Enabled:  on,
			InConfig: true,
			EntryCUE: formatCUE(entry),
		})
	}

	pluginsNode := cfg.LookupPath(cue.ParsePath("plugins"))
	data.PluginsCUE = formatCUE(pluginsNode)
	data.ConfigCUE = formatCUE(cfg)
}

func containsPluginFlag(flags map[string]bool, id string) bool {
	_, ok := flags[id]
	return ok
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
