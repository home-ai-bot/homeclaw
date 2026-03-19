// Package miio 提供 MIoT 网络监控实现
// 对应 Python 版 miot/miot_network.py 中的 MIoTNetwork
package miio

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/event"
	"github.com/sipeed/picoclaw/pkg/logger"
)

// ---------- 默认检测地址 ----------

var (
	// DefaultIPAddressList 默认 IP 检测地址
	DefaultIPAddressList = []string{
		"1.2.4.8", // CNNIC sDNS
		"8.8.8.8", // Google Public DNS
		"9.9.9.9", // Quad9
	}
	// DefaultURLAddressList 默认 HTTP 检测地址
	DefaultURLAddressList = []string{
		"https://www.bing.com",
		"https://www.google.com",
		"https://www.baidu.com",
	}
)

// ---------- 默认常量 ----------

const (
	// defaultLocalNetRefreshInterval 默认刷新间隔
	defaultLocalNetRefreshInterval = 30 * time.Second
	// defaultDetectTimeout 默认检测超时（秒）
	defaultDetectTimeout = 6.0
)

// ---------- 回调类型 ----------

// NetworkStatusHandler 网络状态变更回调
type NetworkStatusHandler func(status bool)

// NetworkInfoHandler 网络接口变更回调
type NetworkInfoHandler func(status InterfaceStatus, info NetworkInterfaceInfo)

// ---------- 配置 ----------

// LocalNetConfig LocalNet 配置
type LocalNetConfig struct {
	IPAddrList      []string      // 用于 ping 检测的 IP 列表
	URLAddrList     []string      // 用于 HTTP 检测的 URL 列表
	RefreshInterval time.Duration // 刷新间隔
}

// ---------- LocalNet ----------

// LocalNet MIoT 网络监控
// 对应 Python 版 MIoTNetwork
// 实现 NetworkMonitor 接口，同时提供网络连通性检测
type LocalNet struct {
	mu sync.RWMutex

	// 检测地址映射：地址 -> 上次延迟（秒），defaultDetectTimeout 表示不可达
	ipAddrMap   map[string]float64
	httpAddrMap map[string]float64

	// HTTP 客户端
	httpClient *http.Client

	// 刷新控制
	refreshInterval time.Duration
	ctx             context.Context
	cancel          context.CancelFunc

	// 状态
	networkStatus bool
	networkInfo   map[string]NetworkInterfaceInfo

	// 订阅者
	statusSubs map[string]NetworkStatusHandler
	infoSubs   map[string]NetworkInfoHandler

	// 初始化完成信号
	doneCh   chan struct{}
	doneOnce sync.Once
}

// NewLocalNet 创建 LocalNet 实例
func NewLocalNet(cfg *LocalNetConfig) *LocalNet {
	if cfg == nil {
		cfg = &LocalNetConfig{}
	}

	ipList := cfg.IPAddrList
	if len(ipList) == 0 {
		ipList = DefaultIPAddressList
	}
	urlList := cfg.URLAddrList
	if len(urlList) == 0 {
		urlList = DefaultURLAddressList
	}

	ipMap := make(map[string]float64, len(ipList))
	for _, ip := range ipList {
		ipMap[ip] = defaultDetectTimeout
	}
	httpMap := make(map[string]float64, len(urlList))
	for _, u := range urlList {
		httpMap[u] = defaultDetectTimeout
	}

	refreshInterval := cfg.RefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = defaultLocalNetRefreshInterval
	}

	return &LocalNet{
		ipAddrMap:       ipMap,
		httpAddrMap:     httpMap,
		httpClient:      &http.Client{Timeout: time.Duration(defaultDetectTimeout) * time.Second},
		refreshInterval: refreshInterval,
		networkStatus:   false,
		networkInfo:     make(map[string]NetworkInterfaceInfo),
		statusSubs:      make(map[string]NetworkStatusHandler),
		infoSubs:        make(map[string]NetworkInfoHandler),
		doneCh:          make(chan struct{}),
	}
}

// Start 启动网络监控，阻塞直到首次检测完成
func (n *LocalNet) Start(ctx context.Context) error {
	n.ctx, n.cancel = context.WithCancel(ctx)
	go n.refreshLoop()

	// 等待首次检测完成
	select {
	case <-n.doneCh:
		return nil
	case <-n.ctx.Done():
		return n.ctx.Err()
	}
}

// Stop 停止网络监控
func (n *LocalNet) Stop() {
	if n.cancel != nil {
		n.cancel()
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	n.networkStatus = false
	n.networkInfo = make(map[string]NetworkInterfaceInfo)
	n.statusSubs = make(map[string]NetworkStatusHandler)
	n.infoSubs = make(map[string]NetworkInfoHandler)
}

// ---------- NetworkMonitor 接口实现 ----------

// GetNetworkInfo 获取当前所有网络接口信息
func (n *LocalNet) GetNetworkInfo() map[string]NetworkInterfaceInfo {
	n.mu.RLock()
	defer n.mu.RUnlock()

	result := make(map[string]NetworkInterfaceInfo, len(n.networkInfo))
	for k, v := range n.networkInfo {
		result[k] = v
	}
	return result
}

// ---------- 网络状态 ----------

// NetworkStatus 返回当前网络连通状态
func (n *LocalNet) NetworkStatus() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.networkStatus
}

// ---------- 地址列表更新 ----------

// UpdateAddrList 更新检测地址列表
func (n *LocalNet) UpdateAddrList(ipAddrList, urlAddrList []string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(ipAddrList) == 0 {
		ipAddrList = DefaultIPAddressList
	}
	newIPMap := make(map[string]float64, len(ipAddrList))
	for _, ip := range ipAddrList {
		if ts, ok := n.ipAddrMap[ip]; ok {
			newIPMap[ip] = ts
		} else {
			newIPMap[ip] = defaultDetectTimeout
		}
	}
	n.ipAddrMap = newIPMap

	if len(urlAddrList) == 0 {
		urlAddrList = DefaultURLAddressList
	}
	newHTTPMap := make(map[string]float64, len(urlAddrList))
	for _, u := range urlAddrList {
		if ts, ok := n.httpAddrMap[u]; ok {
			newHTTPMap[u] = ts
		} else {
			newHTTPMap[u] = defaultDetectTimeout
		}
	}
	n.httpAddrMap = newHTTPMap
}

// Refresh 手动触发刷新
func (n *LocalNet) Refresh() {
	go n.updateStatusAndInfo()
}

// ---------- 网络检测 ----------

// GetNetworkStatus 检测当前网络连通性
func (n *LocalNet) GetNetworkStatus() bool {
	// 先用历史最快的 IP 进行快速检测
	bestIP, bestIPTs := n.bestAddr(n.ipAddrMap)
	if bestIPTs < defaultDetectTimeout {
		if n.pingMulti([]string{bestIP}) {
			return true
		}
	}

	// 再用历史最快的 URL 进行快速检测
	bestURL, bestURLTs := n.bestAddr(n.httpAddrMap)
	if bestURLTs < defaultDetectTimeout {
		if n.httpMulti([]string{bestURL}) {
			return true
		}
	}

	// 全量检测
	pingOK := n.pingMulti(nil)
	httpOK := n.httpMulti(nil)
	return pingOK || httpOK
}

// PingMulti 并发 ping 多个地址，返回是否有任何可达
func (n *LocalNet) PingMulti(ipList []string) bool {
	return n.pingMulti(ipList)
}

// HTTPMulti 并发 HTTP 检测多个 URL，返回是否有任何可达
func (n *LocalNet) HTTPMulti(urlList []string) bool {
	return n.httpMulti(urlList)
}

// ---------- 内部方法 ----------

// bestAddr 从地址映射中找出延迟最低的地址
func (n *LocalNet) bestAddr(m map[string]float64) (string, float64) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var bestAddr string
	bestTs := defaultDetectTimeout
	for addr, ts := range m {
		if ts < bestTs {
			bestAddr = addr
			bestTs = ts
		}
	}
	return bestAddr, bestTs
}

// refreshLoop 定时刷新循环
func (n *LocalNet) refreshLoop() {
	// 首次立即执行
	n.updateStatusAndInfo()

	ticker := time.NewTicker(n.refreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-n.ctx.Done():
			return
		case <-ticker.C:
			n.updateStatusAndInfo()
		}
	}
}

// updateStatusAndInfo 更新网络状态和接口信息
func (n *LocalNet) updateStatusAndInfo() {
	defer func() {
		// 标记首次检测完成
		n.doneOnce.Do(func() {
			close(n.doneCh)
		})
	}()

	status := n.GetNetworkStatus()
	infos := n.detectNetworkInterfaces()

	n.mu.Lock()
	defer n.mu.Unlock()

	// 处理网络状态变更
	if n.networkStatus != status {
		n.networkStatus = status
		// Publish EventTypeNet event via the event center
		evt := event.NewEvent(event.EventTypeNet, "local_net", &event.NetData{
			Kind:   "status",
			Online: status,
		})
		event.GetCenter().Publish(evt)
	}

	// 处理接口变更：检查已有接口
	for name := range n.networkInfo {
		if newInfo, ok := infos[name]; ok {
			// 接口仍存在 —— 检查是否有更新
			oldInfo := n.networkInfo[name]
			if newInfo.IP != oldInfo.IP || newInfo.Netmask != oldInfo.Netmask {
				n.networkInfo[name] = newInfo
				n.callNetworkInfoChange(InterfaceStatusUpdate, newInfo)
			}
			delete(infos, name)
		} else {
			// 接口已移除
			removed := n.networkInfo[name]
			delete(n.networkInfo, name)
			n.callNetworkInfoChange(InterfaceStatusRemove, removed)
		}
	}

	// 新增接口
	for name, info := range infos {
		n.networkInfo[name] = info
		n.callNetworkInfoChange(InterfaceStatusAdd, info)
	}
}

// callNetworkInfoChange 通知所有订阅者接口变更（需持有写锁）
func (n *LocalNet) callNetworkInfoChange(status InterfaceStatus, info NetworkInterfaceInfo) {
	// Publish EventTypeNet event via the event center
	evt := event.NewEvent(event.EventTypeNet, "local_net", &event.NetData{
		Kind:    "interface",
		Status:  int(status),
		Name:    info.Name,
		IP:      info.IP,
		Netmask: info.Netmask,
		NetSeg:  info.NetSeg,
	})
	event.GetCenter().Publish(evt)
}

// pingMulti 并发 ping，返回是否有任何地址可达
func (n *LocalNet) pingMulti(ipList []string) bool {
	n.mu.RLock()
	if len(ipList) == 0 {
		ipList = make([]string, 0, len(n.ipAddrMap))
		for ip := range n.ipAddrMap {
			ipList = append(ipList, ip)
		}
	}
	n.mu.RUnlock()

	type result struct {
		addr string
		ts   float64
	}

	ch := make(chan result, len(ipList))
	for _, addr := range ipList {
		go func(a string) {
			ch <- result{addr: a, ts: n.ping(a)}
		}(addr)
	}

	anyOK := false
	n.mu.Lock()
	for range ipList {
		r := <-ch
		if _, ok := n.ipAddrMap[r.addr]; ok {
			n.ipAddrMap[r.addr] = r.ts
		}
		if r.ts < defaultDetectTimeout {
			anyOK = true
		}
	}
	n.mu.Unlock()
	return anyOK
}

// httpMulti 并发 HTTP 检测，返回是否有任何 URL 可达
func (n *LocalNet) httpMulti(urlList []string) bool {
	n.mu.RLock()
	if len(urlList) == 0 {
		urlList = make([]string, 0, len(n.httpAddrMap))
		for u := range n.httpAddrMap {
			urlList = append(urlList, u)
		}
	}
	n.mu.RUnlock()

	type result struct {
		url string
		ts  float64
	}

	ch := make(chan result, len(urlList))
	for _, u := range urlList {
		go func(url string) {
			ch <- result{url: url, ts: n.httpCheck(url)}
		}(u)
	}

	anyOK := false
	n.mu.Lock()
	for range urlList {
		r := <-ch
		if _, ok := n.httpAddrMap[r.url]; ok {
			n.httpAddrMap[r.url] = r.ts
		}
		if r.ts < defaultDetectTimeout {
			anyOK = true
		}
	}
	n.mu.Unlock()
	return anyOK
}

// ping 使用系统 ping 命令检测地址延迟，返回秒数
func (n *LocalNet) ping(address string) float64 {
	start := time.Now()
	timeoutSec := int(defaultDetectTimeout)

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(n.ctx,
			"ping", "-n", "1", "-w", fmt.Sprintf("%d", timeoutSec*1000), address)
	} else {
		cmd = exec.CommandContext(n.ctx,
			"ping", "-c", "1", "-W", fmt.Sprintf("%d", timeoutSec), address)
	}

	if err := cmd.Run(); err != nil {
		logger.DebugCF("LocalNet", "ping failed", map[string]any{"address": address, "err": err})
		return defaultDetectTimeout
	}
	return time.Since(start).Seconds()
}

// httpCheck 通过 HTTP GET 检测 URL 可达性，返回秒数
func (n *LocalNet) httpCheck(url string) float64 {
	start := time.Now()
	resp, err := n.httpClient.Get(url)
	if err != nil {
		return defaultDetectTimeout
	}
	resp.Body.Close()
	return time.Since(start).Seconds()
}

// detectNetworkInterfaces 获取本机所有有效 IPv4 网络接口信息
func (n *LocalNet) detectNetworkInterfaces() map[string]NetworkInterfaceInfo {
	results := make(map[string]NetworkInterfaceInfo)

	ifaces, err := net.Interfaces()
	if err != nil {
		logger.ErrorCF("LocalNet", "failed to get network interfaces", map[string]any{"err": err})
		return results
	}

	for _, iface := range ifaces {
		name := iface.Name

		// 跳过 hassio 和 docker* 接口
		if name == "hassio" || strings.HasPrefix(name, "docker") {
			continue
		}

		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logger.DebugCF("LocalNet", "failed to get addresses for interface", map[string]any{"name": name, "err": err})
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipNet.IP.To4()
			if ip4 == nil {
				continue
			}

			// 跳过 loopback
			if ip4.IsLoopback() {
				continue
			}

			ipStr := ip4.String()
			maskStr := net.IP(ipNet.Mask).String()
			netSeg := calcNetworkAddress(ip4, ipNet.Mask)

			results[name] = NetworkInterfaceInfo{
				Name:    name,
				IP:      ipStr,
				Netmask: maskStr,
				NetSeg:  netSeg,
			}
			// 每个接口只取第一个有效 IPv4 地址
			break
		}
	}

	return results
}

// calcNetworkAddress 计算网络段地址
func calcNetworkAddress(ip net.IP, mask net.IPMask) string {
	ip4 := ip.To4()
	if ip4 == nil || len(mask) != 4 {
		return ""
	}
	network := make(net.IP, 4)
	for i := 0; i < 4; i++ {
		network[i] = ip4[i] & mask[i]
	}
	return network.String()
}
