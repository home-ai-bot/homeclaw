package config

// DeviceActionsJSON contains the JSON schema of smart home device types and their supported actions.
const DeviceActionsJSON = `{
  "light": ["turn_on", "turn_off", "get_state", "get_brightness", "set_brightness", "get_color_temp", "set_color_temp", "get_rgb_color", "set_rgb_color", "get_effect", "set_effect"],
  "switch": ["turn_on", "turn_off", "get_state"],
  "camera": ["get_image", "get_stream", "enable_motion_detection", "disable_motion_detection"],
  "vacuum": ["start", "pause", "stop", "return_to_base", "locate", "set_fan_speed", "clean_spot", "clean_zone", "get_state"],
  "fan": ["turn_on", "turn_off", "get_state", "get_percentage", "set_percentage", "get_preset_mode", "set_preset_mode", "get_oscillate", "oscillate", "get_direction", "set_direction"],
  "climate": ["turn_on", "turn_off", "get_state", "set_hvac_mode", "set_temperature", "set_humidity", "set_fan_mode", "set_swing_mode", "set_preset_mode"],
  "cover": ["open", "close", "stop", "set_position", "get_state"],
  "humidifier": ["turn_on", "turn_off", "get_state", "set_mode", "set_humidity"],
  "water_heater": ["turn_on", "turn_off", "get_state", "set_temperature", "set_operation_mode"],
  "media_player": ["turn_on", "turn_off", "get_state", "play", "pause", "stop", "next_track", "previous_track", "set_volume", "mute", "select_source"],
  "sensor": ["get_state"]
}`

// DeviceType 设备类型
type DeviceType string

const (
	DeviceTypeLight       DeviceType = "light"        // 灯/照明
	DeviceTypeSwitch      DeviceType = "switch"       // 开关/插座
	DeviceTypeCamera      DeviceType = "camera"       // 摄像头
	DeviceTypeVacuum      DeviceType = "vacuum"       // 扫地机/拖地机
	DeviceTypeFan         DeviceType = "fan"          // 风扇
	DeviceTypeClimate     DeviceType = "climate"      // 空调/暖气
	DeviceTypeCover       DeviceType = "cover"        // 窗帘/晾衣架/窗户
	DeviceTypeHumidifier  DeviceType = "humidifier"   // 加湿器/除湿机
	DeviceTypeWaterHeater DeviceType = "water_heater" // 热水器/饮水机
	DeviceTypeMediaPlayer DeviceType = "media_player" // 电视/音箱/投影仪
	DeviceTypeSensor      DeviceType = "sensor"       // 传感器(只读)
)

// ActionType 操作类型
type ActionType string

// 通用操作
const (
	ActionTurnOn   ActionType = "turn_on"   // 打开
	ActionTurnOff  ActionType = "turn_off"  // 关闭
	ActionGetState ActionType = "get_state" // 获取状态
)

// 灯光操作
const (
	ActionGetBrightness ActionType = "get_brightness" // 获取亮度
	ActionSetBrightness ActionType = "set_brightness" // 设置亮度
	ActionGetColorTemp  ActionType = "get_color_temp" // 获取色温
	ActionSetColorTemp  ActionType = "set_color_temp" // 设置色温
	ActionGetRGBColor   ActionType = "get_rgb_color"  // 获取RGB颜色
	ActionSetRGBColor   ActionType = "set_rgb_color"  // 设置RGB颜色
	ActionGetEffect     ActionType = "get_effect"     // 获取灯效
	ActionSetEffect     ActionType = "set_effect"     // 设置灯效
)

// 摄像头操作
const (
	ActionGetImage               ActionType = "get_image"                // 获取图像
	ActionGetStream              ActionType = "get_stream"               // 获取视频流
	ActionEnableMotionDetection  ActionType = "enable_motion_detection"  // 开启移动侦测
	ActionDisableMotionDetection ActionType = "disable_motion_detection" // 关闭移动侦测
)

// 扫地机操作
const (
	ActionStart        ActionType = "start"          // 开始清扫
	ActionPause        ActionType = "pause"          // 暂停
	ActionStop         ActionType = "stop"           // 停止
	ActionReturnToBase ActionType = "return_to_base" // 回充
	ActionLocate       ActionType = "locate"         // 定位/查找设备
	ActionSetFanSpeed  ActionType = "set_fan_speed"  // 设置吸力档位
	ActionCleanSpot    ActionType = "clean_spot"     // 定点清扫
	ActionCleanZone    ActionType = "clean_zone"     // 划区清扫
)

// 风扇操作
const (
	ActionGetPercentage ActionType = "get_percentage"  // 获取风速百分比
	ActionSetPercentage ActionType = "set_percentage"  // 设置风速百分比
	ActionGetPresetMode ActionType = "get_preset_mode" // 获取预设模式
	ActionSetPresetMode ActionType = "set_preset_mode" // 设置预设模式
	ActionGetOscillate  ActionType = "get_oscillate"   // 获取摇头状态
	ActionOscillate     ActionType = "oscillate"       // 摇头开关
	ActionGetDirection  ActionType = "get_direction"   // 获取风向
	ActionSetDirection  ActionType = "set_direction"   // 设置风向
)

// 空调操作
const (
	ActionSetHVACMode    ActionType = "set_hvac_mode"   // 设置模式
	ActionSetTemperature ActionType = "set_temperature" // 设置目标温度
	ActionSetHumidity    ActionType = "set_humidity"    // 设置目标湿度
	ActionSetFanMode     ActionType = "set_fan_mode"    // 设置风速
	ActionSetSwingMode   ActionType = "set_swing_mode"  // 设置扫风模式
)

// 窗帘操作
const (
	ActionOpen        ActionType = "open"         // 打开
	ActionClose       ActionType = "close"        // 关闭
	ActionSetPosition ActionType = "set_position" // 设置位置
)

// 加湿器操作
const (
	ActionSetMode ActionType = "set_mode" // 设置模式
)

// 热水器操作
const (
	ActionSetOperationMode ActionType = "set_operation_mode" // 设置工作模式
)

// 媒体播放器操作
const (
	ActionPlay          ActionType = "play"           // 播放
	ActionNextTrack     ActionType = "next_track"     // 下一曲
	ActionPreviousTrack ActionType = "previous_track" // 上一曲
	ActionSetVolume     ActionType = "set_volume"     // 设置音量
	ActionMute          ActionType = "mute"           // 静音
	ActionSelectSource  ActionType = "select_source"  // 选择输入源
)

// DeviceActions 设备支持的操作映射
var DeviceActions = map[DeviceType][]ActionType{
	DeviceTypeLight: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionGetBrightness,
		ActionSetBrightness,
		ActionGetColorTemp,
		ActionSetColorTemp,
		ActionGetRGBColor,
		ActionSetRGBColor,
		ActionGetEffect,
		ActionSetEffect,
	},
	DeviceTypeSwitch: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
	},
	DeviceTypeCamera: {
		ActionGetImage,
		ActionGetStream,
		ActionEnableMotionDetection,
		ActionDisableMotionDetection,
	},
	DeviceTypeVacuum: {
		ActionStart,
		ActionPause,
		ActionStop,
		ActionReturnToBase,
		ActionLocate,
		ActionSetFanSpeed,
		ActionCleanSpot,
		ActionCleanZone,
		ActionGetState,
	},
	DeviceTypeFan: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionGetPercentage,
		ActionSetPercentage,
		ActionGetPresetMode,
		ActionSetPresetMode,
		ActionGetOscillate,
		ActionOscillate,
		ActionGetDirection,
		ActionSetDirection,
	},
	DeviceTypeClimate: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionSetHVACMode,
		ActionSetTemperature,
		ActionSetHumidity,
		ActionSetFanMode,
		ActionSetSwingMode,
		ActionSetPresetMode,
	},
	DeviceTypeCover: {
		ActionOpen,
		ActionClose,
		ActionStop,
		ActionSetPosition,
		ActionGetState,
	},
	DeviceTypeHumidifier: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionSetMode,
		ActionSetHumidity,
	},
	DeviceTypeWaterHeater: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionSetTemperature,
		ActionSetOperationMode,
	},
	DeviceTypeMediaPlayer: {
		ActionTurnOn,
		ActionTurnOff,
		ActionGetState,
		ActionPlay,
		ActionPause,
		ActionStop,
		ActionNextTrack,
		ActionPreviousTrack,
		ActionSetVolume,
		ActionMute,
		ActionSelectSource,
	},
	DeviceTypeSensor: {
		ActionGetState, // 传感器只支持获取状态
	},
}

// DeviceTypeNames 设备类型的中文名称
var DeviceTypeNames = map[DeviceType]string{
	DeviceTypeLight:       "灯/照明",
	DeviceTypeSwitch:      "开关/插座",
	DeviceTypeCamera:      "摄像头",
	DeviceTypeVacuum:      "扫地机/拖地机",
	DeviceTypeFan:         "风扇",
	DeviceTypeClimate:     "空调/暖气",
	DeviceTypeCover:       "窗帘/晾衣架/窗户",
	DeviceTypeHumidifier:  "加湿器/除湿机",
	DeviceTypeWaterHeater: "热水器/饮水机",
	DeviceTypeMediaPlayer: "电视/音箱/投影仪",
	DeviceTypeSensor:      "传感器",
}

// ActionTypeNames 操作类型的中文名称
var ActionTypeNames = map[ActionType]string{
	// 通用操作
	ActionTurnOn:   "打开",
	ActionTurnOff:  "关闭",
	ActionGetState: "获取状态",
	// 灯光操作
	ActionGetBrightness: "获取亮度",
	ActionSetBrightness: "设置亮度",
	ActionGetColorTemp:  "获取色温",
	ActionSetColorTemp:  "设置色温",
	ActionGetRGBColor:   "获取RGB颜色",
	ActionSetRGBColor:   "设置RGB颜色",
	ActionGetEffect:     "获取灯效",
	ActionSetEffect:     "设置灯效",
	// 摄像头操作
	ActionGetImage:               "获取图像",
	ActionGetStream:              "获取视频流",
	ActionEnableMotionDetection:  "开启移动侦测",
	ActionDisableMotionDetection: "关闭移动侦测",
	// 扫地机操作
	ActionStart:        "开始清扫",
	ActionPause:        "暂停",
	ActionStop:         "停止",
	ActionReturnToBase: "回充",
	ActionLocate:       "定位/查找设备",
	ActionSetFanSpeed:  "设置吸力档位",
	ActionCleanSpot:    "定点清扫",
	ActionCleanZone:    "划区清扫",
	// 风扇操作
	ActionGetPercentage: "获取风速百分比",
	ActionSetPercentage: "设置风速百分比",
	ActionGetPresetMode: "获取预设模式",
	ActionSetPresetMode: "设置预设模式",
	ActionGetOscillate:  "获取摇头状态",
	ActionOscillate:     "摇头开关",
	ActionGetDirection:  "获取风向",
	ActionSetDirection:  "设置风向",
	// 空调操作
	ActionSetHVACMode:    "设置模式(制冷/制热/除湿/送风/自动)",
	ActionSetTemperature: "设置目标温度",
	ActionSetHumidity:    "设置目标湿度",
	ActionSetFanMode:     "设置风速",
	ActionSetSwingMode:   "设置扫风模式",
	// 窗帘操作
	ActionOpen:        "打开",
	ActionClose:       "关闭",
	ActionSetPosition: "设置位置",
	// 加湿器操作
	ActionSetMode: "设置模式",
	// 热水器操作
	ActionSetOperationMode: "设置工作模式",
	// 媒体播放器操作
	ActionPlay:          "播放",
	ActionNextTrack:     "下一曲",
	ActionPreviousTrack: "上一曲",
	ActionSetVolume:     "设置音量",
	ActionMute:          "静音",
	ActionSelectSource:  "选择输入源",
}

// GetDeviceActions 获取指定设备类型支持的操作列表
func GetDeviceActions(deviceType DeviceType) []ActionType {
	return DeviceActions[deviceType]
}

// GetAllDeviceTypes 获取所有设备类型
func GetAllDeviceTypes() []DeviceType {
	return []DeviceType{
		DeviceTypeLight,
		DeviceTypeSwitch,
		DeviceTypeCamera,
		DeviceTypeVacuum,
		DeviceTypeFan,
		DeviceTypeClimate,
		DeviceTypeCover,
		DeviceTypeHumidifier,
		DeviceTypeWaterHeater,
		DeviceTypeMediaPlayer,
		DeviceTypeSensor,
	}
}

// IsValidAction 检查操作是否对指定设备类型有效
func IsValidAction(deviceType DeviceType, action ActionType) bool {
	actions := DeviceActions[deviceType]
	for _, a := range actions {
		if a == action {
			return true
		}
	}
	return false
}
