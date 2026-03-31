---
name: mi-camera
description: Capture and analyze images from Xiaomi/Mi Home cameras. Use when the user wants to see what's happening on camera, check camera view, capture a snapshot, or analyze camera footage. Triggers on commands like "what does the camera see", "show me the living room camera", "is anyone at the door", "check the baby monitor", or any request to view or analyze Xiaomi camera images.
---

# Mi Camera - Capture and Analyze Xiaomi Camera Images

Capture frames from Xiaomi/Mi Home cameras and analyze visual content using vision AI.

**Tools Used:**
- **`hc_list_cameras`** - List all camera devices with RTSP URLs
- **`hc_private_camera_analyze`** - Capture and analyze camera frame

## Workflow

1. **Find Camera** → `hc_list_cameras`
2. **Capture & Analyze** → `hc_private_camera_analyze`

## Step 1: Find Camera Device

```
hc_list_cameras
```

Returns camera list with `from_id`, `from`, `name`, `type`, `online`, `space_name`, `rtsp_url`

Example response:
```json
{
  "cameras": [
    {
      "from_id": "cam001",
      "from": "xiaomi",
      "name": "Living Room Camera",
      "type": "chuangmi.camera.ipc019e",
      "online": true,
      "space_name": "Living Room",
      "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"
    }
  ]
}
```

## Step 2: Capture & Analyze Frame

```
hc_private_camera_analyze
- rtsp_url: the constructed RTSP URL from step 2
- prompt: (optional) specific question about the image, e.g. "Is there anyone at the door?"
```

Returns JSON with:
```json
{
  "analysis": "Description of what's visible in the camera frame...",
  "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_12345"
}
```

## Examples

### Example 1: Check What Camera Sees

User: "What does the living room camera see?"

```
1. hc_list_cameras
   → {"cameras": [{"from_id": "cam001", "name": "Living Room Camera", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001", ...}]}

2. hc_private_camera_analyze {
     "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001",
     "prompt": "Describe what you see in this living room camera view"
   }
   → {"analysis": "The living room appears well-lit with a sofa on the left, a TV mounted on the wall, and a coffee table in the center. No people are visible in the frame.", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"}
```

### Example 2: Check if Someone is at the Door

User: "Is anyone at the front door?"

```
1. hc_list_cameras
   → {"cameras": [{"from_id": "door123", "name": "Door Camera", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_door123", ...}]}

2. hc_private_camera_analyze {
     "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_door123",
     "prompt": "Is there anyone at the door? Describe any people or activity visible."
   }
   → {"analysis": "Yes, there is a delivery person standing at the door holding a package. They appear to be waiting.", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_door123"}
```

### Example 3: Check Baby Monitor

User: "Is the baby sleeping?"

```
1. hc_list_cameras
   → {"cameras": [{"from_id": "baby456", "name": "Baby Room Camera", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_baby456", ...}]}

2. hc_private_camera_analyze {
     "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_baby456",
     "prompt": "Is there a baby in the crib? Are they sleeping or awake?"
   }
   → {"analysis": "A baby is visible in the crib, appears to be sleeping peacefully. The room is dimly lit.", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_baby456"}
```

### Example 4: Pet Feeder Camera Check

User: "Did the cat eat the food?"

```
1. hc_list_cameras
   → {"cameras": [{"from_id": "feeder789", "name": "Pet Feeder", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_feeder789", ...}]}

2. hc_private_camera_analyze {
     "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_feeder789",
     "prompt": "Is there a cat near the feeder? Is the food bowl empty or full?"
   }
   → {"analysis": "The food bowl appears to be empty. No cat is currently visible near the feeder.", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_feeder789"}
```

### Example 5: General Security Check

User: "Check all cameras for any activity"

```
1. hc_list_cameras
   → {"cameras": [
       {"from_id": "cam001", "name": "Living Room", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"},
       {"from_id": "cam002", "name": "Backyard", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam002"}
     ]}

2. For each camera, analyze:
   
   a) hc_private_camera_analyze {
        "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001",
        "prompt": "Describe any activity or people visible"
      }
   
   b) hc_private_camera_analyze {
        "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam002",
        "prompt": "Describe any activity or people visible"
      }

3. Summarize findings to user
```

## Camera Device Type Patterns

Xiaomi camera devices can be identified by model/type containing:
- `.camera.` - Standard cameras (e.g., `chuangmi.camera.ipc019e`)
- `.cateye.` - Smart doorbells/peepholes (e.g., `madv.cateye.mowl02`)
- `.feeder.` - Pet feeders with camera (e.g., `mmgg.feeder.fi1`)

## Error Handling

- **Camera not found**: Ask user for specific camera name or room
- **RTSP connection failed**: Camera may be offline or go2rtc not running
- **FFmpeg not available**: FFmpeg must be installed for frame capture
- **Analysis failed**: Vision model may be unavailable

## Prerequisites

- Xiaomi devices must be synced (`mi_sync_devices`)
- go2rtc must be running to serve RTSP streams
- FFmpeg must be installed for frame capture
- Vision-capable LLM must be configured as intent provider
