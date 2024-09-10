package driver

import "github.com/sirupsen/logrus"

type PluginInfo struct {
	Author     string // 作者
	PluginName string // 插件名
	Version    string // 版本
	Details    string // 插件信息
}

// IPlugin is the plugin of the ZeroBot
type IPlugin interface {
	// GetPluginInfo 获取插件信息
	GetPluginInfo() PluginInfo
	// Start 开启工作
	Start()
	// preLoad 插件预加载
	PreLoad()
}

var plugins []IPlugin

func RegisterPlugin(plugin IPlugin) {
	plugins = append(plugins, plugin)
}
func loadPlugin() {
	for _, plugin := range plugins {
		go func(plugin IPlugin) {
			plugin.Start()
			logrus.Infof("[bot] load plugin %s success", plugin.GetPluginInfo().PluginName)
		}(plugin)
	}
}

func preloadPlugin() {
	for _, plugin := range plugins {
		plugin.PreLoad()
	}
}
