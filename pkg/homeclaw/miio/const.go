// Package miio 小米 MIoT 协议相关常量定义
// 对应 Python 版 miot/const.py
package miio

import "time"

// ---------- 基础标识 ----------

const (
	// Domain 集成域名
	Domain = "xiaomi_home"
	// DefaultName 集成默认名称
	DefaultName = "Xiaomi Home"
	// DefaultNickName 默认昵称
	DefaultNickName = "Xiaomi"
)

// ---------- HTTP / MQTT 超时 ----------

const (
	// MIHomeHTTPAPITimeout HTTP API 请求超时时间
	MIHomeHTTPAPITimeout = 30 * time.Second
	// MIHomeMQTTKeepalive MQTT 心跳间隔
	MIHomeMQTTKeepalive = 60 * time.Second
	// MIHomeCertExpireMargin 证书提前刷新边距（3天）
	MIHomeCertExpireMargin = 3600 * 24 * 3 * time.Second
	// NetworkRefreshInterval 网络刷新间隔
	NetworkRefreshInterval = 30 * time.Second
)

// ---------- OAuth2 / API 地址 ----------

const (
	// OAuth2ClientID 小米 OAuth2 客户端 ID（Home Assistant 集成注册，请勿修改）
	OAuth2ClientID = "2882303761520251711"
	// OAuth2AuthURL 小米 OAuth2 授权地址
	OAuth2AuthURL = "https://account.xiaomi.com/oauth2/authorize"
	// DefaultOAuth2APIHost 默认 MIoT API 主机
	DefaultOAuth2APIHost = "ha.api.io.mi.com"
	// DefaultCloudBrokerHost 默认云端 MQTT Broker 主机
	DefaultCloudBrokerHost = "ha.mqtt.io.mi.com"
	// OAuthRedirectURL OAuth2 回调地址（Xiaomi OAuth2 服务已注册，请勿修改）
	OAuthRedirectURL = "http://homeassistant.local:8123"
)

// ---------- Spec / 制造商缓存有效期 ----------

const (
	// SpecStdLibEffectiveTime Spec 标准库有效期（14天）
	SpecStdLibEffectiveTime = 3600 * 24 * 14 * time.Second
	// ManufacturerEffectiveTime 制造商信息有效期（14天）
	ManufacturerEffectiveTime = 3600 * 24 * 14 * time.Second
)

// ---------- 云服务器 ----------

const (
	// DefaultCloudServer 默认云服务器区域
	DefaultCloudServer = "cn"
)

// CloudServers 支持的云服务器区域及显示名称
var CloudServers = map[string]string{
	"cn": "中国大陆",
	"de": "Europe",
	"i2": "India",
	"ru": "Russia",
	"sg": "Singapore",
	"us": "United States",
}

// SupportCentralGatewayCtrl 支持中央网关控制的服务器区域
var SupportCentralGatewayCtrl = []string{"cn"}

// ---------- 不支持的设备型号 ----------

// UnsupportedModels 不支持的设备型号列表
var UnsupportedModels = []string{
	"chuangmi.ir.v2",
	"era.airp.cwb03",
	"hmpace.motion.v6nfc",
	"k0918.toothbrush.t700",
}

// ---------- 窗帘死区 ----------

const (
	// DefaultCoverDeadZoneWidth 默认窗帘死区宽度
	DefaultCoverDeadZoneWidth = 0
	// MinCoverDeadZoneWidth 最小窗帘死区宽度
	MinCoverDeadZoneWidth = 0
	// MaxCoverDeadZoneWidth 最大窗帘死区宽度
	MaxCoverDeadZoneWidth = 5
)

// ---------- 控制模式 ----------

const (
	// DefaultCtrlMode 默认控制模式
	DefaultCtrlMode = "auto"
)

// ---------- 国际化 ----------

const (
	// DefaultIntegrationLanguage 默认集成语言
	DefaultIntegrationLanguage = "en"
)

// IntegrationLanguages 支持的集成语言
var IntegrationLanguages = map[string]string{
	"de":      "Deutsch",
	"en":      "English",
	"es":      "Español",
	"fr":      "Français",
	"it":      "Italiano",
	"ja":      "日本語",
	"nl":      "Nederlands",
	"pt":      "Português",
	"pt-BR":   "Português (Brasil)",
	"ru":      "Русский",
	"tr":      "Türkçe",
	"zh-Hans": "简体中文",
	"zh-Hant": "繁體中文",
}

// ---------- 支持的平台 ----------

// SupportedPlatforms Home Assistant 中支持的设备平台列表
var SupportedPlatforms = []string{
	"binary_sensor",
	"button",
	"climate",
	"cover",
	"device_tracker",
	"event",
	"fan",
	"humidifier",
	"light",
	"media_player",
	"notify",
	"number",
	"select",
	"sensor",
	"switch",
	"text",
	"vacuum",
	"water_heater",
}

// ---------- 米家 CA 证书 ----------

// MIHomeCACert 米家根 CA 证书（PEM 格式）
const MIHomeCACert = "-----BEGIN CERTIFICATE-----\n" +
	"MIIBazCCAQ+gAwIBAgIEA/UKYDAMBggqhkjOPQQDAgUAMCIxEzARBgNVBAoTCk1p\n" +
	"amlhIFJvb3QxCzAJBgNVBAYTAkNOMCAXDTE2MTEyMzAxMzk0NVoYDzIwNjYxMTEx\n" +
	"MDEzOTQ1WjAiMRMwEQYDVQQKEwpNaWppYSBSb290MQswCQYDVQQGEwJDTjBZMBMG\n" +
	"ByqGSM49AgEGCCqGSM49AwEHA0IABL71iwLa4//4VBqgRI+6xE23xpovqPCxtv96\n" +
	"2VHbZij61/Ag6jmi7oZ/3Xg/3C+whglcwoUEE6KALGJ9vccV9PmjLzAtMAwGA1Ud\n" +
	"EwQFMAMBAf8wHQYDVR0OBBYEFJa3onw5sblmM6n40QmyAGDI5sURMAwGCCqGSM49\n" +
	"BAMCBQADSAAwRQIgchciK9h6tZmfrP8Ka6KziQ4Lv3hKfrHtAZXMHPda4IYCIQCG\n" +
	"az93ggFcbrG9u2wixjx1HKW4DUA5NXZG0wWQTpJTbQ==\n" +
	"-----END CERTIFICATE-----\n" +
	"-----BEGIN CERTIFICATE-----\n" +
	"MIIBjzCCATWgAwIBAgIBATAKBggqhkjOPQQDAjAiMRMwEQYDVQQKEwpNaWppYSBS\n" +
	"b290MQswCQYDVQQGEwJDTjAgFw0yMjA2MDkxNDE0MThaGA8yMDcyMDUyNzE0MTQx\n" +
	"OFowLDELMAkGA1UEBhMCQ04xHTAbBgNVBAoMFE1JT1QgQ0VOVFJBTCBHQVRFV0FZ\n" +
	"MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEdYrzbnp/0x/cZLZnuEDXTFf8mhj4\n" +
	"CVpZPwgj9e9Ve5r3K7zvu8Jjj7JF1JjQYvEC6yhp1SzBgglnK4L8xQzdiqNQME4w\n" +
	"HQYDVR0OBBYEFCf9+YBU7pXDs6K6CAQPRhlGJ+cuMB8GA1UdIwQYMBaAFJa3onw5\n" +
	"sblmM6n40QmyAGDI5sURMAwGA1UdEwQFMAMBAf8wCgYIKoZIzj0EAwIDSAAwRQIh\n" +
	"AKUv+c8v98vypkGMTzMwckGjjVqTef8xodsy6PhcSCq+AiA/n9mDs62hAo5zXyJy\n" +
	"Bs1s7mqXPf1XgieoxIvs1MqyiA==\n" +
	"-----END CERTIFICATE-----\n"

// MIHomeCACertSHA256 米家 CA 证书 SHA256 指纹
const MIHomeCACertSHA256 = "8b7bf306be3632e08b0ead308249e5f2b2520dc921ad143872d5fcc7c68d6759"
