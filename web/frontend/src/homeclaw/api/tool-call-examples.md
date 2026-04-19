/**
 * Smart Home Tool Call Examples
 * 
 * This file demonstrates how to use the new callTool API in smart home pages.
 */

import { callTool } from "@/homeclaw/api/device-command-executor"
import { useSmartHomeWebSocket } from "@/homeclaw/hooks/use-smart-home-websocket"

// ═══════════════════════════════════════════════════════════
// Example 1: Using callTool directly (for custom logic)
// ═══════════════════════════════════════════════════════════

/**
 * Execute a device operation with response waiting
 */
async function example_DeviceOperation() {
  const result = await callTool(
    {
      toolName: "hc_cli",
      method: "exe",
      brand: "xiaomi",
      params: {
        from_id: "device_123",
        from: "xiaomi",
        ops: "turn_on",
      },
    },
    {
      timeout: 60000,
      waitForResponse: true,
      successMessage: "设备已开启",
    }
  )

  if (result.success) {
    console.log("Success:", result.message)
  } else {
    console.error("Failed:", result.error)
  }
}

/**
 * Fire-and-forget mode for long-running operations
 */
async function example_FireAndForget() {
  const result = await callTool(
    {
      toolName: "hc_cli",
      method: "generate_ops",
      brand: "tuya",
      params: {
        from_id: "device_456",
        from: "tuya",
        device_name: "Smart Light",
      },
    },
    {
      waitForResponse: false, // Don't wait for response
      successMessage: "已发送请求，结果将在日志中显示",
    }
  )

  // Returns immediately
  console.log("Fire-and-forget:", result.fireAndForget) // true
}

/**
 * Xiaomi login
 */
async function example_XiaomiLogin() {
  const result = await callTool(
    {
      toolName: "xiaomi",
      method: "login",
      brand: "xiaomi",
      params: {
        username: "user@example.com",
        password: "password123",
      },
    },
    {
      successMessage: "登录成功",
    }
  )

  return result
}

/**
 * Tuya token save
 */
async function example_TuyaSaveToken() {
  const result = await callTool(
    {
      toolName: "tuya",
      method: "save_token",
      brand: "tuya",
      params: {
        token: "your_access_token_here",
      },
    },
    {
      successMessage: "令牌已保存",
    }
  )

  return result
}

// ═══════════════════════════════════════════════════════════
// Example 2: Using executeToolCall from hook (recommended)
// ═══════════════════════════════════════════════════════════

/**
 * React component example using the hook
 */
function example_Component() {
  const { executeToolCall, wsStatus } = useSmartHomeWebSocket()

  const handleTurnOnDevice = async () => {
    const result = await executeToolCall(
      {
        toolName: "hc_cli",
        method: "exe",
        brand: "xiaomi",
        params: {
          from_id: "device_123",
          from: "xiaomi",
          ops: "turn_on",
        },
      },
      {
        successMessage: "设备已开启",
      }
    )

    // Result is automatically logged to the communication panel
    if (result.success) {
      // Additional handling if needed
      console.log("Device turned on")
    }
  }

  const handleGenerateOps = async () => {
    // Fire-and-forget for long operations
    await executeToolCall(
      {
        toolName: "hc_cli",
        method: "generate_ops",
        brand: "tuya",
        params: {
          from_id: "device_456",
          from: "tuya",
        },
      },
      {
        waitForResponse: false,
        successMessage: "已发送生成请求",
      }
    )
  }

  return {
    handleTurnOnDevice,
    handleGenerateOps,
    wsStatus,
  }
}

// ═══════════════════════════════════════════════════════════
// Example 3: Migration from old API
// ═══════════════════════════════════════════════════════════

/**
 * OLD API (deprecated but still works):
 */
async function oldWay() {
  // import { executeDeviceOperation } from "@/homeclaw/api/device-command-executor"
  // const result = await executeDeviceOperation("device_123", "xiaomi", "turn_on")
}

/**
 * NEW API (recommended):
 */
async function newWay() {
  const result = await callTool(
    {
      toolName: "hc_cli",
      method: "exe",
      brand: "xiaomi",
      params: {
        from_id: "device_123",
        from: "xiaomi",
        ops: "turn_on",
      },
    },
    {
      successMessage: "设备已开启",
    }
  )
}

// ═══════════════════════════════════════════════════════════
// Example 4: Custom tool calls for different brands
// ═══════════════════════════════════════════════════════════

/**
 * Xiaomi specific operations
 */
async function xiaomiOperations() {
  // Query device status
  await callTool({
    toolName: "xiaomi",
    method: "query",
    brand: "xiaomi",
    params: { device_id: "123" },
  }, { successMessage: "状态已查询" })

  // Control device
  await callTool({
    toolName: "xiaomi",
    method: "control",
    brand: "xiaomi",
    params: {
      device_id: "123",
      action: "turn_on",
    },
  }, { successMessage: "设备已控制" })
}

/**
 * Tuya specific operations
 */
async function tuyaOperations() {
  // Login
  await callTool({
    toolName: "tuya",
    method: "login",
    brand: "tuya",
    params: {
      region: "AZ",
      email: "user@example.com",
      password: "password",
    },
  }, { successMessage: "登录成功" })

  // Logout
  await callTool({
    toolName: "tuya",
    method: "logout",
    brand: "tuya",
  }, { successMessage: "已退出登录" })
}

/**
 * HomeKit specific operations
 */
async function homekitOperations() {
  // Pair device
  await callTool({
    toolName: "homekit",
    method: "pair",
    brand: "homekit",
    params: {
      device_id: "HomeKit-Device-123",
      pin: "12345678",
    },
  }, { successMessage: "设备配对成功" })

  // Unpair device
  await callTool({
    toolName: "homekit",
    method: "unpair",
    brand: "homekit",
    params: {
      paired_id: "paired-123",
    },
  }, { successMessage: "设备已取消配对" })
}

// ═══════════════════════════════════════════════════════════
// Summary
// ═══════════════════════════════════════════════════════════

/**
 * Key Benefits of the New API:
 * 
 * 1. ✅ Universal: Works with any tool (hc_cli, xiaomi, tuya, homekit, etc.)
 * 2. ✅ Flexible: Supports both async and fire-and-forget modes
 * 3. ✅ Auto-logging: All calls are automatically logged to communication panel
 * 4. ✅ Tooltip support: Shows success messages to users
 * 5. ✅ Type-safe: Full TypeScript support with proper interfaces
 * 6. ✅ Backward compatible: Old executeDeviceOperation() still works
 * 
 * Usage Patterns:
 * 
 * - Quick operations (turn on/off, login, etc.):
 *   → Use waitForResponse: true (default)
 * 
 * - Long operations (generate ops, sync devices, etc.):
 *   → Use waitForResponse: false
 * 
 * - Want automatic logging?
 *   → Use executeToolCall() from useSmartHomeWebSocket hook
 * 
 * - Want manual control?
 *   → Use callTool() directly
 */
