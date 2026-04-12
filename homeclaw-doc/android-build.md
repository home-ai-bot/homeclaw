# Android 编译指南

本文档说明如何将 Homeclaw 编译为 Android 平台的 .so 文件。

## 概述

Homeclaw 为 Android 平台编译的 `.so` 文件**不是传统的动态链接库**，而是**静态链接的完整 Go 可执行文件**，使用 `.so` 扩展名是为了兼容 `picoclaw_fui` 的 Gradle 打包方式。

### 架构说明

```
Flutter App (Dart)
    ↓ MethodChannel
Kotlin Service (PicoClawService.kt)
    ↓ ProcessBuilder 启动进程
libhomeclaw.so (Gateway 进程 - 实际是可执行文件)
libhomeclaw-web.so (Web Console 进程 - 实际是可执行文件)
```

### 编译输出

| 文件 | 大小 | 说明 |
|------|------|------|
| `libhomeclaw.so` | ~28MB | Gateway 核心服务 |
| `libhomeclaw-web.so` | ~21MB | Web Console 界面 |

## 快速开始

### 方法一：使用 PowerShell 脚本（推荐）

```powershell
# 编译 arm64-v8a（现代手机，推荐）
.\scripts\build-android.ps1

# 编译 armeabi-v7a（旧设备）
.\scripts\build-android.ps1 -Architecture arm

# 编译 x86_64（模拟器）
.\scripts\build-android.ps1 -Architecture amd64

# 编译所有架构
.\scripts\build-android.ps1 -AllArchitectures

# 只编译 Gateway
.\scripts\build-android.ps1 -SkipWeb

# 只编译 Web Console
.\scripts\build-android.ps1 -SkipGateway
```

### 方法二：使用 Make

```bash
# 编译 arm64-v8a (Gateway + Web)
make build-android-full

# 只编译 Gateway
make build-android

# 只编译 Web Console
make build-android-web

# 编译所有架构
make build-android-all
```

### 方法三：手动编译

```powershell
# 设置环境变量
$env:CGO_ENABLED = "0"
$env:GOOS = "linux"
$env:GOARCH = "arm64"

# 编译 Gateway
go build -v -tags goolm,stdjson -ldflags "-s -w" `
  -o build\android\arm64-v8a\libhomeclaw.so `
  .\cmd\picoclaw

# 编译 Web Console
cd web\backend
go build -ldflags "-s -w" `
  -o ..\..\build\android\arm64-v8a\libhomeclaw-web.so `
  .
```

## 支持的平台

| 架构 | Gradle ABI | 设备类型 |
|------|-----------|---------|
| `arm64` | arm64-v8a | 现代 64 位 ARM 设备（推荐） |
| `arm` | armeabi-v7a | 旧 32 位 ARM 设备 |
| `amd64` | x86_64 | Android 模拟器 |

## 集成到 Flutter 项目

### 1. 复制 .so 文件

```powershell
# 假设 Flutter 项目在 G:\code\homeclaw-fui
copy build\android\arm64-v8a\libhomeclaw.so G:\code\homeclaw-fui\android\app\src\main\jniLibs\arm64-v8a\
copy build\android\arm64-v8a\libhomeclaw-web.so G:\code\homeclaw-fui\android\app\src\main\jniLibs\arm64-v8a\
```

### 2. 配置 build.gradle.kts

在 `android/app/build.gradle.kts` 中添加：

```kotlin
android {
    // ... 其他配置 ...
    
    packaging {
        jniLibs {
            // 不要 strip libhomeclaw*.so（它们不是标准动态库）
            keepDebugSymbols += "**/libhomeclaw.so"
            keepDebugSymbols += "**/libhomeclaw-web.so"
            // 不压缩，直接从 APK 中映射使用
            useLegacyPackaging = true
        }
    }
}
```

### 3. Kotlin Service 实现

参考 `G:\code\picoclaw_fui\android\app\src\main\kotlin\com\sipeed\picoclaw\service\PicoClawService.kt`

核心逻辑：
- 从 `nativeLibraryDir` 获取 .so 文件
- 如不存在，从 APK 中提取
- 设置可执行权限
- 使用 `ProcessBuilder` 启动进程
- 通过 HTTP API 通信（localhost:18800）

### 4. Flutter Dart 端调用

```dart
class HomeclawChannel {
  static const _channel = MethodChannel('com.sipeed.homeclaw/homeclaw');
  
  static Future<bool> startService({int port = 18800, String args = ''}) async {
    final result = await _channel.invokeMethod<bool>('startService', {
      'port': port,
      'args': args,
    });
    return result ?? false;
  }
  
  static Future<bool> stopService() async {
    final result = await _channel.invokeMethod<bool>('stopService');
    return result ?? false;
  }
}
```

## 环境变量

Gateway 进程需要以下环境变量：

```kotlin
private fun buildEnvironment(): Map<String, String> {
    val homeclawHome = File(filesDir, "homeclaw")
    homeclawHome.mkdirs()
    
    return mapOf(
        "HOME" to filesDir.absolutePath,
        "HOMECLAW_HOME" to homeclawHome.absolutePath,
        "HOMECLAW_CONFIG" to File(homeclawHome, "config.json").absolutePath,
        "HOMECLAW_BINARY" to getGatewayBinaryFile().absolutePath,
        "PATH" to "/system/bin:/system/xbin",
        "LANG" to "en_US.UTF-8",
    )
}
```

## 验证编译结果

### 检查文件大小

```powershell
Get-ChildItem build\android\arm64-v8a\*.so | 
    Format-Table Name, @{Label="Size(MB)";Expression={[math]::Round($_.Length/1MB,2)}}
```

预期输出：
- `libhomeclaw.so` ≈ 28MB
- `libhomeclaw-web.so` ≈ 21MB

### 在 Android 设备上测试

```bash
# 推送 .so 到设备
adb push build/android/arm64-v8a/libhomeclaw.so /data/local/tmp/

# 设置可执行权限
adb shell chmod +x /data/local/tmp/libhomeclaw.so

# 测试执行
adb shell /data/local/tmp/libhomeclaw.so version
```

## 编译参数说明

| 参数 | 说明 |
|------|------|
| `CGO_ENABLED=0` | 禁用 CGO，纯 Go 编译，无需 NDK |
| `GOOS=linux` | Android 基于 Linux 内核 |
| `GOARCH=arm64` | 目标架构（arm64/arm/amd64） |
| `GOARM=7` | ARM v7 指令集（仅 arm 架构需要） |
| `-ldflags "-s -w"` | 去掉调试信息，减小文件大小 |

## 注意事项

1. **不需要 Android NDK**：使用纯 Go 交叉编译
2. **不需要 CGO**：`CGO_ENABLED=0`
3. **文件命名**：必须使用 `lib*.so` 格式
4. **架构匹配**：确保 .so 架构与目标设备匹配
5. **可执行权限**：从 APK 提取后需要 `setExecutable(true)`
6. **进程隔离**：每个 .so 是独立进程，通过 HTTP 通信

## 故障排除

### 编译失败

```powershell
# 清理缓存
go clean -cache

# 重新下载依赖
go mod download
go mod tidy

# 重新编译
.\scripts\build-android.ps1
```

### 文件大小异常

- Gateway 应该在 25-30MB 之间
- Web Console 应该在 18-23MB 之间
- 如果文件过小，可能编译失败

### APK 打包后无法运行

1. 检查 `build.gradle.kts` 中的 `useLegacyPackaging = true`
2. 检查是否设置了 `keepDebugSymbols`
3. 确认 .so 文件在 APK 的 `lib/arm64-v8a/` 目录中
4. 确认 Kotlin Service 正确设置可执行权限

## 参考文件

- `scripts/build-android.ps1` - PowerShell 编译脚本
- `Makefile` - Make 编译配置（build-android-* 目标）
- `G:\code\picoclaw_fui\android\app\src\main\kotlin\com\sipeed\picoclaw\service\PicoClawService.kt` - Kotlin 服务实现参考
- `G:\code\picoclaw_fui\android\app\build.gradle.kts` - Gradle 配置参考

## 相关文件

编译输出的 .so 文件应放置于：

```
<flutter-project>/
└── android/
    └── app/
        └── src/
            └── main/
                └── jniLibs/
                    ├── arm64-v8a/
                    │   ├── libhomeclaw.so
                    │   └── libhomeclaw-web.so
                    ├── armeabi-v7a/
                    │   ├── libhomeclaw.so
                    │   └── libhomeclaw-web.so
                    └── x86_64/
                        ├── libhomeclaw.so
                        └── libhomeclaw-web.so
```
