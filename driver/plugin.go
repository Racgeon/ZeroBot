package driver

import (
	"github.com/sirupsen/logrus"
	"sync"
)

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
	// PreLoad 插件预加载
	PreLoad()
}

var plugins []IPlugin

func RegisterPlugin(plugin IPlugin) {
	plugins = append(plugins, plugin)
}
func loadPlugin() {
	for _, plugin := range plugins {
		go func(plugin IPlugin) {
			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("[bot] load plugin %s failed: %v", plugin.GetPluginInfo().PluginName, r)
				}
			}()
			plugin.Start()
			logrus.Infof("[bot] load plugin %s success", plugin.GetPluginInfo().PluginName)
		}(plugin)
	}
}

func preloadPlugin() {
	wg := sync.WaitGroup{}
	wg.Add(len(plugins))
	for i, plugin := range plugins {
		go func(i int, plugin IPlugin, plugins *[]IPlugin) {
			defer func() {
				if r := recover(); r != nil {
					logrus.Errorf("[bot] load plugin %s failed: %v", plugin.GetPluginInfo().PluginName, r)
					*plugins = append((*plugins)[:i], (*plugins)[i+1:]...)
					wg.Done()
				}
			}()
			plugin.PreLoad()
			wg.Done()
		}(i, plugin, &plugins)
	}
	wg.Wait()
}
